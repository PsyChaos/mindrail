// Package impact answers "what does changing this symbol touch" as bounded,
// self-explaining traversal over stored structural facts (MR-009). Analysis
// is pure computation: deterministic in the index rows and the request,
// stored nowhere (decision D-147). Results block nothing (spec §31) — they
// feed MR-010's runner and MR-013's gate as explanation, never as verdict.
package impact

import (
	"context"
	"errors"

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

// Edge kinds. Direct rode a resolved reference; the TASK-02 fallbacks add
// name-match, file and ambiguous kinds.
const (
	DirectEdge = "direct"
)

// Input is one changed symbol, by durable uid or by logical key. A key may
// resolve to several uids across units: every one is analyzed, never chosen
// between. Inputs that resolve nowhere are dropped, never an error —
// traversal judges stored facts, not caller spelling.
type Input struct {
	UID string
	Key string
}

// Request is one analysis. Depth at or below zero means 1 (decision D-150);
// above 1 runs only when passed explicitly. BreadthOverride with a
// Justification records a caller-justified wider breadth (path policy,
// public API rule, explicit invariant scope); without it the structural cap
// holds.
type Request struct {
	Symbols         []Input
	Depth           int
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
type Entry struct {
	TargetUID      string
	TargetKey      string
	TargetName     string
	TargetPath     string
	Via            Via
	Confidence     float64
	Depth          int
	FallbackReason string
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
	if request.BreadthOverride != "" {
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
				result.Entries = append(result.Entries, Entry{
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
				})
				if level < depth {
					if referrerUID, ok, err := s.indexes.UIDForKey(ctx, referrer.UnitID, referrer.Path, referrer.Key); err != nil {
						return Result{}, err
					} else if ok && !visited[referrerUID] {
						visited[referrerUID] = true
						next = append(next, referrerUID)
					}
				}
			}
		}
		frontier = next
	}
	return result, nil
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
