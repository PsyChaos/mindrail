package mcp

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/workflow"
)

func TestAutomaticContextUsesServerSessionIdentity(t *testing.T) {
	one := new(sdk.ServerSession)
	two := new(sdk.ServerSession)
	heartbeat := &countingHeartbeat{}
	server := &Server{automatic: map[*sdk.ServerSession]*automaticContext{
		one: {run: workflow.Run{RunKey: "one", TaskID: "task-one"}, heartbeat: heartbeat},
	}}

	run, gotHeartbeat, err := server.automaticRun(t.Context(), &sdk.CallToolRequest{Session: one}, "")
	if err != nil || run.TaskID != "task-one" || gotHeartbeat != heartbeat {
		t.Fatalf("first connection = %+v, %v", run, err)
	}
	if _, _, err := server.automaticRun(t.Context(), &sdk.CallToolRequest{Session: two}, ""); err == nil {
		t.Fatal("distinct *ServerSession inherited another connection's context")
	}
}

func TestAutomaticCleanupStopsEveryHeartbeat(t *testing.T) {
	one := new(sdk.ServerSession)
	two := new(sdk.ServerSession)
	first := &countingHeartbeat{}
	second := &countingHeartbeat{}
	server := &Server{automatic: map[*sdk.ServerSession]*automaticContext{
		one: {heartbeat: first},
		two: {heartbeat: second},
	}}

	server.forgetAutomatic(one)
	if first.stops.Load() != 1 || len(server.automatic) != 1 {
		t.Fatalf("single cleanup: stops=%d contexts=%d", first.stops.Load(), len(server.automatic))
	}
	server.closeAutomatic()
	if second.stops.Load() != 1 || len(server.automatic) != 0 {
		t.Fatalf("server cleanup: stops=%d contexts=%d", second.stops.Load(), len(server.automatic))
	}
}

func TestAutomaticDisconnectStopsHeartbeat(t *testing.T) {
	implementation := sdk.NewServer(&sdk.Implementation{Name: "cleanup-test", Version: "v0"}, nil)
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := implementation.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "cleanup-client", Version: "v0"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	heartbeat := &countingHeartbeat{}
	server := &Server{automatic: make(map[*sdk.ServerSession]*automaticContext)}
	server.rememberAutomatic(serverSession, automaticContext{heartbeat: heartbeat})
	if err := clientSession.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		server.autoMu.RLock()
		remaining := len(server.automatic)
		server.autoMu.RUnlock()
		if remaining == 0 && heartbeat.stops.Load() == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	server.autoMu.RLock()
	remaining := len(server.automatic)
	server.autoMu.RUnlock()
	t.Fatalf("disconnect cleanup: stops=%d contexts=%d", heartbeat.stops.Load(), remaining)
}

type countingHeartbeat struct {
	stops atomic.Int32
	err   error
}

func (h *countingHeartbeat) Stop()         { h.stops.Add(1) }
func (h *countingHeartbeat) Health() error { return h.err }

func TestAutomaticHeartbeatFailureIsVisible(t *testing.T) {
	session := new(sdk.ServerSession)
	want := errors.New("renewal lost")
	server := &Server{automatic: map[*sdk.ServerSession]*automaticContext{
		session: {run: workflow.Run{RunKey: "failed"}, heartbeat: &countingHeartbeat{err: want}},
	}}
	_, heartbeat, err := server.automaticRun(t.Context(), &sdk.CallToolRequest{Session: session}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(heartbeat.Health(), want) {
		t.Fatalf("health = %v, want %v", heartbeat.Health(), want)
	}
	finalize := true
	if _, _, err := server.complete(t.Context(), &sdk.CallToolRequest{Session: session}, CompleteIn{Finalize: &finalize}); !errors.Is(err, want) {
		t.Fatalf("complete error = %v, want renewal failure", err)
	}
}

func TestAutomaticFinalizeErrorNeverReportsCompletion(t *testing.T) {
	session := new(sdk.ServerSession)
	server := &Server{
		workflow: failingFinalizeWorkflow{},
		automatic: map[*sdk.ServerSession]*automaticContext{
			session: {
				run:       workflow.Run{RunKey: "release-error"},
				heartbeat: &countingHeartbeat{},
			},
		},
	}
	finalize := true
	_, out, err := server.complete(t.Context(), &sdk.CallToolRequest{Session: session}, CompleteIn{Finalize: &finalize})
	if err == nil || !strings.Contains(err.Error(), "release failed") {
		t.Fatalf("complete error = %v", err)
	}
	if (out.Completed != nil && *out.Completed) || out.Allow {
		t.Fatalf("release error reported completion: %#v", out)
	}
}

type failingFinalizeWorkflow struct{ workflowPort }

func (failingFinalizeWorkflow) Finalize(context.Context, workflow.FinalizeInput) (workflow.Finalization, error) {
	return workflow.Finalization{Completed: true}, errors.New("release failed")
}

func TestAutomaticDelayedDuplicateFinalizeCannotReplaceNewBootstrap(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	application := bootstrap.New(bootstrap.Options{StartDir: root, Mode: bootstrap.ModeInit})
	if err := application.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := application.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	server, err := New(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	session, err := server.SDK().Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "rollover", Version: "v0"}, nil)
	connection, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	delayed := &delayedFinalizeWorkflow{workflowPort: server.workflow, entered: make(chan struct{}), release: make(chan struct{}), heartbeats: map[string]*countingHeartbeat{}}
	server.workflow = delayed
	var release sync.Once
	unblock := func() { release.Do(func() { close(delayed.release) }) }
	defer unblock()
	req := &sdk.CallToolRequest{Session: session}
	goal, firstKey, nextKey := "finish clean work", "first", "next"
	if out, err := server.automaticStart(t.Context(), req, BootstrapIn{Goal: &goal, RunKey: &firstKey}); err != nil || !out.Started {
		t.Fatalf("first bootstrap: %+v %v", out, err)
	}
	finalize := true
	result := make(chan error, 1)
	go func() {
		_, _, err := server.complete(t.Context(), req, CompleteIn{Finalize: &finalize})
		result <- err
	}()
	select {
	case <-delayed.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first finalize did not reach delayed reply")
	}
	if _, out, err := server.complete(t.Context(), req, CompleteIn{Finalize: &finalize}); err != nil || out.Completed == nil || !*out.Completed {
		t.Fatalf("duplicate finalize: %+v %v", out, err)
	}
	if out, err := server.automaticStart(t.Context(), req, BootstrapIn{Goal: &goal, RunKey: &nextKey}); err != nil || !out.Started {
		t.Fatalf("next bootstrap: %+v %v", out, err)
	}
	unblock()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	current, found := server.automaticSnapshot(session)
	if !found || current.run.RunKey != nextKey || current.heartbeat != delayed.heartbeats[nextKey] || delayed.heartbeats[nextKey].stops.Load() != 0 {
		t.Fatalf("old finalize replaced new run: current=%+v next heartbeat stops=%d", current.run, delayed.heartbeats[nextKey].stops.Load())
	}
}

type delayedFinalizeWorkflow struct {
	workflowPort
	entered, release chan struct{}
	calls            atomic.Int32
	heartbeats       map[string]*countingHeartbeat
}

func (f *delayedFinalizeWorkflow) Finalize(ctx context.Context, input workflow.FinalizeInput) (workflow.Finalization, error) {
	result, err := f.workflowPort.Finalize(ctx, input)
	if f.calls.Add(1) == 1 {
		close(f.entered)
		select {
		case <-f.release:
		case <-ctx.Done():
			return result, ctx.Err()
		}
	}
	return result, err
}

func (f *delayedFinalizeWorkflow) StartHeartbeat(_ context.Context, key string, _ time.Duration) (automaticHeartbeat, error) {
	heartbeat := &countingHeartbeat{}
	f.heartbeats[key] = heartbeat
	return heartbeat, nil
}

func TestAutomaticLateScopeResultCannotReplaceNewRun(t *testing.T) {
	session := new(sdk.ServerSession)
	heartbeat := &countingHeartbeat{}
	server := &Server{automatic: map[*sdk.ServerSession]*automaticContext{
		session: {run: workflow.Run{RunKey: "next", State: coordination.StateInProgress}, heartbeat: heartbeat},
	}}
	server.updateAutomatic(session, workflow.Run{RunKey: "previous", State: coordination.StateInProgress})
	current, found := server.automaticSnapshot(session)
	if !found || current.run.RunKey != "next" || current.heartbeat != heartbeat || heartbeat.stops.Load() != 0 {
		t.Fatalf("old scope result replaced new run: %+v", current)
	}
}

func TestAutomaticLateScopeResultCannotRewindCompletedRun(t *testing.T) {
	session := new(sdk.ServerSession)
	server := &Server{automatic: map[*sdk.ServerSession]*automaticContext{
		session: {run: workflow.Run{RunKey: "same", State: coordination.StateCompleted, Revision: 5}},
	}}
	server.updateAutomatic(session, workflow.Run{RunKey: "same", State: coordination.StateInProgress, Revision: 3})
	current, found := server.automaticSnapshot(session)
	if !found || current.run.State != coordination.StateCompleted || current.run.Revision != 5 {
		t.Fatalf("late scope reply rewound completed run: %+v", current.run)
	}
}

func TestAutomaticDelayedBootstrapCannotRewindCompletedRun(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	application := bootstrap.New(bootstrap.Options{StartDir: root, Mode: bootstrap.ModeInit})
	if err := application.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := application.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	server, err := New(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	session, err := server.SDK().Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "bootstrap-rollover", Version: "v0"}, nil)
	connection, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	delayed := &delayedBootstrapWorkflow{workflowPort: server.workflow, entered: make(chan struct{}), release: make(chan struct{})}
	server.workflow = delayed
	var release sync.Once
	unblock := func() { release.Do(func() { close(delayed.release) }) }
	defer unblock()
	req := &sdk.CallToolRequest{Session: session}
	goal, key := "finish clean work", "first"
	input := BootstrapIn{Goal: &goal, RunKey: &key}
	if out, err := server.automaticStart(t.Context(), req, input); err != nil || !out.Started {
		t.Fatalf("bootstrap: %+v %v", out, err)
	}
	type result struct {
		out BootstrapOut
		err error
	}
	response := make(chan result, 1)
	go func() {
		out, err := server.automaticStart(t.Context(), req, input)
		response <- result{out, err}
	}()
	select {
	case <-delayed.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("duplicate bootstrap did not reach delayed reply")
	}
	finalize := true
	if _, out, err := server.complete(t.Context(), req, CompleteIn{Finalize: &finalize}); err != nil || out.Completed == nil || !*out.Completed {
		t.Fatalf("finalize: %+v %v", out, err)
	}
	unblock()
	got := <-response
	if got.err != nil || got.out.State != string(coordination.StateCompleted) {
		t.Fatalf("late bootstrap reported stale run: %+v %v", got.out, got.err)
	}
	current, _ := server.automaticSnapshot(session)
	if current.run.State != coordination.StateCompleted || current.heartbeat != nil {
		t.Fatalf("late bootstrap rewound completed run or renewed heartbeat: %+v", current)
	}
	next := "next"
	if out, err := server.automaticStart(t.Context(), req, BootstrapIn{Goal: &goal, RunKey: &next}); err != nil || !out.Started {
		t.Fatalf("next bootstrap blocked by old reply: %+v %v", out, err)
	}
}

type delayedBootstrapWorkflow struct {
	workflowPort
	entered, release chan struct{}
	calls            atomic.Int32
}

func (f *delayedBootstrapWorkflow) Start(ctx context.Context, input workflow.StartInput) (workflow.Run, error) {
	run, err := f.workflowPort.Start(ctx, input)
	if f.calls.Add(1) == 2 {
		close(f.entered)
		select {
		case <-f.release:
		case <-ctx.Done():
			return run, ctx.Err()
		}
	}
	return run, err
}
