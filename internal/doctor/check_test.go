package doctor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestCheckFuncFillsItsOwnIdentity proves the adapter, not every check body,
// is responsible for stamping the name and section onto the result.
func TestCheckFuncFillsItsOwnIdentity(t *testing.T) {
	check := CheckFunc{
		CheckName:    "probe",
		CheckSection: "Repository",
		Fn: func(context.Context) Result {
			return Result{State: StateOK, Summary: "fine"}
		},
	}

	if check.Name() != "probe" {
		t.Errorf("Name() = %q, want %q", check.Name(), "probe")
	}

	got := check.Run(t.Context())
	if got.Name != "probe" {
		t.Errorf("Result.Name = %q, want %q", got.Name, "probe")
	}
	if got.Section != "Repository" {
		t.Errorf("Result.Section = %q, want %q", got.Section, "Repository")
	}
}

// TestCheckFuncWithoutFunctionIsNotSilent pins the behaviour of a misregistered
// check: it reports itself rather than passing as healthy.
func TestCheckFuncWithoutFunctionIsNotSilent(t *testing.T) {
	got := CheckFunc{CheckName: "empty", CheckSection: "Repository"}.Run(t.Context())

	if got.State == StateOK {
		t.Fatalf("a check with no function reported %q", got.State)
	}
	assertDiagnosable(t, got)
}

// TestRunnerReportsInRegistrationOrder pins report order, which the human
// renderer and every golden depend on.
func TestRunnerReportsInRegistrationOrder(t *testing.T) {
	runner := NewRunner(okCheck("first", "A"), okCheck("second", "B"))
	runner.Register(okCheck("third", "C"))

	wantNames := []string{"first", "second", "third"}
	gotNames := runner.Names()
	if len(gotNames) != len(wantNames) {
		t.Fatalf("Names() = %v, want %v", gotNames, wantNames)
	}
	for i := range wantNames {
		if gotNames[i] != wantNames[i] {
			t.Errorf("Names()[%d] = %q, want %q", i, gotNames[i], wantNames[i])
		}
	}

	report := runner.Run(t.Context())
	if report.Command != "doctor" {
		t.Errorf("Report.Command = %q, want %q", report.Command, "doctor")
	}
	if len(report.Checks) != len(wantNames) {
		t.Fatalf("report has %d checks, want %d", len(report.Checks), len(wantNames))
	}
	for i := range wantNames {
		if report.Checks[i].Name != wantNames[i] {
			t.Errorf("Checks[%d].Name = %q, want %q", i, report.Checks[i].Name, wantNames[i])
		}
	}
}

// TestRunnerConstructorDoesNotAliasItsArgument keeps a caller's slice from
// rewriting the registered check set after construction.
func TestRunnerConstructorDoesNotAliasItsArgument(t *testing.T) {
	checks := []Check{okCheck("first", "A")}
	runner := NewRunner(checks...)
	checks[0] = okCheck("swapped", "A")

	if got := runner.Names(); got[0] != "first" {
		t.Errorf("runner check set aliased its argument: Names()[0] = %q", got[0])
	}
}

// TestWorstStateRanksSeverity pins the single ordering used for both the
// reported worst_state and, through it, the doctor exit code (decision D-14).
func TestWorstStateRanksSeverity(t *testing.T) {
	tests := []struct {
		name   string
		states []State
		want   State
	}{
		{name: "no checks", states: nil, want: StateOK},
		{name: "all ok", states: []State{StateOK, StateOK}, want: StateOK},
		{name: "not applicable never outranks ok", states: []State{StateOK, StateNotApplicable}, want: StateOK},
		{name: "only not applicable", states: []State{StateNotApplicable}, want: StateNotApplicable},
		{name: "degraded outranks ok", states: []State{StateOK, StateDegraded}, want: StateDegraded},
		{name: "unavailable outranks degraded", states: []State{StateDegraded, StateUnavailable}, want: StateUnavailable},
		{name: "error outranks everything", states: []State{StateUnavailable, StateError, StateDegraded}, want: StateError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checks := make([]Check, 0, len(tc.states))
			for i, state := range tc.states {
				checks = append(checks, stateCheck(string(rune('a'+i)), state))
			}

			if got := NewRunner(checks...).Run(t.Context()).WorstState; got != tc.want {
				t.Errorf("WorstState = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestReportErrOnlyForErrorState pins decision D-14: an absent optional
// capability must never make CI fail, but a genuine ERROR must.
func TestReportErrOnlyForErrorState(t *testing.T) {
	tests := []struct {
		name    string
		states  []State
		wantErr bool
	}{
		{name: "healthy", states: []State{StateOK}},
		{name: "degraded", states: []State{StateDegraded}},
		{name: "unavailable", states: []State{StateUnavailable}},
		{name: "not applicable", states: []State{StateNotApplicable}},
		{name: "error", states: []State{StateOK, StateError}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checks := make([]Check, 0, len(tc.states))
			for i, state := range tc.states {
				checks = append(checks, stateCheck(string(rune('a'+i)), state))
			}

			err := NewRunner(checks...).Run(t.Context()).Err()
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("Err() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Err() = nil, want a domain error")
			}

			var domain *app.DomainError
			if !errors.As(err, &domain) {
				t.Fatalf("Err() = %v, want an *app.DomainError", err)
			}
			if domain.Why == "" || domain.Impact == "" || len(domain.NextAction) == 0 {
				t.Errorf("Err() lost the diagnosis: %+v", domain)
			}
		})
	}
}

// TestReportErrExitCodeFollowsTheFailureClass pins decision D-03 through the
// only path a caller has: the exit code of the error doctor returns.
func TestReportErrExitCodeFollowsTheFailureClass(t *testing.T) {
	tests := []struct {
		code app.Code
		want int
	}{
		{code: app.CodeNotAGitRepository, want: app.ExitUsage},
		{code: app.CodeBareRepository, want: app.ExitUsage},
		{code: app.CodeConfigInvalid, want: app.ExitUsage},
		{code: app.CodePathEscapesRoot, want: app.ExitUsage},
		{code: app.CodeRuntimePathUnwritable, want: app.ExitUnavailable},
		{code: app.CodeRuntimeDBUnavailable, want: app.ExitUnavailable},
		{code: app.CodeRuntimeDBCorrupt, want: app.ExitUnavailable},
		{code: app.CodeGitUnavailable, want: app.ExitUnavailable},
		{code: app.CodeGitTimeout, want: app.ExitUnavailable},
		{code: app.CodeMigrationFailed, want: app.ExitFailed},
		{code: app.CodeMigrationChecksumMismatch, want: app.ExitFailed},
		{code: app.CodeRuntimeDBSchemaTooNew, want: app.ExitFailed},
		{code: app.CodeKnowledgeSchemaUnsupported, want: app.ExitFailed},
	}

	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			result := stateCheck("probe", StateError).Run(t.Context())
			result.Code = tc.code
			report := Report{Command: "doctor", Checks: []Result{result}, WorstState: StateError}

			if got := app.ExitCode(report.Err()); got != tc.want {
				t.Errorf("exit code for %s = %d, want %d", tc.code, got, tc.want)
			}
		})
	}
}

// TestRunnerRespectsContextCancellation proves the context reaches the check
// bodies. A doctor that ignored it would keep a cancelled process alive for as
// long as its slowest probe.
func TestRunnerRespectsContextCancellation(t *testing.T) {
	blocked := CheckFunc{
		CheckName:    "blocking",
		CheckSection: "Repository",
		Fn: func(ctx context.Context) Result {
			select {
			case <-ctx.Done():
				return Result{
					State:      StateUnavailable,
					Summary:    "check cancelled",
					Diagnostic: ctx.Err().Error(),
					Impact:     "This check reports nothing for this run.",
					NextAction: []string{"Run mindrail doctor again."},
				}
			case <-time.After(30 * time.Second):
				return Result{State: StateOK, Summary: "never reached"}
			}
		},
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	done := make(chan Report, 1)
	go func() {
		done <- NewRunner(append(DefaultChecks(healthySubject()), blocked)...).Run(ctx)
	}()

	select {
	case report := <-done:
		if len(report.Checks) != len(DefaultChecks(healthySubject()))+1 {
			t.Fatalf("report has %d checks, want every registered check to report", len(report.Checks))
		}
		last := report.Checks[len(report.Checks)-1]
		if last.Name != "blocking" {
			t.Fatalf("last check is %q, want %q", last.Name, "blocking")
		}
		if last.State != StateUnavailable {
			t.Errorf("cancelled check reported %q, want %q", last.State, StateUnavailable)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return on a cancelled context")
	}
}

// okCheck is a minimal healthy check used to exercise the runner itself.
func okCheck(name, section string) Check {
	return stateCheckIn(name, section, StateOK)
}

func stateCheck(name string, state State) Check {
	return stateCheckIn(name, "Repository", state)
}

// stateCheckIn returns a check that reports state and, when that state is not
// OK, carries the full diagnosis every non-OK result owes its reader.
func stateCheckIn(name, section string, state State) Check {
	return CheckFunc{
		CheckName:    name,
		CheckSection: section,
		Fn: func(context.Context) Result {
			result := Result{State: state, Summary: name + " reported " + string(state)}
			if state != StateOK {
				result.Diagnostic = "synthetic " + string(state)
				result.Impact = "nothing, this is a test double"
				result.NextAction = []string{"ignore this result"}
			}
			return result
		},
	}
}
