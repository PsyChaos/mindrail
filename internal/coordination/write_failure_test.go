package coordination_test

import (
	"os"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestAFailedWriteIsNamedByTheStorageLayerFirst is finding F04, and acceptance
// criterion AC-04.5 turned into an assertion.
//
// (*Store).writeFailure asks storage.WriteFailure to classify the failure and
// falls back to COORDINATION_WRITE_FAILED only when it cannot. Nothing drove
// either arm: replacing the body with a plain writeFailed left `make verify`
// green, and a coverage run reported execution count 0 for every statement in
// it.
//
// The ordering is load-bearing beyond the message. COORDINATION_WRITE_FAILED
// sits in the exit-1 class *because* the storage conditions that really are
// "come back later" are named before it is reached; without the classifier the
// same disk produces exit 1 where it used to produce exit 4, and an
// orchestrator branching on "runtime unavailable" reads it as "the operation
// was refused".
//
// The test lives here rather than in the command layer because the command
// layer no longer reaches it: a repository whose runtime path is unwritable is
// caught by the startup verdict now, which is right, and would make this
// assertion pass over a store that had stopped classifying anything.
func TestAFailedWriteIsNamedByTheStorageLayerFirst(t *testing.T) {
	writers := map[string]func(t *testing.T, store *coordination.Store, f fixture,
		session coordination.Session, task coordination.Task) error{
		"OpenSession": func(t *testing.T, store *coordination.Store, f fixture, _ coordination.Session, _ coordination.Task) error {
			_, err := store.OpenSession(t.Context(), f.spaceID, "")
			return err
		},
		"OpenTask": func(t *testing.T, store *coordination.Store, f fixture, session coordination.Session, _ coordination.Task) error {
			_, _, err := store.OpenTask(t.Context(), f.projectID,
				coordination.NamedSession(session.ID), "a task nothing can write")
			return err
		},
		"Transition": func(t *testing.T, store *coordination.Store, _ fixture, session coordination.Session, task coordination.Task) error {
			_, _, err := store.Transition(t.Context(), task.ID,
				coordination.NamedSession(session.ID), coordination.StateClaimed, "")
			return err
		},
		"WriteCheckpoint": func(t *testing.T, store *coordination.Store, f fixture, session coordination.Session, task coordination.Task) error {
			_, _, err := store.WriteCheckpoint(t.Context(), task.ID,
				coordination.NamedSession(session.ID), f.spaceID, "a note nothing can write", false)
			return err
		},
	}

	for name, write := range writers {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			session := f.session(t)
			task := f.task(t, session.ID, "a task to write against")

			// The other arm first: on a writable database none of these
			// produces either code, so the assertion below is about the
			// permissions and not about the operation.
			//
			// It runs against a task of its own. Sharing one would leave the
			// second call asking for a move the first one had already made,
			// and the lifecycle would refuse it before any write was attempted
			// — a green test that never reached the code under test.
			warmUp := f.task(t, session.ID, "a task the writable arm moves")
			if err := write(t, f.store, f, session, warmUp); err != nil {
				t.Fatalf("%s failed on a writable database: %v", name, err)
			}

			// The handle has to be reopened after the permissions change.
			// SQLite checks them when it opens the file, so a connection that
			// was already open goes on writing through a descriptor the mode
			// bits no longer describe — which would make this test pass over a
			// store that had stopped classifying anything.
			if err := f.db.Close(); err != nil {
				t.Fatalf("closing the writable handle: %v", err)
			}
			refuseWrites(t, f.path)

			db, err := storage.Open(t.Context(), storage.Options{Path: f.path})
			if err != nil {
				t.Fatalf("reopening %s: %v", f.path, err)
			}
			t.Cleanup(func() { _ = db.Close() })
			store := coordination.NewStore(db.DB, newStepClock())

			err = write(t, store, f, session, task)
			if err == nil {
				t.Fatalf("%s succeeded against a database nothing can write", name)
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("%s failed with %v, which carries no payload a caller can branch on", name, err)
			}
			if payload.Code != app.CodeRuntimePathUnwritable {
				t.Errorf("%s reported %q, want %q: the storage layer knows this condition by name, "+
					"and %q is the fallback for the ones it does not",
					name, payload.Code, app.CodeRuntimePathUnwritable, app.CodeCoordinationWriteFailed)
			}
		})
	}
}

// refuseWrites takes write permission off the database file, which is what
// `chmod 444 mindrail.db` leaves behind.
//
// The directory stays writable on purpose. SQLite in WAL mode needs to create
// the -wal and -shm sidecars to open the database at all, so a read-only
// directory turns this into a failure to open — a real condition, but a
// different one, reported by a different layer before any write is attempted.
// The condition under test is a database that opens and then refuses to commit.
func refuseWrites(t *testing.T, dbPath string) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("permission-denied scenarios are unreachable as root")
	}

	restrict(t, dbPath, 0o444)
}

// restrict applies a mode and puts the old one back afterwards, so that
// t.TempDir's cleanup can still remove the tree.
func restrict(t *testing.T, path string, mode os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("stat %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, info.Mode().Perm()) })
}
