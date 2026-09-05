package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/status"
)

// restoreWritePrefix is the one spelling the write-refusal remedy uses to
// introduce the paths it names. The tests below carry the remedy out rather
// than assert its wording, and this is the seam they read it through.
const restoreWritePrefix = "restore write permission on "

// TestAnUnwritableRuntimeDatabaseIsNotReportedHealthy is finding W5.
//
// A mindrail.db whose mode bits refuse writes opens perfectly. Every probe in
// MR-001 stopped there: `doctor` read the pragmas back, found WAL and foreign
// keys in force, printed `Overall: OK` and exited 0 — while `mindrail init` in
// the same directory failed forever on `attempt to write a readonly database
// (8)`, and its remedy sent the user to that doctor and back to init. A closed
// loop over an installation the tool declared healthy, and the same defect class
// as the already-closed H2 and F9.
//
// The test asserts both halves, because either alone is satisfiable by a lie.
// Reporting ERROR is worthless if the remedy cannot be carried out, so the
// remedy the report actually printed is executed here — the paths are parsed out
// of the emitted next_action, not chosen by the test — and the repository has to
// come back to a working `mindrail init` and a clean `mindrail doctor`.
func TestAnUnwritableRuntimeDatabaseIsNotReportedHealthy(t *testing.T) {
	repo := newInitializedRepo(t)
	chmodForTest(t, runtimeDBPath(t, repo), 0o444)

	remedy := assertReportsAnUnwritableDatabase(t, repo)
	carryOutWriteRemedy(t, remedy)
	assertRepositoryWorksAgain(t, repo)
}

// TestAnUnwritableSharedMemoryIndexIsNotReportedHealthy is the second shape of
// W5, and the one a probe that stopped at the database file's own mode would
// miss.
//
// SQLite maps `mindrail.db-shm` read-write. When its mode refuses writes every
// write fails with SQLITE_READONLY while the database file's own mode is
// perfectly correct — so a report that inspected only mindrail.db would print
// `Overall: OK` over exactly the condition it was added to catch. It is also how
// the remedy for the first shape can fail halfway: a `doctor` run against a
// read-only database leaves a `-shm` behind carrying the same mode, and a chmod
// of mindrail.db alone is followed by a re-run that fails on the file the first
// run created.
//
// The state is reached the way a user reaches it, not by construction: the
// database is made read-only, a report is read, and that report's own run leaves
// a `-shm` behind at the database file's mode. Restoring write permission on
// mindrail.db alone then leaves the repository broken by a file the diagnosis
// itself created — which is why the remedy has to name the whole footprint, and
// why this test only restores the one path a narrower remedy would have named.
func TestAnUnwritableSharedMemoryIndexIsNotReportedHealthy(t *testing.T) {
	repo := newInitializedRepo(t)
	db := runtimeDBPath(t, repo)
	chmodForTest(t, db, 0o444)

	remedy := assertReportsAnUnwritableDatabase(t, repo)
	if !slices.ContainsFunc(remedy, func(path string) bool { return strings.HasSuffix(path, "-shm") }) {
		t.Skipf("this run left no shared-memory index behind, so the second shape is not reachable here: %v", remedy)
	}

	// The half-remedy: mindrail.db only, exactly what a report that inspected
	// the database file alone would have told the reader to do.
	carryOutWriteRemedy(t, []string{db})
	if initOut := run(t, repo, "init"); initOut.code == app.ExitSuccess {
		t.Fatalf("restoring mindrail.db alone was enough, so the shared-memory index refuses nothing:\n%s",
			initOut.stdout)
	}

	shmRemedy := assertReportsAnUnwritableDatabase(t, repo)
	if !slices.ContainsFunc(shmRemedy, func(path string) bool { return strings.HasSuffix(path, "-shm") }) {
		t.Errorf("the remedy does not name the shared-memory index that refused the write: %v", shmRemedy)
	}
	carryOutWriteRemedy(t, shmRemedy)
	assertRepositoryWorksAgain(t, repo)
}

// TestAReadOnlyWriteAheadLogIsNotReportedUnwritable is the first over-fire
// guard, and it is the adjacent condition the new detection must not swallow.
//
// A `-wal` whose mode refuses writes costs nothing: SQLite brings its mode into
// line with the database file's and writes through it. Claiming it as a refusal
// would fail a repository on which every command still works — the mistake a
// pass-2 fix made when an unwritable runtime path also fired on the cache
// directory and bricked a working repository. The assertion is in both
// directions: the report stays healthy, and the writes it says are possible
// really are.
func TestAReadOnlyWriteAheadLogIsNotReportedUnwritable(t *testing.T) {
	repo := newInitializedRepo(t)
	wal := runtimeDBPath(t, repo) + "-wal"
	if _, err := os.Stat(wal); os.IsNotExist(err) {
		writeFile(t, wal, nil)
	}
	chmodForTest(t, wal, 0o444)

	assertRepositoryWorksAgain(t, repo)
}

// TestAnUnwritableCacheDirectoryIsNotAnUnwritableDatabase is the second
// over-fire guard.
//
// The cache directory is outside the runtime database's footprint and MR-001
// stores nothing in it: `mindrail init` still succeeds and every command still
// runs. Letting it decide the database's verdict is finding F13 — a fully
// working repository driven to ERROR, BLOCKED and exit 4 — wearing the new
// probe's name, so the database reading must be unmoved by it.
func TestAnUnwritableCacheDirectoryIsNotAnUnwritableDatabase(t *testing.T) {
	repo := newInitializedRepo(t)
	denyWrites(t, filepath.Join(repo, ".git", "mindrail", "cache"))

	got := run(t, repo, "doctor", "--json")
	got.requireExit(t, app.ExitSuccess)

	var health doctor.Report
	decodeData(t, got.stdout, &health)

	sqlite := checkNamed(t, health, "sqlite")
	if sqlite.State != doctor.StateOK {
		t.Errorf("sqlite check is %s over an unusable cache directory: %+v", sqlite.State, sqlite)
	}
	if sqlite.Details["db_writable"] != "true" {
		t.Errorf("db_writable = %q, want \"true\": the cache directory is not part of the database's footprint",
			sqlite.Details["db_writable"])
	}
	if initOut := run(t, repo, "init"); initOut.code != app.ExitSuccess {
		t.Fatalf("`%s` exited %d over an unusable cache directory: %v\n%s",
			initRemedy, initOut.code, initOut.err, initOut.stdout)
	}
}

// TestAHealthyDatabaseReportsThatItsWritabilityWasChecked is the mutation guard
// the W5 family kept escaping through.
//
// Every reading before this one asserted that the database *opened*. None of them
// could tell an installation whose writability was checked and found good from
// one where nobody asked — which is precisely why deleting the question left the
// suite green for four audits. The published answer is the evidence that the
// check looked, so it is asserted on a healthy repository too, not only on a
// broken one.
func TestAHealthyDatabaseReportsThatItsWritabilityWasChecked(t *testing.T) {
	repo := newInitializedRepo(t)

	got := run(t, repo, "doctor", "--json")
	got.requireExit(t, app.ExitSuccess)

	var health doctor.Report
	decodeData(t, got.stdout, &health)

	sqlite := checkNamed(t, health, "sqlite")
	writable, published := sqlite.Details["db_writable"]
	if !published {
		t.Fatalf("the sqlite check publishes no db_writable: a reading that never asked whether the "+
			"database can be written must not report OK. details = %v", sqlite.Details)
	}
	if writable != "true" {
		t.Errorf("db_writable = %q on a healthy repository, want \"true\"", writable)
	}
}

// assertReportsAnUnwritableDatabase runs both reporting commands against a
// repository whose database refuses writes and returns the paths the remedy
// named, so the caller can carry it out.
//
// Both commands are checked in one place because the defect was a disagreement
// between them and `mindrail init`: whatever else changes, they may not describe
// this repository as workable.
func assertReportsAnUnwritableDatabase(t *testing.T, repo string) []string {
	t.Helper()

	health := run(t, repo, "doctor", "--json")
	health.requireExit(t, app.ExitUnavailable)

	var report doctor.Report
	decodeData(t, health.stdout, &report)
	if report.WorstState != doctor.StateError {
		t.Errorf("doctor worst_state = %q, want %q: the database cannot be written",
			report.WorstState, doctor.StateError)
	}
	sqlite := checkNamed(t, report, "sqlite")
	if sqlite.State != doctor.StateError {
		t.Errorf("sqlite check = %q, want %q: opened successfully is not usable", sqlite.State, doctor.StateError)
	}
	if sqlite.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("sqlite code = %q, want %q", sqlite.Code, app.CodeRuntimePathUnwritable)
	}
	if sqlite.Details["db_writable"] != "false" {
		t.Errorf("db_writable = %q, want \"false\"", sqlite.Details["db_writable"])
	}

	state := run(t, repo, "status", "--json")
	state.requireExit(t, app.ExitUnavailable)

	var readiness status.Report
	decodeData(t, state.stdout, &readiness)
	if readiness.Readiness != status.ReadinessBlocked {
		t.Errorf("status readiness = %q, want %q", readiness.Readiness, status.ReadinessBlocked)
	}
	if readiness.BlockingComponent != status.ComponentRuntimeDB {
		t.Errorf("blocking component = %q, want %q", readiness.BlockingComponent, status.ComponentRuntimeDB)
	}
	component := readiness.Components[status.ComponentRuntimeDB]
	if component.State != doctor.StateError {
		t.Errorf("runtime_db = %q, want %q", component.State, doctor.StateError)
	}

	// The `mindrail init` this report is about really does fail here. Without
	// this the test would pass against a report that had simply become
	// pessimistic, which is the other half of the same defect.
	if initOut := run(t, repo, "init"); initOut.code == app.ExitSuccess {
		t.Fatalf("`%s` succeeded, so there is no unwritable database to report:\n%s", initRemedy, initOut.stdout)
	}

	return writeRemedyPaths(t, sqlite.NextAction)
}

// writeRemedyPaths reads the paths out of the remedy the report printed.
//
// The test carries out what the tool actually said rather than what it ought to
// have said. A remedy that stops naming one of the files that refuses a write
// leaves that file untouched here, and the `mindrail init` that follows fails —
// which is the failure a user would hit, reproduced.
func writeRemedyPaths(t *testing.T, actions []string) []string {
	t.Helper()

	for _, action := range actions {
		_, named, found := strings.Cut(action, restoreWritePrefix)
		if !found {
			continue
		}
		paths := strings.Split(strings.TrimSuffix(named, "."), ", ")
		if len(paths) == 0 || paths[0] == "" {
			t.Fatalf("the remedy names no path to restore: %q", action)
		}
		return paths
	}

	t.Fatalf("no remedy tells the reader how to restore write access: %v", actions)
	return nil
}

// carryOutWriteRemedy executes the printed remedy. A remedy that cannot be
// carried out is not a remedy, and this is the step that finds out.
func carryOutWriteRemedy(t *testing.T, paths []string) {
	t.Helper()

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("the remedy names %s, which cannot be inspected: %v", path, err)
		}
		if err := os.Chmod(path, info.Mode().Perm()|0o200); err != nil {
			t.Fatalf("restore write permission on %s: %v", path, err)
		}
	}
}

// assertRepositoryWorksAgain is the proof the loop is closed: `mindrail init`
// succeeds, `mindrail doctor` is clean and `mindrail status` is READY.
func assertRepositoryWorksAgain(t *testing.T, repo string) {
	t.Helper()

	if initOut := run(t, repo, "init"); initOut.code != app.ExitSuccess {
		t.Fatalf("`%s` still exits %d after the remedy was carried out: %v\n%s",
			initRemedy, initOut.code, initOut.err, initOut.stdout)
	}

	health := run(t, repo, "doctor", "--json")
	health.requireExit(t, app.ExitSuccess)

	var report doctor.Report
	decodeData(t, health.stdout, &report)
	if report.WorstState != doctor.StateOK {
		t.Errorf("doctor worst_state = %q after the remedy, want %q", report.WorstState, doctor.StateOK)
	}

	state := run(t, repo, "status", "--json")
	state.requireExit(t, app.ExitSuccess)

	var readiness status.Report
	decodeData(t, state.stdout, &readiness)
	if readiness.Readiness != status.ReadinessReady {
		t.Errorf("status readiness = %q after the remedy, want %q", readiness.Readiness, status.ReadinessReady)
	}
}

// checkNamed pulls one reading out of a doctor report by name.
func checkNamed(t *testing.T, report doctor.Report, name string) doctor.Result {
	t.Helper()

	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("doctor report has no %q check: %+v", name, report.Checks)
	return doctor.Result{}
}

// TestANonUTF8RepositoryPathIsRefusedRatherThanMangled is finding W9, end to
// end.
//
// A repository path is a sequence of bytes on every Unix and nothing requires
// them to be valid UTF-8. `--json` used to publish such a path with each invalid
// byte replaced by U+FFFD: well-formed JSON stating a location that does not
// exist on disk, which a consumer cannot stat, open or hand back to git. The
// answer taken here is the refusal — the document is not published at all —
// because any lossless escaping still changes the bytes a consumer reads back
// and so keeps lying to every consumer that has not been told about the scheme.
//
// The refusal is only honest if it leaves the reader somewhere to go, so the
// remedy it prints is carried out: the human report renders the same repository
// with the bytes untouched.
func TestANonUTF8RepositoryPathIsRefusedRatherThanMangled(t *testing.T) {
	repo := newRepoUnderAnUnrepresentablePath(t)

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitUsage)

	var envelope struct {
		OK    bool              `json:"ok"`
		Data  json.RawMessage   `json:"data"`
		Error *app.ErrorPayload `json:"error"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &envelope); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, got.stdout)
	}
	if envelope.Error == nil || envelope.Error.Code != app.CodePathNotRepresentable {
		t.Fatalf("error = %+v, want code %q", envelope.Error, app.CodePathNotRepresentable)
	}
	if len(envelope.Data) != 0 && string(envelope.Data) != "null" {
		t.Errorf("the refused report was published anyway: %s", envelope.Data)
	}
	if strings.ContainsRune(got.stdout, '�') {
		t.Errorf("the envelope still carries a mangled value:\n%s", got.stdout)
	}

	// The remedy the report printed, carried out.
	human := run(t, repo, "status")
	human.requireExit(t, app.ExitSuccess)
	if !strings.Contains(human.stdout, repo) {
		t.Errorf("the human report does not carry the repository path through unchanged:\n%s", human.stdout)
	}
}

// TestAValidUTF8RepositoryPathIsNotRefused is the over-fire guard for the
// refusal at the command boundary.
//
// A repository whose path is non-ASCII but valid is ordinary, and `--json` has
// to keep working there — refusing it would break machine consumption for a
// large part of the world over a defect that is not present.
func TestAValidUTF8RepositoryPathIsNotRefused(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "проект-café-日本語")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Skipf("this filesystem will not hold a non-ASCII directory name: %v", err)
	}
	initGitRepo(t, repo)

	if got := run(t, repo, "init"); got.code != app.ExitSuccess {
		t.Fatalf("init exited %d under a non-ASCII path: %v\n%s", got.code, got.err, got.stdout)
	}

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)
	assertNoErrorEnvelope(t, "status", got.stdout)

	var report status.Report
	decodeData(t, got.stdout, &report)
	if report.Repository.WorktreeRoot != repo {
		t.Errorf("worktree_root = %q, want %q", report.Repository.WorktreeRoot, repo)
	}
}

// newRepoUnderAnUnrepresentablePath builds a repository whose directory name
// holds a byte no valid UTF-8 string contains. Filesystems that refuse the name
// simply cannot reach the condition, and the test says so rather than passing
// vacuously.
func newRepoUnderAnUnrepresentablePath(t *testing.T) string {
	t.Helper()

	parent := t.TempDir()
	repo := filepath.Join(parent, "repo\xff name")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Skipf("this filesystem will not hold a non-UTF-8 directory name: %v", err)
	}
	initGitRepo(t, repo)
	return repo
}

// initGitRepo makes dir a Git repository with the identity the suite's other
// fixtures use. It exists because the two tests above have to choose the
// directory *name* — that is the condition under test — and so cannot go through
// newRepo, which owns the name it creates.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	requireGit(t)
	isolateEnvironment(t)

	for _, args := range [][]string{
		{"init", "--quiet", dir},
		{"-C", dir, "config", "user.email", "test@example.invalid"},
		{"-C", dir, "config", "user.name", "Mindrail Test"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}
