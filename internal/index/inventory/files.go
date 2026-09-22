package inventory

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/PsyChaos/mindrail/internal/index"
)

// File pairs a supported regular source file with the deepest discovered
// project unit that owns it. It is a read-only input to TASK-06 scheduling.
type File struct {
	Unit index.ProjectUnit
	Path string
}

var sourceExtensions = map[string]bool{
	".py": true, ".js": true, ".mjs": true, ".cjs": true,
	".ts": true, ".mts": true, ".cts": true, ".tsx": true,
}

// Files walks only this physical repository root. It does not follow symlinks
// or descend into generated directories or another linked worktree.
func Files(ctx context.Context, repoRoot string, units []index.ProjectUnit) ([]File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := CanonicalRoot(repoRoot)
	if err != nil {
		return nil, err
	}
	var files []File
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == ".mindrail" || entry.Name() == "node_modules" || isLinkedWorktreeRoot(path)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !sourceExtensions[filepath.Ext(path)] {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		unit, ok := Owner(path, units)
		if !ok || !contains(root, path) {
			return nil
		}
		files = append(files, File{Unit: unit, Path: path})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inventory: source walk %s: %w", root, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}
