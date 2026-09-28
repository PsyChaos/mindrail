package agent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

const (
	ObservationSourceMCPMeta    = "mcp_meta"
	ObservationSourceHost       = "host_adapter"
	ObservationSelfReported     = "self_reported"
	ObservationStructuredHost   = "structured_host"
	ObservationDerived          = "derived"
	ObservationModelUnknown     = "unknown"
	ObservationEffortNotApplied = "not_applicable"
)

type RuntimeObservationInput struct {
	ModelKey, Effort, Source, Confidence string
	ContextUsed, ContextLimit            *int64
}

type RuntimeObservation struct {
	RuntimeID, ModelKey, Effort, Source, Confidence string
	ContextUsed, ContextLimit                       *int64
	ObservedAt                                      time.Time
	Revision                                        int64
}

type RuntimeObservationStore struct {
	db    *sql.DB
	clock app.Clock
}

func NewRuntimeObservationStore(db *sql.DB, clock app.Clock) (*RuntimeObservationStore, error) {
	if db == nil || clock == nil {
		return nil, errors.New("runtime observation store needs a database handle and clock")
	}
	return &RuntimeObservationStore{db: db, clock: clock}, nil
}

func (s *RuntimeObservationStore) Observe(ctx context.Context, runtimeID string, in RuntimeObservationInput) (RuntimeObservation, error) {
	if err := requireAgentTelemetrySchema(ctx, s.db); err != nil {
		return RuntimeObservation{}, err
	}
	if runtimeID == "" {
		return RuntimeObservation{}, errors.New("runtime observation needs a runtime id")
	}
	in.ModelKey = canonicalModelKey(in.ModelKey)
	if err := validateObservation(in); err != nil {
		return RuntimeObservation{}, err
	}
	now := s.clock.Now().UTC()
	result, err := s.db.ExecContext(ctx, `INSERT INTO agent_runtime_observations
		(runtime_id, model_key, effort, context_used, context_limit, source, confidence, observed_at, revision)
	SELECT runtime_id, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, 1
	FROM agent_runtimes WHERE runtime_id = ? AND ended_at IS NULL
	ON CONFLICT(runtime_id) DO UPDATE SET
		model_key = excluded.model_key, effort = excluded.effort,
		context_used = excluded.context_used, context_limit = excluded.context_limit,
		source = excluded.source, confidence = excluded.confidence,
		observed_at = excluded.observed_at, revision = agent_runtime_observations.revision + 1
	WHERE EXISTS (SELECT 1 FROM agent_runtimes r WHERE r.runtime_id = excluded.runtime_id AND r.ended_at IS NULL)`,
		in.ModelKey, in.Effort, nullableInt64Pointer(in.ContextUsed), nullableInt64Pointer(in.ContextLimit),
		in.Source, in.Confidence, app.FormatTime(now), runtimeID)
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("observe agent runtime: %w", err)
	}
	written, err := result.RowsAffected()
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("observe agent runtime: %w", err)
	}
	if written != 1 {
		return RuntimeObservation{}, errors.New("runtime observation requires a live attributed runtime")
	}
	return s.Get(ctx, runtimeID)
}

func (s *RuntimeObservationStore) Get(ctx context.Context, runtimeID string) (RuntimeObservation, error) {
	if err := requireAgentTelemetrySchema(ctx, s.db); err != nil {
		return RuntimeObservation{}, err
	}
	if runtimeID == "" {
		return RuntimeObservation{}, errors.New("runtime observation needs a runtime id")
	}
	var out RuntimeObservation
	var model, effort sql.NullString
	var used, limit sql.NullInt64
	var observed string
	err := s.db.QueryRowContext(ctx, `SELECT runtime_id, model_key, effort, context_used, context_limit,
		source, confidence, observed_at, revision FROM agent_runtime_observations WHERE runtime_id = ?`, runtimeID).
		Scan(&out.RuntimeID, &model, &effort, &used, &limit, &out.Source, &out.Confidence, &observed, &out.Revision)
	if err != nil {
		return RuntimeObservation{}, err
	}
	out.ModelKey, out.Effort = canonicalModelKey(model.String), canonicalEffort(effort.String)
	if used.Valid {
		value := used.Int64
		out.ContextUsed = &value
	}
	if limit.Valid {
		value := limit.Int64
		out.ContextLimit = &value
	}
	if out.ObservedAt, err = app.ParseTime(observed); err != nil {
		return RuntimeObservation{}, fmt.Errorf("read runtime observation time: %w", err)
	}
	return out, nil
}

func canonicalModelKey(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	aliases := map[string]string{
		"claude-opus": "claude-opus", "claude-sonnet": "claude-sonnet", "claude-haiku": "claude-haiku",
		"gpt-5": "gpt-5", "gpt-5-codex": "gpt-5-codex", "gpt-5.6-sol": "gpt-5.6-sol",
		"gpt-5.6-terra": "gpt-5.6-terra", "gpt-5.6-luna": "gpt-5.6-luna", "gpt-5.5": "gpt-5.5",
		"gpt-6-astra": "gpt-6-astra", "unknown": "unknown",
	}
	return aliases[key]
}

func canonicalEffort(raw string) string {
	switch raw {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max", "ultra", "not_applicable":
		return raw
	default:
		return ""
	}
}

func validateObservation(in RuntimeObservationInput) error {
	if canonicalEffort(in.Effort) != in.Effort && in.Effort != "" {
		return errors.New("runtime observation effort is invalid")
	}
	if (in.ContextUsed == nil) != (in.ContextLimit == nil) {
		return errors.New("runtime observation context counters must be an atomic pair")
	}
	if in.ContextUsed != nil && (*in.ContextUsed < 0 || *in.ContextLimit < 1 || *in.ContextUsed > *in.ContextLimit || *in.ContextLimit > 100_000_000) {
		return errors.New("runtime observation context counters are invalid")
	}
	if in.Source != ObservationSourceMCPMeta && in.Source != ObservationSourceHost {
		return errors.New("runtime observation source is invalid")
	}
	if in.Confidence != ObservationSelfReported && in.Confidence != ObservationStructuredHost && in.Confidence != ObservationDerived {
		return errors.New("runtime observation confidence is invalid")
	}
	return nil
}

func nullableInt64Pointer(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
