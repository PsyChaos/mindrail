package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/continuity"
	"github.com/PsyChaos/mindrail/internal/workflow"
)

const runtimeTelemetryMetaKey = "io.mindrail/runtime-telemetry"
const runtimeTelemetryMaxBytes = 1024
const runtimeTelemetryFreshness = 30 * time.Second
const runtimeTelemetryFutureSkew = 5 * time.Second

type observationRecorder interface {
	Observe(context.Context, string, agent.RuntimeObservationInput) (agent.RuntimeObservation, error)
}

type runtimeTelemetryMeta struct {
	Model      string                `json:"model,omitempty"`
	Effort     string                `json:"effort,omitempty"`
	Context    *runtimeContextCounts `json:"context,omitempty"`
	Sequence   int64                 `json:"sequence,omitempty"`
	ObservedAt string                `json:"observed_at,omitempty"`
}

type runtimeContextCounts struct {
	Used  int64 `json:"used"`
	Limit int64 `json:"limit"`
}

func parseRuntimeTelemetry(request sdk.Request) (agent.RuntimeObservationInput, bool) {
	params, ok := request.GetParams().(*sdk.CallToolParamsRaw)
	if !ok || params == nil || params.Meta == nil {
		return agent.RuntimeObservationInput{}, false
	}
	raw, ok := params.Meta[runtimeTelemetryMetaKey]
	if !ok {
		return agent.RuntimeObservationInput{}, false
	}
	return decodeRuntimeTelemetry(raw)
}

func decodeRuntimeTelemetry(raw any) (agent.RuntimeObservationInput, bool) {
	encoded, err := json.Marshal(raw)
	if err != nil || len(encoded) == 0 || len(encoded) > runtimeTelemetryMaxBytes {
		return agent.RuntimeObservationInput{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var meta runtimeTelemetryMeta
	if err := decoder.Decode(&meta); err != nil {
		return agent.RuntimeObservationInput{}, false
	}
	input := agent.RuntimeObservationInput{
		ModelKey: meta.Model, Effort: meta.Effort,
		Source: agent.ObservationSourceMCPMeta, Confidence: agent.ObservationSelfReported,
		ProducerSequence: meta.Sequence,
	}
	if meta.ObservedAt != "" {
		observedAt, err := time.Parse(time.RFC3339Nano, meta.ObservedAt)
		if err != nil {
			return agent.RuntimeObservationInput{}, false
		}
		input.ObservedAt = observedAt
	}
	if meta.Context != nil {
		if meta.Sequence < 1 || input.ObservedAt.IsZero() {
			return agent.RuntimeObservationInput{}, false
		}
		input.ContextUsed = &meta.Context.Used
		input.ContextLimit = &meta.Context.Limit
	}
	return input, true
}

func (s *Server) recordRuntimeTelemetry(ctx context.Context, session *sdk.ServerSession, request sdk.Request) {
	if session == nil || s.observations == nil {
		return
	}
	input, ok := parseRuntimeTelemetry(request)
	if !ok {
		return
	}
	s.autoMu.RLock()
	current := s.automatic[session]
	var runtimeID string
	if current != nil && current.presence != nil {
		runtimeID = current.presence.id
	}
	s.autoMu.RUnlock()
	if runtimeID == "" {
		return
	}
	s.recordObservation(ctx, session, runtimeID, input)
}

func (s *Server) recordObservation(ctx context.Context, session *sdk.ServerSession, runtimeID string, input agent.RuntimeObservationInput) {
	observed, err := s.observations.Observe(ctx, runtimeID, input)
	if err != nil || !observed.Accepted || observed.ProducerSequence <= 0 || observed.ContextUsed == nil || observed.ContextLimit == nil || s.continuityService == nil {
		return
	}
	if !runtimeObservationFresh(s.clock.Now(), observed.ObservedAt) {
		return
	}
	intentID, err := s.ensureContinuityIntent(ctx, session)
	if err != nil {
		return
	}
	_, _ = s.continuityService.Observe(ctx, intentID, continuity.Observation{
		RuntimeID: runtimeID, Sequence: max(observed.ProducerSequence, observed.Revision), Model: observed.ModelKey, Effort: observed.Effort,
		ContextUsed: *observed.ContextUsed, ContextLimit: *observed.ContextLimit, ObservedAt: observed.ObservedAt,
		Source: observed.Source, Confidence: continuity.ConfidenceReported,
	})
}

func runtimeObservationFresh(now, observed time.Time) bool {
	if observed.IsZero() {
		return false
	}
	age := now.UTC().Sub(observed.UTC())
	return age >= -runtimeTelemetryFutureSkew && age <= runtimeTelemetryFreshness
}

func (s *Server) ensureContinuityIntent(ctx context.Context, session *sdk.ServerSession) (string, error) {
	s.continuityMu.Lock()
	defer s.continuityMu.Unlock()
	s.autoMu.RLock()
	current := s.automatic[session]
	if current == nil {
		s.autoMu.RUnlock()
		return "", errors.New("automatic run unavailable")
	}
	run, intentID := current.run, current.continuityIntentID
	s.autoMu.RUnlock()
	if intentID != "" {
		return intentID, nil
	}
	if active, findErr := s.continuityStore.FindActive(ctx, run.TaskID, run.SessionID); findErr == nil {
		s.autoMu.Lock()
		if latest := s.automatic[session]; latest != nil && latest.run.RunKey == run.RunKey {
			latest.continuityIntentID = active.ID
		}
		s.autoMu.Unlock()
		return active.ID, nil
	} else if !errors.Is(findErr, continuity.ErrIntentNotFound) {
		return "", findErr
	}
	digest := sha256.Sum256([]byte(run.RunKey))
	created, err := s.continuityStore.Create(ctx, continuity.CreateIntent{
		ProjectID: s.projectIDValue, WorkspaceID: s.workspaceIDValue, TaskID: run.TaskID,
		PredecessorSessionID: run.SessionID, PredecessorRunHash: digest[:], Kind: continuity.IntentSameTask,
		ExpiresAt: s.clock.Now().Add(24 * time.Hour),
	})
	if err != nil {
		return "", err
	}
	s.autoMu.Lock()
	if latest := s.automatic[session]; latest != nil && latest.run.RunKey == run.RunKey {
		latest.continuityIntentID = created.ID
	}
	s.autoMu.Unlock()
	return created.ID, nil
}

func (s *Server) CheckpointContinuity(ctx context.Context, intent continuity.Intent) (string, error) {
	s.autoMu.RLock()
	var key string
	for _, current := range s.automatic {
		if current.run.TaskID == intent.TaskID && current.run.SessionID == intent.PredecessorSessionID {
			key = current.run.RunKey
			break
		}
	}
	s.autoMu.RUnlock()
	if key == "" {
		noted, err := s.workflow.CheckpointByRunHash(ctx, intent.PredecessorRunHash, "Automatic continuity checkpoint before context handoff.", false)
		if err != nil {
			return "", err
		}
		return noted.Checkpoint.ID, nil
	}
	noted, err := s.workflow.Checkpoint(ctx, key, "Automatic continuity checkpoint before context handoff.", false)
	if err != nil {
		return "", err
	}
	return noted.Checkpoint.ID, nil
}

func (s *Server) HandoffContinuity(ctx context.Context, intent continuity.Intent) error {
	s.autoMu.RLock()
	var session *sdk.ServerSession
	var run workflow.Run
	for candidate, current := range s.automatic {
		if current.run.TaskID == intent.TaskID && current.run.SessionID == intent.PredecessorSessionID {
			session, run = candidate, current.run
			break
		}
	}
	s.autoMu.RUnlock()
	if session == nil {
		_, err := s.workflow.CheckpointByRunHash(ctx, intent.PredecessorRunHash, "Automatic continuity handoff to prepared successor.", true)
		return err
	}
	if _, err := s.workflow.Checkpoint(ctx, run.RunKey, "Automatic continuity handoff to prepared successor.", true); err != nil {
		return err
	}
	s.forgetAutomaticIfCurrent(session, run)
	return nil
}

type mcpManualHost struct{}

func (mcpManualHost) Capabilities(context.Context) continuity.HostCapabilities {
	return continuity.HostCapabilities{ContextTelemetry: true}
}
func (mcpManualHost) PrepareSuccessor(context.Context, continuity.BootstrapEnvelope) (continuity.PreparedSuccessor, error) {
	return continuity.PreparedSuccessor{}, continuity.ErrHostCapabilityUnavailable
}
func (mcpManualHost) ActivateSuccessor(context.Context, string) error {
	return continuity.ErrHostCapabilityUnavailable
}
