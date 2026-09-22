package symbol

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/inventory"
)

// Verdict is what target resolution concluded. Resolved and ambiguous both
// name live lineages; unresolvable names none; deferred means the question is
// not asked yet because the file was never indexed (decision D-99).
type Verdict string

const (
	VerdictResolved     Verdict = "resolved"
	VerdictAmbiguous    Verdict = "ambiguous"
	VerdictUnresolvable Verdict = "unresolvable"
	VerdictDeferred     Verdict = "deferred"
)

// Resolution is one target lookup: the owning unit and file, the verdict,
// and — only for resolved — the lineage uid. Ambiguous FILE targets additionally
// name every live uid so the caller can bind each (the pair is the grain).
type Resolution struct {
	Unit       index.ProjectUnit
	Path       string
	Verdict    Verdict
	UID        string
	Candidates []string
	Detail     string
}

// splitTarget separates the file part from the dotted declaration name at
// the last colon: colons are legal inside POSIX path segments, so only the
// final one can be the reference separator. Either side may be empty only if
// the whole target is malformed, which is refused rather than resolved.
func splitTarget(target string) (file, dotted string, err error) {
	at := strings.LastIndex(target, ":")
	if at < 0 {
		return target, "", nil
	}
	file, dotted = target[:at], target[at+1:]
	if file == "" || dotted == "" {
		return "", "", fmt.Errorf("symbol: malformed target %q", target)
	}
	for _, segment := range strings.Split(dotted, ".") {
		if segment == "" {
			return "", "", fmt.Errorf("symbol: malformed target %q", target)
		}
	}
	return file, dotted, nil
}

// ResolveTarget maps one SYMBOL or FILE target onto a live lineage. FILE
// targets bind every allocated uid in the file; SYMBOL targets walk the
// file's rows segment by segment, where each segment must name exactly one
// declaration under the current container. Overloads converging on one uid
// bind; divergence — several live uids, or a uid-less row whose identity is
// still open — is ambiguous, never guessed (decision D-97).
func (s *Service) ResolveTarget(ctx context.Context, projectID, repoRoot string, units []index.ProjectUnit, target string) (Resolution, error) {
	file, dotted, err := splitTarget(target)
	if err != nil {
		return Resolution{}, err
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(file))
	unit, ok := inventory.Owner(abs, units)
	if !ok {
		// No unit can ever produce rows for this path: absence is proven by
		// construction, so this is unresolvable rather than deferred
		// (decision D-110).
		return Resolution{Verdict: VerdictUnresolvable, Detail: "target names no discovered unit"}, nil
	}
	observed, err := s.store.ReadFileState(ctx, abs)
	if err != nil {
		return Resolution{}, err
	}
	if !observed.Exists {
		if drained, err := s.unitDrained(ctx, unit.ID); err != nil {
			return Resolution{}, err
		} else if !drained {
			return Resolution{Unit: unit, Path: abs, Verdict: VerdictDeferred, Detail: "file never indexed"}, nil
		}
		// Vanished in a drained unit: the file is truly gone (decision D-99).
		if rows, err := s.store.ListSymbolsInFile(ctx, unit.ID, abs); err != nil {
			return Resolution{}, err
		} else if len(rows) > 0 {
			return s.resolveRows(unit, abs, dotted, rows)
		}
		return Resolution{Unit: unit, Path: abs, Verdict: VerdictUnresolvable, Detail: "file vanished after the cold drain"}, nil
	}
	switch observed.State.State {
	case index.StatePending:
		return Resolution{Unit: unit, Path: abs, Verdict: VerdictDeferred, Detail: "file never indexed"}, nil
	case index.StateUnsupported:
		return Resolution{Unit: unit, Path: abs, Verdict: VerdictDeferred, Detail: "file language is out of scope"}, nil
	}
	rows, err := s.store.ListSymbolsInFile(ctx, unit.ID, abs)
	if err != nil {
		return Resolution{}, err
	}
	return s.resolveRows(unit, abs, dotted, rows)
}

// unitDrained reports whether a unit holds no pending work: the condition
// under which absence of rows proves absence of the symbol (decision D-99).
func (s *Service) unitDrained(ctx context.Context, unitID string) (bool, error) {
	counts, err := s.store.CountByState(ctx, unitID)
	if err != nil {
		return false, err
	}
	return counts[index.StatePending] == 0, nil
}

// resolveRows walks one file's rows. An empty dotted name is a FILE target
// and binds every allocated uid it holds; a dotted name descends the
// container chain, where every level must be unambiguous.
func (s *Service) resolveRows(unit index.ProjectUnit, path, dotted string, rows []index.Symbol) (Resolution, error) {
	if dotted == "" {
		return bindFileUIDs(unit, path, rows)
	}
	container := ""
	segments := strings.Split(dotted, ".")
	for _, segment := range segments[:len(segments)-1] {
		var parent *index.Symbol
		var contenders []string
		for i := range rows {
			if rows[i].Name != segment || rows[i].Container != container {
				continue
			}
			if rows[i].UID != "" {
				contenders = append(contenders, rows[i].UID)
			}
			if parent != nil {
				return Resolution{Unit: unit, Path: path, Verdict: VerdictAmbiguous,
					Candidates: contenders,
					Detail:     fmt.Sprintf("container %q names several declarations", segment)}, nil
			}
			parent = &rows[i]
		}
		if parent == nil {
			return Resolution{Unit: unit, Path: path, Verdict: VerdictUnresolvable,
				Detail: fmt.Sprintf("container %q matches no declaration", segment)}, nil
		}
		container = parent.LogicalKey
	}
	name := segments[len(segments)-1]
	uids := map[string]struct{}{}
	var candidates []string
	open := false
	for i := range rows {
		if rows[i].Name != name || rows[i].Container != container {
			continue
		}
		if rows[i].UID == "" {
			open = true
			continue
		}
		if _, ok := uids[rows[i].UID]; !ok {
			uids[rows[i].UID] = struct{}{}
			candidates = append(candidates, rows[i].UID)
		}
	}
	switch {
	case len(uids) == 1 && !open:
		for uid := range uids {
			return Resolution{Unit: unit, Path: path, Verdict: VerdictResolved, UID: uid}, nil
		}
	case len(uids) == 0 && !open:
		return Resolution{Unit: unit, Path: path, Verdict: VerdictUnresolvable,
			Detail: fmt.Sprintf("no declaration named %q", name)}, nil
	default:
		return Resolution{Unit: unit, Path: path, Verdict: VerdictAmbiguous,
			Candidates: candidates,
			Detail:     fmt.Sprintf("%q names several live lineages", name)}, nil
	}
	return Resolution{}, fmt.Errorf("symbol: unreachable resolution")
}

// bindFileUIDs binds a FILE target: every allocated uid with live rows in
// the file. Zero uids is unresolvable; several is one candidate each — the
// refresh fans out because the pair is the grain.
func bindFileUIDs(unit index.ProjectUnit, path string, rows []index.Symbol) (Resolution, error) {
	var candidates []string
	seen := map[string]struct{}{}
	for i := range rows {
		if rows[i].UID == "" {
			return Resolution{Unit: unit, Path: path, Verdict: VerdictAmbiguous,
				Detail: "file holds symbols with open identities"}, nil
		}
		if _, ok := seen[rows[i].UID]; !ok {
			seen[rows[i].UID] = struct{}{}
			candidates = append(candidates, rows[i].UID)
		}
	}
	switch len(candidates) {
	case 0:
		return Resolution{Unit: unit, Path: path, Verdict: VerdictUnresolvable, Detail: "file holds no symbols"}, nil
	case 1:
		return Resolution{Unit: unit, Path: path, Verdict: VerdictResolved, UID: candidates[0]}, nil
	default:
		return Resolution{Unit: unit, Path: path, Verdict: VerdictAmbiguous,
			Candidates: candidates,
			Detail:     "file holds several live lineages; bind each"}, nil
	}
}
