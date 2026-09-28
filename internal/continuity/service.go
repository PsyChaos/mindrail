package continuity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Checkpointer interface {
	CheckpointContinuity(context.Context, Intent) (string, error)
	HandoffContinuity(context.Context, Intent) error
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

func (s *Service) Recover(ctx context.Context, intentID string) (ObserveResult, error) {
	intent, err := s.store.Get(ctx, intentID)
	if err != nil {
		return ObserveResult{}, err
	}
	if !intent.ExpiresAt.IsZero() && !intent.ExpiresAt.After(time.Now().UTC()) {
		intent.State = StateExpired
		expired, saveErr := s.store.Save(ctx, intent, intent.Revision)
		return ObserveResult{Intent: expired, Decision: Decision{State: StateExpired}}, saveErr
	}
	switch intent.State {
	case StateWarned:
		if intent.LastUsedBasisPoints < s.policy.HandoffUsedBasisPoints ||
			intent.ConsecutiveHandoffObservations < s.policy.ConsecutiveReports {
			return ObserveResult{Intent: intent, Decision: Decision{State: intent.State, Ignored: true}}, nil
		}
		return s.prepare(ctx, intent, Decision{State: intent.State, UsedBasisPoints: intent.LastUsedBasisPoints, RequestHandoff: true})
	case StateCheckpointed, StateSpawnRequested, StateSpawnReady, StateHandedOff:
		return s.prepare(ctx, intent, Decision{State: intent.State, UsedBasisPoints: intent.LastUsedBasisPoints, RequestHandoff: true})
	default:
		return ObserveResult{Intent: intent, Decision: Decision{State: intent.State, Ignored: true}}, nil
	}
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
		if !current.ExpiresAt.IsZero() && !current.ExpiresAt.After(time.Now().UTC()) {
			current.State = StateExpired
			expired, saveErr := s.store.Save(ctx, current, current.Revision)
			return ObserveResult{Intent: expired, Decision: Decision{State: StateExpired}}, saveErr
		}
		if current.State == StateCheckpointed || current.State == StateSpawnRequested ||
			current.State == StateSpawnReady || current.State == StateHandedOff {
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
	if !caps.ContextTelemetry || !caps.IdempotentPrepare || !caps.IdempotentActivate || !caps.SuccessorActivation || !caps.LocalAutoSpawnApproved {
		intent.State = StateManualRequired
		intent.FailureCode = capabilityFailure(caps)
		saved, saveErr := s.store.Save(ctx, intent, intent.Revision)
		return ObserveResult{Intent: saved, Decision: decision}, saveErr
	}

	requested := intent
	var err error
	envelope := BootstrapEnvelope{IntentID: intent.ID, TaskID: intent.TaskID, CheckpointID: checkpointID}
	if intent.State == StateCheckpointed {
		envelope.TakeoverToken, intent.TakeoverTokenHash, err = newTakeoverToken()
		if err != nil {
			return ObserveResult{}, err
		}
		intent.State = StateSpawnRequested
		requested, err = s.store.Save(ctx, intent, intent.Revision)
		if err != nil {
			return ObserveResult{}, err
		}
	}
	ready := requested
	if requested.State == StateSpawnRequested {
		if len(requested.TakeoverTokenHash) != sha256.Size {
			return ObserveResult{}, errors.New("continuity spawn request has no durable takeover proposal")
		}
		prepared, prepareErr := s.host.PrepareSuccessor(ctx, envelope)
		if errors.Is(prepareErr, ErrHostPreparationMissing) && envelope.TakeoverToken == "" {
			envelope.TakeoverToken, requested.TakeoverTokenHash, err = newTakeoverToken()
			if err != nil {
				return ObserveResult{}, err
			}
			requested, err = s.store.Save(ctx, requested, requested.Revision)
			if err != nil {
				return ObserveResult{}, err
			}
			prepared, prepareErr = s.host.PrepareSuccessor(ctx, envelope)
		}
		if prepareErr != nil {
			if errors.Is(prepareErr, ErrHostCapabilityUnavailable) {
				requested.State = StateManualRequired
				requested.FailureCode = "host_prepare_unavailable"
				failed, saveErr := s.store.Save(ctx, requested, requested.Revision)
				if saveErr != nil {
					return ObserveResult{}, saveErr
				}
				return ObserveResult{Intent: failed, Decision: decision, Envelope: &envelope}, nil
			}
			return ObserveResult{Intent: requested, Decision: decision, Envelope: &envelope}, fmt.Errorf("continuity host prepare: %w", prepareErr)
		}
		preparedHash := sha256.Sum256([]byte(prepared.TakeoverToken))
		if prepared.OperationID == "" || subtle.ConstantTimeCompare(preparedHash[:], requested.TakeoverTokenHash) != 1 {
			return ObserveResult{}, errors.New("continuity host returned an invalid prepared successor")
		}
		requested.State = StateSpawnReady
		requested.HostOperationID = prepared.OperationID
		ready, err = s.store.Save(ctx, requested, requested.Revision)
		if err != nil {
			return ObserveResult{}, err
		}
	}
	handedOff := ready
	if ready.State == StateSpawnReady {
		if err := s.checkpointer.HandoffContinuity(ctx, ready); err != nil {
			return ObserveResult{}, fmt.Errorf("continuity handoff: %w", err)
		}
		ready.State = StateHandedOff
		handedOff, err = s.store.Save(ctx, ready, ready.Revision)
		if err != nil {
			return ObserveResult{}, err
		}
	}
	if handedOff.State == StateHandedOff && handedOff.ActivatedAt == nil {
		if err := s.host.ActivateSuccessor(ctx, handedOff.HostOperationID); err != nil {
			if errors.Is(err, ErrHostCapabilityUnavailable) {
				handedOff.State = StateManualRequired
				handedOff.FailureCode = "host_activate_unavailable"
				failed, saveErr := s.store.Save(ctx, handedOff, handedOff.Revision)
				return ObserveResult{Intent: failed, Decision: decision, Envelope: &envelope}, saveErr
			}
			return ObserveResult{Intent: handedOff, Decision: decision, Envelope: &envelope}, fmt.Errorf("continuity host activate: %w", err)
		}
		handedOff, err = s.store.MarkActivated(ctx, handedOff.ID)
		if err != nil {
			return ObserveResult{}, err
		}
	}
	return ObserveResult{Intent: handedOff, Decision: decision, Envelope: &envelope}, nil
}

func newTakeoverToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("continuity takeover token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}

func capabilityFailure(caps HostCapabilities) string {
	switch {
	case !caps.ContextTelemetry:
		return "context_telemetry_unavailable"
	case !caps.IdempotentPrepare:
		return "idempotent_prepare_unavailable"
	case !caps.IdempotentActivate:
		return "idempotent_activate_unavailable"
	case !caps.SuccessorActivation:
		return "successor_activation_unavailable"
	default:
		return "auto_spawn_not_approved"
	}
}

func (s *Service) ClaimTakeover(ctx context.Context, intentID, token, runKey string) (Intent, error) {
	if intentID == "" || len(strings.TrimSpace(token)) < 32 || runKey == "" {
		return Intent{}, ErrInvalidToken
	}
	tokenHash := sha256.Sum256([]byte(token))
	runHash := sha256.Sum256([]byte(runKey))
	return s.store.ClaimTakeover(ctx, intentID, tokenHash[:], runHash[:])
}

func (s *Service) ResumeTakeover(ctx context.Context, intentID, runKey, sessionID string) (Intent, error) {
	if intentID == "" || runKey == "" || sessionID == "" {
		return Intent{}, ErrReservationLost
	}
	runHash := sha256.Sum256([]byte(runKey))
	return s.store.ResumeTakeover(ctx, intentID, runHash[:], sessionID)
}

func (s *Service) CompleteTakeover(ctx context.Context, intentID, runKey, sessionID string) (Intent, error) {
	if intentID == "" || runKey == "" || sessionID == "" {
		return Intent{}, ErrReservationLost
	}
	runHash := sha256.Sum256([]byte(runKey))
	return s.store.CompleteTakeover(ctx, intentID, runHash[:], sessionID)
}
