// Package impact answers "what does changing this symbol touch" as bounded,
// self-explaining traversal over stored structural facts (MR-009). Analysis
// is pure computation: deterministic in the index rows and the request,
// stored nowhere (decision D-147). Results block nothing (spec §31) — they
// feed MR-010's runner and MR-013's gate as explanation, never as verdict.
package impact

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/PsyChaos/mindrail/internal/index"
)

// Breadth is the validation breadth ladder (spec §124). Structural evidence
// alone never recommends above MODULE: direct-only results are TARGETED,
// any fallback engages MODULE, and wider breadths ride an explicit caller
// justification recorded verbatim (decision D-149).
type Breadth string

const (
	TargetedBreadth   Breadth = "TARGETED"
	ModuleBreadth     Breadth = "MODULE"
	PackageBreadth    Breadth = "PACKAGE"
	ServiceBreadth    Breadth = "SERVICE"
	RepositoryBreadth Breadth = "REPOSITORY"
)

// Edge kinds. Direct rode a resolved reference; name-match is a cross-file
// name use capped at the structural name-match confidence; file is the
// changed-file floor; ambiguous names every same-name candidate without
// choosing.
const (
	DirectEdge    = "direct"
	NameMatchEdge = "name-match"
	FileEdge      = "file"
	AmbiguousEdge = "ambiguous"
)

// NameMatchConfidence is the confidence of every name-match and ambiguous
// entry: the structural name-match constant the indexer itself writes on
// every reference row, reused as vocabulary rather than invented per entry
// (decision D-146).
const NameMatchConfidence = 0.5

// Input is one changed symbol, by durable uid or by logical key. A key may
// resolve to several uids across units: every one is analyzed, never chosen
// between. Inputs that resolve nowhere are dropped, never an error —
// traversal judges stored facts, not caller spelling.
type Input struct {
	UID string
	Key string
}

// Request is one analysis. Depth at or below zero means 1 (decision D-150);
// above 1 runs only when passed explicitly. DirectOnly skips every fallback
// and holds TARGETED breadth: the direct-only mode AC-01.4 names.
// BreadthOverride with a Justification records a caller-justified wider
// breadth (path policy, public API rule, explicit invariant scope); without
// it the structural cap holds.
type Request struct {
	Symbols         []Input
	Depth           int
	DirectOnly      bool
	BreadthOverride Breadth
	Justification   string
}

// Via is the source edge one entry rode: the referrer and the words the
// extraction saw. Kind is direct in TASK-01; fallbacks extend it.
type Via struct {
	ReferrerKey  string
	ReferrerPath string
	TargetText   string
	Kind         string
}

// Entry is one affected symbol with its explanation. Confidence is read
// from the reference row, never invented; Depth is the entry's own reverse
// distance; FallbackReason is empty exactly when the edge is direct.
// Invariants lists the bound invariant ids beside the entry's symbols, and
// ScopeJustification names them as explicit scope text when present.
type Entry struct {
	TargetUID          string
	TargetKey          string
	TargetName         string
	TargetPath         string
	Via                Via
	Confidence         float64
	Depth              int
	FallbackReason     string
	Invariants         []string
	ScopeJustification string
}

// Result is one analysis. StructuralBreadth is what the evidence alone
// supports (TARGETED in TASK-01, MODULE once a fallback engages); Breadth
// is the override when justified, else the structural breadth.
type Result struct {
	Entries           []Entry
	StructuralBreadth Breadth
	Breadth           Breadth
	Justification     string
}

// Service traverses stored structural facts. It holds the index store the
// way changes.Service does; callers thread nothing else.
type Service struct {
	indexes *index.Store
}

// New builds a Service over the index store it reads.
func New(indexes *index.Store) (*Service, error) {
	if indexes == nil {
		return nil, errors.New("impact: needs an index store")
	}
	return &Service{indexes: indexes}, nil
}

// Analyze runs one bounded reverse traversal. Depth counts reverse edges
// from each changed symbol; a visited set on uids keeps cycles and shared
// referrers to one entry each.
func (s *Service) Analyze(ctx context.Context, request Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	depth := request.Depth
	if depth <= 0 {
		depth = 1
	}
	result := Result{StructuralBreadth: TargetedBreadth, Justification: request.Justification}
	result.Breadth = result.StructuralBreadth
	if request.BreadthOverride != "" && request.Justification != "" {
		result.Breadth = request.BreadthOverride
	}
	frontier, err := s.resolveInputs(ctx, request.Symbols)
	if err != nil {
		return Result{}, err
	}
	visited := map[string]bool{}
	for _, uid := range frontier {
		visited[uid] = true
	}
	seen := map[[2]string]bool{}
	fallback := false
	for level := 1; level <= depth && len(frontier) > 0; level++ {
		var next []string
		for _, uid := range frontier {
			referrers, err := s.indexes.ReferrersOfUID(ctx, uid)
			if err != nil {
				return Result{}, err
			}
			target := targetOf(ctx, s.indexes, uid)
			for _, referrer := range referrers {
				key := [2]string{referrer.Key, uid}
				if seen[key] {
					continue
				}
				seen[key] = true
				referrerUID, ok, err := s.indexes.UIDForKey(ctx, referrer.UnitID, referrer.Path, referrer.Key)
				if err != nil {
					return Result{}, err
				}
				entry, err := s.withInvariants(ctx, Entry{
					TargetUID:  uid,
					TargetKey:  target.key,
					TargetName: target.name,
					TargetPath: target.path,
					Via: Via{
						ReferrerKey:  referrer.Key,
						ReferrerPath: referrer.Path,
						TargetText:   referrer.TargetText,
						Kind:         DirectEdge,
					},
					Confidence: referrer.Confidence,
					Depth:      level,
				}, uidsOf(ok, referrerUID, uid))
				if err != nil {
					return Result{}, err
				}
				result.Entries = append(result.Entries, entry)
				if level < depth && ok && !visited[referrerUID] {
					visited[referrerUID] = true
					next = append(next, referrerUID)
				}
			}
			if !request.DirectOnly {
				engaged, err := s.fallbacks(ctx, &result, seen, uid, target, level)
				if err != nil {
					return Result{}, err
				}
				fallback = fallback || engaged
			}
		}
		frontier = next
	}
	if fallback {
		result.StructuralBreadth = ModuleBreadth
		result.Breadth = ModuleBreadth
	}
	return result, nil
}

// fallbacks adds the weak layers for one analyzed symbol: the changed-file
// floor always, then the name layer — one name-match entry per unresolved
// name user, or one ambiguous entry naming every candidate when several
// declarations share the name (decision D-154). The climb never follows
// fallback edges: weak signals do not amplify. It reports whether any
// fallback engaged.
func (s *Service) fallbacks(ctx context.Context, result *Result, seen map[[2]string]bool, uid string, target targetFact, level int) (bool, error) {
	unitID, path, found, err := s.indexes.UnitForUID(ctx, uid)
	if err != nil {
		return false, err
	}
	if found {
		file, err := s.withInvariants(ctx, Entry{
			TargetUID:  uid,
			TargetKey:  target.key,
			TargetName: target.name,
			TargetPath: path,
			Via: Via{
				ReferrerPath: path,
				Kind:         FileEdge,
			},
			Confidence:     1,
			Depth:          level,
			FallbackReason: "changed file floor: " + path + " in unit " + unitID,
		}, []string{uid})
		if err != nil {
			return false, err
		}
		result.Entries = append(result.Entries, file)
	}
	if target.name == "" {
		return found, nil
	}
	candidates, err := s.indexes.SymbolsNamed(ctx, target.name)
	if err != nil {
		return false, err
	}
	var fresh []index.NamedSymbol
	for _, candidate := range candidates {
		if candidate.UID == uid || seen[[2]string{candidate.Key, uid}] {
			continue
		}
		fresh = append(fresh, candidate)
	}
	switch len(fresh) {
	case 0:
	case 1:
		candidate := fresh[0]
		seen[[2]string{candidate.Key, uid}] = true
		match, err := s.withInvariants(ctx, Entry{
			TargetUID:  uid,
			TargetKey:  target.key,
			TargetName: target.name,
			TargetPath: target.path,
			Via: Via{
				ReferrerKey:  candidate.Key,
				ReferrerPath: candidate.Path,
				TargetText:   target.name,
				Kind:         NameMatchEdge,
			},
			Confidence:     NameMatchConfidence,
			Depth:          level,
			FallbackReason: "cross-file name use of " + target.name + " by " + candidate.Key + ", unresolved (D-90)",
		}, []string{uid, candidate.UID})
		if err != nil {
			return false, err
		}
		result.Entries = append(result.Entries, match)
	default:
		var uids []string
		for _, candidate := range fresh {
			seen[[2]string{candidate.Key, uid}] = true
			uids = append(uids, candidate.UID)
		}
		ambiguous, err := s.withInvariants(ctx, Entry{
			TargetUID:  uid,
			TargetKey:  target.key,
			TargetName: target.name,
			TargetPath: target.path,
			Via: Via{
				TargetText: target.name,
				Kind:       AmbiguousEdge,
			},
			Confidence:     NameMatchConfidence,
			Depth:          level,
			FallbackReason: strings.Join(uids, ", ") + " share name " + target.name + ": no choice taken (D-154)",
		}, append([]string{uid}, uids...))
		if err != nil {
			return false, err
		}
		result.Entries = append(result.Entries, ambiguous)
	}
	return true, nil
}

// withInvariants lists bound invariant ids beside an entry's symbols and
// names them as explicit scope text when present (AC-02.4).
func (s *Service) withInvariants(ctx context.Context, entry Entry, uids []string) (Entry, error) {
	union := map[string]bool{}
	for _, uid := range uids {
		if uid == "" {
			continue
		}
		bound, err := s.indexes.BindingsForUID(ctx, uid)
		if err != nil {
			return Entry{}, err
		}
		for _, id := range bound {
			union[id] = true
		}
	}
	for id := range union {
		entry.Invariants = append(entry.Invariants, id)
	}
	sort.Strings(entry.Invariants)
	if len(entry.Invariants) > 0 {
		entry.ScopeJustification = "explicit invariant scope: " + strings.Join(entry.Invariants, ", ")
	}
	return entry, nil
}

func uidsOf(ok bool, referrerUID, targetUID string) []string {
	if ok {
		return []string{targetUID, referrerUID}
	}
	return []string{targetUID}
}

func (s *Service) resolveInputs(ctx context.Context, inputs []Input) ([]string, error) {
	var frontier []string
	known := map[string]bool{}
	for _, input := range inputs {
		var uids []string
		if input.UID != "" {
			uids = []string{input.UID}
		} else if input.Key != "" {
			resolved, err := s.indexes.KeyToUIDs(ctx, input.Key)
			if err != nil {
				return nil, err
			}
			uids = resolved
		}
		for _, uid := range uids {
			if uid == "" || known[uid] {
				continue
			}
			known[uid] = true
			frontier = append(frontier, uid)
		}
	}
	return frontier, nil
}

type targetFact struct {
	key  string
	name string
	path string
}

func targetOf(ctx context.Context, indexes *index.Store, uid string) targetFact {
	key, name, path, found, err := indexes.FactsForUID(ctx, uid)
	if err != nil || !found {
		return targetFact{}
	}
	return targetFact{key: key, name: name, path: path}
}
