package dashboard

import (
	"time"

	"github.com/PsyChaos/mindrail/internal/status"
)

const (
	maxTasks         = 200
	maxSessions      = 120
	maxLeases        = 300
	maxCheckpoints   = 200
	maxEvidence      = 200
	maxProfiles      = 100
	maxJEVRoutes     = 100
	maxAgentRuntimes = 120
	maxSessionTasks  = 20
	maxIDRunes       = 128
	maxTextRunes     = 1024
)

type Snapshot struct {
	Sequence       uint64          `json:"sequence"`
	GeneratedAt    time.Time       `json:"generated_at"`
	Dashboard      DashboardState  `json:"dashboard"`
	Project        Project         `json:"project"`
	Summary        Summary         `json:"summary"`
	Tasks          []Task          `json:"tasks"`
	Sessions       []Session       `json:"sessions"`
	Leases         []Lease         `json:"leases"`
	Checkpoints    []Checkpoint    `json:"checkpoints"`
	Evidence       []Evidence      `json:"evidence"`
	Profiles       []Profile       `json:"profiles"`
	Readiness      ReadinessView   `json:"readiness"`
	CI             ProviderState   `json:"ci"`
	Merge          ProviderState   `json:"merge"`
	JEV            JEVState        `json:"jev"`
	JEVRouteEvents []JEVRouteEvent `json:"jev_route_events"`
	AgentRuntimes  []AgentRuntime  `json:"agent_runtimes"`
	Completion     CompletionState `json:"completion"`
	Capabilities   Capabilities    `json:"capabilities"`
	Truncated      map[string]bool `json:"truncated,omitempty"`
}

type DashboardState struct {
	StartedAt     time.Time `json:"started_at"`
	UptimeSeconds int64     `json:"uptime_seconds"`
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
	ID                    string        `json:"id"`
	WorkspaceID           string        `json:"workspace_id"`
	Label                 string        `json:"label,omitempty"`
	StartedAt             time.Time     `json:"started_at"`
	Engaged               bool          `json:"engaged"`
	TaskCount             int           `json:"task_count"`
	LeaseCount            int           `json:"lease_count"`
	ActivityStatus        string        `json:"activity_status"`
	CurrentTaskCount      int           `json:"current_task_count"`
	CurrentTasks          []SessionTask `json:"current_tasks"`
	CurrentTasksTruncated bool          `json:"current_tasks_truncated"`
	LatestActivityAt      *time.Time    `json:"latest_activity_at,omitempty"`
	LatestLeaseRenewedAt  *time.Time    `json:"latest_lease_renewed_at,omitempty"`
	NextLeaseExpiresAt    *time.Time    `json:"next_lease_expires_at,omitempty"`
}

type SessionTask struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updated_at"`
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
	Configured         bool       `json:"configured"`
	Source             string     `json:"source"`
	Provider           string     `json:"provider"`
	Mode               string     `json:"mode"`
	Used               bool       `json:"used"`
	RouteCount         int        `json:"route_count"`
	AcceptedRouteCount int        `json:"accepted_route_count"`
	LatestRouteAt      *time.Time `json:"latest_route_at,omitempty"`
}

// RouteDimension intentionally exposes only bounded numeric metadata. Candidate
// IDs and descriptions are caller-controlled and must never enter a snapshot.
type RouteDimension struct {
	CandidateCount  int  `json:"candidate_count"`
	SelectedOrdinal *int `json:"selected_ordinal,omitempty"`
	ConfidenceMilli *int `json:"confidence_milli,omitempty"`
}

type JEVRouteEvent struct {
	ID               string         `json:"id"`
	WorkspaceID      string         `json:"workspace_id"`
	TaskID           string         `json:"task_id"`
	SessionID        string         `json:"session_id"`
	Provider         string         `json:"provider"`
	Model            string         `json:"model"`
	Version          int            `json:"version"`
	Status           string         `json:"status"`
	Reason           string         `json:"reason"`
	CredentialSource string         `json:"credential_source"`
	StartedAt        time.Time      `json:"started_at"`
	EndedAt          time.Time      `json:"ended_at"`
	DurationMS       int64          `json:"duration_ms"`
	Tools            RouteDimension `json:"tools"`
	Agents           RouteDimension `json:"agents"`
	Models           RouteDimension `json:"models"`
	Efforts          RouteDimension `json:"efforts"`
}

// AgentRuntime is connection telemetry, not model/process/thought liveness.
// ClientFamily is a server-owned canonical projection derived from self-reported
// MCP ClientInfo. Raw client title and version never cross this public boundary.
type AgentRuntime struct {
	ID              string            `json:"id"`
	WorkspaceID     string            `json:"workspace_id"`
	TaskID          string            `json:"task_id"`
	SessionID       string            `json:"session_id"`
	ClientFamily    string            `json:"client_name"`
	Host            string            `json:"host,omitempty"`
	AgentKind       string            `json:"agent_kind,omitempty"`
	AgentType       string            `json:"agent_type,omitempty"`
	ParentRuntimeID string            `json:"parent_runtime_id,omitempty"`
	StartedAt       time.Time         `json:"started_at"`
	LastHeartbeatAt time.Time         `json:"last_heartbeat_at"`
	LastActivityAt  time.Time         `json:"last_activity_at"`
	Sequence        int64             `json:"sequence"`
	EndedAt         *time.Time        `json:"ended_at,omitempty"`
	EndReason       string            `json:"end_reason,omitempty"`
	Status          string            `json:"status"`
	Telemetry       RuntimeTelemetry  `json:"telemetry"`
	Continuity      RuntimeContinuity `json:"continuity"`
}

type RuntimeTelemetry struct {
	State        string     `json:"state"`
	Model        string     `json:"model,omitempty"`
	Effort       string     `json:"effort,omitempty"`
	ContextUsed  *int64     `json:"context_used,omitempty"`
	ContextLimit *int64     `json:"context_limit,omitempty"`
	UsedPercent  *float64   `json:"used_percent,omitempty"`
	Source       string     `json:"source,omitempty"`
	Confidence   string     `json:"confidence,omitempty"`
	ObservedAt   *time.Time `json:"observed_at,omitempty"`
	Revision     int64      `json:"revision,omitempty"`
}

type RuntimeContinuity struct {
	IntentID            string     `json:"intent_id,omitempty"`
	State               string     `json:"state"`
	FailureCode         string     `json:"failure_code,omitempty"`
	CheckpointID        string     `json:"checkpoint_id,omitempty"`
	HostOperationID     string     `json:"host_operation_id,omitempty"`
	SuccessorSessionID  string     `json:"successor_session_id,omitempty"`
	ThresholdPercent    int        `json:"threshold_percent,omitempty"`
	HardProtection      bool       `json:"hard_protection"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
	PhaseElapsedSeconds int64      `json:"phase_elapsed_seconds,omitempty"`
	Stalled             bool       `json:"stalled"`
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
