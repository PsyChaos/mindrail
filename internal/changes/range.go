package changes

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/git"
)

// verifyCIOperationID converges repeat CI runs onto one NULL-task change
// instead of minting a fresh retroactive row per call — the D-215 principle
// applied to the range path (decision D-227).
const verifyCIOperationID = "verify-ci"

// ReconcileRange runs the canonical path over a committed range: range
// entries, head bytes for hashing and symbol sync, linkage into a NULL-task
// Change converging on the fixed verify-ci operation id. Same rows
// downstream as worktree and staged reconcile — the file source is the only
// difference. A git that cannot answer fails the run.
func (s *Service) ReconcileRange(ctx context.Context, projectID, repoRoot, mergeBaseSHA, headSHA string, runner git.CommandRunner) (ReconcileResult, error) {
	if err := ctx.Err(); err != nil {
		return ReconcileResult{}, err
	}
	if projectID == "" {
		return ReconcileResult{}, invalidInput("range reconcile needs a project")
	}
	entries, err := s.store.DiscoverFilesRange(ctx, runner, repoRoot, mergeBaseSHA, headSHA)
	if err != nil {
		return ReconcileResult{}, err
	}
	hints, err := s.renameHints(ctx, runner, repoRoot)
	if err != nil {
		return ReconcileResult{}, err
	}
	change, err := s.store.EnsureOpenChange(ctx, "", verifyCIOperationID)
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
		content, deleted, err := rangeContent(ctx, runner, repoRoot, headSHA, file)
		if err != nil {
			return ReconcileResult{}, err
		}
		fileHints := fileHintsFor(hints, file.Path)
		if err := s.SyncFileSymbols(ctx, projectID, repoRoot, change.ID, file.Path, content, deleted, fileHints, ViaReconcile, units); err != nil {
			return ReconcileResult{}, err
		}
	}
	divergence, err := s.divergence(ctx, "", entries)
	if err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult{Change: change, Divergence: divergence}, nil
}

// DiscoverFilesRange reads a committed range: what CI judges, never the
// worktree desk (decision D-222). Hashes and exclusions ride the shared
// staged path; only the entry source and the content bytes differ.
func (s *Store) DiscoverFilesRange(ctx context.Context, runner git.CommandRunner, root, mergeBaseSHA, headSHA string) ([]FileChange, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root == "" {
		return nil, invalidInput("range discovery needs a worktree root")
	}
	entries, err := git.RangeEntries(ctx, runner, root, mergeBaseSHA, headSHA)
	if err != nil {
		return nil, err
	}
	var files []FileChange
	for _, entry := range entries {
		changed, err := s.rangeFile(ctx, runner, root, headSHA, entry)
		if err != nil {
			return nil, err
		}
		if changed != nil {
			files = append(files, *changed)
		}
	}
	return files, nil
}

// rangeFile maps one range entry onto a file delta with head bytes.
// Deleted paths carry no bytes; everything else hashes what head records,
// never what the disk holds now.
func (s *Store) rangeFile(ctx context.Context, runner git.CommandRunner, root, headSHA string, entry git.StatusEntry) (*FileChange, error) {
	if entry.Path == "" || strings.Contains(entry.Path, "\\") {
		return nil, invalidInput("git entry names no usable path")
	}
	if excludedPath(entry.Path) {
		return nil, nil
	}
	abs := filepath.Join(root, filepath.FromSlash(entry.Path))
	if !isBelow(root, abs) {
		return nil, invalidInput("git entry escapes its worktree root")
	}
	kind, oldRel := entryKind(entry)
	changed := &FileChange{Path: abs, Kind: kind, Via: ViaReconcile}
	if oldRel != "" {
		oldAbs := filepath.Join(root, filepath.FromSlash(oldRel))
		if !isBelow(root, oldAbs) {
			return nil, invalidInput("git rename source escapes its worktree root")
		}
		changed.OldPath = oldAbs
	}
	if kind == FileDeleted {
		return changed, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	content, found, err := git.ShowRev(ctx, runner, root, headSHA, entry.Path)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, invalidInput("range entry has no head bytes: " + entry.Path)
	}
	changed.Hash = contentHashBytes(content)
	return changed, nil
}

// rangeContent reads one discovered file's head bytes for symbol sync.
// Deleted paths sync empty with the deleted flag, exactly like staged
// deletion; missing head bytes fail closed (discovery promised them).
func rangeContent(ctx context.Context, runner git.CommandRunner, root, headSHA string, file FileChange) (string, bool, error) {
	if file.Kind == FileDeleted {
		return "", true, nil
	}
	rel, err := filepath.Rel(root, file.Path)
	if err != nil {
		return "", false, invalidInput("range file escapes its worktree root: " + file.Path)
	}
	content, found, err := git.ShowRev(ctx, runner, root, headSHA, rel)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, invalidInput("range entry has no head bytes: " + file.Path)
	}
	return string(content), false, nil
}
