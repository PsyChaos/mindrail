package git

import (
	"context"
	"errors"
	"fmt"
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
// is inside the .git directory of an ordinary one. Both report "false" for
// --is-inside-work-tree, and only the first has a name a user can act on.
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
	// Reaching here means git found a repository and said the directory is not
	// in its worktree, and that the repository is not bare: the only remaining
	// shape is a directory inside .git. That is a different place to be from
	// "outside every repository", so it gets its own remedy.
	return insideGitDirectoryError(startDir)
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

// revParse returns rev-parse's trimmed stdout together with its stderr. The
// second value is not decoration: when git fails, its stderr is the only text
// that distinguishes an absent repository from a damaged one, and the exit
// status alone cannot tell them apart (finding F1).
func (a *Adapter) revParse(ctx context.Context, startDir string, args ...string) (string, string, error) {
	stdout, stderr, err := a.runner.Run(ctx, startDir, append([]string{"rev-parse"}, args...)...)
	trimmedErr := stderrSummary(string(stderr))
	if err != nil {
		return "", trimmedErr, err
	}
	return strings.TrimSpace(string(stdout)), trimmedErr, nil
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
	if gitReportedNoRepository(stderr) {
		return notARepositoryError(startDir, joinStderr(cause, stderr))
	}
	return repositoryUnreadableError(startDir, stderr, cause)
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
func notARepositoryError(startDir string, cause error) error {
	return app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		"the directory is not inside a Git repository worktree",
		"Mindrail scopes all of its state to a repository and has nothing to attach to here",
		"change into a Git repository and run the command again",
		"or run `git init` to create one first",
	).WithMetadata("start_dir", startDir).WithCause(errors.Join(ErrNotARepository, cause))
}

// insideGitDirectoryError reports a start directory inside a repository's .git
// directory.
//
// It keeps the NOT_A_GIT_REPOSITORY code — the code is the stable machine
// contract and is not reworded (decision D-18) — but not the remedy: advising
// `git init` here would be followed literally and would create a second
// repository nested inside the first one's administrative directory. The
// worktree the user wants is one level out, and that is what it says.
func insideGitDirectoryError(startDir string) error {
	return app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		"the directory is inside the repository's .git directory, which is not a working tree",
		"Mindrail tracks work against files in a worktree, and the .git directory holds none; creating a repository here would nest a second one inside this repository's administrative directory",
		"change into the repository's working tree (the directory that contains this .git) and run the command again",
		"or run the command from a linked worktree of this repository",
	).WithMetadata("start_dir", startDir).
		WithCause(errors.Join(ErrNotARepository, ErrInsideGitDirectory))
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
