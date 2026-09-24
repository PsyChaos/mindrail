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

// TestKnowledgeValidateCorruptFailsClosed is TASK-01 AC-01.3: a
// newer-than-binary record fails the whole validation with its code
// before any source check. (Malformed JSON is a listed non-fatal problem;
// an unreadable schema version is fatal.)
func TestKnowledgeValidateCorruptFailsClosed(t *testing.T) {
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

// TestVerifyStagedKnowledgeFirst pins the D-217 ordering: corrupt knowledge
// denies verify before any source evaluation runs.
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
