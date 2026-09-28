package continuity

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceFallsBackTruthfullyWhenHostCannotSpawn(t *testing.T) {
	store := &memoryStore{intent: Intent{ID: "CTI-1", TaskID: "TSK-1", State: StateWarned, Revision: 1, LastObservationSequence: 1, ConsecutiveHandoffObservations: 1}}
	service, err := NewService(store, DefaultPolicy(), UnavailableHost{}, fakeCheckpointer{id: "CHK-1"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Observe(t.Context(), "CTI-1", observation(2, 600_000, time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Intent.State != StateManualRequired || result.Intent.CheckpointID != "CHK-1" || result.Intent.FailureCode != "context_telemetry_unavailable" {
		t.Fatalf("result = %#v", result)
	}
}

func TestServicePreparesIdempotentSuccessorAfterCheckpoint(t *testing.T) {
	store := &memoryStore{intent: Intent{ID: "CTI-1", TaskID: "TSK-1", State: StateWarned, Revision: 1, LastObservationSequence: 1, ConsecutiveHandoffObservations: 1}}
	host := &fakeHost{caps: HostCapabilities{ContextTelemetry: true, IdempotentPrepare: true, IdempotentActivate: true, SuccessorActivation: true, LocalAutoSpawnApproved: true}}
	service, _ := NewService(store, DefaultPolicy(), host, fakeCheckpointer{id: "CHK-1"})
	result, err := service.Observe(t.Context(), "CTI-1", observation(2, 600_000, time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Intent.State != StateHandedOff || result.Intent.HostOperationID != "HOST-1" || result.Envelope == nil {
		t.Fatalf("result = %#v", result)
	}
	if host.prepared.CheckpointID != "CHK-1" || len(result.Intent.TakeoverTokenHash) != 32 {
		t.Fatalf("host=%#v intent=%#v", host, result.Intent)
	}
}

func TestServiceRecoversPersistedCheckpointedIntent(t *testing.T) {
	store := &memoryStore{intent: Intent{ID: "CTI-1", TaskID: "TSK-1", State: StateCheckpointed, Revision: 4, LastObservationSequence: 2}}
	service, _ := NewService(store, DefaultPolicy(), UnavailableHost{}, fakeCheckpointer{id: "CHK-RECOVERED"})
	result, err := service.Observe(t.Context(), "CTI-1", observation(2, 600_000, time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Intent.State != StateManualRequired || result.Intent.CheckpointID != "CHK-RECOVERED" {
		t.Fatalf("result = %#v", result)
	}
}

func TestServiceRecoversOnlyHandoffEligibleWarnedIntent(t *testing.T) {
	eligible := Intent{ID: "CTI-1", TaskID: "TSK-1", State: StateWarned, Revision: 4,
		LastObservationSequence: 2, LastUsedBasisPoints: 6000, ConsecutiveHandoffObservations: 2}
	store := &memoryStore{intent: eligible}
	service, _ := NewService(store, DefaultPolicy(), UnavailableHost{}, fakeCheckpointer{id: "CHK-RECOVERED"})
	result, err := service.Recover(t.Context(), "CTI-1")
	if err != nil || result.Intent.State != StateManualRequired || result.Intent.CheckpointID != "CHK-RECOVERED" {
		t.Fatalf("eligible result=%#v err=%v", result, err)
	}
	store.intent = Intent{ID: "CTI-2", TaskID: "TSK-1", State: StateWarned, Revision: 2,
		LastObservationSequence: 1, LastUsedBasisPoints: 5500, ConsecutiveHandoffObservations: 0}
	result, err = service.Recover(t.Context(), "CTI-2")
	if err != nil || !result.Decision.Ignored || result.Intent.State != StateWarned || result.Intent.CheckpointID != "" {
		t.Fatalf("warning-only result=%#v err=%v", result, err)
	}
}

func TestServiceRetriesAmbiguousHostSideEffects(t *testing.T) {
	store := &memoryStore{intent: Intent{ID: "CTI-1", TaskID: "TSK-1", State: StateCheckpointed, Revision: 4,
		CheckpointID: "CHK-1", LastObservationSequence: 2, LastUsedBasisPoints: 6000}}
	host := &fakeHost{caps: HostCapabilities{ContextTelemetry: true, IdempotentPrepare: true, IdempotentActivate: true, SuccessorActivation: true, LocalAutoSpawnApproved: true},
		prepareErrors: 1}
	service, _ := NewService(store, DefaultPolicy(), host, fakeCheckpointer{id: "CHK-1"})
	result, err := service.Recover(t.Context(), "CTI-1")
	if err == nil || result.Intent.State != StateSpawnRequested || !errors.Is(err, errAmbiguousHost) {
		t.Fatalf("ambiguous prepare result=%#v err=%v", result, err)
	}
	host.activateErrors = 1
	result, err = service.Recover(t.Context(), "CTI-1")
	if err == nil || result.Intent.State != StateHandedOff || !errors.Is(err, errAmbiguousHost) {
		t.Fatalf("ambiguous activate result=%#v err=%v", result, err)
	}
	result, err = service.Recover(t.Context(), "CTI-1")
	if err != nil || result.Intent.State != StateHandedOff || result.Intent.ActivatedAt == nil {
		t.Fatalf("recovered result=%#v err=%v", result, err)
	}
	if host.preparations != 2 || host.activations != 2 {
		t.Fatalf("prepare=%d activate=%d", host.preparations, host.activations)
	}
}

func TestServiceRecoversEveryPreparedHandoffCrashPoint(t *testing.T) {
	for _, state := range []State{StateSpawnRequested, StateSpawnReady, StateHandedOff} {
		t.Run(string(state), func(t *testing.T) {
			intent := Intent{ID: "CTI-1", TaskID: "TSK-1", State: state, Revision: 4,
				CheckpointID: "CHK-1", LastObservationSequence: 2, LastUsedBasisPoints: 6000}
			if state == StateSpawnRequested || state == StateSpawnReady || state == StateHandedOff {
				intent.TakeoverTokenHash = make([]byte, 32)
			}
			if state == StateSpawnReady || state == StateHandedOff {
				intent.HostOperationID = "HOST-1"
			}
			store := &memoryStore{intent: intent}
			host := &fakeHost{caps: HostCapabilities{ContextTelemetry: true, IdempotentPrepare: true, IdempotentActivate: true, SuccessorActivation: true, LocalAutoSpawnApproved: true}}
			handoffs := 0
			service, _ := NewService(store, DefaultPolicy(), host, fakeCheckpointer{id: "CHK-1", handoffs: &handoffs})
			result, err := service.Recover(t.Context(), "CTI-1")
			if err != nil || result.Intent.State != StateHandedOff {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if host.activations != 1 {
				t.Fatalf("activations=%d", host.activations)
			}
			wantPrepare, wantHandoff := 0, 0
			if state == StateSpawnRequested {
				wantPrepare = 1
			}
			if state != StateHandedOff {
				wantHandoff = 1
			}
			if host.preparations != wantPrepare || handoffs != wantHandoff {
				t.Fatalf("prepare=%d/%d handoff=%d/%d", host.preparations, wantPrepare, handoffs, wantHandoff)
			}
		})
	}
}

func TestServiceDoesNotReactivateMarkedSuccessorDuringRecovery(t *testing.T) {
	activatedAt := time.Now().UTC()
	store := &memoryStore{intent: Intent{ID: "CTI-1", TaskID: "TSK-1", State: StateHandedOff, Revision: 6,
		CheckpointID: "CHK-1", HostOperationID: "HOST-1", TakeoverTokenHash: make([]byte, 32), ActivatedAt: &activatedAt}}
	host := &fakeHost{caps: HostCapabilities{ContextTelemetry: true, IdempotentPrepare: true, IdempotentActivate: true, SuccessorActivation: true, LocalAutoSpawnApproved: true}}
	service, _ := NewService(store, DefaultPolicy(), host, fakeCheckpointer{id: "CHK-1"})
	result, err := service.Recover(t.Context(), "CTI-1")
	if err != nil || result.Intent.State != StateHandedOff {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if host.activations != 0 {
		t.Fatalf("marked successor activated %d times", host.activations)
	}
}

func TestServiceRejectsHostReplacementToken(t *testing.T) {
	store := &memoryStore{intent: Intent{ID: "CTI-1", TaskID: "TSK-1", State: StateCheckpointed, Revision: 4,
		CheckpointID: "CHK-1", LastObservationSequence: 2, LastUsedBasisPoints: 6000}}
	host := &fakeHost{caps: HostCapabilities{ContextTelemetry: true, IdempotentPrepare: true, IdempotentActivate: true, SuccessorActivation: true, LocalAutoSpawnApproved: true},
		replacementToken: "predictable-predictable-predictable-1"}
	service, _ := NewService(store, DefaultPolicy(), host, fakeCheckpointer{id: "CHK-1"})
	if _, err := service.Recover(t.Context(), "CTI-1"); err == nil {
		t.Fatal("host replacement token was accepted")
	}
	if store.intent.State != StateSpawnRequested || len(store.intent.TakeoverTokenHash) != 32 {
		t.Fatalf("intent mutated after replacement token: %#v", store.intent)
	}
}

func TestServiceExpiresBeforeRecoverySideEffects(t *testing.T) {
	store := &memoryStore{intent: Intent{ID: "CTI-1", TaskID: "TSK-1", State: StateHandedOff, Revision: 5,
		CheckpointID: "CHK-1", HostOperationID: "HOST-1", TakeoverTokenHash: make([]byte, 32), ExpiresAt: time.Now().Add(-time.Minute)}}
	host := &fakeHost{caps: HostCapabilities{ContextTelemetry: true, IdempotentPrepare: true, IdempotentActivate: true, SuccessorActivation: true, LocalAutoSpawnApproved: true}}
	service, _ := NewService(store, DefaultPolicy(), host, fakeCheckpointer{id: "CHK-1"})
	result, err := service.Recover(t.Context(), "CTI-1")
	if err != nil || result.Intent.State != StateExpired || host.activations != 0 {
		t.Fatalf("result=%#v activations=%d err=%v", result, host.activations, err)
	}
}

type memoryStore struct{ intent Intent }

func (s *memoryStore) Create(context.Context, CreateIntent) (Intent, error) { return Intent{}, nil }
func (s *memoryStore) Get(context.Context, string) (Intent, error)          { return s.intent, nil }
func (s *memoryStore) Save(_ context.Context, next Intent, expected int64) (Intent, error) {
	if s.intent.Revision != expected {
		return Intent{}, ErrConflict
	}
	next.Revision = expected + 1
	s.intent = next
	return next, nil
}
func (s *memoryStore) FindActive(context.Context, string, string) (Intent, error) {
	return s.intent, nil
}
func (s *memoryStore) ClaimTakeover(_ context.Context, _ string, tokenHash, runHash []byte) (Intent, error) {
	if s.intent.State != StateHandedOff || !bytes.Equal(s.intent.TakeoverTokenHash, tokenHash) {
		return Intent{}, ErrInvalidToken
	}
	s.intent.State = StateClaimed
	s.intent.SuccessorRunHash = append([]byte(nil), runHash...)
	s.intent.Revision++
	return s.intent, nil
}
func (s *memoryStore) ResumeTakeover(_ context.Context, _ string, runHash []byte, sessionID string) (Intent, error) {
	if s.intent.State != StateClaimed || !bytes.Equal(s.intent.SuccessorRunHash, runHash) {
		return Intent{}, ErrReservationLost
	}
	s.intent.State, s.intent.SuccessorSessionID = StateResumed, sessionID
	s.intent.Revision++
	return s.intent, nil
}
func (s *memoryStore) CompleteTakeover(_ context.Context, _ string, runHash []byte, sessionID string) (Intent, error) {
	if s.intent.State != StateResumed || !bytes.Equal(s.intent.SuccessorRunHash, runHash) || s.intent.SuccessorSessionID != sessionID {
		return Intent{}, ErrReservationLost
	}
	s.intent.State = StateCompleted
	s.intent.Revision++
	return s.intent, nil
}
func (s *memoryStore) ListRecoverable(context.Context, string) ([]Intent, error) {
	return []Intent{s.intent}, nil
}
func (s *memoryStore) MarkActivated(_ context.Context, _ string) (Intent, error) {
	now := time.Now().UTC()
	s.intent.ActivatedAt = &now
	s.intent.Revision++
	return s.intent, nil
}

type fakeCheckpointer struct {
	id       string
	handoffs *int
}

func (f fakeCheckpointer) CheckpointContinuity(context.Context, Intent) (string, error) {
	return f.id, nil
}
func (f fakeCheckpointer) HandoffContinuity(context.Context, Intent) error {
	if f.handoffs != nil {
		*f.handoffs++
	}
	return nil
}

type fakeHost struct {
	caps                      HostCapabilities
	prepared                  BootstrapEnvelope
	preparations, activations int
	prepareErrors             int
	activateErrors            int
	replacementToken          string
	retainedToken             string
}

var errAmbiguousHost = errors.New("ambiguous host result")

func (f *fakeHost) Capabilities(context.Context) HostCapabilities { return f.caps }
func (f *fakeHost) PrepareSuccessor(_ context.Context, envelope BootstrapEnvelope) (PreparedSuccessor, error) {
	f.prepared = envelope
	if envelope.TakeoverToken == "" && f.retainedToken == "" {
		return PreparedSuccessor{}, ErrHostPreparationMissing
	}
	if envelope.TakeoverToken != "" && f.retainedToken == "" {
		f.retainedToken = envelope.TakeoverToken
	}
	f.preparations++
	if f.prepareErrors > 0 {
		f.prepareErrors--
		return PreparedSuccessor{}, errAmbiguousHost
	}
	if f.replacementToken != "" {
		return PreparedSuccessor{OperationID: "HOST-1", TakeoverToken: f.replacementToken}, nil
	}
	return PreparedSuccessor{OperationID: "HOST-1", TakeoverToken: f.retainedToken}, nil
}
func (f *fakeHost) ActivateSuccessor(context.Context, string) error {
	f.activations++
	if f.activateErrors > 0 {
		f.activateErrors--
		return errAmbiguousHost
	}
	return nil
}
