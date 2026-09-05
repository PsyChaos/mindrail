package cli_test

import (
	"database/sql"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/status"
)

// initRemedy is the one command MR-001 tells a user to run. It is spelled once
// here so the invariant and the over-fire guards below cannot drift apart from
// each other or from doctor's own constant.
const initRemedy = "mindrail init"

// coherenceSetup is one repository state, rendered by every command that
// reports on it.
type coherenceSetup struct {
	name  string
	setup func(t *testing.T) string
}

// coherenceSetups is the matrix the invariant runs over.
//
// The first four are conditions `mindrail init` cannot clear, which is the half
// where a contradiction costs the reader a wasted command and their trust in the
// report. The last three are the over-fire guards: a healthy repository, a
// repository whose only problem is that init has not run yet, and the adjacent
// condition — an unusable *cache* directory — that must never be read as an
// obstruction, because grading it like the runtime root is what bricked a
// working repository once already (finding F13).
func coherenceSetups() []coherenceSetup {
	return []coherenceSetup{
		{
			name: "runtime directory unwritable after init",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyWrites(t, filepath.Join(repo, ".git", "mindrail"))
				return repo
			},
		},
		{
			name: "runtime directory unwritable before init",
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				denyWrites(t, filepath.Join(repo, ".git"))
				return repo
			},
		},
		{
			name: "database path occupied by a directory",
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				if err := os.MkdirAll(runtimeDBPath(t, repo), 0o755); err != nil {
					t.Fatalf("occupy the database path: %v", err)
				}
				return repo
			},
		},
		{
			name: "runtime database corrupt",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				corruptDatabase(t, repo)
				return repo
			},
		},
		{
			// Finding W6. The matrix reached the runtime root through its mode
			// bits and through the database path's contents, and never through a
			// regular file standing where the root itself should be — so the
			// disagreement this invariant exists to catch went on being produced
			// by a condition it simply never ran over. Every row here is a
			// condition, and a condition nobody set up is a condition nobody
			// checked.
			name: "regular file occupying the runtime root",
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				writeFile(t, filepath.Join(repo, ".git", "mindrail"), []byte("not a directory"))
				return repo
			},
		},
		{
			// Finding W5's condition, added here as well as to its own test: the
			// document it produces has an error object and four readings of the
			// runtime store, and they have to agree about whether `mindrail init`
			// is worth running. It is not — the write it makes is the one that
			// fails.
			name: "runtime database file unwritable",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				chmodForTest(t, runtimeDBPath(t, repo), 0o444)
				return repo
			},
		},
		{name: "healthy initialised repository", setup: newInitializedRepo},
		{name: "repository that was never initialised", setup: newRepo},
		{
			name: "uninitialised repository with an occupied cache directory",
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				runtimeRoot := filepath.Join(repo, ".git", "mindrail")
				if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
					t.Fatalf("create %s: %v", runtimeRoot, err)
				}
				writeFile(t, filepath.Join(runtimeRoot, "cache"), []byte("not a directory"))
				return repo
			},
		},
	}
}

// TestNoDocumentContradictsItsOwnErrorObject is the invariant that stops finding
// H11's class from coming back.
//
// One command produces one document. That document carries a top-level `error`
// object naming the condition that stopped the run and the remedy for it, and it
// carries a `next_action` on the report as a whole, on every component of a
// status report and on every check of a doctor report. All of them describe the
// same repository at the same instant, so they have to agree about what the
// reader should do next.
//
// They did not. On an initialised repository whose runtime root had become
// unwritable, the error object said to fix the permissions while the `runtime_db`
// component and the `sqlite` check — in the very same JSON document — said to run
// `mindrail init`, the one command that cannot succeed there. The reader who
// follows the component gets exit 4 and the same advice again.
//
// The contradiction this asserts on is the sharpest one MR-001 can express:
// whether `mindrail init` is worth running. The error object is the authority,
// because it is the value `ok`, the exit code and the remedy are all derived
// from (finding F11); if it does not offer init, nothing else in the document
// may. It deliberately says nothing about documents with no error object — a
// repository that merely has not been initialised has no authority to contradict,
// and its components are supposed to recommend init.
func TestNoDocumentContradictsItsOwnErrorObject(t *testing.T) {
	for _, tc := range coherenceSetups() {
		t.Run(tc.name, func(t *testing.T) {
			for _, command := range []string{"status", "doctor", "init"} {
				t.Run(command, func(t *testing.T) {
					repo := tc.setup(t)
					assertDocumentIsCoherent(t, command, run(t, repo, command, "--json").stdout)
				})
			}
		})
	}
}

// assertDocumentIsCoherent applies the invariant to one rendered envelope.
func assertDocumentIsCoherent(t *testing.T, command, stdout string) {
	t.Helper()

	var envelope struct {
		Error *app.ErrorPayload `json:"error"`
		Data  json.RawMessage   `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("%s: stdout is not a JSON envelope: %v\n%s", command, err, stdout)
	}

	if envelope.Error == nil || offersInitRemedy(envelope.Error.NextAction) {
		return
	}

	for _, remedy := range remediesIn(t, command, envelope.Data) {
		if offersInitRemedy(remedy.actions) {
			t.Errorf(""+
				"%s: %s recommends `%s`, which the error object in the same document contradicts.\n"+
				"  error object: %s => %v\n"+
				"  %s: %v",
				command, remedy.where, initRemedy,
				envelope.Error.Code, envelope.Error.NextAction,
				remedy.where, remedy.actions)
		}
	}
}

// remedy is one next_action list found in a rendered document, with the JSON
// path it was found at so a failure names the field rather than the value.
type remedy struct {
	where   string
	actions []string
}

// remediesIn collects every next_action in a command payload.
//
// It walks the decoded JSON rather than the Go types on purpose. The three
// payloads have three shapes, and a field added by a later change is covered the
// moment it exists: an invariant that had to be taught about each new carrier
// would go stale exactly when a new carrier introduced a new contradiction.
func remediesIn(t *testing.T, command string, data json.RawMessage) []remedy {
	t.Helper()

	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("%s: data is not JSON: %v", command, err)
	}

	var found []remedy
	var walk func(path string, node any)
	walk = func(path string, node any) {
		switch value := node.(type) {
		case map[string]any:
			for _, key := range slices.Sorted(maps.Keys(value)) {
				child := join(path, key)
				if key == "next_action" {
					if actions, ok := stringsOf(value[key]); ok {
						found = append(found, remedy{where: child, actions: actions})
						continue
					}
				}
				walk(child, value[key])
			}
		case []any:
			for _, element := range value {
				walk(path+"[]", element)
			}
		}
	}
	walk("data", document)

	sort.Slice(found, func(i, j int) bool { return found[i].where < found[j].where })
	return found
}

// offersInitRemedy reports whether any of these actions sends the reader to
// `mindrail init`.
//
// The match is on a mention, not on the whole action, and that is the point. A
// document whose error object says "Move the file aside and run `mindrail init`"
// has told the reader init is worth running once the obstruction is gone, so a
// component saying the same thing agrees with it. A document whose error object
// says "make this directory writable" has said the opposite, and a component
// that mentions init at all is contradicting it — moving something aside first
// does not make init succeed against a directory nothing can write.
func offersInitRemedy(actions []string) bool {
	for _, action := range actions {
		if strings.Contains(action, initRemedy) {
			return true
		}
	}
	return false
}

// TestEveryUninitialisedSpellingExitsZero is finding H14.
//
// An interrupted first `mindrail init` can leave a repository in three shapes:
// nothing under the runtime root, a mindrail.db that was created and never
// written to, and a database that was created and not yet migrated. They are
// one state — the repository is not initialised — with one remedy, and decision
// D-03 puts that state at exit 0 with BLOCKED readiness.
//
// Two of the three were reported that way. The middle one was opened, found to
// be in rollback-journal mode because nothing had ever converted it, and
// reported as a fatal RUNTIME_DB_UNAVAILABLE at exit 4 with ok:false — a driver
// fact about a file nobody had finished creating, promoted to a failure. The
// rows are run together rather than separately because the defect is the
// disagreement between them, not any one row's value.
func TestEveryUninitialisedSpellingExitsZero(t *testing.T) {
	setups := map[string]func(t *testing.T) string{
		"nothing under the runtime root": newRepo,
		"a database file that was created and never written": func(t *testing.T) string {
			repo := newRepo(t)
			runtimeRoot := filepath.Join(repo, ".git", "mindrail")
			if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
				t.Fatalf("create %s: %v", runtimeRoot, err)
			}
			writeFile(t, runtimeDBPath(t, repo), nil)
			return repo
		},
		"a database created and not yet migrated": func(t *testing.T) string {
			repo := newRepo(t)
			createUnmigratedDatabase(t, repo)
			return repo
		},
	}

	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			repo := setup(t)

			got := run(t, repo, "status", "--json")
			got.requireExit(t, app.ExitSuccess)
			assertNoErrorEnvelope(t, "status", got.stdout)

			var report status.Report
			decodeData(t, got.stdout, &report)

			if report.Readiness != status.ReadinessBlocked {
				t.Errorf("readiness = %q, want %q", report.Readiness, status.ReadinessBlocked)
			}
			component := report.Components[status.ComponentRuntimeDB]
			if component.Code != app.CodeWorkspaceNotInitialized {
				t.Errorf("runtime_db code = %q, want %q: this is one state with one spelling",
					component.Code, app.CodeWorkspaceNotInitialized)
			}
			if !slices.Contains(component.NextAction, initRemedy) {
				t.Errorf("runtime_db does not offer `%s`: %v", initRemedy, component.NextAction)
			}

			// The remedy the report prints is the one that has to work.
			if initOut := run(t, repo, "init"); initOut.code != app.ExitSuccess {
				t.Fatalf("`%s` exited %d: %v\n%s", initRemedy, initOut.code, initOut.err, initOut.stdout)
			}
			run(t, repo, "status", "--json").requireExit(t, app.ExitSuccess)
		})
	}
}

// TestAPopulatedNonWALDatabaseIsNotCalledUninitialised is the over-fire guard
// for finding H14, and the adjacent condition the new answer must not swallow.
//
// A mindrail.db that holds data but was never converted to WAL is not an
// unfinished first run: something else wrote it, Mindrail will not read it as
// it stands, and reporting it as "not initialised, exit 0" would hide a real
// problem behind a state a consumer is told to ignore. Only the zero-length
// file — the one that holds nothing at all — is the state.
func TestAPopulatedNonWALDatabaseIsNotCalledUninitialised(t *testing.T) {
	repo := newRepo(t)
	runtimeRoot := filepath.Join(repo, ".git", "mindrail")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatalf("create %s: %v", runtimeRoot, err)
	}

	path := runtimeDBPath(t, repo)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open %s with database/sql: %v", path, err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE seeded (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed a rollback-journal database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close the seed database: %v", err)
	}

	got := run(t, repo, "status", "--json")
	if got.code == app.ExitSuccess {
		t.Fatalf("status exited 0 on a database Mindrail refuses to read:\n%s", got.stdout)
	}

	var report status.Report
	decodeData(t, got.stdout, &report)

	component := report.Components[status.ComponentRuntimeDB]
	if component.Code == app.CodeWorkspaceNotInitialized {
		t.Errorf("runtime_db code = %q, which calls somebody else's database an unfinished first run",
			component.Code)
	}
	if component.State != doctor.StateError {
		t.Errorf("runtime_db state = %q, want %q", component.State, doctor.StateError)
	}
}

// TestHealthyRepositoryIsUntouchedByRemedyCoherence is the first over-fire guard
// for finding H11's fix.
//
// The fix reaches into every reading of the runtime store and can replace its
// remedy. On a repository where nothing is wrong there is nothing to replace,
// and a report that started naming an obstruction here would be describing a
// failure that does not exist.
func TestHealthyRepositoryIsUntouchedByRemedyCoherence(t *testing.T) {
	repo := newInitializedRepo(t)

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)
	assertNoErrorEnvelope(t, "status", got.stdout)

	var report status.Report
	decodeData(t, got.stdout, &report)

	if report.Readiness != status.ReadinessReady {
		t.Fatalf("readiness = %q, want %q on a healthy repository", report.Readiness, status.ReadinessReady)
	}
	for name, component := range report.Components {
		if component.State == doctor.StateError || component.State == doctor.StateUnavailable {
			t.Errorf("component %q is %s on a healthy repository: %+v", name, component.State, component)
		}
		if len(component.NextAction) != 0 {
			t.Errorf("component %q offers a remedy on a healthy repository: %v", name, component.NextAction)
		}
	}

	doctorOut := run(t, repo, "doctor", "--json")
	doctorOut.requireExit(t, app.ExitSuccess)

	var health doctor.Report
	decodeData(t, doctorOut.stdout, &health)
	for _, check := range health.Checks {
		if check.State != doctor.StateOK {
			t.Errorf("check %q is %s on a healthy repository: %+v", check.Name, check.State, check)
		}
	}
}

// TestAnOccupiedCacheDirectoryIsNotAnObstruction is the second over-fire guard,
// and it is the adjacent condition the fix must not swallow.
//
// A runtime root nothing can write makes `mindrail init` impossible, so a
// reading that recommends init there is replaced by the obstruction. A cache
// directory nothing can write makes nothing impossible: MR-001 stores nothing in
// it, init still succeeds, and the repository is still usable. Grading the two
// alike is precisely how a fully working repository was once driven to BLOCKED
// and exit 4 (finding F13), so this asserts the opposite in both directions —
// the remedy is still `mindrail init`, and running it still works.
func TestAnOccupiedCacheDirectoryIsNotAnObstruction(t *testing.T) {
	repo := newRepo(t)
	runtimeRoot := filepath.Join(repo, ".git", "mindrail")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatalf("create %s: %v", runtimeRoot, err)
	}
	writeFile(t, filepath.Join(runtimeRoot, "cache"), []byte("not a directory"))

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)
	assertNoErrorEnvelope(t, "status", got.stdout)

	var report status.Report
	decodeData(t, got.stdout, &report)

	component, present := report.Components[status.ComponentRuntimeDB]
	if !present {
		t.Fatalf("status report has no runtime_db component: %v", report.Components)
	}
	if component.Code != app.CodeWorkspaceNotInitialized {
		t.Errorf("runtime_db code = %q, want %q: an occupied cache directory is not an obstruction",
			component.Code, app.CodeWorkspaceNotInitialized)
	}
	if !slices.Contains(component.NextAction, initRemedy) {
		t.Fatalf("runtime_db no longer offers `%s` on a repository init would fix: %v",
			initRemedy, component.NextAction)
	}

	if initOut := run(t, repo, "init"); initOut.code != app.ExitSuccess {
		t.Fatalf("the remedy the report printed exited %d: %v\n%s",
			initOut.code, initOut.err, initOut.stdout)
	}
}

// stringsOf converts a decoded JSON array of strings. A next_action that is not
// one is a shape defect the JSON contract tests catch; here it is simply not a
// remedy this invariant can read.
func stringsOf(node any) ([]string, bool) {
	elements, ok := node.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(elements))
	for _, element := range elements {
		text, ok := element.(string)
		if !ok {
			return nil, false
		}
		out = append(out, text)
	}
	return out, true
}
