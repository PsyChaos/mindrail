package continuity

import (
	"context"
	"errors"
)

var ErrHostCapabilityUnavailable = errors.New("continuity host capability unavailable")

type HostCapabilities struct {
	ContextTelemetry       bool
	IdempotentPrepare      bool
	SuccessorActivation    bool
	LocalAutoSpawnApproved bool
}

type BootstrapEnvelope struct{ IntentID, TaskID, CheckpointID, TakeoverToken string }
type PreparedSuccessor struct{ OperationID string }

// HostBridge is implemented by a client-specific, host-local adapter. Generic
// MCP wiring must use an unavailable bridge instead of overstating capability.
type HostBridge interface {
	Capabilities(context.Context) HostCapabilities
	PrepareSuccessor(context.Context, BootstrapEnvelope) (PreparedSuccessor, error)
	ActivateSuccessor(context.Context, string) error
}

type UnavailableHost struct{}

func (UnavailableHost) Capabilities(context.Context) HostCapabilities { return HostCapabilities{} }
func (UnavailableHost) PrepareSuccessor(context.Context, BootstrapEnvelope) (PreparedSuccessor, error) {
	return PreparedSuccessor{}, ErrHostCapabilityUnavailable
}
func (UnavailableHost) ActivateSuccessor(context.Context, string) error {
	return ErrHostCapabilityUnavailable
}
