package mcp_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/mcp"
)

func denialCodes(t *testing.T, out map[string]any) []string {
	t.Helper()
	denials, ok := out["denials"].([]any)
	if !ok {
		t.Fatalf("denials = %+v", out)
	}
	var codes []string
	for _, denial := range denials {
		item, ok := denial.(map[string]any)
		if !ok {
			t.Fatalf("denial = %+v", denial)
		}
		code, _ := item["code"].(string)
		reason, _ := item["reason"].(string)
		key, _ := item["key"].(string)
		next, _ := item["next_action"].([]any)
		if code == "" || reason == "" || key == "" || len(next) == 0 {
			t.Fatalf("denial unexplained: %+v", item)
		}
		codes = append(codes, code)
	}
	return codes
}

// TestCompleteAllowsClean is TASK-02 AC-02.1: nothing discovered, nothing
// required — ALLOW with empty denials.
func TestCompleteAllowsClean(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)

	out := callTool(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID})
	if allow, ok := out["allow"].(bool); !ok || !allow {
		t.Fatalf("complete = %+v, want ALLOW", out)
	}
	if denials, ok := out["denials"].([]any); !ok || len(denials) != 0 {
		t.Fatalf("complete = %+v, want no denials", out)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": ""}); err == nil {
		t.Fatal("empty task accepted")
	}
}

// TestCompleteDeniesAttribution is TASK-02 AC-02.1: overlapping baselines
// ambiguate through the tool with the gate's code.
func TestCompleteDeniesAttribution(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, session := openTaskAndSession(t, coord, workspaceID, projectID)
	abs := filepath.Join(root, "pkg", "a.py")
	twin, _, err := coord.OpenTask(t.Context(), projectID,
		coordination.NamedSession(session), "Twin work")
	if err != nil {
		t.Fatal(err)
	}
	callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{
		"task_id": taskID, "paths": []string{abs},
	})
	callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{
		"task_id": twin.ID, "paths": []string{abs},
	})
	if err := os.WriteFile(abs, []byte("def helper():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	callTool(t, server, "probe", mcp.ToolAfterChange, map[string]any{"task_id": taskID})
	seedTwinRows(t, db, twin.ID, abs, taskID)

	out := callTool(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID})
	if allow, ok := out["allow"].(bool); !ok || allow {
		t.Fatalf("complete = %+v, want DENY", out)
	}
	codes := denialCodes(t, out)
	if len(codes) != 1 || codes[0] != "RECONCILE_AMBIGUOUS" {
		t.Fatalf("codes = %+v", codes)
	}
}

// seedTwinRows mirrors a task's change rows onto a twin change: the shared
// index consumes real twin discovery (MR-007 lesson), so the twin rides
// the same lineage by seeding.
func seedTwinRows(t *testing.T, db *sql.DB, twinID, path, taskID string) {
	t.Helper()
	twinStore, err := changes.NewStore(db, app.FixedClock{})
	if err != nil {
		t.Fatal(err)
	}
	var changeID string
	if err := db.QueryRowContext(t.Context(),
		`SELECT change_id FROM changes WHERE task_id = ?`, taskID).Scan(&changeID); err != nil {
		t.Fatal(err)
	}
	rows, err := twinStore.ReadChangeSymbols(t.Context(), changeID)
	if err != nil {
		t.Fatal(err)
	}
	twinChange, err := twinStore.EnsureOpenChange(t.Context(), twinID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := twinStore.UpsertFileRows(t.Context(), twinChange.ID, []changes.FileChange{
		{Path: path, Kind: changes.FileModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
	var lineage []changes.SymbolChange
	for _, symbol := range rows {
		lineage = append(lineage, changes.SymbolChange{
			Key: symbol.Key, UID: symbol.UID, Kind: symbol.Kind,
			Body: symbol.Body, Signature: symbol.Signature, Structure: symbol.Structure,
			SigHash: symbol.SigHash, BodyHash: symbol.BodyHash, StructHash: symbol.StructHash,
			Via: changes.ViaReconcile,
		})
	}
	if err := twinStore.UpsertSymbolRows(t.Context(), twinChange.ID, lineage); err != nil {
		t.Fatal(err)
	}
}

// TestCompleteDeniesStaleEvidence is TASK-02 AC-02.2: stale evidence denies
// through the tool with exactly the gate unit's code and profile remedy.
func TestCompleteDeniesStaleEvidence(t *testing.T) {
	root := newProfileRepo(t)
	gitCommitFile(t, root, ".")
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
	callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{
		"task_id": taskID, "paths": []string{filepath.Join(root, "tests", "t.py")},
	})

	ran := callTool(t, server, "probe", mcp.ToolValidate, map[string]any{"profile": "test"})
	if _, ok := ran["evidence"]; !ok {
		t.Fatalf("validate = %+v", ran)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "t.py"), []byte("print(2)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := callTool(t, server, "probe", mcp.ToolComplete, map[string]any{
		"task_id": taskID, "required": []string{"test"},
	})
	if allow, ok := out["allow"].(bool); !ok || allow {
		t.Fatalf("complete = %+v, want DENY", out)
	}
	codes := denialCodes(t, out)
	if len(codes) != 1 || codes[0] != "REQUIRED_EVIDENCE_NOT_CURRENT" {
		t.Fatalf("codes = %+v, want gate parity", codes)
	}
}

// TestCompleteDeniesOrphaned is TASK-02 AC-02.1: an orphaned binding under
// an active CRITICAL invariant denies through the tool with the gate code.
func TestCompleteDeniesOrphaned(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
	abs := filepath.Join(root, "pkg", "a.py")
	registerUnit(t, db, filepath.Join(root, "pkg"))
	seedIdentity(t, db, "SYM-O-1", "orphan-key")
	seedSymbolRow(t, db, abs, "orphan-key", "orphan", "SYM-O-1")
	seedBinding(t, db, "INV-0002", "SYM-O-1", "orphaned")
	writeInvariantFile(t, root, "INV-0002", "CRITICAL")
	callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{
		"task_id": taskID, "paths": []string{abs},
	})
	seedChangeRows(t, db, taskID, abs, "orphan-key", "SYM-O-1")

	out := callTool(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID})
	if allow, ok := out["allow"].(bool); !ok || allow {
		t.Fatalf("complete = %+v, want DENY", out)
	}
	codes := denialCodes(t, out)
	found := false
	for _, code := range codes {
		if code == "ORPHANED_PROTECTED_SYMBOL" {
			found = true
		}
	}
	if !found {
		t.Fatalf("codes = %+v, want orphaned", codes)
	}
}

// TestCompleteDeniesWeakenedGuard is TASK-02 AC-02.1: a removed CRITICAL
// verification test denies through the tool with the guard code — composed
// from git deltas and reference-derived mapping, nothing hand-fed.
func TestCompleteDeniesWeakenedGuard(t *testing.T) {
	testCompleteDeniesWeakenedGuard(t, "seed")
}

// Discovery must also retain the old proof mapping when the caller never
// recorded an after_change and completion is the first discovery path.
func TestCompleteDeniesWeakenedGuardWithoutAfterChange(t *testing.T) {
	testCompleteDeniesWeakenedGuard(t, "")
}

func TestCompleteDeniesWeakenedGuardAfterPublicDiscovery(t *testing.T) {
	for _, tool := range []string{mcp.ToolAfterChange, mcp.ToolReconcile} {
		t.Run(tool, func(t *testing.T) { testCompleteDeniesWeakenedGuard(t, tool) })
	}
}

func TestCompleteRetainsBindingIntroducedAfterEmptyBaseline(t *testing.T) {
	testCompleteDeniesWeakenedGuard(t, "late-binding")
}

func testCompleteDeniesWeakenedGuard(t *testing.T, discovery string) {
	root := cleanCompletionRepo(t)
	prod := filepath.Join(root, "pkg", "a.py")
	test := filepath.Join(root, "tests", "test_a.py")
	writeScopeFile(t, root, "tests/test_a.py", "def test_helper():\n    assert helper()\n")
	gitCommitFile(t, root, "tests/test_a.py")
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
	registerUnit(t, db, filepath.Join(root, "pkg"))
	registerUnit(t, db, filepath.Join(root, "tests"))
	prodUID := indexFile(t, db, filepath.Join(root, "pkg"), prod)
	indexPath(t, db, filepath.Join(root, "tests"), test)
	testKey := indexFileKey(t, db, test, "test_helper")
	seedResolvedReference(t, db, test, testKey, prodUID)
	if discovery == "late-binding" {
		out := callTool(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID})
		if out["allow"] != true {
			t.Fatalf("clean empty-baseline control = %+v", out)
		}
	}
	seedBinding(t, db, "INV-0002", prodUID, "bound")
	writeInvariantFile(t, root, "INV-0002", "CRITICAL")
	callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{
		"task_id": taskID, "paths": []string{test, prod},
	})
	if err := os.WriteFile(test, []byte("def test_other():\n    assert x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if discovery == "seed" {
		prodKey := indexFileKey(t, db, prod, "helper")
		seedChangeFileRow(t, db, taskID, test)
		seedChangeSymbolRow(t, db, taskID, prodKey, prodUID)
	} else if discovery != "" && discovery != "late-binding" {
		callTool(t, server, "probe", discovery, map[string]any{"task_id": taskID})
	}

	for _, reader := range []*mcp.Server{server, server, newTestServer(t, root)} {
		out := callTool(t, reader, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID})
		if allow, ok := out["allow"].(bool); !ok || allow {
			t.Fatalf("complete = %+v, want DENY", out)
		}
		codes := denialCodes(t, out)
		found := false
		for _, code := range codes {
			if code == "TEST_GUARD_WEAKENED" {
				found = true
			}
		}
		if !found {
			t.Fatalf("codes = %+v, want guard denial", codes)
		}
	}
	// A stored mapping must not cache policy: severity still comes from the
	// current knowledge store, and restoring proof must clear the signal.
	writeInvariantFile(t, root, "INV-0002", "LOW")
	low := callTool(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID})
	if slices.Contains(denialCodes(t, low), "TEST_GUARD_WEAKENED") {
		t.Fatalf("snapshot cached obsolete CRITICAL policy: %+v", low)
	}
	writeInvariantFile(t, root, "INV-0002", "CRITICAL")
	writeScopeFile(t, root, "tests/test_a.py", "def test_helper():\n    assert helper()\n")
	repaired := callTool(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID})
	if slices.Contains(denialCodes(t, repaired), "TEST_GUARD_WEAKENED") {
		t.Fatalf("restored proof kept a stale guard denial: %+v", repaired)
	}
}

func registerUnit(t *testing.T, db *sql.DB, pkg string) string {
	t.Helper()
	indexes := index.NewStore(db, app.FixedClock{})
	unit, err := indexes.UpsertUnit(t.Context(), pkg, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	return unit.ID
}

func seedIdentity(t *testing.T, db *sql.DB, uid, key string) {
	t.Helper()
	var unitID string
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM project_units LIMIT 1`).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT OR IGNORE INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES (?, 'PRJ-1', ?, 'python', ?, '[]', '2026-09-23T10:00:00Z')`,
		uid, unitID, key); err != nil {
		t.Fatal(err)
	}
}

func seedSymbolRow(t *testing.T, db *sql.DB, path, key, name, uid string) {
	t.Helper()
	var unitID string
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM project_units LIMIT 1`).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO symbols
		(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col,
		signature_hash, body_hash, structure_hash, symbol_uid)
		VALUES (?, ?, ?, 'function', ?, 1, 0, 2, 0, 's', 'b', 't', ?)`,
		unitID, path, key, name, uid); err != nil {
		t.Fatal(err)
	}
}

func seedBinding(t *testing.T, db *sql.DB, invariant, uid, status string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), `INSERT INTO invariant_symbol_bindings
		(invariant_id, symbol_uid, status, updated_at)
		VALUES (?, ?, ?, '2026-09-23T10:00:00Z')`, invariant, uid, status); err != nil {
		t.Fatal(err)
	}
}

func writeInvariantFile(t *testing.T, root, id, severity string) {
	t.Helper()
	content := `{"schema_version":1,"kind":"invariant","id":"` + id + `","status":"active",` +
		`"created_at":"2026-09-23T10:00:00Z","statement":"Hold the line.",` +
		`"severity":"` + severity + `","scope":{"level":"PROJECT"}}`
	abs := filepath.Join(root, ".mindrail", "knowledge", "invariants", id+".json")
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func seedChangeRows(t *testing.T, db *sql.DB, taskID, path, key, uid string) {
	t.Helper()
	store, err := changes.NewStore(db, app.FixedClock{})
	if err != nil {
		t.Fatal(err)
	}
	change, err := store.EnsureOpenChange(t.Context(), taskID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFileRows(t.Context(), change.ID, []changes.FileChange{
		{Path: path, Kind: changes.FileModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSymbolRows(t.Context(), change.ID, []changes.SymbolChange{
		{Key: key, UID: uid, Kind: changes.SymbolModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
}

func seedChangeFileRow(t *testing.T, db *sql.DB, taskID, path string) {
	t.Helper()
	store, err := changes.NewStore(db, app.FixedClock{})
	if err != nil {
		t.Fatal(err)
	}
	change, err := store.EnsureOpenChange(t.Context(), taskID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFileRows(t.Context(), change.ID, []changes.FileChange{
		{Path: path, Kind: changes.FileModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
}

func seedChangeSymbolRow(t *testing.T, db *sql.DB, taskID, key, uid string) {
	t.Helper()
	store, err := changes.NewStore(db, app.FixedClock{})
	if err != nil {
		t.Fatal(err)
	}
	change, err := store.EnsureOpenChange(t.Context(), taskID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSymbolRows(t.Context(), change.ID, []changes.SymbolChange{
		{Key: key, UID: uid, Kind: changes.SymbolModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
}

func indexFile(t *testing.T, db *sql.DB, pkg, path string) string {
	t.Helper()
	indexPath(t, db, pkg, path)
	var uid string
	if err := db.QueryRowContext(t.Context(), `SELECT symbol_uid FROM symbols WHERE path = ? AND name = 'helper'`, path).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	return uid
}

func indexPath(t *testing.T, db *sql.DB, pkg, path string) {
	t.Helper()
	clock := app.FixedClock{}
	indexes := index.NewStore(db, clock)
	unit, err := indexes.UpsertUnit(t.Context(), pkg, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	indexer := index.NewIndexer(indexes, registry, snapshot.New(filesystem.RuntimePaths{CacheDir: t.TempDir()}))
	if _, err := indexer.IndexFile(t.Context(), "PRJ-1", unit, path); err != nil {
		t.Fatal(err)
	}
}

func writeScopeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func indexFileKey(t *testing.T, db *sql.DB, path, name string) string {
	t.Helper()
	var key string
	if err := db.QueryRowContext(t.Context(), `SELECT logical_key FROM symbols WHERE path = ? AND name = ?`, path, name).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

func seedResolvedReference(t *testing.T, db *sql.DB, path, referrerKey, targetUID string) {
	t.Helper()
	var targetID int64
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE symbol_uid = ?`, targetUID).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	var unitID string
	if err := db.QueryRowContext(t.Context(), `SELECT unit_id FROM symbols WHERE path = ? AND logical_key = ?`, path, referrerKey).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO symbol_references
		(unit_id, path, referrer_key, target_text, label, confidence, resolved_symbol_id)
		VALUES (?, ?, ?, 'helper', 'STRUCTURAL_NAME_MATCH', 0.5, ?)`,
		unitID, path, referrerKey, targetID); err != nil {
		t.Fatal(err)
	}
}

// TestCompleteVersionErrors is TASK-02 AC-02.3: budget, escalation and
// approval refuse as structured version errors on both tools, absent
// means default flow.
func TestCompleteVersionErrors(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)

	for _, tool := range []string{mcp.ToolValidate, mcp.ToolComplete} {
		base := map[string]any{"task_id": taskID}
		if tool == mcp.ToolValidate {
			base = map[string]any{"profile": "test"}
		}
		for _, param := range []string{"budget", "escalation", "approval"} {
			args := map[string]any{}
			for key, value := range base {
				args[key] = value
			}
			args[param] = "gold"
			got := callTool(t, server, "probe", tool, args)
			refusal, ok := got["refusal"].(map[string]any)
			if !ok || refusal["code"] != "NOT_IMPLEMENTED_IN_THIS_VERSION" {
				t.Fatalf("%s %s = %+v, want version error", tool, param, got)
			}
			if actions, ok := refusal["next_action"].([]any); !ok || len(actions) == 0 {
				t.Fatalf("%s %s refusal without next_action: %+v", tool, param, got)
			}
		}
	}
}

// TestCompleteDeniesAnchoredAmbiguity is the fifth family beat: an
// ambiguity anchored to an active CRITICAL invariant denies with the
// identity code.
func TestCompleteDeniesAnchoredAmbiguity(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
	registerUnit(t, db, filepath.Join(root, "pkg"))
	seedIdentity(t, db, "SYM-A-1", "amb-key")
	seedBinding(t, db, "INV-0002", "SYM-A-1", "bound")
	writeInvariantFile(t, root, "INV-0002", "CRITICAL")
	if _, err := db.ExecContext(t.Context(), `INSERT INTO symbol_identity_ambiguities
		(unit_id, removed_uid, removed_key, candidate_keys, created_at)
		SELECT unit_id, 'SYM-A-1', 'amb-key', '["k1","k2"]', '2026-09-23T10:00:00Z'
		FROM symbol_identities WHERE symbol_uid = 'SYM-A-1'`); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(root, "pkg", "a.py")
	callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{
		"task_id": taskID, "paths": []string{abs},
	})
	seedChangeRows(t, db, taskID, abs, "amb-key", "SYM-A-1")

	out := callTool(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID})
	if allow, ok := out["allow"].(bool); !ok || allow {
		t.Fatalf("complete = %+v, want DENY", out)
	}
	codes := denialCodes(t, out)
	found := false
	for _, code := range codes {
		if code == "SYMBOL_IDENTITY_AMBIGUOUS" {
			found = true
		}
	}
	if !found {
		t.Fatalf("codes = %+v, want anchored ambiguity", codes)
	}
}

// TestCompleteRefusesUnknownTask pins fail-closed composition: completion
// of a task the repository never held refuses instead of allowing empty.
func TestCompleteRefusesUnknownTask(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)
	if err := callToolRaw(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": "TSK-NOPE"}); err == nil {
		t.Fatal("unknown task allowed")
	}
}
