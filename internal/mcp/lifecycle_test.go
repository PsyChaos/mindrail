package mcp_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/mcp"
	"github.com/PsyChaos/mindrail/internal/storage"
)

func coordinationStore(t *testing.T, root string) (*coordination.Store, *sql.DB) {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{
		Path: filepath.Join(root, ".git", "mindrail", "mindrail.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := app.FixedClock{Instant: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)}
	indexes := index.NewStore(db.DB, clock)
	if _, err := indexes.UpsertUnit(t.Context(), filepath.Join(root, "pkg"), index.UnitPython); err != nil {
		t.Fatal(err)
	}
	return coordination.NewStore(db.DB, clock), db.DB
}

func workspaceOf(t *testing.T, db *sql.DB) (workspaceID, projectID string) {
	t.Helper()
	if err := db.QueryRowContext(t.Context(),
		`SELECT workspace_id, project_id FROM workspaces LIMIT 1`).Scan(&workspaceID, &projectID); err != nil {
		t.Fatal(err)
	}
	return workspaceID, projectID
}

func openTaskAndSession(t *testing.T, coord *coordination.Store, workspaceID, projectID string) (taskID, session string) {
	t.Helper()
	opened, _, err := coord.OpenSession(t.Context(), workspaceID, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := coord.OpenTask(t.Context(), projectID,
		coordination.NamedSession(opened.ID), "Fix the gate")
	if err != nil {
		t.Fatal(err)
	}
	return task.ID, opened.ID
}

// TestClaimLifecycle is TASK-01 AC-01.1: claim transitions to CLAIMED with
// revision, unknown identities refuse before writes, and revision mismatch
// conflicts loudly.
func TestClaimLifecycle(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, session := openTaskAndSession(t, coord, workspaceID, projectID)

	claimed := callTool(t, server, "probe", mcp.ToolClaim, map[string]any{
		"task_id": taskID, "session": session,
	})
	if claimed["state"] != "CLAIMED" {
		t.Fatalf("claim = %+v", claimed)
	}
	revision, ok := claimed["revision"].(float64)
	if !ok || revision < 1 {
		t.Fatalf("claim = %+v, want a revision", claimed)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolClaim, map[string]any{
		"task_id": "", "session": session,
	}); err == nil || !strings.Contains(err.Error(), "needs a task") {
		t.Fatalf("empty task did not refuse at the tool: %v", err)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolClaim, map[string]any{
		"task_id": taskID, "session": "SES-NOPE",
	}); err == nil {
		t.Fatal("unknown session accepted")
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolClaim, map[string]any{
		"task_id": "TSK-NOPE", "session": session,
	}); err == nil {
		t.Fatal("unknown task accepted")
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolClaim, map[string]any{
		"task_id": taskID, "session": session, "expected_revision": revision + 100,
	}); err == nil {
		t.Fatal("stale revision accepted")
	}
	// Release back to OPEN so the next stale claim fails on revision, not
	// on transition shape: the conflict path must prove itself distinctly.
	if _, _, err := coord.Transition(t.Context(), taskID,
		coordination.NamedSession(session), coordination.StateOpen, "release"); err != nil {
		t.Fatal(err)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolClaim, map[string]any{
		"task_id": taskID, "session": session, "expected_revision": revision,
	}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "revision") {
		t.Fatalf("stale revision did not conflict on revision: %v", err)
	}
	second, _, err := coord.OpenTask(t.Context(), projectID,
		coordination.NamedSession(session), "Second work")
	if err != nil {
		t.Fatal(err)
	}
	guarded := callTool(t, server, "probe", mcp.ToolClaim, map[string]any{
		"task_id": second.ID, "session": session, "expected_revision": second.Revision,
	})
	if guarded["state"] != "CLAIMED" {
		t.Fatalf("guarded claim = %+v", guarded)
	}
}

// TestBeforeChangeDeclaresScope is TASK-01 AC-01.2: baseline summary with
// scope, claimless declaration, and malformed refusals.
func TestBeforeChangeDeclaresScope(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
	abs := filepath.Join(root, "pkg", "a.py")

	declared := callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{
		"task_id": taskID, "paths": []string{abs},
	})
	if declared["task_id"] != taskID {
		t.Fatalf("declare = %+v", declared)
	}
	if files, ok := declared["files"].(float64); !ok || files != 1 {
		t.Fatalf("declare = %+v, want 1 file", declared)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolBeforeChange, map[string]any{
		"task_id": "", "paths": []string{abs},
	}); err == nil {
		t.Fatal("empty task accepted")
	}
}

// TestAfterChangeRecordsFindings is TASK-01 AC-01.3: the open change plus
// attribution posture with pending and remedies; same operation id replays
// by identity without duplicating.
func TestAfterChangeRecordsFindings(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, session := openTaskAndSession(t, coord, workspaceID, projectID)
	abs := filepath.Join(root, "pkg", "a.py")
	callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{
		"task_id": taskID, "paths": []string{abs},
	})

	first := callTool(t, server, "probe", mcp.ToolAfterChange, map[string]any{
		"task_id": taskID, "operation_id": "OP-MCP-1",
	})
	changeID, ok := first["change_id"].(string)
	if !ok || changeID == "" {
		t.Fatalf("after = %+v, want a change", first)
	}
	// Overlap a second task on the same file, then edit: the discovered
	// change ambiguates, so the answer carries pending with remedies.
	// B's rows are seeded row-for-row with A's lineage — the way an
	// independent discovery of identical content would — because indexing
	// consumes: whoever indexes first leaves nothing for a second real
	// discovery on a shared database (MR-007 twin-fixture lesson).
	other, _, err := coord.OpenTask(t.Context(), projectID,
		coordination.NamedSession(session), "Twin work")
	if err != nil {
		t.Fatal(err)
	}
	callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{
		"task_id": other.ID, "paths": []string{abs},
	})
	if err := os.WriteFile(abs, []byte("def helper():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	callTool(t, server, "probe", mcp.ToolAfterChange, map[string]any{
		"task_id": taskID, "operation_id": "OP-MCP-2",
	})
	changeStore, err := changes.NewStore(db, app.FixedClock{})
	if err != nil {
		t.Fatal(err)
	}
	aSymbols, err := changeStore.ReadChangeSymbols(t.Context(), changeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aSymbols) == 0 {
		t.Fatal("discovery found no symbols")
	}
	changeB, err := changeStore.EnsureOpenChange(t.Context(), other.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := changeStore.UpsertFileRows(t.Context(), changeB.ID, []changes.FileChange{
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
	if err := changeStore.UpsertSymbolRows(t.Context(), changeB.ID, lineage); err != nil {
		t.Fatal(err)
	}
	ambiguous := callTool(t, server, "probe", mcp.ToolAfterChange, map[string]any{
		"task_id": taskID, "operation_id": "OP-MCP-3",
	})
	if pending, ok := ambiguous["pending"].(bool); !ok || !pending {
		t.Fatalf("ambiguous after = %+v, want pending", ambiguous)
	}
	findings, ok := ambiguous["findings"].([]any)
	if !ok || len(findings) == 0 {
		t.Fatalf("ambiguous after = %+v, want findings", ambiguous)
	}
	firstFinding, ok := findings[0].(map[string]any)
	if !ok || firstFinding["code"] == "" || firstFinding["next_action"] == nil {
		t.Fatalf("finding = %+v, want code + next_action", findings[0])
	}
	second := callTool(t, server, "probe", mcp.ToolAfterChange, map[string]any{
		"task_id": taskID, "operation_id": "OP-MCP-1",
	})
	if second["change_id"] != changeID {
		t.Fatalf("replay changed identity: %+v vs %+v", first, second)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM changes WHERE task_id = ?`, taskID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("changes = %d, want 1 row", rows)
	}
}
