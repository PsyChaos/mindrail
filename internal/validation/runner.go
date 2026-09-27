// Package validation runs project-defined validation profiles as real
// processes and records snapshot-bound evidence (MR-010). Execution is
// argv-only: a shell string never exists, not even in tests. Results are
// values (pass/fail/timeout/error), never blockers.
package validation

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// OutputLimit bounds each captured stream (decision D-157): the first
// OutputLimit bytes are kept with a truncation marker, the rest is dropped.
const OutputLimit = 65536

// Run statuses. Timeout is distinct from failure: a killed run proved
// nothing, while a finished non-zero run proved failure.
const (
	StatusPass    = "pass"
	StatusFail    = "fail"
	StatusTimeout = "timeout"
	StatusError   = "error"
)

// Result is one finished command: normalized status, the resolved
// executable that actually ran, bounded raw streams, and the timing the run
// reported (never asserted exactly). Raw output lives only here; the store
// redacts before insert.
type Result struct {
	Status      string
	ExitCode    int
	ResolvedExe string
	Stdout      string
	Stderr      string
	Truncated   bool
	TimedOut    bool
	Duration    time.Duration
	ErrText     string
}

// Runner executes argv in a fixed working directory with a fixed timeout.
// Zero value runs nothing: NewRunner refuses non-positive timeouts, and a
// non-positive output limit falls back to OutputLimit.
type Runner struct {
	workdir string
	timeout time.Duration
	limit   int64
}

// NewRunner builds a Runner over an absolute working directory.
func NewRunner(workdir string, timeout time.Duration, limit int64) (*Runner, error) {
	if workdir == "" {
		return nil, invalidInput("validation runner needs a working directory")
	}
	if timeout <= 0 {
		return nil, invalidInput("validation runner needs a positive timeout")
	}
	if limit <= 0 {
		limit = OutputLimit
	}
	return &Runner{workdir: workdir, timeout: timeout, limit: limit}, nil
}

// Run executes one argv directly — exec.Command, never a shell — and
// normalizes the outcome. Empty argv and blank executables are refused
// before spawn; the executable is resolved to an absolute path first, so
// evidence records what actually ran and unresolvable names fail here
// instead of at spawn. The context timeout kills the whole process group;
// streams are bounded. Diagnostics name the executable only, never the full
// argv: arguments may carry secret values.
func (r *Runner) Run(ctx context.Context, argv []string) Result {
	start := time.Now()
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return Result{Status: StatusError, ExitCode: -1, ErrText: "refused empty command"}
	}
	if err := ctx.Err(); err != nil {
		return Result{Status: StatusError, ExitCode: -1, ErrText: "run context already done"}
	}
	resolved, err := exec.LookPath(argv[0])
	if err != nil {
		return Result{Status: StatusError, ExitCode: -1, ErrText: "cannot resolve executable " + argv[0]}
	}
	timeout, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	command := exec.CommandContext(timeout, resolved, argv[1:]...)
	command.Dir = r.workdir
	newProcessGroup(command)
	var stdout, stderr boundedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	stdout.cap = r.limit
	stderr.cap = r.limit
	if err := command.Start(); err != nil {
		return Result{Status: StatusError, ExitCode: -1, ResolvedExe: resolved,
			ErrText: "could not start " + argv[0]}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-timeout.Done():
			if errors.Is(timeout.Err(), context.DeadlineExceeded) && command.Process != nil {
				killProcessGroup(command.Process.Pid)
			}
		case <-done:
		}
	}()
	err = command.Wait()
	close(done)
	result := Result{
		ResolvedExe: resolved,
		Stdout:      stdout.String(),
		Stderr:      stderr.String(),
		Truncated:   stdout.truncated || stderr.truncated,
		Duration:    time.Since(start),
	}
	var exitErr *exec.ExitError
	switch {
	case errors.Is(timeout.Err(), context.DeadlineExceeded):
		result.Status = StatusTimeout
		result.ExitCode = -1
		result.TimedOut = true
	case ctx.Err() != nil:
		result.Status = StatusError
		result.ExitCode = -1
		result.ErrText = "run context done"
	case err == nil:
		result.Status = StatusPass
	case errors.As(err, &exitErr):
		result.Status = StatusFail
		result.ExitCode = command.ProcessState.ExitCode()
	default:
		result.Status = StatusError
		result.ExitCode = -1
		result.ErrText = "could not start " + argv[0]
	}
	return result
}

// boundedBuffer keeps the first cap bytes and records the drop.
type boundedBuffer struct {
	cap       int64
	buf       bytes.Buffer
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	room := b.cap - int64(b.buf.Len())
	if room <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if int64(len(p)) > room {
		b.buf.Write(p[:room])
		b.truncated = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *boundedBuffer) String() string {
	if !b.truncated {
		return b.buf.String()
	}
	return b.buf.String() + "\n[truncated]"
}

func invalidInput(why string) error {
	return app.NewError(app.CodeCommandLineInvalid, app.KindUsage, why,
		"No validation facts were changed.", "Pass a valid profile, scope and command.")
}
