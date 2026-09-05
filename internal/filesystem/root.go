// Package filesystem owns every path decision Mindrail makes: where the
// containment boundary of a repository is, how a path is proved to be inside
// it, and how paths are spelled when they are stored or printed.
//
// Nothing above this package is allowed to hand a raw os path to the operating
// system, because "is this path inside the repository?" is a security question
// (spec §113) and it deserves exactly one answer.
package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

// Permission modes for everything Mindrail creates. Runtime state can carry
// repository contents and local configuration, so it is owner-only by default
// rather than by later hardening (spec §113).
const (
	DirMode  os.FileMode = 0o700
	FileMode os.FileMode = 0o600
)

var (
	// ErrEscapesRoot reports a path that resolves outside its containment
	// boundary. It stays a plain sentinel so callers can branch on it with
	// errors.Is even when it arrives wrapped in a domain error.
	ErrEscapesRoot = errors.New("path escapes the allowed root")

	// ErrNotDirectory reports a path that exists but is not a directory.
	ErrNotDirectory = errors.New("path is not a directory")

	// errUninitializedRoot guards the zero Root. A zero value would otherwise
	// contain every path relative to the process working directory, which is
	// the opposite of what a containment boundary is for.
	errUninitializedRoot = errors.New("filesystem: root is not initialized")
)

// maxSymlinkHops bounds link traversal so a symlink cycle cannot be turned into
// an unbounded recursion by an untrusted repository.
const maxSymlinkHops = 64

// Root is a canonicalized containment boundary. It is constructed once per
// repository and consulted for every path derived from untrusted input.
//
// Canonicalization happens at construction, not per call, so the policy of
// decision D-32 holds: symbolic links inside the root are allowed, and links
// that leave it are refused. Denying links outright would break the macOS
// /tmp -> /private/tmp fixture path and ordinary developer layouts.
type Root struct {
	canonical string
}

// NewRoot canonicalizes dir into a containment boundary. The directory must
// already exist: a boundary that cannot be resolved cannot be enforced, and
// silently accepting a missing root would let every later containment check
// pass against a path that does not mean what the caller thinks.
func NewRoot(dir string) (Root, error) {
	if strings.TrimSpace(dir) == "" {
		return Root{}, fmt.Errorf("filesystem: %w", errUninitializedRoot)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return Root{}, fmt.Errorf("filesystem: absolute path of %q: %w", dir, err)
	}

	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Root{}, fmt.Errorf("filesystem: canonicalize root %q: %w", dir, err)
	}

	info, err := os.Stat(canonical)
	if err != nil {
		return Root{}, fmt.Errorf("filesystem: stat root %q: %w", dir, err)
	}
	if !info.IsDir() {
		return Root{}, fmt.Errorf("filesystem: root %q: %w", dir, ErrNotDirectory)
	}

	return Root{canonical: filepath.Clean(canonical)}, nil
}

// Path returns the canonical absolute path of the boundary.
func (r Root) Path() string { return r.canonical }

// Resolve turns a repository-relative path into a containment-checked absolute
// path. The result is canonical, so a caller that hands it straight to the
// operating system cannot be redirected by a link it did not inspect.
//
// The path need not exist yet; only the part of it that does exist is followed.
func (r Root) Resolve(rel string) (string, error) {
	if r.canonical == "" {
		return "", errUninitializedRoot
	}
	if isAbsoluteInput(rel) {
		return "", escapeError(rel)
	}

	candidate := filepath.Join(r.canonical, filepath.FromSlash(rel))
	canonical, err := canonicalize(candidate)
	if err != nil {
		return "", err
	}
	if !r.holds(canonical) {
		return "", escapeError(rel)
	}
	return canonical, nil
}

// Contains reports whether an absolute path canonicalizes to the root or a
// descendant of it. A path that is simply outside is not an error — that is the
// question being asked — so the error return is reserved for a boundary that
// could not be evaluated at all.
func (r Root) Contains(abs string) (bool, error) {
	canonical, err := r.canonicalizeAbsolute(abs)
	if err != nil {
		return false, err
	}
	return r.holds(canonical), nil
}

// RelSlash renders an absolute path as repository-relative with "/" separators,
// which is the only spelling Mindrail persists or prints (tech-stack §74). The
// root itself renders as ".".
func (r Root) RelSlash(abs string) (string, error) {
	canonical, err := r.canonicalizeAbsolute(abs)
	if err != nil {
		return "", err
	}
	if !r.holds(canonical) {
		return "", escapeError(abs)
	}

	rel, err := filepath.Rel(r.canonical, canonical)
	if err != nil {
		return "", escapeError(abs)
	}
	return filepath.ToSlash(rel), nil
}

// EnsureDir creates a directory inside the root, together with any missing
// parents, and returns its absolute path. It is a no-op when the directory is
// already there; an existing non-directory is an error rather than a silent
// replacement.
func (r Root) EnsureDir(rel string) (string, error) {
	abs, err := r.Resolve(rel)
	if err != nil {
		return "", err
	}

	if info, statErr := os.Stat(abs); statErr == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("filesystem: %q: %w", abs, ErrNotDirectory)
		}
		return abs, nil
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return "", fmt.Errorf("filesystem: stat %q: %w", abs, statErr)
	}

	if err := os.MkdirAll(abs, DirMode); err != nil {
		return "", fmt.Errorf("filesystem: create directory %q: %w", abs, err)
	}
	return abs, nil
}

// WriteFileIfAbsent creates a file only when nothing occupies its path, and
// reports whether it wrote. Existing content is never overwritten: spec §82
// requires init to leave a user's configuration alone.
//
// The create is O_EXCL, so an attacker who wins the race between the
// containment check and the open still cannot get Mindrail to follow a
// planted symlink.
func (r Root) WriteFileIfAbsent(rel string, data []byte) (bool, string, error) {
	abs, err := r.Resolve(rel)
	if err != nil {
		return false, "", err
	}

	if err := os.MkdirAll(filepath.Dir(abs), DirMode); err != nil {
		return false, abs, fmt.Errorf("filesystem: create parent of %q: %w", abs, err)
	}

	file, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, FileMode)
	if errors.Is(err, fs.ErrExist) {
		return false, abs, nil
	}
	if err != nil {
		return false, abs, fmt.Errorf("filesystem: create %q: %w", abs, err)
	}

	if _, writeErr := file.Write(data); writeErr != nil {
		// A half-written file is worse than no file: the next run would take
		// the truncated content for a deliberate user edit.
		_ = file.Close()
		_ = os.Remove(abs)
		return false, abs, fmt.Errorf("filesystem: write %q: %w", abs, writeErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		_ = os.Remove(abs)
		return false, abs, fmt.Errorf("filesystem: close %q: %w", abs, closeErr)
	}
	return true, abs, nil
}

// Normalize converts an OS-native relative path to the stored slash form. The
// empty path stays empty rather than becoming ".", so a caller can tell "no
// path" from "the current directory".
//
// Traversal is cleaned but not rejected here: Normalize is a spelling rule, and
// containment is Root's job.
func Normalize(p string) string {
	if p == "" {
		return ""
	}
	return path.Clean(filepath.ToSlash(p))
}

// holds reports whether an already-canonical path is the root or below it.
// filepath.Rel is used rather than a string prefix so that a sibling directory
// sharing the root's name prefix cannot pass.
func (r Root) holds(canonical string) bool {
	if canonical == r.canonical {
		return true
	}
	rel, err := filepath.Rel(r.canonical, canonical)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return rel != "" && !filepath.IsAbs(rel)
}

// canonicalizeAbsolute validates and canonicalizes an absolute input path.
func (r Root) canonicalizeAbsolute(abs string) (string, error) {
	if r.canonical == "" {
		return "", errUninitializedRoot
	}
	if !filepath.IsAbs(abs) {
		return "", fmt.Errorf("filesystem: %q is not an absolute path", abs)
	}
	return canonicalize(abs)
}

// isAbsoluteInput rejects both native absolute paths and the slash-rooted form,
// so a stored "/etc/passwd" cannot be joined onto the root and quietly land
// inside it.
func isAbsoluteInput(p string) bool {
	return filepath.IsAbs(p) || strings.HasPrefix(p, "/")
}

// canonicalize resolves symbolic links in the longest existing prefix of abs
// and re-attaches the remainder literally. A path that does not exist yet still
// has to be containment-checked — that is the whole point of checking before a
// create — and filepath.EvalSymlinks refuses those outright.
func canonicalize(abs string) (string, error) {
	return canonicalizeHop(filepath.Clean(abs), 0)
}

func canonicalizeHop(abs string, hops int) (string, error) {
	if hops > maxSymlinkHops {
		return "", fmt.Errorf("filesystem: too many symbolic links resolving %q", abs)
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("filesystem: canonicalize %q: %w", abs, err)
	}

	parent := filepath.Dir(abs)
	if parent == abs {
		return abs, nil
	}
	canonicalParent, err := canonicalizeHop(parent, hops+1)
	if err != nil {
		return "", err
	}

	candidate := filepath.Join(canonicalParent, filepath.Base(abs))

	// A dangling symlink is reported as "does not exist", yet it still steers
	// a create at its target. Follow it by hand so an escape through one is
	// caught rather than treated as a plain missing file.
	if info, lstatErr := os.Lstat(candidate); lstatErr == nil && info.Mode()&fs.ModeSymlink != 0 {
		target, readErr := os.Readlink(candidate)
		if readErr != nil {
			return "", fmt.Errorf("filesystem: read link %q: %w", candidate, readErr)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(canonicalParent, target)
		}
		return canonicalizeHop(filepath.Clean(target), hops+1)
	}

	return candidate, nil
}

// escapeError pairs the sentinel with the structured payload the CLI renders,
// so errors.Is and app.PayloadOf both work on the same value (tech-stack §72).
func escapeError(target string) error {
	return app.NewError(
		app.CodePathEscapesRoot,
		app.KindUsage,
		"the path resolves outside the repository root",
		"Mindrail refuses the operation rather than reading or writing outside the repository",
		"use a path inside the repository",
		"replace any symbolic link on the path that points outside the repository",
	).WithMetadata("path", target).WithCause(ErrEscapesRoot)
}
