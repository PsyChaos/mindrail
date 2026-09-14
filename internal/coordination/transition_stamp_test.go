package coordination_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// lockProbingClock answers the question "did the store hold the write lock
// when it read the clock?" from inside the reading itself.
//
// Every Now tries to begin a write transaction over the same file through a
// second, independent handle whose busy timeout is one millisecond. If the
// store is inside its BEGIN IMMEDIATE, that attempt is refused; if the store
// read the clock before its transaction, the attempt succeeds and is rolled
// back. Nothing sleeps and nothing races: the probe runs on the calling
// goroutine, between the store's own statements.
type lockProbingClock struct {
	probe *storage.DB
	mu    sync.Mutex
	next  time.Time
	held  []bool // one entry per reading: whether a second writer was refused
}

func (c *lockProbingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	tx, err := c.probe.BeginTx(ctx, nil)
	if err == nil {
		_ = tx.Rollback()
	}
	c.held = append(c.held, err != nil)

	now := c.next
	c.next = c.next.Add(time.Minute)
	return now.UTC()
}

// TestAMoveIsStampedUnderTheWriteLock is from the verification pass after the
// round-2 remediation.
//
// updated_at replaces an earlier value on the same row, so it has to follow
// commit order. Item 9b moved Transition's clock reading before its
// transaction to pass one instant to attribute — and a move that then waited
// on the lock behind another writer committed second carrying the earlier
// stamp: the row's updated_at went backwards. The inserting writers stamp new
// rows, and a reading before BEGIN is harmless there; a move must read the
// clock after it holds the lock, and that is asserted here from inside the
// reading rather than by racing two processes.
func TestAMoveIsStampedUnderTheWriteLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	f := openFixture(t, path, app.FixedClock{Instant: baseInstant})

	// A second handle over the same file, with the shortest busy budget the
	// option accepts, so a held lock answers in a millisecond rather than five
	// seconds.
	probe, err := storage.Open(t.Context(), storage.Options{Path: path, BusyTimeout: time.Millisecond})
	if err != nil {
		t.Fatalf("opening the probe handle: %v", err)
	}
	t.Cleanup(func() { _ = probe.Close() })

	// The probe is proved live before it is trusted: on an idle database it
	// must be able to begin, or a probe that always failed would satisfy the
	// assertion below for the wrong reason.
	if tx, err := probe.BeginTx(t.Context(), nil); err != nil {
		t.Fatalf("the probe cannot begin a transaction on an idle database: %v", err)
	} else {
		_ = tx.Rollback()
	}

	clock := &lockProbingClock{probe: probe, next: baseInstant}
	store := coordination.NewStore(f.db.DB, clock)

	session := f.session(t)
	task := f.task(t, session.ID, "a task whose moves are stamped in commit order")

	if _, _, err := store.Transition(t.Context(), task.ID, coordination.NamedSession(session.ID), coordination.StateClaimed, ""); err != nil {
		t.Fatalf("Transition = %v, want no error", err)
	}

	if len(clock.held) != 1 {
		t.Fatalf("Transition read the clock %d time(s), want exactly once", len(clock.held))
	}
	if !clock.held[0] {
		t.Errorf("Transition read the clock without holding the write lock; a move that waits on the lock would carry a stamp earlier than the move it overtakes")
	}
}
