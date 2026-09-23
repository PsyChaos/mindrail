package validation

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/PsyChaos/mindrail/internal/config"
)

// Service composes the runner, redaction, snapshot and store into the
// single profile → run → redact → snapshot → store flow (AC-02.5). Later
// milestones drive it; tests drive it now.
type Service struct {
	runner *Runner
	store  *Store
}

// NewService builds a Service over the runner and store it coordinates.
func NewService(runner *Runner, store *Store) (*Service, error) {
	if runner == nil || store == nil {
		return nil, invalidInput("validation service needs a runner and a store")
	}
	return &Service{runner: runner, store: store}, nil
}

// RunProfile runs every command of one named profile against the root and
// records one evidence row per command. Snapshot precedes execution: a
// scope that cannot hash refuses before anything spawns. Redaction applies
// at record time from the live environment; the runner's raw output never
// persists beyond this call. A non-empty operation id scopes per command
// (`id#index`), so multi-command profiles replay without conflict.
func (s *Service) RunProfile(ctx context.Context, name string, profile config.ValidationProfile, root string, secretEnv []string, operationID string) ([]Evidence, error) {
	if name == "" || profile.Type == "" || len(profile.Paths) == 0 || len(profile.Commands) == 0 {
		return nil, invalidInput("profile run needs a name, a type, a scope and commands")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot, err := SnapshotScope(root, profile.Paths)
	if err != nil {
		return nil, err
	}
	redactor, err := NewRedactor(secretEnv)
	if err != nil {
		return nil, err
	}
	var out []Evidence
	for i, argv := range profile.Commands {
		if len(argv) == 0 {
			return nil, invalidInput("profile run needs argv for every command")
		}
		result := s.runner.Run(ctx, argv)
		provenance, err := json.Marshal(map[string]any{
			"profile":       name,
			"command_index": i,
			"scope":         snapshot.Scope,
		})
		if err != nil {
			return nil, invalidInput("profile run provenance is not representable")
		}
		commandOp := operationID
		if commandOp != "" {
			commandOp = operationID + "#" + strconv.Itoa(i)
		}
		record, err := s.store.Record(ctx, name, profile.Type, argv, result,
			snapshot.Hash, string(provenance), commandOp, redactor)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, nil
}
