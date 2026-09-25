package perf_test

import (
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/perf"
)

// manualClock is the deterministic clock AC-01.4 demands: every Now
// answers from a script the test advances by hand.
type manualClock struct {
	now time.Time
}

func (c *manualClock) Now() time.Time { return c.now }

func (c *manualClock) advance(d time.Duration) { c.now = c.now.Add(d) }

// TestTimerCheckpointsAccumulate pins the breakdown contract: checkpoints
// bank elapsed slices under their names and Total spans the whole run.
func TestTimerCheckpointsAccumulate(t *testing.T) {
	clock := &manualClock{now: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)}
	timer := perf.Start(clock)
	clock.advance(10 * time.Millisecond)
	timer.Checkpoint(perf.SQLiteWait)
	clock.advance(20 * time.Millisecond)
	timer.Checkpoint(perf.Parse)
	clock.advance(5 * time.Millisecond)
	timer.Checkpoint(perf.SQLiteWait)

	sample := timer.Stop()
	if got := sample.Breakdowns[perf.SQLiteWait]; got != 15*time.Millisecond {
		t.Fatalf("sqlite_wait = %v, want 15ms", got)
	}
	if got := sample.Breakdowns[perf.Parse]; got != 20*time.Millisecond {
		t.Fatalf("parse = %v, want 20ms", got)
	}
	if got := sample.Total(); got != 35*time.Millisecond {
		t.Fatalf("total = %v, want 35ms", got)
	}
}

// TestPercentileNearestRank pins the statistics: exact ranks on a known
// spread, clamping past the edges, zero on empty.
func TestPercentileNearestRank(t *testing.T) {
	recorder := perf.NewRecorder("status")
	if got := recorder.Percentile(95); got != 0 {
		t.Fatalf("empty p95 = %v, want 0", got)
	}
	for _, d := range []time.Duration{10, 20, 30, 40, 50, 60, 70, 80, 90, 100} {
		recorder.AddDuration(d * time.Millisecond)
	}
	if got := recorder.Percentile(50); got != 50*time.Millisecond {
		t.Fatalf("p50 = %v, want 50ms", got)
	}
	if got := recorder.Percentile(95); got != 100*time.Millisecond {
		t.Fatalf("p95 = %v, want 100ms (nearest-rank of 10)", got)
	}
	if got := recorder.Percentile(0); got != 10*time.Millisecond {
		t.Fatalf("p0 = %v, want 10ms", got)
	}
	if got := recorder.Percentile(100); got != 100*time.Millisecond {
		t.Fatalf("p100 = %v, want 100ms", got)
	}
}

// TestGradeFailsClosed pins the grading contract: unknown operations and
// empty records fail, under-target passes, over-target fails.
func TestGradeFailsClosed(t *testing.T) {
	unknown := perf.NewRecorder("no_such_op")
	unknown.AddDuration(time.Millisecond)
	if verdict := unknown.Grade(); verdict.Pass {
		t.Fatal("unknown operation graded pass")
	}
	empty := perf.NewRecorder("status")
	if verdict := empty.Grade(); verdict.Pass {
		t.Fatal("empty record graded pass")
	}
	over := perf.NewRecorder("status")
	over.AddDuration(10 * time.Second)
	if verdict := over.Grade(); verdict.Pass {
		t.Fatal("10s status graded pass against 150ms")
	}
	under := perf.NewRecorder("status")
	under.AddDuration(time.Millisecond)
	verdict := under.Grade()
	if !verdict.Pass || verdict.Target != 150*time.Millisecond {
		t.Fatalf("verdict = %+v, want pass at 150ms", verdict)
	}
}

// TestTimerUsesAppClock pins the seam: production passes SystemClock,
// tests pass FixedClock — the timer itself is clock-agnostic, and Total
// is always banked under either.
func TestTimerUsesAppClock(t *testing.T) {
	fixed := app.FixedClock{Instant: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)}
	if _, ok := perf.Start(fixed).Stop().Breakdowns[perf.Total]; !ok {
		t.Fatal("fixed-clock sample banks no total")
	}
	system := perf.Start(app.SystemClock{})
	if _, ok := system.Stop().Breakdowns[perf.Total]; !ok {
		t.Fatal("system-clock sample banks no total")
	}
}
