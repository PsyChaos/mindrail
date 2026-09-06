package cli

import (
	"errors"
	"testing"

	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/status"
)

// TestTerminalStateNeedsEitherAnswerAlone is finding F14's first pair of
// mutation survivors, killed from a value.
//
// Both halves of the disjunction that decides spec §82's terminal line were
// documented as load-bearing and neither was exercised: dropping either one
// passed the whole suite, because every condition a running command can reach
// sets both at once. The two middle rows are the ones that matter — each is a
// clause on its own — and they are reachable here and nowhere else.
func TestTerminalStateNeedsEitherAnswerAlone(t *testing.T) {
	failure := errors.New("something the checks reported")

	tests := []struct {
		name      string
		verdict   error
		readiness status.Readiness
		want      status.TerminalState
	}{
		{
			name:      "nothing wrong",
			readiness: status.ReadinessReady,
			want:      status.TerminalReady,
		},
		{
			name:      "a verdict with readiness that does not block",
			verdict:   failure,
			readiness: status.ReadinessReady,
			want:      status.TerminalBlocked,
		},
		{
			name:      "blocked readiness with no verdict",
			readiness: status.ReadinessBlocked,
			want:      status.TerminalBlocked,
		},
		{
			name:      "both",
			verdict:   failure,
			readiness: status.ReadinessBlocked,
			want:      status.TerminalBlocked,
		},
		{
			// Degraded is the adjacent state, and the over-fire guard: a
			// repository that is imperfect and usable has to keep its READY
			// line, or init reports every warning as a refusal.
			name:      "degraded is not blocked",
			readiness: status.ReadinessDegraded,
			want:      status.TerminalReady,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := terminalStateOf(tc.verdict, tc.readiness); got != tc.want {
				t.Errorf("terminalStateOf(%v, %q) = %q, want %q", tc.verdict, tc.readiness, got, tc.want)
			}
		})
	}
}

// TestInitVerdictPrefersTheChecksAndKeepsWhatTheyCannotSee covers the fold no
// running command produced.
//
// The flush is init's own last write, and its failure has to reach the report or
// init calls a repository it could not finish writing ready. But the checks run
// after it and usually describe the same condition in the vocabulary `status`
// and `doctor` use, so theirs is the sentence a reader gets. Neither half was
// reachable through a command: dropping the fold left the whole suite green.
func TestInitVerdictPrefersTheChecksAndKeepsWhatTheyCannotSee(t *testing.T) {
	checked := errors.New("the runtime path is not writable")
	flush := errors.New("the log could not be written back")

	tests := []struct {
		name    string
		checked error
		flush   error
		want    error
	}{
		{name: "nothing wrong"},
		{
			name:  "only the flush failed",
			flush: flush,
			want:  flush,
		},
		{
			name:    "the checks saw it too",
			checked: checked,
			flush:   flush,
			want:    checked,
		},
		{
			name:    "only the checks failed",
			checked: checked,
			want:    checked,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := initVerdict(tc.checked, tc.flush); !errors.Is(got, tc.want) && got != tc.want {
				t.Errorf("initVerdict(%v, %v) = %v, want %v", tc.checked, tc.flush, got, tc.want)
			}
		})
	}
}

// TestSchemaIsCurrentNeedsAllThreeAnswers is finding F14's third survivor.
//
// `schema_current` in the init report is a claim that the runtime schema was
// actually established, and its own comment says all three conditions are
// needed. Only two of them were pinned: dropping the pending-count arm let init
// report a current schema over a database with migrations still to apply, and
// the whole suite stayed green because a successful ModeInit run applies
// everything and never leaves that state behind.
func TestSchemaIsCurrentNeedsAllThreeAnswers(t *testing.T) {
	failure := errors.New("the ledger could not be read")

	tests := []struct {
		name    string
		subject doctor.Subject
		want    bool
	}{
		{
			name:    "a database that is present, readable and fully migrated",
			subject: doctor.Subject{DBPresent: true},
			want:    true,
		},
		{
			name:    "no database at all",
			subject: doctor.Subject{},
			want:    false,
		},
		{
			name:    "a ledger that could not be read",
			subject: doctor.Subject{DBPresent: true, MigrateErr: failure},
			want:    false,
		},
		{
			name:    "migrations this binary knows and has not applied",
			subject: doctor.Subject{DBPresent: true, PendingCount: 1},
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := schemaIsCurrent(tc.subject); got != tc.want {
				t.Errorf("schemaIsCurrent(%+v) = %v, want %v", tc.subject, got, tc.want)
			}
		})
	}
}
