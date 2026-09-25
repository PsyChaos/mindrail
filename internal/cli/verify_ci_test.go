package cli_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// gitRev resolves one rev to its SHA through the real binary, never through
// the code under test.
func gitRev(t *testing.T, repo, rev string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", rev).CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse %s: %v: %s", rev, err, out)
	}
	return strings.TrimSpace(string(out))
}

// changeRowCount reads the accumulated change rows: a refusal that never
// evaluated source must leave none behind.
func changeRowCount(t *testing.T, repo string) int {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM changes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

type ciDenial struct {
	Code string `json:"code"`
	Key  string `json:"key"`
}

func ciDenials(t *testing.T, repo string, args ...string) (int, []ciDenial) {
	t.Helper()
	got := run(t, repo, args...)
	var data struct {
		Allow   bool       `json:"allow"`
		Denials []ciDenial `json:"denials"`
	}
	decodeData(t, got.stdout, &data)
	return got.code, data.Denials
}

func denialCodes(denials []ciDenial) map[string]int {
	out := map[string]int{}
	for _, denial := range denials {
		out[denial.Code+"\x00"+denial.Key]++
	}
	return out
}

// TestVerifyModesAreExclusive pins decision D-221: the index and a
// committed range are different things to judge, so both flags together —
// and stray revs without --ci — refuse with usage.
func TestVerifyModesAreExclusive(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "README.md", "# docs\n")
	gitCommitFile(t, repo, "README.md")
	// The commit exists so that --ci alone would verify green: the
	// refusal below can only come from the exclusivity guard, never
	// from an unresolvable range.
	got := run(t, repo, "verify", "--staged", "--ci", "--json")
	got.requireExit(t, app.ExitUsage)

	rev := run(t, repo, "verify", "--staged", "--base", "HEAD", "--json")
	rev.requireExit(t, app.ExitUsage)
	if payload := rev.errorPayload(t); payload.Code != app.CodeCommandLineInvalid {
		t.Fatalf("code = %q, want COMMAND_LINE_INVALID", payload.Code)
	}
}

// TestVerifyCICleanGreen is TASK-01 AC-01.1's quiet half: an empty range
// (base == head) verifies green with empty denials.
func TestVerifyCICleanGreen(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "README.md", "# docs\n")
	gitCommitFile(t, repo, "README.md")
	head := gitRev(t, repo, "HEAD")

	code, denials := ciDenials(t, repo, "verify", "--ci", "--base", head, "--head", head, "--json")
	if code != app.ExitSuccess || len(denials) != 0 {
		t.Fatalf("code = %d, denials = %+v, want green", code, denials)
	}
}

// TestVerifyCIUnregisteredDenies is TASK-01 AC-01.1: a committed file no
// baseline covers denies with the attribution code, like the staged path.
func TestVerifyCIUnregisteredDenies(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	base := gitRev(t, repo, "HEAD")
	gitCommitFile(t, repo, "pkg/a.py")

	code, denials := ciDenials(t, repo, "verify", "--ci", "--base", base, "--json")
	if code != app.ExitFailed {
		t.Fatalf("code = %d, want %d (denials %+v)", code, app.ExitFailed, denials)
	}
	found := false
	for _, denial := range denials {
		if denial.Code == string(app.CodeUnregisteredChange) {
			found = true
		}
	}
	if !found {
		t.Fatalf("denials = %+v, want the attribution code", denials)
	}
}

// TestVerifyCIParityWithStaged is TASK-01 AC-01.5: the same content denied
// through --staged denies through --ci with the identical code set. The
// guard trigger string names the path by construction ("staged" vs "ci"),
// so the comparison is over code + key, never the trigger word.
func TestVerifyCIParityWithStaged(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, repo, "pkg/b.py", "def other():\n    return 2\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	base := gitRev(t, repo, "HEAD")
	gitStageFile(t, repo, "pkg/a.py")
	gitStageFile(t, repo, "pkg/b.py")

	_, staged := ciDenials(t, repo, "verify", "--staged", "--json")

	// One commit carries both files: the staged index above already held
	// the same content, so the range judges exactly what staging judged.
	gitCommitFile(t, repo, "pkg/a.py")

	code, ranged := ciDenials(t, repo, "verify", "--ci", "--base", base, "--json")
	if code != app.ExitFailed {
		t.Fatalf("code = %d, want denial (denials %+v)", code, ranged)
	}
	want, got := denialCodes(staged), denialCodes(ranged)
	if len(want) == 0 {
		t.Fatalf("staged run denied nothing; parity is vacuous (ranged %+v)", ranged)
	}
	for key := range want {
		if got[key] == 0 {
			t.Fatalf("staged denial %q missing from CI denials %+v", key, ranged)
		}
	}
	for key := range got {
		if want[key] == 0 {
			t.Fatalf("CI denial %q has no staged counterpart (staged %+v)", key, staged)
		}
	}
}

// TestVerifyCIBadBaseRefuses is TASK-01 AC-01.2: an unresolvable base
// refuses with the usage code and writes no change rows — the range was
// never named, so nothing about it was certified.
func TestVerifyCIBadBaseRefuses(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	gitCommitFile(t, repo, "pkg/a.py")

	got := run(t, repo, "verify", "--ci", "--base", "no-such-base", "--json")
	got.requireExit(t, app.ExitUsage)
	if payload := got.errorPayload(t); payload.Code != app.CodeCommandLineInvalid {
		t.Fatalf("code = %q, want COMMAND_LINE_INVALID", payload.Code)
	}
	if n := changeRowCount(t, repo); n != 0 {
		t.Fatalf("changes = %d, want 0: refusal evaluated source", n)
	}
}

// TestVerifyCIUnbornHeadRefuses pins the same rule at the other end: with
// no commit at all there is no head to judge.
func TestVerifyCIUnbornHeadRefuses(t *testing.T) {
	repo := newInitializedRepo(t)
	got := run(t, repo, "verify", "--ci", "--json")
	got.requireExit(t, app.ExitUsage)
}

// TestVerifyCIKnowledgeFirst is TASK-01 AC-01.4: fatal knowledge refuses
// before rev resolution — a bogus --base alongside corrupt knowledge still
// reports the knowledge failure, and no change row is written.
func TestVerifyCIKnowledgeFirst(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	gitCommitFile(t, repo, "pkg/a.py")
	writeKnowledgeRecord(t, repo, "DEC-0001.json",
		`{"schema_version":999,"kind":"decision","id":"DEC-0001","status":"active",`+
			`"created_at":"2026-09-23T10:00:00Z","title":"Future","decision":"Later."}`)

	got := run(t, repo, "verify", "--ci", "--base", "no-such-base", "--json")
	if got.code == app.ExitSuccess {
		t.Fatalf("verify passed over fatal knowledge: %s", got.stdout)
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
	if n := changeRowCount(t, repo); n != 0 {
		t.Fatalf("changes = %d, want 0: knowledge-first violated", n)
	}
}

// TestVerifyCIDefaultBase pins decision D-223's quiet half: with no --base,
// the documented chain resolves the branch the fixture committed on.
func TestVerifyCIDefaultBase(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "README.md", "# docs\n")
	gitCommitFile(t, repo, "README.md")
	if out, err := exec.Command("git", "-C", repo, "branch", "-f", "main", "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("branch main: %v: %s", err, out)
	}

	code, denials := ciDenials(t, repo, "verify", "--ci", "--json")
	if code != app.ExitSuccess || len(denials) != 0 {
		t.Fatalf("code = %d, denials = %+v, want green", code, denials)
	}
}

// TestVerifyCIGuardDenies is the CI half of the AC-34 story: a weakened
// CRITICAL test committed on head denies blocking with base-vs-head bytes.
func TestVerifyCIGuardDenies(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/test_a.py",
		"def helper():\n    return 1\n\ndef test_helper():\n    assert helper()\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	gitCommitFile(t, repo, "pkg/test_a.py")
	base := gitRev(t, repo, "HEAD")
	registerTestUnit(t, repo, "pkg")
	writeRepoFile(t, repo, "pkg/test_a.py", "def helper():\n    return 1\n")
	gitCommitFile(t, repo, "pkg/test_a.py")

	// First pass populates the index (helper uid); the guard beat seeds
	// binding + reference on top, like the staged two-pass test.
	run(t, repo, "verify", "--ci", "--base", base, "--json")
	seedGuardBinding(t, repo, "pkg/test_a.py")

	code, denials := ciDenials(t, repo, "verify", "--ci", "--base", base, "--json")
	if code != app.ExitFailed {
		t.Fatalf("code = %d, want denial (denials %+v)", code, denials)
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

// TestVerifyCIDefaultBaseNamesCandidates pins the loud half of D-223: with
// no candidate branch, the refusal names what was tried.
func TestVerifyCIDefaultBaseNamesCandidates(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "README.md", "# docs\n")
	gitCommitFile(t, repo, "README.md")
	// Rename the only branch away: no candidate of the default chain may
	// resolve afterwards.
	if out, err := exec.Command("git", "-C", repo, "branch", "-m", "lonely").CombinedOutput(); err != nil {
		t.Fatalf("branch -m lonely: %v: %s", err, out)
	}

	got := run(t, repo, "verify", "--ci", "--json")
	got.requireExit(t, app.ExitUsage)
	if !strings.Contains(got.stdout, "origin/main") {
		t.Fatalf("refusal names no candidates: %s", got.stdout)
	}
}
