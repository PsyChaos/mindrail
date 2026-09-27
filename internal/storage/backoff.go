package storage

import (
	"context"
	"math/rand/v2"
	"time"
)

// Backoff is the jittered ladder spec §11 asks for on SQLITE_BUSY: 25, 50, 100,
// 200, 400 ms and then 400 for every further step, each drawn from the upper
// half of its step so that n processes refused together do not retry together.
//
// It is a value with two durations rather than a table of five, because the
// budget it runs under (busy_timeout, 5 s) is twelve steps long at the cap and
// a table would have to say what comes after its last row anyway.
type Backoff struct {
	Base time.Duration
	Cap  time.Duration

	// Rand returns a value in [0, 1). nil means math/rand/v2, which is what
	// production wants; a test passes a fixed source and reads the steps back.
	Rand func() float64
}

// DefaultBackoff is the ladder every busy wait in this package runs.
var DefaultBackoff = Backoff{Base: 25 * time.Millisecond, Cap: 400 * time.Millisecond}

// Step returns how long to wait after the attempt numbered attempt, counting
// from zero: the first refusal waits a jittered 25 ms, the second 50, and so on
// up to the cap.
func (b Backoff) Step(attempt int) time.Duration {
	step := b.Base
	for i := 0; i < attempt && step < b.Cap; i++ {
		step *= 2
	}
	if step > b.Cap || step <= 0 {
		step = b.Cap
	}

	random := b.Rand
	if random == nil {
		random = rand.Float64
	}

	half := step / 2
	return half + time.Duration(float64(half)*random())
}

// retrier is how a busy wait passes and reads time: the ladder it sleeps by,
// the sleep itself, and the clock the budget is measured on. It is a value so
// the ladder can be asserted step by step against a virtual clock, without the
// test sleeping through it.
type retrier struct {
	ladder Backoff
	sleep  func(context.Context, time.Duration) error
	now    func() time.Time
}

// defaultRetrier is what production waits with.
var defaultRetrier = retrier{ladder: DefaultBackoff, sleep: sleepContext, now: time.Now}

// sleepContext is the production sleep: a timer that a cancelled context cuts
// short, so a Ctrl-C during a contended start does not wait out the budget
// before reporting the same failure.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// waitBusy runs attempt until it succeeds, fails for a reason that is not
// contention, or the budget is spent, sleeping by the ladder between refusals.
// It returns the last error and how long it waited in total.
//
// A busy refusal met after the budget is returned as it came, not rewritten:
// the caller knows what it was trying to do and names the condition (finding
// W1's rule that the layer which detected the failure classifies it). The
// budget is measured from the first attempt, and the last sleep is cut to the
// time left so the final attempt lands on the deadline rather than a step
// beyond it.
func waitBusy(ctx context.Context, budget time.Duration, r retrier, attempt func() error) (time.Duration, error) {
	started := r.now()
	deadline := started.Add(budget)

	for refusals := 0; ; refusals++ {
		err := attempt()
		if err == nil {
			return r.now().Sub(started), nil
		}
		if !isBusyError(err) || ctx.Err() != nil {
			return r.now().Sub(started), err
		}

		wait := min(r.ladder.Step(refusals), deadline.Sub(r.now()))
		if wait <= 0 {
			return r.now().Sub(started), err
		}
		if sleepErr := r.sleep(ctx, wait); sleepErr != nil {
			return r.now().Sub(started), err
		}
	}
}
