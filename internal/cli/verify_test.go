package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
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

// TestVerifyBareRefusesMode pins the no-silent-default rule: verify
// without --staged refuses with usage and a next action.
func TestVerifyBareRefusesMode(t *testing.T) {
	repo := newInitializedRepo(t)
	got := run(t, repo, "verify", "--json")
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
