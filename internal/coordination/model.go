package coordination

import "time"

// Session is one agent's run against one workspace.
//
// It is minted and never resumed (decision D-56). Nothing here is a handle the
// next agent reattaches to: the next agent gets a session of its own and finds
// the work through the Task and the Checkpoint. What a session is *for* is
// attribution — which run opened this task, which run left this note — and
// attribution is only useful if the identity is never reused.
type Session struct {
	ID          string    `json:"session_id"`
	WorkspaceID string    `json:"workspace_id"`
	Label       string    `json:"label,omitempty"`
	StartedAt   time.Time `json:"started_at"`
}

// Task is the unit of work two sequential agents share.
//
// It belongs to a project rather than to a workspace (decision D-60): spec §61
// shares task ownership at project level so two worktrees of one repository see
// one task.
//
// ClaimedBy records which session claimed it and nothing more. There is no
// expiry, no renewal, and no refusal on the grounds of identity — a second
// session moving a task the first one claimed is the handover working, not a
// conflict. MR-004 adds the lease that makes ownership enforceable when two
// agents are running at once.
type Task struct {
	ID    string `json:"task_id"`
	Title string `json:"title"`
	State State  `json:"state"`
	// BlockedReason is set when a task enters BLOCKED and cleared when it
	// leaves, so a reader never meets a reason belonging to a block that was
	// lifted.
	BlockedReason string    `json:"blocked_reason,omitempty"`
	ProjectID     string    `json:"project_id"`
	OpenedBy      string    `json:"opened_by"`
	ClaimedBy     string    `json:"claimed_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Checkpoint is a note one session left on one task.
//
// Append-only (decision D-59): never updated, never deleted. It is the third of
// the three questions the arriving agent has — what was being done, how far it
// got, and what the last agent said about it — and the only one whose answer
// nothing but a human or an agent can produce.
//
// Handoff marks the checkpoint written on the way out. Nothing in MR-003
// branches on it; it is recorded because a column added later would need a
// default that lies about every row already written.
type Checkpoint struct {
	ID          string    `json:"checkpoint_id"`
	TaskID      string    `json:"task_id"`
	SessionID   string    `json:"session_id"`
	WorkspaceID string    `json:"workspace_id"`
	Note        string    `json:"note"`
	Handoff     bool      `json:"handoff"`
	CreatedAt   time.Time `json:"created_at"`
}

// Handover is the whole answer to "what was happening here": a task and the
// newest thing said about it.
//
// Checkpoint is a pointer because a task nobody has checkpointed yet is a normal
// state rather than a failure, and "there is no note" has to be distinguishable
// from "there is an empty note". It marshals as null, which is what a reader
// branching on its presence needs.
type Handover struct {
	Task       Task        `json:"task"`
	Checkpoint *Checkpoint `json:"checkpoint"`
}

// CheckpointRef is the narrow view of a checkpoint that `status` publishes:
// which task, which session, and when. The note itself is deliberately absent —
// status is a fixed-size report, and an agent's free text is the one field in
// this package with no bound on its length.
type CheckpointRef struct {
	TaskID    string    `json:"task_id"`
	SessionID string    `json:"session_id"`
	At        time.Time `json:"at"`
}

// Summary is what `status` publishes about coordination.
//
// The three counts are the states a reader can act on: work waiting, work
// happening, work stuck. COMPLETED and ABANDONED are deliberately not counted —
// they only grow, so a number that never goes down would say nothing about the
// repository's current state and would make the block look busier every week.
type Summary struct {
	Open           int            `json:"tasks_open"`
	InProgress     int            `json:"tasks_in_progress"`
	Blocked        int            `json:"tasks_blocked"`
	LastCheckpoint *CheckpointRef `json:"last_checkpoint"`
}
