package mcp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/mcp"
)

func gitCommitFile(t *testing.T, root, rel string) {
	t.Helper()
	for _, args := range [][]string{
		{"-C", root, "config", "user.email", "t@t"},
		{"-C", root, "config", "user.name", "t"},
		{"-C", root, "add", rel},
		{"-C", root, "commit", "--quiet", "-m", "base"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

// TestReconcileDiscoversUndeclared is TASK-02 AC-02.1: an edit nobody
// declared reconciles into the task's change with findings, and the same
// operation id replays it.
func TestReconcileDiscoversUndeclared(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
	abs := filepath.Join(root, "pkg", "a.py")
	gitCommitFile(t, root, "pkg/a.py")
	if err := os.WriteFile(abs, []byte("def helper():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first := callTool(t, server, "probe", mcp.ToolReconcile, map[string]any{
		"task_id": taskID, "operation_id": "OP-REC-1",
	})
	changeID, ok := first["change_id"].(string)
	if !ok || changeID == "" {
		t.Fatalf("reconcile = %+v, want a change", first)
	}
	if files, ok := first["files"].([]any); !ok || len(files) == 0 {
		t.Fatalf("reconcile = %+v, want discovered files", first)
	}
	second := callTool(t, server, "probe", mcp.ToolReconcile, map[string]any{
		"task_id": taskID, "operation_id": "OP-REC-1",
	})
	if second["change_id"] != changeID {
		t.Fatalf("replay changed identity: %+v vs %+v", first, second)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolReconcile, map[string]any{
		"task_id": "",
	}); err == nil || !strings.Contains(err.Error(), "reconcile needs a task") {
		t.Fatalf("empty task did not refuse at the tool: %v", err)
	}
}

// TestCheckpointCrossSessionRead is TASK-02 AC-02.2: a note written under
// one session reads back under another; note-absent reads, note-present
// writes; unknown sessions refuse.
func TestCheckpointCrossSessionRead(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, first := openTaskAndSession(t, coord, workspaceID, projectID)
	opened, _, err := coord.OpenSession(t.Context(), workspaceID, "agent-2")
	if err != nil {
		t.Fatal(err)
	}

	written := callTool(t, server, "probe", mcp.ToolCheckpoint, map[string]any{
		"task_id": taskID, "session": first, "note": "tests green, handing over", "handoff": true,
	})
	if written["note"] != "tests green, handing over" {
		t.Fatalf("write = %+v", written)
	}
	read := callTool(t, server, "probe", mcp.ToolCheckpoint, map[string]any{
		"task_id": taskID, "session": opened.ID,
	})
	if read["note"] != "tests green, handing over" {
		t.Fatalf("read = %+v, want the handover", read)
	}
	readable, ok := read["readable_by"].(string)
	if !ok || readable == "" {
		t.Fatalf("read = %+v, want named readability", read)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolCheckpoint, map[string]any{
		"task_id": taskID, "session": "SES-NOPE", "note": "x",
	}); err == nil {
		t.Fatal("unknown session accepted")
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolCheckpoint, map[string]any{
		"task_id": "TSK-NOPE", "session": opened.ID,
	}); err == nil {
		t.Fatal("read on unknown task accepted")
	}
}

// TestLifecycleAcrossIdentities is TASK-02 AC-02.3: the full flow across
// two client identities — claim, declare, edit, after, undeclared
// reconcile by the second identity, checkpoint, cross-session read — with
// the registry holding eleven tools and no new codes.
func TestLifecycleAcrossIdentities(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, first := openTaskAndSession(t, coord, workspaceID, projectID)
	second, _, err := coord.OpenSession(t.Context(), workspaceID, "agent-2")
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(root, "pkg", "a.py")
	gitCommitFile(t, root, "pkg/a.py")

	claimed := callTool(t, server, "agent-1", mcp.ToolClaim, map[string]any{
		"task_id": taskID, "session": first,
	})
	if claimed["state"] != "CLAIMED" {
		t.Fatalf("claim = %+v", claimed)
	}
	callTool(t, server, "agent-1", mcp.ToolBeforeChange, map[string]any{
		"task_id": taskID, "paths": []string{abs},
	})
	if err := os.WriteFile(abs, []byte("def helper():\n    return 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := callTool(t, server, "agent-1", mcp.ToolAfterChange, map[string]any{"task_id": taskID})
	if after["change_id"] == "" {
		t.Fatalf("after = %+v", after)
	}
	rediscovered := callTool(t, server, "agent-2", mcp.ToolReconcile, map[string]any{"task_id": taskID})
	if rediscovered["change_id"] != after["change_id"] {
		t.Fatalf("reconcile diverged: %+v vs %+v", rediscovered, after)
	}
	callTool(t, server, "agent-1", mcp.ToolCheckpoint, map[string]any{
		"task_id": taskID, "session": first, "note": "done", "handoff": true,
	})
	handover := callTool(t, server, "agent-2", mcp.ToolCheckpoint, map[string]any{
		"task_id": taskID, "session": second.ID,
	})
	if handover["note"] != "done" {
		t.Fatalf("handover = %+v", handover)
	}
	if names := listToolNames(t, server, "agent-1"); len(names) != 11 {
		t.Fatalf("tools = %d, want 11", len(names))
	}
}
