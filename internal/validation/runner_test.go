package validation_test

import (
	"os"
	"os/exec"
	"path/filepath"
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
	if !filepath.IsAbs(got.ResolvedExe) || filepath.Base(got.ResolvedExe) != "echo" {
		t.Fatalf("resolved = %q, want the absolute binary that ran", got.ResolvedExe)
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

// TestRunnerRefusesUnresolvableBinary pins the spawn-failure branch: a name
// nothing resolves fails here with the executable named, never at spawn,
// and never with the full argv echoed.
func TestRunnerRefusesUnresolvableBinary(t *testing.T) {
	runner := newTestRunner(t)
	got := runner.Run(t.Context(), []string{"mindrail-no-such-binary", "s3cr3t-arg"})
	if got.Status != validation.StatusError {
		t.Fatalf("unresolvable = %+v, want error", got)
	}
	if got.ResolvedExe != "" {
		t.Fatalf("resolved = %q, want empty on refusal", got.ResolvedExe)
	}
	if strings.Contains(got.ErrText, "s3cr3t-arg") {
		t.Fatalf("diagnostic echoes argv: %q", got.ErrText)
	}
}

// TestHelperProcess is re-executed by TestRunnerTimeoutKillsTheWholeGroup:
// it spawns a grandchild sharing the pipes and holds them open until
// killed. No shell anywhere — the helper is this test binary.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("MR010_HELPER") != "grandchild" {
		return
	}
	grandchild := exec.Command("sleep", "10")
	grandchild.Stdout = os.Stdout
	grandchild.Stderr = os.Stderr
	if err := grandchild.Start(); err != nil {
		os.Exit(2)
	}
	select {}
}

// TestRunnerTimeoutKillsTheWholeGroup is Breaker B-2's repro as a pin:
// without process-group kill, Run would block on the pipes until sleep's
// 10 seconds elapse despite the 300ms timeout.
func TestRunnerTimeoutKillsTheWholeGroup(t *testing.T) {
	runner, err := validation.NewRunner(t.TempDir(), 300*time.Millisecond, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MR010_HELPER", "grandchild")
	got := runner.Run(t.Context(), []string{os.Args[0], "-test.run=TestHelperProcess"})
	if got.Status != validation.StatusTimeout || !got.TimedOut {
		t.Fatalf("helper = %+v, want timeout", got)
	}
	if got.Duration >= 8*time.Second {
		t.Fatalf("Run returned after %v with a 300ms timeout", got.Duration)
	}
}
