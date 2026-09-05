package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

func TestExecRunnerUsesArgvNeverShell(t *testing.T) {
	dir := t.TempDir()

	// Every one of these is inert as an argv element and dangerous the moment a
	// shell sees it. tech-stack §35 forbids the shell outright.
	hostile := []string{
		"; rm -rf /",
		"$(id)",
		"`whoami`",
		"--upload-pack=evil",
		"-x",
		"--exec=touch /tmp/pwned",
		"a b\tc",
		"line1\nline2",
		"*",
		"&& echo owned",
		"| tee /tmp/out",
		"'",
		`"`,
		"",
	}

	for _, arg := range hostile {
		t.Run(strings.ReplaceAll(arg, "\n", `\n`), func(t *testing.T) {
			runner := NewExecRunner()
			cmd := runner.command(context.Background(), dir, []string{"rev-parse", arg})

			if base := filepath.Base(cmd.Path); base != "git" && base != "git.exe" {
				t.Fatalf("Cmd.Path = %q, want the git binary", cmd.Path)
			}
			for _, shell := range []string{"sh", "bash", "zsh", "dash", "cmd.exe", "powershell.exe"} {
				if filepath.Base(cmd.Path) == shell {
					t.Fatalf("Cmd.Path = %q, which is a shell", cmd.Path)
				}
			}

			want := []string{cmd.Args[0], "rev-parse", arg}
			if !slices.Equal(cmd.Args, want) {
				t.Fatalf("Cmd.Args = %#v, want %#v", cmd.Args, want)
			}
			if cmd.Args[2] != arg {
				t.Fatalf("argument was rewritten: got %q, want %q", cmd.Args[2], arg)
			}
			if cmd.Dir != dir {
				t.Fatalf("Cmd.Dir = %q, want %q; the directory must travel out of band", cmd.Dir, dir)
			}
			if cmd.Stdin != nil {
				t.Fatalf("Cmd.Stdin = %v, want nil so git can never block on input", cmd.Stdin)
			}
		})
	}
}

func TestExecRunnerSanitizesGitEnv(t *testing.T) {
	t.Run("inherited git variables are stripped", func(t *testing.T) {
		parent := []string{
			"PATH=/usr/bin",
			"GIT_DIR=/somewhere/else/.git",
			"GIT_WORK_TREE=/somewhere/else",
			"GIT_COMMON_DIR=/somewhere/else/.git",
			"GIT_INDEX_FILE=/somewhere/else/.git/index",
			"GIT_OBJECT_DIRECTORY=/somewhere/else/.git/objects",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES=/alt",
			"GIT_NAMESPACE=refs/namespaces/x",
			// Both of these steer the discovery walk itself rather than the
			// repository it lands on, which is the harder leak to notice: they
			// make a valid repository look absent instead of making a different
			// one look present.
			"GIT_CEILING_DIRECTORIES=/somewhere",
			"GIT_DISCOVERY_ACROSS_FILESYSTEM=1",
			"HOME=/home/user",
		}

		got := SanitizedEnv(parent)

		for _, name := range []string{
			"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE",
			"GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM",
		} {
			if value, ok := lookupEnv(got, name); ok {
				t.Errorf("%s survived sanitation with value %q", name, value)
			}
		}

		for name, want := range map[string]string{
			"GIT_OPTIONAL_LOCKS":  "0",
			"GIT_TERMINAL_PROMPT": "0",
			"LC_ALL":              "C",
		} {
			value, ok := lookupEnv(got, name)
			if !ok {
				t.Errorf("%s is not set", name)
				continue
			}
			if value != want {
				t.Errorf("%s = %q, want %q", name, value, want)
			}
		}

		// Unrelated variables must survive: PATH is how git is found at all.
		for _, name := range []string{"PATH", "HOME"} {
			if _, ok := lookupEnv(got, name); !ok {
				t.Errorf("%s was dropped; only the git-specific variables are stripped", name)
			}
		}
	})

	// TestExecRunnerSanitizesGitEnv/command-line config injection is the
	// regression test for finding F2. GIT_CONFIG_COUNT and its indexed pairs are
	// git's command-line-config channel: they splice configuration ahead of
	// every file git reads, which is the same ambient redirection D-29 exists to
	// prevent. A stale or malformed count makes *every* invocation exit 128, so
	// a perfectly valid repository was reported as no repository at all.
	t.Run("the command-line config injection channel is stripped", func(t *testing.T) {
		parent := []string{
			"PATH=/usr/bin",
			"GIT_CONFIG_COUNT=2",
			"GIT_CONFIG_KEY_0=core.worktree",
			"GIT_CONFIG_VALUE_0=/somewhere/else",
			"GIT_CONFIG_KEY_1=safe.directory",
			"GIT_CONFIG_VALUE_1=/somewhere/else",
			// Indices are unbounded, so the rule cannot be an enumeration.
			"GIT_CONFIG_KEY_137=include.path",
			"GIT_CONFIG_VALUE_137=/somewhere/else/extra",
		}

		got := SanitizedEnv(parent)

		for _, name := range []string{
			"GIT_CONFIG_COUNT",
			"GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0",
			"GIT_CONFIG_KEY_1", "GIT_CONFIG_VALUE_1",
			"GIT_CONFIG_KEY_137", "GIT_CONFIG_VALUE_137",
		} {
			if value, ok := lookupEnv(got, name); ok {
				t.Errorf("%s survived sanitation with value %q", name, value)
			}
		}
		if _, ok := lookupEnv(got, "PATH"); !ok {
			t.Errorf("PATH was dropped; only the git-specific variables are stripped")
		}
	})

	// The over-fire guard for the rule above. Stripping too much is the failure
	// mode that turns a healthy repository into a broken one, and three of these
	// names are how this package's own fixtures detach from the developer's
	// global git configuration — taking them would break every fixture test for
	// a reason that has nothing to do with repository discovery.
	t.Run("configuration variables that select files, not repositories, survive", func(t *testing.T) {
		parent := []string{
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_CONFIG_NOSYSTEM=1",
			// Neither of these is part of the injection channel; a prefix rule
			// of "GIT_CONFIG_KEY" or "GIT_CONFIG" rather than
			// "GIT_CONFIG_KEY_" would swallow them.
			"GIT_CONFIG_KEYRING=login",
			"GIT_CONFIG_VALUES_SUMMARY=1",
			"GIT_CONFIG=/etc/gitconfig",
			// Ordinary git variables that steer nothing about discovery.
			"GIT_AUTHOR_NAME=Someone",
			"GIT_EDITOR=vi",
			"GIT_PAGER=cat",
			"GIT_SSH_COMMAND=ssh -o Foo=bar",
		}

		got := SanitizedEnv(parent)

		for _, entry := range parent {
			name, want, _ := strings.Cut(entry, "=")
			value, ok := lookupEnv(got, name)
			if !ok {
				t.Errorf("%s was dropped; it selects a configuration file or an unrelated behaviour, not a repository", name)
				continue
			}
			if value != want {
				t.Errorf("%s = %q, want %q", name, value, want)
			}
		}
	})

	t.Run("forced variables are set exactly once", func(t *testing.T) {
		parent := []string{
			"LC_ALL=tr_TR.UTF-8",
			"GIT_OPTIONAL_LOCKS=1",
			"GIT_TERMINAL_PROMPT=1",
		}

		got := SanitizedEnv(parent)

		for _, name := range []string{"LC_ALL", "GIT_OPTIONAL_LOCKS", "GIT_TERMINAL_PROMPT"} {
			if n := countEnv(got, name); n != 1 {
				t.Errorf("%s appears %d times, want exactly 1", name, n)
			}
		}
		if value, _ := lookupEnv(got, "LC_ALL"); value != "C" {
			t.Errorf("LC_ALL = %q, want the inherited locale to be overridden with C", value)
		}
	})

	t.Run("the built command carries the sanitized environment", func(t *testing.T) {
		t.Setenv("GIT_DIR", "/hook/provided/.git")
		t.Setenv("GIT_INDEX_FILE", "/hook/provided/.git/index")

		cmd := NewExecRunner().command(context.Background(), t.TempDir(), []string{"rev-parse", "--git-dir"})

		if cmd.Env == nil {
			t.Fatalf("Cmd.Env is nil, which makes the child inherit the parent environment verbatim")
		}
		if _, ok := lookupEnv(cmd.Env, "GIT_DIR"); ok {
			t.Errorf("GIT_DIR reached the child; a pre-commit hook would redirect discovery")
		}
		if _, ok := lookupEnv(cmd.Env, "GIT_INDEX_FILE"); ok {
			t.Errorf("GIT_INDEX_FILE reached the child")
		}
	})
}

func TestExecRunnerCancellationKillsChild(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	markerFile := filepath.Join(dir, "completed")

	runner := helperRunner(t, 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		waitForFile(pidFile, 10*time.Second)
		cancel()
	}()

	start := time.Now()
	_, _, err := runner.Run(ctx, dir, helperArgv("sleep", "30s", pidFile, markerFile)...)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want it to unwrap to context.Canceled", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("Run took %s; cancellation must not wait for the child to finish", elapsed)
	}
	if _, statErr := os.Stat(markerFile); statErr == nil {
		t.Fatalf("the child ran to completion despite cancellation")
	}

	if runtime.GOOS == "windows" {
		return
	}
	pid := readPID(t, pidFile)
	if processAlive(pid) {
		t.Fatalf("child process %d is still running after cancellation", pid)
	}
}

func TestExecRunnerTimeout(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	markerFile := filepath.Join(dir, "completed")

	runner := helperRunner(t, 200*time.Millisecond)

	start := time.Now()
	_, _, err := runner.Run(context.Background(), dir, helperArgv("sleep", "30s", pidFile, markerFile)...)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrGitTimeout) {
		t.Fatalf("Run error = %v, want it to unwrap to ErrGitTimeout", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("Run took %s; the timeout must bound the wait", elapsed)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("timeout error carries no domain payload: %v", err)
	}
	if payload.Code != app.CodeGitTimeout {
		t.Fatalf("payload code = %q, want %q", payload.Code, app.CodeGitTimeout)
	}
	if len(payload.NextAction) == 0 {
		t.Fatalf("timeout payload has no next action")
	}
	if got := app.ExitCode(err); got != app.ExitUnavailable {
		t.Fatalf("ExitCode = %d, want %d (unavailable)", got, app.ExitUnavailable)
	}

	// A timeout is an environment condition, not the caller giving up.
	if errors.Is(err, context.Canceled) {
		t.Fatalf("a timeout must not present itself as a cancellation")
	}
}

func TestDefaultTimeoutIsBounded(t *testing.T) {
	if DefaultTimeout != 10*time.Second {
		t.Fatalf("DefaultTimeout = %s, want 10s (decision D-30)", DefaultTimeout)
	}
	if got := NewExecRunner().timeout(); got != DefaultTimeout {
		t.Fatalf("zero Timeout = %s, want the default %s", got, DefaultTimeout)
	}
	if got := (&ExecRunner{Timeout: time.Second}).timeout(); got != time.Second {
		t.Fatalf("explicit Timeout was overridden: got %s", got)
	}
	if got := NewExecRunner().binary(); got != "git" {
		t.Fatalf("zero Bin = %q, want %q", got, "git")
	}
}

// helperRunner points an ExecRunner at this test binary, which stands in for
// git. Building a stub script would need a shell — the one thing this package
// is not allowed to touch.
func helperRunner(t *testing.T, timeout time.Duration) *ExecRunner {
	t.Helper()

	t.Setenv(helperEnvVar, "1")
	return &ExecRunner{Bin: os.Args[0], Timeout: timeout}
}

func helperArgv(args ...string) []string {
	return append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)
}

const helperEnvVar = "MINDRAIL_GIT_TEST_HELPER"

// TestHelperProcess is not a test. It is the body of the child process spawned
// by the runner tests; it exits early unless the parent asked for it.
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnvVar) != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		return
	}

	switch args[0] {
	case "sleep":
		duration, err := time.ParseDuration(args[1])
		if err != nil {
			os.Exit(2)
		}
		_ = os.WriteFile(args[2], []byte(strconv.Itoa(os.Getpid())), 0o600)
		time.Sleep(duration)
		_ = os.WriteFile(args[3], []byte("completed"), 0o600)
	default:
		os.Exit(2)
	}
}

func lookupEnv(env []string, name string) (string, bool) {
	prefix := name + "="
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, prefix); ok {
			return value, true
		}
	}
	return "", false
}

func countEnv(env []string, name string) int {
	prefix := name + "="
	count := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			count++
		}
	}
	return count
}

func waitForFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func readPID(t *testing.T, path string) int {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse child pid %q: %v", raw, err)
	}
	return pid
}

// processAlive reports whether a pid still names a live process. Signal 0 does
// no work; it only asks the kernel whether the target is deliverable.
func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
