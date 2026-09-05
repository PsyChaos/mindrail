// Package git is Mindrail's only door to version control. It drives the system
// git binary through os/exec with an explicit argument vector, never a shell
// (tech-stack §34/§35), and hands every subprocess a deliberately constructed
// environment.
//
// Nothing above this package is allowed to build a git command line of its own,
// because the isolation rules below are only worth anything if there is exactly
// one place that can forget them.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// DefaultTimeout bounds a single git invocation. A stale index.lock or a
// network filesystem can otherwise hang discovery forever, which would blow the
// warm-path budget with no defined failure mode (decision D-30).
const DefaultTimeout = 10 * time.Second

// waitDelay bounds how long Wait lingers after the process is killed, in case a
// grandchild still holds the output pipe. Without it a cancelled call can
// outlive its cancellation.
const waitDelay = 250 * time.Millisecond

// CommandRunner is the seam every git call passes through. Tests substitute
// FakeRunner for it; nothing else about the adapter changes.
type CommandRunner interface {
	Run(ctx context.Context, dir string, args ...string) (stdout, stderr []byte, err error)
}

// ExecRunner runs the real git binary. Its zero value is usable and equivalent
// to NewExecRunner().
type ExecRunner struct {
	// Bin is the git executable; the empty string means "git" on PATH.
	Bin string
	// Timeout bounds one invocation; zero means DefaultTimeout.
	Timeout time.Duration
}

// NewExecRunner returns a runner for the git binary on PATH.
func NewExecRunner() *ExecRunner { return &ExecRunner{} }

// Run executes one git invocation and returns its captured output.
//
// The argument vector is passed through untouched: no quoting, no splitting and
// no interpolation happen anywhere on this path, so a repository or branch
// named `; rm -rf /` is just a string.
func (r *ExecRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()

	cmd := r.command(runCtx, dir, args)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), nil
	}

	// The caller giving up and our own deadline expiring are different events
	// with different remedies, so they must not collapse into one error.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("git %s: %w", strings.Join(args, " "), ctxErr)
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return stdout.Bytes(), stderr.Bytes(), timeoutError(args, r.timeout())
	}
	// A working directory that cannot be entered fails before git is ever
	// executed, and its failure looks exactly like a missing binary — both
	// unwrap to os.ErrNotExist. Asking about the directory first is what keeps a
	// mistyped -C from being reported as a broken git installation.
	if dirErr := workingDirectoryFault(dir, err); dirErr != nil {
		return stdout.Bytes(), stderr.Bytes(), dirErr
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
		return stdout.Bytes(), stderr.Bytes(), unavailableError(r.binary(), stderrSummary(stderr.String()), err)
	}

	return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
}

// workingDirectoryFault reports whether a failed invocation failed because of
// Cmd.Dir rather than because of git. It returns nil for a usable directory, so
// every other failure keeps its existing classification.
//
// The *fs.PathError with Op == "chdir" is os/exec's own pre-flight verdict and
// carries the exact cause, so it is consulted first — but it is only ever
// raised for a directory that fails os.Stat. Every other refusal reaches us as
// a fork/exec error naming the *binary*, so the directory has to be probed
// directly (finding H6).
func workingDirectoryFault(dir string, runErr error) error {
	if dir == "" {
		return nil
	}

	// git having run and exited settles the question: the directory was
	// entered. Probing it again could only misfire on a concurrent chmod and
	// turn an ordinary git fatal into a directory complaint.
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return nil
	}

	var pathErr *fs.PathError
	if errors.As(runErr, &pathErr) && pathErr.Op == "chdir" {
		return workingDirectoryError(dir, pathErr.Err)
	}

	if fault := directoryEntryFault(dir); fault != nil {
		return workingDirectoryError(dir, fault)
	}
	return nil
}

// directoryEntryFault reports why dir cannot be used as a working directory, or
// nil when it can.
//
// The enterability probe is a stat of dir's "." entry rather than of dir
// itself. os.Stat(dir) answers "does something exist there", which is true of a
// directory at mode 0000 that no process can enter — and that gap is the whole
// of finding H6, because os/exec's own pre-flight check is the same stat: the
// 0000 directory passes it, the refusal happens in the child after the fork,
// and it surfaces as an *fs.PathError whose Op is "fork/exec" and whose Path is
// the git binary. Read literally that error says git could not be executed, so
// the reader was told to install git and never told which directory was at
// fault.
//
// Resolving the "." component requires search permission on dir, which is
// exactly the permission chdir needs, so the probe answers the question that
// was actually asked.
//
// It has to be "." specifically. Opening dir needs *read* permission, which
// chdir does not: an execute-only directory (mode 0111) that git enters happily
// would be condemned by an os.Open probe, and a read-only one (mode 0444) that
// git cannot enter would be waved through by it. Both were measured.
func directoryEntryFault(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return underlyingError(err)
	}
	if !info.IsDir() {
		return errors.New("not a directory")
	}
	if _, err := os.Stat(dir + string(os.PathSeparator) + "."); err != nil {
		return underlyingError(err)
	}
	return nil
}

// underlyingError strips the path-and-syscall framing off a filesystem error so
// the remaining sentence composes into a message that already names the path.
func underlyingError(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && pathErr.Err != nil {
		return pathErr.Err
	}
	return err
}

// command builds the subprocess. It is separate from Run so the isolation rules
// — argv, environment, working directory, no stdin — can be asserted directly
// instead of inferred from a process's behaviour.
func (r *ExecRunner) command(ctx context.Context, dir string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.binary(), args...)
	cmd.Dir = dir
	cmd.Env = SanitizedEnv(os.Environ())
	// git must never be able to ask for a password on a stalled connection.
	cmd.Stdin = nil
	cmd.WaitDelay = waitDelay
	return cmd
}

func (r *ExecRunner) binary() string {
	if r.Bin == "" {
		return "git"
	}
	return r.Bin
}

func (r *ExecRunner) timeout() time.Duration {
	if r.Timeout <= 0 {
		return DefaultTimeout
	}
	return r.Timeout
}

// inheritedGitVars are the variables that let the ambient environment decide
// which repository git answers about. MR-017 will call this adapter from a
// pre-commit hook, where several of them are already set; inheriting any one of
// them would silently point discovery at whatever git was doing rather than at
// the worktree Mindrail was asked about (decision D-29, spec §113).
//
// The list is "anything that can move the answer", not "anything git sets":
//
//   - GIT_DIR, GIT_WORK_TREE, GIT_COMMON_DIR name the repository outright.
//   - GIT_INDEX_FILE, GIT_OBJECT_DIRECTORY, GIT_ALTERNATE_OBJECT_DIRECTORIES,
//     GIT_NAMESPACE move the state git reads once it has found one.
//   - GIT_CEILING_DIRECTORIES halts the upward walk, so an inherited value
//     makes a perfectly valid subdirectory report "not a repository".
//   - GIT_DISCOVERY_ACROSS_FILESYSTEM lets the same walk cross a mount point,
//     so it can reach a repository the caller's own git would never have found.
//
// The rest of the GIT_* surface stays. GIT_PREFIX moves nothing — git
// recomputes the prefix — and GIT_CONFIG_GLOBAL, GIT_CONFIG_SYSTEM and
// GIT_CONFIG_NOSYSTEM select configuration *files*, not repositories: a git
// that cannot find its own configuration is not a safer git, and a test harness
// legitimately uses those three to detach from the developer's own config.
var inheritedGitVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_COMMON_DIR",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_NAMESPACE",
	"GIT_CEILING_DIRECTORIES",
	"GIT_DISCOVERY_ACROSS_FILESYSTEM",
	// GIT_CONFIG_COUNT is the header of git's command-line-config injection
	// channel: it declares how many GIT_CONFIG_KEY_n/GIT_CONFIG_VALUE_n pairs
	// git must splice in ahead of every configuration file it reads.
	//
	// D-29's literal list does not name it, and it was previously left in on the
	// grounds that an injected core.worktree does not move rev-parse's answer.
	// That measurement still holds on git 2.55 — but it is the wrong test. D-29
	// exists so that the ambient environment cannot decide what git answers, and
	// this channel does decide it, by denial:
	//
	//	GIT_CONFIG_COUNT=notanumber  →  error: bogus count in GIT_CONFIG_COUNT
	//	                                fatal: unable to parse command-line config
	//	GIT_CONFIG_COUNT=1 (no KEY_0) →  error: missing config key GIT_CONFIG_KEY_0
	//	                                fatal: unable to parse command-line config
	//
	// Both are exit 128 on *every* invocation, so a perfectly valid repository
	// answers nothing at all — the same shape of failure GIT_CEILING_DIRECTORIES
	// is already on this list to prevent, and reachable by accident rather than
	// by attack. A stale count is exactly what MR-017's pre-commit hook will
	// inherit from whatever tooling ran before it.
	//
	// The narrow "does this variable move discovery on today's git" test is also
	// not stable: which keys reach discovery is a property of a git version this
	// package does not pin.
	"GIT_CONFIG_COUNT",
}

// inheritedGitVarPrefixes covers the indexed half of the same injection
// channel. The names are GIT_CONFIG_KEY_0, GIT_CONFIG_VALUE_0, … with no fixed
// upper bound, so they cannot be enumerated the way inheritedGitVars is.
//
// Dropping GIT_CONFIG_COUNT alone would already disarm them — git reads no pair
// it was not told to count — but leaving the pairs behind would let a child
// process of ours reassemble the channel from a count of its own.
//
// The prefixes end in an underscore on purpose: GIT_CONFIG_GLOBAL,
// GIT_CONFIG_SYSTEM and GIT_CONFIG_NOSYSTEM must survive, and a looser
// "GIT_CONFIG" prefix would take all three with it.
var inheritedGitVarPrefixes = []string{
	"GIT_CONFIG_KEY_",
	"GIT_CONFIG_VALUE_",
}

// forcedGitVars make every invocation quiet, non-interactive and parseable.
// GIT_OPTIONAL_LOCKS=0 keeps a read-only probe from taking the index lock,
// GIT_TERMINAL_PROMPT=0 turns a credential prompt into an error rather than a
// hang, and LC_ALL=C pins the message language the parser sees.
var forcedGitVars = []string{
	"GIT_OPTIONAL_LOCKS=0",
	"GIT_TERMINAL_PROMPT=0",
	"LC_ALL=C",
}

// SanitizedEnv derives the child environment from parent: it drops the
// variables git uses to redirect discovery, then forces the ones Mindrail
// depends on. Everything else — PATH, HOME, proxy settings — is passed through,
// because a git that cannot find its own configuration is not a safer git.
//
// Each forced variable appears exactly once, whatever the parent contained.
func SanitizedEnv(parent []string) []string {
	drop := make(map[string]struct{}, len(inheritedGitVars)+len(forcedGitVars))
	for _, name := range inheritedGitVars {
		drop[name] = struct{}{}
	}
	for _, entry := range forcedGitVars {
		name, _, _ := strings.Cut(entry, "=")
		drop[name] = struct{}{}
	}

	env := make([]string, 0, len(parent)+len(forcedGitVars))
	for _, entry := range parent {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, dropped := drop[name]; dropped {
			continue
		}
		if hasAnyPrefix(name, inheritedGitVarPrefixes) {
			continue
		}
		env = append(env, entry)
	}

	return append(env, slices.Clone(forcedGitVars)...)
}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// timeoutError reports a git invocation that outlived its deadline. It is an
// environment condition rather than a user mistake, so it carries the
// "unavailable" kind of decision D-03.
func timeoutError(args []string, limit time.Duration) error {
	return app.NewError(
		app.CodeGitTimeout,
		app.KindUnavailable,
		fmt.Sprintf("git did not answer within %s", limit),
		"Mindrail cannot determine the repository layout, so no command that needs it can run",
		"check whether the repository is on a slow or unresponsive filesystem",
		"remove a stale .git/index.lock left by an interrupted git command",
	).WithMetadata("command", "git "+strings.Join(args, " ")).
		WithMetadata("timeout", limit.String()).
		WithCause(ErrGitTimeout)
}

// workingDirectoryError reports a start directory git was never able to enter.
//
// It keeps the NOT_A_GIT_REPOSITORY code — a directory that does not exist is
// certainly not inside a repository, and the code is the stable machine
// contract (decision D-18) — and the usage exit class of decision D-03, because
// a mistyped `-C` is deterministic and user-correctable. What it must not do is
// blame the git binary: git is fine, and "install git and make sure it is on
// PATH" sends the reader to fix something that is not broken.
//
// The directory is named in the message and not only in the metadata, because
// the whole content of this failure is *which* directory.
func workingDirectoryError(dir string, cause error) error {
	return app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		fmt.Sprintf("the directory %q could not be entered: %v", dir, cause),
		"Mindrail runs git in that directory to discover the repository, and it never got that far",
		fmt.Sprintf("check that %q exists and is a directory you can enter", dir),
		"correct the -C argument, or run the command from inside the repository",
	).WithMetadata("start_dir", dir).
		WithMetadata("detail", cause.Error()).
		WithCause(errors.Join(ErrNotARepository, ErrWorkingDirectory, cause))
}

// unavailableError reports a git binary that could not be executed at all.
//
// stderr is whatever the process managed to say before failing; it is usually
// empty here, because a binary that could not be started says nothing. When it
// is not empty it is the most specific thing anyone knows about the failure, so
// it is carried rather than dropped.
func unavailableError(binary, stderr string, cause error) error {
	err := app.NewError(
		app.CodeGitUnavailable,
		app.KindUnavailable,
		fmt.Sprintf("the git executable %q could not be run", binary),
		"Mindrail drives the system git binary and cannot operate without it",
		"install git and make sure it is on PATH",
		"or verify that the git binary is executable",
	).WithMetadata("binary", binary)

	if stderr != "" {
		err = err.WithMetadata("git_stderr", stderr)
	}
	return err.WithCause(errors.Join(ErrGitUnavailable, joinStderr(cause, stderr)))
}
