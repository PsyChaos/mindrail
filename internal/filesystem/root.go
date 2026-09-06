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
	"strconv"
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

// Modes are the permissions one create uses.
//
// The two travel together as a named value rather than as two os.FileMode
// arguments in a row, because a caller that swaps those two arguments gets no
// complaint from the compiler and a 0o600 directory at runtime.
//
// They are a parameter at all because not everything Mindrail creates is
// machine-local: the runtime tree is owner-only (spec §113), while the
// repository scaffolding under .mindrail/ is committed, reviewed and read by
// every contributor and by CI, so it takes ordinary repository modes. Both
// still have to pass the same containment boundary, and before this existed the
// package that needed the second pair went around the boundary to get it.
type Modes struct {
	Dir  os.FileMode
	File os.FileMode
}

// PrivateModes is the owner-only pair used for everything machine-local. It is
// a function rather than a variable so no caller can reassign the default out
// from under the rest of the process.
func PrivateModes() Modes {
	return Modes{Dir: DirMode, File: FileMode}
}

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
		return "", escapeError(rel, rel)
	}

	candidate := filepath.Join(r.canonical, filepath.FromSlash(rel))
	canonical, err := canonicalize(candidate)
	if err != nil {
		return "", err
	}
	if !r.holds(canonical) {
		return "", escapeError(rel, r.escapingComponent(rel))
	}
	return canonical, nil
}

// escapingComponent names the shallowest prefix of rel that already leaves the
// root, which is the entry a reader has to change.
//
// The requested path is not that answer, and quoting it made two commands
// disagree about one link: `status` resolves `.mindrail/config.toml` because it
// wants to read the file, `init` resolves `.mindrail` because it wants to create
// the directory, and both are refused by the same symbolic link one level up. A
// remedy that named what each command happened to ask for described the link as
// being in two places (finding F15).
//
// A prefix that cannot be canonicalized at all is the answer too — it is as far
// as the walk got, and the reader has to look there either way — and it needs no
// clause of its own to be. canonicalize returns the empty string with its error,
// no root holds the empty string, so the containment test below answers both
// cases. An `err != nil ||` in front of it read as a second condition and was a
// restatement of the first: it could not change an outcome, which is the shape
// finding F16 is about, so it is written down here instead of left in the code
// looking load-bearing.
func (r Root) escapingComponent(rel string) string {
	cleaned := filepath.Clean(filepath.FromSlash(rel))
	parts := strings.Split(filepath.ToSlash(cleaned), "/")

	for i := range parts {
		prefix := filepath.Join(r.canonical, filepath.FromSlash(strings.Join(parts[:i+1], "/")))
		canonical, _ := canonicalize(prefix)
		if !r.holds(canonical) {
			return prefix
		}
	}
	return filepath.Join(r.canonical, cleaned)
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
		return "", escapeError(abs, abs)
	}

	rel, err := filepath.Rel(r.canonical, canonical)
	if err != nil {
		return "", escapeError(abs, abs)
	}
	return filepath.ToSlash(rel), nil
}

// EnsureDir creates a directory inside the root, together with any missing
// parents, and returns its absolute path. It is a no-op when the directory is
// already there; an existing non-directory is an error rather than a silent
// replacement.
func (r Root) EnsureDir(rel string) (string, error) {
	abs, _, err := r.EnsureDirMode(rel, DirMode)
	return abs, err
}

// EnsureDirMode is EnsureDir with the directory permission spelled out, and it
// also reports whether the directory was created by this call.
//
// The second result is what lets a caller say what an operation changed without
// a stat of its own: a stat outside this package would be taken on a path the
// boundary has not vetted, which is the mistake this method exists to remove.
func (r Root) EnsureDirMode(rel string, mode os.FileMode) (abs string, created bool, err error) {
	abs, err = r.Resolve(rel)
	if err != nil {
		return "", false, err
	}

	if info, statErr := os.Stat(abs); statErr == nil {
		if !info.IsDir() {
			return "", false, fmt.Errorf("filesystem: %q: %w", abs, ErrNotDirectory)
		}
		return abs, false, nil
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return "", false, fmt.Errorf("filesystem: stat %q: %w", abs, statErr)
	}

	if err := os.MkdirAll(abs, mode); err != nil {
		return "", false, fmt.Errorf("filesystem: create directory %q: %w", abs, err)
	}
	return abs, true, nil
}

// WriteFileIfAbsent creates a file only when nothing occupies its path, and
// reports whether it wrote. Existing content is never overwritten: spec §82
// requires init to leave a user's configuration alone.
//
// The create is O_EXCL, so an attacker who wins the race between the
// containment check and the open still cannot get Mindrail to follow a
// planted symlink.
func (r Root) WriteFileIfAbsent(rel string, data []byte) (bool, string, error) {
	return r.WriteFileIfAbsentMode(rel, data, PrivateModes())
}

// WriteFileIfAbsentMode is WriteFileIfAbsent with the permissions spelled out,
// for content that is repository material rather than machine-local state.
func (r Root) WriteFileIfAbsentMode(rel string, data []byte, modes Modes) (bool, string, error) {
	abs, err := r.Resolve(rel)
	if err != nil {
		return false, "", err
	}

	if err := os.MkdirAll(filepath.Dir(abs), modes.Dir); err != nil {
		return false, abs, fmt.Errorf("filesystem: create parent of %q: %w", abs, err)
	}

	file, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, modes.File)
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
//
// inspect is what the reader has to change, and quoting it is the whole of
// finding F15. For a path resolved against the root it is the absolute
// component that leaves it; for a configured override it is the value as
// configured, because that is the thing the reader edits and an absolute
// rendering of `../outside` would name a directory nobody wrote down.
// "Replace any symbolic link on the path that points
// outside the repository" named no path at all, so in a repository with more
// than one link the reader had to hunt for the one meant — while every
// neighbouring condition, obstruction and permission and space and read-only
// mount, already quotes the path it is about. The sentence also carried no
// absolute path for the invariant that compares what two documents say about
// one path, so it could never be caught contradicting the error object beside
// it.
func escapeError(target, inspect string) error {
	return app.NewError(
		app.CodePathEscapesRoot,
		app.KindUsage,
		"the path "+strconv.Quote(inspect)+" resolves outside the repository root",
		"Mindrail refuses the operation rather than reading or writing outside the repository",
		"use a path inside the repository",
		// One sentence for both causes, because escapeError serves both: a
		// configured value carrying a ".." segment, and a resolved path with a
		// symbolic link on it that leaves the root. Naming only the link would
		// be wrong for the first, and naming neither is what F15 is.
		"check "+strconv.Quote(inspect)+` for a ".." segment or a symbolic link that leaves the repository`,
	).WithMetadata("path", target).
		WithMetadata("inspect_path", inspect).
		WithCause(ErrEscapesRoot)
}
