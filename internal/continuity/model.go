// Package continuity coordinates context-pressure observations and durable
// handoff intents without assuming that an MCP client can create a successor
// conversation.
package continuity

import "time"

type State string

const (
	StateObserving      State = "OBSERVING"
	StateWarned         State = "WARNED"
	StateCheckpointed   State = "CHECKPOINTED"
	StateSpawnRequested State = "SPAWN_REQUESTED"
	StateSpawnReady     State = "SPAWN_READY"
	StateHandedOff      State = "HANDED_OFF"
	StateClaimed        State = "CLAIMED"
	StateResumed        State = "RESUMED"
	StateCompleted      State = "COMPLETED"
	StateManualRequired State = "MANUAL_REQUIRED"
	StateCancelled      State = "CANCELLED"
	StateExpired        State = "EXPIRED"
)

type IntentKind string

const (
	IntentSameTask IntentKind = "SAME_TASK"
	IntentNextTask IntentKind = "NEXT_TASK"
)

type Confidence string

const (
	ConfidenceMeasured Confidence = "measured"
	ConfidenceReported Confidence = "reported"
	ConfidenceInferred Confidence = "inferred"
)

// Observation is safe operational metadata reported by a trusted host bridge.
// Sequence is scoped to one runtime and must increase monotonically.
type Observation struct {
	RuntimeID    string
	Sequence     int64
	Model        string
	Effort       string
	ContextUsed  int64
	ContextLimit int64
	ObservedAt   time.Time
	Source       string
	Confidence   Confidence
}

func (o Observation) HasContextPressure() bool {
	return o.ContextUsed > 0 && o.ContextLimit > 0 && o.ContextUsed <= o.ContextLimit
}

func (o Observation) UsedBasisPoints() int {
	if !o.HasContextPressure() {
		return -1
	}
	return int(o.ContextUsed * 10_000 / o.ContextLimit)
}

type Intent struct {
	ID, ProjectID, WorkspaceID, TaskID string
	PredecessorSessionID               string
	PredecessorRunHash                 []byte
	Kind                               IntentKind
	TargetTaskID                       string
	State                              State
	Revision                           int64
	LastObservationSequence            int64
	LastUsedBasisPoints                int
	ConsecutiveHandoffObservations     int
	CheckpointID, HostOperationID      string
	TakeoverTokenHash                  []byte
	SuccessorRunHash                   []byte
	SuccessorSessionID                 string
	FailureCode                        string
	CreatedAt, UpdatedAt, ExpiresAt    time.Time
	ActivatedAt                        *time.Time
}

type Decision struct {
	State           State
	UsedBasisPoints int
	Warn            bool
	Checkpoint      bool
	RequestHandoff  bool
	HardProtection  bool
	Ignored         bool
}
