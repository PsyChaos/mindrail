package coordination

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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

// TableSchemaVersion is the newest migration this store's tables and columns
// come from: 000003, which adds the leases and operations tables and the
// revision column on tasks (decision D-73). It was 2 while MR-003's three
// tables were all the store read.
//
// It exists for the same split workspace.TableSchemaVersion does, one step
// later: a database written by an older binary has its workspaces table and
// the row in it, so the upgrade lookup succeeds, but it lacks what this store
// queries — no coordination tables at all from an MR-002 binary, no leases
// and no revision from an MR-003 one. A caller that resolves the workspace
// and then uses this store anyway answers with the SQL error and a remedy
// that cannot clear it, where the condition is one `mindrail init` genuinely
// fixes (audit round 2, §4.1).
const TableSchemaVersion = 3

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

	// op is the operation the next write runs under, bound by Idempotent;
	// the zero value is no operation (decision D-71).
	op Operation
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
//
// It is one transaction like every other writer, since MR-004: it used to be a
// bare ExecContext, which under a held write lock waited the whole busy budget
// like the others and then published no `waited_ms` where they did — one
// condition, two answers (TASK-01's gate). Going through InTxMeasured is also
// what TASK-05 needs, since the operation record must land in the same
// transaction as the row.
//
// It returns a Write like the other writers, attributed to the session it
// minted, so the transaction's cost rides on it (decision D-75); the first
// shape returned the session alone and discarded the measurement (TASK-03's
// gate).
func (s *Store) OpenSession(ctx context.Context, workspaceID, label string) (Session, Write, error) {
	if workspaceID == "" {
		return Session{}, Write{}, noWorkspace("a session")
	}

	if err := s.refuseInvalidOperation(); err != nil {
		return Session{}, Write{}, err
	}
	hash, err := requestHash("session open", struct {
		Workspace string `json:"workspace"`
		Label     string `json:"label"`
	}{workspaceID, strings.TrimSpace(label)})
	if err != nil {
		return Session{}, Write{}, err
	}

	session := Session{
		ID:          identity.NewID(sessionIDPrefix),
		WorkspaceID: workspaceID,
		Label:       strings.TrimSpace(label),
		StartedAt:   s.clock.Now().UTC(),
	}

	var write Write
	stats, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if replayed, found, err := s.replay(ctx, tx, "session open", hash, &session); err != nil {
			return err
		} else if found {
			write = replayed
			return nil
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sessions (session_id, workspace_id, label, started_at) VALUES (?, ?, ?, ?)`,
			session.ID, session.WorkspaceID, nullable(session.Label), app.FormatTime(session.StartedAt)); err != nil {
			return err
		}
		write = Write{Session: session, Minted: true, OperationID: s.op.ID}
		return s.record(ctx, tx, "session open", hash, write, session, session.StartedAt)
	})
	if err != nil {
		return Session{}, Write{}, s.adopt(ctx, "the agent session", session.ID, err)
	}
	write.Timing = stats

	return session, write, nil
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
// it was attributed to, whether that session was minted here, and what the
// transaction cost.
//
// Minted is returned rather than inferred by the caller, because "you are
// working under a session you did not name" is something the result has to say
// out loud: an agent that wanted continuity and forgot the flag would otherwise
// carry on under a fresh identity without noticing.
//
// Timing is the wait for the write lock and the time it was held, measured by
// storage.InTxMeasured on every write (decision D-75). The command logs it;
// nothing publishes it on the wire until MR-019 decides the shape.
type Write struct {
	Session Session
	Minted  bool
	Timing  storage.TxStats

	// OperationID is the id the write ran under, empty when it ran under
	// none; Replayed is true when the operations table answered — the
	// result is the first delivery's, nothing was written, and an agent
	// reading it is reading the state as of that delivery (decision D-71).
	OperationID string
	Replayed    bool
}

// attribute resolves an Attribution inside the transaction that carries the
// write, either by finding the named session or by minting one.
//
// now is the instant the caller stamped on its own row, and a minted session
// starts then rather than at a second reading of the clock. The mint used to
// happen first, in the CLI, so the session always started before the row it
// opened; moved into the transaction with a clock read of its own, it always
// started after it, and both fields are on the wire of `task open --json`
// (audit round 2, §4.12). Round 1 asked for the mint to be placed inside the
// write's transaction, which is a requirement on where the INSERT runs and not
// on when the session begins — a session minted for a write begins with it.
func (s *Store) attribute(ctx context.Context, tx *sql.Tx, by Attribution, now time.Time) (Write, error) {
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
		StartedAt:   now,
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

	if err := s.refuseInvalidOperation(); err != nil {
		return Task{}, Write{}, err
	}
	hash, err := requestHash("task open", struct {
		Project string `json:"project"`
		By      string `json:"by"`
		Title   string `json:"title"`
	}{projectID, attributionKey(by), title})
	if err != nil {
		return Task{}, Write{}, err
	}

	now := s.clock.Now().UTC()
	task := Task{
		ID:        identity.NewID(taskIDPrefix),
		Title:     title,
		State:     StateOpen,
		ProjectID: projectID,
		CreatedAt: now,
		UpdatedAt: now,
		Revision:  1,
	}

	var write Write
	stats, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if replayed, found, err := s.replay(ctx, tx, "task open", hash, &task); err != nil {
			return err
		} else if found {
			write = replayed
			return nil
		}

		resolved, err := s.attribute(ctx, tx, by, now)
		if err != nil {
			return err
		}
		write = resolved
		write.OperationID = s.op.ID
		task.OpenedBy = resolved.Session.ID

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, project_id, title, state, blocked_reason,
				opened_by, claimed_by, created_at, updated_at, revision)
			 VALUES (?, ?, ?, ?, NULL, ?, NULL, ?, ?, ?)`,
			task.ID, task.ProjectID, task.Title, string(task.State),
			task.OpenedBy, app.FormatTime(now), app.FormatTime(now), task.Revision); err != nil {
			return err
		}
		return s.record(ctx, tx, "task open", hash, write, task, now)
	})
	if err != nil {
		return Task{}, Write{}, s.adopt(ctx, "the task", task.ID, err)
	}
	write.Timing = stats

	return task, write, nil
}

// Move is what Transition reports: the task as written, the lease the move
// left active on it — nil after a release to OPEN or a terminal move — and the
// expired tenure the move took over, when it did (decision D-67: a takeover is
// reported, never silent).
type Move struct {
	Task       Task   `json:"task"`
	Lease      *Lease `json:"lease"`
	Superseded *Lease `json:"superseded"`
}

// Transition moves a task through decision D-55's table with no expectation
// about its revision. It is TransitionExpecting with the expectation left
// out; the command line passes what it was given, and a caller with no
// reading of its own to be stale has nothing to expect.
func (s *Store) Transition(ctx context.Context, taskID string, by Attribution, to State, reason string) (Move, Write, error) {
	return s.TransitionExpecting(ctx, taskID, by, to, reason, 0)
}

// TransitionExpecting moves a task through decision D-55's table, under the
// lease and the revision MR-004 put in front of it.
//
// The whole operation is one transaction: the task is read, the move is judged
// and the row is written without anything else being able to move it in
// between. Reading outside the transaction and writing inside it would make the
// refusal a statement about a state the task may already have left.
//
// Three judgments run before the table, in this order (design §7):
//
//  1. Revision (decision D-72). A caller that read the task at revision n and
//     expects n is refused with STATE_REVISION_CONFLICT if the row has moved
//     on, before anything else is looked at — a caller whose reading is stale
//     should not be told about the lease as if its reading were current. Zero
//     expects nothing.
//  2. Lease (decision D-67). A task whose active lease another session holds
//     is LEASE_CONFLICT whatever the destination: the reason the move cannot
//     happen is ownership, and a state refusal would read the same whether
//     the lease had expired or not.
//  3. The state table (D-55) and the blocked-reason rule (D-63), as before.
//
// Then the effect by destination: CLAIMED and the working states acquire or
// renew the lease for the mover — taking over an expired tenure out loud —
// and make the mover the claimant; OPEN releases and clears the claimant; the
// terminal states release and leave the claimant as attribution (decision
// D-68). The row is written with `revision = revision + 1 WHERE revision = ?`
// against the revision read in this transaction; under BEGIN IMMEDIATE that
// guard cannot fail, so no row affected is a defect and not a second conflict.
func (s *Store) TransitionExpecting(ctx context.Context, taskID string, by Attribution, to State, reason string, expectRevision int64) (Move, Write, error) {
	reason = strings.TrimSpace(reason)
	if expectRevision < 0 {
		// Revisions start at 1 and zero means no expectation; a negative one
		// is a mistake, not a weaker expectation, and the command line's
		// guard does not stand in front of every caller (TASK-04's Breaker).
		return Move{}, Write{}, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			fmt.Sprintf("an expected revision of %d names no revision a task can be at", expectRevision),
			"Nothing was read and nothing was written.",
			"Pass --expect-revision as the revision `mindrail task show` printed, 1 or above, or leave it out.",
		).WithMetadata("expected_revision", strconv.FormatInt(expectRevision, 10))
	}

	if err := s.refuseInvalidOperation(); err != nil {
		return Move{}, Write{}, err
	}
	hash, err := requestHash("task state", struct {
		Task   string `json:"task"`
		By     string `json:"by"`
		To     State  `json:"to"`
		Reason string `json:"reason"`
		Expect int64  `json:"expect_revision"`
	}{taskID, attributionKey(by), to, reason, expectRevision})
	if err != nil {
		return Move{}, Write{}, err
	}

	var (
		move  Move
		write Write
	)
	stats, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if replayed, found, err := s.replay(ctx, tx, "task state", hash, &move); err != nil {
			return err
		} else if found {
			write = replayed
			return nil
		}

		// One clock reading per write, and it is taken here, under the write
		// lock, not before the transaction the way the two inserting writers
		// take theirs. updated_at replaces an earlier value on the same row,
		// so it has to follow commit order: a move that waited on the lock and
		// committed second must not carry the earlier stamp, which is what a
		// reading before BEGIN gave it (verification pass after the round-2
		// remediation). A session minted here starts at this same instant, and
		// the lease is judged against it (decision D-65).
		now := s.clock.Now().UTC()

		resolved, err := s.attribute(ctx, tx, by, now)
		if err != nil {
			return err
		}
		write = resolved
		write.OperationID = s.op.ID
		sessionID := resolved.Session.ID

		current, err := scanTask(tx.QueryRowContext(ctx, selectTask+` WHERE task_id = ?`, taskID))
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return taskNotFound(taskID)
		case err != nil:
			return readFailed("the task", taskID, err)
		}

		if expectRevision > 0 && current.Revision != expectRevision {
			return revisionConflict(current, expectRevision)
		}

		target := TaskTarget(taskID)
		held, hasLease, err := unreleasedLeaseOn(ctx, tx, current.ProjectID, target, now)
		if err != nil {
			return err
		}
		if hasLease && held.Status == LeaseActive && held.Holder != sessionID {
			return leaseConflict(held)
		}

		if !CanTransition(current.State, to) {
			return transitionNotAvailable(taskID, current.State, to, current.ClaimedBy)
		}
		if to == StateBlocked && reason == "" {
			return blockedReasonMissing(taskID)
		}

		updated := current
		updated.State = to
		updated.UpdatedAt = now
		updated.Revision = current.Revision + 1

		// A block carries its reason; anything else clears it, so a reader never
		// meets a reason belonging to a block that was lifted.
		updated.BlockedReason = ""
		if to == StateBlocked {
			updated.BlockedReason = reason
		}

		// The lease follows the destination (design §7). A move into CLAIMED or
		// a working state is a claim: the mover acquires or renews, and becomes
		// the claimant. OPEN is the release path and clears the claimant; the
		// terminal states release and leave the claimant as attribution of the
		// last claim (decision D-68).
		switch to {
		case StateClaimed, StateInProgress, StateBlocked, StateReadyToComplete:
			acquired, err := s.acquireIn(ctx, tx, current.ProjectID, sessionID, target, now)
			if err != nil {
				return err
			}
			move.Lease = &acquired.Lease
			move.Superseded = acquired.Superseded
			updated.ClaimedBy = sessionID
		case StateOpen:
			if hasLease {
				if _, err := closeIn(ctx, tx, held, now, closeReason(held, ReleaseReasonReleased)); err != nil {
					return err
				}
			}
			updated.ClaimedBy = ""
		case StateCompleted, StateAbandoned:
			if hasLease {
				if _, err := closeIn(ctx, tx, held, now, closeReason(held, ReleaseReasonFinished)); err != nil {
					return err
				}
			}
		}

		result, err := tx.ExecContext(ctx,
			`UPDATE tasks SET state = ?, blocked_reason = ?, claimed_by = ?, updated_at = ?,
				revision = revision + 1
			 WHERE task_id = ? AND revision = ?`,
			string(updated.State), nullable(updated.BlockedReason), nullable(updated.ClaimedBy),
			app.FormatTime(updated.UpdatedAt), taskID, current.Revision)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return err
		} else if affected != 1 {
			return fmt.Errorf("the task's revision moved under the write lock: %d rows updated at revision %d", affected, current.Revision)
		}

		move.Task = updated
		return s.record(ctx, tx, "task state", hash, write, move, now)
	})
	if err != nil {
		return Move{}, Write{}, s.adopt(ctx, "the task state", taskID, err)
	}
	write.Timing = stats

	return move, write, nil
}

// closeReason is the reason a release path writes on the tenure it closes: the
// path's own, unless the tenure had already run out — a tenure that expired is
// recorded as expired whoever closes it and whichever way, so the history says
// what happened to it rather than what the closer was doing.
func closeReason(held Lease, pathReason string) string {
	if held.Status == LeaseExpired {
		return ReleaseReasonExpired
	}
	return pathReason
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

// Noted is what WriteCheckpoint reports: the checkpoint, and the task's lease
// as the write left it — renewed when the writer held it, closed with reason
// `handoff` when the writer held it and was leaving, nil when the writer held
// nothing (decision D-78). A note is not a claim: a checkpoint by any other
// session is written and touches no lease.
type Noted struct {
	Checkpoint Checkpoint `json:"checkpoint"`
	Lease      *Lease     `json:"lease"`
}

// WriteCheckpoint appends a note to a task.
//
// Append-only (decision D-59). There is no Update and no Delete on this table,
// and the absence is the feature: a handover note that could be rewritten after
// the fact is one the arriving agent cannot rely on.
//
// The task is read whole rather than probed for its id, since MR-004: the
// probe selected only task_id, so a task row `task show`, `task list` and
// `task state` refused as undecodable still took a checkpoint (MR-003 §7's
// second LOW finding). It is also the read that says which project the lease
// lives in.
func (s *Store) WriteCheckpoint(ctx context.Context, taskID string, by Attribution, workspaceID, note string, handoff bool) (Noted, Write, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return Noted{}, Write{}, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"a checkpoint needs a note",
			"Nothing was written.",
			"Re-run with --note saying where the work stands.",
		)
	}

	if err := s.refuseInvalidOperation(); err != nil {
		return Noted{}, Write{}, err
	}
	hash, err := requestHash("checkpoint write", struct {
		Task      string `json:"task"`
		By        string `json:"by"`
		Workspace string `json:"workspace"`
		Note      string `json:"note"`
		Handoff   bool   `json:"handoff"`
	}{taskID, attributionKey(by), workspaceID, note, handoff})
	if err != nil {
		return Noted{}, Write{}, err
	}

	checkpoint := Checkpoint{
		ID:          identity.NewID(checkpointIDPrefix),
		TaskID:      taskID,
		WorkspaceID: workspaceID,
		Note:        note,
		Handoff:     handoff,
	}

	var (
		noted Noted
		write Write
	)
	stats, err := storage.InTxMeasured(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		if replayed, found, err := s.replay(ctx, tx, "checkpoint write", hash, &noted); err != nil {
			return err
		} else if found {
			write = replayed
			return nil
		}

		// The clock is read under the write lock, as Transition reads it: the
		// row's stamp and the lease judgment below are about the same instant,
		// and a tenure that ran out while this write waited on the lock is
		// judged expired rather than renewed by a stamp taken before the wait
		// (TASK-04's Breaker; decision D-65). A session minted here starts at
		// this same instant.
		checkpoint.CreatedAt = s.clock.Now().UTC()

		resolved, err := s.attribute(ctx, tx, by, checkpoint.CreatedAt)
		if err != nil {
			return err
		}
		write = resolved
		write.OperationID = s.op.ID
		checkpoint.SessionID = resolved.Session.ID

		task, err := scanTask(tx.QueryRowContext(ctx, selectTask+` WHERE task_id = ?`, taskID))
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
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO checkpoints (checkpoint_id, task_id, session_id, workspace_id,
				note, handoff, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			checkpoint.ID, checkpoint.TaskID, checkpoint.SessionID, checkpoint.WorkspaceID,
			checkpoint.Note, handoffValue, app.FormatTime(checkpoint.CreatedAt)); err != nil {
			return err
		}
		noted.Checkpoint = checkpoint

		// The holder's note renews; the holder's handoff releases; anyone
		// else's touches nothing (decision D-78). The instant is the row's own
		// stamp, which is the instant a session minted for this write began.
		held, hasLease, err := unreleasedLeaseOn(ctx, tx, task.ProjectID, TaskTarget(taskID), checkpoint.CreatedAt)
		if err != nil {
			return err
		}
		if hasLease && held.Status == LeaseActive && held.Holder == checkpoint.SessionID {
			var after Lease
			if handoff {
				after, err = closeIn(ctx, tx, held, checkpoint.CreatedAt, ReleaseReasonHandoff)
			} else {
				after, err = renewIn(ctx, tx, held, checkpoint.CreatedAt)
			}
			if err != nil {
				return err
			}
			noted.Lease = &after
		}
		return s.record(ctx, tx, "checkpoint write", hash, write, noted, checkpoint.CreatedAt)
	})
	if err != nil {
		return Noted{}, Write{}, s.adopt(ctx, "the checkpoint", checkpoint.ID, err)
	}
	write.Timing = stats

	return noted, write, nil
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
	// The task and its newest tenure are read by one statement, so the pair
	// is one snapshot: read as two, a takeover committing between them put a
	// claimant beside another session's active lease on the wire — a
	// decision D-68 violation the rows never had (TASK-04's Breaker, seven
	// in four thousand reads under a takeover storm). The newest tenure is
	// reported in whatever status it is, so a claimant whose lease has run
	// out is shown with the tenure's end rather than as if the claim still
	// bound.
	task, lease, hasLease, err := scanTaskWithNewestLease(s.db.QueryRowContext(ctx,
		selectTaskWithNewestLease+` WHERE t.task_id = ?`, taskID), s.clock.Now().UTC())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Handover{}, taskNotFound(taskID)
	case err != nil:
		return Handover{}, readFailed("the task", taskID, err)
	}

	handover := Handover{Task: task}
	if hasLease {
		handover.Lease = &lease
	}

	checkpoint, err := s.LastCheckpoint(ctx, taskID)
	switch {
	case errors.Is(err, ErrCheckpointNotFound):
	case err != nil:
		return Handover{}, err
	default:
		handover.Checkpoint = &checkpoint
	}
	return handover, nil
}

// selectTaskWithNewestLease is Handover's one statement: the task's columns
// and, LEFT JOINed, the columns of the newest lease row on it, NULL when there
// has never been one.
const selectTaskWithNewestLease = `SELECT t.task_id, t.project_id, t.title, t.state, t.blocked_reason,
	t.opened_by, t.claimed_by, t.created_at, t.updated_at, t.revision,
	l.lease_id, l.project_id, l.target_kind, l.target_key, l.holder,
	l.acquired_at, l.renewed_at, l.expires_at, l.released_at, l.release_reason
	FROM tasks t
	LEFT JOIN leases l ON l.rowid = (
		SELECT max(rowid) FROM leases
		 WHERE project_id = t.project_id AND target_kind = 'task' AND target_key = t.task_id)`

// scanTaskWithNewestLease decodes the joined row; the bool reports whether a
// lease was joined.
func scanTaskWithNewestLease(row rowScanner, now time.Time) (Task, Lease, bool, error) {
	var (
		task       Task
		state      string
		reason     sql.NullString
		claimedBy  sql.NullString
		createdAt  string
		updatedAt  string
		leaseID    sql.NullString
		lProject   sql.NullString
		kind       sql.NullString
		key        sql.NullString
		holder     sql.NullString
		acquiredAt sql.NullString
		renewedAt  sql.NullString
		expiresAt  sql.NullString
		releasedAt sql.NullString
		lReason    sql.NullString
	)
	if err := row.Scan(&task.ID, &task.ProjectID, &task.Title, &state, &reason,
		&task.OpenedBy, &claimedBy, &createdAt, &updatedAt, &task.Revision,
		&leaseID, &lProject, &kind, &key, &holder,
		&acquiredAt, &renewedAt, &expiresAt, &releasedAt, &lReason); err != nil {
		return Task{}, Lease{}, false, err
	}

	task.State = State(state)
	task.BlockedReason = reason.String
	task.ClaimedBy = claimedBy.String
	var err error
	if task.CreatedAt, err = app.ParseTime(createdAt); err != nil {
		return Task{}, Lease{}, false, fmt.Errorf("task %s created_at: %w", task.ID, err)
	}
	if task.UpdatedAt, err = app.ParseTime(updatedAt); err != nil {
		return Task{}, Lease{}, false, fmt.Errorf("task %s updated_at: %w", task.ID, err)
	}
	if !leaseID.Valid {
		return task, Lease{}, false, nil
	}

	lease := Lease{
		ID:            leaseID.String,
		ProjectID:     lProject.String,
		TargetKind:    TargetKind(kind.String),
		TargetKey:     key.String,
		Holder:        holder.String,
		ReleaseReason: lReason.String,
	}
	if lease.AcquiredAt, err = app.ParseTime(acquiredAt.String); err != nil {
		return Task{}, Lease{}, false, fmt.Errorf("lease %s acquired_at: %w", lease.ID, err)
	}
	if lease.RenewedAt, err = app.ParseTime(renewedAt.String); err != nil {
		return Task{}, Lease{}, false, fmt.Errorf("lease %s renewed_at: %w", lease.ID, err)
	}
	if lease.ExpiresAt, err = app.ParseTime(expiresAt.String); err != nil {
		return Task{}, Lease{}, false, fmt.Errorf("lease %s expires_at: %w", lease.ID, err)
	}
	if releasedAt.Valid {
		released, err := app.ParseTime(releasedAt.String)
		if err != nil {
			return Task{}, Lease{}, false, fmt.Errorf("lease %s released_at: %w", lease.ID, err)
		}
		lease.ReleasedAt = &released
	}
	lease.Status = lease.statusAt(now)
	return task, lease, true, nil
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
	opened_by, claimed_by, created_at, updated_at, revision FROM tasks`

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
		&task.OpenedBy, &claimedBy, &createdAt, &updatedAt, &task.Revision); err != nil {
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
