package cli_test

// TEMPORARY MR-020 demonstration driver — deleted before milestone close
// (D-241: no implementation change; scratch driver only, never committed).
//
// Every verdict below comes from the real built binary (MINDRAIL_DEMO_BIN,
// default /tmp/mr020/bin/mindrail) executed over scratch repositories.
// t.Log carries the verbatim gate outputs that findings paste.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/storage"
)

func demoBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("MINDRAIL_DEMO_BIN")
	if bin == "" {
		bin = "/tmp/mr020/bin/mindrail"
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("demo binary missing: %s", bin)
	}
	return bin
}

type demoResult struct {
	code   int
	stdout string
	stderr string
}

func demoRun(t *testing.T, dir string, args ...string) demoResult {
	t.Helper()
	cmd := exec.Command(demoBin(t), append(args, "-C", dir)...)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatalf("exec %v: %v", args, err)
		}
	}
	return demoResult{code: code, stdout: string(out)}
}

func demoGit(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatalf("git %v: %v: %s", full, err, out)
		}
	}
	return code, string(out)
}

func demoLog(t *testing.T, title string, r demoResult) {
	t.Helper()
	t.Logf("=== %s (exit %d) ===\n%s", title, r.code, r.stdout)
}

// TestMR020DemoAllowTwoWorktrees is MR-020 AC-01.1: agent A, working in its
// own worktree, stages a symbol-free docs change; the independent gate
// allows it.
func TestMR020DemoAllowTwoWorktrees(t *testing.T) {
	repo := newRepoWithCommit(t)
	if got := demoRun(t, repo, "init"); got.code != app.ExitSuccess {
		t.Fatalf("init exit %d:\n%s", got.code, got.stdout)
	}
	for _, wt := range []string{"wt-a", "wt-b"} {
		if code, out := demoGit(t, repo, "worktree", "add", "--quiet", wt, "-b", wt); code != 0 {
			t.Fatalf("worktree add %s: %s", wt, out)
		}
	}
	wtA := filepath.Join(repo, "wt-a")
	if got := demoRun(t, wtA, "init"); got.code != app.ExitSuccess {
		t.Fatalf("worktree init exit %d:\n%s", got.code, got.stdout)
	}
	writeRepoFile(t, wtA, "docs/notes.md", "# notes\n")
	demoGit(t, wtA, "add", "docs/notes.md")

	got := demoRun(t, wtA, "verify", "--staged", "--json")
	demoLog(t, "agent-A docs-only change: verify --staged", got)
	if got.code != app.ExitSuccess {
		t.Fatalf("honest run denied:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, `"allow":true`) {
		t.Fatalf("no ALLOW in output:\n%s", got.stdout)
	}
}

// TestMR020DemoForgottenCall is MR-020 AC-01.2: agent B stages a real code
// change with no protocol call behind it; reconcile discovers the actual
// diff and the gate denies it as unregistered.
func TestMR020DemoForgottenCall(t *testing.T) {
	repo := newRepoWithCommit(t)
	if got := demoRun(t, repo, "init"); got.code != app.ExitSuccess {
		t.Fatalf("init exit %d:\n%s", got.code, got.stdout)
	}
	if code, out := demoGit(t, repo, "worktree", "add", "--quiet", "wt-b", "-b", "wt-b"); code != 0 {
		t.Fatalf("worktree add: %s", out)
	}
	wtB := filepath.Join(repo, "wt-b")
	if got := demoRun(t, wtB, "init"); got.code != app.ExitSuccess {
		t.Fatalf("worktree init exit %d:\n%s", got.code, got.stdout)
	}
	// No claim, no before_change — the agent just edits and stages.
	writeRepoFile(t, wtB, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, wtB, "pkg/pyproject.toml", "[project]\n")
	demoGit(t, wtB, "add", "pkg/a.py")

	got := demoRun(t, wtB, "verify", "--staged", "--json")
	demoLog(t, "agent-B forgotten call: verify --staged", got)
	if got.code == app.ExitSuccess {
		t.Fatalf("undeclared diff allowed:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "UNREGISTERED_CHANGE") {
		t.Fatalf("no unregistered-change denial:\n%s", got.stdout)
	}
}

// TestMR020DemoNoVerifyCI is MR-020 AC-02.1: the local hook denies a staged
// violation; the agent ships it with --no-verify anyway; a fresh clone's
// verify --ci reproduces the identical denial codes.
func TestMR020DemoNoVerifyCI(t *testing.T) {
	origin := newInitializedRepo(t)
	demoGit(t, origin, "config", "user.email", "t@t")
	demoGit(t, origin, "config", "user.name", "t")
	binDir := t.TempDir()
	binPath := demoBin(t)
	raw, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "mindrail"), raw, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if got := demoRun(t, origin, "hook", "install"); got.code != app.ExitSuccess {
		t.Fatalf("hook install exit %d:\n%s", got.code, got.stdout)
	}
	writeRepoFile(t, origin, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	demoGit(t, origin, "add", "pkg/pyproject.toml")
	if code, out := demoGit(t, origin, "commit", "--quiet", "-m", "base"); code != 0 {
		t.Fatalf("base commit: %s", out)
	}
	base := strings.TrimSpace(func() string {
		_, out := demoGit(t, origin, "rev-parse", "HEAD")
		return out
	}())
	demoGit(t, origin, "add", "pkg/a.py")

	staged := demoRun(t, origin, "verify", "--staged", "--json")
	demoLog(t, "local gate: verify --staged", staged)
	if staged.code == app.ExitSuccess {
		t.Fatal("staged run denied nothing; bypass is vacuous")
	}

	code, hookOut := demoGit(t, origin, "commit", "-m", "sneaky")
	t.Logf("=== plain commit against the hook (exit %d) ===\n%s", code, hookOut)
	if code == 0 {
		t.Fatal("hook let the violation through; bypass story is void")
	}
	code, bypassOut := demoGit(t, origin, "commit", "--no-verify", "--quiet", "-m", "bypass")
	t.Logf("=== commit --no-verify (exit %d) ===\n%s", code, bypassOut)
	if code != 0 {
		t.Fatalf("commit --no-verify: %s", bypassOut)
	}

	clone := cloneRepo(t, origin)
	initClone(t, clone)
	ci := demoRun(t, clone, "verify", "--ci", "--base", base, "--json")
	demoLog(t, "fresh clone: verify --ci", ci)
	if ci.code == app.ExitSuccess {
		t.Fatalf("CI allowed the bypass:\n%s", ci.stdout)
	}
	stagedCodes, ciCodes := denialCodeSet(t, staged.stdout), denialCodeSet(t, ci.stdout)
	if len(stagedCodes) == 0 {
		t.Fatal("staged run denied nothing; bypass is vacuous")
	}
	for code, n := range stagedCodes {
		if ciCodes[code] != n {
			t.Fatalf("staged denial %q x%d != clone %v:\n%s", code, n, ciCodes, ci.stdout)
		}
	}
	for code := range ciCodes {
		if stagedCodes[code] == 0 {
			t.Fatalf("clone denial %q has no staged counterpart:\n%s", code, ci.stdout)
		}
	}
}

func denialCodeSet(t *testing.T, stdout string) map[string]int {
	t.Helper()
	var data struct {
		Data struct {
			Denials []struct {
				Code string `json:"code"`
			} `json:"denials"`
		} `json:"data"`
	}
	// The binary appends a human summary line after the JSON envelope.
	first, _, _ := strings.Cut(stdout, "\n")
	if err := json.Unmarshal([]byte(first), &data); err != nil {
		// Human-output runs (hook block) carry no JSON; they are logged, not graded.
		return map[string]int{}
	}
	out := map[string]int{}
	for _, d := range data.Data.Denials {
		out[d.Code]++
	}
	return out
}

// TestMR020DemoTestWeakening is MR-020 AC-03.1: agent B, working in its own
// worktree, touches only the critical test — production code is
// byte-identical — and the guard still blocks.
func TestMR020DemoTestWeakening(t *testing.T) {
	repo := newRepoWithCommit(t)
	if got := demoRun(t, repo, "init"); got.code != app.ExitSuccess {
		t.Fatalf("init exit %d:\n%s", got.code, got.stdout)
	}
	writeRepoFile(t, repo, "pkg/test_a.py",
		"def helper():\n    return 1\n\ndef test_helper():\n    assert helper()\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	gitCommitFile(t, repo, "pkg/test_a.py")
	if code, out := demoGit(t, repo, "worktree", "add", "--quiet", "wt-c", "-b", "wt-c"); code != 0 {
		t.Fatalf("worktree add: %s", out)
	}
	wtC := filepath.Join(repo, "wt-c")
	if got := demoRun(t, wtC, "init"); got.code != app.ExitSuccess {
		t.Fatalf("worktree init exit %d:\n%s", got.code, got.stdout)
	}
	registerTestUnitWorktree(t, wtC, "pkg")
	writeRepoFile(t, wtC, "pkg/test_a.py", "def helper():\n    return 1\n")
	gitStageFile(t, wtC, "pkg/test_a.py")
	demoRun(t, wtC, "verify", "--staged", "--json")
	seedGuardBindingWorktree(t, wtC, "pkg/test_a.py")

	// Production-untouched pin: the helper bytes survive in both revisions.
	_, committed := demoGit(t, wtC, "show", "HEAD:pkg/test_a.py")
	staged, err := os.ReadFile(filepath.Join(wtC, "pkg", "test_a.py"))
	if err != nil {
		t.Fatal(err)
	}
	const prod = "def helper():\n    return 1\n"
	if !strings.Contains(committed, prod) || !strings.Contains(string(staged), prod) {
		t.Fatalf("production bytes differ: committed=%q staged=%q", committed, staged)
	}
	if strings.Contains(string(staged), "test_helper") {
		t.Fatalf("staged file still contains the test: %q", staged)
	}

	got := demoRun(t, wtC, "verify", "--staged", "--json")
	demoLog(t, "agent-B weakened critical test, production untouched: verify --staged", got)
	if got.code == app.ExitSuccess {
		t.Fatalf("weakened test allowed:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "TEST_GUARD_WEAKENED") {
		t.Fatalf("no guard denial:\n%s", got.stdout)
	}
}

// demoCommonDBPath resolves the runtime database through the git common
// dir: linked worktrees share one database there, while their own .git is
// a gitdir-pointer file the suite helpers cannot open.
func demoCommonDBPath(t *testing.T, wt string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", wt, "rev-parse", "--git-common-dir").CombinedOutput()
	if err != nil {
		t.Fatalf("git common-dir: %v: %s", err, out)
	}
	common := strings.TrimSpace(string(out))
	if !filepath.IsAbs(common) {
		common = filepath.Join(wt, common)
	}
	return filepath.Join(common, "mindrail", "mindrail.db")
}

// registerTestUnitWorktree mirrors registerTestUnit against the shared
// common-dir database instead of <worktree>/.git (a file, not a dir).
func registerTestUnitWorktree(t *testing.T, wt, dir string) {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: demoCommonDBPath(t, wt)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	indexes := index.NewStore(db.DB, app.FixedClock{})
	if _, err := indexes.UpsertUnit(t.Context(), filepath.Join(wt, dir), index.UnitPython); err != nil {
		t.Fatal(err)
	}
}

// seedGuardBindingWorktree mirrors seedGuardBinding against the shared
// common-dir database, binding the worktree's own indexed helper.
func seedGuardBindingWorktree(t *testing.T, wt, test string) {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: demoCommonDBPath(t, wt)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	abs := filepath.Join(wt, filepath.FromSlash(test))
	var unitID string
	if err := db.DB.QueryRowContext(t.Context(),
		`SELECT id FROM project_units WHERE path = ?`, filepath.Join(wt, "pkg")).Scan(&unitID); err != nil {
		t.Fatalf("worktree unit missing: %v", err)
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
	writeGuardInvariant(t, wt)
}
