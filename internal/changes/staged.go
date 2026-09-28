package changes

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
)

// ReconcileStaged runs the canonical path over the index: staged entries,
// staged bytes for hashing and symbol sync, linkage into the task's open
// Change (or a fresh retroactive Change when no task is named). Same rows
// downstream as worktree reconcile — the file source is the only
// difference (decision D-215). A git that cannot answer fails the run.
func (s *Service) ReconcileStaged(ctx context.Context, projectID, repoRoot, taskID, operationID string, runner git.CommandRunner) (ReconcileResult, error) {
	s.stagedMu.Lock()
	defer s.stagedMu.Unlock()
	entries, err := s.store.DiscoverFilesStaged(ctx, runner, repoRoot)
	if err != nil {
		return ReconcileResult{}, err
	}
	return s.reconcileStagedSnapshot(ctx, projectID, repoRoot, taskID, operationID, entries, runner)
}

// ReconcileStagedSnapshot reconciles an already-discovered Git index snapshot.
// Verify uses it so guard evaluation and change projection judge the same path
// set; content hashes are rechecked while bytes are read.
func (s *Service) ReconcileStagedSnapshot(ctx context.Context, projectID, repoRoot, taskID, operationID string, entries []FileChange, runner git.CommandRunner) (ReconcileResult, error) {
	s.stagedMu.Lock()
	defer s.stagedMu.Unlock()
	return s.reconcileStagedSnapshot(ctx, projectID, repoRoot, taskID, operationID, entries, runner)
}

func (s *Service) reconcileStagedSnapshot(ctx context.Context, projectID, repoRoot, taskID, operationID string, entries []FileChange, runner git.CommandRunner) (ReconcileResult, error) {
	if err := ctx.Err(); err != nil {
		return ReconcileResult{}, err
	}
	if projectID == "" {
		return ReconcileResult{}, invalidInput("staged reconcile needs a project")
	}
	hints, err := s.renameHints(ctx, runner, repoRoot)
	if err != nil {
		return ReconcileResult{}, err
	}
	units, err := s.indexes.ListUnits(ctx, repoRoot)
	if err != nil {
		return ReconcileResult{}, err
	}
	divergence, err := s.divergence(ctx, taskID, entries)
	if err != nil {
		return ReconcileResult{}, err
	}
	target, err := s.store.EnsureOpenChange(ctx, taskID, operationID)
	if err != nil {
		return ReconcileResult{}, err
	}
	previous, err := s.store.ReadChangeFiles(ctx, target.ID)
	if err != nil {
		return ReconcileResult{}, err
	}
	if sameFileProjection(previous, entries) {
		for _, file := range entries {
			if _, _, err := stagedContent(ctx, runner, repoRoot, file); err != nil {
				return ReconcileResult{}, err
			}
		}
		if err := s.ensureStagedProjection(ctx, runner, repoRoot, entries); err != nil {
			return ReconcileResult{}, err
		}
		return ReconcileResult{Change: target, Divergence: divergence}, nil
	}
	var symbols []SymbolChange
	for _, file := range entries {
		content, deleted, err := stagedContent(ctx, runner, repoRoot, file)
		if err != nil {
			return ReconcileResult{}, err
		}
		fallback, err := s.stagedHEADDelta(ctx, repoRoot, file, content, deleted, units, runner)
		if err != nil {
			return ReconcileResult{}, err
		}
		fileHints := fileHintsFor(hints, file.Path)
		rows, err := s.fileSymbolRows(ctx, projectID, repoRoot, file.Path, content, deleted, fileHints, ViaReconcile, units, fallback, true)
		if err != nil {
			return ReconcileResult{}, err
		}
		symbols = append(symbols, rows...)
	}
	if err := s.ensureStagedProjection(ctx, runner, repoRoot, entries); err != nil {
		return ReconcileResult{}, err
	}
	if err := s.store.replaceChangeProjection(ctx, target.ID, entries, symbols); err != nil {
		return ReconcileResult{}, err
	}
	target, err = s.store.readChange(ctx, target.ID)
	if err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult{Change: target, Divergence: divergence}, nil
}

func (s *Service) ensureStagedProjection(ctx context.Context, runner git.CommandRunner, repoRoot string, expected []FileChange) error {
	current, err := s.store.DiscoverFilesStaged(ctx, runner, repoRoot)
	if err != nil {
		return err
	}
	if !sameFileProjection(expected, current) {
		return invalidInput("staged index changed during verification; retry")
	}
	return nil
}

func sameFileProjection(left, right []FileChange) bool {
	if len(left) != len(right) {
		return false
	}
	byPath := make(map[string]FileChange, len(left))
	for _, file := range left {
		byPath[file.Path] = file
	}
	for _, file := range right {
		previous, ok := byPath[file.Path]
		if !ok || previous.Kind != file.Kind || previous.OldPath != file.OldPath ||
			previous.Hash != file.Hash || previous.Via != file.Via {
			return false
		}
	}
	return true
}

// stagedHEADDelta derives the authoritative staged delta from committed bytes
// to index bytes without depending on mutable semantic-index state. This keeps
// first runs and retries equivalent after any earlier partial indexing.
func (s *Service) stagedHEADDelta(ctx context.Context, repoRoot string, file FileChange, content string, deleted bool, units []index.ProjectUnit, runner git.CommandRunner) ([]SymbolChange, error) {
	unit, ok := s.unitFor(file.Path, units)
	if !ok {
		return nil, nil
	}
	var current []index.Symbol
	var err error
	if !deleted {
		current, err = s.indexer.ExtractCurrent(ctx, unit, file.Path, []byte(content))
		if err != nil {
			return nil, err
		}
	}
	beforePath := file.Path
	if file.OldPath != "" {
		beforePath = file.OldPath
	}
	rel, err := filepath.Rel(repoRoot, beforePath)
	if err != nil {
		return nil, invalidInput("staged HEAD path escapes its worktree root: " + beforePath)
	}
	before, found, err := git.ShowHEAD(ctx, runner, repoRoot, filepath.ToSlash(rel))
	if err != nil {
		return nil, err
	}
	var baseline []index.Symbol
	if found {
		// Qualify both sides at the destination path. A pure rename is owned at
		// file level; content changes still produce comparable logical keys.
		baseline, err = s.indexer.ExtractCurrent(ctx, unit, file.Path, before)
		if err != nil {
			return nil, err
		}
		identityKeys := make(map[string]string, len(baseline))
		identityUnit := unit
		if candidate, exists := s.unitFor(beforePath, units); exists {
			identityUnit = candidate
		}
		identityBaseline, extractErr := s.indexer.ExtractCurrent(ctx, identityUnit, beforePath, before)
		if extractErr != nil {
			return nil, extractErr
		}
		for _, symbol := range identityBaseline {
			if local, ok := qualifiedLocalKey(symbol.LogicalKey); ok {
				identityKeys[local] = symbol.LogicalKey
			}
		}
		for i := range baseline {
			identityKey := baseline[i].LogicalKey
			if local, ok := qualifiedLocalKey(identityKey); ok && identityKeys[local] != "" {
				identityKey = identityKeys[local]
			}
			uids, resolveErr := s.indexes.KeyToUIDs(ctx, identityKey)
			if resolveErr != nil {
				return nil, resolveErr
			}
			if len(uids) == 1 {
				baseline[i].UID = uids[0]
			}
		}
	}
	return diffSymbols(baseline, current, ViaReconcile), nil
}

func qualifiedLocalKey(key string) (string, bool) {
	var qualified [2]string
	if err := json.Unmarshal([]byte(key), &qualified); err != nil || qualified[1] == "" {
		return "", false
	}
	return qualified[1], true
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
	if file.Hash != "" && contentHashBytes(content) != file.Hash {
		return "", false, invalidInput("staged index changed during verification; retry")
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
	ownership, err := s.store.ReadTaskOwnership(ctx)
	if err != nil {
		return TaskAttribution{}, err
	}
	all, err := s.store.ListTaskChanges(ctx)
	if err != nil {
		return TaskAttribution{}, err
	}
	changesByTask := make(map[string]string, len(all))
	for _, change := range all {
		changesByTask[change.TaskID] = change.ID
	}
	for _, change := range changeIDs {
		files, err := s.store.ReadChangeFiles(ctx, change)
		if err != nil {
			return TaskAttribution{}, err
		}
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
			} else if row.Kind == SymbolRemoved {
				if projected, ok := projectedFileForKey(files, row.Key); ok {
					// Removed symbols no longer have a live symbols row. The staged
					// projection still carries both the exact changed files and the
					// qualified key written by our indexer, so ownership can be
					// recovered without consulting mutable worktree state.
					file, found = projected, true
					attributed.File = projected
				}
			}
			var owners, candidates []string
			if found {
				owners = PreferredTaskOwners(scopes, ownership, file)
				for _, taskID := range owners {
					if changeID, ok := changesByTask[taskID]; ok {
						candidates = append(candidates, changeID)
					} else {
						// Keep the scope owner visible even when its Change row is
						// absent. Otherwise two owners could collapse to one and
						// incorrectly authorize a source-file commit.
						candidates = append(candidates, taskID)
					}
				}
				sort.Strings(candidates)
			}
			switch len(owners) {
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

// projectedFileForKey resolves the path component of an indexer-qualified
// logical key against one change's exact file projection. It returns no path
// for legacy keys or ambiguous suffixes rather than guessing.
func projectedFileForKey(files []FileChange, key string) (string, bool) {
	var qualified []string
	if err := json.Unmarshal([]byte(key), &qualified); err != nil ||
		len(qualified) != 2 || qualified[0] == "" || qualified[1] == "" {
		return "", false
	}
	rel := filepath.ToSlash(filepath.Clean(qualified[0]))
	if rel == "." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return "", false
	}
	var match string
	for _, file := range files {
		path := filepath.ToSlash(filepath.Clean(file.Path))
		if path != rel && !strings.HasSuffix(path, "/"+rel) {
			continue
		}
		if match != "" && match != file.Path {
			return "", false
		}
		match = file.Path
	}
	return match, match != ""
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
