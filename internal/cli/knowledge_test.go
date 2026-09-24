package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestKnowledgeValidateClean is TASK-01 AC-01.3's quiet half: healthy
// knowledge validates green with counts.
func TestKnowledgeValidateClean(t *testing.T) {
	repo := newInitializedRepo(t)
	got := run(t, repo, "knowledge", "validate", "--json")
	got.requireExit(t, app.ExitSuccess)
	var data struct {
		Present    bool  `json:"present"`
		Decisions  int   `json:"decisions"`
		Invariants int   `json:"invariants"`
		Problems   []any `json:"problems"`
	}
	decodeData(t, got.stdout, &data)
	if len(data.Problems) != 0 {
		t.Fatalf("data = %+v", data)
	}
}

// TestKnowledgeValidateNewerSchemaFailsClosed is TASK-01 AC-01.3: a
// newer-than-binary record fails the whole validation with its code
// before any source check. (Malformed JSON is a listed non-fatal problem
// by loader design — see the AC-01.3 deviation note in findings.)
func TestKnowledgeValidateNewerSchemaFailsClosed(t *testing.T) {
	repo := newInitializedRepo(t)
	dir := filepath.Join(repo, ".mindrail", "knowledge", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	future := `{"schema_version":999,"kind":"decision","id":"DEC-0001","status":"active",` +
		`"created_at":"2026-09-23T10:00:00Z","title":"Future","decision":"Later."}`
	if err := os.WriteFile(filepath.Join(dir, "DEC-0001.json"), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}
	got := run(t, repo, "knowledge", "validate", "--json")
	if got.code == app.ExitSuccess {
		t.Fatalf("future knowledge validated: %s", got.stdout)
	}
}

// TestKnowledgeValidateStoreRootNotDir is Breaker B-3's repro as a pin:
// a store root that is not a directory fails closed instead of reporting
// an empty success.
func TestKnowledgeValidateStoreRootNotDir(t *testing.T) {
	repo := newInitializedRepo(t)
	root := filepath.Join(repo, ".mindrail", "knowledge")
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := run(t, repo, "knowledge", "validate", "--json")
	if got.code == app.ExitSuccess {
		t.Fatalf("file-root knowledge validated: %s", got.stdout)
	}
}
func TestVerifyStagedKnowledgeFirst(t *testing.T) {
	repo := newInitializedRepo(t)
	dir := filepath.Join(repo, ".mindrail", "knowledge", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	future := `{"schema_version":999,"kind":"decision","id":"DEC-0001","status":"active",` +
		`"created_at":"2026-09-23T10:00:00Z","title":"Future","decision":"Later."}`
	if err := os.WriteFile(filepath.Join(dir, "DEC-0001.json"), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	gitStageFile(t, repo, "pkg/a.py")

	got := run(t, repo, "verify", "--staged", "--json")
	if got.code == app.ExitSuccess {
		t.Fatalf("verify passed over corrupt knowledge: %s", got.stdout)
	}
	var dbRows int
	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM changes`).Scan(&dbRows); err != nil {
		t.Fatal(err)
	}
	if dbRows != 0 {
		t.Fatalf("knowledge-first violated: %d change rows written", dbRows)
	}
}
