package workflow

import (
	"context"
	"fmt"
	"sort"

	"github.com/PsyChaos/mindrail/internal/completion"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/gate"
)

// Finalize runs configured profiles, then the shared gate, then the guarded
// transitions. A denied gate leaves the task and its leases available for repair.
func (s *Service) Finalize(ctx context.Context, in FinalizeInput) (Finalization, error) {
	unlock := s.lockRun(in.RunKey)
	defer unlock()
	r, err := s.Resolve(ctx, in.RunKey)
	out := Finalization{Run: r, Profiles: []string{}}
	for name := range s.options.Profiles {
		out.Profiles = append(out.Profiles, name)
	}
	sort.Strings(out.Profiles)
	out.NoProfilesConfigured = len(out.Profiles) == 0
	if err != nil {
		return out, err
	}
	if r.State == coordination.StateCompleted {
		var move coordination.Move
		found, err := s.readPhase(ctx, in.RunKey, fmt.Sprintf("complete:%d", r.Revision-1), &move)
		if err != nil {
			return out, err
		}
		if !found || move.Task.ID != r.TaskID {
			return out, invalid("task was completed outside this automatic run")
		}
		// The terminal operation is the durable validation report. Current
		// configuration may have changed since this task actually completed.
		out.Profiles = append([]string{}, move.RequiredProfiles...)
		out.NoProfilesConfigured = len(out.Profiles) == 0
		if err := s.releaseFiles(ctx, r); err != nil {
			return out, err
		}
		out.Completed = true
		out.Decision = gate.Decision{Allow: true, Denials: []gate.Denial{}}
		return out, nil
	}
	if r.State.Terminal() {
		return out, invalid("automatic task is terminal")
	}
	if err := s.Renew(ctx, in.RunKey); err != nil {
		return out, err
	}
	view, err := s.scoped(ctx, r)
	if err != nil {
		return out, err
	}
	if _, err := s.reconcile(ctx, r, view); err != nil {
		return out, err
	}
	// Validation executes on each unfinished attempt: failed or stale evidence
	// must be replaced after a repair, not replayed merely because run_key agrees.
	for _, name := range out.Profiles {
		if _, err := s.options.Validation.RunProfile(ctx, name, s.options.Profiles[name], s.options.Root, s.options.SecretEnv, ""); err != nil {
			return out, err
		}
	}
	if err := s.Renew(ctx, in.RunKey); err != nil {
		return out, err
	}
	view, err = s.scoped(ctx, r)
	if err != nil {
		return out, err
	}
	evaluator, err := completion.New(s.options.Root, view, s.options.Indexes, s.options.Evidence, s.options.Guard)
	if err != nil {
		return out, err
	}
	out.Decision, err = evaluator.Evaluate(ctx, s.options.ProjectID, r.TaskID, out.Profiles)
	if err != nil {
		return out, err
	}
	if !out.Decision.Allow {
		return out, nil
	}
	if err := s.Renew(ctx, in.RunKey); err != nil {
		return out, err
	}
	by := coordination.NamedSession(r.SessionID)
	if r.State != coordination.StateReadyToComplete {
		if r.State != coordination.StateInProgress {
			return out, invalid("automatic task must be in progress before finalization")
		}
		move, _, err := s.options.Coordination.Idempotent(s.operation(in.RunKey, fmt.Sprintf("ready:%d", r.Revision))).TransitionExpecting(ctx, r.TaskID, by, coordination.StateReadyToComplete, "automatic validation passed", r.Revision)
		if err != nil {
			return out, err
		}
		r.State, r.Revision = move.Task.State, move.Task.Revision
	}
	move, _, err := s.options.Coordination.Idempotent(s.operation(in.RunKey, fmt.Sprintf("complete:%d", r.Revision))).CompleteExpecting(ctx, r.TaskID, by, r.Revision, out.Profiles)
	if err != nil {
		return out, err
	}
	r.State, r.Revision = move.Task.State, move.Task.Revision
	out.Run = r
	if err := s.releaseFiles(ctx, r); err != nil {
		return out, err
	}
	out.Completed = true
	return out, nil
}

func (s *Service) releaseFiles(ctx context.Context, r Run) error {
	leases, err := s.options.Coordination.ListLeases(ctx, s.options.ProjectID)
	if err != nil {
		return err
	}
	for _, l := range leases {
		if l.Holder == r.SessionID && l.TargetKind == coordination.TargetFile {
			if _, _, err := s.options.Coordination.ReleaseLease(ctx, l.ID, coordination.NamedSession(r.SessionID)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Checkpoint uses the content and current revision as its retry identity.
// Handoff stops this logical run; a new run_key and explicit task resume are
// required before another agent can work or finalize it.
func (s *Service) Checkpoint(ctx context.Context, key, note string, handoff bool) (coordination.Noted, error) {
	unlock := s.lockRun(key)
	defer unlock()
	r, err := s.Resolve(ctx, key)
	if err != nil {
		return coordination.Noted{}, err
	}
	phase := "checkpoint:" + digest([]byte(fmt.Sprintf("%d:%t:%s", r.Revision, handoff, note)))[:24]
	var prior coordination.Noted
	if found, err := s.readPhase(ctx, key, phase, &prior); err != nil {
		return coordination.Noted{}, err
	} else if found {
		if handoff {
			if err := s.releaseFiles(ctx, r); err != nil {
				return prior, err
			}
			if err := s.recordFailure(ctx, key, invalid("run handed off")); err != nil {
				return prior, err
			}
		}
		return prior, nil
	}
	if err := s.Renew(ctx, key); err != nil {
		return coordination.Noted{}, err
	}
	noted, _, err := s.options.Coordination.Idempotent(s.operation(key, phase)).WriteCheckpoint(ctx, r.TaskID, coordination.NamedSession(r.SessionID), s.options.WorkspaceID, note, handoff)
	if err != nil {
		return noted, err
	}
	if handoff {
		if err := s.releaseFiles(ctx, r); err != nil {
			return noted, err
		}
		if err := s.recordFailure(ctx, key, invalid("run handed off")); err != nil {
			return noted, err
		}
	}
	return noted, nil
}
