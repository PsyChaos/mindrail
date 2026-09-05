package status

import (
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/git"
)

// TestRepositoryBlockIsMarkedWhenDiscoveryDidNotAnswer is finding H12.
//
// Every other block of the report gained an Observation, and the repository
// block — the one acceptance criterion 3 is actually about — did not. `init`
// renders a whole report even when startup stopped at resolve_repository, so a
// run that never learned which repository it was in still published three empty
// path strings and a flat "is_linked_worktree: false": one unasked question
// answered four times, in the block a reader consults precisely to find out
// which repository Mindrail looked at.
func TestRepositoryBlockIsMarkedWhenDiscoveryDidNotAnswer(t *testing.T) {
	report := Build(subjectWithoutRepository(), 3*time.Millisecond)

	if report.Repository.Observation != Indeterminate {
		t.Errorf("repository observation = %q, want %q when discovery failed",
			report.Repository.Observation, Indeterminate)
	}
	if report.Repository.Observation.Known() {
		t.Error("repository block claims its zero values are findings")
	}

	human := renderHuman(t, report)
	if !strings.Contains(human, "Linked worktree: unknown") {
		t.Errorf("human report states a linked-worktree verdict it never observed:\n%s", human)
	}
	if !strings.Contains(human, observationNote(Indeterminate)) {
		t.Errorf("human report carries no observation note for the repository block:\n%s", human)
	}
}

// TestRepositoryBlockIsMarkedWhenGitIsUnavailable is the same defect through the
// condition that hides it best.
//
// Decision D-30 keeps a missing or hanging git UNAVAILABLE rather than ERROR, so
// grading the block by "did the reading fail?" would call it observed. Nothing
// was read: the layout fields are zero because no `git rev-parse` ever ran.
func TestRepositoryBlockIsMarkedWhenGitIsUnavailable(t *testing.T) {
	s := subjectWithoutRepository()
	s.RepoErr = app.NewError(app.CodeGitUnavailable, app.KindUnavailable,
		"git is not on PATH",
		"Mindrail cannot discover the repository, so no command can run.",
		"Install git and make sure it is on PATH.").
		WithCause(git.ErrGitUnavailable)

	report := Build(s, 3*time.Millisecond)
	if report.Repository.Observation.Known() {
		t.Errorf("repository observation = %q, want a marker: no git ran, so nothing was read",
			report.Repository.Observation)
	}
}

// TestRepositoryBlockIsObservedOnAHealthyRepository is the first over-fire guard
// for finding H12: the marker must not appear where the reading is real, or
// every healthy report would carry a caveat about the one answer nobody has to
// double-check.
func TestRepositoryBlockIsObservedOnAHealthyRepository(t *testing.T) {
	report := Build(healthySubject(), 3*time.Millisecond)

	if report.Repository.Observation != Observed {
		t.Fatalf("repository observation = %q, want %q on a resolved repository",
			report.Repository.Observation, Observed)
	}

	human := renderHuman(t, report)
	if !strings.Contains(human, "Linked worktree: false") {
		t.Errorf("human report hides an observed linked-worktree answer:\n%s", human)
	}
	if strings.Contains(human, "not observed") || strings.Contains(human, "indeterminate") {
		t.Errorf("healthy report carries an observation caveat:\n%s", human)
	}
}

// TestRepositoryBlockSurvivesALaterStepFailing is the second over-fire guard,
// and the adjacent condition the marker must not spread to.
//
// The repository was resolved. That the database could not be opened afterwards
// says nothing about the Git layout, which was read before the failure and is
// still a finding — the whole point of grading blocks separately is that one
// broken subsystem does not erase the readings that came before it.
func TestRepositoryBlockSurvivesALaterStepFailing(t *testing.T) {
	for name, build := range map[string]func() Report{
		"database will not open": func() Report { return Build(haltedAtSQLiteSubject(), 3*time.Millisecond) },
		"never initialised":      func() Report { return Build(uninitializedSubject(), 3*time.Millisecond) },
	} {
		t.Run(name, func(t *testing.T) {
			report := build()
			if report.Repository.Observation != Observed {
				t.Errorf("repository observation = %q, want %q: discovery succeeded, a later step did not",
					report.Repository.Observation, Observed)
			}
			if report.Repository.CommonDir == "" || report.Repository.WorktreeRoot == "" {
				t.Errorf("repository block lost the layout it did observe: %+v", report.Repository)
			}
		})
	}
}

// TestBlockingComponentIsNeverOneNobodyInspected is finding H15.
//
// When startup stops before any component's inputs exist, every blocking
// candidate is a subsystem that was never read, and naming the first of them
// asserted that `knowledge` is why work cannot start — while the error object in
// the same document attributed the halt to the `git` check. Two fields, one
// document, two answers.
func TestBlockingComponentIsNeverOneNobodyInspected(t *testing.T) {
	report := Build(subjectWithoutRepository(), 3*time.Millisecond)

	if report.Readiness != ReadinessBlocked {
		t.Fatalf("readiness = %q, want %q: nothing could be verified", report.Readiness, ReadinessBlocked)
	}
	if report.BlockingComponent != "" {
		t.Errorf("blocking_component = %q, but no component was inspected; stopped_at_step = %q",
			report.BlockingComponent, report.StoppedAtStep)
	}
	if report.StoppedAtStep == "" {
		t.Error("report names no component and no step, so it says nothing about what stopped the run")
	}
	if len(report.NextAction) == 0 {
		t.Error("report is BLOCKED with no next action")
	}
}

// TestBlockingComponentIsStillNamedWhenOneWasInspected is the over-fire guard
// for finding H15. Dropping the name is only honest where there is nothing to
// name; a report that stopped naming the component it did inspect would lose the
// field that tells a reader where to look.
func TestBlockingComponentIsStillNamedWhenOneWasInspected(t *testing.T) {
	tests := map[string]struct {
		subject func() (Report, ComponentName)
	}{
		"runtime store was inspected and is empty": {subject: func() (Report, ComponentName) {
			return Build(uninitializedSubject(), 3*time.Millisecond), ComponentRuntimeDB
		}},
		"knowledge was inspected and refused": {subject: func() (Report, ComponentName) {
			return Build(subjectWithKnowledgeProblem(true), 3*time.Millisecond), ComponentKnowledge
		}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			report, want := tc.subject()
			if report.Readiness != ReadinessBlocked {
				t.Fatalf("readiness = %q, want %q", report.Readiness, ReadinessBlocked)
			}
			if report.BlockingComponent != want {
				t.Errorf("blocking_component = %q, want %q", report.BlockingComponent, want)
			}
		})
	}
}

// TestHealthyReportNamesNoBlockingComponent is the second over-fire guard: the
// field stays empty on a repository where nothing blocks, which is what makes
// its presence meaningful anywhere else.
func TestHealthyReportNamesNoBlockingComponent(t *testing.T) {
	report := Build(healthySubject(), 3*time.Millisecond)

	if report.Readiness != ReadinessReady {
		t.Fatalf("readiness = %q, want %q", report.Readiness, ReadinessReady)
	}
	if report.BlockingComponent != "" {
		t.Errorf("blocking_component = %q on a healthy repository", report.BlockingComponent)
	}
	if report.StoppedAtStep != "" {
		t.Errorf("stopped_at_step = %q on a repository whose startup completed", report.StoppedAtStep)
	}
}
