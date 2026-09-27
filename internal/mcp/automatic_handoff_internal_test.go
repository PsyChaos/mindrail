package mcp

import (
	"context"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/workflow"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type delayedCheckpointWorkflow struct {
	workflowPort
	entered, release chan struct{}
	calls            atomic.Int32
}

func (w *delayedCheckpointWorkflow) Checkpoint(ctx context.Context, key, note string, handoff bool) (coordination.Noted, error) {
	out, err := w.workflowPort.Checkpoint(ctx, key, note, handoff)
	if w.calls.Add(1) == 1 {
		close(w.entered)
		select {
		case <-w.release:
		case <-ctx.Done():
			return out, ctx.Err()
		}
	}
	return out, err
}

func TestAutomaticDelayedHandoffDoesNotForgetNextRun(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	a := bootstrap.New(bootstrap.Options{StartDir: root, Mode: bootstrap.ModeInit})
	if err := a.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := a.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	s, err := New(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	ct, st := sdk.NewInMemoryTransports()
	ss, err := s.SDK().Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	c := sdk.NewClient(&sdk.Implementation{Name: "review", Version: "v0"}, nil)
	cs, err := c.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	w := &delayedCheckpointWorkflow{workflowPort: s.workflow, entered: make(chan struct{}), release: make(chan struct{})}
	s.workflow = w
	var once sync.Once
	unblock := func() { once.Do(func() { close(w.release) }) }
	defer unblock()
	req := &sdk.CallToolRequest{Session: ss}
	goal, oldKey, nextKey := "test handoff", "old", "next"
	if out, err := s.automaticStart(t.Context(), req, BootstrapIn{Goal: &goal, RunKey: &oldKey}); err != nil || !out.Started {
		t.Fatalf("old bootstrap: %+v %v", out, err)
	}
	note := "continue later"
	done := make(chan error, 1)
	go func() {
		_, _, err := s.checkpoint(t.Context(), req, CheckpointIn{Note: &note, Handoff: true})
		done <- err
	}()
	select {
	case <-w.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("checkpoint not delayed")
	}
	if _, _, err := s.checkpoint(t.Context(), req, CheckpointIn{Note: &note, Handoff: true}); err != nil {
		t.Fatalf("duplicate checkpoint: %v", err)
	}
	if out, err := s.automaticStart(t.Context(), req, BootstrapIn{Goal: &goal, RunKey: &nextKey}); err != nil || !out.Started {
		t.Fatalf("next bootstrap: %+v %v", out, err)
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	current, ok := s.automaticSnapshot(ss)
	if !ok || current.run.RunKey != nextKey {
		t.Fatalf("delayed handoff discarded the next run and stopped its heartbeat: present=%v run=%+v", ok, current.run)
	}
}

func TestAutomaticHandoffCleanupKeepsNewerSameRunRevision(t *testing.T) {
	session := new(sdk.ServerSession)
	heartbeat := &countingHeartbeat{}
	server := &Server{automatic: map[*sdk.ServerSession]*automaticContext{
		session: {run: workflow.Run{RunKey: "same", Revision: 4}, heartbeat: heartbeat},
	}}
	server.forgetAutomaticIfCurrent(session, workflow.Run{RunKey: "same", Revision: 3})
	current, present := server.automaticSnapshot(session)
	if !present || current.run.Revision != 4 || heartbeat.stops.Load() != 0 {
		t.Fatalf("stale handoff removed newer state: present=%v current=%+v stops=%d", present, current.run, heartbeat.stops.Load())
	}
	server.forgetAutomaticIfCurrent(session, workflow.Run{RunKey: "same", Revision: 4})
	if _, present := server.automaticSnapshot(session); present || heartbeat.stops.Load() != 1 {
		t.Fatalf("current handoff failed to release binding: present=%v stops=%d", present, heartbeat.stops.Load())
	}
}
