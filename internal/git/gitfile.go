package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

// The two ways a real worktree stops resolving. Both report "not a git
// repository" from git and neither is fixed by `git init`, so both need a
// sentinel of their own — prose cannot be branched on (findings H7 and H8).
var (
	// ErrDanglingGitFile is a `.git` file whose `gitdir:` target no longer
	// exists. The worktree's files are all still there; only the pointer to the
	// repository is broken.
	ErrDanglingGitFile = errors.New("the .git file points at a repository that is not there")

	// ErrOrphanedWorktree is the linked-worktree shape of the same break: the
	// `.git` file points into a `worktrees/` administrative directory that has
	// been removed, usually with the repository that owned it.
	ErrOrphanedWorktree = errors.New("the linked worktree's repository is not there")
)

// gitFilePrefix is the whole of the `.git` file format: one line, this prefix,
// then a path that may be relative to the directory holding the file.
const gitFilePrefix = "gitdir: "

// linkedWorktreeDirName is the directory git keeps one administrative
// subdirectory per linked worktree in, immediately below the common dir.
const linkedWorktreeDirName = "worktrees"

// gitFileFault is a `.git` file that points nowhere, and what that implies.
type gitFileFault struct {
	// GitFile is the `.git` file itself.
	GitFile string
	// Target is where it points, made absolute against GitFile's directory.
	Target string
	// CommonDir is the repository the linked worktree belonged to, or "" when
	// the fault is not a linked worktree.
	CommonDir string
	// CommonDirPresent distinguishes a repository that is gone from one that is
	// still there and has merely forgotten this worktree. The remedies differ.
	CommonDirPresent bool
}

func (f gitFileFault) isLinkedWorktree() bool { return f.CommonDir != "" }

// brokenGitFileFault reports the broken `.git` pointer that explains git's
// refusal, or nil when there is not one.
//
// Two independent signals have to agree before it returns anything, because the
// cost of a false positive here is telling someone their working checkout is
// orphaned:
//
//   - git's own message has to be the indirection form. Measured against git
//     2.55, a `.git` pointer that does not resolve produces "not a git
//     repository:" with a colon, while a directory genuinely outside every
//     repository produces "not a git repository (or any of the parent
//     directories)" or "not a git repository (or any parent up to mount point
//     …)" — a bracket, never that colon.
//   - and a `.git` file that really is dangling has to be found on disk.
//
// Requiring both means the mount-point case cannot misfire: this walk does not
// stop at a filesystem boundary and git's does, but on the far side of one git
// prints the bracket form, so nothing is classified from a `.git` file git
// never looked at.
//
// Neither signal firing leaves the existing "not a git repository" verdict
// untouched, which is never actively wrong — the same degradation rule
// gitReportedNoRepository is written to.
func brokenGitFileFault(startDir, stderr string) *gitFileFault {
	if !gitReportedBrokenIndirection(stderr) {
		return nil
	}
	return findDanglingGitFile(startDir)
}

// gitReportedBrokenIndirection reports whether git's message is the one it
// prints when a pointer to a repository failed to resolve, as opposed to the
// one it prints when it walked up and found nothing.
func gitReportedBrokenIndirection(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "not a git repository:")
}

// findDanglingGitFile walks from startDir towards the filesystem root looking
// for the `.git` entry git would have used, and returns a fault only when that
// entry is a file whose target is not a directory on disk.
//
// The walk stops at the first `.git` entry of any kind. A `.git` *directory*
// ends it with no fault: git found one and still refused, which is some other
// condition and not this one.
func findDanglingGitFile(startDir string) *gitFileFault {
	for dir := filepath.Clean(startDir); ; {
		if fault, found := inspectGitEntry(dir); found {
			return fault
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

// inspectGitEntry classifies dir's `.git` entry. The second result reports
// whether the walk should stop here, so a nil fault and a stop mean "git's
// entry is here and it is not the thing we are looking for".
func inspectGitEntry(dir string) (*gitFileFault, bool) {
	path := filepath.Join(dir, ".git")

	// Lstat, not Stat: a `.git` symlink to a deleted target must not be read as
	// an absent entry and send the walk further up.
	info, err := os.Lstat(path)
	if err != nil {
		return nil, false
	}
	if info.IsDir() {
		return nil, true
	}

	target, ok := readGitFile(path, dir)
	if !ok {
		// Not the `gitdir:` format at all. git says "invalid gitfile format"
		// for that and we would not be on this branch, so it is not ours to
		// name — but it is still the entry git stopped at.
		return nil, true
	}
	if isDirectory(target) {
		// The pointer resolves. Whatever git objected to is not a dangling
		// pointer, so this classification stays out of it.
		return nil, true
	}

	fault := &gitFileFault{GitFile: path, Target: target}
	if commonDir, isLinked := linkedWorktreeCommonDir(target); isLinked {
		fault.CommonDir = commonDir
		fault.CommonDirPresent = isDirectory(commonDir)
	}
	return fault, true
}

// readGitFile parses a `.git` file the way git's own read_gitfile_gently does:
// the fixed prefix, then a path with trailing newlines and spaces removed. The
// parse is mirrored rather than improved on purpose — the only thing this is
// used for is deciding whether the path git resolved is missing, so looking at
// a different path than git did would be the one way to get it wrong.
//
// A relative target is resolved against the directory holding the file, which
// is what git does with it.
func readGitFile(path, dir string) (string, bool) {
	// A `.git` file is one short line. Anything larger is not one, and reading
	// it whole would be at the mercy of whatever is actually there.
	content, err := readSmallFile(path, maxGitFileSize)
	if err != nil {
		return "", false
	}

	line, ok := strings.CutPrefix(string(content), gitFilePrefix)
	if !ok {
		return "", false
	}
	target := strings.TrimRight(line, "\n ")
	if target == "" {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	return filepath.Clean(target), true
}

// maxGitFileSize bounds the read of a `.git` file. git's own limit for the
// format is far below this; the constant only exists so a `.git` file that is
// really something else cannot be pulled into memory whole.
const maxGitFileSize = 8 << 10

func readSmallFile(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, errors.New("file is too large to be a .git file")
	}
	return os.ReadFile(path)
}

// linkedWorktreeCommonDir reports the repository a linked worktree's
// administrative directory belongs to. git's layout is
// <common-dir>/worktrees/<name>, so the shared directory is two levels up, and
// the "worktrees" component is what marks the target as a linked worktree's
// rather than an ordinary repository's.
func linkedWorktreeCommonDir(target string) (string, bool) {
	parent := filepath.Dir(target)
	if filepath.Base(parent) != linkedWorktreeDirName {
		return "", false
	}
	commonDir := filepath.Dir(parent)
	if commonDir == parent {
		return "", false
	}
	return commonDir, true
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// asError renders the fault as the domain error for its shape.
func (f gitFileFault) asError(startDir, stderr string, cause error) error {
	if f.isLinkedWorktree() {
		return orphanedWorktreeError(startDir, f, stderr, cause)
	}
	return danglingGitFileError(startDir, f, stderr, cause)
}

// danglingGitFileError reports a worktree whose `.git` file points at a
// repository that is no longer on disk (finding H7).
//
// It keeps the NOT_A_GIT_REPOSITORY code, which is the stable machine contract
// and is not reworded (decision D-18), and the usage exit class of D-03. What
// it drops is the `git init` remedy: run literally here, `git init` succeeds,
// overwrites the `.git` file with a brand-new empty repository and destroys the
// only surviving record of where the real one was.
func danglingGitFileError(startDir string, fault gitFileFault, stderr string, cause error) error {
	return decorate(app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		"the .git file in this directory points at a repository that is not there",
		"Mindrail scopes all of its state to a repository, and this checkout no longer names one that exists",
		"inspect "+fault.GitFile+", which points at "+fault.Target+", and restore that directory or correct the path",
		"if the repository is gone for good, clone it again rather than creating a new one over this checkout",
	), startDir, stderr).
		WithMetadata("git_file", fault.GitFile).
		WithMetadata("git_file_target", fault.Target).
		WithCause(errors.Join(ErrNotARepository, ErrDanglingGitFile, joinStderr(cause, stderr)))
}

// orphanedWorktreeError reports a linked worktree cut off from the repository
// it belongs to (finding H8).
//
// `git init` is wrong here for the reason it is wrong above, and one step
// further: a linked worktree is administered from the main repository, so the
// action that resolves this is taken over there and not in this directory at
// all. Which action depends on whether the repository still exists, so that is
// established rather than guessed.
//
// Both branches were carried out against a real orphaned worktree before they
// were written down, and the first draft of them survived to pass 6 without
// being. It said to run `git worktree prune` in the surviving repository and
// then re-create the worktree with `git worktree add`, and neither half does
// anything: `prune` only deletes administrative records for worktrees that have
// gone missing, so against a record that is already gone it is a no-op that
// prints nothing, and `git worktree add` then refuses with
// "fatal: '<path>' already exists" because the directory is still there with the
// reader's files in it. `git worktree repair` was tried too and does not help —
// it re-links a record that exists, and this one does not. What works, measured,
// is to move the directory aside, add the worktree back, and put the files
// where they were.
func orphanedWorktreeError(startDir string, fault gitFileFault, stderr string, cause error) error {
	// `git worktree add` will not adopt a directory that already has anything in
	// it, so every route back goes through emptying this path first. Both
	// branches say so in the same words, because it is the same obstacle.
	recreate := "move " + startDir + " aside, run `git worktree add " + startDir +
		"` in the repository at " + fault.CommonDir + ", then copy your files back into it"

	why := "this is a linked worktree, and the repository it belongs to is not there"
	remedy := []string{
		"restore the repository at " + fault.CommonDir + ", which is where this worktree's administrative record lives",
		"or clone the repository again and then " + recreate,
	}
	if fault.CommonDirPresent {
		why = "this is a linked worktree, and the repository it belongs to no longer has a record of it"
		remedy = []string{
			recreate,
			"`git worktree prune` cannot restore this: it only removes records for worktrees that have gone missing, and this worktree's record is the one that is already gone",
		}
	}

	actions := append(remedy,
		"the files in this directory are not touched by git either way; copy anything unsaved out of it first")

	return decorate(app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		why,
		"Mindrail keeps a repository's state under its common directory, and this worktree can no longer reach one",
		actions...,
	), startDir, stderr).
		WithMetadata("git_file", fault.GitFile).
		WithMetadata("git_file_target", fault.Target).
		WithMetadata("common_dir", fault.CommonDir).
		WithCause(errors.Join(ErrNotARepository, ErrOrphanedWorktree, joinStderr(cause, stderr)))
}

// decorate attaches the two pieces of context every classification in this
// package carries: where the question was asked, and what git said.
func decorate(err *app.DomainError, startDir, stderr string) *app.DomainError {
	err = err.WithMetadata("start_dir", startDir)
	if stderr != "" {
		err = err.WithMetadata("git_stderr", stderr)
	}
	return err
}
