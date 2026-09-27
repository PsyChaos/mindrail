package validation

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/identity"
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
	runID := identity.NewID("VRN")
	var out []Evidence
	for i, argv := range profile.Commands {
		if len(argv) == 0 {
			return nil, invalidInput("profile run needs argv for every command")
		}
		result := s.runner.Run(ctx, argv)
		provenance, err := json.Marshal(evidenceProvenance{
			Profile: name, RunID: runID, CommandIndex: i, CommandCount: len(profile.Commands),
			ScopePaths: profile.Paths, Scope: snapshot.Scope,
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
		recordedProvenance, ok := readProvenance(record)
		if !ok || recordedProvenance.Profile != name || recordedProvenance.CommandCount != len(profile.Commands) ||
			recordedProvenance.CommandIndex != i || !slices.Equal(recordedProvenance.ScopePaths, profile.Paths) ||
			!slices.Equal(recordedProvenance.Scope, snapshot.Scope) {
			return nil, provenanceConflict(commandOp)
		}
		if i == 0 && commandOp != "" {
			// A durable first command owns the logical run identity. Reusing it
			// lets a retry fill rows that were never recorded after interruption.
			runID = recordedProvenance.RunID
		} else if recordedProvenance.RunID != runID {
			return nil, provenanceConflict(commandOp)
		}
		out = append(out, record)
	}
	return out, nil
}

func provenanceConflict(operationID string) error {
	if operationID != "" {
		return operationConflict(operationID)
	}
	return invalidInput("recorded evidence provenance does not match the profile run")
}
