package continuity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

type Checkpointer interface {
	CheckpointContinuity(context.Context, Intent) (string, error)
}

type ObserveResult struct {
	Intent   Intent
	Decision Decision
	Envelope *BootstrapEnvelope
}

type Service struct {
	store        Store
	policy       Policy
	host         HostBridge
	checkpointer Checkpointer
}

func NewService(store Store, policy Policy, host HostBridge, checkpointer Checkpointer) (*Service, error) {
	if store == nil || host == nil || checkpointer == nil {
		return nil, errors.New("continuity service needs store, host and checkpointer")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &Service{store: store, policy: policy, host: host, checkpointer: checkpointer}, nil
}

// Observe applies one monotonic host observation. It durably checkpoints before
// asking the host to prepare a successor. Unsupported hosts fail closed to a
// resumable MANUAL_REQUIRED intent.
func (s *Service) Observe(ctx context.Context, intentID string, observation Observation) (ObserveResult, error) {
	for attempt := 0; attempt < 3; attempt++ {
		current, err := s.store.Get(ctx, intentID)
		if err != nil {
			return ObserveResult{}, err
		}
		if current.State == StateCheckpointed || current.State == StateSpawnRequested {
			return s.prepare(ctx, current, Decision{State: current.State, UsedBasisPoints: current.LastUsedBasisPoints, RequestHandoff: true})
		}
		next, decision, err := s.policy.Evaluate(current, observation)
		if err != nil {
			return ObserveResult{}, err
		}
		if decision.Ignored {
			return ObserveResult{Intent: current, Decision: decision}, nil
		}
		saved, err := s.store.Save(ctx, next, current.Revision)
		if errors.Is(err, ErrConflict) {
			continue
		}
		if err != nil {
			return ObserveResult{}, err
		}
		if !decision.RequestHandoff {
			return ObserveResult{Intent: saved, Decision: decision}, nil
		}
		return s.prepare(ctx, saved, decision)
	}
	return ObserveResult{}, ErrConflict
}

func (s *Service) prepare(ctx context.Context, intent Intent, decision Decision) (ObserveResult, error) {
	if intent.State == StateWarned || (intent.State == StateCheckpointed && intent.CheckpointID == "") {
		checkpointID, err := s.checkpointer.CheckpointContinuity(ctx, intent)
		if err != nil {
			return ObserveResult{}, fmt.Errorf("continuity checkpoint: %w", err)
		}
		intent.CheckpointID = checkpointID
		intent.State = StateCheckpointed
		intent, err = s.store.Save(ctx, intent, intent.Revision)
		if err != nil {
			return ObserveResult{}, err
		}
	}
	checkpointID := intent.CheckpointID
	caps := s.host.Capabilities(ctx)
	if !caps.ContextTelemetry || !caps.IdempotentPrepare || !caps.SuccessorActivation || !caps.LocalAutoSpawnApproved {
		intent.State = StateManualRequired
		intent.FailureCode = capabilityFailure(caps)
		saved, saveErr := s.store.Save(ctx, intent, intent.Revision)
		return ObserveResult{Intent: saved, Decision: decision}, saveErr
	}

	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return ObserveResult{}, fmt.Errorf("continuity takeover token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	hash := sha256.Sum256(rawToken)
	intent.State = StateSpawnRequested
	intent.TakeoverTokenHash = hash[:]
	requested, err := s.store.Save(ctx, intent, intent.Revision)
	if err != nil {
		return ObserveResult{}, err
	}
	envelope := BootstrapEnvelope{IntentID: requested.ID, TaskID: requested.TaskID, CheckpointID: checkpointID, TakeoverToken: token}
	prepared, err := s.host.PrepareSuccessor(ctx, envelope)
	if err != nil {
		requested.State = StateManualRequired
		requested.FailureCode = "host_prepare_failed"
		failed, saveErr := s.store.Save(ctx, requested, requested.Revision)
		if saveErr != nil {
			return ObserveResult{}, saveErr
		}
		return ObserveResult{Intent: failed, Decision: decision, Envelope: &envelope}, nil
	}
	requested.State = StateSpawnReady
	requested.HostOperationID = prepared.OperationID
	ready, err := s.store.Save(ctx, requested, requested.Revision)
	if err != nil {
		return ObserveResult{}, err
	}
	return ObserveResult{Intent: ready, Decision: decision, Envelope: &envelope}, nil
}

func capabilityFailure(caps HostCapabilities) string {
	switch {
	case !caps.ContextTelemetry:
		return "context_telemetry_unavailable"
	case !caps.IdempotentPrepare:
		return "idempotent_prepare_unavailable"
	case !caps.SuccessorActivation:
		return "successor_activation_unavailable"
	default:
		return "auto_spawn_not_approved"
	}
}
