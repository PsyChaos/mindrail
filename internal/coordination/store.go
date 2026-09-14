package coordination

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/identity"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// Identifier prefixes. Like internal/workspace's, they are part of the persisted
// value, so an id read out of a log or an agent's transcript says what kind of
// thing it names without a lookup.
const (
	sessionIDPrefix    = "SES"
	taskIDPrefix       = "TSK"
	checkpointIDPrefix = "CKP"
)

// TableSchemaVersion is the migration that creates the sessions, tasks and
// checkpoints tables.
//
// It exists for the same split workspace.TableSchemaVersion does, one step
// later: a database written by an older binary has its workspaces table and
// the row in it, so the upgrade lookup succeeds, but it has no coordination
// tables to query. A caller that resolves the workspace and then uses this
// store anyway answers with the SQL error and a remedy that cannot clear it,
// where the condition is one `mindrail init` genuinely fixes (audit round 2,
// §4.1).
const TableSchemaVersion = 2

// selectNewestCheckpointOfProject is the query `status` runs on every start.
//
// EXISTS rather than a join, and the difference is the whole cost of it. A join
// drives from tasks — SEARCH t USING idx_tasks_project, a probe into the
// checkpoint index per task, then a temp b-tree to sort what came back — so the
// work is proportional to the number of tasks in the project even when the
// project has no checkpoints at all: 1.2 ms at 1,000 tasks and 7–29 ms at
// 20,000, depending on how much of `status` is timed around it, paid on every
// read-only startup (finding F48; this comment said "1.2 ms at 20,000" until
// audit round 2, §4.11, traced the figure to the 1,000-task row).
//
// Written this way the plan is SCAN c in reverse rowid order with an EXISTS
// probe per row, and LIMIT 1 stops at the first checkpoint belonging to this
// project — 10 µs on the same database, because the newest checkpoint is the
// first row the scan meets. It is a package-level constant so that the test
// asserting the plan reads the same string this runs.
//
// The trade is conditional, not free. When the queried project has no
// checkpoints and the same database holds another project's history — which
// moving a repository directory and re-running `init` produces, since project
// identity is the git common dir and the database travels inside it — the
// reverse scan probes every checkpoint and matches none: about 2.3 µs per
// checkpoint, 45 ms at 20,000, on every read-only startup until the project's
// first checkpoint is written. The join in that state cost O(tasks of the
// queried project), which is nearly nothing (audit round 2, §4.13).
//
// The comment in migrations/000002_coordination.sql attributes this query to
// `task show` and names the task-scoped one instead. It is wrong and it stays:
// the migrator checksums the file it applied, so editing an applied migration
// would report MIGRATION_CHECKSUM_MISMATCH on every existing database — the
// loudest possible failure for a corrected comment. The correction is written
// where the file's readers can find it, migrations/README.md, and
// migrations/shipped_test.go pins the file's bytes so the edit cannot ship.
const selectNewestCheckpointOfProject = `SELECT c.task_id, c.session_id, c.created_at
	   FROM checkpoints c
	  WHERE EXISTS (SELECT 1 FROM tasks t
	                 WHERE t.task_id = c.task_id AND t.project_id = ?)
	  ORDER BY c.rowid DESC LIMIT 1`

// Store reads and writes the session, task and checkpoint rows.
//
// Every method is one short transaction (tech-stack §11). There is no method
// that holds a transaction open across a decision the caller makes, because the
// caller is a command that has already exited by the time the next one runs.
type Store struct {
	db    *sql.DB
	clock app.Clock
}

// NewStore builds a store over an already-migrated database.
func NewStore(db *sql.DB, clock app.Clock) *Store {
	return &Store{db: db, clock: clock}
}

// OpenSession mints a session for one workspace.
//
// Nothing looks an existing session up first: a session is a run, and two runs
// against one workspace are two sessions (decision D-56). The label is whatever
// the caller wants to recognise itself by later and is never interpreted.
func (s *Store) OpenSession(ctx context.Context, workspaceID, label string) (Session, error) {
	if workspaceID == "" {
		return Session{}, noWorkspace("a session")
	}

	session := Session{
		ID:          identity.NewID(sessionIDPrefix),
		WorkspaceID: workspaceID,
		Label:       strings.TrimSpace(label),
		StartedAt:   s.clock.Now().UTC(),
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (session_id, workspace_id, label, started_at) VALUES (?, ?, ?, ?)`,
		session.ID, session.WorkspaceID, nullable(session.Label), app.FormatTime(session.StartedAt))
	if err != nil {
		return Session{}, s.writeFailure(ctx, "the agent session", session.ID, err)
	}

	return session, nil
}

// Attribution says which agent session a write belongs to.
//
// A caller either names a session it already holds or asks for one to be
// minted, and the difference matters for where the mint happens. It used to
// happen first, in the caller, through OpenSession's own statement — so a write
// that was then refused left behind a session row for an agent that never did
// anything, under an `impact` reading "Nothing was written" (finding F02).
// Thirty refusals in a loop left thirty rows, and no shipped command can list
// or remove one.
//
// Passed as a value here, the mint happens inside the transaction that carries
// the write, so a refusal anywhere in that transaction takes it back with
// everything else.
//
// mint is carried explicitly rather than inferred from an empty handle, so that
// NamedSession("") is a lookup of nothing — ErrSessionNotFound — rather than a
// silent request to mint.
type Attribution struct {
	handle      string
	workspaceID string
	mint        bool
}

// NamedSession attributes a write to a session that already exists. A handle
// that names nothing is ErrSessionNotFound, and nothing is written.
func NamedSession(handle string) Attribution { return Attribution{handle: handle} }

// MintFor attributes a write to a session minted for this workspace, in the
// write's own transaction (decision D-61).
func MintFor(workspaceID string) Attribution {
	return Attribution{workspaceID: workspaceID, mint: true}
}

// Write is what an attributed write reports beside its own result: the session
// it was attributed to, and whether that session was minted here.
//
// Minted is returned rather than inferred by the caller, because "you are
// working under a session you did not name" is something the result has to say
// out loud: an agent that wanted continuity and forgot the flag would otherwise
// carry on under a fresh identity without noticing.
type Write struct {
	Session Session
	Minted  bool
}

// attribute resolves an Attribution inside the transaction that carries the
// write, either by finding the named session or by minting one.
func (s *Store) attribute(ctx context.Context, tx *sql.Tx, by Attribution) (Write, error) {
	if !by.mint {
		session, err := requireSession(ctx, tx, by.handle)
		if err != nil {
			return Write{}, err
		}
		return Write{Session: session}, nil
	}

	if by.workspaceID == "" {
		return Write{}, noWorkspace("a session minted for a write")
	}

	session := Session{
		ID:          identity.NewID(sessionIDPrefix),
		WorkspaceID: by.workspaceID,
		StartedAt:   s.clock.Now().UTC(),
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO sessions (session_id, workspace_id, label, started_at) VALUES (?, ?, ?, ?)`,
		session.ID, session.WorkspaceID, nullable(session.Label),
		app.FormatTime(session.StartedAt)); err != nil {
		return Write{}, err
	}
	return Write{Session: session, Minted: true}, nil
}

// A session is looked up by requireSession, inside the transaction that is
// about to attribute a write to it. There is no standalone FindSession: it had
// one caller, the CLI's eager mint, and reading a session outside the
// transaction that uses it is the shape finding F02 was about — the answer can
// stop being true between the read and the write.

// OpenTask creates a task in OPEN, attributed to the session that opened it.
//
// The session is resolved inside the transaction rather than before it. A
// handle that stopped being valid between the check and the insert would
// otherwise write a task pointing at nothing — and the foreign key would refuse
// it with a message about a constraint rather than about a session. A session
// minted for this write is minted in the same transaction for the mirror-image
// reason: a refusal below takes it back with everything else.
func (s *Store) OpenTask(ctx context.Context, projectID string, by Attribution, title string) (Task, Write, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, Write{}, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"a task needs a title",
			"Nothing was written.",
			"Re-run with --title describing the work in one line.",
		)
	}

	now := s.clock.Now().UTC()
	task := Task{
		ID:        identity.NewID(taskIDPrefix),
		Title:     title,
		State:     StateOpen,
		ProjectID: projectID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	var write Write
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		resolved, err := s.attribute(ctx, tx, by)
		if err != nil {
			return err
		}
		write = resolved
		task.OpenedBy = resolved.Session.ID

		_, err = tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, project_id, title, state, blocked_reason,
				opened_by, claimed_by, created_at, updated_at)
			 VALUES (?, ?, ?, ?, NULL, ?, NULL, ?, ?)`,
			task.ID, task.ProjectID, task.Title, string(task.State),
			task.OpenedBy, app.FormatTime(now), app.FormatTime(now))
		return err
	})
	if err != nil {
		return Task{}, Write{}, s.adopt(ctx, "the task", task.ID, err)
	}

	return task, write, nil
}

// Transition moves a task through decision D-55's table.
//
// The whole operation is one transaction: the task is read, the move is judged
// and the row is written without anything else being able to move it in
// between. Reading outside the transaction and writing inside it would make the
// refusal a statement about a state the task may already have left.
//
// It does not check who claimed the task. A second session moving a task the
// first one claimed is the handover working (decision D-58); ownership becomes
// enforceable in MR-004, where a lease makes "who may move this" a question with
// a time bound behind it.
func (s *Store) Transition(ctx context.Context, taskID string, by Attribution, to State, reason string) (Task, Write, error) {
	reason = strings.TrimSpace(reason)

	var (
		updated Task
		write   Write
	)
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		resolved, err := s.attribute(ctx, tx, by)
		if err != nil {
			return err
		}
		write = resolved
		sessionID := resolved.Session.ID

		current, err := scanTask(tx.QueryRowContext(ctx, selectTask+` WHERE task_id = ?`, taskID))
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return taskNotFound(taskID)
		case err != nil:
			return readFailed("the task", taskID, err)
		}

		if !CanTransition(current.State, to) {
			return transitionNotAvailable(taskID, current.State, to, current.ClaimedBy)
		}
		if to == StateBlocked && reason == "" {
			return blockedReasonMissing(taskID)
		}

		updated = current
		updated.State = to
		updated.UpdatedAt = s.clock.Now().UTC()

		// A block carries its reason; anything else clears it, so a reader never
		// meets a reason belonging to a block that was lifted.
		updated.BlockedReason = ""
		if to == StateBlocked {
			updated.BlockedReason = reason
		}

		// The claim is recorded on the way in and released on the way out. OPEN
		// is the release path (D-55), and a released task that kept its claimant
		// would report itself as owned by a session that gave it up.
		switch to {
		case StateClaimed:
			updated.ClaimedBy = sessionID
		case StateOpen:
			updated.ClaimedBy = ""
		}

		_, err = tx.ExecContext(ctx,
			`UPDATE tasks SET state = ?, blocked_reason = ?, claimed_by = ?, updated_at = ?
			 WHERE task_id = ?`,
			string(updated.State), nullable(updated.BlockedReason), nullable(updated.ClaimedBy),
			app.FormatTime(updated.UpdatedAt), taskID)
		return err
	})
	if err != nil {
		return Task{}, Write{}, s.adopt(ctx, "the task state", taskID, err)
	}

	return updated, write, nil
}

// FindTask returns the task with this id, or ErrTaskNotFound.
func (s *Store) FindTask(ctx context.Context, id string) (Task, error) {
	task, err := scanTask(s.db.QueryRowContext(ctx, selectTask+` WHERE task_id = ?`, id))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Task{}, taskNotFound(id)
	case err != nil:
		return Task{}, readFailed("the task", id, err)
	}
	return task, nil
}

// ListTasks returns a project's tasks, newest first, optionally of one state.
//
// Newest first because the question a reader has is "what is happening now",
// and the id is time-sortable so the order is the mint order rather than the
// clock's. `state` empty means every state.
func (s *Store) ListTasks(ctx context.Context, projectID string, state State) ([]Task, error) {
	query := selectTask + ` WHERE project_id = ?`
	args := []any{projectID}
	if state != "" {
		query += ` AND state = ?`
		args = append(args, string(state))
	}
	query += ` ORDER BY task_id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, readFailed("this project's tasks", projectID, err)
	}
	defer func() { _ = rows.Close() }()

	tasks := make([]Task, 0, 8)
	for rows.Next() {
		task, scanErr := scanTask(rows)
		if scanErr != nil {
			return nil, readFailed("this project's tasks", projectID, scanErr)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, readFailed("this project's tasks", projectID, err)
	}
	return tasks, nil
}

// WriteCheckpoint appends a note to a task.
//
// Append-only (decision D-59). There is no Update and no Delete on this table,
// and the absence is the feature: a handover note that could be rewritten after
// the fact is one the arriving agent cannot rely on.
func (s *Store) WriteCheckpoint(ctx context.Context, taskID string, by Attribution, workspaceID, note string, handoff bool) (Checkpoint, Write, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return Checkpoint{}, Write{}, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"a checkpoint needs a note",
			"Nothing was written.",
			"Re-run with --note saying where the work stands.",
		)
	}

	checkpoint := Checkpoint{
		ID:          identity.NewID(checkpointIDPrefix),
		TaskID:      taskID,
		WorkspaceID: workspaceID,
		Note:        note,
		Handoff:     handoff,
		CreatedAt:   s.clock.Now().UTC(),
	}

	var write Write
	err := storage.InTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		resolved, err := s.attribute(ctx, tx, by)
		if err != nil {
			return err
		}
		write = resolved
		checkpoint.SessionID = resolved.Session.ID

		var exists string
		err = tx.QueryRowContext(ctx, `SELECT task_id FROM tasks WHERE task_id = ?`, taskID).Scan(&exists)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return taskNotFound(taskID)
		case err != nil:
			return readFailed("the task", taskID, err)
		}

		handoffValue := 0
		if handoff {
			handoffValue = 1
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO checkpoints (checkpoint_id, task_id, session_id, workspace_id,
				note, handoff, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			checkpoint.ID, checkpoint.TaskID, checkpoint.SessionID, checkpoint.WorkspaceID,
			checkpoint.Note, handoffValue, app.FormatTime(checkpoint.CreatedAt))
		return err
	})
	if err != nil {
		return Checkpoint{}, Write{}, s.adopt(ctx, "the checkpoint", checkpoint.ID, err)
	}

	return checkpoint, write, nil
}

// LastCheckpoint returns a task's newest checkpoint, or ErrCheckpointNotFound.
//
// Newest by rowid, which is the order SQLite inserted the rows in. Neither of
// the two columns that look like they answer this question does.
//
// created_at is stamped in Go before the transaction opens, and app.FormatTime
// writes RFC3339Nano, which trims trailing zeros — so the TEXT column is not
// even lexicographically ordered, and an injected clock does not move at all.
//
// checkpoint_id was the answer until finding F37. Its 48-bit millisecond prefix
// is monotonic *within one process*, because the tie-break under the prefix is
// a per-process counter; two processes writing in the same millisecond order by
// 80 random bits. Two agents handing over is the case this milestone exists
// for, so "the newest checkpoint" was decided by a coin flip exactly when it
// mattered most — the writes serialise correctly and the read that decides
// which write was last did not.
//
// rowid is assigned by the database inside the insert, in insertion order, and
// decision D-59 forbids deleting a checkpoint, so it is never reused.
func (s *Store) LastCheckpoint(ctx context.Context, taskID string) (Checkpoint, error) {
	row := s.db.QueryRowContext(ctx,
		selectCheckpoint+` WHERE task_id = ? ORDER BY rowid DESC LIMIT 1`, taskID)

	checkpoint, err := scanCheckpoint(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Checkpoint{}, checkpointNotFound(taskID)
	case err != nil:
		return Checkpoint{}, readFailed("the last checkpoint of this task", taskID, err)
	}
	return checkpoint, nil
}

// Handover answers the arriving agent's whole question in one call: the task and
// the newest thing said about it.
//
// A task with no checkpoint is not a failure — it is a task nobody has left a
// note on yet — so the absent checkpoint is reported as a nil pointer rather
// than as an error the caller has to classify.
func (s *Store) Handover(ctx context.Context, taskID string) (Handover, error) {
	task, err := s.FindTask(ctx, taskID)
	if err != nil {
		return Handover{}, err
	}

	checkpoint, err := s.LastCheckpoint(ctx, taskID)
	switch {
	case errors.Is(err, ErrCheckpointNotFound):
		return Handover{Task: task}, nil
	case err != nil:
		return Handover{}, err
	}
	return Handover{Task: task, Checkpoint: &checkpoint}, nil
}

// Summarize is what `status` publishes: the three counts a reader can act on,
// and the newest checkpoint anywhere in the project.
func (s *Store) Summarize(ctx context.Context, projectID string) (Summary, error) {
	summary := Summary{}

	rows, err := s.db.QueryContext(ctx,
		`SELECT state, count(*) FROM tasks WHERE project_id = ? GROUP BY state`, projectID)
	if err != nil {
		return Summary{}, readFailed("the task counts of this project", projectID, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			state string
			count int
		)
		if err := rows.Scan(&state, &count); err != nil {
			return Summary{}, readFailed("the task counts of this project", projectID, err)
		}
		switch State(state) {
		case StateOpen:
			summary.Open = count
		case StateInProgress:
			summary.InProgress = count
		case StateBlocked:
			summary.Blocked = count
		}
	}
	if err := rows.Err(); err != nil {
		return Summary{}, readFailed("the task counts of this project", projectID, err)
	}

	var (
		taskID    string
		sessionID string
		createdAt string
	)
	err = s.db.QueryRowContext(ctx, selectNewestCheckpointOfProject, projectID).
		Scan(&taskID, &sessionID, &createdAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return summary, nil
	case err != nil:
		return Summary{}, readFailed("the newest checkpoint of this project", projectID, err)
	}

	at, err := app.ParseTime(createdAt)
	if err != nil {
		return Summary{}, readFailed("the newest checkpoint of this project", taskID, err)
	}
	summary.LastCheckpoint = &CheckpointRef{TaskID: taskID, SessionID: sessionID, At: at}
	return summary, nil
}

const selectTask = `SELECT task_id, project_id, title, state, blocked_reason,
	opened_by, claimed_by, created_at, updated_at FROM tasks`

const selectCheckpoint = `SELECT checkpoint_id, task_id, session_id, workspace_id,
	note, handoff, created_at FROM checkpoints`

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so the single-row and
// multi-row paths decode identically.
type rowScanner interface{ Scan(dest ...any) error }

func scanSession(row rowScanner) (Session, error) {
	var (
		session   Session
		label     sql.NullString
		startedAt string
	)
	if err := row.Scan(&session.ID, &session.WorkspaceID, &label, &startedAt); err != nil {
		return Session{}, err
	}
	session.Label = label.String

	var err error
	if session.StartedAt, err = app.ParseTime(startedAt); err != nil {
		return Session{}, fmt.Errorf("session %s started_at: %w", session.ID, err)
	}
	return session, nil
}

func scanTask(row rowScanner) (Task, error) {
	var (
		task      Task
		state     string
		reason    sql.NullString
		claimedBy sql.NullString
		createdAt string
		updatedAt string
	)
	if err := row.Scan(&task.ID, &task.ProjectID, &task.Title, &state, &reason,
		&task.OpenedBy, &claimedBy, &createdAt, &updatedAt); err != nil {
		return Task{}, err
	}

	task.State = State(state)
	task.BlockedReason = reason.String
	task.ClaimedBy = claimedBy.String

	var err error
	if task.CreatedAt, err = app.ParseTime(createdAt); err != nil {
		return Task{}, fmt.Errorf("task %s created_at: %w", task.ID, err)
	}
	if task.UpdatedAt, err = app.ParseTime(updatedAt); err != nil {
		return Task{}, fmt.Errorf("task %s updated_at: %w", task.ID, err)
	}
	return task, nil
}

func scanCheckpoint(row rowScanner) (Checkpoint, error) {
	var (
		checkpoint Checkpoint
		handoff    int
		createdAt  string
	)
	if err := row.Scan(&checkpoint.ID, &checkpoint.TaskID, &checkpoint.SessionID,
		&checkpoint.WorkspaceID, &checkpoint.Note, &handoff, &createdAt); err != nil {
		return Checkpoint{}, err
	}
	checkpoint.Handoff = handoff != 0

	var err error
	if checkpoint.CreatedAt, err = app.ParseTime(createdAt); err != nil {
		return Checkpoint{}, fmt.Errorf("checkpoint %s created_at: %w", checkpoint.ID, err)
	}
	return checkpoint, nil
}

// requireSession refuses a handle that names no session, inside the caller's
// transaction.
//
// A row it finds and cannot decode is a read failure naming the session, not a
// write failure naming the row the caller was about to insert. adopt turns any
// bare error that leaves a write transaction into COORDINATION_WRITE_FAILED
// carrying the caller's subject, so a session whose started_at does not parse
// used to be published as "the task could not be written" with a subject_id no
// statement ever attempted — and the code is defined by what happened to a row,
// not by the verb of the command that met it (audit round 2, §4.7). The two
// precondition reads in Transition and WriteCheckpoint answer the same way, so
// a damaged row is reported under one code whichever command reaches it.
func requireSession(ctx context.Context, tx *sql.Tx, id string) (Session, error) {
	if id == "" {
		return Session{}, sessionNotFound(id)
	}

	session, err := scanSession(tx.QueryRowContext(ctx,
		`SELECT session_id, workspace_id, label, started_at FROM sessions WHERE session_id = ?`, id))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Session{}, sessionNotFound(id)
	case err != nil:
		return Session{}, readFailed("the agent session", id, err)
	}
	return session, nil
}

// nullable stores an empty string as NULL.
//
// The two are different facts in this schema: a task with no claimant has no
// claimant, and a task claimed by the session whose id is the empty string is
// not a thing that can exist. Storing "" would make the column's NOT NULL
// siblings and its nullable ones read differently for the same absence.
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// adopt turns a transaction failure into the error a reader can act on.
//
// A domain error raised inside the transaction — a missing session, a refused
// transition, a row that could not be read — is already the answer and passes
// through unchanged. Anything else is a write that did not land, and the
// storage layer classifies it first: an
// unwritable database, a full disk and a busy lock all have remedies of their
// own, and the generic one is only correct for what is left.
func (s *Store) adopt(ctx context.Context, what, id string, err error) error {
	if _, isDomain := app.PayloadOf(err); isDomain {
		return err
	}
	return s.writeFailure(ctx, what, id, err)
}

func (s *Store) writeFailure(ctx context.Context, what, id string, cause error) error {
	if named := storage.WriteFailure(ctx, s.db, "write "+what, cause); named != nil {
		return named.WithMetadata("subject_id", id)
	}
	return writeFailed(what, id, cause)
}
