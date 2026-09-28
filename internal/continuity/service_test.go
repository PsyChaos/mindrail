package continuity

import (
	"context"
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
	host := &fakeHost{caps: HostCapabilities{ContextTelemetry: true, IdempotentPrepare: true, SuccessorActivation: true, LocalAutoSpawnApproved: true}}
	service, _ := NewService(store, DefaultPolicy(), host, fakeCheckpointer{id: "CHK-1"})
	result, err := service.Observe(t.Context(), "CTI-1", observation(2, 600_000, time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Intent.State != StateSpawnReady || result.Intent.HostOperationID != "HOST-1" || result.Envelope == nil || result.Envelope.TakeoverToken == "" {
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

type fakeCheckpointer struct{ id string }

func (f fakeCheckpointer) CheckpointContinuity(context.Context, Intent) (string, error) {
	return f.id, nil
}

type fakeHost struct {
	caps     HostCapabilities
	prepared BootstrapEnvelope
}

func (f *fakeHost) Capabilities(context.Context) HostCapabilities { return f.caps }
func (f *fakeHost) PrepareSuccessor(_ context.Context, envelope BootstrapEnvelope) (PreparedSuccessor, error) {
	f.prepared = envelope
	return PreparedSuccessor{OperationID: "HOST-1"}, nil
}
func (f *fakeHost) ActivateSuccessor(context.Context, string) error { return nil }
