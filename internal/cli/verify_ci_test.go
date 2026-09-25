package cli_test

import (
	"os"
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

// changeFileCount reads the reconciled file rows: a green verdict over an
// allegedly non-empty range must have judged files, not an empty diff.
func changeFileCount(t *testing.T, repo string) int {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM change_files`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// removeDB deletes the runtime database so the next command starts from
// no index state, the way a fresh clone does.
func removeDB(t *testing.T, repo string) error {
	t.Helper()
	return os.Remove(runtimeDBPath(t, repo))
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

	// Fresh database between the modes: symbol rows are deltas against
	// index state, so judging the same content twice on one database
	// would let the first run's indexing hide the second run's rows.
	// Local and CI verdicts always come from different databases in
	// real use; the reset reproduces that instead of the artifact.
	gitCommitFile(t, repo, "pkg/a.py")
	if err := removeDB(t, repo); err != nil {
		t.Fatal(err)
	}
	if got := run(t, repo, "init"); got.code != app.ExitSuccess {
		t.Fatalf("re-init exited %d: %v\n%s", got.code, got.err, got.stdout)
	}

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

// TestVerifyCIDefaultEmptyRefuses pins the default-empty rule: a default
// base that equals head judges nothing, and judging nothing by default
// would certify a main-tip bypass green. Explicit equal revs stay green
// (caller's choice); a default that finds nothing refuses loudly.
func TestVerifyCIDefaultEmptyRefuses(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "README.md", "# docs\n")
	gitCommitFile(t, repo, "README.md")
	if out, err := exec.Command("git", "-C", repo, "branch", "-f", "main", "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("branch main: %v: %s", err, out)
	}

	got := run(t, repo, "verify", "--ci", "--json")
	got.requireExit(t, app.ExitUsage)
	if payload := got.errorPayload(t); payload.Code != app.CodeCommandLineInvalid {
		t.Fatalf("code = %q, want COMMAND_LINE_INVALID", payload.Code)
	}
}

// TestVerifyCIDefaultBaseBehind pins decision D-223's working half: with
// the default chain resolving to a base behind head, bare --ci judges
// the range.
func TestVerifyCIDefaultBaseBehind(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "README.md", "# docs\n")
	gitCommitFile(t, repo, "README.md")
	if out, err := exec.Command("git", "-C", repo, "branch", "-f", "main", "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("branch main: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", repo, "checkout", "--quiet", "-b", "work").CombinedOutput(); err != nil {
		t.Fatalf("checkout -b work: %v: %s", err, out)
	}
	writeRepoFile(t, repo, "docs/notes.md", "# notes\n")
	gitCommitFile(t, repo, "docs/notes.md")

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
	// The binding's knowledge record travels committed, like code: the
	// desk stays clean and head stays put across both passes below, so
	// the second pass re-judges the same range (same change, rows
	// accumulate) instead of a moved one.
	writeGuardInvariant(t, repo)
	gitCommitFile(t, repo, ".mindrail/knowledge/invariants/INV-G.json")
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

// TestVerifyCIInvertedRangeRefuses pins the Breaker B1 fix: a base that is
// a descendant of head judges an empty range, and an empty range must
// never certify green — it refuses with usage instead.
func TestVerifyCIInvertedRangeRefuses(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	gitCommitFile(t, repo, "pkg/a.py")
	ahead := gitRev(t, repo, "HEAD")
	head := gitRev(t, repo, "HEAD~1")
	// Check out the judged head: the worktree gate must pass so that only
	// the inversion guard can produce the refusal below.
	if out, err := exec.Command("git", "-C", repo, "checkout", "--quiet", head).CombinedOutput(); err != nil {
		t.Fatalf("checkout head: %v: %s", err, out)
	}

	got := run(t, repo, "verify", "--ci", "--base", ahead, "--head", head, "--json")
	got.requireExit(t, app.ExitUsage)
	if payload := got.errorPayload(t); payload.Code != app.CodeCommandLineInvalid {
		t.Fatalf("code = %q, want COMMAND_LINE_INVALID", payload.Code)
	}
	if n := changeRowCount(t, repo); n != 0 {
		t.Fatalf("changes = %d, want 0: refusal evaluated source", n)
	}
}

// TestVerifyCIWorktreeMismatchRefuses pins the Breaker B3 fix: the desk
// must be a checkout of the judged head, because indexing reads desk
// bytes. A head the checkout does not have refuses instead of crashing
// on missing files.
func TestVerifyCIWorktreeMismatchRefuses(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	base := gitRev(t, repo, "HEAD")
	gitCommitFile(t, repo, "pkg/a.py")
	head := gitRev(t, repo, "HEAD")
	if out, err := exec.Command("git", "-C", repo, "checkout", "--quiet", base).CombinedOutput(); err != nil {
		t.Fatalf("checkout base: %v: %s", err, out)
	}

	got := run(t, repo, "verify", "--ci", "--base", base, "--head", head, "--json")
	got.requireExit(t, app.ExitUsage)
	if !strings.Contains(got.stdout, head[:7]) {
		t.Fatalf("refusal names no judged head: %s", got.stdout)
	}
}

// TestVerifyCIDirtyDeskRefuses pins the Breaker B5 fix: uncommitted edits
// would have the verdict certify bytes it never judged, so a dirty desk
// refuses with the paths named.
func TestVerifyCIDirtyDeskRefuses(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	base := gitRev(t, repo, "HEAD")
	gitCommitFile(t, repo, "pkg/a.py")
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 2\n")

	got := run(t, repo, "verify", "--ci", "--base", base, "--json")
	got.requireExit(t, app.ExitUsage)
	if !strings.Contains(got.stdout, "pkg/a.py") {
		t.Fatalf("refusal names no dirty path: %s", got.stdout)
	}
}

// TestVerifyCIRangeScoping pins the Breaker B4 fix in the forward
// direction: content that was staged (and judged) but never committed
// must not ride a later CI verdict.
func TestVerifyCIRangeScoping(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	base := gitRev(t, repo, "HEAD")

	writeRepoFile(t, repo, "pkg/staged.py", "def staged_one():\n    return 1\n")
	gitStageFile(t, repo, "pkg/staged.py")
	run(t, repo, "verify", "--staged", "--json")
	if out, err := exec.Command("git", "-C", repo, "rm", "--quiet", "--cached", "pkg/staged.py").CombinedOutput(); err != nil {
		t.Fatalf("unstage: %v: %s", err, out)
	}
	if err := os.Remove(repo + "/pkg/staged.py"); err != nil {
		t.Fatal(err)
	}

	writeRepoFile(t, repo, "pkg/ci.py", "def ci_one():\n    return 2\n")
	gitCommitFile(t, repo, "pkg/ci.py")

	code, denials := ciDenials(t, repo, "verify", "--ci", "--base", base, "--json")
	if code != app.ExitFailed {
		t.Fatalf("code = %d, want denial (denials %+v)", code, denials)
	}
	for _, denial := range denials {
		if strings.Contains(denial.Key, "staged") {
			t.Fatalf("CI verdict judges uncommitted content: %+v", denials)
		}
	}
	found := false
	for _, denial := range denials {
		if strings.Contains(denial.Key, "ci_one") || strings.Contains(denial.Key, "ci.py") {
			found = true
		}
	}
	if !found {
		t.Fatalf("CI verdict misses its own range: %+v", denials)
	}
}

// TestVerifyStagedIgnoresCIRows pins the Breaker B4 fix in the reverse
// direction: committed content a CI run judged must not ride a later
// staged verdict.
func TestVerifyStagedIgnoresCIRows(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	base := gitRev(t, repo, "HEAD")
	writeRepoFile(t, repo, "pkg/ci.py", "def ci_one():\n    return 2\n")
	gitCommitFile(t, repo, "pkg/ci.py")
	run(t, repo, "verify", "--ci", "--base", base, "--json")

	writeRepoFile(t, repo, "pkg/staged.py", "def staged_one():\n    return 1\n")
	gitStageFile(t, repo, "pkg/staged.py")

	code, denials := ciDenials(t, repo, "verify", "--staged", "--json")
	if code != app.ExitFailed {
		t.Fatalf("code = %d, want denial (denials %+v)", code, denials)
	}
	for _, denial := range denials {
		if strings.Contains(denial.Key, "ci_one") || (strings.Contains(denial.Key, "ci.py") && !strings.Contains(denial.Key, "staged")) {
			t.Fatalf("staged verdict judges committed-only content: %+v", denials)
		}
	}
}

// TestVerifyCIKnowledgeDirtRefuses pins the N1/F1 fix: a committed-fatal
// knowledge record deleted uncommitted in the desk refuses — the verdict
// may not certify the committed range against desk-fixed knowledge.
func TestVerifyCIKnowledgeDirtRefuses(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	writeKnowledgeRecord(t, repo, "DEC-0001.json",
		`{"schema_version":999,"kind":"decision","id":"DEC-0001","status":"active",`+
			`"created_at":"2026-09-23T10:00:00Z","title":"Future","decision":"Later."}`)
	gitCommitFile(t, repo, "pkg/a.py")
	if out, err := exec.Command("git", "-C", repo, "add",
		".mindrail/knowledge/decisions/DEC-0001.json").CombinedOutput(); err != nil {
		t.Fatalf("add knowledge: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", repo, "commit", "--quiet", "-m", "fatal knowledge").CombinedOutput(); err != nil {
		t.Fatalf("commit knowledge: %v: %s", err, out)
	}
	base := gitRev(t, repo, "HEAD~1")
	if err := os.Remove(repo + "/.mindrail/knowledge/decisions/DEC-0001.json"); err != nil {
		t.Fatal(err)
	}

	got := run(t, repo, "verify", "--ci", "--base", base, "--json")
	got.requireExit(t, app.ExitUsage)
	if !strings.Contains(got.stdout, "DEC-0001.json") {
		t.Fatalf("refusal names no knowledge path: %s", got.stdout)
	}
}

// TestVerifyCIKnowledgeAddedDirtRefuses pins the reverse half: committed-
// clean knowledge plus a desk-added fatal record refuses as well —
// through the knowledge-first gate, which reads the same desk bytes and
// fails closed before the range is ever named.
func TestVerifyCIKnowledgeAddedDirtRefuses(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "pkg/a.py", "def helper():\n    return 1\n")
	writeRepoFile(t, repo, "pkg/pyproject.toml", "[project]\n")
	gitCommitFile(t, repo, "pkg/pyproject.toml")
	gitCommitFile(t, repo, "pkg/a.py")
	base := gitRev(t, repo, "HEAD~1")
	writeKnowledgeRecord(t, repo, "DEC-0009.json",
		`{"schema_version":999,"kind":"decision","id":"DEC-0009","status":"active",`+
			`"created_at":"2026-09-23T10:00:00Z","title":"Future","decision":"Later."}`)

	got := run(t, repo, "verify", "--ci", "--base", base, "--json")
	got.requireExit(t, app.ExitFailed)
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
}

// TestVerifyCIUnrelatedBasesRefuse is the CLI half of the merge-base
// story (Reader F2): two roots share no history, so there is no range to
// judge — structured refusal, never empty-green.
func TestVerifyCIUnrelatedBasesRefuse(t *testing.T) {
	repo := newInitializedRepo(t)
	writeRepoFile(t, repo, "README.md", "# docs\n")
	gitCommitFile(t, repo, "README.md")
	out, err := exec.Command("git", "-C", repo, "branch", "--show-current").CombinedOutput()
	if err != nil {
		t.Fatalf("current branch: %v: %s", err, out)
	}
	first := strings.TrimSpace(string(out))
	if out, err := exec.Command("git", "-C", repo, "checkout", "--quiet", "--orphan", "other").CombinedOutput(); err != nil {
		t.Fatalf("orphan: %v: %s", err, out)
	}
	writeRepoFile(t, repo, "OTHER.md", "# other\n")
	gitCommitFile(t, repo, "OTHER.md")
	// Judge from the first root's checkout: the worktree gate must pass
	// so that only the merge-base failure can produce the refusal.
	if out, err := exec.Command("git", "-C", repo, "checkout", "--quiet", first).CombinedOutput(); err != nil {
		t.Fatalf("checkout %s: %v: %s", first, err, out)
	}

	got := run(t, repo, "verify", "--ci", "--base", "other", "--head", first, "--json")
	if got.code == app.ExitSuccess {
		t.Fatalf("unrelated histories verified: %s", got.stdout)
	}
	got.requireExit(t, app.ExitUsage)
}
