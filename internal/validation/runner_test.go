package validation_test

import (
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/validation"
)

func newTestRunner(t *testing.T) *validation.Runner {
	t.Helper()
	runner, err := validation.NewRunner(t.TempDir(), 10*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

// TestRunnerRefusesBeforeSpawn is TASK-01 AC-01.2 at the gate: empty argv,
// blank executables, bad construction and a dead context never reach spawn.
func TestRunnerRefusesBeforeSpawn(t *testing.T) {
	runner := newTestRunner(t)
	for name, argv := range map[string][]string{
		"nil":   nil,
		"empty": {},
		"blank": {"  "},
	} {
		if got := runner.Run(t.Context(), argv); got.Status != validation.StatusError {
			t.Fatalf("%s = %+v, want error", name, got)
		}
	}
	if _, err := validation.NewRunner("", time.Second, 0); err == nil {
		t.Fatal("empty workdir accepted")
	}
	if _, err := validation.NewRunner(t.TempDir(), 0, 0); err == nil {
		t.Fatal("non-positive timeout accepted")
	}
}

// TestRunnerRunsArgvWithoutAShell is TASK-01 AC-01.2 and AC-01.5: a real
// process runs, and shell metacharacters in argv are data, never code.
func TestRunnerRunsArgvWithoutAShell(t *testing.T) {
	runner := newTestRunner(t)
	got := runner.Run(t.Context(), []string{"echo", "hello; echo pwned"})
	if got.Status != validation.StatusPass || got.ExitCode != 0 {
		t.Fatalf("echo = %+v", got)
	}
	if got.Stdout != "hello; echo pwned\n" {
		t.Fatalf("stdout = %q, want literal argv", got.Stdout)
	}
	if got.Stderr != "" || got.Truncated || got.TimedOut {
		t.Fatalf("echo = %+v", got)
	}
	failed := runner.Run(t.Context(), []string{"false"})
	if failed.Status != validation.StatusFail || failed.ExitCode == 0 {
		t.Fatalf("false = %+v", failed)
	}
}

// TestRunnerTimeoutKillsTheRun is TASK-01 AC-01.3: an overrunning process is
// killed and normalizes to timeout, distinct from failure.
func TestRunnerTimeoutKillsTheRun(t *testing.T) {
	runner, err := validation.NewRunner(t.TempDir(), 200*time.Millisecond, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := runner.Run(t.Context(), []string{"sleep", "30"})
	if got.Status != validation.StatusTimeout || !got.TimedOut {
		t.Fatalf("sleep = %+v, want timeout", got)
	}
}

// TestRunnerBoundsOutput is TASK-01 AC-01.4: the first OutputLimit bytes per
// stream survive with a marker, the rest is dropped.
func TestRunnerBoundsOutput(t *testing.T) {
	runner := newTestRunner(t)
	got := runner.Run(t.Context(), []string{"echo", strings.Repeat("x", validation.OutputLimit+1000)})
	if got.Status != validation.StatusPass {
		t.Fatalf("echo = %+v", got)
	}
	if !got.Truncated {
		t.Fatal("oversized output not marked truncated")
	}
	if len(got.Stdout) != validation.OutputLimit+len("\n[truncated]") {
		t.Fatalf("stdout len = %d", len(got.Stdout))
	}
}
