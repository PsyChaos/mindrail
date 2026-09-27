package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/storage"
)

func writeRepoFile(t *testing.T, repo, rel, content string) string {
	t.Helper()
	abs := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return abs
}

func gitStageFile(t *testing.T, repo, rel string) {
	t.Helper()
	for _, args := range [][]string{
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
		{"add", rel},
	} {
		full := append([]string{"-C", repo}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", full, err, out)
		}
	}
}

func gitCommitFile(t *testing.T, repo, rel string) {
	t.Helper()
	gitStageFile(t, repo, rel)
	full := []string{"-C", repo, "commit", "--no-verify", "--quiet", "-m", "base"}
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", full, err, out)
	}
}

// Local verification defaults to the existing staged path.
func TestVerifyBareDefaultsStaged(t *testing.T) {
	repo := newInitializedRepo(t)
	got := run(t, repo, "verify", "--json")
	got.requireExit(t, app.ExitSuccess)
}

// TestVerifyExtraArgsRefused pins exact invocation: stray positionals
// refuse instead of judging a possibly-mistyped request as clean.
func TestVerifyExtraArgsRefused(t *testing.T) {
	repo := newInitializedRepo(t)
	got := run(t, repo, "verify", "foo", "--staged", "--json")
	got.requireExit(t, app.ExitUsage)
}

// TestVerifyStagedJudgesIndexNotWorktree pins decision D-216: staged A
// with unstaged B on disk judges A twice identically. The worktree desk
// is invisible (a broken desk is a known index-layer limitation, recorded
// in findings — the index follows the desk while rows follow the commit).
func TestVerifyStagedJudgesIndexNotWorktree(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def f():\n    return 1\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitStageFile(t, repo, "pkg/a.py")

	firstHashes := stagedSymbolHashes(t, repo)
	writeRepoFile(t, repo, "pkg/a.py", "def f():\n    return 2\n")
	secondHashes := stagedSymbolHashes(t, repo)
	if firstHashes != secondHashes {
		t.Fatalf("worktree edit moved staged judgment: %q vs %q", firstHashes, secondHashes)
	}
}

func stagedSymbolHashes(t *testing.T, repo string) string {
	t.Helper()
	got := run(t, repo, "verify", "--staged", "--json")
	var data struct {
		Denials []struct {
			Code string `json:"code"`
		} `json:"denials"`
	}
	decodeData(t, got.stdout, &data)
	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.DB.QueryContext(t.Context(),
		`SELECT logical_key, body_hash FROM change_symbols ORDER BY logical_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var digest string
	for rows.Next() {
		var key, hash string
		if err := rows.Scan(&key, &hash); err != nil {
			t.Fatal(err)
		}
		digest += key + "=" + hash + ";"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return digest
}

// TestVerifyStagedGuardDeniesBlocking is TASK-02 AC-02.3: a staged
// weakened CRITICAL test denies through the real command.
func TestVerifyStagedGuardDeniesBlocking(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/test_a.py",
		"def helper():\n    return 1\n\ndef test_helper():\n    assert helper()\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/test_a.py")
	registerTestUnit(t, repo, "pkg")
	writeRepoFile(t, repo, "pkg/test_a.py", "def helper():\n    return 1\n")
	gitStageFile(t, repo, "pkg/test_a.py")
	// First pass populates the index (helper uid); the guard beat seeds
	// binding + reference on top, like the service-level two-pass test.
	run(t, repo, "verify", "--staged", "--json")
	seedGuardBinding(t, repo, "pkg/test_a.py")

	got := run(t, repo, "verify", "--staged", "--json")
	got.requireExit(t, app.ExitFailed)
	var data struct {
		Denials []struct {
			Code string `json:"code"`
		} `json:"denials"`
	}
	decodeData(t, got.stdout, &data)
	found := false
	for _, denial := range data.Denials {
		if denial.Code == string(app.CodeTestGuardWeakened) {
			found = true
		}
	}
	if !found {
		t.Fatalf("denials = %+v, want guard denial", data.Denials)
	}
}

func registerTestUnit(t *testing.T, repo, dir string) {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	indexes := index.NewStore(db.DB, app.FixedClock{})
	if _, err := indexes.UpsertUnit(t.Context(), filepath.Join(repo, dir), index.UnitPython); err != nil {
		t.Fatal(err)
	}
}

func seedGuardBinding(t *testing.T, repo, test string) {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	abs := filepath.Join(repo, filepath.FromSlash(test))
	var unitID string
	if err := db.DB.QueryRowContext(t.Context(),
		`SELECT id FROM project_units LIMIT 1`).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	var helperUID string
	if err := db.DB.QueryRowContext(t.Context(),
		`SELECT symbol_uid FROM symbols WHERE path = ? AND name = 'helper'`, abs).Scan(&helperUID); err != nil {
		t.Fatalf("helper not indexed (run verify once first): %v", err)
	}
	if _, err := db.DB.ExecContext(t.Context(), `INSERT OR IGNORE INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES ('SYM-T-SEED', 'PRJ-1', ?, 'python', 'test-key', '[]', '2026-09-23T10:00:00Z')`,
		unitID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(t.Context(), `INSERT INTO symbols
		(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col,
		signature_hash, body_hash, structure_hash, symbol_uid)
		VALUES (?, ?, 'test-key', 'function', 'test_helper', 1, 0, 2, 0, 's', 'b', 't', 'SYM-T-SEED')`,
		unitID, abs); err != nil {
		t.Fatal(err)
	}
	var helperID int64
	if err := db.DB.QueryRowContext(t.Context(), `SELECT id FROM symbols WHERE symbol_uid = ?`,
		helperUID).Scan(&helperID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(t.Context(), `INSERT INTO symbol_references
		(unit_id, path, referrer_key, target_text, label, confidence, resolved_symbol_id)
		VALUES (?, ?, 'test-key', 'helper', 'STRUCTURAL_NAME_MATCH', 0.5, ?)`,
		unitID, abs, helperID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(t.Context(), `INSERT INTO invariant_symbol_bindings
		(invariant_id, symbol_uid, status, updated_at)
		VALUES ('INV-G', ?, 'bound', '2026-09-23T10:00:00Z')`, helperUID); err != nil {
		t.Fatal(err)
	}
	writeGuardInvariant(t, repo)
}

// seedGuardProductionBinding adds only the production invariant binding.
// Unlike seedGuardBinding it creates no baseline test symbol or reference;
// CI must recover those from merge-base bytes.
func seedGuardProductionBinding(t *testing.T, repo, test string) {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	abs := filepath.Join(repo, filepath.FromSlash(test))
	var helperUID string
	if err := db.DB.QueryRowContext(t.Context(),
		`SELECT symbol_uid FROM symbols WHERE path = ? AND name = 'helper'`, abs).Scan(&helperUID); err != nil {
		t.Fatalf("helper not indexed: %v", err)
	}
	if _, err := db.DB.ExecContext(t.Context(), `INSERT INTO invariant_symbol_bindings
		(invariant_id, symbol_uid, status, updated_at)
		VALUES ('INV-G', ?, 'bound', '2026-09-23T10:00:00Z')`, helperUID); err != nil {
		t.Fatal(err)
	}
}

// writeGuardInvariant drops the CRITICAL INV-G record the guard beat
// binds against. Callers judging a committed range commit it first.
func writeGuardInvariant(t *testing.T, repo string) {
	t.Helper()
	dir := filepath.Join(repo, ".mindrail", "knowledge", "invariants")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	inv := `{"schema_version":1,"kind":"invariant","id":"INV-G","status":"active",` +
		`"created_at":"2026-09-23T10:00:00Z","statement":"Helpers hold.",` +
		`"severity":"CRITICAL","scope":{"level":"PROJECT"}}`
	if err := os.WriteFile(filepath.Join(dir, "INV-G.json"), []byte(inv), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestVerifyStagedAllowWarnOnly is TASK-02 AC-02.3: a staged change with
// no symbols and no scope impact verifies green — warnings alone never
// deny at the gate.
func TestVerifyStagedAllowWarnOnly(t *testing.T) {
	repo := newInitializedRepo(t)
	readme := writeRepoFile(t, repo, "README.md", "# docs\n")
	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	var projectID, workspaceID string
	if err := db.DB.QueryRowContext(t.Context(), `SELECT project_id FROM projects LIMIT 1`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.QueryRowContext(t.Context(), `SELECT workspace_id FROM workspaces LIMIT 1`).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	const sessionID = "SES-DOCS"
	if _, err := db.DB.ExecContext(t.Context(), `INSERT INTO sessions
		(session_id, workspace_id, label, started_at)
		VALUES (?, ?, 'docs', '2026-09-28T00:00:00Z')`, sessionID, workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(t.Context(), `INSERT INTO tasks
		(task_id, project_id, title, state, opened_by, created_at, updated_at)
		VALUES ('TSK-DOCS', ?, 'docs', 'OPEN', ?, '2026-09-28T00:00:00Z', '2026-09-28T00:00:00Z')`,
		projectID, sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(t.Context(), `INSERT INTO change_baselines
		(task_id, path, content_hash, captured_at) VALUES ('TSK-DOCS', ?, '', '2026-09-28T00:00:00Z')`, readme); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	gitStageFile(t, repo, "README.md")

	got := run(t, repo, "verify", "--staged", "--json")
	got.requireExit(t, app.ExitSuccess)
	var data struct {
		Allow   bool  `json:"allow"`
		Denials []any `json:"denials"`
	}
	decodeData(t, got.stdout, &data)
	if !data.Allow || len(data.Denials) != 0 {
		t.Fatalf("data = %+v, want ALLOW", data)
	}
}

// TestVerifyStagedCleanGreen is TASK-01 AC-01.4: an empty index verifies
// green with empty denials.
func TestVerifyStagedCleanGreen(t *testing.T) {
	repo := newInitializedRepo(t)
	got := run(t, repo, "verify", "--staged", "--json")
	got.requireExit(t, app.ExitSuccess)
	var data struct {
		Allow   bool  `json:"allow"`
		Denials []any `json:"denials"`
	}
	decodeData(t, got.stdout, &data)
	if !data.Allow || len(data.Denials) != 0 {
		t.Fatalf("data = %+v", data)
	}
}

// TestVerifyStagedUnregisteredDenies is TASK-01 AC-01.1: a staged file no
// baseline covers denies with the attribution code and exit.
func TestVerifyStagedUnregisteredDenies(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitStageFile(t, repo, "pkg/a.py")

	got := run(t, repo, "verify", "--staged", "--json")
	got.requireExit(t, app.ExitFailed)
	var data struct {
		Allow   bool `json:"allow"`
		Denials []struct {
			Code       string   `json:"code"`
			NextAction []string `json:"next_action"`
		} `json:"denials"`
	}
	decodeData(t, got.stdout, &data)
	if data.Allow || len(data.Denials) == 0 {
		t.Fatalf("data = %+v, want DENY", data)
	}
	if data.Denials[0].Code != string(app.CodeUnregisteredChange) {
		t.Fatalf("code = %q", data.Denials[0].Code)
	}
	if len(data.Denials[0].NextAction) == 0 {
		t.Fatal("denial without remedy")
	}
}
