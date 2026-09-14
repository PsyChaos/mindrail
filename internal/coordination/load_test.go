package coordination_test

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestEightSessionsMakeTwentyFiveMovesEachWithoutStarvingOneAnother is
// AC-10.6: eight sessions, each moving its own task twenty-five times over
// one database and one store, at once. Every write completes, none is
// refused as busy, and the longest a write held the lock is under 250 ms —
// the bound is on `Held`, because `Waited` also counts the pool's queue
// (D-75 as amended). The eight run as goroutines and not processes on
// purpose: the question is how long the store keeps the lock, and the pool
// is part of the answer.
func TestEightSessionsMakeTwentyFiveMovesEachWithoutStarvingOneAnother(t *testing.T) {
	const (
		sessions   = 8
		moves      = 25
		heldBudget = 250 * time.Millisecond
	)
	f, _ := leaseFixture(t)

	type lane struct {
		session coordination.Session
		task    coordination.Task
	}
	lanes := make([]lane, sessions)
	for i := range lanes {
		session := f.session(t)
		lanes[i] = lane{session, f.task(t, session.ID, fmt.Sprintf("lane %d", i))}
	}

	// The route every lane walks: the claim, the start, then blocked and
	// back until the count is reached.
	route := []coordination.State{coordination.StateClaimed, coordination.StateInProgress}
	for len(route) < moves {
		route = append(route, coordination.StateBlocked, coordination.StateInProgress)
	}
	route = route[:moves]

	type outcome struct {
		lane, move int
		timing     storage.TxStats
		err        error
	}
	outcomes := make(chan outcome, sessions*moves)
	var wg sync.WaitGroup
	for i, l := range lanes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m, to := range route {
				reason := ""
				if to == coordination.StateBlocked {
					reason = "waiting on the lane next door"
				}
				_, write, err := f.store.Transition(t.Context(), l.task.ID, coordination.NamedSession(l.session.ID), to, reason)
				outcomes <- outcome{i, m, write.Timing, err}
			}
		}()
	}
	wg.Wait()
	close(outcomes)

	var (
		completed int
		held      []time.Duration
		waited    []time.Duration
	)
	for o := range outcomes {
		if o.err != nil {
			code := app.Code("")
			if payload, ok := app.PayloadOf(o.err); ok {
				code = payload.Code
			}
			t.Errorf("lane %d move %d failed with %s: %v", o.lane, o.move, code, o.err)
			continue
		}
		completed++
		held = append(held, o.timing.Held)
		waited = append(waited, o.timing.Waited)
	}
	if completed != sessions*moves {
		t.Errorf("%d of %d writes completed", completed, sessions*moves)
	}
	if len(held) == 0 {
		t.Fatal("no write completed")
	}
	slices.Sort(held)
	slices.Sort(waited)
	longest := held[len(held)-1]
	if longest >= heldBudget {
		t.Errorf("the longest write held the lock for %v, want under %v", longest, heldBudget)
	}
	t.Logf("%d writes over %d sessions: held median %v, p99 %v, longest %v; waited median %v, longest %v",
		completed, sessions,
		held[len(held)/2].Round(time.Microsecond), held[len(held)*99/100].Round(time.Microsecond), longest.Round(time.Microsecond),
		waited[len(waited)/2].Round(time.Microsecond), waited[len(waited)-1].Round(time.Microsecond))

	// The table after the run: each lane's task is where its route ends, at
	// the revision its moves add up to, claimed by its own session, and one
	// lease per task is active.
	for i, l := range lanes {
		got, err := f.store.FindTask(t.Context(), l.task.ID)
		if err != nil {
			t.Fatalf("lane %d: FindTask = %v", i, err)
		}
		if got.State != route[moves-1] || got.ClaimedBy != l.session.ID || got.Revision != int64(1+moves) {
			t.Errorf("lane %d ended at %s revision %d claimed by %s, want %s revision %d claimed by %s",
				i, got.State, got.Revision, got.ClaimedBy, route[moves-1], 1+moves, l.session.ID)
		}
	}
	active, err := f.store.ListLeases(t.Context(), f.projectID)
	if err != nil {
		t.Fatalf("ListLeases = %v", err)
	}
	if len(active) != sessions {
		t.Errorf("%d active leases after the run, want one per lane", len(active))
	}
}
