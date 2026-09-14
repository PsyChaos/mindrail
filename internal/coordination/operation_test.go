package coordination_test

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
)

func countRows(t *testing.T, f fixture, table string) int {
	t.Helper()
	var n int
	if err := f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatalf("count %s = %v", table, err)
	}
	return n
}

// idempotentWriter is one of the seven writers, driven twice under one id so
// the table below can be one loop: run performs the write under the bound
// store and returns the writer's own result and its Write.
type idempotentWriter struct {
	name    string
	table   string // the entity table the write inserts or updates
	inserts bool   // whether a fresh run adds a row to table
	run     func(t *testing.T, s *coordination.Store, f fixture, me coordination.Session, task coordination.Task, lease coordination.Lease) (any, coordination.Write, error)
}

var writers = []idempotentWriter{
	{"session open", "sessions", true, func(t *testing.T, s *coordination.Store, f fixture, _ coordination.Session, _ coordination.Task, _ coordination.Lease) (any, coordination.Write, error) {
		return s.OpenSession(t.Context(), f.spaceID, "a label")
	}},
	{"task open", "tasks", true, func(t *testing.T, s *coordination.Store, f fixture, me coordination.Session, _ coordination.Task, _ coordination.Lease) (any, coordination.Write, error) {
		return s.OpenTask(t.Context(), f.projectID, coordination.NamedSession(me.ID), "an idempotent task")
	}},
	{"task state", "tasks", false, func(t *testing.T, s *coordination.Store, f fixture, me coordination.Session, task coordination.Task, _ coordination.Lease) (any, coordination.Write, error) {
		return s.Transition(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateClaimed, "")
	}},
	{"checkpoint write", "checkpoints", true, func(t *testing.T, s *coordination.Store, f fixture, me coordination.Session, task coordination.Task, _ coordination.Lease) (any, coordination.Write, error) {
		return s.WriteCheckpoint(t.Context(), task.ID, coordination.NamedSession(me.ID), f.spaceID, "a note", false)
	}},
	{"lease acquire", "leases", true, func(t *testing.T, s *coordination.Store, f fixture, me coordination.Session, _ coordination.Task, _ coordination.Lease) (any, coordination.Write, error) {
		return s.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(me.ID), mustFile(t, "src/idempotent.go"))
	}},
	{"lease renew", "leases", false, func(t *testing.T, s *coordination.Store, f fixture, me coordination.Session, _ coordination.Task, lease coordination.Lease) (any, coordination.Write, error) {
		return s.RenewLease(t.Context(), lease.ID, coordination.NamedSession(me.ID))
	}},
	{"lease release", "leases", false, func(t *testing.T, s *coordination.Store, f fixture, me coordination.Session, _ coordination.Task, lease coordination.Lease) (any, coordination.Write, error) {
		return s.ReleaseLease(t.Context(), lease.ID, coordination.NamedSession(me.ID))
	}},
}

// arrange builds what a writer needs: a session, an OPEN task, and a lease
// the session holds on another file.
func arrange(t *testing.T, f fixture) (coordination.Session, coordination.Task, coordination.Lease) {
	t.Helper()
	me := f.session(t)
	task := f.task(t, me.ID, "a task for the writers")
	held, _, err := f.store.AcquireLease(t.Context(), f.projectID, coordination.NamedSession(me.ID), mustFile(t, "src/held.go"))
	if err != nil {
		t.Fatal(err)
	}
	return me, task, held.Lease
}

// TestWithoutAnOperationEveryWriterRecordsNothing is AC-06.1: the zero
// binding runs as before, and the operations table stays empty.
func TestWithoutAnOperationEveryWriterRecordsNothing(t *testing.T) {
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			f, _ := leaseFixture(t)
			me, task, lease := arrange(t, f)
			if _, write, err := w.run(t, f.store, f, me, task, lease); err != nil {
				t.Fatalf("%s = %v", w.name, err)
			} else if write.Replayed || write.OperationID != "" {
				t.Errorf("%s reports %+v under no operation", w.name, write)
			}
			if n := countRows(t, f, "operations"); n != 0 {
				t.Errorf("%s recorded %d operation row(s) under no operation", w.name, n)
			}
		})
	}
}

// TestTheSameOperationTwiceIsOneWriteAndOneRecord is AC-06.2 over every
// writer: one entity row, one operation row, and the second answer is the
// first's marked replayed.
func TestTheSameOperationTwiceIsOneWriteAndOneRecord(t *testing.T) {
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			f, clock := leaseFixture(t)
			me, task, lease := arrange(t, f)
			before := countRows(t, f, w.table)

			bound := f.store.Idempotent("op-" + strings.ReplaceAll(w.name, " ", "-"))
			first, firstWrite, err := w.run(t, bound, f, me, task, lease)
			if err != nil {
				t.Fatalf("first %s = %v", w.name, err)
			}
			if firstWrite.Replayed || firstWrite.OperationID == "" {
				t.Errorf("first write = %+v, want not replayed and carrying the operation id", firstWrite)
			}
			afterFirst := countRows(t, f, w.table)
			if w.inserts && afterFirst != before+1 {
				t.Fatalf("first %s left %d rows in %s, want %d", w.name, afterFirst, w.table, before+1)
			}
			if n := countRows(t, f, "operations"); n != 1 {
				t.Fatalf("after the first %s the operations table holds %d rows, want 1", w.name, n)
			}

			clock.Advance(time.Minute)
			second, secondWrite, err := w.run(t, bound, f, me, task, lease)
			if err != nil {
				t.Fatalf("second %s = %v, want the replay", w.name, err)
			}
			if !secondWrite.Replayed || secondWrite.OperationID != firstWrite.OperationID {
				t.Errorf("second write = %+v, want replayed under the same id", secondWrite)
			}
			if secondWrite.Session.ID != firstWrite.Session.ID || secondWrite.Minted != firstWrite.Minted {
				t.Errorf("second write's attribution %+v differs from the first's %+v", secondWrite.Session, firstWrite.Session)
			}
			if !reflect.DeepEqual(first, second) {
				t.Errorf("the replayed result differs from the first:\n first  %+v\n second %+v", first, second)
			}
			if n := countRows(t, f, w.table); n != afterFirst {
				t.Errorf("the second %s changed %s from %d to %d rows", w.name, w.table, afterFirst, n)
			}
			if n := countRows(t, f, "operations"); n != 1 {
				t.Errorf("after the second %s the operations table holds %d rows, want still 1", w.name, n)
			}
		})
	}
}

// TestTheSameIDForADifferentRequestIsAConflict is AC-06.3: a reused id is
// refused and writes nothing, and a different session handle is a different
// request.
func TestTheSameIDForADifferentRequestIsAConflict(t *testing.T) {
	f, _ := leaseFixture(t)
	me, other := f.session(t), f.session(t)

	bound := f.store.Idempotent("op-1")
	if _, _, err := bound.OpenTask(t.Context(), f.projectID, coordination.NamedSession(me.ID), "the first request"); err != nil {
		t.Fatal(err)
	}
	tasks, ops := countRows(t, f, "tasks"), countRows(t, f, "operations")

	for name, attempt := range map[string]func() error{
		"a different title": func() error {
			_, _, err := bound.OpenTask(t.Context(), f.projectID, coordination.NamedSession(me.ID), "another request")
			return err
		},
		"a different session": func() error {
			_, _, err := bound.OpenTask(t.Context(), f.projectID, coordination.NamedSession(other.ID), "the first request")
			return err
		},
		"a minted session instead of the named one": func() error {
			_, _, err := bound.OpenTask(t.Context(), f.projectID, coordination.MintFor(f.spaceID), "the first request")
			return err
		},
		"a different command": func() error {
			_, _, err := bound.OpenSession(t.Context(), f.spaceID, "the first request")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := attempt()
			payload := requireCode(t, err, app.CodeOperationIDConflict)
			if !errors.Is(err, coordination.ErrOperationConflict) {
				t.Error("errors.Is(err, ErrOperationConflict) = false")
			}
			if payload.Metadata["operation_id"] != "op-1" || payload.Metadata["recorded_command"] != "task open" {
				t.Errorf("metadata = %v, want the id and the recorded command", payload.Metadata)
			}
			if countRows(t, f, "tasks") != tasks || countRows(t, f, "operations") != ops || countRows(t, f, "sessions") != 2 {
				t.Errorf("a refused reuse wrote something: tasks %d ops %d sessions %d", countRows(t, f, "tasks"), countRows(t, f, "operations"), countRows(t, f, "sessions"))
			}
		})
	}
}

// TestARefusedOperationRecordsNothing is AC-06.4: a move refused as a lease
// conflict under an id leaves no record, and the same id retried once the
// lease is released succeeds and records then.
func TestARefusedOperationRecordsNothing(t *testing.T) {
	f, _ := leaseFixture(t)
	holder, me := f.session(t), f.session(t)
	task := f.taskIn(t, holder.ID, coordination.StateInProgress)
	held, _ := activeTaskLease(t, f, task.ID)

	bound := f.store.Idempotent("op-move")
	_, _, err := bound.Transition(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateBlocked, "waiting")
	requireCode(t, err, app.CodeLeaseConflict)
	if n := countRows(t, f, "operations"); n != 0 {
		t.Fatalf("a refused move recorded %d operation row(s); a transient refusal must not be frozen", n)
	}

	if _, _, err := f.store.ReleaseLease(t.Context(), held.ID, coordination.NamedSession(holder.ID)); err != nil {
		t.Fatal(err)
	}
	move, write, err := bound.Transition(t.Context(), task.ID, coordination.NamedSession(me.ID), coordination.StateBlocked, "waiting")
	if err != nil {
		t.Fatalf("the retry after the release = %v, want the move", err)
	}
	if write.Replayed || move.Task.State != coordination.StateBlocked {
		t.Errorf("the retry = %+v / %+v, want a fresh move, not a replay", move.Task, write)
	}
	if n := countRows(t, f, "operations"); n != 1 {
		t.Errorf("after the retry the operations table holds %d rows, want 1", n)
	}
}

// TestTwoDeliveriesOfOneOperationSerialise is AC-06.5: concurrent deliveries
// of one request produce one entity row and one record, under -race.
func TestTwoDeliveriesOfOneOperationSerialise(t *testing.T) {
	f, _ := leaseFixture(t)
	me := f.session(t)

	const deliveries = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		replayed int
		fresh    int
		ids      = map[string]struct{}{}
	)
	for range deliveries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, write, err := f.store.Idempotent("op-concurrent").OpenTask(t.Context(), f.projectID, coordination.NamedSession(me.ID), "delivered many times")
			if err != nil {
				t.Errorf("OpenTask = %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			ids[task.ID] = struct{}{}
			if write.Replayed {
				replayed++
			} else {
				fresh++
			}
		}()
	}
	wg.Wait()

	if fresh != 1 || replayed != deliveries-1 || len(ids) != 1 {
		t.Errorf("fresh = %d, replayed = %d, distinct ids = %d; want 1, %d, 1", fresh, replayed, len(ids), deliveries-1)
	}
	if n := countRows(t, f, "tasks"); n != 1 {
		t.Errorf("tasks = %d, want 1", n)
	}
	if n := countRows(t, f, "operations"); n != 1 {
		t.Errorf("operations = %d, want 1", n)
	}
}

// TestAnOperationIDMustBeAnIdentifier is D-71's grammar at the store, for
// the callers with no command line in front of them.
func TestAnOperationIDMustBeAnIdentifier(t *testing.T) {
	f, _ := leaseFixture(t)
	me := f.session(t)

	for _, ok := range []string{"a", "01J8ZK3Q5M9Y2B7V4C6D8E0F1G", "550e8400-e29b-41d4-a716-446655440000", "run.7:step_3", strings.Repeat("x", 128)} {
		if !coordination.ValidOperationID(ok) {
			t.Errorf("ValidOperationID(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "-leading-dash", ".dot", "has space", "tab\t", "ünïcode", "slash/y", strings.Repeat("x", 129)} {
		if coordination.ValidOperationID(bad) {
			t.Errorf("ValidOperationID(%q) = true, want false", bad)
		}
		_, _, err := f.store.Idempotent(bad).OpenTask(t.Context(), f.projectID, coordination.NamedSession(me.ID), "under a bad id")
		if bad == "" {
			if err != nil {
				t.Errorf("an empty binding is no binding, got %v", err)
			}
			continue
		}
		requireCode(t, err, app.CodeCommandLineInvalid)
	}
	if n := countRows(t, f, "operations"); n != 0 {
		t.Errorf("refused ids recorded %d row(s)", n)
	}
}
