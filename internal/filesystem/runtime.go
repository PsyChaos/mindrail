package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProductName is the single spelling of the product used in on-disk names. It
// lives here because every directory and file Mindrail creates is derived from
// it, and a second spelling anywhere would strand an existing installation.
const ProductName = "mindrail"

// databaseFileName and cacheDirName complete the layout of tech-stack §112.
const (
	databaseFileName = ProductName + ".db"
	cacheDirName     = "cache"
	repoConfigDir    = "." + ProductName
)

// RuntimePaths are the machine-local locations Mindrail works with, all derived
// from the Git common-dir so that every linked worktree of a repository shares
// one runtime state (tech-stack §112).
//
// These are locations, not identity: decision D-26 keeps identity in opaque ids
// precisely because a clone can move.
type RuntimePaths struct {
	CommonDir     string `json:"common_dir"`
	WorktreeRoot  string `json:"worktree_root"`
	RuntimeRoot   string `json:"runtime_root"`
	DBPath        string `json:"db_path"`
	CacheDir      string `json:"cache_dir"`
	RepoConfigDir string `json:"repo_config_dir"`
}

// PathOptions is the input to path derivation. The two overrides exist because
// tech-stack §131 requires injectable roots and the clean-binary smoke test
// drives a compiled binary, which cannot be wired through a constructor
// (decision D-07). They are isolation seams, not a user feature.
type PathOptions struct {
	CommonDir          string
	WorktreeRoot       string
	RuntimeDirOverride string
	CacheDirOverride   string
}

// ResolveRuntimePaths derives every runtime location from the Git layout. It
// touches no disk: derivation must be answerable for a repository Mindrail has
// never been run in, so that status can report a missing setup rather than
// creating one (decision D-01).
func ResolveRuntimePaths(o PathOptions) (RuntimePaths, error) {
	commonDir, err := requireAbsolute("common dir", o.CommonDir)
	if err != nil {
		return RuntimePaths{}, err
	}
	worktreeRoot, err := requireAbsolute("worktree root", o.WorktreeRoot)
	if err != nil {
		return RuntimePaths{}, err
	}

	runtimeRoot, err := overrideOrDefault(commonDir, o.RuntimeDirOverride, filepath.Join(commonDir, ProductName))
	if err != nil {
		return RuntimePaths{}, err
	}
	cacheDir, err := overrideOrDefault(runtimeRoot, o.CacheDirOverride, filepath.Join(runtimeRoot, cacheDirName))
	if err != nil {
		return RuntimePaths{}, err
	}

	return RuntimePaths{
		CommonDir:     commonDir,
		WorktreeRoot:  worktreeRoot,
		RuntimeRoot:   runtimeRoot,
		DBPath:        filepath.Join(runtimeRoot, databaseFileName),
		CacheDir:      cacheDir,
		RepoConfigDir: filepath.Join(worktreeRoot, repoConfigDir),
	}, nil
}

// EnsureDirs creates the runtime directories with owner-only permissions and
// does nothing when they already exist.
//
// RepoConfigDir is deliberately not created: it is repository content owned by
// init, and conjuring it here would let status or doctor mutate the working
// tree, which spec §83 forbids.
func (p RuntimePaths) EnsureDirs() error {
	for _, r := range p.roots() {
		if err := ensureDir(r.kind, r.dir); err != nil {
			return err
		}
	}
	return nil
}

// ensureDir creates one runtime directory, classifying what stops it the same
// way ProbeWritable does.
//
// The two obstructions checked before MkdirAll are the ones MkdirAll cannot
// describe. A dangling symlink is invisible to it -- it reports the EEXIST of a
// link it cannot write through, which was rendered as "could not be created,
// check the permissions" over a parent whose permissions were already correct
// (finding F6). A file in the way produces ENOTDIR, which was rendered as a
// different sentence from the one the probe prints for the identical condition
// (finding F8). Naming them here is what makes `doctor` and `init` agree.
func ensureDir(kind RootKind, dir string) error {
	// Both obstructions are named by ObstructedDir, which is also what the
	// config loader asks before it reads: one function answers "is something
	// else standing here?" for every caller, so no two of them can answer it
	// differently.
	if err := ObstructedDir(kind, dir); err != nil {
		return err
	}

	if err := os.MkdirAll(dir, DirMode); err != nil {
		return unwritableError(kind, dir, err)
	}
	info, statErr := os.Stat(dir)
	if statErr != nil {
		return unwritableError(kind, dir, statErr)
	}
	if !info.IsDir() {
		return unwritableError(kind, dir, ErrNotDirectory)
	}
	return nil
}

// Exists reports whether the runtime database file is already present. It is
// the "has this repository been initialized?" question, and it is answered by a
// stat rather than by opening the database so that a read-only command can ask
// it without creating anything.
func (p RuntimePaths) Exists() bool {
	if p.DBPath == "" {
		return false
	}
	info, err := os.Stat(p.DBPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// requireAbsolute rejects the empty and relative forms. A relative common-dir
// would make every derived path depend on the process working directory, which
// changes between a shell invocation and a Git hook.
func requireAbsolute(label, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("filesystem: %s is required", label)
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("filesystem: %s %q must be absolute", label, value)
	}
	return filepath.Clean(value), nil
}

// overrideOrDefault applies an isolation override, or falls back to the derived
// location. An override is refused when it is spelled with a parent segment:
// cleaning one away would silently accept `.../.git/../../etc` from an
// environment variable, and the override exists to relocate state deliberately,
// never to walk out of somewhere.
func overrideOrDefault(base, override, derived string) (string, error) {
	if strings.TrimSpace(override) == "" {
		return derived, nil
	}
	if hasParentSegment(override) {
		return "", escapeError(override, override)
	}
	if filepath.IsAbs(override) {
		return filepath.Clean(override), nil
	}
	return filepath.Join(base, filepath.FromSlash(override)), nil
}

func hasParentSegment(p string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(p), "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

// unwritableError reports a runtime location Mindrail could not create. It is
// an environment condition rather than a user mistake, so it maps to the
// "temporarily unavailable" exit class of decision D-03.
//
// The remedy names the nearest directory that actually exists, for the same
// reason ProbeWritable reports it: when the create failed because the parent is
// read-only, the mode to fix is the parent's.
func unwritableError(kind RootKind, dir string, cause error) error {
	probed, _, _ := nearestExisting(dir)
	return unwritablePathError(kind, dir, probed, cause)
}
