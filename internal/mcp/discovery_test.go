package mcp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/coordination"
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
	// A twin declares the same file: agent-1's after_change carries
	// pending with remedies, pinning state visibility in the lifecycle.
	twin, _, err := coord.OpenTask(t.Context(), projectID,
		coordination.NamedSession(second.ID), "Twin work")
	if err != nil {
		t.Fatal(err)
	}
	callTool(t, server, "agent-2", mcp.ToolBeforeChange, map[string]any{
		"task_id": twin.ID, "paths": []string{abs},
	})
	if err := os.WriteFile(abs, []byte("def helper():\n    return 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := callTool(t, server, "agent-1", mcp.ToolAfterChange, map[string]any{"task_id": taskID})
	if after["change_id"] == "" {
		t.Fatalf("after = %+v", after)
	}
	// Seed the twin's rows from A's lineage (real twin discovery would
	// consume the shared index — MR-007 lesson): agent-1's next answer
	// carries pending with remedies, pinning state visibility.
	changeStore, err := changes.NewStore(db, app.FixedClock{})
	if err != nil {
		t.Fatal(err)
	}
	changeID, _ := after["change_id"].(string)
	aSymbols, err := changeStore.ReadChangeSymbols(t.Context(), changeID)
	if err != nil {
		t.Fatal(err)
	}
	changeTwin, err := changeStore.EnsureOpenChange(t.Context(), twin.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := changeStore.UpsertFileRows(t.Context(), changeTwin.ID, []changes.FileChange{
		{Path: abs, Kind: changes.FileModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
	var lineage []changes.SymbolChange
	for _, symbol := range aSymbols {
		lineage = append(lineage, changes.SymbolChange{
			Key: symbol.Key, UID: symbol.UID, Kind: symbol.Kind,
			Body: symbol.Body, Signature: symbol.Signature, Structure: symbol.Structure,
			SigHash: symbol.SigHash, BodyHash: symbol.BodyHash, StructHash: symbol.StructHash,
			Via: changes.ViaReconcile,
		})
	}
	if err := changeStore.UpsertSymbolRows(t.Context(), changeTwin.ID, lineage); err != nil {
		t.Fatal(err)
	}
	pending := callTool(t, server, "agent-1", mcp.ToolAfterChange, map[string]any{"task_id": taskID})
	if ok, _ := pending["pending"].(bool); !ok {
		t.Fatalf("after = %+v, want pending with twin overlap", pending)
	}
	findings, ok := pending["findings"].([]any)
	if !ok || len(findings) == 0 {
		t.Fatalf("pending = %+v, want findings", pending)
	}
	blocked, ok := findings[0].(map[string]any)
	if !ok || blocked["code"] == "" || blocked["next_action"] == nil {
		t.Fatalf("finding = %+v, want code + next_action", findings[0])
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
	if names := listToolNames(t, server, "agent-1"); len(names) != 13 {
		t.Fatalf("tools = %d, want 13", len(names))
	}
}
