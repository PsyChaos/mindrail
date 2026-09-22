package changes

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// File change kinds ride the CHECK vocabulary of change_files.
const (
	FileAdded    = "added"
	FileModified = "modified"
	FileDeleted  = "deleted"
	FileRenamed  = "renamed"
)

// DiscoveredVia names which path found a row: the baseline the agent
// declared, or the reconcile Git truth. Mixed flows are the norm, so the
// provenance rides each row, never the Change.
const (
	ViaBaseline  = "baseline"
	ViaReconcile = "reconcile"
)

// FileChange is one discovered file delta, ready to upsert.
type FileChange struct {
	Path    string
	Kind    string
	OldPath string
	Hash    string
	Via     string
}

// DiscoverFilesGit reads the worktree's change set through porcelain and
// returns absolute file deltas. Runtime and knowledge subtrees never become
// rows; non-regular paths still do, with an empty hash and no symbols —
// discovery stays complete where parsing cannot follow (decision D-117).
func (s *Store) DiscoverFilesGit(ctx context.Context, runner git.CommandRunner, root string) ([]FileChange, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root == "" {
		return nil, invalidInput("git discovery needs a worktree root")
	}
	entries, err := git.StatusEntries(ctx, runner, root)
	if err != nil {
		return nil, err
	}
	var files []FileChange
	for _, entry := range entries {
		changed, err := s.gitFile(ctx, root, entry)
		if err != nil {
			return nil, err
		}
		if changed != nil {
			files = append(files, *changed)
		}
	}
	return files, nil
}

// gitFile maps one porcelain entry onto a file delta, or nil when the entry
// names excluded machinery. Malformed repository-relative spellings fail the
// discovery closed rather than joining it half-read.
func (s *Store) gitFile(ctx context.Context, root string, entry git.StatusEntry) (*FileChange, error) {
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
	hash, err := contentHash(abs)
	if err != nil {
		return nil, err
	}
	changed.Hash = hash
	return changed, nil
}

// entryKind maps porcelain codes onto the D-117 vocabulary. The worktree
// code wins when set: what the disk holds now is what discovery reports.
// Rename similarity is staged-only by Git construction — an unstaged move
// surfaces as a delete plus an add, which this mapping reports honestly
// instead of reuniting; Y == 'R' is accepted because a staged rename edited
// further in the worktree is still a rename.
func entryKind(entry git.StatusEntry) (kind, oldRel string) {
	switch {
	case entry.X == '?' && entry.Y == '?':
		return FileAdded, ""
	case entry.X == 'R' || entry.Y == 'R':
		return FileRenamed, entry.OrigPath
	case entry.X == 'C' || entry.Y == 'C':
		return FileAdded, ""
	case entry.X == 'D' || entry.Y == 'D':
		return FileDeleted, ""
	case entry.X == 'A':
		return FileAdded, ""
	default:
		return FileModified, ""
	}
}

// excludedPath drops runtime and knowledge machinery from discovery: the
// database lives under .git and the records under .mindrail, and neither is
// source either path may claim.
func excludedPath(rel string) bool {
	return rel == ".git" || strings.HasPrefix(rel, ".git/") ||
		rel == ".mindrail" || strings.HasPrefix(rel, ".mindrail/")
}

// isBelow reports whether path stays inside root after cleaning.
func isBelow(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// BaselineFileDelta diffs caller scope against the task's baseline: changed
// hashes — or scope files the baseline never captured — become rows. The via
// is always baseline on this path; reconcile computes its own.
func (s *Store) BaselineFileDelta(ctx context.Context, taskID string, scope []string) ([]FileChange, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	baseline, err := s.ReadBaseline(ctx, taskID)
	if err != nil {
		return nil, err
	}
	var delta []FileChange
	for _, path := range scope {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, invalidInput("file delta needs clean absolute paths")
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hash, err := contentHash(path)
		if err != nil {
			return nil, err
		}
		recorded, ok := baseline[path]
		if ok && recorded == hash {
			continue
		}
		kind := FileModified
		if _, err := os.Lstat(path); err != nil {
			kind = FileDeleted
		} else if !ok {
			kind = FileAdded
		}
		delta = append(delta, FileChange{Path: path, Kind: kind, Hash: hash, Via: ViaBaseline})
	}
	return delta, nil
}

// UpsertFileRows accumulates file deltas into a Change by natural key:
// redelivery converges instead of duplicating (the AC-15 shape at row
// level, under the operation log at call level).
func (s *Store) UpsertFileRows(ctx context.Context, changeID string, rows []FileChange) error {
	if changeID == "" {
		return invalidInput("file rows need a change")
	}
	if err := s.requireSchema(ctx); err != nil {
		return err
	}
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		for _, row := range rows {
			if row.Path == "" {
				return invalidInput("file row needs a path")
			}
			switch row.Kind {
			case FileAdded, FileModified, FileDeleted, FileRenamed:
			default:
				return invalidInput("file row kind must be added, modified, deleted or renamed")
			}
			switch row.Via {
			case ViaBaseline, ViaReconcile:
			default:
				return invalidInput("file row provenance must be baseline or reconcile")
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO change_files
				(change_id, path, kind, old_path, content_hash, discovered_via)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT(change_id, path) DO UPDATE SET
				kind = excluded.kind, old_path = excluded.old_path,
				content_hash = excluded.content_hash, discovered_via = excluded.discovered_via`,
				changeID, row.Path, row.Kind, row.OldPath, row.Hash, row.Via); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return writeFailure(ctx, s.db, "change files", err)
	}
	return nil
}

// ReadChangeFiles returns one change's file rows in path order.
func (s *Store) ReadChangeFiles(ctx context.Context, changeID string) ([]FileChange, error) {
	if changeID == "" {
		return nil, invalidInput("file listing needs a change")
	}
	if err := s.requireSchema(ctx); err != nil {
		return nil, err
	}
	dbRows, err := s.db.QueryContext(ctx, `SELECT path, kind, old_path, content_hash, discovered_via
		FROM change_files WHERE change_id = ? ORDER BY path`, changeID)
	if err != nil {
		return nil, corruptState(err)
	}
	defer dbRows.Close()
	var files []FileChange
	for dbRows.Next() {
		var file FileChange
		if err := dbRows.Scan(&file.Path, &file.Kind, &file.OldPath, &file.Hash, &file.Via); err != nil {
			return nil, corruptState(err)
		}
		files = append(files, file)
	}
	if err := dbRows.Err(); err != nil {
		return nil, corruptState(err)
	}
	return files, nil
}
