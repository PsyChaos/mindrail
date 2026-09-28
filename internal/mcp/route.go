package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"math"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/workflow"
)

const (
	ToolRoute              = "mindrail_route"
	routeMinimumInterval   = 10 * time.Second
	routeDuplicateInterval = 30 * time.Second
	routeStateIdleInterval = routeDuplicateInterval
	routeStateLimit        = 256
)

type routeRunner interface {
	Route(context.Context, io.Reader) agent.RouteResult
}

type routeRecorder interface {
	Record(context.Context, agent.RouteEventInput) (agent.RouteEvent, error)
}

type RouteCandidate struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type RouteIn struct {
	Goal    string           `json:"goal"`
	Context any              `json:"context,omitempty"`
	Tools   []RouteCandidate `json:"tools,omitempty"`
	Agents  []RouteCandidate `json:"agents,omitempty"`
	Models  []RouteCandidate `json:"models,omitempty"`
	Efforts []RouteCandidate `json:"efforts,omitempty"`
}

type RouteSelection struct {
	Candidate  *string `json:"candidate"`
	Confidence float64 `json:"confidence"`
	Accepted   bool    `json:"accepted"`
	Reason     string  `json:"reason"`
}

type RouteOut struct {
	Version           int                       `json:"version"`
	Enabled           bool                      `json:"enabled"`
	Advisory          bool                      `json:"advisory"`
	Mode              string                    `json:"mode"`
	Status            string                    `json:"status"`
	Reason            string                    `json:"reason"`
	Selections        map[string]RouteSelection `json:"selections"`
	RouteID           string                    `json:"route_id,omitempty"`
	TelemetryRecorded bool                      `json:"telemetry_recorded"`
	credentialSource  string
	candidateCounts   map[string]int
	ordinals          map[string]int
}

type routeSessionState struct {
	mu                     sync.Mutex
	lastProvider           time.Time
	recent                 map[[sha256.Size]byte]time.Time
	lastUsed               time.Time
	active                 int
	lastDuplicateTelemetry time.Time
	lastRateTelemetry      time.Time
}

func (s *Server) registerRoute() {
	sdk.AddTool(s.impl, &sdk.Tool{
		Name:        ToolRoute,
		Description: "Return optional JEV advice for caller-filtered tool, agent, model, or reasoning-effort candidates. Advice never grants permission or bypasses Mindrail gates.",
	}, s.route)
}

func (s *Server) route(ctx context.Context, req *sdk.CallToolRequest, in RouteIn) (*sdk.CallToolResult, RouteOut, error) {
	run, _, err := s.automaticRun(ctx, req, "")
	if err != nil {
		return nil, RouteOut{}, err
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, RouteOut{}, Invalid("route input is not JSON encodable")
	}
	acquiredAt := s.clock.Now().UTC()
	state, release, acquired := s.acquireRouteState(run.SessionID, acquiredAt)
	if !acquired {
		return nil, throttledRoute("rate_limited", in), nil
	}
	defer release()
	if !state.mu.TryLock() {
		return nil, throttledRoute("rate_limited", in), nil
	}
	defer state.mu.Unlock()

	now := s.clock.Now().UTC()
	digest := sha256.Sum256(raw)
	state.prune(now)
	if seen, ok := state.recent[digest]; ok && now.Sub(seen) < routeDuplicateInterval {
		return nil, s.recordThrottledRoute(ctx, run, state, "duplicate", in, now), nil
	}
	if !state.lastProvider.IsZero() && now.Sub(state.lastProvider) < routeMinimumInterval {
		return nil, s.recordThrottledRoute(ctx, run, state, "rate_limited", in, now), nil
	}
	state.lastProvider = now
	state.recent[digest] = now
	result := s.router.Route(ctx, bytes.NewReader(raw))
	ended := s.clock.Now().UTC()
	out := routeOut(result)
	out.candidateCounts = routeCandidateCounts(in)
	return nil, s.recordRoute(ctx, run, out, now, ended), nil
}

func (s *Server) recordThrottledRoute(ctx context.Context, run workflow.Run, state *routeSessionState, reason string, in RouteIn, now time.Time) RouteOut {
	out := throttledRoute(reason, in)
	if !state.admitFallbackTelemetry(reason, now) {
		return out
	}
	return s.recordRoute(ctx, run, out, now, now)
}

func (s *Server) acquireRouteState(sessionID string, now time.Time) (*routeSessionState, func(), bool) {
	s.routeMu.Lock()
	defer s.routeMu.Unlock()
	s.pruneRouteStates(now)
	state := s.routeSessions[sessionID]
	if state == nil {
		if len(s.routeSessions) >= routeStateLimit {
			return nil, nil, false
		}
		state = &routeSessionState{recent: make(map[[sha256.Size]byte]time.Time), lastUsed: now}
		s.routeSessions[sessionID] = state
	}
	state.active++
	return state, func() {
		s.routeMu.Lock()
		state.active--
		state.lastUsed = s.clock.Now().UTC()
		s.routeMu.Unlock()
	}, true
}

func (s *Server) pruneRouteStates(now time.Time) {
	for key, state := range s.routeSessions {
		if state.active == 0 && now.Sub(state.lastUsed) >= routeStateIdleInterval {
			delete(s.routeSessions, key)
		}
	}
}

func (state *routeSessionState) prune(now time.Time) {
	for digest, seen := range state.recent {
		if now.Sub(seen) >= routeDuplicateInterval {
			delete(state.recent, digest)
		}
	}
}

func (state *routeSessionState) admitFallbackTelemetry(reason string, now time.Time) bool {
	var last *time.Time
	var window time.Duration
	switch reason {
	case "duplicate":
		last, window = &state.lastDuplicateTelemetry, routeDuplicateInterval
	case "rate_limited":
		last, window = &state.lastRateTelemetry, routeMinimumInterval
	default:
		return false
	}
	if !last.IsZero() && now.Sub(*last) < window {
		return false
	}
	*last = now
	return true
}

func routeOut(result agent.RouteResult) RouteOut {
	out := RouteOut{
		Version: result.Version, Enabled: result.Enabled, Advisory: result.Advisory,
		Mode: result.Mode, Status: result.Status, Reason: result.Reason,
		Selections:       make(map[string]RouteSelection, len(result.Selections)),
		credentialSource: result.CredentialSource, candidateCounts: result.CandidateCounts,
		ordinals: make(map[string]int, len(result.Selections)),
	}
	for dimension, selection := range result.Selections {
		out.Selections[dimension] = RouteSelection{
			Candidate: selection.Candidate, Confidence: selection.Confidence,
			Accepted: selection.Accepted, Reason: selection.Reason,
		}
		out.ordinals[dimension] = selection.Ordinal
	}
	return out
}

func throttledRoute(reason string, in RouteIn) RouteOut {
	return RouteOut{
		Version: 1, Enabled: true, Advisory: true, Mode: "shadow",
		Status: agent.RouteStatusFallback, Reason: reason,
		Selections: map[string]RouteSelection{}, credentialSource: agent.RouteCredentialNone,
		candidateCounts: routeCandidateCounts(in), ordinals: map[string]int{},
	}
}

func routeCandidateCounts(in RouteIn) map[string]int {
	return map[string]int{
		"tool": len(in.Tools), "agent": len(in.Agents), "model": len(in.Models), "effort": len(in.Efforts),
	}
}

func (s *Server) recordRoute(ctx context.Context, run workflow.Run, out RouteOut, started, ended time.Time) RouteOut {
	input := agent.RouteEventInput{
		ProjectID: s.projectIDValue, WorkspaceID: s.workspaceIDValue,
		TaskID: run.TaskID, SessionID: run.SessionID,
		Provider: agent.RouteProviderTypeSafe, Model: agent.RouteModelJEVLatest, Version: 1,
		Status: out.Status, Reason: out.Reason, CredentialSource: agent.RouteCredentialNone,
		StartedAt: started, EndedAt: ended,
	}
	if out.credentialSource != "" {
		input.CredentialSource = out.credentialSource
	}
	input.Tool = routeDimension("tool", out)
	input.Agent = routeDimension("agent", out)
	input.ModelChoice = routeDimension("model", out)
	input.Effort = routeDimension("effort", out)
	event, err := s.routes.Record(ctx, input)
	if err != nil {
		out.RouteID = ""
		out.TelemetryRecorded = false
		if out.Status == agent.RouteStatusOK {
			out.Status = agent.RouteStatusFallback
			out.Reason = "telemetry_failed"
			out.Selections = map[string]RouteSelection{}
		}
		return out
	}
	out.RouteID = event.ID
	out.TelemetryRecorded = true
	return out
}

func routeDimension(name string, out RouteOut) agent.RouteDimension {
	dimension := agent.RouteDimension{CandidateCount: out.candidateCounts[name]}
	selection, ok := out.Selections[name]
	if !ok {
		return dimension
	}
	confidence := int(math.Round(selection.Confidence * 1000))
	dimension.ConfidenceMilli = &confidence
	if selection.Candidate == nil {
		return dimension
	}
	ordinal := -1
	if value, ok := out.ordinals[name]; ok {
		ordinal = value
	}
	if ordinal < 0 || ordinal >= dimension.CandidateCount {
		return dimension
	}
	dimension.SelectedOrdinal = &ordinal
	return dimension
}
