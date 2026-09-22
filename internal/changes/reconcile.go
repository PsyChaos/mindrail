package changes

import (
	"context"
	"path/filepath"

	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
)

// DivergenceEntry is one Git-discovered changed file outside the task's
// baseline scope: reported, never absorbed (decision D-122). It is MR-008's
// input shape, produced here.
type DivergenceEntry struct {
	Path   string
	Kind   string
	Reason string
}

// ReconcileResult is one reconcile pass: the change carrying discovered
// rows, plus the divergence list (empty means none).
type ReconcileResult struct {
	Change     Change
	Divergence []DivergenceEntry
}

// Reconcile runs the canonical correctness path (spec §65): Git truth,
// structural symbol deltas, index updates with identity migration, and
// linkage into the task's open Change — or a fresh retroactive Change when
// no task is named. A Git that cannot answer fails the run: reconcile must
// never certify a dirty tree clean on an empty diff.
func (s *Service) Reconcile(ctx context.Context, projectID, repoRoot, taskID, operationID string, runner git.CommandRunner) (ReconcileResult, error) {
	if err := ctx.Err(); err != nil {
		return ReconcileResult{}, err
	}
	if projectID == "" {
		return ReconcileResult{}, invalidInput("reconcile needs a project")
	}
	entries, err := s.store.DiscoverFilesGit(ctx, runner, repoRoot)
	if err != nil {
		return ReconcileResult{}, err
	}
	hints, err := s.renameHints(ctx, runner, repoRoot)
	if err != nil {
		return ReconcileResult{}, err
	}
	change, err := s.store.EnsureOpenChange(ctx, taskID, operationID)
	if err != nil {
		return ReconcileResult{}, err
	}
	if err := s.store.UpsertFileRows(ctx, change.ID, entries); err != nil {
		return ReconcileResult{}, err
	}
	units, err := s.indexes.ListUnits(ctx, repoRoot)
	if err != nil {
		return ReconcileResult{}, err
	}
	for _, file := range entries {
		content, deleted := readScopeContent(file)
		fileHints := fileHintsFor(hints, file.Path)
		if err := s.SyncFileSymbols(ctx, projectID, repoRoot, change.ID, file.Path, content, deleted, fileHints, ViaReconcile, units); err != nil {
			return ReconcileResult{}, err
		}
	}
	divergence, err := s.divergence(ctx, taskID, entries)
	if err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult{Change: change, Divergence: divergence}, nil
}

// renameHints fetches the corroboration set once per run: a Git that cannot
// answer yields no hints (best-effort, decision D-101), while discovery
// itself already failed closed above.
func (s *Service) renameHints(ctx context.Context, runner git.CommandRunner, repoRoot string) ([]index.RenameHint, error) {
	renames, err := git.DiffRenames(ctx, runner, repoRoot)
	if err != nil {
		return nil, err
	}
	hints := make([]index.RenameHint, 0, len(renames))
	for _, rename := range renames {
		hints = append(hints, index.RenameHint{
			OldPath: joinRoot(repoRoot, rename.OldPath),
			NewPath: joinRoot(repoRoot, rename.NewPath),
		})
	}
	return hints, nil
}

// fileHintsFor selects the hints renaming into one file: the only cross-file
// ancestors its completion may consult.
func fileHintsFor(hints []index.RenameHint, newPath string) []index.RenameHint {
	var relevant []index.RenameHint
	for _, hint := range hints {
		if hint.NewPath == newPath {
			relevant = append(relevant, hint)
		}
	}
	return relevant
}

// divergence lists discovered files outside the task's baseline scope. A
// NULL-task run has no baseline by construction and never diverges; an
// empty scope list means none.
func (s *Service) divergence(ctx context.Context, taskID string, entries []FileChange) ([]DivergenceEntry, error) {
	if taskID == "" {
		return nil, nil
	}
	baseline, err := s.store.ReadBaseline(ctx, taskID)
	if err != nil {
		return nil, err
	}
	var divergence []DivergenceEntry
	for _, file := range entries {
		if _, ok := baseline[file.Path]; ok {
			continue
		}
		divergence = append(divergence, DivergenceEntry{
			Path:   file.Path,
			Kind:   file.Kind,
			Reason: "changed outside the task baseline scope",
		})
	}
	return divergence, nil
}

// joinRoot joins a worktree root with a repo-relative Git path into the
// clean absolute form every downstream path comparison expects.
func joinRoot(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}
