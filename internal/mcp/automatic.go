package mcp

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/status"
	"github.com/PsyChaos/mindrail/internal/workflow"
)

// automaticHeartbeat is the narrow lifecycle the MCP adapter needs. Keeping
// it behind this interface makes connection cleanup testable without teaching
// the SDK-neutral workflow service about MCP sessions.
type automaticHeartbeat interface {
	Stop()
	Health() error
}

type presenceRecorder interface {
	Start(context.Context, agent.PresenceStart) (agent.RuntimePresence, error)
	Heartbeat(context.Context, string) (agent.RuntimePresence, error)
	Activity(context.Context, string) (agent.RuntimePresence, error)
	End(context.Context, string, string) (agent.RuntimePresence, error)
}

type presenceBinding struct {
	id         string
	cancel     context.CancelFunc
	cancelOnce sync.Once
	mu         sync.Mutex
	ended      bool
	reason     string
}

// workflowPort is deliberately the workflow service's public orchestration
// surface and no more. MCP maps wire requests to it; domain composition and
// durable replay remain in internal/workflow.
type workflowPort interface {
	Start(context.Context, workflow.StartInput) (workflow.Run, error)
	Resolve(context.Context, string) (workflow.Run, error)
	ExtendScope(context.Context, string, []string) (workflow.Run, error)
	Reconcile(context.Context, string) (changes.ReconcileResult, error)
	Finalize(context.Context, workflow.FinalizeInput) (workflow.Finalization, error)
	Checkpoint(context.Context, string, string, bool) (coordination.Noted, error)
	CheckpointByRunHash(context.Context, []byte, string, bool) (coordination.Noted, error)
	StartHeartbeat(context.Context, string, time.Duration) (automaticHeartbeat, error)
}

type workflowAdapter struct{ *workflow.Service }

func (a workflowAdapter) StartHeartbeat(ctx context.Context, runKey string, interval time.Duration) (automaticHeartbeat, error) {
	return a.Service.StartHeartbeat(ctx, runKey, interval)
}

type automaticContext struct {
	run                workflow.Run
	heartbeat          automaticHeartbeat
	presence           *presenceBinding
	presenceStarting   bool
	presenceGeneration uint64
	watched            bool
	continuityIntentID string
}

const (
	automaticHeartbeatInterval     = coordination.LeaseTTL / 3
	agentPresenceHeartbeatInterval = 5 * time.Second
	presenceEndAttempts            = 3
)

func (s *Server) automaticStart(ctx context.Context, req *sdk.CallToolRequest, in BootstrapIn) (BootstrapOut, error) {
	if req == nil || req.Session == nil {
		return BootstrapOut{}, Invalid("automatic bootstrap needs an MCP connection")
	}
	// workflow.Start is already serialized at the domain layer. Mirror that
	// here so two simultaneous bootstrap calls on one MCP connection cannot
	// both create durable runs before the connection chooses one of them.
	s.autoStartMu.Lock()
	defer s.autoStartMu.Unlock()
	current, hasCurrent := s.automaticSnapshot(req.Session)
	goal, runKey := *in.Goal, *in.RunKey
	var paths []string
	if in.Paths != nil {
		paths = *in.Paths
	}
	var resumeTaskID string
	if in.ResumeTaskID != nil {
		resumeTaskID = *in.ResumeTaskID
	}
	if hasCurrent && current.run.RunKey != runKey && !terminalRun(current.run) {
		return BootstrapOut{}, Invalid("this MCP connection already has an automatic run")
	}
	takeover := in.ContinuityIntentID != nil || in.TakeoverToken != nil
	if takeover {
		if in.ContinuityIntentID == nil || in.TakeoverToken == nil || resumeTaskID == "" || s.continuityService == nil {
			return BootstrapOut{}, Invalid("continuity takeover needs continuity_intent_id, takeover_token, and resume_task_id")
		}
		claimed, err := s.continuityService.ClaimTakeover(ctx, *in.ContinuityIntentID, *in.TakeoverToken, runKey)
		if err != nil || claimed.TaskID != resumeTaskID {
			return BootstrapOut{}, Invalid("continuity takeover token is invalid or no longer available")
		}
	}
	run, startErr := s.workflow.Start(ctx, workflow.StartInput{
		Goal: goal, RunKey: runKey, Paths: paths, ResumeTaskID: resumeTaskID,
	})
	out := s.bootstrapRunOut(run)
	if startErr != nil {
		if run.RunKey == "" {
			return BootstrapOut{}, startErr
		}
		if hasCurrent && (run.TaskID == "" || run.SessionID == "") {
			out = s.bootstrapRunOut(current.run)
		}
		out.Failure = automaticFailure(startErr)
		return out, nil
	}
	if takeover {
		if _, err := s.continuityService.CompleteTakeover(ctx, *in.ContinuityIntentID, runKey, run.SessionID); err != nil {
			out.Failure = automaticFailure(err)
			return out, nil
		}
	}

	s.autoMu.Lock()
	currentPtr := s.automatic[req.Session]
	if currentPtr != nil && currentPtr.run.RunKey != run.RunKey && !terminalRun(currentPtr.run) {
		s.autoMu.Unlock()
		return BootstrapOut{}, Invalid("this MCP connection already has an automatic run")
	}
	if currentPtr != nil && currentPtr.run.RunKey == run.RunKey && currentPtr.run.Revision > run.Revision {
		// A concurrent finalization/scope response may have published newer
		// durable state while Start's response was in flight.
		run = currentPtr.run
		out = s.bootstrapRunOut(run)
	}
	if currentPtr != nil && currentPtr.run.RunKey == run.RunKey && terminalRun(run) {
		currentPtr.run = run
		heartbeat := currentPtr.heartbeat
		presence := currentPtr.presence
		currentPtr.heartbeat = nil
		currentPtr.presenceStarting = false
		currentPtr.presenceGeneration++
		s.autoMu.Unlock()
		if heartbeat != nil {
			heartbeat.Stop()
		}
		if s.stopPresence(presence, agent.RuntimeEndCompleted) {
			s.clearPresenceIfCurrent(req.Session, presence)
		}
		out.Started = true
		return out, nil
	}
	if currentPtr != nil && currentPtr.run.RunKey == run.RunKey && currentPtr.heartbeat != nil && currentPtr.heartbeat.Health() == nil {
		currentPtr.run = run
		s.autoMu.Unlock()
		s.ensurePresence(ctx, req, run)
		out.Started = true
		return out, nil
	}
	s.autoMu.Unlock()

	var heartbeat automaticHeartbeat
	var heartbeatErr error
	if !terminalRun(run) {
		heartbeat, heartbeatErr = s.workflow.StartHeartbeat(context.Background(), run.RunKey, automaticHeartbeatInterval)
	}
	published, accepted := s.rememberAutomatic(req.Session, automaticContext{run: run, heartbeat: heartbeat})
	if published.run.RunKey != run.RunKey {
		return BootstrapOut{}, Invalid("this MCP connection already has an automatic run")
	}
	out = s.bootstrapRunOut(published.run)
	if accepted && heartbeatErr != nil {
		out.Failure = automaticFailure(heartbeatErr)
		return out, nil
	}
	if accepted && !terminalRun(published.run) {
		s.ensurePresence(ctx, req, published.run)
		if s.hostRuntimes != nil {
			attr := agent.HostRuntimeAttribution{ProjectID: s.projectIDValue, WorkspaceID: s.workspaceIDValue,
				TaskID: published.run.TaskID, SessionID: published.run.SessionID}
			_, _ = s.hostRuntimes.ClaimPendingRun(ctx, attr, published.run.RunKey, s.automaticPresenceID(req.Session))
		}
	}
	out.Started = true
	return out, nil
}

func (s *Server) automaticPresenceID(session *sdk.ServerSession) string {
	if session == nil {
		return ""
	}
	s.autoMu.RLock()
	defer s.autoMu.RUnlock()
	if current := s.automatic[session]; current != nil && current.presence != nil {
		return current.presence.id
	}
	return ""
}

func (s *Server) automaticSnapshot(session *sdk.ServerSession) (automaticContext, bool) {
	s.autoMu.RLock()
	defer s.autoMu.RUnlock()
	current := s.automatic[session]
	if current == nil {
		return automaticContext{}, false
	}
	return *current, true
}

func automaticFailure(err error) *AutomaticFailure {
	failure := &AutomaticFailure{Message: err.Error()}
	if payload, ok := app.PayloadOf(err); ok {
		failure.Code = string(payload.Code)
		failure.Why = payload.Why
		failure.Impact = payload.Impact
		failure.NextAction = payload.NextAction
		failure.Metadata = payload.Metadata
	}
	if failure.NextAction == nil {
		failure.NextAction = []string{}
	}
	return failure
}

func terminalRun(run workflow.Run) bool {
	return run.State == coordination.StateCompleted || run.State == coordination.StateAbandoned
}

func (s *Server) rememberAutomatic(session *sdk.ServerSession, next automaticContext) (automaticContext, bool) {
	s.autoMu.Lock()
	current := s.automatic[session]
	if current != nil && ((current.run.RunKey == next.run.RunKey && current.run.Revision > next.run.Revision) ||
		(current.run.RunKey != next.run.RunKey && !terminalRun(current.run))) {
		// Recheck at publication, including the gap while a heartbeat starts.
		// A discarded reply must not replace or stop the current run's owner.
		preserved := *current
		s.autoMu.Unlock()
		if next.heartbeat != nil && next.heartbeat != preserved.heartbeat {
			next.heartbeat.Stop()
		}
		return preserved, false
	}
	var replacedPresence *presenceBinding
	if current != nil {
		next.watched = current.watched
		if current.run.RunKey == next.run.RunKey {
			next.presence = current.presence
			next.presenceStarting = current.presenceStarting
			next.presenceGeneration = current.presenceGeneration
			next.continuityIntentID = current.continuityIntentID
		} else {
			replacedPresence = current.presence
		}
		if current.heartbeat != nil && current.heartbeat != next.heartbeat {
			current.heartbeat.Stop()
		}
	}
	if !next.watched {
		next.watched = true
		go func() {
			_ = session.Wait()
			s.forgetAutomatic(session)
		}()
	}
	s.automatic[session] = &next
	s.autoMu.Unlock()
	s.stopPresence(replacedPresence, agent.RuntimeEndReplaced)
	return next, true
}

func (s *Server) automaticRun(ctx context.Context, req *sdk.CallToolRequest, explicitRunKey string) (workflow.Run, automaticHeartbeat, error) {
	if req == nil || req.Session == nil {
		return workflow.Run{}, nil, Invalid("automatic operation needs an MCP connection")
	}
	s.autoMu.RLock()
	current := s.automatic[req.Session]
	if current == nil {
		s.autoMu.RUnlock()
		if explicitRunKey == "" {
			return workflow.Run{}, nil, Invalid("automatic operation needs mindrail_bootstrap on this connection")
		}
		run, err := s.workflow.Resolve(ctx, explicitRunKey)
		if err != nil {
			return workflow.Run{}, nil, err
		}
		var heartbeat automaticHeartbeat
		if !terminalRun(run) {
			heartbeat, err = s.workflow.StartHeartbeat(context.Background(), run.RunKey, automaticHeartbeatInterval)
			if err != nil {
				return workflow.Run{}, nil, err
			}
		}
		published, _ := s.rememberAutomatic(req.Session, automaticContext{run: run, heartbeat: heartbeat})
		if published.run.RunKey != explicitRunKey {
			return workflow.Run{}, nil, Invalid("run_key does not match this MCP connection")
		}
		if !terminalRun(published.run) {
			s.ensurePresence(ctx, req, published.run)
		}
		return published.run, published.heartbeat, nil
	}
	run := current.run
	heartbeat := current.heartbeat
	s.autoMu.RUnlock()
	if explicitRunKey != "" && explicitRunKey != run.RunKey {
		return workflow.Run{}, nil, Invalid("run_key does not match this MCP connection")
	}
	return run, heartbeat, nil
}

func (s *Server) updateAutomatic(session *sdk.ServerSession, run workflow.Run) {
	s.autoMu.Lock()
	if current := s.automatic[session]; current != nil && current.run.RunKey == run.RunKey && current.run.Revision <= run.Revision {
		current.run = run
	}
	s.autoMu.Unlock()
}

// retainTerminalAutomatic keeps the connection-to-run binding for an exact
// ID-free finalize retry, while stopping renewal for work that is complete.
// A later bootstrap may replace this terminal binding with a fresh run.
func (s *Server) retainTerminalAutomatic(session *sdk.ServerSession, run workflow.Run) {
	s.autoMu.Lock()
	current := s.automatic[session]
	var heartbeat automaticHeartbeat
	var presence *presenceBinding
	if current != nil && current.run.RunKey == run.RunKey && current.run.Revision <= run.Revision {
		current.run = run
		heartbeat = current.heartbeat
		current.heartbeat = nil
		presence = current.presence
		current.presenceStarting = false
		current.presenceGeneration++
	}
	s.autoMu.Unlock()
	if heartbeat != nil {
		heartbeat.Stop()
	}
	if s.stopPresence(presence, agent.RuntimeEndCompleted) {
		s.clearPresenceIfCurrent(session, presence)
	}
}

func (s *Server) forgetAutomatic(session *sdk.ServerSession) {
	s.autoMu.Lock()
	current := s.automatic[session]
	delete(s.automatic, session)
	s.autoMu.Unlock()
	if current != nil && current.heartbeat != nil {
		current.heartbeat.Stop()
	}
	if current != nil {
		s.stopPresence(current.presence, agent.RuntimeEndDisconnected)
	}
}

// forgetAutomaticIfCurrent belongs to a completed request, unlike connection
// shutdown. A delayed handoff reply must not remove a newer binding or stop
// the heartbeat another run/revision installed on the same connection.
func (s *Server) forgetAutomaticIfCurrent(session *sdk.ServerSession, expected workflow.Run) {
	s.autoMu.Lock()
	current := s.automatic[session]
	if current == nil || current.run.RunKey != expected.RunKey || current.run.Revision != expected.Revision {
		s.autoMu.Unlock()
		return
	}
	delete(s.automatic, session)
	s.autoMu.Unlock()
	if current.heartbeat != nil {
		current.heartbeat.Stop()
	}
	s.stopPresence(current.presence, agent.RuntimeEndReplaced)
}

func (s *Server) closeAutomatic() {
	s.autoMu.Lock()
	contexts := s.automatic
	s.automatic = make(map[*sdk.ServerSession]*automaticContext)
	s.autoMu.Unlock()
	for _, current := range contexts {
		if current.heartbeat != nil {
			current.heartbeat.Stop()
		}
		s.stopPresence(current.presence, agent.RuntimeEndShutdown)
	}
}

func (s *Server) ensurePresence(ctx context.Context, req *sdk.CallToolRequest, run workflow.Run) {
	if s.presence == nil || req == nil || req.Session == nil || terminalRun(run) {
		return
	}
	s.autoMu.RLock()
	current := s.automatic[req.Session]
	if current == nil || current.run.RunKey != run.RunKey || current.presence != nil || current.presenceStarting {
		s.autoMu.RUnlock()
		return
	}
	s.autoMu.RUnlock()
	s.autoMu.Lock()
	current = s.automatic[req.Session]
	if current == nil || current.run.RunKey != run.RunKey || current.presence != nil || current.presenceStarting {
		s.autoMu.Unlock()
		return
	}
	current.presenceStarting = true
	current.presenceGeneration++
	generation := current.presenceGeneration
	s.autoMu.Unlock()

	client := agent.ClientInfo{Name: "unknown"}
	if info := req.ClientInfo(); info != nil {
		client.Name = boundedClientValue(info.Name, 128)
		client.Title = boundedClientValue(info.Title, 256)
		client.Version = boundedClientValue(info.Version, 64)
		if client.Name == "" {
			client.Name = "unknown"
		}
	}
	runtime, err := s.presence.Start(ctx, agent.PresenceStart{
		ProjectID: s.projectIDValue, WorkspaceID: s.workspaceIDValue,
		TaskID: run.TaskID, SessionID: run.SessionID, Client: client,
	})
	if err != nil {
		s.autoMu.Lock()
		if current = s.automatic[req.Session]; current != nil && current.run.RunKey == run.RunKey && current.presenceGeneration == generation {
			current.presenceStarting = false
		}
		s.autoMu.Unlock()
		return
	}
	heartbeatCtx, cancel := context.WithCancel(context.Background())
	binding := &presenceBinding{id: runtime.ID, cancel: cancel}
	s.autoMu.Lock()
	current = s.automatic[req.Session]
	if current == nil || current.run.RunKey != run.RunKey || current.presenceGeneration != generation ||
		terminalRun(current.run) || current.presence != nil || !current.presenceStarting {
		if current != nil && current.run.RunKey == run.RunKey && current.presenceGeneration == generation {
			current.presenceStarting = false
		}
		s.autoMu.Unlock()
		s.stopPresence(binding, agent.RuntimeEndReplaced)
		return
	}
	current.presenceStarting = false
	current.presence = binding
	s.autoMu.Unlock()
	go s.runPresenceHeartbeat(heartbeatCtx, binding.id)
}

func (s *Server) runPresenceHeartbeat(ctx context.Context, runtimeID string) {
	interval := s.presenceHeartbeatTestInterval
	if interval <= 0 {
		interval = agentPresenceHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			callCtx, cancel := context.WithTimeout(context.Background(), interval)
			_, _ = s.presence.Heartbeat(callCtx, runtimeID)
			cancel()
		}
	}
}

func (s *Server) stopPresence(binding *presenceBinding, reason string) bool {
	if binding == nil || s.presence == nil {
		return true
	}
	binding.cancelOnce.Do(func() {
		if binding.cancel != nil {
			binding.cancel()
		}
	})
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.ended {
		return true
	}
	if binding.reason == "" {
		binding.reason = reason
	}
	for range presenceEndAttempts {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := s.presence.End(ctx, binding.id, binding.reason)
		cancel()
		if err == nil {
			binding.ended = true
			return true
		}
	}
	return false
}

func (s *Server) presenceActivityMiddleware(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, request sdk.Request) (sdk.Result, error) {
		session, _ := request.GetSession().(*sdk.ServerSession)
		touched := false
		if method == "tools/call" {
			touched = s.touchPresence(ctx, session)
		}
		result, err := next(ctx, method, request)
		if method == "tools/call" && !touched {
			s.touchPresence(ctx, session)
		}
		if method == "tools/call" {
			s.recordRuntimeTelemetry(ctx, session, request)
		}
		return result, err
	}
}

func (s *Server) touchPresence(ctx context.Context, session *sdk.ServerSession) bool {
	if session == nil || s.presence == nil {
		return false
	}
	s.autoMu.RLock()
	current := s.automatic[session]
	var runtimeID string
	var run workflow.Run
	var binding *presenceBinding
	if current != nil && current.presence != nil {
		runtimeID = current.presence.id
		binding = current.presence
	}
	if current != nil {
		run = current.run
	}
	s.autoMu.RUnlock()
	if terminalRun(run) && binding != nil {
		if s.stopPresence(binding, agent.RuntimeEndCompleted) {
			s.clearPresenceIfCurrent(session, binding)
		}
		return false
	}
	if runtimeID == "" && run.RunKey != "" && !terminalRun(run) {
		s.ensurePresence(ctx, &sdk.CallToolRequest{Session: session}, run)
		s.autoMu.RLock()
		if current = s.automatic[session]; current != nil && current.run.RunKey == run.RunKey && current.presence != nil {
			runtimeID = current.presence.id
		}
		s.autoMu.RUnlock()
	}
	if runtimeID != "" {
		_, _ = s.presence.Activity(ctx, runtimeID)
		return true
	}
	return false
}

func (s *Server) clearPresenceIfCurrent(session *sdk.ServerSession, binding *presenceBinding) {
	s.autoMu.Lock()
	if current := s.automatic[session]; current != nil && current.presence == binding {
		current.presence = nil
	}
	s.autoMu.Unlock()
}

func boundedClientValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	for len(value) > limit {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}

func (s *Server) bootstrapRunOut(run workflow.Run) BootstrapOut {
	paths := s.app.Paths()
	return BootstrapOut{
		WorktreeRoot: paths.WorktreeRoot,
		RuntimeRoot:  paths.RuntimeRoot,
		DBPath:       paths.DBPath,
		Readiness:    string(status.Build(s.app.Subject(), time.Since(s.started)).Readiness),
		Automatic:    true,
		RunKey:       run.RunKey,
		SessionID:    run.SessionID,
		TaskID:       run.TaskID,
		State:        string(run.State),
		Revision:     run.Revision,
		Paths:        run.Paths,
	}
}
