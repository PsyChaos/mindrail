package dashboard

import (
	"time"

	"github.com/PsyChaos/mindrail/internal/status"
)

const (
	maxTasks       = 200
	maxSessions    = 120
	maxLeases      = 300
	maxCheckpoints = 200
	maxEvidence    = 200
	maxProfiles    = 100
	maxIDRunes     = 128
	maxTextRunes   = 1024
)

type Snapshot struct {
	Sequence     uint64          `json:"sequence"`
	GeneratedAt  time.Time       `json:"generated_at"`
	Project      Project         `json:"project"`
	Summary      Summary         `json:"summary"`
	Tasks        []Task          `json:"tasks"`
	Sessions     []Session       `json:"sessions"`
	Leases       []Lease         `json:"leases"`
	Checkpoints  []Checkpoint    `json:"checkpoints"`
	Evidence     []Evidence      `json:"evidence"`
	Profiles     []Profile       `json:"profiles"`
	Readiness    ReadinessView   `json:"readiness"`
	CI           ProviderState   `json:"ci"`
	Merge        ProviderState   `json:"merge"`
	JEV          JEVState        `json:"jev"`
	Completion   CompletionState `json:"completion"`
	Capabilities Capabilities    `json:"capabilities"`
	Truncated    map[string]bool `json:"truncated,omitempty"`
}

type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	WorkspaceID    string `json:"workspace_id"`
	LinkedWorktree bool   `json:"linked_worktree"`
}

type ReadinessView struct {
	Readiness  status.Readiness              `json:"readiness"`
	Components map[string]ReadinessComponent `json:"components"`
}

type ReadinessComponent struct {
	State   string `json:"state"`
	Summary string `json:"summary"`
	Pending *int   `json:"pending,omitempty"`
	Failed  *int   `json:"failed,omitempty"`
	Units   *int   `json:"units,omitempty"`
}

type Summary struct {
	States        map[string]int `json:"states"`
	ActiveLeases  int            `json:"active_leases"`
	ExpiredLeases int            `json:"expired_leases"`
	AgentsEngaged int            `json:"agents_engaged"`
	EvidencePass  int            `json:"evidence_pass"`
	EvidenceFail  int            `json:"evidence_fail"`
}

type Task struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	State         string    `json:"state"`
	BlockedReason string    `json:"blocked_reason,omitempty"`
	OpenedBy      string    `json:"opened_by"`
	ClaimedBy     string    `json:"claimed_by,omitempty"`
	Revision      int64     `json:"revision"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Session struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Label       string    `json:"label,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	Engaged     bool      `json:"engaged"`
	TaskCount   int       `json:"task_count"`
	LeaseCount  int       `json:"lease_count"`
}

type Lease struct {
	ID            string     `json:"id"`
	TargetKind    string     `json:"target_kind"`
	TargetKey     string     `json:"target_key"`
	Holder        string     `json:"holder"`
	Status        string     `json:"status"`
	AcquiredAt    time.Time  `json:"acquired_at"`
	RenewedAt     time.Time  `json:"renewed_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	ReleasedAt    *time.Time `json:"released_at,omitempty"`
	ReleaseReason string     `json:"release_reason,omitempty"`
}

type Checkpoint struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	SessionID   string    `json:"session_id"`
	WorkspaceID string    `json:"workspace_id"`
	Handoff     bool      `json:"handoff"`
	CreatedAt   time.Time `json:"created_at"`
}

type Evidence struct {
	ID           string    `json:"id"`
	Profile      string    `json:"profile"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	ExitCode     int       `json:"exit_code"`
	SnapshotHash string    `json:"snapshot_hash"`
	CreatedAt    time.Time `json:"created_at"`
}

type Profile struct {
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	ScopeCount   int        `json:"scope_count"`
	CommandCount int        `json:"command_count"`
	LatestStatus string     `json:"latest_status"`
	LatestAt     *time.Time `json:"latest_at,omitempty"`
}

type ProviderState struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
	Source string `json:"source,omitempty"`
}

type JEVState struct {
	Configured bool   `json:"configured"`
	Source     string `json:"source"`
	Provider   string `json:"provider"`
	Mode       string `json:"mode"`
}

type CompletionState struct {
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Findings int    `json:"findings"`
}

type Capabilities struct {
	ReadOnly            bool   `json:"read_only"`
	LiveTransport       string `json:"live_transport"`
	TaskRevisionHistory string `json:"task_revision_history"`
	CheckpointNotes     string `json:"checkpoint_notes"`
	SensitiveData       string `json:"sensitive_data"`
	ReadinessFreshness  string `json:"readiness_freshness"`
}
