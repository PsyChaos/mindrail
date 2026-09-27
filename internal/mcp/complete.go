package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/completion"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/workflow"
)

// CompleteIn asks local completion. Required names evidence profiles;
// budget/escalation/approval are version-error parameters (decision
// D-208): present means refusal, absent means the default flow.
type CompleteIn struct {
	TaskID     string   `json:"task_id,omitempty"`
	Required   []string `json:"required,omitempty"`
	Finalize   *bool    `json:"finalize,omitempty"`
	RunKey     string   `json:"run_key,omitempty"`
	Budget     *string  `json:"budget,omitempty"`
	Escalation *string  `json:"escalation,omitempty"`
	Approval   *string  `json:"approval,omitempty"`
}

// CompleteDenial is one gate denial on the wire.
type CompleteDenial struct {
	Code       string   `json:"code"`
	Reason     string   `json:"reason"`
	Key        string   `json:"key"`
	Provenance string   `json:"provenance"`
	NextAction []string `json:"next_action"`
}

// CompleteOut is the gate Decision verbatim: composition proves parity by
// construction (decision D-206).
type CompleteOut struct {
	Allow                bool             `json:"allow"`
	Denials              []CompleteDenial `json:"denials"`
	Refusal              *Refusal         `json:"refusal,omitempty"`
	Automatic            bool             `json:"automatic,omitempty"`
	Completed            *bool            `json:"completed,omitempty"`
	TaskID               string           `json:"task_id,omitempty"`
	State                string           `json:"state,omitempty"`
	Revision             int64            `json:"revision,omitempty"`
	Profiles             []string         `json:"profiles,omitempty"`
	NoProfilesConfigured bool             `json:"no_profiles_configured,omitempty"`
}

func (s *Server) complete(ctx context.Context, req *sdk.CallToolRequest, in CompleteIn) (*sdk.CallToolResult, CompleteOut, error) {
	if refusal := versionedParam("budget", in.Budget); refusal != nil {
		return nil, CompleteOut{Denials: []CompleteDenial{}, Refusal: refusal}, nil
	}
	if refusal := versionedParam("escalation", in.Escalation); refusal != nil {
		return nil, CompleteOut{Denials: []CompleteDenial{}, Refusal: refusal}, nil
	}
	if refusal := versionedParam("approval", in.Approval); refusal != nil {
		return nil, CompleteOut{Denials: []CompleteDenial{}, Refusal: refusal}, nil
	}
	if in.Finalize != nil || in.RunKey != "" {
		if in.Finalize == nil || !*in.Finalize {
			return nil, CompleteOut{}, Invalid("automatic complete needs finalize true")
		}
		if in.TaskID != "" || len(in.Required) > 0 {
			return nil, CompleteOut{}, Invalid("automatic complete does not accept task_id or required")
		}
		run, heartbeat, err := s.automaticRun(ctx, req, in.RunKey)
		if err != nil {
			return nil, CompleteOut{}, err
		}
		if heartbeat != nil {
			if err := heartbeat.Health(); err != nil {
				return nil, CompleteOut{}, err
			}
		} else if !terminalRun(run) {
			return nil, CompleteOut{}, Invalid("automatic lease heartbeat is not active; retry bootstrap with the same run_key")
		}
		finished, err := s.workflow.Finalize(ctx, workflow.FinalizeInput{RunKey: run.RunKey})
		if err != nil {
			return nil, CompleteOut{}, err
		}
		out := completionOut(finished.Decision)
		out.Automatic = true
		out.Completed = &finished.Completed
		out.TaskID = finished.Run.TaskID
		out.State = string(finished.Run.State)
		out.Revision = finished.Run.Revision
		out.Profiles = finished.Profiles
		out.NoProfilesConfigured = finished.NoProfilesConfigured
		if finished.Completed {
			s.retainTerminalAutomatic(req.Session, finished.Run)
		} else {
			s.updateAutomatic(req.Session, finished.Run)
		}
		return nil, out, nil
	}
	if in.TaskID == "" {
		return nil, CompleteOut{}, Invalid("complete needs a task")
	}
	projectID, err := s.projectID(ctx, in.TaskID)
	if err != nil {
		return nil, CompleteOut{}, err
	}
	service, err := completion.New(s.root, s.changes, s.indexes, s.evidence, s.guard)
	if err != nil {
		return nil, CompleteOut{}, err
	}
	decision, err := service.Evaluate(ctx, projectID, in.TaskID, in.Required)
	if err != nil {
		return nil, CompleteOut{}, err
	}
	return nil, completionOut(decision), nil
}

func completionOut(decision gate.Decision) CompleteOut {
	out := CompleteOut{Allow: decision.Allow}
	for _, denial := range decision.Denials {
		out.Denials = append(out.Denials, CompleteDenial{
			Code:       string(denial.Code),
			Reason:     denial.Reason,
			Key:        denial.Key,
			Provenance: denial.Provenance,
			NextAction: denial.NextAction,
		})
	}
	if out.Denials == nil {
		out.Denials = []CompleteDenial{}
	}
	return out
}
