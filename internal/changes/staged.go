package changes

import (
	"context"
	"path/filepath"
	"sort"

	"github.com/PsyChaos/mindrail/internal/git"
)

// ReconcileStaged runs the canonical path over the index: staged entries,
// staged bytes for hashing and symbol sync, linkage into the task's open
// Change (or a fresh retroactive Change when no task is named). Same rows
// downstream as worktree reconcile — the file source is the only
// difference (decision D-215). A git that cannot answer fails the run.
func (s *Service) ReconcileStaged(ctx context.Context, projectID, repoRoot, taskID, operationID string, runner git.CommandRunner) (ReconcileResult, error) {
	if err := ctx.Err(); err != nil {
		return ReconcileResult{}, err
	}
	if projectID == "" {
		return ReconcileResult{}, invalidInput("staged reconcile needs a project")
	}
	entries, err := s.store.DiscoverFilesStaged(ctx, runner, repoRoot)
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
		content, deleted, err := stagedContent(ctx, runner, repoRoot, file)
		if err != nil {
			return ReconcileResult{}, err
		}
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

// stagedContent reads one discovered file's index bytes for symbol sync.
// Deleted paths sync empty with the deleted flag, exactly like worktree
// deletion; missing index bytes fail closed (discovery promised them).
func stagedContent(ctx context.Context, runner git.CommandRunner, root string, file FileChange) (string, bool, error) {
	if file.Kind == FileDeleted {
		return "", true, nil
	}
	rel, err := filepath.Rel(root, file.Path)
	if err != nil {
		return "", false, invalidInput("staged file escapes its worktree root: " + file.Path)
	}
	content, found, err := git.ShowStaged(ctx, runner, root, rel)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, invalidInput("staged entry has no index bytes: " + file.Path)
	}
	return string(content), false, nil
}

// AttributeChanges attributes one verify run's discovery globally: every
// symbol of the given changes against every open change's baseline. Zero
// candidates is unregistered, two or more is ambiguous, one attributes —
// the D-133 rule without an evaluating task, because verify judges the
// commit, not a task. Scoping to the run's own changes is what keeps the
// staged and CI verdicts from judging each other's rows: the two modes
// write different NULL-task changes into one database, and a verdict
// carrying denials for content outside its judged set would be gate
// confusion, not coverage.
func (s *Service) AttributeChanges(ctx context.Context, changeIDs []string) (TaskAttribution, error) {
	answer := TaskAttribution{}
	scopes, err := s.store.ReadAllBaselines(ctx)
	if err != nil {
		return TaskAttribution{}, err
	}
	all, err := s.store.ListTaskChanges(ctx)
	if err != nil {
		return TaskAttribution{}, err
	}
	for _, change := range changeIDs {
		symbols, err := s.store.ReadChangeSymbols(ctx, change)
		if err != nil {
			return TaskAttribution{}, err
		}
		for _, row := range symbols {
			attributed := SymbolAttribution{Key: row.Key, UID: row.UID, Via: row.Via}
			file, found, err := s.indexes.PathForUID(ctx, row.UID)
			if err != nil {
				return TaskAttribution{}, err
			}
			if found {
				attributed.File = file
			}
			var candidates []string
			if found {
				for _, other := range all {
					for _, scope := range scopes[other.TaskID] {
						if scope == file {
							candidates = append(candidates, other.ID)
							break
						}
					}
				}
				sort.Strings(candidates)
			}
			switch len(candidates) {
			case 1:
				attributed.Outcome = AttributedOutcome
				attributed.AttributedTo = candidates[0]
			case 0:
				attributed.Outcome = UnregisteredOutcome
				finding := UnregisteredChange("", change, row.Key, row.Via, nil)
				attributed.Finding = &finding
			default:
				attributed.Outcome = AmbiguousOutcome
				attributed.Candidates = candidates
				finding := AmbiguousAttribution("", change, row.Key, candidates, nil)
				attributed.Finding = &finding
			}
			answer.Symbols = append(answer.Symbols, attributed)
		}
	}
	return answer, nil
}

// Store exposes the change facts for composition callers (verify reads
// staged rows back through it). The handle shares the service's database;
// callers must not retain it past the service.
func (s *Service) Store() *Store {
	return s.store
}

// listNullTaskChanges returns the ownerless changes: rows no task claims.
// Neutral read beside ListTaskChanges, kept for diagnosis; attribution no
// longer unions across them (scoped per run).
func (s *Store) listNullTaskChanges(ctx context.Context) ([]string, error) {
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT change_id FROM changes
		WHERE task_id IS NULL ORDER BY change_id`)
	if err != nil {
		return nil, corruptState(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, corruptState(err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return out, nil
}
