package mcp

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/workflow"
)

type routeTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *routeTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *routeTestClock) Add(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	c.mu.Unlock()
}

type fakeRouteRunner struct {
	result agent.RouteResult
	calls  atomic.Int32
	run    func()
}

func (r *fakeRouteRunner) Route(context.Context, io.Reader) agent.RouteResult {
	r.calls.Add(1)
	if r.run != nil {
		r.run()
	}
	return r.result
}

type fakeRouteRecorder struct {
	mu     sync.Mutex
	inputs []agent.RouteEventInput
	err    error
}

type fakePresenceRecorder struct {
	mu            sync.Mutex
	starts        []agent.PresenceStart
	startFailures int
	heartbeats    []string
	activities    []string
	ends          map[string]string
	endFailures   int
	endCalls      int
	startEntered  chan struct{}
	startRelease  chan struct{}
}

func (p *fakePresenceRecorder) Start(_ context.Context, in agent.PresenceStart) (agent.RuntimePresence, error) {
	p.mu.Lock()
	p.starts = append(p.starts, in)
	call := len(p.starts)
	failed := call <= p.startFailures
	entered, release := p.startEntered, p.startRelease
	p.mu.Unlock()
	if entered != nil && call == 1 {
		close(entered)
		<-release
	}
	if failed {
		return agent.RuntimePresence{}, errors.New("start unavailable")
	}
	id := "RUN-" + string(rune('0'+call))
	return agent.RuntimePresence{ID: id}, nil
}

func (p *fakePresenceRecorder) Heartbeat(_ context.Context, id string) (agent.RuntimePresence, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.heartbeats = append(p.heartbeats, id)
	return agent.RuntimePresence{ID: id}, nil
}

func (p *fakePresenceRecorder) Activity(_ context.Context, id string) (agent.RuntimePresence, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activities = append(p.activities, id)
	return agent.RuntimePresence{ID: id}, nil
}

func (p *fakePresenceRecorder) End(_ context.Context, id, reason string) (agent.RuntimePresence, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.endCalls++
	if p.endCalls <= p.endFailures {
		return agent.RuntimePresence{}, errors.New("end unavailable")
	}
	if p.ends == nil {
		p.ends = make(map[string]string)
	}
	if _, exists := p.ends[id]; !exists {
		p.ends[id] = reason
	}
	return agent.RuntimePresence{ID: id, EndReason: p.ends[id]}, nil
}

func (p *fakePresenceRecorder) snapshot() (starts, heartbeats, activities int, ends map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	copyEnds := make(map[string]string, len(p.ends))
	for id, reason := range p.ends {
		copyEnds[id] = reason
	}
	return len(p.starts), len(p.heartbeats), len(p.activities), copyEnds
}

func (r *fakeRouteRecorder) Record(_ context.Context, in agent.RouteEventInput) (agent.RouteEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inputs = append(r.inputs, in)
	if r.err != nil {
		return agent.RouteEvent{}, r.err
	}
	return agent.RouteEvent{ID: "JRV-safe"}, nil
}

func newRouteUnitServer(runner routeRunner, recorder routeRecorder, clock *routeTestClock, session *sdk.ServerSession) *Server {
	return &Server{
		router: runner, routes: recorder, clock: clock,
		projectIDValue: "PRJ-1", workspaceIDValue: "WSP-1",
		routeSessions: make(map[string]*routeSessionState),
		automatic: map[*sdk.ServerSession]*automaticContext{session: {run: workflow.Run{
			TaskID: "TSK-1", SessionID: "SES-1", RunKey: "run-1",
		}}},
	}
}

func acceptedRouteResult() agent.RouteResult {
	candidate := "rg"
	return agent.RouteResult{
		Version: 1, Enabled: true, Advisory: true, Mode: "shadow",
		Status: agent.RouteStatusOK, Reason: agent.RouteReasonAdviceAvailable,
		CredentialSource: agent.RouteCredentialKeyring,
		CandidateCounts:  map[string]int{"tool": 2},
		Selections: map[string]agent.Selection{"tool": {
			Candidate: &candidate, Confidence: .875, Accepted: true, Reason: "accepted", Ordinal: 1,
		}},
	}
}

func TestRouteAcceptedAdviceRecordsOnlySafeOrdinalTelemetry(t *testing.T) {
	session := new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)}
	runner := &fakeRouteRunner{result: acceptedRouteResult()}
	recorder := &fakeRouteRecorder{}
	server := newRouteUnitServer(runner, recorder, clock, session)

	_, out, err := server.route(t.Context(), &sdk.CallToolRequest{Session: session}, RouteIn{
		Goal:  "choose a repository search tool",
		Tools: []RouteCandidate{{ID: "git", Description: "history"}, {ID: "rg", Description: "text search"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "ok" || !out.TelemetryRecorded || out.RouteID != "JRV-safe" || len(out.Selections) != 1 {
		t.Fatalf("route output = %#v", out)
	}
	if len(recorder.inputs) != 1 {
		t.Fatalf("records = %d", len(recorder.inputs))
	}
	recorded := recorder.inputs[0]
	if recorded.ProjectID != "PRJ-1" || recorded.WorkspaceID != "WSP-1" || recorded.TaskID != "TSK-1" || recorded.SessionID != "SES-1" {
		t.Fatalf("attribution = %#v", recorded)
	}
	if recorded.Tool.CandidateCount != 2 || recorded.Tool.SelectedOrdinal == nil || *recorded.Tool.SelectedOrdinal != 1 || recorded.Tool.ConfidenceMilli == nil || *recorded.Tool.ConfidenceMilli != 875 {
		t.Fatalf("safe tool summary = %#v", recorded.Tool)
	}
}

func TestRouteClearsSuccessfulAdviceWhenTelemetryFails(t *testing.T) {
	session := new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Now()}
	server := newRouteUnitServer(&fakeRouteRunner{result: acceptedRouteResult()}, &fakeRouteRecorder{err: errors.New("db unavailable")}, clock, session)

	_, out, err := server.route(t.Context(), &sdk.CallToolRequest{Session: session}, RouteIn{
		Goal: "choose", Tools: []RouteCandidate{{ID: "git", Description: "history"}, {ID: "rg", Description: "search"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "fallback" || out.Reason != "telemetry_failed" || out.TelemetryRecorded || len(out.Selections) != 0 {
		t.Fatalf("unproven advice escaped: %#v", out)
	}
}

func TestRouteMissingKeyRemainsDisabledAndFailOpen(t *testing.T) {
	session := new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Now()}
	runner := &fakeRouteRunner{result: agent.RouteResult{
		Version: 1, Enabled: false, Advisory: true, Mode: "shadow",
		Status: agent.RouteStatusDisabled, Reason: "api_key_missing",
		CredentialSource: agent.RouteCredentialNone, Selections: map[string]agent.Selection{},
		CandidateCounts: map[string]int{"tool": 2},
	}}
	recorder := &fakeRouteRecorder{}
	server := newRouteUnitServer(runner, recorder, clock, session)
	_, out, err := server.route(t.Context(), &sdk.CallToolRequest{Session: session}, RouteIn{
		Goal: "choose", Tools: []RouteCandidate{{ID: "rg", Description: "search"}, {ID: "git", Description: "history"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Enabled || out.Status != "disabled" || out.Reason != "api_key_missing" || !out.TelemetryRecorded || len(out.Selections) != 0 {
		t.Fatalf("disabled route = %#v", out)
	}
	if len(recorder.inputs) != 1 || recorder.inputs[0].CredentialSource != agent.RouteCredentialNone || recorder.inputs[0].Tool.CandidateCount != 2 {
		t.Fatalf("disabled telemetry = %#v", recorder.inputs)
	}
}

func TestRouteDuplicateAndMinimumIntervalSkipProvider(t *testing.T) {
	session := new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Now()}
	runner := &fakeRouteRunner{result: acceptedRouteResult()}
	server := newRouteUnitServer(runner, &fakeRouteRecorder{}, clock, session)
	req := &sdk.CallToolRequest{Session: session}
	first := RouteIn{Goal: "choose", Tools: []RouteCandidate{{ID: "rg", Description: "search"}, {ID: "git", Description: "history"}}}
	if _, out, err := server.route(t.Context(), req, first); err != nil || out.Status != "ok" {
		t.Fatalf("first = %#v, %v", out, err)
	}
	if _, out, err := server.route(t.Context(), req, first); err != nil || out.Reason != "duplicate" {
		t.Fatalf("duplicate = %#v, %v", out, err)
	}
	different := first
	different.Goal = "choose another"
	if _, out, err := server.route(t.Context(), req, different); err != nil || out.Reason != "rate_limited" {
		t.Fatalf("rate limit = %#v, %v", out, err)
	}
	if runner.calls.Load() != 1 {
		t.Fatalf("provider calls = %d", runner.calls.Load())
	}
}

func TestRouteCallsOnOneConnectionAreSerialized(t *testing.T) {
	session := new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Now()}
	var active, maximum atomic.Int32
	runner := &fakeRouteRunner{result: acceptedRouteResult()}
	runner.run = func() {
		current := active.Add(1)
		for {
			seen := maximum.Load()
			if current <= seen || maximum.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		clock.Add(11 * time.Second)
		active.Add(-1)
	}
	server := newRouteUnitServer(runner, &fakeRouteRecorder{}, clock, session)
	req := &sdk.CallToolRequest{Session: session}

	var wg sync.WaitGroup
	for index := range 2 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, _, _ = server.route(t.Context(), req, RouteIn{
				Goal: string(rune('a' + index)), Tools: []RouteCandidate{{ID: "rg", Description: "search"}, {ID: "git", Description: "history"}},
			})
		}(index)
	}
	wg.Wait()
	if maximum.Load() != 1 || runner.calls.Load() != 2 {
		t.Fatalf("maximum concurrent=%d provider calls=%d", maximum.Load(), runner.calls.Load())
	}
}

func TestRouteStateFollowsDurableSessionAcrossTransportBoundaries(t *testing.T) {
	one, two := new(sdk.ServerSession), new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Now()}
	runner := &fakeRouteRunner{result: acceptedRouteResult()}
	server := newRouteUnitServer(runner, &fakeRouteRecorder{}, clock, one)
	server.automatic[two] = &automaticContext{run: workflow.Run{TaskID: "TSK-1", SessionID: "SES-1", RunKey: "run-1"}}
	input := RouteIn{Goal: "choose", Tools: []RouteCandidate{{ID: "rg", Description: "search"}}}
	if _, out, err := server.route(t.Context(), &sdk.CallToolRequest{Session: one}, input); err != nil || out.Status != agent.RouteStatusOK {
		t.Fatalf("first route = %#v, %v", out, err)
	}
	if _, out, err := server.route(t.Context(), &sdk.CallToolRequest{Session: two}, input); err != nil || out.Reason != "duplicate" {
		t.Fatalf("same durable session did not share duplicate state: %#v, %v", out, err)
	}
	server.automatic[one].run = workflow.Run{TaskID: "TSK-2", SessionID: "SES-2", RunKey: "run-2"}
	input.Goal = "choose for new workflow"
	if _, out, err := server.route(t.Context(), &sdk.CallToolRequest{Session: one}, input); err != nil || out.Status != agent.RouteStatusOK {
		t.Fatalf("new durable session inherited throttle: %#v, %v", out, err)
	}
	if runner.calls.Load() != 2 {
		t.Fatalf("provider calls = %d, want 2", runner.calls.Load())
	}
}

func TestRouteCallsAcrossTransportsForSameDurableSessionAreSerialized(t *testing.T) {
	one, two := new(sdk.ServerSession), new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Now()}
	var active, maximum atomic.Int32
	runner := &fakeRouteRunner{result: acceptedRouteResult(), run: func() {
		current := active.Add(1)
		for {
			seen := maximum.Load()
			if current <= seen || maximum.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		clock.Add(11 * time.Second)
		active.Add(-1)
	}}
	server := newRouteUnitServer(runner, &fakeRouteRecorder{}, clock, one)
	server.automatic[two] = &automaticContext{run: workflow.Run{TaskID: "TSK-1", SessionID: "SES-1", RunKey: "run-1"}}
	var wg sync.WaitGroup
	for index, session := range []*sdk.ServerSession{one, two} {
		wg.Add(1)
		go func(index int, session *sdk.ServerSession) {
			defer wg.Done()
			_, _, _ = server.route(t.Context(), &sdk.CallToolRequest{Session: session}, RouteIn{
				Goal: string(rune('a' + index)), Tools: []RouteCandidate{{ID: "rg", Description: "search"}},
			})
		}(index, session)
	}
	wg.Wait()
	if maximum.Load() != 1 || runner.calls.Load() != 2 {
		t.Fatalf("maximum concurrent=%d provider calls=%d", maximum.Load(), runner.calls.Load())
	}
}

func TestRouteStateCacheIsBounded(t *testing.T) {
	clock := &routeTestClock{now: time.Now()}
	server := &Server{clock: clock, routeSessions: make(map[string]*routeSessionState)}
	for index := range routeStateLimit + 50 {
		_, release, acquired := server.acquireRouteState(string(rune(index+1)), clock.Now())
		if acquired {
			release()
		}
	}
	if len(server.routeSessions) != routeStateLimit {
		t.Fatalf("route state cache grew to %d, want limit %d", len(server.routeSessions), routeStateLimit)
	}
	clock.Add(routeStateIdleInterval)
	_, release, acquired := server.acquireRouteState("after-idle", clock.Now())
	if !acquired {
		t.Fatal("idle route states were not evicted")
	}
	release()
}

func TestRouteFullStateCacheDoesNotAmplifyFallbackTelemetry(t *testing.T) {
	session := new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Now()}
	runner := &fakeRouteRunner{result: acceptedRouteResult()}
	recorder := &fakeRouteRecorder{}
	server := newRouteUnitServer(runner, recorder, clock, session)
	server.automatic[session].run.SessionID = "SES-unadmitted"
	for index := range routeStateLimit {
		_, release, acquired := server.acquireRouteState("SES-full-"+string(rune(index+1)), clock.Now())
		if !acquired {
			t.Fatalf("failed to fill route state slot %d", index)
		}
		release()
	}
	req := &sdk.CallToolRequest{Session: session}
	recorded := 0
	for index := range 1000 {
		_, out, err := server.route(t.Context(), req, RouteIn{
			Goal: "overflow-" + string(rune(index+1)), Tools: []RouteCandidate{{ID: "rg", Description: "search"}},
		})
		if err != nil || out.Status != agent.RouteStatusFallback || out.Reason != "rate_limited" {
			t.Fatalf("overflow route %d = %#v, %v", index, out, err)
		}
		if out.TelemetryRecorded {
			recorded++
		}
	}
	if runner.calls.Load() != 0 {
		t.Fatalf("overflow provider calls = %d", runner.calls.Load())
	}
	recorder.mu.Lock()
	writes := len(recorder.inputs)
	recorder.mu.Unlock()
	if writes != 0 || recorded != 0 {
		t.Fatalf("overflow telemetry writes = %d, recorded responses = %d; want 0/0", writes, recorded)
	}
}

func TestRouteAdmittedSessionFallbackTelemetryIsBoundedPerWindow(t *testing.T) {
	session := new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Now()}
	runner := &fakeRouteRunner{result: acceptedRouteResult()}
	recorder := &fakeRouteRecorder{}
	server := newRouteUnitServer(runner, recorder, clock, session)
	req := &sdk.CallToolRequest{Session: session}
	first := RouteIn{Goal: "first", Tools: []RouteCandidate{{ID: "rg", Description: "search"}}}
	if _, out, err := server.route(t.Context(), req, first); err != nil || out.Status != agent.RouteStatusOK {
		t.Fatalf("first route = %#v, %v", out, err)
	}
	recorded := 1
	for range 1000 {
		_, out, err := server.route(t.Context(), req, first)
		if err != nil || out.Reason != "duplicate" {
			t.Fatalf("duplicate spam = %#v, %v", out, err)
		}
		if out.TelemetryRecorded {
			recorded++
		}
	}
	for index := range 1000 {
		_, out, err := server.route(t.Context(), req, RouteIn{
			Goal: "different-" + string(rune(index+1)), Tools: first.Tools,
		})
		if err != nil || out.Reason != "rate_limited" {
			t.Fatalf("rate spam %d = %#v, %v", index, out, err)
		}
		if out.TelemetryRecorded {
			recorded++
		}
	}
	if runner.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", runner.calls.Load())
	}
	recorder.mu.Lock()
	writes := len(recorder.inputs)
	recorder.mu.Unlock()
	if writes != 3 || recorded != 3 {
		t.Fatalf("telemetry writes = %d, recorded responses = %d; want 3/3 (success + one duplicate + one rate limit)", writes, recorded)
	}
}

func TestRouteFallbackTelemetryFailureDoesNotRetryOnEveryRequest(t *testing.T) {
	session := new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Now()}
	runner := &fakeRouteRunner{result: acceptedRouteResult()}
	recorder := &fakeRouteRecorder{err: errors.New("db unavailable")}
	server := newRouteUnitServer(runner, recorder, clock, session)
	req := &sdk.CallToolRequest{Session: session}
	input := RouteIn{Goal: "same", Tools: []RouteCandidate{{ID: "rg", Description: "search"}}}
	if _, out, err := server.route(t.Context(), req, input); err != nil || out.Status != agent.RouteStatusFallback || out.Reason != "telemetry_failed" || len(out.Selections) != 0 {
		t.Fatalf("unproven advice = %#v, %v", out, err)
	}
	for range 1000 {
		if _, out, err := server.route(t.Context(), req, input); err != nil || out.Reason != "duplicate" {
			t.Fatalf("duplicate fallback = %#v, %v", out, err)
		}
	}
	recorder.mu.Lock()
	writes := len(recorder.inputs)
	recorder.mu.Unlock()
	if writes != 2 {
		t.Fatalf("failed telemetry attempts = %d, want 2 (advice proof + one duplicate attempt)", writes)
	}
}

func TestRouteThrottleBoundariesRemainExactlyTenAndThirtySeconds(t *testing.T) {
	t.Run("rate limit admits at ten seconds", func(t *testing.T) {
		session := new(sdk.ServerSession)
		clock := &routeTestClock{now: time.Now()}
		runner := &fakeRouteRunner{result: acceptedRouteResult()}
		server := newRouteUnitServer(runner, &fakeRouteRecorder{}, clock, session)
		req := &sdk.CallToolRequest{Session: session}
		if _, out, err := server.route(t.Context(), req, RouteIn{Goal: "one", Tools: []RouteCandidate{{ID: "rg", Description: "search"}}}); err != nil || out.Status != agent.RouteStatusOK {
			t.Fatalf("first = %#v, %v", out, err)
		}
		clock.Add(routeMinimumInterval)
		if _, out, err := server.route(t.Context(), req, RouteIn{Goal: "two", Tools: []RouteCandidate{{ID: "rg", Description: "search"}}}); err != nil || out.Status != agent.RouteStatusOK {
			t.Fatalf("at exact ten seconds = %#v, %v", out, err)
		}
		if runner.calls.Load() != 2 {
			t.Fatalf("provider calls = %d", runner.calls.Load())
		}
	})

	t.Run("duplicate expires at thirty seconds", func(t *testing.T) {
		session := new(sdk.ServerSession)
		clock := &routeTestClock{now: time.Now()}
		runner := &fakeRouteRunner{result: acceptedRouteResult()}
		server := newRouteUnitServer(runner, &fakeRouteRecorder{}, clock, session)
		req := &sdk.CallToolRequest{Session: session}
		input := RouteIn{Goal: "same", Tools: []RouteCandidate{{ID: "rg", Description: "search"}}}
		if _, out, err := server.route(t.Context(), req, input); err != nil || out.Status != agent.RouteStatusOK {
			t.Fatalf("first = %#v, %v", out, err)
		}
		clock.Add(routeDuplicateInterval)
		if _, out, err := server.route(t.Context(), req, input); err != nil || out.Status != agent.RouteStatusOK {
			t.Fatalf("at exact thirty seconds = %#v, %v", out, err)
		}
		if runner.calls.Load() != 2 {
			t.Fatalf("provider calls = %d", runner.calls.Load())
		}
	})
}

func TestRouteDimensionRetainsSafeFallbackEvidence(t *testing.T) {
	confidenceOnly := routeDimension("tool", RouteOut{
		Selections:      map[string]RouteSelection{"tool": {Confidence: .621, Accepted: false, Reason: "no_match"}},
		candidateCounts: map[string]int{"tool": 2}, ordinals: map[string]int{"tool": -1},
	})
	if confidenceOnly.SelectedOrdinal != nil || confidenceOnly.ConfidenceMilli == nil || *confidenceOnly.ConfidenceMilli != 621 {
		t.Fatalf("no-match evidence = %#v", confidenceOnly)
	}
	candidate := "private-id-must-not-persist"
	atomicFallback := routeDimension("tool", RouteOut{
		Selections:      map[string]RouteSelection{"tool": {Candidate: &candidate, Confidence: .554, Accepted: false, Reason: "atomic_fallback"}},
		candidateCounts: map[string]int{"tool": 3}, ordinals: map[string]int{"tool": 2},
	})
	if atomicFallback.SelectedOrdinal == nil || *atomicFallback.SelectedOrdinal != 2 || atomicFallback.ConfidenceMilli == nil || *atomicFallback.ConfidenceMilli != 554 {
		t.Fatalf("atomic fallback evidence = %#v", atomicFallback)
	}
}

func TestRouteFallbackResponseStaysUnusableWhileSafeEvidenceIsRecorded(t *testing.T) {
	session := new(sdk.ServerSession)
	clock := &routeTestClock{now: time.Now()}
	candidate := "secret-candidate-id"
	runner := &fakeRouteRunner{result: agent.RouteResult{
		Version: 1, Enabled: true, Advisory: true, Mode: "shadow",
		Status: agent.RouteStatusFallback, Reason: "atomic_fallback",
		CredentialSource: agent.RouteCredentialKeyring,
		CandidateCounts:  map[string]int{"tool": 2},
		Selections: map[string]agent.Selection{"tool": {
			Candidate: &candidate, Confidence: .55, Accepted: false, Reason: "atomic_fallback", Ordinal: 1,
		}},
	}}
	recorder := &fakeRouteRecorder{}
	server := newRouteUnitServer(runner, recorder, clock, session)
	_, out, err := server.route(t.Context(), &sdk.CallToolRequest{Session: session}, RouteIn{
		Goal: "choose", Tools: []RouteCandidate{{ID: "one", Description: "first"}, {ID: candidate, Description: "second"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := out.Selections["tool"]
	if out.Status != agent.RouteStatusFallback || selection.Accepted {
		t.Fatalf("fallback became usable: %#v", out)
	}
	if len(recorder.inputs) != 1 || recorder.inputs[0].Tool.SelectedOrdinal == nil || *recorder.inputs[0].Tool.SelectedOrdinal != 1 ||
		recorder.inputs[0].Tool.ConfidenceMilli == nil || *recorder.inputs[0].Tool.ConfidenceMilli != 550 {
		t.Fatalf("safe fallback telemetry = %#v", recorder.inputs)
	}
}

func TestPresenceStartsFromBootstrapHeartbeatsTracksToolsAndEndsOnDisconnect(t *testing.T) {
	root := t.TempDir()
	if output, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	initializer := bootstrap.New(bootstrap.Options{StartDir: root, Mode: bootstrap.ModeInit})
	if err := initializer.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := initializer.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	server, err := New(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	presence := &fakePresenceRecorder{}
	server.presence = presence
	server.presenceHeartbeatTestInterval = 2 * time.Millisecond
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.SDK().Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "claude-code", Title: "Claude Code", Version: "1.2.3"}, nil)
	connection, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := connection.CallTool(t.Context(), &sdk.CallToolParams{Name: ToolBootstrap, Arguments: map[string]any{
		"goal": "presence test", "run_key": "presence-test-run",
	}})
	if err != nil || result.IsError {
		t.Fatalf("bootstrap: %+v %v", result, err)
	}
	if _, err := connection.CallTool(t.Context(), &sdk.CallToolParams{Name: ToolStatus, Arguments: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		starts, heartbeats, activities, _ := presence.snapshot()
		if starts == 1 && heartbeats > 0 && activities > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	starts, heartbeats, activities, _ := presence.snapshot()
	if starts != 1 || heartbeats == 0 || activities == 0 {
		t.Fatalf("presence starts=%d heartbeats=%d activities=%d", starts, heartbeats, activities)
	}
	presence.mu.Lock()
	started := presence.starts[0]
	presence.mu.Unlock()
	if started.Client.Name != "claude-code" || started.Client.Title != "Claude Code" || started.Client.Version != "1.2.3" {
		t.Fatalf("self-reported client = %#v", started.Client)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		_, _, _, ends := presence.snapshot()
		if ends["RUN-1"] == agent.RuntimeEndDisconnected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	_, _, _, ends := presence.snapshot()
	t.Fatalf("disconnect end = %#v", ends)
}

func TestPresenceCompletionEndsRuntimeAndReconnectGetsUniqueRuntime(t *testing.T) {
	presence := &fakePresenceRecorder{}
	one, two := new(sdk.ServerSession), new(sdk.ServerSession)
	server := &Server{
		presence: presence, projectIDValue: "PRJ-1", workspaceIDValue: "WSP-1",
		presenceHeartbeatTestInterval: time.Hour,
		automatic: map[*sdk.ServerSession]*automaticContext{
			one: {run: workflow.Run{RunKey: "same", TaskID: "TSK-1", SessionID: "SES-1"}},
			two: {run: workflow.Run{RunKey: "same", TaskID: "TSK-1", SessionID: "SES-1"}},
		},
		routeSessions: make(map[string]*routeSessionState),
	}
	run := workflow.Run{RunKey: "same", TaskID: "TSK-1", SessionID: "SES-1"}
	server.ensurePresence(t.Context(), &sdk.CallToolRequest{Session: one}, run)
	server.ensurePresence(t.Context(), &sdk.CallToolRequest{Session: two}, run)
	starts, _, _, ends := presence.snapshot()
	if starts != 2 || len(ends) != 0 {
		t.Fatalf("reconnect starts=%d ends=%#v", starts, ends)
	}
	server.retainTerminalAutomatic(two, workflow.Run{RunKey: "same", TaskID: "TSK-1", SessionID: "SES-1", Revision: 1})
	_, _, _, ends = presence.snapshot()
	if ends["RUN-2"] != agent.RuntimeEndCompleted {
		t.Fatalf("completion end = %#v", ends)
	}
	if _, endedFirst := ends["RUN-1"]; endedFirst {
		t.Fatalf("older crashed runtime was falsely ended: %#v", ends)
	}
	server.closeAutomatic()
}

func TestPresenceStartFailureRetriesOnLaterToolActivity(t *testing.T) {
	presence := &fakePresenceRecorder{startFailures: 1}
	session := new(sdk.ServerSession)
	run := workflow.Run{RunKey: "run", TaskID: "TSK-1", SessionID: "SES-1"}
	server := &Server{presence: presence, automatic: map[*sdk.ServerSession]*automaticContext{session: {run: run}}, presenceHeartbeatTestInterval: time.Hour}
	server.ensurePresence(t.Context(), &sdk.CallToolRequest{Session: session}, run)
	if !server.touchPresence(t.Context(), session) {
		t.Fatal("later tool activity did not recreate presence")
	}
	starts, _, activities, _ := presence.snapshot()
	if starts != 2 || activities != 1 {
		t.Fatalf("starts=%d activities=%d", starts, activities)
	}
	server.closeAutomatic()
}

func TestConcurrentPresenceRecoveryStartsOnlyOneRuntime(t *testing.T) {
	presence := &fakePresenceRecorder{startEntered: make(chan struct{}), startRelease: make(chan struct{})}
	session := new(sdk.ServerSession)
	run := workflow.Run{RunKey: "run", TaskID: "TSK-1", SessionID: "SES-1"}
	server := &Server{presence: presence, automatic: map[*sdk.ServerSession]*automaticContext{session: {run: run}}, presenceHeartbeatTestInterval: time.Hour}
	done := make(chan struct{})
	go func() { server.touchPresence(t.Context(), session); close(done) }()
	<-presence.startEntered
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() { defer wg.Done(); server.touchPresence(t.Context(), session) }()
	}
	wg.Wait()
	close(presence.startRelease)
	<-done
	starts, _, _, _ := presence.snapshot()
	if starts != 1 {
		t.Fatalf("concurrent recovery starts = %d", starts)
	}
	server.closeAutomatic()
}

func TestLatePresenceStartCannotPublishAfterCompletion(t *testing.T) {
	presence := &fakePresenceRecorder{startEntered: make(chan struct{}), startRelease: make(chan struct{})}
	session := new(sdk.ServerSession)
	run := workflow.Run{RunKey: "run", TaskID: "TSK-1", SessionID: "SES-1", Revision: 1}
	server := &Server{presence: presence, automatic: map[*sdk.ServerSession]*automaticContext{session: {run: run}}, presenceHeartbeatTestInterval: time.Hour}
	done := make(chan struct{})
	go func() {
		server.ensurePresence(t.Context(), &sdk.CallToolRequest{Session: session}, run)
		close(done)
	}()
	<-presence.startEntered
	completed := run
	completed.State = "COMPLETED"
	completed.Revision++
	server.retainTerminalAutomatic(session, completed)
	close(presence.startRelease)
	<-done
	current, ok := server.automaticSnapshot(session)
	if !ok || current.presence != nil || current.presenceStarting {
		t.Fatalf("late start published after completion: %#v", current)
	}
	_, _, _, ends := presence.snapshot()
	if ends["RUN-1"] != agent.RuntimeEndReplaced {
		t.Fatalf("late runtime was not ended: %#v", ends)
	}
}

func TestPresenceEndFailureRetriesOnSubsequentTerminalTrigger(t *testing.T) {
	presence := &fakePresenceRecorder{endFailures: presenceEndAttempts}
	session := new(sdk.ServerSession)
	binding := &presenceBinding{id: "RUN-1"}
	run := workflow.Run{RunKey: "run", TaskID: "TSK-1", SessionID: "SES-1", State: "COMPLETED", Revision: 1}
	server := &Server{presence: presence, automatic: map[*sdk.ServerSession]*automaticContext{session: {run: run, presence: binding}}}
	server.retainTerminalAutomatic(session, run)
	if server.automatic[session].presence != binding {
		t.Fatal("failed terminal write discarded its retry handle")
	}
	server.touchPresence(t.Context(), session)
	if server.automatic[session].presence != nil {
		t.Fatal("later tool trigger did not clear ended binding")
	}
	presence.mu.Lock()
	endCalls := presence.endCalls
	activities := len(presence.activities)
	presence.mu.Unlock()
	if endCalls != presenceEndAttempts+1 {
		t.Fatalf("end calls = %d", endCalls)
	}
	if activities != 0 {
		t.Fatalf("terminal runtime received %d activity refreshes", activities)
	}
}

func TestConcurrentTerminalPathsEndOnlyCapturedRuntimeOnce(t *testing.T) {
	presence := &fakePresenceRecorder{}
	session := new(sdk.ServerSession)
	oldBinding := &presenceBinding{id: "RUN-old"}
	newBinding := &presenceBinding{id: "RUN-new"}
	server := &Server{presence: presence, automatic: map[*sdk.ServerSession]*automaticContext{
		session: {run: workflow.Run{RunKey: "new", SessionID: "SES-new"}, presence: newBinding},
	}}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if server.stopPresence(oldBinding, agent.RuntimeEndReplaced) {
				server.clearPresenceIfCurrent(session, oldBinding)
			}
		}()
	}
	wg.Wait()
	current, ok := server.automaticSnapshot(session)
	if !ok || current.presence != newBinding {
		t.Fatalf("old terminal path cleared replacement: %#v", current)
	}
	presence.mu.Lock()
	endCalls := presence.endCalls
	presence.mu.Unlock()
	_, _, _, ends := presence.snapshot()
	if endCalls != 1 || ends["RUN-old"] != agent.RuntimeEndReplaced {
		t.Fatalf("old runtime terminal writes=%d ends=%#v", endCalls, ends)
	}
	if _, ended := ends["RUN-new"]; ended {
		t.Fatalf("replacement runtime was ended: %#v", ends)
	}
}

func TestPresenceHeartbeatProductionIntervalIsExactlyFiveSeconds(t *testing.T) {
	if agentPresenceHeartbeatInterval != 5*time.Second {
		t.Fatalf("production presence heartbeat = %s", agentPresenceHeartbeatInterval)
	}
}
