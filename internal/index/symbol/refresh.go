package symbol

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/knowledge/record"
)

// Outcome is one invariant's refresh answer: the bindings written, and the
// finding when the answer is not quiet. Finding is nil for bound and
// deferred outcomes; Blocking answers whether completion must stop
// (ambiguous or orphaned over an active HIGH/CRITICAL invariant, spec §24).
type Outcome struct {
	InvariantID string
	UIDs        []string
	Status      string
	Finding     error
	Blocking    bool
}

// Report is one RefreshBindings pass over active invariants.
type Report struct {
	Outcomes []Outcome
}

// RefreshBindings resolves every active invariant target against live rows
// and stores the outcomes. It writes bindings, never knowledge records: the
// schema stays v1 (decision D-98), and a refresh over an empty invariant set
// writes nothing at all.
//
// A bound binding persists while its uid has live rows, whatever the target
// text now says (decision D-110, the sticky rule): the binding names a
// lineage, not a spelling, so a rename carried by migration needs no update
// and a stale target cannot silently unprotect. Only a uid with no live rows
// loses its binding, and only then is the target re-resolved toward orphan.
//
// Orphanhood without a dead lineage to name (a target that never bound) is
// finding-only: bindings join lineages, and there is no lineage to join.
// Findings always carry the code and the remedy regardless (decision D-111).
func (s *Service) RefreshBindings(ctx context.Context, projectID, repoRoot string, units []index.ProjectUnit, invariants []record.Invariant) (Report, error) {
	if projectID == "" {
		return Report{}, fmt.Errorf("symbol: refresh needs a project ID")
	}
	var report Report
	for _, invariant := range invariants {
		if invariant.Status != record.StatusActive {
			if _, err := s.store.DeleteBindingsExcept(ctx, invariant.ID, nil); err != nil {
				return Report{}, err
			}
			continue
		}
		outcome, err := s.refreshOne(ctx, projectID, repoRoot, units, invariant)
		if err != nil {
			return Report{}, err
		}
		report.Outcomes = append(report.Outcomes, outcome)
	}
	return report, nil
}

// refreshOne resolves one active invariant. Unknown scope levels are refused:
// an invariant that applies nowhere known must not bind quietly.
func (s *Service) refreshOne(ctx context.Context, projectID, repoRoot string, units []index.ProjectUnit, invariant record.Invariant) (Outcome, error) {
	switch invariant.Scope.Level {
	case record.ScopeProject:
		return Outcome{InvariantID: invariant.ID, Status: index.BindingBound}, nil
	case record.ScopeFile, record.ScopeSymbol:
		return s.refreshTarget(ctx, projectID, repoRoot, units, invariant)
	case record.ScopeModule, record.ScopePackage:
		return s.refreshPrefix(ctx, repoRoot, units, invariant)
	default:
		return Outcome{}, fmt.Errorf("symbol: unknown scope level %q", invariant.Scope.Level)
	}
}

// refreshTarget resolves FILE and SYMBOL targets.
func (s *Service) refreshTarget(ctx context.Context, projectID, repoRoot string, units []index.ProjectUnit, invariant record.Invariant) (Outcome, error) {
	if invariant.Scope.Target == "" {
		return Outcome{}, fmt.Errorf("symbol: %s scope needs a target", invariant.Scope.Level)
	}
	resolution, err := s.ResolveTarget(ctx, projectID, repoRoot, units, invariant.Scope.Target)
	if err != nil {
		return Outcome{}, err
	}
	switch resolution.Verdict {
	case VerdictResolved:
		if err := s.store.UpsertBinding(ctx, invariant.ID, resolution.UID, index.BindingBound, ""); err != nil {
			return Outcome{}, err
		}
		if _, err := s.store.DeleteBindingsExcept(ctx, invariant.ID, []string{resolution.UID}); err != nil {
			return Outcome{}, err
		}
		return Outcome{InvariantID: invariant.ID, UIDs: []string{resolution.UID}, Status: index.BindingBound}, nil
	case VerdictDeferred:
		return Outcome{InvariantID: invariant.ID, Status: "deferred"}, nil
	case VerdictAmbiguous:
		// A FILE target naming several live lineages is not a blocked
		// decision — it is a fan-out: the scope asks for the whole file, so
		// every lineage binds quietly. Every other divergence blocks.
		if invariant.Scope.Level == record.ScopeFile && len(resolution.Candidates) > 0 {
			for _, uid := range resolution.Candidates {
				if err := s.store.UpsertBinding(ctx, invariant.ID, uid, index.BindingBound, ""); err != nil {
					return Outcome{}, err
				}
			}
			kept := append([]string(nil), resolution.Candidates...)
			if _, err := s.store.DeleteBindingsExcept(ctx, invariant.ID, kept); err != nil {
				return Outcome{}, err
			}
			return Outcome{InvariantID: invariant.ID, UIDs: kept, Status: index.BindingBound}, nil
		}
		return s.blockOnSeverity(ctx, invariant, resolution)
	default:
		return s.orphanUnlessSticky(ctx, invariant, resolution)
	}
}

// refreshPrefix binds MODULE and PACKAGE scopes: every allocated uid under
// the target prefix, each as its own pair. Prefixes filter in Go — SQL LIKE
// cannot tell "%" the wildcard from "%" the path byte (the D-84 lesson).
// Targets are repository-relative, so paths are relativized against the
// repository root, not the unit root: a nested unit's rows still match the
// repo-relative prefix that names them.
func (s *Service) refreshPrefix(ctx context.Context, repoRoot string, units []index.ProjectUnit, invariant record.Invariant) (Outcome, error) {
	if invariant.Scope.Target == "" {
		return Outcome{}, fmt.Errorf("symbol: %s scope needs a target", invariant.Scope.Level)
	}
	prefix := invariant.Scope.Target
	var bound []string
	seen := map[string]struct{}{}
	for _, unit := range units {
		paths, err := s.store.SymbolsPaths(ctx, unit.ID)
		if err != nil {
			return Outcome{}, err
		}
		for _, path := range paths {
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			// Suffix the separator so "pkg" never claims "pkg2".
			relSlash := filepath.ToSlash(rel)
			if relSlash != prefix && !strings.HasPrefix(relSlash, prefix+"/") {
				continue
			}
			uids, err := s.store.DistinctUIDsInFile(ctx, unit.ID, path)
			if err != nil {
				return Outcome{}, err
			}
			for _, uid := range uids {
				if _, ok := seen[uid]; ok {
					continue
				}
				seen[uid] = struct{}{}
				if err := s.store.UpsertBinding(ctx, invariant.ID, uid, index.BindingBound, ""); err != nil {
					return Outcome{}, err
				}
				bound = append(bound, uid)
			}
		}
	}
	if len(bound) == 0 {
		return s.orphanUnlessSticky(ctx, invariant, Resolution{Detail: "prefix holds no allocated symbols"})
	}
	if _, err := s.store.DeleteBindingsExcept(ctx, invariant.ID, bound); err != nil {
		return Outcome{}, err
	}
	return Outcome{InvariantID: invariant.ID, UIDs: bound, Status: index.BindingBound}, nil
}

// orphanUnlessSticky drops bindings whose lineage died and evaluates the
// orphan track — unless an existing bound binding still has live rows, in
// which case the lineage survived (a carried rename) and the binding stays
// exactly where it is (decision D-110). A dead lineage the store left
// explicitly undecided reports ambiguous instead of orphaned: the lineage did
// not vanish, the decision is what is missing.
func (s *Service) orphanUnlessSticky(ctx context.Context, invariant record.Invariant, resolution Resolution) (Outcome, error) {
	kept, dropped, err := s.partitionLiveBindings(ctx, invariant.ID)
	if err != nil {
		return Outcome{}, err
	}
	if len(kept) > 0 {
		return Outcome{InvariantID: invariant.ID, UIDs: kept, Status: index.BindingBound}, nil
	}
	for _, uid := range dropped {
		ambiguities, err := s.store.ListAmbiguitiesForUID(ctx, uid)
		if err != nil {
			return Outcome{}, err
		}
		if len(ambiguities) == 0 {
			continue
		}
		if err := s.store.UpsertBinding(ctx, invariant.ID, uid, index.BindingAmbiguous, resolution.Detail); err != nil {
			return Outcome{}, err
		}
		finding := index.AmbiguousHeirs(uid, ambiguities[0].CandidateKeys)
		return Outcome{InvariantID: invariant.ID, UIDs: []string{uid}, Status: index.BindingAmbiguous,
			Finding: finding, Blocking: blocks(invariant)}, nil
	}
	if len(dropped) > 0 {
		if err := s.store.UpsertBinding(ctx, invariant.ID, dropped[0], index.BindingOrphaned, resolution.Detail); err != nil {
			return Outcome{}, err
		}
		return Outcome{InvariantID: invariant.ID, UIDs: []string{dropped[0]}, Status: index.BindingOrphaned,
			Finding:  index.OrphanedProtectedSymbol(invariant.ID, invariant.Scope.Target),
			Blocking: blocks(invariant)}, nil
	}
	finding := index.OrphanedProtectedSymbol(invariant.ID, invariant.Scope.Target)
	return Outcome{InvariantID: invariant.ID, Status: index.BindingOrphaned,
		Finding: finding, Blocking: blocks(invariant)}, nil
}

// blockOnSeverity records an ambiguous binding over the live candidates the
// resolution named. Divergence without a removed identity writes no
// symbol_identity_ambiguities row — that table records rename decisions
// (TASK-04); this one records that a target names several live lineages and
// none was chosen.
func (s *Service) blockOnSeverity(ctx context.Context, invariant record.Invariant, resolution Resolution) (Outcome, error) {
	uids := append([]string(nil), resolution.Candidates...)
	for _, uid := range uids {
		if err := s.store.UpsertBinding(ctx, invariant.ID, uid, index.BindingAmbiguous, resolution.Detail); err != nil {
			return Outcome{}, err
		}
	}
	if _, err := s.store.DeleteBindingsExcept(ctx, invariant.ID, uids); err != nil {
		return Outcome{}, err
	}
	finding := index.AmbiguousTarget(invariant.Scope.Target, resolution.Detail, uids)
	return Outcome{InvariantID: invariant.ID, UIDs: uids, Status: index.BindingAmbiguous,
		Finding: finding, Blocking: blocks(invariant)}, nil
}

// partitionLiveBindings splits an invariant's bound bindings into the uids
// that still have live rows and the ones that lost them, deleting everything
// that is not kept. Ambiguous rows from an earlier pass are dropped by the
// prune: the current pass re-decides them from the world as it is.
func (s *Service) partitionLiveBindings(ctx context.Context, invariantID string) (kept, dropped []string, err error) {
	bindings, err := s.store.ListBindingsForInvariant(ctx, invariantID)
	if err != nil {
		return nil, nil, err
	}
	for _, binding := range bindings {
		if binding.Status != index.BindingBound {
			continue
		}
		live, err := s.store.UidHasLiveRows(ctx, binding.UID)
		if err != nil {
			return nil, nil, err
		}
		if live {
			kept = append(kept, binding.UID)
		} else {
			dropped = append(dropped, binding.UID)
		}
	}
	if _, err := s.store.DeleteBindingsExcept(ctx, invariantID, kept); err != nil {
		return nil, nil, err
	}
	return kept, dropped, nil
}

// blocks answers whether a non-quiet outcome stops completion: ambiguous or
// orphaned over an active HIGH/CRITICAL invariant (spec §24, decision D-98).
// Callers only pass active invariants here; the severity alone decides.
func blocks(invariant record.Invariant) bool {
	return invariant.Severity == record.SeverityCritical || invariant.Severity == record.SeverityHigh
}
