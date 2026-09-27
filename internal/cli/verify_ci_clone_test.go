package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// cloneRepo fresh-clones src through the real git binary: the
// hook-bypass story only means anything over an independent checkout,
// never over the developer's own worktree.
func cloneRepo(t *testing.T, src string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "clone")
	if out, err := exec.Command("git", "clone", "--quiet", src, dst).CombinedOutput(); err != nil {
		t.Fatalf("clone %s: %v: %s", src, err, out)
	}
	for _, args := range [][]string{
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dst}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dst
}

// initClone runs mindrail init in a fresh clone, the way CI would.
func initClone(t *testing.T, clone string) {
	t.Helper()
	if got := run(t, clone, "init"); got.code != app.ExitSuccess {
		t.Fatalf("clone init exited %d: %v\n%s", got.code, got.err, got.stdout)
	}
	// These legacy origins predate the generated instruction file. Keep the
	// range fixture clean; setup preservation has dedicated end-to-end tests.
	if err := os.Remove(filepath.Join(clone, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
}

// TestVerifyCIFreshCloneCleanGreen is TASK-02 AC-02.1's quiet half: a
// non-empty range of symbol-free changes verifies green in a fresh
// clone (Reader F1: clean is pinned over a non-empty range here).
func TestVerifyCIFreshCloneCleanGreen(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "README.md", "# docs\n")
	gitCommitFile(t, origin, "README.md")
	base := gitRev(t, origin, "HEAD")
	writeRepoFile(t, origin, "docs/notes.md", "# notes\n")
	gitCommitFile(t, origin, "docs/notes.md")

	clone := cloneRepo(t, origin)
	initClone(t, clone)

	code, denials := ciDenials(t, clone, "verify", "--ci", "--base", base, "--json")
	if code != app.ExitSuccess || len(denials) != 0 {
		t.Fatalf("code = %d, denials = %+v, want green", code, denials)
	}
	if n := changeFileCount(t, clone); n != 1 {
		t.Fatalf("files = %d, want 1: the green must judge the range, not an empty diff", n)
	}
}

// TestVerifyCINoVerifyReproduced is TASK-02 AC-02.1: a denial the local
// hook produced is committed with --no-verify anyway, and the fresh
// clone reproduces the identical code set through --ci.
func TestVerifyCINoVerifyReproduced(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, origin, "pkg/pyproject.toml")
	base := gitRev(t, origin, "HEAD")
	gitStageFile(t, origin, "pkg/a.py")

	_, staged := ciDenials(t, origin, "verify", "--staged", "--json")
	if len(staged) == 0 {
		t.Fatal("staged run denied nothing; bypass is vacuous")
	}
	// The hook would deny here; --no-verify ships it anyway, spelling
	// the flag the way agents actually reach for it.
	if out, err := exec.Command("git", "-C", origin,
		"commit", "--quiet", "--no-verify", "-m", "bypass").CombinedOutput(); err != nil {
		t.Fatalf("commit --no-verify: %v: %s", err, out)
	}

	clone := cloneRepo(t, origin)
	initClone(t, clone)

	code, ranged := ciDenials(t, clone, "verify", "--ci", "--base", base, "--json")
	if code != app.ExitFailed {
		t.Fatalf("code = %d, want the bypassed denial (denials %+v)", code, ranged)
	}
	want, got := denialCodes(staged), denialCodes(ranged)
	for key := range want {
		if got[key] == 0 {
			t.Fatalf("staged denial %q missing from clone CI %+v", key, ranged)
		}
	}
	for key := range got {
		if want[key] == 0 {
			t.Fatalf("clone denial %q has no staged counterpart %+v", key, staged)
		}
	}
}

// TestVerifyCIFreshCloneGuardBlocks is TASK-02 AC-02.2: a weakened
// CRITICAL test committed on head denies blocking in the fresh clone,
// with base-vs-head bytes.
func TestVerifyCIFreshCloneGuardBlocks(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "pkg/test_a.py",
		"def helper():\n    return 1\n\ndef test_helper():\n    assert helper()\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, origin, "pkg/pyproject.toml")
	gitCommitFile(t, origin, "pkg/test_a.py")
	// The binding's knowledge record travels committed in origin, so the
	// clone judges it like any fresh checkout — and head stays put
	// across both passes below.
	writeGuardInvariant(t, origin)
	gitCommitFile(t, origin, ".mindrail/knowledge/invariants/INV-G.json")
	base := gitRev(t, origin, "HEAD")
	writeRepoFile(t, origin, "pkg/test_a.py", "def helper():\n    return 1\n")
	gitCommitFile(t, origin, "pkg/test_a.py")

	clone := cloneRepo(t, origin)
	initClone(t, clone)
	registerTestUnit(t, clone, "pkg")
	// Populate only HEAD production facts, as normal inventory would. No
	// merge-base test symbol/reference is inserted into the runtime DB.
	run(t, clone, "verify", "--ci", "--base", base, "--json")
	seedGuardProductionBinding(t, clone, "pkg/test_a.py")

	// This is the first invocation with a binding to judge. Its mapping must
	// come from immutable merge-base bytes, not a synthetic persisted referrer.
	code, denials := ciDenials(t, clone, "verify", "--ci", "--base", base, "--json")
	if code != app.ExitFailed {
		t.Fatalf("code = %d, want guard denial (denials %+v)", code, denials)
	}
	found := false
	for _, denial := range denials {
		if denial.Code == string(app.CodeTestGuardWeakened) {
			found = true
		}
	}
	if !found {
		t.Fatalf("denials = %+v, want guard denial", denials)
	}
}

func TestVerifyCIFirstInvocationResolvesImportedProtectedSymbol(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "pkg/helper.py", "def helper():\n    return 1\n")
	writeRepoFile(t, origin, "pkg/test_a.py",
		"from helper import helper\n\ndef test_helper():\n    assert helper()\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	writeRepoFile(t, origin, ".mindrail/knowledge/invariants/INV-G.json",
		`{"schema_version":1,"kind":"invariant","id":"INV-G","status":"active",`+
			`"created_at":"2026-09-23T10:00:00Z","statement":"Helper holds.",`+
			`"severity":"CRITICAL","scope":{"level":"SYMBOL","target":"pkg/helper.py:helper"}}`)
	if out, err := exec.Command("git", "-C", origin, "add", "pkg", ".mindrail/knowledge/invariants/INV-G.json").CombinedOutput(); err != nil {
		t.Fatalf("git add fixture: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", origin, "-c", "user.email=t@t", "-c", "user.name=t",
		"commit", "--quiet", "--no-verify", "-m", "guard baseline").CombinedOutput(); err != nil {
		t.Fatalf("git commit fixture: %v: %s", err, out)
	}
	base := gitRev(t, origin, "HEAD")
	writeRepoFile(t, origin, "pkg/test_a.py", "from helper import helper\n")
	gitCommitFile(t, origin, "pkg/test_a.py")

	clone := cloneRepo(t, origin)
	initClone(t, clone)
	registerTestUnit(t, clone, "pkg")
	code, denials := ciDenials(t, clone, "verify", "--ci", "--base", base, "--json")
	if code != app.ExitFailed {
		t.Fatalf("first CI code = %d, want guard denial (%+v)", code, denials)
	}
	for _, denial := range denials {
		if denial.Code == string(app.CodeTestGuardWeakened) {
			return
		}
	}
	t.Fatalf("first CI denials = %+v, want imported-symbol guard denial", denials)
}

func TestVerifyCIFirstInvocationResolvesParentRelativeImportedSymbol(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "pkg/shared/helper.py", "def helper():\n    return 1\n")
	writeRepoFile(t, origin, "pkg/tests/test_a.py",
		"from ..shared.helper import helper\n\ndef test_helper():\n    assert helper()\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	writeRepoFile(t, origin, ".mindrail/knowledge/invariants/INV-G.json",
		`{"schema_version":1,"kind":"invariant","id":"INV-G","status":"active",`+
			`"created_at":"2026-09-23T10:00:00Z","statement":"Shared helper holds.",`+
			`"severity":"CRITICAL","scope":{"level":"SYMBOL","target":"pkg/shared/helper.py:helper"}}`)
	if out, err := exec.Command("git", "-C", origin, "add", "pkg", ".mindrail/knowledge/invariants/INV-G.json").CombinedOutput(); err != nil {
		t.Fatalf("git add fixture: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", origin, "-c", "user.email=t@t", "-c", "user.name=t",
		"commit", "--quiet", "--no-verify", "-m", "relative guard baseline").CombinedOutput(); err != nil {
		t.Fatalf("git commit fixture: %v: %s", err, out)
	}
	base := gitRev(t, origin, "HEAD")
	writeRepoFile(t, origin, "pkg/tests/test_a.py", "from ..shared.helper import helper\n")
	gitCommitFile(t, origin, "pkg/tests/test_a.py")

	clone := cloneRepo(t, origin)
	initClone(t, clone)
	registerTestUnit(t, clone, "pkg")
	code, denials := ciDenials(t, clone, "verify", "--ci", "--base", base, "--json")
	if code != app.ExitFailed {
		t.Fatalf("first CI code = %d, want relative-import guard denial (%+v)", code, denials)
	}
	for _, denial := range denials {
		if denial.Code == string(app.CodeTestGuardWeakened) {
			return
		}
	}
	t.Fatalf("first CI denials = %+v, want parent-relative imported-symbol guard denial", denials)
}

func TestVerifyCIFirstInvocationDoesNotJoinSameNameFromAnotherModule(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, origin, "pkg/b.py", "def helper():\n    return 2\n")
	writeRepoFile(t, origin, "pkg/test_b.py",
		"from b import helper\n\ndef test_helper():\n    assert helper() == 2\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	writeRepoFile(t, origin, ".mindrail/knowledge/invariants/INV-G.json",
		`{"schema_version":1,"kind":"invariant","id":"INV-G","status":"active",`+
			`"created_at":"2026-09-23T10:00:00Z","statement":"A helper holds.",`+
			`"severity":"CRITICAL","scope":{"level":"SYMBOL","target":"pkg/a.py:helper"}}`)
	if out, err := exec.Command("git", "-C", origin, "add", "pkg", ".mindrail/knowledge/invariants/INV-G.json").CombinedOutput(); err != nil {
		t.Fatalf("git add fixture: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", origin, "-c", "user.email=t@t", "-c", "user.name=t",
		"commit", "--quiet", "--no-verify", "-m", "same-name baseline").CombinedOutput(); err != nil {
		t.Fatalf("git commit fixture: %v: %s", err, out)
	}
	base := gitRev(t, origin, "HEAD")
	writeRepoFile(t, origin, "pkg/test_b.py", "from b import helper\n")
	gitCommitFile(t, origin, "pkg/test_b.py")

	clone := cloneRepo(t, origin)
	initClone(t, clone)
	registerTestUnit(t, clone, "pkg")
	_, denials := ciDenials(t, clone, "verify", "--ci", "--base", base, "--json")
	for _, denial := range denials {
		if denial.Code == string(app.CodeTestGuardWeakened) {
			t.Fatalf("denials = %+v, must not bind b.helper's test to protected a.helper", denials)
		}
	}
}

func TestVerifyCIFirstInvocationFailsClosedForOrphanedProtectedSymbol(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "pkg/helper.py", "def helper():\n    return 1\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	writeRepoFile(t, origin, ".mindrail/knowledge/invariants/INV-G.json",
		`{"schema_version":1,"kind":"invariant","id":"INV-G","status":"active",`+
			`"created_at":"2026-09-23T10:00:00Z","statement":"Helper holds.",`+
			`"severity":"CRITICAL","scope":{"level":"SYMBOL","target":"pkg/helper.py:helper"}}`)
	if out, err := exec.Command("git", "-C", origin, "add", "pkg", ".mindrail/knowledge/invariants/INV-G.json").CombinedOutput(); err != nil {
		t.Fatalf("git add fixture: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", origin, "-c", "user.email=t@t", "-c", "user.name=t",
		"commit", "--quiet", "--no-verify", "-m", "protected baseline").CombinedOutput(); err != nil {
		t.Fatalf("git commit fixture: %v: %s", err, out)
	}
	base := gitRev(t, origin, "HEAD")
	writeRepoFile(t, origin, "pkg/helper.py", "def replacement():\n    return 1\n")
	gitCommitFile(t, origin, "pkg/helper.py")

	clone := cloneRepo(t, origin)
	initClone(t, clone)
	got := run(t, clone, "verify", "--ci", "--base", base, "--json")
	if got.code != app.ExitFailed {
		t.Fatalf("code = %d, want fail-closed orphan denial (%v)", got.code, got.err)
	}
	if payload := got.errorPayload(t); payload.Code != app.CodeOrphanedProtectedSymbol {
		t.Fatalf("error code = %q, want %q", payload.Code, app.CodeOrphanedProtectedSymbol)
	}
}

func TestVerifyCIRenamedTestUsesMergeBasePath(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "pkg/old_test.py",
		"def helper():\n    return 1\n\ndef keep_one():\n    return 1\n\ndef keep_two():\n    return 2\n\ndef test_helper():\n    assert helper()\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, origin, "pkg/pyproject.toml")
	gitCommitFile(t, origin, "pkg/old_test.py")
	writeGuardInvariant(t, origin)
	gitCommitFile(t, origin, ".mindrail/knowledge/invariants/INV-G.json")
	base := gitRev(t, origin, "HEAD")
	if out, err := exec.Command("git", "-C", origin, "mv", "pkg/old_test.py", "pkg/new_test.py").CombinedOutput(); err != nil {
		t.Fatalf("git mv: %v: %s", err, out)
	}
	writeRepoFile(t, origin, "pkg/new_test.py",
		"def helper():\n    return 1\n\ndef keep_one():\n    return 1\n\ndef keep_two():\n    return 2\n")
	if out, err := exec.Command("git", "-C", origin, "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("git add rename: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", origin, "commit", "--quiet", "--no-verify", "-m", "rename weakened test").CombinedOutput(); err != nil {
		t.Fatalf("git commit rename: %v: %s", err, out)
	}

	clone := cloneRepo(t, origin)
	initClone(t, clone)
	registerTestUnit(t, clone, "pkg")
	run(t, clone, "verify", "--ci", "--base", base, "--json")
	seedGuardProductionBinding(t, clone, "pkg/new_test.py")
	code, denials := ciDenials(t, clone, "verify", "--ci", "--base", base, "--json")
	if code != app.ExitFailed {
		t.Fatalf("code = %d, want renamed-test guard denial (%+v)", code, denials)
	}
	found := false
	for _, denial := range denials {
		found = found || denial.Code == string(app.CodeTestGuardWeakened)
	}
	if !found {
		t.Fatalf("denials = %+v, want renamed-test guard denial", denials)
	}
}

// TestVerifyCIFreshCloneKnowledgeFailsClosed is TASK-02 AC-02.3: fatal
// knowledge committed at head refuses in the fresh clone before any
// source check, with no change row written.
func TestVerifyCIFreshCloneKnowledgeFailsClosed(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, origin, "pkg/pyproject.toml")
	writeKnowledgeRecord(t, origin, "DEC-0001.json",
		`{"schema_version":999,"kind":"decision","id":"DEC-0001","status":"active",`+
			`"created_at":"2026-09-23T10:00:00Z","title":"Future","decision":"Later."}`)
	gitCommitFile(t, origin, "pkg/a.py")
	if out, err := exec.Command("git", "-C", origin, "add",
		".mindrail/knowledge/decisions/DEC-0001.json").CombinedOutput(); err != nil {
		t.Fatalf("add knowledge: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", origin, "commit", "--no-verify", "--quiet", "-m", "fatal knowledge").CombinedOutput(); err != nil {
		t.Fatalf("commit knowledge: %v: %s", err, out)
	}
	base := gitRev(t, origin, "HEAD~2")

	clone := cloneRepo(t, origin)
	// init still initializes (DB, workspace) but exits fail-closed on
	// the fatal record — that refusal is the first half of the story.
	gotInit := run(t, clone, "init")
	gotInit.requireExit(t, app.ExitFailed)

	got := run(t, clone, "verify", "--ci", "--base", base, "--json")
	if got.code == app.ExitSuccess {
		t.Fatalf("clone passed over fatal knowledge: %s", got.stdout)
	}
	var data struct {
		Knowledge []struct {
			Fatal bool `json:"fatal"`
		} `json:"knowledge_problems"`
	}
	decodeData(t, got.stdout, &data)
	fatal := false
	for _, problem := range data.Knowledge {
		fatal = fatal || problem.Fatal
	}
	if !fatal {
		t.Fatalf("report names no fatal problem: %s", got.stdout)
	}
	if n := changeRowCount(t, clone); n != 0 {
		t.Fatalf("changes = %d, want 0: knowledge-first violated in clone", n)
	}
}

// TestVerifyCIDenialShape is TASK-02 AC-02.4: every clone denial carries
// code + provenance + remedy, and the exit follows the envelope table
// with no new codes.
func TestVerifyCIDenialShape(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, origin, "pkg/pyproject.toml")
	base := gitRev(t, origin, "HEAD")
	gitCommitFile(t, origin, "pkg/a.py")

	clone := cloneRepo(t, origin)
	initClone(t, clone)

	got := run(t, clone, "verify", "--ci", "--base", base, "--json")
	got.requireExit(t, app.ExitFailed)
	var data struct {
		Denials []struct {
			Code       string   `json:"code"`
			Provenance string   `json:"provenance"`
			NextAction []string `json:"next_action"`
		} `json:"denials"`
	}
	decodeData(t, got.stdout, &data)
	if len(data.Denials) == 0 {
		t.Fatalf("no denials: %s", got.stdout)
	}
	known := map[string]bool{}
	for _, line := range []string{
		"UNREGISTERED_CHANGE", "RECONCILE_AMBIGUOUS", "SCOPE_DRIFT",
		"TEST_GUARD_WEAKENED", "ORPHANED_PROTECTED_SYMBOL",
		"SYMBOL_IDENTITY_AMBIGUOUS",
	} {
		known[line] = true
	}
	for _, denial := range data.Denials {
		if denial.Code == "" || denial.Provenance == "" || len(denial.NextAction) == 0 {
			t.Fatalf("shapeless denial: %+v", denial)
		}
		if !known[denial.Code] && !strings.HasPrefix(denial.Code, "KNOWLEDGE") {
			t.Fatalf("unknown code %q (registry drift?)", denial.Code)
		}
	}
}

// TestVerifyCIDefaultOverBypassRefuses pins the main-tip rule: a bare
// --ci whose default base equals head judges nothing, so it refuses
// instead of certifying a bypassed tip green.
func TestVerifyCIDefaultOverBypassRefuses(t *testing.T) {
	origin := newInitializedRepo(t)
	writeRepoFile(t, origin, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, origin, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, origin, "pkg/pyproject.toml")
	gitStageFile(t, origin, "pkg/a.py")
	if _, staged := ciDenials(t, origin, "verify", "--staged", "--json"); len(staged) == 0 {
		t.Fatal("staged run denied nothing; bypass is vacuous")
	}
	if out, err := exec.Command("git", "-C", origin,
		"commit", "--quiet", "--no-verify", "-m", "bypass").CombinedOutput(); err != nil {
		t.Fatalf("commit --no-verify: %v: %s", err, out)
	}

	clone := cloneRepo(t, origin)
	initClone(t, clone)

	got := run(t, clone, "verify", "--ci", "--json")
	got.requireExit(t, app.ExitUsage)
}

// TestVerifyCINarrowsAcrossRuns pins recompute semantics: a wide denial
// followed by an explicit empty range greens — stale rows from the wider
// run are not re-judged.
func TestVerifyCINarrowsAcrossRuns(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	base := gitRev(t, repo, "HEAD")
	gitCommitFile(t, repo, "pkg/a.py")
	head := gitRev(t, repo, "HEAD")

	wide, denials := ciDenials(t, repo, "verify", "--ci", "--base", base, "--json")
	if wide != app.ExitFailed || len(denials) == 0 {
		t.Fatalf("wide run: code = %d, denials = %+v, want denial", wide, denials)
	}
	narrow, narrowed := ciDenials(t, repo, "verify", "--ci", "--base", head, "--head", head, "--json")
	if narrow != app.ExitSuccess || len(narrowed) != 0 {
		t.Fatalf("narrow run: code = %d, denials = %+v, want green", narrow, narrowed)
	}
}
