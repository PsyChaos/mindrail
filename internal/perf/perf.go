// Package perf is Mindrail's stopwatch: operation timing with named
// breakdowns, percentile reports against the STRUCTURAL warm-path
// targets, and cooperative budgets for deferral. It observes only —
// the timer cannot fail an operation, and identical inputs still
// produce identical outputs with or without it (decision D-230).
package perf

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// Breakdown names every measured segment. Total is the whole operation;
// the rest partition the work the latency telemetry requires (spec-1.0
// §28): SQLite wait, parse, and impact traversal.
const (
	Total      = "total"
	SQLiteWait = "sqlite_wait"
	Parse      = "parse"
	Traversal  = "traversal"
)

// Timer measures one operation run against an injected clock. Production
// passes app.SystemClock; tests pass app.FixedClock (or a manual clock)
// for deterministic samples — wall-clock assertions never appear in unit
// tests (requirements AC-01.4).
type Timer struct {
	clock app.Clock
	start time.Time
	marks map[string]time.Duration
	last  time.Time
}

// Start begins one measurement.
func Start(clock app.Clock) *Timer {
	now := clock.Now()
	return &Timer{clock: clock, start: now, last: now, marks: map[string]time.Duration{}}
}

// Checkpoint banks the time since the previous checkpoint (or Start)
// under name. Calling it twice under one name accumulates.
func (t *Timer) Checkpoint(name string) {
	now := t.clock.Now()
	t.marks[name] += now.Sub(t.last)
	t.last = now
}

// Stop closes the measurement and returns the sample: Total plus every
// checkpointed breakdown.
func (t *Timer) Stop() Sample {
	now := t.clock.Now()
	out := Sample{Breakdowns: map[string]time.Duration{}}
	for name, spent := range t.marks {
		out.Breakdowns[name] = spent
	}
	out.Breakdowns[Total] = now.Sub(t.start)
	return out
}

// Sample is one measured run: the total beside its named parts.
type Sample struct {
	Breakdowns map[string]time.Duration
}

// Total returns the whole-operation duration, zero when unmeasured.
func (s Sample) Total() time.Duration {
	return s.Breakdowns[Total]
}

// Observe banks one measured segment under name. Nil timers — the common
// case outside benchmarks — sink the observation silently, which is what
// keeps telemetry an observer rather than a participant (decision D-230).
func (t *Timer) Observe(name string, spent time.Duration) {
	if t == nil {
		return
	}
	t.marks[name] += spent
}

// timerKey carries the ambient timer through context.Context, so services
// observe without signature changes: handlers and benchmarks attach,
// layers read. No globals, no goroutine leaks — the timer dies with the
// request that carried it.
type timerKey struct{}

// WithTimer attaches a measurement to the context.
func WithTimer(ctx context.Context, timer *Timer) context.Context {
	return context.WithValue(ctx, timerKey{}, timer)
}

// Observe banks spent under name on the context's timer, if any.
func Observe(ctx context.Context, name string, spent time.Duration) {
	if timer, ok := ctx.Value(timerKey{}).(*Timer); ok {
		timer.Observe(name, spent)
	}
}

// Span starts one named segment measured with the wall clock and returns
// its stop: `stop := perf.Span(ctx, perf.Traversal); defer stop()`. The
// duration is observed, never asserted in unit tests.
func Span(ctx context.Context, name string) func() {
	start := time.Now()
	return func() { Observe(ctx, name, time.Since(start)) }
}

// Bench runs fn b.N times under a fresh ambient timer each iteration,
// grades p95 against the STRUCTURAL target, and fails the benchmark when
// the target is missed (decision D-229). One warmup call runs before
// measurement so iteration 1 does not price cold caches. Breakdowns
// observed on the path (parse, sqlite-wait, traversal) ride the timer
// into the log line; phases a path never touches stay absent rather than
// zero-filled.
//
// The timer travels in the context the benchmark hands to fn. Transports
// that do not propagate context values (the MCP wire, including the
// in-memory transport) receive totals only — breakdowns are measured at
// the service layer, where the context arrives intact.
func Bench(b *testing.B, op string, fn func(ctx context.Context)) {
	b.Helper()
	fn(context.Background()) // warmup: cold caches are not the warm path
	recorder := NewRecorder(op)
	b.ResetTimer()
	for range b.N {
		timer := Start(app.SystemClock{})
		fn(WithTimer(context.Background(), timer))
		recorder.Add(timer)
	}
	b.StopTimer()
	verdict := recorder.Grade()
	var parts []string
	for _, part := range []string{SQLiteWait, Parse, Traversal} {
		if p95 := recorder.PartPercentile(part, 95); p95 > 0 {
			parts = append(parts, part+"="+p95.String())
		}
	}
	line := fmt.Sprintf("%s p50=%s p95=%s target=%s samples=%d",
		verdict.Operation, verdict.P50, verdict.P95, verdict.Target, verdict.Samples)
	if len(parts) > 0 {
		line += " [" + strings.Join(parts, ", ") + "]"
	}
	line += fmt.Sprintf(" pass=%v", verdict.Pass)
	b.Log(line)
	if !verdict.Pass {
		b.Fatalf("%s p95 %s exceeds target %s over %d samples", op, verdict.P95, verdict.Target, verdict.Samples)
	}
}

// Recorder accumulates samples for one operation.
type Recorder struct {
	name    string
	samples []time.Duration
	parts   map[string][]time.Duration
}

// NewRecorder opens one operation's record.
func NewRecorder(name string) *Recorder {
	return &Recorder{name: name, parts: map[string][]time.Duration{}}
}

// Add records one finished timer.
func (r *Recorder) Add(timer *Timer) {
	sample := timer.Stop()
	r.samples = append(r.samples, sample.Total())
	for name, spent := range sample.Breakdowns {
		if name == Total {
			continue
		}
		r.parts[name] = append(r.parts[name], spent)
	}
}

// AddDuration records one raw duration (for benchmarks measuring
// externally).
func (r *Recorder) AddDuration(d time.Duration) {
	r.samples = append(r.samples, d)
}

// Name is the measured operation.
func (r *Recorder) Name() string { return r.name }

// Count is the recorded sample count.
func (r *Recorder) Count() int { return len(r.samples) }

// Percentile returns the p-th percentile sample (p in 0..100) by the
// nearest-rank method. Empty records report zero, never panic.
func (r *Recorder) Percentile(p float64) time.Duration {
	return percentileOf(r.samples, p)
}

// PartPercentile reports one breakdown's percentile, zero when the part
// was never checkpointed.
func (r *Recorder) PartPercentile(part string, p float64) time.Duration {
	return percentileOf(r.parts[part], p)
}

func percentileOf(samples []time.Duration, p float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	rank := int((p / 100) * float64(len(sorted)))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// Targets is the STRUCTURAL warm-path table (kernel-scope §5, spec-1.0
// §28). FULL-semantic targets do not exist in 0.1 — no resolvers, no
// targets. Recalibration lands as a documented decision with numbers,
// never a quiet edit.
var Targets = map[string]time.Duration{
	"status":        150 * time.Millisecond,
	"context":       250 * time.Millisecond,
	"before_change": 300 * time.Millisecond,
	"after_change":  1500 * time.Millisecond,
	"reconcile":     2000 * time.Millisecond,
}

// Verdict grades one operation's p95 against its target.
type Verdict struct {
	Operation string
	Samples   int
	P50       time.Duration
	P95       time.Duration
	Target    time.Duration
	Pass      bool
}

// Grade computes the verdict; unknown operations fail closed (a target
// that cannot be named cannot be claimed).
func (r *Recorder) Grade() Verdict {
	target, ok := Targets[r.name]
	verdict := Verdict{
		Operation: r.name,
		Samples:   len(r.samples),
		P50:       r.Percentile(50),
		P95:       r.Percentile(95),
		Target:    target,
	}
	verdict.Pass = ok && len(r.samples) > 0 && verdict.P95 <= target
	return verdict
}

// Report renders the verdicts human-readably, one line per operation,
// with breakdown p95s where checkpointed.
func Report(recorders []*Recorder) string {
	var b strings.Builder
	for _, recorder := range recorders {
		verdict := recorder.Grade()
		status := "PASS"
		if !verdict.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "%-13s p50=%-9s p95=%-9s target=%-7s samples=%-4d %s",
			verdict.Operation, verdict.P50, verdict.P95, verdict.Target, verdict.Samples, status)
		var parts []string
		for _, part := range []string{SQLiteWait, Parse, Traversal} {
			if p95 := recorder.PartPercentile(part, 95); p95 > 0 {
				parts = append(parts, fmt.Sprintf("%s p95=%s", part, p95))
			}
		}
		if len(parts) > 0 {
			b.WriteString(" [" + strings.Join(parts, ", ") + "]")
		}
		b.WriteString("\n")
	}
	return b.String()
}
