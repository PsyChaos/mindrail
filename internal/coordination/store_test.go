package coordination_test

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/workspace"
	"github.com/PsyChaos/mindrail/migrations"
)

// baseInstant sits in a non-UTC zone on purpose: a timestamp that comes back as
// UTC anyway proves the conversion happened rather than that the test host was
// already on UTC.
var baseInstant = time.Date(2026, time.September, 9, 11, 30, 0, 0, time.FixedZone("UTC+3", 3*60*60))

// stepClock advances by a fixed step on every read, so a test can tell "the row
// was rewritten" from "the row was left alone" without sleeping.
type stepClock struct {
	mu   sync.Mutex
	next time.Time
}

func newStepClock() *stepClock { return &stepClock{next: baseInstant} }

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.next
	c.next = c.next.Add(time.Minute)
	return now.UTC()
}

// fixture is a migrated database with one registered project and workspace,
// which is what the foreign keys on every coordination table require.
type fixture struct {
	store     *coordination.Store
	db        *storage.DB
	path      string
	projectID string
	spaceID   string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return openFixture(t, filepath.Join(t.TempDir(), "mindrail.db"), newStepClock())
}

// openFixture opens the database at path, migrating it and registering the
// worktree if that has not happened yet. Taking the path lets a test close the
// handle and open a second one over the same file, which is what "survives a
// restart" means.
func openFixture(t *testing.T, path string, clock app.Clock) fixture {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatalf("storage.Open = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatalf("migration.Load = %v, want no error", err)
	}
	if _, err := migration.New(db.DB, set, app.FixedClock{Instant: baseInstant}).Up(t.Context()); err != nil {
		t.Fatalf("migration Up = %v, want no error", err)
	}

	spaces := workspace.NewStore(db.DB, app.FixedClock{Instant: baseInstant})
	project, space, err := spaces.Register(t.Context(), workspace.Registration{
		CommonDir:    "/repo/.git",
		WorktreeRoot: "/repo",
		GitDir:       "/repo/.git",
	})
	if err != nil {
		t.Fatalf("Register = %v, want no error", err)
	}

	return fixture{
		store:     coordination.NewStore(db.DB, clock),
		db:        db,
		path:      path,
		projectID: project.ID,
		spaceID:   space.ID,
	}
}

// session mints a session for the fixture's workspace.
func (f fixture) session(t *testing.T) coordination.Session {
	t.Helper()

	session, err := f.store.OpenSession(t.Context(), f.spaceID, "")
	if err != nil {
		t.Fatalf("OpenSession = %v, want no error", err)
	}
	return session
}

// task opens a task in OPEN.
func (f fixture) task(t *testing.T, sessionID, title string) coordination.Task {
	t.Helper()

	task, err := f.store.OpenTask(t.Context(), f.projectID, sessionID, title)
	if err != nil {
		t.Fatalf("OpenTask = %v, want no error", err)
	}
	return task
}

// move performs one transition and fails the test if it is refused.
func (f fixture) move(t *testing.T, taskID, sessionID string, to coordination.State, reason string) coordination.Task {
	t.Helper()

	task, err := f.store.Transition(t.Context(), taskID, sessionID, to, reason)
	if err != nil {
		t.Fatalf("Transition(%s -> %s) = %v, want no error", taskID, to, err)
	}
	return task
}

// pathTo is the shortest walk from OPEN to each state, written out rather than
// searched for. A route computed from the transition table would be a second
// reader of the thing under test; these are the routes a person would take.
var pathTo = map[coordination.State][]coordination.State{
	coordination.StateOpen:            {},
	coordination.StateClaimed:         {coordination.StateClaimed},
	coordination.StateInProgress:      {coordination.StateClaimed, coordination.StateInProgress},
	coordination.StateBlocked:         {coordination.StateClaimed, coordination.StateInProgress, coordination.StateBlocked},
	coordination.StateReadyToComplete: {coordination.StateClaimed, coordination.StateInProgress, coordination.StateReadyToComplete},
	coordination.StateCompleted: {
		coordination.StateClaimed, coordination.StateInProgress,
		coordination.StateReadyToComplete, coordination.StateCompleted,
	},
	coordination.StateAbandoned: {coordination.StateAbandoned},
}

// taskIn returns a fresh task already in the given state.
func (f fixture) taskIn(t *testing.T, sessionID string, state coordination.State) coordination.Task {
	t.Helper()

	route, known := pathTo[state]
	if !known {
		t.Fatalf("no route to %s is written down", state)
	}

	task := f.task(t, sessionID, "a task on its way to "+string(state))
	for _, step := range route {
		reason := ""
		if step == coordination.StateBlocked {
			reason = "waiting on the route"
		}
		task = f.move(t, task.ID, sessionID, step, reason)
	}
	if task.State != state {
		t.Fatalf("the route to %s left the task in %s", state, task.State)
	}
	return task
}

// TestATaskAndItsCheckpointSurviveAReopenedDatabase is AC-04.2, the restart
// criterion, at the layer that owns the rows.
//
// The assertion is made on a second Store over a second handle to the same file,
// because the first Store's return value would be satisfied by a struct that
// never reached SQLite at all. Everything is compared, not just the id: a
// restart that lost the title, the claimant or the block reason would lose
// exactly what the arriving agent came for.
func TestATaskAndItsCheckpointSurviveAReopenedDatabase(t *testing.T) {
	first := newFixture(t)
	writer := first.session(t)

	task := first.taskIn(t, writer.ID, coordination.StateBlocked)
	checkpoint, err := first.store.WriteCheckpoint(t.Context(), task.ID, writer.ID, first.spaceID,
		"stopped waiting for the upstream fix", true)
	if err != nil {
		t.Fatalf("WriteCheckpoint = %v, want no error", err)
	}
	if err := first.db.Close(); err != nil {
		t.Fatalf("closing the database: %v", err)
	}

	second := openFixture(t, first.path, newStepClock())
	handover, err := second.store.Handover(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("Handover after reopening = %v, want no error", err)
	}

	got := handover.Task
	switch {
	case got.ID != task.ID:
		t.Errorf("task id = %q, want %q", got.ID, task.ID)
	case got.Title != task.Title:
		t.Errorf("title = %q, want %q", got.Title, task.Title)
	case got.State != coordination.StateBlocked:
		t.Errorf("state = %s, want %s", got.State, coordination.StateBlocked)
	case got.BlockedReason != task.BlockedReason:
		t.Errorf("blocked_reason = %q, want %q", got.BlockedReason, task.BlockedReason)
	case got.ClaimedBy != writer.ID:
		t.Errorf("claimed_by = %q, want the session that claimed it, %q", got.ClaimedBy, writer.ID)
	case got.OpenedBy != writer.ID:
		t.Errorf("opened_by = %q, want %q", got.OpenedBy, writer.ID)
	case !got.CreatedAt.Equal(task.CreatedAt):
		t.Errorf("created_at = %v, want %v", got.CreatedAt, task.CreatedAt)
	}

	if handover.Checkpoint == nil {
		t.Fatal("the reopened database reports no checkpoint; the handover note is what the next agent came for")
	}
	if handover.Checkpoint.Note != checkpoint.Note {
		t.Errorf("note = %q, want %q", handover.Checkpoint.Note, checkpoint.Note)
	}
	if handover.Checkpoint.SessionID != writer.ID {
		t.Errorf("the checkpoint is attributed to %q, want %q", handover.Checkpoint.SessionID, writer.ID)
	}
	if !handover.Checkpoint.Handoff {
		t.Error("handoff = false, want the flag the writer set")
	}
}

// TestASecondSessionContinuesTheFirstsTask is the milestone's scenario at this
// layer, and decision D-58's other half.
//
// The second session is a different id and it moves a task the first one
// claimed. That is the handover working rather than a conflict: MR-003 records
// who acted and refuses nobody on the grounds of identity, and MR-004 is where a
// lease makes ownership enforceable once two agents can run at once.
func TestASecondSessionContinuesTheFirstsTask(t *testing.T) {
	f := newFixture(t)
	first := f.session(t)
	task := f.taskIn(t, first.ID, coordination.StateInProgress)
	if _, err := f.store.WriteCheckpoint(t.Context(), task.ID, first.ID, f.spaceID, "handing over", true); err != nil {
		t.Fatalf("WriteCheckpoint = %v, want no error", err)
	}

	second := f.session(t)
	if second.ID == first.ID {
		t.Fatal("the second session reused the first one's id; a session is minted, never resumed")
	}

	handover, err := f.store.Handover(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("Handover = %v, want no error", err)
	}
	if handover.Checkpoint == nil || handover.Checkpoint.SessionID != first.ID {
		t.Fatalf("the arriving session cannot see the first session's note: %+v", handover.Checkpoint)
	}

	moved := f.move(t, task.ID, second.ID, coordination.StateReadyToComplete, "")
	if moved.State != coordination.StateReadyToComplete {
		t.Errorf("state = %s, want %s", moved.State, coordination.StateReadyToComplete)
	}
	// The claim is not rewritten by a move that is not a claim: attribution of
	// the claim belongs to the session that made it.
	if moved.ClaimedBy != first.ID {
		t.Errorf("claimed_by = %q, want the original claimant %q", moved.ClaimedBy, first.ID)
	}
}

// TestEveryTransitionTheTableRefusesLeavesTheRowUntouched is AC-04.3, over every
// illegal pair rather than over one.
//
// The row is re-read after each refusal instead of the error being trusted. A
// store that wrote first and judged afterwards would return exactly the same
// error while having already moved the task, and the difference is invisible
// from the return value alone.
func TestEveryTransitionTheTableRefusesLeavesTheRowUntouched(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)

	refusals := 0
	for _, from := range coordination.States() {
		for _, to := range coordination.States() {
			if coordination.CanTransition(from, to) {
				continue
			}
			refusals++

			task := f.taskIn(t, session.ID, from)
			before, err := f.store.FindTask(t.Context(), task.ID)
			if err != nil {
				t.Fatalf("FindTask = %v, want no error", err)
			}

			_, err = f.store.Transition(t.Context(), task.ID, session.ID, to, "a reason, in case it is needed")
			if !errors.Is(err, coordination.ErrTransitionNotAvailable) {
				t.Errorf("Transition(%s -> %s) = %v, want ErrTransitionNotAvailable", from, to, err)
				continue
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Errorf("Transition(%s -> %s) carries no domain payload", from, to)
			} else if payload.Code != app.CodeTaskStateInvalid {
				t.Errorf("Transition(%s -> %s) carries %q, want %q", from, to, payload.Code, app.CodeTaskStateInvalid)
			}

			after, err := f.store.FindTask(t.Context(), task.ID)
			if err != nil {
				t.Fatalf("FindTask after the refusal = %v, want no error", err)
			}
			if after.State != before.State {
				t.Errorf("a refused %s -> %s moved the task to %s", from, to, after.State)
			}
			if !after.UpdatedAt.Equal(before.UpdatedAt) {
				t.Errorf("a refused %s -> %s rewrote updated_at: %v -> %v",
					from, to, before.UpdatedAt, after.UpdatedAt)
			}
		}
	}

	if refusals != 49-13 {
		t.Errorf("the sweep exercised %d refusals; 7 states make 49 pairs and 13 are legal", refusals)
	}
}

// TestEveryTransitionTheTableAllowsIsWritten is the other arm, and the one that
// stops the test above from passing on a store that refuses everything.
func TestEveryTransitionTheTableAllowsIsWritten(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)

	accepted := 0
	for _, from := range coordination.States() {
		for _, to := range coordination.TransitionsFrom(from) {
			accepted++

			task := f.taskIn(t, session.ID, from)
			moved, err := f.store.Transition(t.Context(), task.ID, session.ID, to, "because the route needs one")
			if err != nil {
				t.Errorf("Transition(%s -> %s) = %v, want no error", from, to, err)
				continue
			}
			if moved.State != to {
				t.Errorf("Transition(%s -> %s) returned state %s", from, to, moved.State)
			}

			persisted, err := f.store.FindTask(t.Context(), task.ID)
			if err != nil {
				t.Fatalf("FindTask = %v, want no error", err)
			}
			if persisted.State != to {
				t.Errorf("Transition(%s -> %s) returned %s but the row says %s", from, to, moved.State, persisted.State)
			}
		}
	}

	if accepted != 13 {
		t.Errorf("the sweep exercised %d legal moves; decision D-55's table has 13", accepted)
	}
}

// TestABlockCarriesItsReasonAndLeavingClearsIt is decision D-63 in both
// directions.
//
// The reason is the whole value of BLOCKED to the next agent: "this is stuck" is
// something the state already says, and the only new information is what it is
// stuck on. Clearing it on the way out matters just as much — a reason left
// behind describes a block that was lifted, which is worse than none.
func TestABlockCarriesItsReasonAndLeavingClearsIt(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)
	task := f.taskIn(t, session.ID, coordination.StateInProgress)

	_, err := f.store.Transition(t.Context(), task.ID, session.ID, coordination.StateBlocked, "   ")
	if !errors.Is(err, coordination.ErrBlockedReasonMissing) {
		t.Fatalf("blocking with blank whitespace = %v, want ErrBlockedReasonMissing", err)
	}
	if unchanged, findErr := f.store.FindTask(t.Context(), task.ID); findErr != nil {
		t.Fatalf("FindTask = %v", findErr)
	} else if unchanged.State != coordination.StateInProgress {
		t.Fatalf("a refused block moved the task to %s", unchanged.State)
	}

	blocked := f.move(t, task.ID, session.ID, coordination.StateBlocked, "waiting on the vendor patch")
	if blocked.BlockedReason != "waiting on the vendor patch" {
		t.Errorf("blocked_reason = %q, want the reason that was given", blocked.BlockedReason)
	}

	resumed := f.move(t, task.ID, session.ID, coordination.StateInProgress, "")
	if resumed.BlockedReason != "" {
		t.Errorf("blocked_reason = %q after resuming, want it cleared", resumed.BlockedReason)
	}
	persisted, err := f.store.FindTask(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("FindTask = %v", err)
	}
	if persisted.BlockedReason != "" {
		t.Errorf("the row still holds blocked_reason %q after the block was lifted", persisted.BlockedReason)
	}
}

// TestClaimingRecordsTheSessionAndReleasingClearsIt is the claim column's two
// arms.
func TestClaimingRecordsTheSessionAndReleasingClearsIt(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)
	task := f.task(t, session.ID, "a task to claim and release")

	if task.ClaimedBy != "" {
		t.Errorf("a new task reports claimed_by %q, want nobody", task.ClaimedBy)
	}

	claimed := f.move(t, task.ID, session.ID, coordination.StateClaimed, "")
	if claimed.ClaimedBy != session.ID {
		t.Errorf("claimed_by = %q, want %q", claimed.ClaimedBy, session.ID)
	}

	released := f.move(t, task.ID, session.ID, coordination.StateOpen, "")
	if released.ClaimedBy != "" {
		t.Errorf("claimed_by = %q after release, want it cleared", released.ClaimedBy)
	}
	persisted, err := f.store.FindTask(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("FindTask = %v", err)
	}
	if persisted.ClaimedBy != "" {
		t.Errorf("the row still holds claimed_by %q after the task was released", persisted.ClaimedBy)
	}
}

// TestTheLastCheckpointIsTheNewestIdNotTheNewestTimestamp is AC-04.4 and
// decision D-59.
//
// The clock is frozen, so every checkpoint here carries the same created_at.
// That is not a contrivance: `app.FixedClock` is what the rest of this suite
// injects, and a real run writing two checkpoints in one millisecond produces
// the same tie. Ordering by created_at would return whichever row SQLite
// happened to reach first.
func TestTheLastCheckpointIsTheNewestIdNotTheNewestTimestamp(t *testing.T) {
	f := openFixture(t, filepath.Join(t.TempDir(), "mindrail.db"), app.FixedClock{Instant: baseInstant})
	session := f.session(t)
	task := f.task(t, session.ID, "a task with several notes")

	notes := []string{"first", "second", "third", "fourth"}
	written := make([]coordination.Checkpoint, 0, len(notes))
	for _, note := range notes {
		checkpoint, err := f.store.WriteCheckpoint(t.Context(), task.ID, session.ID, f.spaceID, note, false)
		if err != nil {
			t.Fatalf("WriteCheckpoint(%q) = %v, want no error", note, err)
		}
		written = append(written, checkpoint)
	}

	for i := 1; i < len(written); i++ {
		if !written[i].CreatedAt.Equal(written[0].CreatedAt) {
			t.Fatalf("the fixture's clock moved; this test needs the timestamps to tie")
		}
		if written[i].ID <= written[i-1].ID {
			t.Fatalf("checkpoint ids are not ascending: %q then %q", written[i-1].ID, written[i].ID)
		}
	}

	last, err := f.store.LastCheckpoint(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("LastCheckpoint = %v, want no error", err)
	}
	if last.Note != "fourth" {
		t.Errorf("LastCheckpoint returned %q, want the newest note %q", last.Note, "fourth")
	}
}

// TestATaskWithNoCheckpointIsNotAFailure keeps the absent-note case out of the
// error path. A task nobody has written on yet is the normal state of a task
// that was just opened.
func TestATaskWithNoCheckpointIsNotAFailure(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)
	task := f.task(t, session.ID, "a task nobody has annotated")

	handover, err := f.store.Handover(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("Handover = %v, want no error on a task with no checkpoint", err)
	}
	if handover.Checkpoint != nil {
		t.Errorf("Handover reported a checkpoint %+v, want none", handover.Checkpoint)
	}
	if handover.Task.ID != task.ID {
		t.Errorf("Handover named task %q, want %q", handover.Task.ID, task.ID)
	}

	// The direct reader still refuses, because a caller that asked for the note
	// specifically has to be able to tell "there is none" from "here it is".
	if _, err := f.store.LastCheckpoint(t.Context(), task.ID); !errors.Is(err, coordination.ErrCheckpointNotFound) {
		t.Errorf("LastCheckpoint = %v, want ErrCheckpointNotFound", err)
	}
}

// TestAnUnknownSessionIsRefusedByEveryWriter is decision D-61's refusal arm,
// asserted on every path that takes a session handle.
//
// One writer honouring it is not enough: the handle reaches three methods, and
// the one that skipped the check would write a row attributed to a session that
// does not exist — which the foreign key would then report as a constraint
// violation rather than as a typo.
func TestAnUnknownSessionIsRefusedByEveryWriter(t *testing.T) {
	f := newFixture(t)
	real := f.session(t)
	task := f.task(t, real.ID, "a task to attempt against a bad handle")

	const bogus = "SES-0000000000000000000000000"

	writers := map[string]func() error{
		"OpenTask": func() error {
			_, err := f.store.OpenTask(t.Context(), f.projectID, bogus, "a task from nowhere")
			return err
		},
		"Transition": func() error {
			_, err := f.store.Transition(t.Context(), task.ID, bogus, coordination.StateClaimed, "")
			return err
		},
		"WriteCheckpoint": func() error {
			_, err := f.store.WriteCheckpoint(t.Context(), task.ID, bogus, f.spaceID, "a note from nowhere", false)
			return err
		},
	}

	for name, write := range writers {
		err := write()
		if !errors.Is(err, coordination.ErrSessionNotFound) {
			t.Errorf("%s with an unknown session = %v, want ErrSessionNotFound", name, err)
			continue
		}
		payload, ok := app.PayloadOf(err)
		if !ok || payload.Code != app.CodeSessionNotFound {
			t.Errorf("%s carries payload %+v, want code %q", name, payload, app.CodeSessionNotFound)
		}
	}

	// Nothing was written under the bogus handle, and the task nobody could
	// claim is still OPEN.
	after, err := f.store.FindTask(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("FindTask = %v", err)
	}
	if after.State != coordination.StateOpen {
		t.Errorf("the task moved to %s under a session that does not exist", after.State)
	}
	if _, err := f.store.LastCheckpoint(t.Context(), task.ID); !errors.Is(err, coordination.ErrCheckpointNotFound) {
		t.Errorf("a checkpoint was written under a session that does not exist: %v", err)
	}
	tasks, err := f.store.ListTasks(t.Context(), f.projectID, "")
	if err != nil {
		t.Fatalf("ListTasks = %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("the project holds %d tasks, want the one that was opened legitimately", len(tasks))
	}
}

// TestAnUnknownTaskIsRefusedByNameRatherThanByAConstraint keeps the foreign key
// from being the thing that talks to the reader.
func TestAnUnknownTaskIsRefusedByNameRatherThanByAConstraint(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)

	const bogus = "TSK-0000000000000000000000000"

	for name, call := range map[string]func() error{
		"FindTask": func() error { _, err := f.store.FindTask(t.Context(), bogus); return err },
		"Handover": func() error { _, err := f.store.Handover(t.Context(), bogus); return err },
		"Transition": func() error {
			_, err := f.store.Transition(t.Context(), bogus, session.ID, coordination.StateClaimed, "")
			return err
		},
		"WriteCheckpoint": func() error {
			_, err := f.store.WriteCheckpoint(t.Context(), bogus, session.ID, f.spaceID, "note", false)
			return err
		},
	} {
		err := call()
		if !errors.Is(err, coordination.ErrTaskNotFound) {
			t.Errorf("%s on an unknown task = %v, want ErrTaskNotFound", name, err)
			continue
		}
		payload, ok := app.PayloadOf(err)
		if !ok || payload.Code != app.CodeTaskNotFound {
			t.Errorf("%s carries payload %+v, want code %q", name, payload, app.CodeTaskNotFound)
		}
		if !strings.Contains(payload.Why, bogus) {
			t.Errorf("%s does not name the id it refused: %q", name, payload.Why)
		}
	}
}

// TestSummarizeCountsTheActionableStatesAndNamesTheNewestNote is what `status`
// publishes, and both arms of what it leaves out.
func TestSummarizeCountsTheActionableStatesAndNamesTheNewestNote(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)

	empty, err := f.store.Summarize(t.Context(), f.projectID)
	if err != nil {
		t.Fatalf("Summarize = %v, want no error", err)
	}
	if empty != (coordination.Summary{}) {
		t.Errorf("Summarize over a project with no tasks = %+v, want zeroes and no checkpoint", empty)
	}

	f.taskIn(t, session.ID, coordination.StateOpen)
	f.taskIn(t, session.ID, coordination.StateOpen)
	f.taskIn(t, session.ID, coordination.StateInProgress)
	f.taskIn(t, session.ID, coordination.StateBlocked)
	// Two states that must NOT be counted: they only ever grow, so a number that
	// never goes down would make the report look busier every week.
	f.taskIn(t, session.ID, coordination.StateCompleted)
	noted := f.taskIn(t, session.ID, coordination.StateAbandoned)

	if _, err := f.store.WriteCheckpoint(t.Context(), noted.ID, session.ID, f.spaceID, "the newest note", true); err != nil {
		t.Fatalf("WriteCheckpoint = %v, want no error", err)
	}

	summary, err := f.store.Summarize(t.Context(), f.projectID)
	if err != nil {
		t.Fatalf("Summarize = %v, want no error", err)
	}
	if summary.Open != 2 || summary.InProgress != 1 || summary.Blocked != 1 {
		t.Errorf("Summarize = open %d, in progress %d, blocked %d; want 2, 1, 1",
			summary.Open, summary.InProgress, summary.Blocked)
	}
	if summary.LastCheckpoint == nil {
		t.Fatal("Summarize reported no checkpoint after one was written")
	}
	if summary.LastCheckpoint.TaskID != noted.ID {
		t.Errorf("the newest checkpoint names task %q, want %q", summary.LastCheckpoint.TaskID, noted.ID)
	}
	if summary.LastCheckpoint.SessionID != session.ID {
		t.Errorf("the newest checkpoint names session %q, want %q", summary.LastCheckpoint.SessionID, session.ID)
	}
}

// TestListTasksReturnsOneProjectsTasksNewestFirst covers the filter and the
// order in one place, including the arm that a state filter narrows rather than
// empties.
func TestListTasksReturnsOneProjectsTasksNewestFirst(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)

	oldest := f.task(t, session.ID, "first opened")
	middle := f.taskIn(t, session.ID, coordination.StateClaimed)
	newest := f.task(t, session.ID, "last opened")

	all, err := f.store.ListTasks(t.Context(), f.projectID, "")
	if err != nil {
		t.Fatalf("ListTasks = %v, want no error", err)
	}
	want := []string{newest.ID, middle.ID, oldest.ID}
	if len(all) != len(want) {
		t.Fatalf("ListTasks returned %d tasks, want %d", len(all), len(want))
	}
	for i, task := range all {
		if task.ID != want[i] {
			t.Errorf("ListTasks[%d] = %q, want %q; the order is newest first", i, task.ID, want[i])
		}
	}

	claimed, err := f.store.ListTasks(t.Context(), f.projectID, coordination.StateClaimed)
	if err != nil {
		t.Fatalf("ListTasks(CLAIMED) = %v, want no error", err)
	}
	if len(claimed) != 1 || claimed[0].ID != middle.ID {
		t.Errorf("ListTasks(CLAIMED) = %v, want only %q", ids(claimed), middle.ID)
	}
}

// TestTimestampsAreStoredAsUTC keeps the injected non-UTC clock from reaching
// the database, which is the same guard internal/workspace carries.
func TestTimestampsAreStoredAsUTC(t *testing.T) {
	f := openFixture(t, filepath.Join(t.TempDir(), "mindrail.db"), app.FixedClock{Instant: baseInstant})
	session := f.session(t)
	task := f.task(t, session.ID, "a task with a timestamp")
	checkpoint, err := f.store.WriteCheckpoint(t.Context(), task.ID, session.ID, f.spaceID, "a note", false)
	if err != nil {
		t.Fatalf("WriteCheckpoint = %v, want no error", err)
	}

	want := app.FormatTime(baseInstant)
	for query, id := range map[string]string{
		`SELECT started_at FROM sessions WHERE session_id = ?`:       session.ID,
		`SELECT created_at FROM tasks WHERE task_id = ?`:             task.ID,
		`SELECT updated_at FROM tasks WHERE task_id = ?`:             task.ID,
		`SELECT created_at FROM checkpoints WHERE checkpoint_id = ?`: checkpoint.ID,
	} {
		var stored string
		if err := f.db.QueryRowContext(t.Context(), query, id).Scan(&stored); err != nil {
			t.Fatalf("%s = %v, want no error", query, err)
		}
		if stored != want {
			t.Errorf("%s stored %q, want %q", query, stored, want)
		}
	}
}

// TestATitlelessTaskAndANotelessCheckpointAreRefused keeps two empty strings out
// of columns declared NOT NULL but not NOT EMPTY.
//
// Both would insert cleanly and both would be useless to the arriving agent,
// which is the reader every row in this package exists for.
func TestATitlelessTaskAndANotelessCheckpointAreRefused(t *testing.T) {
	f := newFixture(t)
	session := f.session(t)

	if _, err := f.store.OpenTask(t.Context(), f.projectID, session.ID, "   "); err == nil {
		t.Error("OpenTask with a blank title succeeded")
	}

	task := f.task(t, session.ID, "a task with a real title")
	if _, err := f.store.WriteCheckpoint(t.Context(), task.ID, session.ID, f.spaceID, "\t\n", false); err == nil {
		t.Error("WriteCheckpoint with a blank note succeeded")
	}

	tasks, err := f.store.ListTasks(t.Context(), f.projectID, "")
	if err != nil {
		t.Fatalf("ListTasks = %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("the project holds %d tasks, want only the one with a title", len(tasks))
	}
	if _, err := f.store.LastCheckpoint(t.Context(), task.ID); !errors.Is(err, coordination.ErrCheckpointNotFound) {
		t.Errorf("a blank checkpoint reached the database: %v", err)
	}
}

func ids(tasks []coordination.Task) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.ID)
	}
	return out
}
