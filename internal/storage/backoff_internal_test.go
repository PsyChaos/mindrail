package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestTheLadderStepsByTheSpecsExample is AC-01.1: spec §11's 25/50/100/200/400
// before jitter, holding at the cap after, and every jittered step in the upper
// half of its own step.
func TestTheLadderStepsByTheSpecsExample(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }

	top := Backoff{Base: ms(25), Cap: ms(400), Rand: func() float64 { return 0 }}
	// Rand returning 1.0 is excluded from [0, 1), so the top of the range is
	// approached, not reached: 0.999… × half is one nanosecond short of half.
	wantTop := []time.Duration{ms(25), ms(50), ms(100), ms(200), ms(400), ms(400), ms(400), ms(400), ms(400), ms(400)}
	for attempt, want := range wantTop {
		if got := top.Step(attempt); got != want/2 {
			t.Errorf("Step(%d) with Rand=0 = %v, want the lower bound %v", attempt, got, want/2)
		}
	}

	almostOne := Backoff{Base: ms(25), Cap: ms(400), Rand: func() float64 { return 0.999999 }}
	for attempt, want := range wantTop {
		got := almostOne.Step(attempt)
		if got <= want/2 || got > want {
			t.Errorf("Step(%d) with Rand≈1 = %v, want in (%v, %v]", attempt, got, want/2, want)
		}
	}

	// The production value carries the same numbers, so the assertions above
	// are about what ships and not about a fixture.
	if DefaultBackoff.Base != ms(25) || DefaultBackoff.Cap != ms(400) {
		t.Errorf("DefaultBackoff = %+v, want base 25ms and cap 400ms", DefaultBackoff)
	}
	if got := DefaultBackoff.Step(0); got < ms(25)/2 || got >= ms(25) {
		t.Errorf("DefaultBackoff.Step(0) = %v, want in [12.5ms, 25ms)", got)
	}
}

// virtualClock lets waitBusy be driven through a whole budget without any real
// time passing: every sleep advances it by exactly the requested amount, and
// the deadline is read from it.
//
// The sleep refuses after maxVirtualSleeps. A loop that lost its deadline —
// the first mutation run against this file did exactly that — would otherwise
// sleep in zero time forever, and a fake that records each sleep grows without
// bound until the kernel kills the test binary and whatever shares its memory.
// Refusing turns that into an ordinary red: the caller returns, and the
// assertion on the sleep count fails with the list in hand.
type virtualClock struct {
	now    time.Time
	sleeps []time.Duration
}

// maxVirtualSleeps is far above any ladder a five-second budget can run: at the
// 400 ms cap the budget is twelve steps long.
const maxVirtualSleeps = 64

func (c *virtualClock) sleep(_ context.Context, d time.Duration) error {
	if len(c.sleeps) >= maxVirtualSleeps {
		return errors.New("virtual clock: more sleeps than any budget allows; the wait loop has lost its deadline")
	}
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
	return nil
}

func (c *virtualClock) retrier(rand float64) retrier {
	return retrier{
		ladder: Backoff{Base: 25 * time.Millisecond, Cap: 400 * time.Millisecond, Rand: func() float64 { return rand }},
		sleep:  c.sleep,
		now:    func() time.Time { return c.now },
	}
}

// genuineBusyError is a SQLITE_BUSY off the driver, provoked by contention
// between two handles on one file; the classifiers this package runs on the
// error are decided on the driver's result code, so a hand-built error would
// prove nothing about them.
func genuineBusyError(t *testing.T) error {
	t.Helper()

	path := filepath.Join(t.TempDir(), "mindrail.db")
	holder, err := Open(t.Context(), Options{Path: path, BusyTimeout: 10 * time.Millisecond, MaxOpenConns: 1})
	if err != nil {
		t.Fatalf("Open(holder) = %v, want no error", err)
	}
	t.Cleanup(func() { _ = holder.Close() })

	tx, err := holder.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("BeginTx = %v, want no error", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	loser, err := Open(t.Context(), Options{Path: path, BusyTimeout: 10 * time.Millisecond, MaxOpenConns: 1})
	if err != nil {
		t.Fatalf("Open(loser) = %v, want no error", err)
	}
	t.Cleanup(func() { _ = loser.Close() })

	err = InTx(t.Context(), loser.DB, func(context.Context, *sql.Tx) error { return nil })
	if err == nil {
		t.Fatal("InTx against a held write lock = nil, want SQLITE_BUSY")
	}
	if !isBusyError(err) {
		t.Fatalf("isBusyError(%v) = false, want true", err)
	}
	return err
}

// TestWaitBusySleepsByTheLadderAndStopsAtTheBudget is AC-01.2's loop: refused
// every time, the wait climbs the ladder, the last sleep is cut to the deadline,
// and no attempt is made after it.
func TestWaitBusySleepsByTheLadderAndStopsAtTheBudget(t *testing.T) {
	busy := genuineBusyError(t)
	clock := &virtualClock{now: time.Unix(1_700_000_000, 0)}

	attempts := 0
	waited, err := waitBusy(t.Context(), 200*time.Millisecond, clock.retrier(0), func() error {
		attempts++
		return busy
	})

	if !errors.Is(err, busy) {
		t.Fatalf("waitBusy = %v, want the last refusal returned as it came", err)
	}
	// Rand=0 puts every step at its lower bound: 12.5, 25, 50, 100 ms, then the
	// 200 ms step is cut to the 12.5 ms left before the deadline.
	ms := func(f float64) time.Duration { return time.Duration(f * float64(time.Millisecond)) }
	wantSleeps := []time.Duration{ms(12.5), ms(25), ms(50), ms(100), ms(12.5)}
	if len(clock.sleeps) != len(wantSleeps) {
		t.Fatalf("sleeps = %v, want %v", clock.sleeps, wantSleeps)
	}
	for i, want := range wantSleeps {
		if clock.sleeps[i] != want {
			t.Errorf("sleep %d = %v, want %v", i, clock.sleeps[i], want)
		}
	}
	if attempts != len(wantSleeps)+1 {
		t.Errorf("attempts = %d, want one per sleep plus the first", attempts)
	}
	if waited != 200*time.Millisecond {
		t.Errorf("waited = %v, want exactly the budget", waited)
	}
}

// TestWaitBusyReturnsSuccessAndPermanentFailuresAtOnce is the other arm: the
// ladder is for contention only.
func TestWaitBusyReturnsSuccessAndPermanentFailuresAtOnce(t *testing.T) {
	busy := genuineBusyError(t)

	t.Run("a success after two refusals ends the wait", func(t *testing.T) {
		clock := &virtualClock{now: time.Unix(1_700_000_000, 0)}
		refusals := 2
		_, err := waitBusy(t.Context(), time.Second, clock.retrier(0), func() error {
			if refusals > 0 {
				refusals--
				return busy
			}
			return nil
		})
		if err != nil {
			t.Fatalf("waitBusy = %v, want no error", err)
		}
		if len(clock.sleeps) != 2 {
			t.Errorf("sleeps = %v, want exactly two", clock.sleeps)
		}
	})

	t.Run("a failure that is not contention is returned without a sleep", func(t *testing.T) {
		clock := &virtualClock{now: time.Unix(1_700_000_000, 0)}
		permanent := errors.New("not a database")
		_, err := waitBusy(t.Context(), time.Second, clock.retrier(0), func() error { return permanent })
		if !errors.Is(err, permanent) {
			t.Fatalf("waitBusy = %v, want the permanent failure", err)
		}
		if len(clock.sleeps) != 0 {
			t.Errorf("sleeps = %v, want none: the ladder is for SQLITE_BUSY only", clock.sleeps)
		}
	})

	t.Run("a cancelled context ends the wait at the next refusal", func(t *testing.T) {
		clock := &virtualClock{now: time.Unix(1_700_000_000, 0)}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := waitBusy(ctx, time.Second, clock.retrier(0), func() error { return busy })
		if !errors.Is(err, busy) {
			t.Fatalf("waitBusy = %v, want the refusal", err)
		}
		if len(clock.sleeps) != 0 {
			t.Errorf("sleeps = %v, want none after cancellation", clock.sleeps)
		}
	})
}

// TestAnOpenRefusedForTheWholeBudgetIsRetryable is AC-01.3 at the open path:
// the exhaustion is MINDRAIL_BUSY_RETRYABLE, exit class unavailable, carrying
// the path, the locked condition and the wait — and not the generic open
// failure whose remedy sends the user to check a directory that is fine.
func TestAnOpenRefusedForTheWholeBudgetIsRetryable(t *testing.T) {
	busy := genuineBusyError(t)
	clock := &virtualClock{now: time.Unix(1_700_000_000, 0)}
	path := "/repo/.git/mindrail/mindrail.db"

	db, err := waitOpen(t.Context(), path, 200*time.Millisecond, clock.retrier(0),
		func() (*DB, error) { return nil, busy })
	if db != nil || err == nil {
		t.Fatalf("waitOpen = (%v, %v), want (nil, MINDRAIL_BUSY_RETRYABLE)", db, err)
	}
	if !errors.Is(err, ErrBusy) {
		t.Errorf("waitOpen = %v, want errors.Is(err, ErrBusy)", err)
	}
	if !isBusyError(err) {
		t.Errorf("isBusyError(%v) = false, want the driver's error kept in the chain", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("waitOpen = %v, want a domain error", err)
	}
	if payload.Code != app.CodeBusyRetryable {
		t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeBusyRetryable)
	}
	if app.ExitCode(err) != app.ExitUnavailable {
		t.Errorf("ExitCode = %d, want %d", app.ExitCode(err), app.ExitUnavailable)
	}
	if payload.Metadata["path"] != path {
		t.Errorf("metadata.path = %q, want %q", payload.Metadata["path"], path)
	}
	if payload.Metadata["condition"] != "locked" {
		t.Errorf("metadata.condition = %q, want %q", payload.Metadata["condition"], "locked")
	}
	if payload.Metadata["waited_ms"] != "200" {
		t.Errorf("metadata.waited_ms = %q, want %q", payload.Metadata["waited_ms"], "200")
	}
	for _, next := range payload.NextAction {
		if next == "Run `mindrail init` if this repository has not been initialised yet." {
			t.Errorf("next_action %q is the remedy for a missing database, not for a held one", next)
		}
	}
}

// TestABusyOpenIsNamedOnce is TASK-01's Breaker finding on the open path: the
// busy branch of classifyOpenError used to wrap the driver's error in
// openFailure, so the exhaustion waitOpen then built carried that object as
// its cause — one error object naming two codes, RUNTIME_DB_UNAVAILABLE
// inside MINDRAIL_BUSY_RETRYABLE. The branch now hands the driver's error
// through bare, and waitOpen is the one place that names it.
func TestABusyOpenIsNamedOnce(t *testing.T) {
	busy := errors.Unwrap(genuineBusyError(t)) // the bare driver error under InTx's wrapper
	if !isBusyError(busy) {
		t.Fatalf("isBusyError(%v) = false; the fixture lost the driver's error", busy)
	}

	classified := classifyOpenError("/repo/.git/mindrail/mindrail.db", busy)
	if !isBusyError(classified) {
		t.Fatalf("classifyOpenError(busy) = %v, want the driver's error kept so waitOpen can retry on it", classified)
	}
	if _, isDomain := app.PayloadOf(classified); isDomain {
		t.Errorf("classifyOpenError(busy) = %v, want no diagnosis of its own: waitOpen names the exhaustion", classified)
	}

	clock := &virtualClock{now: time.Unix(1_700_000_000, 0)}
	_, err := waitOpen(t.Context(), "/repo/.git/mindrail/mindrail.db", 100*time.Millisecond, clock.retrier(0),
		func() (*DB, error) { return nil, classified })
	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != app.CodeBusyRetryable {
		t.Fatalf("waitOpen = %v, want MINDRAIL_BUSY_RETRYABLE", err)
	}
	if strings.Contains(err.Error(), string(app.CodeRuntimeDBUnavailable)) {
		t.Errorf("the busy open failure names a second code in its own text: %q", err.Error())
	}
	if strings.Contains(payload.Cause, string(app.CodeRuntimeDBUnavailable)) {
		t.Errorf("payload.Cause = %q, names the code this condition no longer carries", payload.Cause)
	}
}

// TestAnOpenThatIsNotContendedIsNotRewritten keeps the exhaustion path off the
// permanent failures: a corrupt file returns as classified, on the first try.
func TestAnOpenThatIsNotContendedIsNotRewritten(t *testing.T) {
	clock := &virtualClock{now: time.Unix(1_700_000_000, 0)}
	corrupt := corruptFailure("/repo/.git/mindrail/mindrail.db", errors.New("file is not a database"))

	attempts := 0
	db, err := waitOpen(t.Context(), "/repo/.git/mindrail/mindrail.db", time.Second, clock.retrier(0),
		func() (*DB, error) { attempts++; return nil, corrupt })
	if db != nil || !errors.Is(err, ErrCorrupt) {
		t.Fatalf("waitOpen = (%v, %v), want (nil, ErrCorrupt)", db, err)
	}
	if attempts != 1 || len(clock.sleeps) != 0 {
		t.Errorf("attempts = %d, sleeps = %v; want one attempt and no sleep", attempts, clock.sleeps)
	}
}
