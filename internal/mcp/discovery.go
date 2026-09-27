package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/git"
)

func (s *Server) registerDiscovery() {
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolReconcile, Description: "Discover undeclared edits into a task's change."}, s.reconcile)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolCheckpoint, Description: "Write a checkpoint note, or read the last handover."}, s.checkpoint)
}

// ReconcileIn discovers. OperationID absent runs without idempotency;
// present replays the first delivery.
type ReconcileIn struct {
	TaskID      string  `json:"task_id,omitempty"`
	OperationID *string `json:"operation_id,omitempty"`
}

func (s *Server) reconcile(ctx context.Context, req *sdk.CallToolRequest, in ReconcileIn) (*sdk.CallToolResult, AfterChangeOut, error) {
	if in.TaskID == "" {
		if in.OperationID != nil {
			return nil, AfterChangeOut{}, Invalid("automatic reconcile does not accept operation_id")
		}
		run, _, err := s.automaticRun(ctx, req, "")
		if err != nil {
			return nil, AfterChangeOut{}, Invalid("reconcile needs a task or automatic bootstrap on this connection")
		}
		projectID, err := s.projectID(ctx, run.TaskID)
		if err != nil {
			return nil, AfterChangeOut{}, err
		}
		runner := git.NewExecRunner()
		baseline, err := s.store.EnsureGuardBaseline(ctx, projectID, s.root, runner)
		if err != nil {
			return nil, AfterChangeOut{}, err
		}
		result, err := s.workflow.Reconcile(ctx, run.RunKey)
		if err != nil {
			return nil, AfterChangeOut{}, err
		}
		if err := baseline.CheckHead(ctx, s.root, runner); err != nil {
			return nil, AfterChangeOut{}, err
		}
		return s.describeChange(ctx, result.Change.ID, run.TaskID)
	}
	projectID, err := s.projectID(ctx, in.TaskID)
	if err != nil {
		return nil, AfterChangeOut{}, err
	}
	var operationID string
	if in.OperationID != nil {
		operationID = *in.OperationID
	}
	runner := git.NewExecRunner()
	baseline, err := s.store.EnsureGuardBaseline(ctx, projectID, s.root, runner)
	if err != nil {
		return nil, AfterChangeOut{}, err
	}
	result, err := s.changes.Reconcile(ctx, projectID, s.root, in.TaskID, operationID, runner)
	if err != nil {
		return nil, AfterChangeOut{}, err
	}
	if err := baseline.CheckHead(ctx, s.root, runner); err != nil {
		return nil, AfterChangeOut{}, err
	}
	return s.describeChange(ctx, result.Change.ID, in.TaskID)
}

// CheckpointIn writes when Note is present and reads the handover when it
// is absent (decision D-200): one tool, two directions, presence decides.
type CheckpointIn struct {
	TaskID      string  `json:"task_id,omitempty"`
	Session     string  `json:"session,omitempty"`
	Note        *string `json:"note,omitempty"`
	Handoff     bool    `json:"handoff,omitempty"`
	OperationID *string `json:"operation_id,omitempty"`
}

// CheckpointOut is the written note or the readable handover. ReadableBy
// names the sessions that can read it back — the handover contract
// (AC-02.2) in one field.
type CheckpointOut struct {
	TaskID     string `json:"task_id"`
	SessionID  string `json:"session_id,omitempty"`
	Note       string `json:"note,omitempty"`
	Handoff    bool   `json:"handoff,omitempty"`
	ReadableBy string `json:"readable_by,omitempty"`
	Replayed   bool   `json:"replayed"`
}

func (s *Server) checkpoint(ctx context.Context, req *sdk.CallToolRequest, in CheckpointIn) (*sdk.CallToolResult, CheckpointOut, error) {
	if in.TaskID == "" && in.Session == "" {
		if in.OperationID != nil {
			return nil, CheckpointOut{}, Invalid("automatic checkpoint does not accept operation_id")
		}
		if in.Handoff && in.Note == nil {
			return nil, CheckpointOut{}, Invalid("automatic handoff checkpoint needs a note")
		}
		run, _, err := s.automaticRun(ctx, req, "")
		if err != nil {
			return nil, CheckpointOut{}, err
		}
		if in.Note == nil {
			handed, err := s.app.Coordination().Handover(ctx, run.TaskID)
			if err != nil {
				return nil, CheckpointOut{}, err
			}
			if handed.Checkpoint == nil {
				return nil, CheckpointOut{}, Invalid("no checkpoint recorded for " + run.TaskID)
			}
			return nil, CheckpointOut{
				TaskID: handed.Checkpoint.TaskID, SessionID: handed.Checkpoint.SessionID,
				Note: handed.Checkpoint.Note, Handoff: handed.Checkpoint.Handoff,
				ReadableBy: "any session holding " + run.TaskID,
			}, nil
		}
		noted, err := s.workflow.Checkpoint(ctx, run.RunKey, *in.Note, in.Handoff)
		if err != nil {
			return nil, CheckpointOut{}, err
		}
		out := CheckpointOut{
			TaskID: noted.Checkpoint.TaskID, SessionID: noted.Checkpoint.SessionID,
			Note: noted.Checkpoint.Note, Handoff: noted.Checkpoint.Handoff,
			ReadableBy: "any session holding " + run.TaskID,
		}
		if in.Handoff {
			s.forgetAutomaticIfCurrent(req.Session, run)
		}
		return nil, out, nil
	}
	if in.TaskID == "" || in.Session == "" {
		return nil, CheckpointOut{}, Invalid("checkpoint needs a task and a session")
	}
	coord := s.app.Coordination()
	if in.OperationID != nil {
		coord = coord.Idempotent(*in.OperationID)
	}
	if in.Note == nil {
		handed, err := coord.Handover(ctx, in.TaskID)
		if err != nil {
			return nil, CheckpointOut{}, err
		}
		if handed.Checkpoint == nil {
			return nil, CheckpointOut{}, Invalid("no checkpoint recorded for " + in.TaskID)
		}
		return nil, CheckpointOut{
			TaskID:     handed.Checkpoint.TaskID,
			SessionID:  handed.Checkpoint.SessionID,
			Note:       handed.Checkpoint.Note,
			Handoff:    handed.Checkpoint.Handoff,
			ReadableBy: "any session holding " + in.TaskID,
		}, nil
	}
	space := s.app.Subject().Workspace
	if space.ID == "" {
		return nil, CheckpointOut{}, Invalid("checkpoint needs a registered worktree")
	}
	noted, write, err := coord.WriteCheckpoint(ctx, in.TaskID,
		coordination.NamedSession(in.Session), space.ID, *in.Note, in.Handoff)
	if err != nil {
		return nil, CheckpointOut{}, err
	}
	return nil, CheckpointOut{
		TaskID:     noted.Checkpoint.TaskID,
		SessionID:  noted.Checkpoint.SessionID,
		Note:       noted.Checkpoint.Note,
		Handoff:    noted.Checkpoint.Handoff,
		ReadableBy: "any session holding " + in.TaskID,
		Replayed:   write.Replayed,
	}, nil
}
