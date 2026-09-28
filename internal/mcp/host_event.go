package mcp

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/agent"
)

type hostRuntimeRecorder interface {
	Observe(context.Context, agent.HostRuntimeAttribution, agent.HostRuntimeEventInput) (agent.HostRuntime, error)
	ObserveLifecycle(context.Context, agent.HostRuntimeAttribution, agent.HostLifecycleInput) (agent.HostRuntime, error)
	BindPending(context.Context, agent.HostRuntimeAttribution, agent.HostRuntimeIdentity) (agent.HostRuntime, error)
	ObservePendingLifecycle(context.Context, agent.HostLifecycleInput) error
	AssociatePendingRun(context.Context, string, string, string, string) error
	ClaimPendingRun(context.Context, agent.HostRuntimeAttribution, string, string) (agent.HostRuntime, error)
}

type HostEventIn struct {
	Host          string `json:"host"`
	HostSessionID string `json:"host_session_id"`
	AgentID       string `json:"agent_id,omitempty"`
	Generation    string `json:"generation,omitempty"`
	AgentType     string `json:"agent_type,omitempty"`
	Event         string `json:"event,omitempty"`
	Sequence      int64  `json:"sequence,omitempty"`
	ObservedAt    string `json:"observed_at,omitempty"`
	Source        string `json:"source,omitempty"`
	Model         string `json:"model,omitempty"`
	Effort        string `json:"effort,omitempty"`
	ContextUsed   *int64 `json:"context_used,omitempty"`
	ContextLimit  *int64 `json:"context_limit,omitempty"`
	ClaimPending  bool   `json:"claim_pending,omitempty"`
	RunKey        string `json:"run_key,omitempty"`
}

type HostEventOut struct {
	RuntimeID       string `json:"runtime_id"`
	Host            string `json:"host"`
	Main            bool   `json:"main"`
	ParentRuntimeID string `json:"parent_runtime_id,omitempty"`
	AgentType       string `json:"agent_type,omitempty"`
	State           string `json:"state"`
	Event           string `json:"event"`
	Sequence        int64  `json:"sequence"`
	Accepted        bool   `json:"accepted"`
}

func (s *Server) registerHostEvent() {
	sdk.AddTool(s.impl, &sdk.Tool{
		Name:        ToolHostEvent,
		Description: "Record self-reported Claude Code or Codex lifecycle/telemetry for this exact MCP run. This is unverified dashboard data; host identifiers are hashed before persistence and it cannot authorize continuity.",
	}, s.hostEvent)
}

func (s *Server) hostEvent(ctx context.Context, req *sdk.CallToolRequest, in HostEventIn) (*sdk.CallToolResult, HostEventOut, error) {
	if s.hostRuntimes == nil {
		return nil, HostEventOut{}, Invalid("host runtime adapter is unavailable")
	}
	var recorded agent.HostRuntime
	var err error
	if in.ClaimPending {
		if in.RunKey == "" || req == nil || req.Session == nil {
			return nil, HostEventOut{}, Invalid("claim_pending needs run_key and an MCP connection")
		}
		pending := agent.HostLifecycleInput{Host: in.Host, HostSessionID: in.HostSessionID, AgentID: in.AgentID,
			AgentType: in.AgentType, Event: in.Event, Source: in.Source, ModelKey: in.Model, Effort: in.Effort,
			ContextUsed: in.ContextUsed, ContextLimit: in.ContextLimit}
		if pending.Event == "" {
			pending.Event = agent.HostEventActivity
		}
		if err := s.hostRuntimes.ObservePendingLifecycle(ctx, pending); err != nil {
			if !errors.Is(err, agent.ErrPendingHostGeneration) || (pending.Event != agent.HostEventActivity && pending.Event != agent.HostEventTelemetry) {
				return nil, HostEventOut{}, Invalid(err.Error())
			}
			pending.Event = agent.HostEventStart
			if err = s.hostRuntimes.ObservePendingLifecycle(ctx, pending); err != nil {
				return nil, HostEventOut{}, Invalid(err.Error())
			}
		}
		if err := s.hostRuntimes.AssociatePendingRun(ctx, in.Host, in.HostSessionID, in.AgentID, in.RunKey); err != nil {
			return nil, HostEventOut{}, Invalid(err.Error())
		}
		if run, _, runErr := s.automaticRun(ctx, req, in.RunKey); runErr == nil {
			attr := agent.HostRuntimeAttribution{ProjectID: s.projectIDValue, WorkspaceID: s.workspaceIDValue, TaskID: run.TaskID, SessionID: run.SessionID}
			recorded, err = s.hostRuntimes.ClaimPendingRun(ctx, attr, in.RunKey, s.automaticPresenceID(req.Session))
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, HostEventOut{}, Invalid(err.Error())
			}
		}
		if recorded.RuntimeID == "" {
			return nil, HostEventOut{Host: in.Host, Main: in.AgentID == "", State: "PENDING", Event: pending.Event, Accepted: true}, nil
		}
	} else {
		run, _, runErr := s.automaticRun(ctx, req, "")
		if runErr != nil {
			return nil, HostEventOut{}, runErr
		}
		attr := agent.HostRuntimeAttribution{ProjectID: s.projectIDValue, WorkspaceID: s.workspaceIDValue,
			TaskID: run.TaskID, SessionID: run.SessionID}
		if in.Generation == "" && in.Sequence == 0 && in.ObservedAt == "" {
			recorded, err = s.hostRuntimes.ObserveLifecycle(ctx, attr, agent.HostLifecycleInput{
				Host: in.Host, HostSessionID: in.HostSessionID, AgentID: in.AgentID, AgentType: in.AgentType,
				Event: in.Event, Source: in.Source, ModelKey: in.Model, Effort: in.Effort,
				ContextUsed: in.ContextUsed, ContextLimit: in.ContextLimit,
			})
		} else {
			observedAt, parseErr := time.Parse(time.RFC3339Nano, in.ObservedAt)
			if parseErr != nil {
				return nil, HostEventOut{}, Invalid("observed_at must be RFC3339")
			}
			event := agent.HostRuntimeEventInput{Host: in.Host, HostSessionID: in.HostSessionID, AgentID: in.AgentID,
				Generation: in.Generation, AgentType: in.AgentType, Event: in.Event, Sequence: in.Sequence,
				ObservedAt: observedAt, ModelKey: in.Model, Effort: in.Effort, ContextUsed: in.ContextUsed, ContextLimit: in.ContextLimit}
			recorded, err = s.hostRuntimes.Observe(ctx, attr, event)
		}
	}
	if err != nil {
		return nil, HostEventOut{}, Invalid(err.Error())
	}
	return nil, HostEventOut{RuntimeID: recorded.RuntimeID, Host: recorded.Host, Main: recorded.IsMain,
		ParentRuntimeID: recorded.ParentRuntimeID, AgentType: recorded.AgentType, State: recorded.LifecycleState,
		Event: recorded.LastEvent, Sequence: recorded.ProducerSequence, Accepted: recorded.Accepted}, nil
}
