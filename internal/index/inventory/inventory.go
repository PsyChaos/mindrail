// Package inventory discovers structural ProjectUnits without parsing source
// files. It owns the filesystem walk; the index Store owns the durable facts.
package inventory

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PsyChaos/mindrail/internal/index"
)

// Result is the durable set one discovery pass observed. Units are sorted by
// canonical absolute path so an unchanged repository gives the same answer on
// every run.
type Result struct {
	Units []index.ProjectUnit
}

// Discover finds the marker roots this structural-index milestone supports and
// records them through store. It never follows directory symlinks, so a marker
// outside the canonical repository root cannot enter its inventory.
func Discover(ctx context.Context, repoRoot string, store *index.Store) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if store == nil {
		return Result{}, fmt.Errorf("inventory: nil index store")
	}
	root, err := CanonicalRoot(repoRoot)
	if err != nil {
		return Result{}, err
	}

	markers := make(map[string]markerSet)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			// WalkDir does not follow directory symlinks, and ignoring file
			// symlinks here keeps a linked marker from naming an outside root.
			return nil
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == ".mindrail" || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			if path != root && isLinkedWorktreeRoot(path) {
				// A linked worktree has its own inventory ownership. Do not let
				// a parent checkout claim its markers merely because it happens
				// to contain the linked checkout on disk.
				return filepath.SkipDir
			}
			return nil
		}

		parent := filepath.Dir(path)
		if !contains(root, parent) {
			return nil
		}
		set := markers[parent]
		switch entry.Name() {
		case "pyproject.toml":
			set.python = true
		case "package.json":
			set.packageJSON = true
		case "tsconfig.json":
			set.tsconfig = true
		default:
			return nil
		}
		markers[parent] = set
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("inventory: walk %s: %w", root, err)
	}

	paths := make([]string, 0, len(markers))
	for path, marker := range markers {
		if marker.python || marker.packageJSON {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	result := Result{Units: make([]index.ProjectUnit, 0, len(paths))}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		unit, err := store.UpsertUnit(ctx, path, markers[path].kind())
		if err != nil {
			return Result{}, err
		}
		result.Units = append(result.Units, unit)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := store.ReconcileUnits(ctx, root, paths); err != nil {
		return Result{}, err
	}
	return result, nil
}

type markerSet struct {
	python      bool
	packageJSON bool
	tsconfig    bool
}

func (m markerSet) kind() index.UnitKind {
	// A colocated tsconfig refines a package.json unit. A directory with both
	// supported marker families deterministically selects Python; the schema
	// intentionally has one kind per root, while nested roots retain their own
	// more-specific units for the later file-assignment pass.
	if m.python {
		return index.UnitPython
	}
	if m.tsconfig {
		return index.UnitTypeScript
	}
	return index.UnitJavaScript
}

// Owner returns the longest ProjectUnit root that contains path. Prefix checks
// use filepath.Rel, not strings.HasPrefix, so /repo/web never owns /repo/website.
func Owner(path string, units []index.ProjectUnit) (index.ProjectUnit, bool) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return index.ProjectUnit{}, false
	}
	var owner index.ProjectUnit
	found := false
	for _, unit := range units {
		if !filepath.IsAbs(unit.Path) || filepath.Clean(unit.Path) != unit.Path || !contains(unit.Path, path) {
			continue
		}
		if !found || len(unit.Path) > len(owner.Path) || len(unit.Path) == len(owner.Path) && unit.ID < owner.ID {
			owner, found = unit, true
		}
	}
	return owner, found
}

// CanonicalRoot resolves an existing repository root to its clean physical
// path. Bootstrap calls it before scoped Store reads; the Store itself remains
// filesystem-free.
func CanonicalRoot(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("inventory: repository root is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("inventory: resolve repository root: %w", err)
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("inventory: canonicalize repository root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inventory: stat repository root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("inventory: repository root is not a directory: %s", root)
	}
	return filepath.Clean(root), nil
}

func isLinkedWorktreeRoot(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	return err == nil && !info.IsDir()
}

func contains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || filepath.IsAbs(rel) {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
