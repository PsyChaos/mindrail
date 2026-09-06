package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

// The failure modes of repository discovery. They stay plain sentinels so
// callers can branch on them with errors.Is, and every one of them is also
// wrapped in an *app.DomainError so the CLI can render a code, an impact and a
// remedy for the same value (tech-stack §72).
var (
	ErrNotARepository = errors.New("not inside a git repository")
	ErrBareRepository = errors.New("bare repository has no worktree")
	ErrGitUnavailable = errors.New("git executable not available")
	ErrGitTimeout     = errors.New("git command timed out")

	// ErrInsideGitDirectory is the narrower case of ErrNotARepository: the
	// start directory is inside a repository's .git directory rather than
	// outside every repository. It is a separate sentinel because the two have
	// different remedies — `git init` is right for one and actively harmful for
	// the other — and prose alone cannot be branched on.
	ErrInsideGitDirectory = errors.New("inside a git directory rather than a worktree")

	// ErrForeignWorktree is the narrower case of ErrNotARepository where the
	// repository's configured working tree is a directory that belongs to a
	// different repository. It is separate because it is the one shape in this
	// family with no command that re-links it: the remedies for its neighbours
	// either refuse or damage somebody else's checkout when run here (finding
	// E6).
	ErrForeignWorktree = errors.New("the configured working tree belongs to a different repository")

	// ErrLinkedWorktreeAdminDir is the narrower case of ErrInsideGitDirectory
	// where the start directory is the administrative directory a repository
	// keeps for one linked worktree, `<common-dir>/worktrees/<name>`. It is
	// separate because that directory has a working tree of its own, and
	// reporting the main worktree's paths describes a different place (finding
	// E9).
	ErrLinkedWorktreeAdminDir = errors.New("inside a linked worktree's administrative directory")

	// ErrWorkingDirectory is the narrower case of ErrNotARepository where the
	// start directory itself could not be entered, so git never ran. It is
	// separate because the remedy is about the path the caller supplied, not
	// about repositories.
	ErrWorkingDirectory = errors.New("the start directory could not be entered")

	// ErrRepositoryUnreadable is the narrower case of ErrGitUnavailable where
	// git ran, failed, and said something other than "there is no repository
	// here" — a damaged object store, an unparseable config, a repository
	// format this git does not know, a refused ownership check. The binary is
	// fine and the directory may well be a repository, so neither
	// ErrGitUnavailable's remedy nor ErrNotARepository's applies unchanged.
	ErrRepositoryUnreadable = errors.New("git could not read the repository")
)

// Repository is the resolved Git layout of one working directory.
//
// The distinction that matters is CommonDir versus GitDir: linked worktrees
// share the first and each own the second, which is why runtime state hangs off
// the common dir (tech-stack §112).
type Repository struct {
	CommonDir        string `json:"common_dir"`
	GitDir           string `json:"git_dir"`
	WorktreeRoot     string `json:"worktree_root"`
	IsLinkedWorktree bool   `json:"is_linked_worktree"`
	IsBare           bool   `json:"is_bare"`
}

// Adapter answers repository questions by running git.
type Adapter struct {
	runner CommandRunner
}

// NewAdapter wires an adapter to a runner. The runner is a parameter rather
// than a package default because every seam in MR-001 is constructor-injected
// (decision D-34).
func NewAdapter(runner CommandRunner) *Adapter {
	return &Adapter{runner: runner}
}

// Resolve determines the Git layout containing startDir.
//
// Each answer comes from its own rev-parse rather than from parsing one
// combined output, because rev-parse's own resolution is the thing being
// trusted here; reimplementing the .git-file and worktree indirection in Go is
// exactly what tech-stack §34 rules out.
func (a *Adapter) Resolve(ctx context.Context, startDir string) (Repository, error) {
	insideWorktree, stderr, err := a.revParse(ctx, startDir, "--is-inside-work-tree")
	if err != nil {
		// git answering "no" and git being unable to answer are different
		// conditions; only the first one means "not a repository".
		if propagated := propagate(err); propagated != nil {
			return Repository{}, propagated
		}
		return Repository{}, a.classifyProbeFailure(startDir, stderr, err)
	}

	if !parseGitBool(insideWorktree) {
		return Repository{}, a.classifyMissingWorktree(ctx, startDir)
	}

	commonDir, err := a.absolutePath(ctx, startDir, "--git-common-dir")
	if err != nil {
		return Repository{}, err
	}
	gitDir, err := a.absolutePath(ctx, startDir, "--git-dir")
	if err != nil {
		return Repository{}, err
	}
	worktreeRoot, err := a.absolutePath(ctx, startDir, "--show-toplevel")
	if err != nil {
		return Repository{}, err
	}
	bare, bareStderr, err := a.revParse(ctx, startDir, "--is-bare-repository")
	if err != nil {
		return Repository{}, wrapUnexpected(err, bareStderr)
	}

	return Repository{
		CommonDir:        commonDir,
		GitDir:           gitDir,
		WorktreeRoot:     worktreeRoot,
		IsLinkedWorktree: gitDir != commonDir,
		IsBare:           parseGitBool(bare),
	}, nil
}

// Version reports the system git version verbatim, as `git --version` prints
// it ("git version 2.55.0"). The raw line is kept because doctor shows it to a
// human and a trimmed-down form would lose the vendor suffixes some
// distributions add.
func (a *Adapter) Version(ctx context.Context) (string, error) {
	stdout, stderr, err := a.runner.Run(ctx, "", "--version")
	if err != nil {
		if propagated := propagate(err); propagated != nil {
			return "", propagated
		}
		// git's own words are the only text that says what went wrong; losing
		// them leaves the reader with an exit status.
		return "", unavailableError("git", stderrSummary(string(stderr)), err)
	}

	version := strings.TrimSpace(string(stdout))
	if version == "" {
		return "", unavailableError("git", "", errors.New("git --version produced no output"))
	}
	return version, nil
}

// classifyMissingWorktree distinguishes a bare repository from a directory that
// is inside the Git directory of an ordinary one. Both report "false" for
// --is-inside-work-tree, and only the first has a name a user can act on.
//
// The non-bare branch then asks git *where* the worktree is instead of assuming
// it. The assumption — that the Git directory is named `.git` and that its
// parent is the worktree — is false for every repository that sets
// core.worktree: submodule Git directories set it, and any repository can. In
// those, git answers --show-toplevel perfectly well from inside the Git
// directory, so guessing was never necessary (finding D6).
func (a *Adapter) classifyMissingWorktree(ctx context.Context, startDir string) error {
	bare, stderr, err := a.revParse(ctx, startDir, "--is-bare-repository")
	if err != nil {
		if propagated := propagate(err); propagated != nil {
			return propagated
		}
		return a.classifyProbeFailure(startDir, stderr, err)
	}

	if parseGitBool(bare) {
		return bareRepositoryError(startDir)
	}

	// Reaching here means git found a repository, said the directory is not in
	// its worktree, and said the repository is not bare. Which worktree that
	// repository has, and whether the reader is standing inside its Git
	// directory or merely outside its worktree, are questions only git can
	// answer — and the second one used to be assumed rather than asked, which is
	// how an ordinary repository with core.worktree set was told it was "inside
	// a Git directory" while its owner stood in the checkout root (finding E7).
	worktree, err := a.probeWorktreeRoot(ctx, startDir)
	if err != nil {
		return err
	}
	commonDir, err := a.probeCommonDir(ctx, startDir)
	if err != nil {
		return err
	}
	standing := standpointOf(startDir, commonDir)

	if worktree == "" {
		// git cannot name a worktree from here. That is the ordinary
		// standing-inside-.git case, the linked worktree's administrative
		// directory, and also the `--separate-git-dir` case where no back-pointer
		// to the worktree exists at all; the Git directory's own path is what
		// separates them.
		if linked := linkedWorktreeOfAdminDir(startDir, commonDir); linked != "" {
			return linkedWorktreeAdminError(startDir, commonDir, linked)
		}
		return insideGitDirectoryError(startDir, commonDir)
	}

	// core.worktree names a worktree, but naming one and being able to work in
	// one are different claims, and the remedy about to be printed is "go there".
	// So the claim is checked before it is made: an unenterable path and a path
	// git finds no repository in both send the reader somewhere that fails again,
	// which is the loop this whole finding is about.
	if fault := directoryEntryFault(worktree); fault != nil {
		return unenterableWorktreeError(startDir, worktree, fault, standing)
	}
	link, err := a.probeWorktreeLinkage(ctx, worktree, commonDir)
	if err != nil {
		return err
	}
	if link.reaches {
		return detachedWorktreeError(startDir, worktree, standing)
	}

	// Both remaining shapes have to name the per-worktree Git directory, which
	// is the one a `.git` back-pointer has to contain. It is asked for here and
	// nowhere above so the branches that do not need it keep their probe count.
	gitDir, err := a.probeGitDir(ctx, startDir)
	if err != nil {
		return err
	}
	if link.commonDir != "" {
		return foreignWorktreeError(startDir, gitDir, worktree, link, standing)
	}
	return unlinkedWorktreeError(startDir, gitDir, worktree, standing)
}

// standpoint records where the reader is standing relative to the repository
// git just found: inside its Git directory, or outside its working tree with
// the Git directory somewhere else entirely.
//
// The distinction is the whole of finding E7. Every message in this family
// opened with "the directory is inside a Git directory", which is true of a
// submodule's `.git/modules/<name>` and false of an ordinary checkout root
// whose config happens to say `core.worktree = /elsewhere` — and the second is
// an ordinary user standing in what they correctly believe is their repository.
type standpoint struct{ insideGitDir bool }

// standpointOf answers the question from the common directory alone.
//
// The common directory is enough because git's per-worktree Git directory is
// always the common one or a directory beneath it (`<common>/worktrees/<name>`),
// so anything inside the former is inside the latter. Asking for --git-dir as
// well would buy nothing and cost a subprocess on every classification.
//
// When git declined to name a common directory the answer is unknown, and
// unknown keeps the wording every one of these messages has always used: we
// cannot claim the reader is outside a directory we cannot locate.
func standpointOf(startDir, commonDir string) standpoint {
	if commonDir == "" {
		return standpoint{insideGitDir: true}
	}
	return standpoint{insideGitDir: pathWithin(startDir, commonDir)}
}

// impact is the consequence of the condition, which differs by standpoint: a
// reader inside a Git directory is one `git init` away from nesting a
// repository inside an administrative directory, and a reader outside a working
// tree is not.
func (s standpoint) impact() string {
	if s.insideGitDir {
		return gitDirectoryImpact
	}
	return outsideWorktreeImpact
}

// cause is the sentinel set for the condition. ErrInsideGitDirectory is
// documented as the narrower case of ErrNotARepository where the start
// directory is inside a repository's Git directory, so it is attached only when
// the reader is actually there; a checkout root whose core.worktree points
// elsewhere is not, and a sentinel that is true of everything can be branched
// on by nobody.
func (s standpoint) cause() error {
	if s.insideGitDir {
		return errors.Join(ErrNotARepository, ErrInsideGitDirectory)
	}
	return ErrNotARepository
}

// pathWithin reports whether path is dir or lies below it.
//
// The comparison is by file identity at every step rather than by string
// prefix, so a repository reached through a symlink — or a /tmp that is really
// /private/tmp — is not read as a different place. That matters here because
// the answer decides which sentence a user is shown about their own checkout.
func pathWithin(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	for current := filepath.Clean(path); ; {
		if sameDirectory(current, dir) {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

// worktreeLinkage is what git finds in the directory core.worktree names.
//
// The bare "does it point back" boolean this replaces collapsed two conditions
// with opposite remedies. A directory with no repository in it can be linked to
// this Git directory, and `git init --separate-git-dir` does exactly that. A
// directory that already belongs to *another* repository cannot: git refuses to
// move that repository's Git directory onto this one, and where the directory
// is merely inside another repository's working tree the same command succeeds
// by carving a nested repository out of it. Printing the linking command for
// both is finding E6.
type worktreeLinkage struct {
	// reaches reports that the directory is a working tree of this very
	// repository, so the link is sound and only the reader is in the wrong place.
	reaches bool
	// commonDir is the Git directory of the *other* repository the configured
	// working tree belongs to, or "" when git finds no repository there at all.
	commonDir string
	// toplevel is that other repository's working tree root, or "" when git
	// named none — the configured path may be a bare repository or a Git
	// directory rather than a checkout.
	toplevel string
}

// probeWorktreeLinkage asks git, in the configured working tree, which
// repository that directory belongs to.
//
// core.worktree is a one-way link. A worktree with no `.git` entry pointing
// back is invisible to discovery, so "change into it" would land the reader on
// `NOT_A_GIT_REPOSITORY` — a second wrong remedy rather than the first one
// fixed. Running git there is the only way to know, and it is what the reader
// is about to do anyway.
//
// An unknown answer counts as "no repository there", which is the conservative
// end: its remedy writes a link, and writing a link that is already sound costs
// nothing, while the confident remedy sends the reader into a directory that
// may refuse them.
func (a *Adapter) probeWorktreeLinkage(ctx context.Context, worktree, commonDir string) (worktreeLinkage, error) {
	if commonDir == "" {
		// Without knowing which repository the reader is in, "this is a different
		// repository" is a claim that cannot be made. Degrade to the shape whose
		// remedy is harmless.
		return worktreeLinkage{}, nil
	}

	inside, _, err := a.revParse(ctx, worktree, "--is-inside-work-tree")
	if err != nil {
		if propagated := propagate(err); propagated != nil {
			return worktreeLinkage{}, propagated
		}
		return worktreeLinkage{}, nil
	}

	reached, err := a.probeCommonDir(ctx, worktree)
	if err != nil {
		return worktreeLinkage{}, err
	}
	if reached == "" || sameDirectory(reached, commonDir) {
		// Same repository. Whether git also calls the directory a working tree
		// decides between "the reader is merely in the wrong place" and a
		// core.worktree pointing into this repository's own administrative
		// directory, which is not a different repository and must not be
		// described as one.
		return worktreeLinkage{reaches: reached != "" && parseGitBool(inside)}, nil
	}

	link := worktreeLinkage{commonDir: reached}
	if parseGitBool(inside) {
		toplevel, err := a.probeWorktreeRoot(ctx, worktree)
		if err != nil {
			return worktreeLinkage{}, err
		}
		link.toplevel = toplevel
	}
	return link, nil
}

// sameDirectory compares two paths by identity rather than by spelling, so a
// symlinked repository root or a /tmp that is really /private/tmp does not read
// as a different repository.
func sameDirectory(left, right string) bool {
	if left == right {
		return true
	}
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return false
	}
	return os.SameFile(leftInfo, rightInfo)
}

// probeWorktreeRoot returns the absolute worktree root git reports for
// startDir, or the empty string when git declines to name one.
//
// Declining is an expected answer here, not a failure: `fatal: this operation
// must be run in a work tree` is exactly what a Git directory with no
// core.worktree says, and it means "no worktree is reachable from here" rather
// than "something is wrong". Only a condition of the subprocess itself — a
// timeout, a missing binary, a cancelled context — is worth propagating, and
// that is what propagate already decides.
func (a *Adapter) probeWorktreeRoot(ctx context.Context, startDir string) (string, error) {
	return a.probeOptionalPath(ctx, startDir, "--show-toplevel")
}

// probeCommonDir returns the absolute common Git directory for startDir, or the
// empty string when git declines to name one. It is best-effort for the same
// reason as probeWorktreeRoot: it only sharpens an error message that is
// already being returned.
func (a *Adapter) probeCommonDir(ctx context.Context, startDir string) (string, error) {
	return a.probeOptionalPath(ctx, startDir, "--git-common-dir")
}

// probeGitDir returns the absolute Git directory for startDir, or the empty
// string when git declines to name one. It is asked for only when a remedy has
// to name the Git directory a worktree should point back at, which is the one
// place where the per-worktree directory — not the shared common one — is the
// right answer.
func (a *Adapter) probeGitDir(ctx context.Context, startDir string) (string, error) {
	return a.probeOptionalPath(ctx, startDir, "--git-dir")
}

func (a *Adapter) probeOptionalPath(ctx context.Context, startDir, flag string) (string, error) {
	out, _, err := a.revParse(ctx, startDir, "--path-format=absolute", flag)
	if err != nil {
		if propagated := propagate(err); propagated != nil {
			return "", propagated
		}
		return "", nil
	}
	if out == "" {
		return "", nil
	}
	return filepath.Clean(out), nil
}

// absolutePath asks rev-parse for one path in absolute form. --path-format is
// passed explicitly so the answer does not depend on which directory the caller
// happened to start from.
func (a *Adapter) absolutePath(ctx context.Context, startDir, flag string) (string, error) {
	out, stderr, err := a.revParse(ctx, startDir, "--path-format=absolute", flag)
	if err != nil {
		return "", wrapUnexpected(err, stderr)
	}
	if out == "" {
		return "", wrapUnexpected(fmt.Errorf("rev-parse %s produced no output", flag), stderr)
	}
	return filepath.Clean(out), nil
}

// revParse returns rev-parse's stdout with its output terminator removed,
// together with its stderr. The second value is not decoration: when git fails,
// its stderr is the only text that distinguishes an absent repository from a
// damaged one, and the exit status alone cannot tell them apart (finding F1).
func (a *Adapter) revParse(ctx context.Context, startDir string, args ...string) (string, string, error) {
	stdout, stderr, err := a.runner.Run(ctx, startDir, append([]string{"rev-parse"}, args...)...)
	trimmedErr := stderrSummary(string(stderr))
	if err != nil {
		return "", trimmedErr, err
	}
	return trimOutputTerminator(string(stdout)), trimmedErr, nil
}

// trimOutputTerminator removes the single newline rev-parse writes after its
// answer, and nothing else.
//
// This used to be strings.TrimSpace, which is wrong for every rev-parse that
// answers with a *path*. A trailing space, tab or carriage return is a legal
// byte in a POSIX path name, so trimming whitespace silently shortens the
// answer to a path that is not the repository — and because the shortened path
// is still plausible, nothing downstream notices: `init` creates `.mindrail/`
// in a directory it invents next to the worktree, reports READY FOR TARGETED
// WORK and exits 0 (finding H5).
//
// The newline is the only byte git adds, so it is the only byte that can be
// removed without guessing. rev-parse emits path names raw: measured against
// git 2.55, neither core.quotePath nor a non-UTF-8 byte nor an embedded quote,
// backslash or newline makes it quote or escape the output, so there is nothing
// to unescape here and a path containing a newline survives intact — only the
// terminator goes.
//
// Boolean answers keep going through parseGitBool, which trims whitespace on
// purpose: "true" is not a path.
func trimOutputTerminator(out string) string {
	return strings.TrimSuffix(out, "\n")
}

// classifyProbeFailure decides what a failed discovery probe actually means.
//
// Exit 128 is git's generic fatal, not a "no repository here" signal: a corrupt
// object store, an unparseable config, a repository format this git is too old
// for and a refused ownership check all produce it from inside a real
// repository. Only git's own message separates them, which is why LC_ALL=C is
// forced on every invocation — the text this reads is pinned to one language.
//
// Anything git did not call "not a git repository" keeps the repository's
// existence an open question, so it must not be handed the `git init` remedy.
func (a *Adapter) classifyProbeFailure(startDir, stderr string, cause error) error {
	if !gitReportedNoRepository(stderr) {
		return repositoryUnreadableError(startDir, stderr, cause)
	}
	// "There is no repository here" and "the pointer to the repository is
	// broken" are the same message from git and completely different situations
	// for the reader: the second one is a checkout that used to work, and
	// `git init` inside it would bury the problem under a new empty repository
	// rather than fix it (findings H7 and H8).
	if fault := brokenGitFileFault(startDir, stderr); fault != nil {
		return fault.asError(startDir, stderr, cause)
	}
	return notARepositoryError(startDir, stderr, cause)
}

// gitReportedNoRepository reports whether stderr is git's own verdict that
// there is no repository at or above the start directory.
//
// The match is deliberately narrow. Widening it re-creates the defect it exists
// to fix — every fatal collapsing into "not a Git repository" — whereas a
// message this does not recognise degrades to the honest "git refused and here
// is what it said", which is never actively wrong.
func gitReportedNoRepository(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "not a git repository")
}

// stderrSummary trims git's stderr to the part worth carrying. git prefixes its
// fatal with "fatal: " and may add follow-up advice lines; all of it is kept,
// with the surrounding whitespace removed so it composes into an error string.
func stderrSummary(stderr string) string {
	return strings.TrimSpace(stderr)
}

// joinStderr appends git's own words to an error, or returns the error
// unchanged when git said nothing.
func joinStderr(err error, stderr string) error {
	if stderr == "" {
		return err
	}
	if err == nil {
		return errors.New(stderr)
	}
	return errors.Join(err, errors.New(stderr))
}

// parseGitBool reads rev-parse's boolean output. Anything that is not an
// explicit "true" is false, so a future git that grows a third answer degrades
// to the conservative branch instead of being misread.
func parseGitBool(out string) bool {
	return strings.EqualFold(strings.TrimSpace(out), "true")
}

// propagate returns err unchanged when it already describes a condition of the
// git subprocess itself — a timeout, a missing binary, a cancelled context —
// and nil when the caller should classify it further.
func propagate(err error) error {
	switch {
	case errors.Is(err, ErrGitTimeout),
		errors.Is(err, ErrGitUnavailable),
		// A start directory that could not be entered is already classified,
		// and already names the directory; re-wrapping it as "not a repository"
		// would throw that away and blame the wrong thing (finding F3).
		errors.Is(err, ErrWorkingDirectory),
		errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return nil
	}
}

// wrapUnexpected classifies a failure from a rev-parse that should have
// succeeded. Discovery already confirmed a worktree, so a later refusal means
// git could not answer rather than that the user is somewhere wrong.
func wrapUnexpected(err error, stderr string) error {
	if propagated := propagate(err); propagated != nil {
		return propagated
	}
	wrapped := app.NewError(
		app.CodeGitUnavailable,
		app.KindUnavailable,
		"git could not report the repository layout",
		"Mindrail cannot locate the repository's runtime state without it",
		"run `git status` in the same directory to see what git reports",
		"check that the repository is not mid-operation or partially removed",
	).WithMetadata("detail", err.Error())

	if stderr != "" {
		wrapped = wrapped.WithMetadata("git_stderr", stderr)
	}
	return wrapped.WithCause(errors.Join(ErrGitUnavailable, joinStderr(err, stderr)))
}

// repositoryUnreadableError reports a git that ran, failed, and did not say the
// repository is absent.
//
// It carries GIT_UNAVAILABLE and the unavailable exit class, matching
// wrapUnexpected: both mean "git could not answer", which is the same class of
// condition whether it happens on the first probe or a later one. What it must
// not carry is NOT_A_GIT_REPOSITORY's `git init` remedy, which inside a
// repository with a damaged config would create nothing and fix nothing.
//
// git's own message is the payload. It travels in three places on purpose: in
// `why`, because it is the only sentence that says what happened; in
// `git_stderr` metadata, so a machine consumer can read it without parsing
// prose; and in the cause chain, so a log line that prints only the cause still
// shows it.
func repositoryUnreadableError(startDir, stderr string, cause error) error {
	why := "git refused to say whether this directory is inside a repository"
	if stderr != "" {
		why = "git refused to say whether this directory is inside a repository: " + firstLine(stderr)
	}

	err := app.NewError(
		app.CodeGitUnavailable,
		app.KindUnavailable,
		why,
		"Mindrail scopes all of its state to a repository and cannot tell which one, if any, applies here",
		"run `git rev-parse --is-inside-work-tree` in the same directory to see git's own message in full",
		"check the repository's .git directory, its config and its permissions, and that this git is new enough to read it",
	).WithMetadata("start_dir", startDir).
		WithMetadata("detail", cause.Error())

	if stderr != "" {
		err = err.WithMetadata("git_stderr", stderr)
	}
	return err.WithCause(errors.Join(ErrGitUnavailable, ErrRepositoryUnreadable, joinStderr(cause, stderr)))
}

// firstLine keeps a one-line summary out of git's possibly multi-line advice,
// so `why` stays a sentence while the metadata keeps everything.
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

// notARepositoryError reports a start directory outside any repository. It is a
// deterministic, user-correctable placement, so it takes the usage exit class
// of decision D-03.
//
// The start directory travels as metadata because "not a repository" is
// unanswerable without knowing where the question was asked: the same message
// is correct advice from a home directory and baffling from what the user
// believes is their checkout. Any renderer of this error is expected to show
// it.
// git's own words travel in `git_stderr` metadata as well as in the cause
// chain. Every sibling classification in this package already does that, and a
// machine consumer that has to regex a joined cause string to recover them is
// reading a field nobody promised the shape of (finding H7).
func notARepositoryError(startDir, stderr string, cause error) error {
	err := app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		"the directory is not inside a Git repository worktree",
		"Mindrail scopes all of its state to a repository and has nothing to attach to here",
		"change into a Git repository and run the command again",
		"or run `git init` to create one first",
	).WithMetadata("start_dir", startDir)

	if stderr != "" {
		err = err.WithMetadata("git_stderr", stderr)
	}
	return err.WithCause(errors.Join(ErrNotARepository, joinStderr(cause, stderr)))
}

// gitDirectoryImpact is the consequence shared by every start directory inside
// a Git directory, whichever shape of Git directory it turns out to be.
const gitDirectoryImpact = "Mindrail tracks work against files in a worktree, and a Git directory holds none; creating a repository here would nest a second one inside this repository's administrative directory"

// outsideWorktreeImpact is the consequence for the other standpoint: the reader
// is in an ordinary directory — often the checkout root itself — of a
// repository whose config points its working tree somewhere else. Nothing is
// about to be nested inside an administrative directory here, and saying so
// describes a situation the reader is not in (finding E7).
const outsideWorktreeImpact = "Mindrail tracks work against the files of a repository's working tree, and this repository's working tree is configured to be somewhere other than this directory"

// detachedWorktreeError reports a start directory inside a Git directory whose
// worktree git can name and which was checked to be reachable — a submodule's
// Git directory, or any repository with core.worktree set.
//
// The whole point of it is that the remedy names an absolute path. The message
// it replaces said "the directory that contains this .git", which in these
// repositories is the directory the reader is already standing in or its
// unrelated parent, so following the remedy changed nothing and the reader
// looped (finding D6).
func detachedWorktreeError(startDir, worktree string, standing standpoint) error {
	why := fmt.Sprintf("this repository's working tree is configured to be %q, and this directory is not inside it", worktree)
	if standing.insideGitDir {
		why = fmt.Sprintf("the directory is inside a Git directory, not a working tree; this repository's working tree is %q", worktree)
	}

	return app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		why,
		standing.impact(),
		fmt.Sprintf("change into %q and run the command again", worktree),
		"or run the command from a linked worktree of this repository",
	).WithMetadata("start_dir", startDir).
		WithMetadata("worktree_root", worktree).
		WithCause(standing.cause())
}

// unenterableWorktreeError reports a core.worktree that names a directory
// nobody can enter — deleted, replaced by a file, or stripped of search
// permission.
//
// "Change into <path>" is not a remedy when <path> refuses; it is the same loop
// with a longer stride. What is actionable is the path itself and why it
// refused, so both are in the sentence.
func unenterableWorktreeError(startDir, worktree string, fault error, standing standpoint) error {
	why := fmt.Sprintf("this repository's configured working tree %q could not be entered: %v", worktree, fault)
	if standing.insideGitDir {
		why = fmt.Sprintf("the directory is inside a Git directory whose configured working tree %q could not be entered: %v", worktree, fault)
	}

	return app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		why,
		standing.impact(),
		fmt.Sprintf("restore %q, or make it a directory you can enter, and run the command from it", worktree),
		fmt.Sprintf("or point this repository at a working tree that exists by running `git config core.worktree <path>` from %q", startDir),
	).WithMetadata("start_dir", startDir).
		WithMetadata("worktree_root", worktree).
		WithMetadata("detail", fault.Error()).
		WithCause(errors.Join(standing.cause(), fault))
}

// unlinkedWorktreeError reports a core.worktree that names a real directory
// which holds no repository at all and does not point back at the Git
// directory.
//
// The link is one-way: core.worktree lets the Git directory find the worktree,
// and only a `.git` entry in the worktree lets discovery go the other way. With
// that entry missing, sending the reader to the worktree earns them
// NOT_A_GIT_REPOSITORY — a second wrong remedy rather than the first one fixed
// — so the remedy repairs the link instead. `git init --separate-git-dir` is
// the git-native way to write it: on an existing repository it reinitialises
// rather than replaces, keeping every object and ref.
//
// That command is only correct because this branch has established that git
// finds *no* repository in the configured directory. Where it finds one, the
// same command either refuses or nests a repository inside somebody else's
// checkout, which is why that case left this function entirely (finding E6) and
// is now foreignWorktreeError.
func unlinkedWorktreeError(startDir, gitDir, worktree string, standing standpoint) error {
	actions := []string{
		fmt.Sprintf("add a .git entry in %q naming this Git directory, then run the command from %q", worktree, worktree),
		"or run the command from a working tree that already points at this repository",
	}
	if gitDir != "" {
		actions = []string{
			fmt.Sprintf("re-link them with `git init --separate-git-dir=%s %s`, then run the command from %q", gitDir, worktree, worktree),
			fmt.Sprintf("or run the command from a working tree whose .git already reads `gitdir: %s`", gitDir),
		}
	}

	why := fmt.Sprintf("this repository's configured working tree %q does not point back at it, so git finds no repository there", worktree)
	if standing.insideGitDir {
		why = fmt.Sprintf("the directory is inside a Git directory whose configured working tree %q does not point back at it, so git finds no repository there", worktree)
	}

	err := app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		why,
		standing.impact(),
		actions...,
	).WithMetadata("start_dir", startDir).
		WithMetadata("worktree_root", worktree)

	if gitDir != "" {
		err = err.WithMetadata("git_dir", gitDir)
	}
	return err.WithCause(standing.cause())
}

// foreignWorktreeError reports a core.worktree that names a directory belonging
// to a different repository.
//
// This is the shape both of unlinkedWorktreeError's remedies dead-ended on
// (finding E6). Measured against git 2.55:
//
//   - When the configured directory is another repository's own root,
//     `git init --separate-git-dir=<gitdir> <path>` exits 128 with
//     "unable to move <path>/.git to <gitdir>: Directory not empty". It is
//     trying to relocate the *other* repository, which is not what the sentence
//     around it claimed to be doing.
//   - When the configured directory merely sits inside another repository's
//     working tree, the same command exits 0 — and what it has done is carve a
//     nested repository out of that checkout, which nobody asked for.
//   - And the alternative remedy, "run the command from a working tree whose
//     .git already reads `gitdir: <gitdir>`", names a working tree that does not
//     exist: if one did, discovery would have found it.
//
// The reason printed alongside them was false too. "git finds no repository
// there" is exactly wrong about a directory that is one.
//
// What is left is to say what is true and to offer the two things that are
// actually available: give this repository a working tree of its own, or go to
// the other repository if that is the one that was meant. The first was carried
// out end to end — init, config, checkout — before it was written down.
func foreignWorktreeError(startDir, gitDir, worktree string, link worktreeLinkage, standing standpoint) error {
	why := fmt.Sprintf("this repository's configured working tree %q belongs to a different repository, the one whose Git directory is %q", worktree, link.commonDir)
	if standing.insideGitDir {
		why = fmt.Sprintf("the directory is inside a Git directory, not a working tree, and the working tree it is configured to use — %q — belongs to a different repository, the one whose Git directory is %q", worktree, link.commonDir)
	}

	actions := []string{"give this repository a working tree of its own and point `core.worktree` at it"}
	if gitDir != "" {
		actions = []string{fmt.Sprintf(
			"give this repository a working tree of its own: pick an unused path <path>, then run `git init --separate-git-dir=%s <path>`, `git --git-dir=%s config core.worktree <path>` and `git -C <path> checkout --force HEAD`, and run the command from <path>",
			gitDir, gitDir)}
	}
	if link.toplevel != "" && directoryEntryFault(link.toplevel) == nil {
		actions = append(actions, fmt.Sprintf("or change into %q and run the command there, if that other repository is the one you meant", link.toplevel))
	}
	if caveat := foreignRelinkCaveat(gitDir, worktree, link); caveat != "" {
		actions = append(actions, caveat)
	}

	err := app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		why,
		standing.impact(),
		actions...,
	).WithMetadata("start_dir", startDir).
		WithMetadata("worktree_root", worktree).
		WithMetadata("other_common_dir", link.commonDir)

	if gitDir != "" {
		err = err.WithMetadata("git_dir", gitDir)
	}
	if link.toplevel != "" {
		err = err.WithMetadata("other_worktree_root", link.toplevel)
	}
	return err.WithCause(errors.Join(standing.cause(), ErrForeignWorktree))
}

// foreignRelinkCaveat says why the command the reader would reach for next does
// not apply, in the words of what it actually does. Both branches were measured
// rather than reasoned about; the sentence is omitted when neither was.
func foreignRelinkCaveat(gitDir, worktree string, link worktreeLinkage) string {
	if gitDir == "" || link.toplevel == "" {
		return ""
	}
	if sameDirectory(link.toplevel, worktree) {
		return fmt.Sprintf("`git init --separate-git-dir=%s %s` cannot re-link them: git refuses, because it would have to move that other repository's Git directory onto this one", gitDir, worktree)
	}
	return fmt.Sprintf("`git init --separate-git-dir=%s %s` would appear to succeed, but it carves a nested repository out of the working tree of the repository at %q", gitDir, worktree, link.toplevel)
}

// insideGitDirectoryError reports a start directory inside a Git directory that
// git cannot match to a worktree.
//
// It keeps the NOT_A_GIT_REPOSITORY code — the code is the stable machine
// contract and is not reworded (decision D-18) — but not the remedy: advising
// `git init` here would be followed literally and would create a second
// repository nested inside the first one's administrative directory.
//
// Two different situations arrive here and they do not share a remedy:
//
//   - An ordinary `.git` directory (or a linked worktree's administrative
//     directory under it). The worktree is the directory holding that `.git`,
//     and worktreeHoldingGitDir confirms it rather than assuming it, so the
//     remedy can name an absolute path.
//   - A Git directory created by `git init --separate-git-dir` or
//     `git clone --separate-git-dir`. Measured against git 2.55 neither records
//     core.worktree, and there is no back-pointer anywhere in the Git directory,
//     so *nothing* — not git, not Mindrail — can name the worktree from here.
//     Inventing one is what the previous message did. Saying so, and saying what
//     the worktree's `.git` file has to contain, is the honest remedy.
func insideGitDirectoryError(startDir, commonDir string) error {
	var (
		why      string
		actions  []string
		worktree = worktreeHoldingGitDir(commonDir)
	)
	switch {
	case worktree != "":
		why = fmt.Sprintf("the directory is inside the repository's .git directory, which is not a working tree; the working tree is %q", worktree)
		actions = []string{
			fmt.Sprintf("change into %q and run the command again", worktree),
			"or run the command from a linked worktree of this repository",
		}
	case commonDir != "":
		why = fmt.Sprintf("the directory is inside the Git directory %q, which is not a working tree and which records no working tree of its own", commonDir)
		actions = []string{
			fmt.Sprintf("run the command from the working tree this Git directory serves — the one whose .git file reads `gitdir: %s`", commonDir),
			fmt.Sprintf("or record it here by running `git config core.worktree <path-to-working-tree>` from %q, then run the command from that working tree", startDir),
		}
	default:
		why = "the directory is inside a Git directory, which is not a working tree"
		actions = []string{
			"run `git rev-parse --path-format=absolute --git-common-dir` here to see which repository this Git directory belongs to",
			"then run the command from that repository's working tree",
		}
	}

	err := app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		why,
		gitDirectoryImpact,
		actions...,
	).WithMetadata("start_dir", startDir)

	if commonDir != "" {
		err = err.WithMetadata("git_common_dir", commonDir)
	}
	if worktree != "" {
		err = err.WithMetadata("worktree_root", worktree)
	}
	return err.WithCause(errors.Join(ErrNotARepository, ErrInsideGitDirectory))
}

// linkedWorktreeAdminError reports a start directory inside the administrative
// directory a repository keeps for one linked worktree.
//
// git answers --show-toplevel with "this operation must be run in a work tree"
// from there and --git-common-dir with the *main* repository's `.git`, so the
// generic Git-directory classification described the main worktree: true of the
// repository, and not a description of where the reader is standing or of the
// checkout they were almost certainly working on. The linked worktree's own
// root is recorded right there in the administrative directory, so naming it
// costs one file read and no guessing (finding E9).
func linkedWorktreeAdminError(startDir, commonDir, worktree string) error {
	err := app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		fmt.Sprintf("the directory is inside the administrative directory this repository keeps for the linked working tree %q, which is not a working tree itself", worktree),
		gitDirectoryImpact,
		fmt.Sprintf("change into %q and run the command again", worktree),
		"or run the command from another working tree of this repository",
	).WithMetadata("start_dir", startDir).
		WithMetadata("worktree_root", worktree)

	if commonDir != "" {
		err = err.WithMetadata("git_common_dir", commonDir)
	}
	return err.WithCause(errors.Join(ErrNotARepository, ErrInsideGitDirectory, ErrLinkedWorktreeAdminDir))
}

// linkedWorktreeOfAdminDir returns the working tree root whose administrative
// directory startDir is in, or "" when startDir is not in one.
//
// The administrative directory is found by walking up from startDir to the
// entry whose parent is `<common-dir>/worktrees`, which is git's documented
// layout. It is derived from the two paths already in hand rather than from a
// fresh `--git-dir` probe on purpose: this branch is reached by every start
// directory inside an ordinary `.git`, and none of them should pay a subprocess
// for a question that a stat answers.
//
// Two facts are then required to agree, because the value is about to be
// printed as somewhere to go: the recorded root has to be a directory the
// reader can enter, and its own `.git` has to point back at the administrative
// directory. A pruned or moved worktree fails one of them and falls through to
// the generic classification, which is never actively wrong.
func linkedWorktreeOfAdminDir(startDir, commonDir string) string {
	if startDir == "" || commonDir == "" {
		return ""
	}
	container := filepath.Join(commonDir, linkedWorktreeDirName)

	adminDir := ""
	for current := filepath.Clean(startDir); ; {
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		if sameDirectory(parent, container) {
			adminDir = current
			break
		}
		current = parent
	}

	// git records the linked worktree's `.git` file here, as an absolute path.
	// Trimmed with git's own set, not the narrower "\n " this used to use:
	// finding F17 is that the two differ for a carriage return, and a second
	// copy of the wrong trim one package file over is the same defect waiting
	// for the shape that reaches it.
	//
	// No test can fail if this regresses, and that is stated rather than
	// implied: the value is only ever handed to filepath.Dir, which discards the
	// last segment and with it any trailing byte, so the two spellings cannot
	// produce different behaviour today. It is corrected for consistency with
	// readGitFile, and the moment anything uses the value whole it will matter.
	recorded, err := readSmallFile(filepath.Join(adminDir, "gitdir"), maxGitFileSize)
	if err != nil {
		return ""
	}
	target := strings.TrimRight(string(recorded), gitTrailingSpace)
	if target == "" || !filepath.IsAbs(target) {
		return ""
	}

	root := filepath.Dir(filepath.Clean(target))
	if directoryEntryFault(root) != nil {
		return ""
	}
	back, ok := readGitFile(filepath.Join(root, ".git"), root)
	if !ok || !sameDirectory(back, adminDir) {
		return ""
	}
	return root
}

// worktreeHoldingGitDir returns the directory that holds commonDir as its
// `.git`, or the empty string when no directory does.
//
// The name test is sufficient here, and saying so is the correction finding F16
// asks for. This function used to follow it with an os.SameFile comparison
// documented as the real check — "the question is not 'is it called .git' but
// 'would git find this same Git directory from the parent'" — and that
// comparison could not fail: once the base name is `.git`,
// filepath.Join(filepath.Dir(commonDir), ".git") rebuilds commonDir byte for
// byte, and both stats were of the same path. It shipped exactly the name-only
// test its own comment said was insufficient.
//
// The shape it claimed to reject is `git init --separate-git-dir=/srv/x/.git
// /realwt`, which makes a Git directory named `.git` whose parent is not the
// working tree it was created for. Rejecting it would be wrong: git discovering
// /srv/x finds that same Git directory and calls /srv/x its worktree, so
// agreeing is agreeing with the oracle. TestWorktreeHoldingGitDirAcceptsA-
// SeparateGitDirectoryNamedGit pins that, which is what the deleted guard never
// did.
func worktreeHoldingGitDir(commonDir string) string {
	if commonDir == "" || filepath.Base(commonDir) != ".git" {
		return ""
	}
	parent := filepath.Dir(commonDir)
	if parent == commonDir {
		return ""
	}

	// A worktree the reader cannot enter is not somewhere they can be told to
	// go; the caller's generic remedy is less wrong than a path that refuses.
	if directoryEntryFault(parent) != nil {
		return ""
	}
	return parent
}

// bareRepositoryError reports a bare repository. Acceptance criterion 3 asks
// for the active worktree to be reported, and a bare repository has none, so it
// is refused rather than half-supported (decision D-31).
func bareRepositoryError(startDir string) error {
	return app.NewError(
		app.CodeBareRepository,
		app.KindUsage,
		"the repository is bare and has no working tree",
		"Mindrail tracks work against files in a worktree, which a bare repository does not have",
		"run the command from a clone or from a linked worktree of this repository",
		"or add one with `git worktree add <path>`",
	).WithMetadata("start_dir", startDir).WithCause(ErrBareRepository)
}
