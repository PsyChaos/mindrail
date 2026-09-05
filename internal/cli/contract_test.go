package cli_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/cli"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/status"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// terminalReady and terminalBlocked are the spec §82 literals. They are spelled
// out here rather than taken from internal/status so that a reword of the
// constant fails this test instead of silently changing the contract.
const (
	terminalReady   = "READY FOR TARGETED WORK"
	terminalBlocked = "BLOCKED:"
)

// updateGoldens rewrites the files in testdata instead of comparing against
// them. Regenerating by hand invites a golden that was edited to match a bug;
// regenerating with a flag makes the diff show up in review.
var updateGoldens = flag.Bool("update", false, "rewrite the golden files in testdata")

// writeGolden records a regenerated golden.
func writeGolden(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write golden %s: %v", path, err)
	}
	t.Logf("updated %s", path)
}

// TestInitPrintsReadyForTargetedWork is the happy path of spec §82: a fresh
// repository, one command, and the sentence a CI job greps for.
func TestInitPrintsReadyForTargetedWork(t *testing.T) {
	repo := newRepo(t)

	got := run(t, repo, "init")

	got.requireExit(t, app.ExitSuccess)
	if !strings.Contains(got.stdout, terminalReady) {
		t.Errorf("stdout does not contain %q:\n%s", terminalReady, got.stdout)
	}
	if strings.Contains(got.stdout, terminalBlocked) {
		t.Errorf("stdout contains %q on a healthy init:\n%s", terminalBlocked, got.stdout)
	}
}

// TestInitBlockedPrintsReason is the other half of §82. A blocked init still
// reports, still names a reason, and still exits non-zero so that neither a
// human nor a pipeline can mistake it for success (decision D-04).
func TestInitBlockedPrintsReason(t *testing.T) {
	repo := newRepo(t)
	denyWrites(t, filepath.Join(repo, ".git"))

	got := run(t, repo, "init")

	got.requireExit(t, app.ExitUnavailable)
	if strings.Contains(got.stdout, terminalReady) {
		t.Errorf("stdout claims readiness on a blocked init:\n%s", got.stdout)
	}

	reason := reasonAfter(got.stdout, terminalBlocked)
	if reason == "" {
		t.Errorf("stdout has no %q line with a reason:\n%s", terminalBlocked, got.stdout)
	}
}

// TestInitSecondRunPreservesConfigAndSaysSo covers spec §82's "existing config
// is never silently overwritten". Silently is the operative word: the bytes
// surviving is half the promise, and telling the user is the other half.
func TestInitSecondRunPreservesConfigAndSaysSo(t *testing.T) {
	repo := newRepo(t)

	if got := run(t, repo, "init"); got.code != app.ExitSuccess {
		t.Fatalf("first init exited %d: %v\n%s", got.code, got.err, got.stdout)
	}

	configPath := filepath.Join(repo, ".mindrail", "config.toml")
	sentinel := append(readFile(t, configPath), []byte("\n# sentinel-do-not-remove\n")...)
	writeFile(t, configPath, sentinel)

	got := run(t, repo, "init")
	got.requireExit(t, app.ExitSuccess)

	if after := readFile(t, configPath); !bytes.Equal(after, sentinel) {
		t.Errorf("second init rewrote the configuration:\nbefore:\n%s\nafter:\n%s", sentinel, after)
	}
	if !strings.Contains(got.stdout, configPath) {
		t.Errorf("stdout does not name the preserved config file %q:\n%s", configPath, got.stdout)
	}
	if !strings.Contains(got.stdout, "preserved") {
		t.Errorf("stdout does not say the config was preserved:\n%s", got.stdout)
	}
}

// TestInitJSONContract pins the envelope another tool parses.
func TestInitJSONContract(t *testing.T) {
	repo := newRepo(t)

	got := run(t, repo, "init", "--json")
	got.requireExit(t, app.ExitSuccess)

	envelope := got.decodeEnvelope(t)
	if envelope["command"] != "init" || envelope["ok"] != true {
		t.Errorf("envelope header = %v/%v, want init/true", envelope["command"], envelope["ok"])
	}
	assertShape(t, "init_json.golden", envelope)
}

// TestStatusJSONContract pins the readiness payload, including the six §107
// component slots.
func TestStatusJSONContract(t *testing.T) {
	repo := newInitializedRepo(t)

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)

	envelope := got.decodeEnvelope(t)
	assertShape(t, "status_json.golden", envelope)

	var report status.Report
	decodeData(t, got.stdout, &report)

	if !report.Readiness.Valid() {
		t.Errorf("readiness %q is outside the four-value enum", report.Readiness)
	}
	if report.Readiness != status.ReadinessReady {
		t.Errorf("readiness = %q, want READY on a freshly initialized repository", report.Readiness)
	}

	wantComponents := status.Components()
	gotComponents := slices.Sorted(maps.Keys(report.Components))
	slices.Sort(wantComponents)
	if !slices.Equal(gotComponents, wantComponents) {
		t.Errorf("components = %v, want %v", gotComponents, wantComponents)
	}
}

// TestDoctorJSONContract pins the check list and the health vocabulary. The
// states are validated against the enum rather than against a literal list, so
// a state this binary does not own could never reach a consumer.
func TestDoctorJSONContract(t *testing.T) {
	repo := newInitializedRepo(t)

	got := run(t, repo, "doctor", "--json")
	got.requireExit(t, app.ExitSuccess)

	var report doctor.Report
	decodeData(t, got.stdout, &report)

	if report.Command != "doctor" {
		t.Errorf("report command = %q, want doctor", report.Command)
	}

	wantChecks := []string{"git", "runtime_paths", "config", "sqlite", "migrations", "knowledge", "workspace"}
	gotChecks := make([]string, 0, len(report.Checks))
	for _, check := range report.Checks {
		gotChecks = append(gotChecks, check.Name)
	}
	if !slices.Equal(gotChecks, wantChecks) {
		t.Errorf("checks = %v, want %v", gotChecks, wantChecks)
	}

	for _, check := range report.Checks {
		if !check.State.Valid() {
			t.Errorf("check %q reported state %q, which is outside the five-value enum", check.Name, check.State)
		}
		if check.Section == "" {
			t.Errorf("check %q has no section", check.Name)
		}
	}
	if !report.WorstState.Valid() {
		t.Errorf("worst_state %q is outside the five-value enum", report.WorstState)
	}
}

// TestVersionJSONContract covers decision D-09. The knowledge schema window is
// part of the payload because it is what another tool has to read before it
// writes a record this binary may later refuse.
func TestVersionJSONContract(t *testing.T) {
	got := run(t, t.TempDir(), "version", "--json")
	got.requireExit(t, app.ExitSuccess)

	envelope := got.decodeEnvelope(t)
	if envelope["command"] != "version" {
		t.Errorf("command = %v, want version", envelope["command"])
	}
	assertShape(t, "version_json.golden", envelope)

	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is %T, want an object", envelope["data"])
	}
	for _, key := range []string{"version", "commit", "go", "platform", "mcp_compatibility"} {
		if value, _ := data[key].(string); value == "" {
			t.Errorf("data.%s is empty", key)
		}
	}
	if data["write_schema_version"] != float64(1) {
		t.Errorf("write_schema_version = %v, want 1", data["write_schema_version"])
	}
	readable, ok := data["readable_schema_versions"].([]any)
	if !ok || len(readable) == 0 {
		t.Errorf("readable_schema_versions = %v, want a non-empty array", data["readable_schema_versions"])
	}
}

// TestStatusOutsideGitRepoIsStructuredUsageError is acceptance criterion 4 at
// its simplest: the most common mistake produces a code, not a stack trace.
func TestStatusOutsideGitRepoIsStructuredUsageError(t *testing.T) {
	requireGit(t)

	got := run(t, t.TempDir(), "status", "--json")

	got.requireExit(t, app.ExitUsage)
	payload := got.errorPayload(t)
	if payload.Code != app.CodeNotAGitRepository {
		t.Errorf("code = %q, want %q", payload.Code, app.CodeNotAGitRepository)
	}
	assertFourErrorKeys(t, got.stdout)
}

// TestStatusUninitializedIsBlockedExitZero is decision D-03's one zero-exit
// row: "you have not run init yet" is a state to report, not a failure.
func TestStatusUninitializedIsBlockedExitZero(t *testing.T) {
	repo := newRepo(t)

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)

	var report status.Report
	decodeData(t, got.stdout, &report)

	if report.Readiness != status.ReadinessBlocked {
		t.Errorf("readiness = %q, want BLOCKED", report.Readiness)
	}
	if report.BlockingComponent != status.ComponentRuntimeDB {
		t.Errorf("blocking_component = %q, want runtime_db", report.BlockingComponent)
	}
	if want := []string{"mindrail init"}; !slices.Equal(report.NextAction, want) {
		t.Errorf("next_action = %v, want %v", report.NextAction, want)
	}
}

// brokenSetup is one broken installation and what the two reporting commands
// owe a caller about it.
//
// The row carries a carrier per command because MR-001 reports breakage in two
// different shapes and both are part of acceptance criterion 4. A setup that
// stops the startup sequence produces a fatal error envelope; a setup the
// sequence survives — an unwritable runtime path, a database path occupied by a
// directory, a runtime directory deleted after init, a knowledge record this
// binary cannot parse — is reported at exit 0 through a non-OK component and
// check state, with no `error` object anywhere.
//
// The previous shape of this test could only express the first: its body called
// errorPayload, which fatals unless the envelope carries an error object, so
// every exit-0 breakage was structurally out of reach. That is the half where a
// wrong remedy costs the most, because the command still succeeds and nothing
// downstream contradicts it.
type brokenSetup struct {
	name  string
	setup func(t *testing.T) string

	// options are the seams this row needs. A row that breaks git cannot break
	// it on disk — the binary is on PATH and shared with every other test — so
	// it substitutes the runner instead (finding F19).
	options cli.Options

	// wantExit is decision D-03's answer for this failure class.
	wantExit int
	// wantCode is the code every carrier of this reading must agree on. It is
	// empty only for a setup that is not a failure at all (decision D-06).
	wantCode app.Code
	// wantState is the reading's state wherever it is carried.
	wantState doctor.State

	// wantError requires a fatal error envelope carrying wantCode from both
	// commands.
	wantError bool
	// wantComponent, when named, is the spec §107 slot `status` must report the
	// reading in. It stays empty for a setup that aborts startup before the
	// runtime locations exist, because there is then no repository to describe
	// and status emits the error alone.
	wantComponent status.ComponentName
	// wantCheck, when named, is the doctor check that must carry the reading.
	wantCheck string

	// unusableRemedies are next actions that would not work on this setup.
	// Naming them is the point of the row: a report that answers every breakage
	// with `mindrail init` passes a test that only checks next_action is
	// non-empty, and that is exactly what shipped.
	unusableRemedies []string

	// noUnderlyingCause marks a refusal that is Mindrail's own policy rather
	// than a lower layer's failure. Nothing broke when a record is outside the
	// reader window; the binary declined to guess. Every other fatal row has a
	// library or syscall message underneath it and has to carry it, because
	// `why` is the same sentence for every instance of a code.
	noUnderlyingCause bool
}

// TestBrokenSetupMatrix is acceptance criterion 4 in full: every broken setup is
// describable by both reporting commands, at the exit code decision D-03 assigns
// it, with the same code, and — the part no test asserted before — with a remedy
// that is neither the command being run nor a command that would fail here.
func TestBrokenSetupMatrix(t *testing.T) {
	tests := []brokenSetup{
		{
			name:      "no repository",
			setup:     func(t *testing.T) string { requireGit(t); return t.TempDir() },
			wantExit:  app.ExitUsage,
			wantCode:  app.CodeNotAGitRepository,
			wantState: doctor.StateError,
			wantError: true,
			wantCheck: "git",
			// init cannot create a repository, and neither reporting command can
			// fix its own precondition.
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name:     "bare repository",
			setup:    newBareRepo,
			wantExit: app.ExitUsage,
			// Decision D-31: acceptance criterion 3 is about an active worktree,
			// and a bare repository has none.
			wantCode:         app.CodeBareRepository,
			wantState:        doctor.StateError,
			wantError:        true,
			wantCheck:        "git",
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name: "unwritable runtime path",
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				denyWrites(t, filepath.Join(repo, ".git"))
				return repo
			},
			wantExit:      app.ExitUnavailable,
			wantCode:      app.CodeRuntimePathUnwritable,
			wantState:     doctor.StateError,
			wantError:     true,
			wantComponent: status.ComponentRuntimeDB,
			wantCheck:     "runtime_paths",
			// init is the command that would fail: it is the one that has to
			// write into the directory nothing can write into.
			unusableRemedies: []string{"mindrail init"},
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
			wantExit:      app.ExitUnavailable,
			wantCode:      app.CodeRuntimeDBUnavailable,
			wantState:     doctor.StateError,
			wantError:     true,
			wantComponent: status.ComponentRuntimeDB,
			wantCheck:     "sqlite",
			// A directory where the database belongs looks exactly like a
			// repository that was never initialised, and init cannot clear it.
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name: "runtime database unreadable",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyAccess(t, runtimeDBPath(t, repo))
				return repo
			},
			wantExit:         app.ExitUnavailable,
			wantCode:         app.CodeRuntimeDBUnavailable,
			wantState:        doctor.StateError,
			wantError:        true,
			wantComponent:    status.ComponentRuntimeDB,
			wantCheck:        "sqlite",
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name: "runtime database corrupt",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				corruptDatabase(t, repo)
				return repo
			},
			wantExit:      app.ExitUnavailable,
			wantCode:      app.CodeRuntimeDBCorrupt,
			wantState:     doctor.StateError,
			wantError:     true,
			wantComponent: status.ComponentRuntimeDB,
			wantCheck:     "sqlite",
			// init would open the same damaged file and fail the same way; the
			// remedy has to move it aside first.
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name: "runtime directory deleted after init",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				if err := os.RemoveAll(filepath.Join(repo, ".git", "mindrail")); err != nil {
					t.Fatalf("remove the runtime directory: %v", err)
				}
				return repo
			},
			// Decision D-03's one zero-exit row: an absent runtime store is a
			// state to report, not a failure. Here `mindrail init` really is the
			// remedy, so the row does not forbid it — assertRemedyWorks runs it.
			wantExit:      app.ExitSuccess,
			wantCode:      app.CodeWorkspaceNotInitialized,
			wantState:     doctor.StateUnavailable,
			wantComponent: status.ComponentRuntimeDB,
			wantCheck:     "sqlite",
		},
		{
			name: "configuration with a syntax error",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeFile(t, configPath(repo), []byte("[[[\n"))
				return repo
			},
			wantExit:  app.ExitUsage,
			wantCode:  app.CodeConfigInvalid,
			wantState: doctor.StateError,
			wantError: true,
			// No component: startup stops at load_config, before the runtime
			// locations exist, so status has no repository to describe.
			wantCheck:        "config",
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name: "configuration with an unknown key",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeFile(t, configPath(repo), []byte("[output]\ncolour = \"always\"\n"))
				return repo
			},
			wantExit:         app.ExitUsage,
			wantCode:         app.CodeConfigInvalid,
			wantState:        doctor.StateError,
			wantError:        true,
			wantCheck:        "config",
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name: "knowledge store unreadable",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyAccess(t, filepath.Join(repo, ".mindrail", "knowledge"))
				return repo
			},
			wantExit:         app.ExitFailed,
			wantCode:         app.CodeKnowledgeUnreadable,
			wantState:        doctor.StateError,
			wantError:        true,
			wantComponent:    status.ComponentKnowledge,
			wantCheck:        "knowledge",
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name: "malformed knowledge record",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "broken.json", "{ this is not json")
				return repo
			},
			// One unparseable record costs the repository that record, not the
			// installation: DEGRADED, exit 0, and no error envelope at all.
			wantExit:         app.ExitSuccess,
			wantCode:         app.CodeKnowledgeUnreadable,
			wantState:        doctor.StateDegraded,
			wantComponent:    status.ComponentKnowledge,
			wantCheck:        "knowledge",
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name: "knowledge record from a future schema",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "future.json",
					`{"schema_version": 999, "id": "D-999", "title": "from the future"}`)
				return repo
			},
			// Decision D-27 and spec §95: fail closed on a record outside the
			// reader window rather than guess at its meaning.
			wantExit:      app.ExitFailed,
			wantCode:      app.CodeKnowledgeSchemaUnsupported,
			wantState:     doctor.StateError,
			wantError:     true,
			wantComponent: status.ComponentKnowledge,
			wantCheck:     "knowledge",
			// Only a newer binary can read it; no local command can.
			unusableRemedies:  []string{"mindrail init"},
			noUnderlyingCause: true,
		},
		{
			name:    "git is not installed",
			setup:   func(t *testing.T) string { isolateEnvironment(t); return t.TempDir() },
			options: cli.Options{Runner: unavailableGit()},
			// Decision D-30 keeps a missing or hanging git UNAVAILABLE rather
			// than ERROR so a slow filesystem cannot fail a correct pipeline.
			// That is exactly why nothing produced an error envelope here and
			// the exit code came from a value the envelope never saw
			// (finding F11).
			wantExit:  app.ExitUnavailable,
			wantCode:  app.CodeGitUnavailable,
			wantState: doctor.StateUnavailable,
			wantError: true,
			wantCheck: "git",
			// No component: startup stops at resolve_repository, before the
			// runtime locations exist, so status has no repository to describe.
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name:             "git does not answer in time",
			setup:            func(t *testing.T) string { isolateEnvironment(t); return t.TempDir() },
			options:          cli.Options{Runner: hangingGit()},
			wantExit:         app.ExitUnavailable,
			wantCode:         app.CodeGitTimeout,
			wantState:        doctor.StateUnavailable,
			wantError:        true,
			wantCheck:        "git",
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name: "unusable cache directory",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				occupyCacheDirectory(t, repo)
				return repo
			},
			// The over-fire guard of finding F13, at the layer a user sees. The
			// repository is fully working: the database opens, the schema is
			// current, the worktree is registered. MR-001 writes nothing to the
			// cache, so this costs nothing and must not stop anything.
			wantExit:      app.ExitSuccess,
			wantCode:      app.CodeRuntimePathUnwritable,
			wantState:     doctor.StateDegraded,
			wantComponent: status.ComponentRuntimeDB,
			wantCheck:     "runtime_paths",
			// init cannot clear a file sitting where the directory belongs.
			unusableRemedies: []string{"mindrail init"},
		},
		{
			name: "first run still in flight",
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				createUnmigratedDatabase(t, repo)
				return repo
			},
			// Finding F16: a database another process created and has not
			// finished migrating. It is "not initialised yet", and the remedy
			// really is init, so the row lets assertRemedyWorks run it.
			wantExit:      app.ExitSuccess,
			wantCode:      app.CodeWorkspaceNotInitialized,
			wantState:     doctor.StateUnavailable,
			wantComponent: status.ComponentRuntimeDB,
			wantCheck:     "workspace",
		},
		{
			name: "knowledge directory absent",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				if err := os.RemoveAll(filepath.Join(repo, ".mindrail", "knowledge")); err != nil {
					t.Fatalf("remove the knowledge directory: %v", err)
				}
				return repo
			},
			// Decision D-06, and the control row of this table: a clean clone of
			// a repository that carries no records must not look broken. It is
			// here so that a future "report everything absent as a problem"
			// change fails a test instead of shipping.
			wantExit:      app.ExitSuccess,
			wantState:     doctor.StateOK,
			wantComponent: status.ComponentKnowledge,
			wantCheck:     "knowledge",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.requireACarrier(t)

			repo := tc.setup(t)

			tc.assertStatus(t, repo)
			tc.assertDoctor(t, repo)
			tc.assertRemedyWorks(t, repo)
		})
	}
}

// requireACarrier refuses a row that asserts nothing. A table entry whose
// expectations are all zero passes silently, which is how the four rows this
// test used to have came to cover none of the exit-0 breakages.
func (tc brokenSetup) requireACarrier(t *testing.T) {
	t.Helper()

	if !tc.wantError && tc.wantComponent == "" && tc.wantCheck == "" {
		t.Fatalf("row %q names neither an error envelope nor a component nor a check", tc.name)
	}
}

// assertStatus drives `status --json` and checks whichever carriers the row
// named.
func (tc brokenSetup) assertStatus(t *testing.T, repo string) {
	t.Helper()

	got := runWith(t, repo, tc.options, "status", "--json")
	got.requireExit(t, tc.wantExit)

	if tc.wantError {
		tc.assertErrorEnvelope(t, "status", got)
	} else {
		assertNoErrorEnvelope(t, "status", got.stdout)
	}

	if tc.wantComponent == "" {
		return
	}

	var report status.Report
	decodeData(t, got.stdout, &report)

	component, present := report.Components[tc.wantComponent]
	if !present {
		t.Fatalf("status report has no %q component: %v", tc.wantComponent, report.Components)
	}
	if component.State != tc.wantState {
		t.Errorf("status component %q state = %q, want %q", tc.wantComponent, component.State, tc.wantState)
	}
	if component.Code != tc.wantCode {
		t.Errorf("status component %q code = %q, want %q", tc.wantComponent, component.Code, tc.wantCode)
	}
	if component.Summary == "" {
		t.Errorf("status component %q has no summary", tc.wantComponent)
	}

	if tc.wantState == doctor.StateOK {
		return
	}
	if len(component.NextAction) == 0 {
		t.Errorf("status component %q is %s with no next action", tc.wantComponent, component.State)
	}
	tc.assertRemediesAreUsable(t, "status component "+string(tc.wantComponent), "status", component.NextAction)
}

// assertDoctor drives `doctor --json` and checks the named check's reading. A
// non-OK doctor reading owes all three of diagnostic, impact and next action
// (spec §84), which is a stricter promise than the status component can make —
// status carries the verdict and the remedy, and leaves the explanation here.
func (tc brokenSetup) assertDoctor(t *testing.T, repo string) {
	t.Helper()

	got := runWith(t, repo, tc.options, "doctor", "--json")
	got.requireExit(t, tc.wantExit)

	if tc.wantError {
		tc.assertErrorEnvelope(t, "doctor", got)
	} else {
		assertNoErrorEnvelope(t, "doctor", got.stdout)
	}

	if tc.wantCheck == "" {
		return
	}

	var report doctor.Report
	decodeData(t, got.stdout, &report)

	check, found := findCheck(report, tc.wantCheck)
	if !found {
		t.Fatalf("doctor report has no %q check: %v", tc.wantCheck, report.Checks)
	}
	if check.State != tc.wantState {
		t.Errorf("doctor check %q state = %q, want %q", tc.wantCheck, check.State, tc.wantState)
	}
	if check.Code != tc.wantCode {
		t.Errorf("doctor check %q code = %q, want %q", tc.wantCheck, check.Code, tc.wantCode)
	}

	if tc.wantState == doctor.StateOK {
		return
	}
	for label, value := range map[string]string{"diagnostic": check.Diagnostic, "impact": check.Impact} {
		if value == "" {
			t.Errorf("doctor check %q is %s with no %s", tc.wantCheck, check.State, label)
		}
	}
	if len(check.NextAction) == 0 {
		t.Errorf("doctor check %q is %s with no next action", tc.wantCheck, check.State)
	}
	tc.assertRemediesAreUsable(t, "doctor check "+tc.wantCheck, "doctor", check.NextAction)
}

// assertErrorEnvelope pins the fatal form: the four §84 keys on the wire, the
// expected code, the same code recoverable through errors.As, and a remedy the
// caller can actually follow.
func (tc brokenSetup) assertErrorEnvelope(t *testing.T, command string, got result) {
	t.Helper()

	payload := got.errorPayload(t)
	if payload.Code != tc.wantCode {
		t.Errorf("%s reported code %q, want %q", command, payload.Code, tc.wantCode)
	}
	assertFourErrorKeys(t, got.stdout)

	recovered, ok := app.PayloadOf(got.err)
	if !ok {
		t.Fatalf("%s: errors.As recovered no payload from %v", command, got.err)
	}
	if recovered.Code != tc.wantCode {
		t.Errorf("%s: errors.As recovered code %q, want %q", command, recovered.Code, tc.wantCode)
	}

	if !tc.noUnderlyingCause && payload.Cause == "" {
		t.Errorf("%s reports %s with no cause, so the report cannot distinguish this instance from any other %s",
			command, tc.wantCode, tc.wantCode)
	}

	tc.assertRemediesAreUsable(t, command+" error payload", command, payload.NextAction)
}

// assertRemediesAreUsable is the assertion whose absence let the shipped binary
// answer every breakage with `mindrail init`.
//
// Two things are forbidden. Re-running the command that just reported the
// problem is a loop, not a remedy. And a command the row has declared would fail
// on this setup is worse than no advice at all, because the user follows it,
// watches it fail, and learns to distrust the report.
//
// Matching is on the whole action, not a substring: "Move the file aside and
// run `mindrail init`" is a good remedy that happens to name the same command,
// and banning it by substring would push the report towards vaguer prose.
func (tc brokenSetup) assertRemediesAreUsable(t *testing.T, where, command string, actions []string) {
	t.Helper()

	forbidden := append([]string{"mindrail " + command, command}, tc.unusableRemedies...)
	for _, action := range actions {
		normalized := strings.Trim(strings.TrimSpace(action), "`.")
		for _, bad := range forbidden {
			if normalized == bad {
				t.Errorf("%s tells the user to run %q, which does not work on this setup (all actions: %v)",
					where, action, actions)
			}
		}
	}
}

// assertRemedyWorks follows the advice. A report that recommends `mindrail init`
// on a repository where init fails is the systemic defect this table exists to
// catch, and the only way to be sure the advice is good is to take it.
func (tc brokenSetup) assertRemedyWorks(t *testing.T, repo string) {
	t.Helper()

	if !tc.recommendsInit(t, repo) {
		return
	}

	if got := runWith(t, repo, tc.options, "init"); got.code != app.ExitSuccess {
		t.Errorf("the report's own next action `mindrail init` exited %d: %v\n%s",
			got.code, got.err, got.stdout)
	}
}

// recommendsInit reports whether `mindrail init` is offered as a whole action by
// either command.
func (tc brokenSetup) recommendsInit(t *testing.T, repo string) bool {
	t.Helper()

	const remedy = "mindrail init"

	statusOut := runWith(t, repo, tc.options, "status", "--json")
	if payload, ok := app.PayloadOf(statusOut.err); ok && slices.Contains(payload.NextAction, remedy) {
		return true
	}
	if statusOut.stdout != "" && strings.Contains(statusOut.stdout, `"`+remedy+`"`) {
		var report status.Report
		if json.Unmarshal([]byte(statusOut.stdout), &struct {
			Data *status.Report `json:"data"`
		}{Data: &report}) == nil {
			if slices.Contains(report.NextAction, remedy) {
				return true
			}
			for _, component := range report.Components {
				if slices.Contains(component.NextAction, remedy) {
					return true
				}
			}
		}
	}
	return false
}

// findCheck locates one reading in a doctor report.
func findCheck(report doctor.Report, name string) (doctor.Result, bool) {
	for _, check := range report.Checks {
		if check.Name == name {
			return check, true
		}
	}
	return doctor.Result{}, false
}

// assertNoErrorEnvelope is the other half of the either/or: a setup MR-001
// reports at exit 0 must carry no error object at all, or a consumer branching
// on `ok` would refuse a repository that is merely degraded.
func assertNoErrorEnvelope(t *testing.T, command, stdout string) {
	t.Helper()

	var envelope struct {
		OK    bool            `json:"ok"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("%s: stdout is not a JSON object: %v\n%s", command, err, stdout)
	}
	if len(envelope.Error) != 0 {
		t.Errorf("%s carries an error payload on a setup it reports at exit 0: %s", command, envelope.Error)
	}
	if !envelope.OK {
		t.Errorf("%s reports ok=false at exit 0:\n%s", command, stdout)
	}
}

// TestLinkedWorktreeIsReportedEndToEnd is acceptance criterion 3 at the layer
// the criterion is stated about.
//
// The Git adapter's own tests prove it parses `git rev-parse` correctly, but
// AC-3 is a promise about what `status` and `doctor` *report*, and until this
// test existed nothing between the adapter and the rendered output was covered:
// the word "worktree" did not appear in either the contract tests or the smoke
// tests. A wiring regression — status reading the git dir where it means the
// common dir, doctor describing the main worktree from inside a linked one —
// would have left every unit test green.
//
// The two roots must differ and the common dir must not: that pair is the whole
// of what a linked worktree is, and it is what decides where the runtime state
// lives for both of them (decision D-26).
func TestLinkedWorktreeIsReportedEndToEnd(t *testing.T) {
	main := newRepoWithCommit(t)
	linked := addWorktree(t, main, "feature")

	// Both roots are initialised: a linked worktree shares the runtime database
	// under the common dir but is a workspace of its own, and registering it is
	// what makes the shared-project/distinct-worktree pair observable.
	for _, root := range []string{main, linked} {
		if got := run(t, root, "init"); got.code != app.ExitSuccess {
			t.Fatalf("init in %s exited %d: %v\n%s", root, got.code, got.err, got.stdout)
		}
	}

	reports := map[string]status.Report{}
	for name, root := range map[string]string{"main": main, "linked": linked} {
		got := run(t, root, "status", "--json")
		got.requireExit(t, app.ExitSuccess)

		var report status.Report
		decodeData(t, got.stdout, &report)
		reports[name] = report
	}

	mainRepo, linkedRepo := reports["main"].Repository, reports["linked"].Repository

	if mainRepo.CommonDir != linkedRepo.CommonDir {
		t.Errorf("common_dir differs between worktrees: main %q, linked %q",
			mainRepo.CommonDir, linkedRepo.CommonDir)
	}
	if mainRepo.WorktreeRoot == linkedRepo.WorktreeRoot {
		t.Errorf("both worktrees report the same worktree_root %q", mainRepo.WorktreeRoot)
	}
	if mainRepo.IsLinkedWorktree {
		t.Errorf("the main worktree reports is_linked_worktree = true")
	}
	if !linkedRepo.IsLinkedWorktree {
		t.Errorf("the linked worktree reports is_linked_worktree = false")
	}
	if mainRepo.GitDir == linkedRepo.GitDir {
		t.Errorf("both worktrees report the same git_dir %q", mainRepo.GitDir)
	}

	// One project, two workspaces: the identity half of decision D-26. Without
	// this a report could satisfy every path assertion above by registering the
	// linked worktree as a separate project.
	mainWS, linkedWS := reports["main"].Workspace, reports["linked"].Workspace
	if mainWS.ProjectID == "" || mainWS.ProjectID != linkedWS.ProjectID {
		t.Errorf("project ids differ across worktrees of one repository: main %q, linked %q",
			mainWS.ProjectID, linkedWS.ProjectID)
	}
	if mainWS.ID == "" || mainWS.ID == linkedWS.ID {
		t.Errorf("both worktrees registered as workspace %q", mainWS.ID)
	}

	// doctor has to agree, because it is the other command AC-3 names and it
	// reads the layout through a different projection of the same subject.
	for name, root := range map[string]string{"main": main, "linked": linked} {
		got := run(t, root, "doctor", "--json")
		got.requireExit(t, app.ExitSuccess)

		var report doctor.Report
		decodeData(t, got.stdout, &report)

		check, found := findCheck(report, "git")
		if !found {
			t.Fatalf("%s: doctor report has no git check", name)
		}
		want := reports[name].Repository
		if got := check.Details["common_dir"]; got != want.CommonDir {
			t.Errorf("%s: doctor common_dir = %q, status said %q", name, got, want.CommonDir)
		}
		if got := check.Details["worktree_root"]; got != want.WorktreeRoot {
			t.Errorf("%s: doctor worktree_root = %q, status said %q", name, got, want.WorktreeRoot)
		}
	}
}

// TestFailureCauseReachesTheUser is the end-to-end half of the error-surface
// fix.
//
// DomainError.Cause was set at every layer that could set it and rendered by
// none: not in the JSON envelope, not on stderr, not under --verbose. Diagnosing
// a runtime-store failure meant patching and rebuilding the binary, because
// `why` is the same sentence for every instance of a code and the sentence that
// tells two instances apart was thrown away one layer below the boundary.
func TestFailureCauseReachesTheUser(t *testing.T) {
	repo := newInitializedRepo(t)
	corruptDatabase(t, repo)

	got := run(t, repo, "status", "--json", "--verbose")
	got.requireExit(t, app.ExitUnavailable)

	payload := got.errorPayload(t)
	if payload.Cause == "" {
		t.Fatalf("the error payload carries no cause:\n%s", got.stdout)
	}
	if payload.Cause == payload.Why {
		t.Errorf("cause repeats why (%q) and so adds nothing", payload.Cause)
	}

	// --verbose is the request for the whole story, and the story is only whole
	// if the layer that actually failed gets to speak.
	if !strings.Contains(got.stderr, payload.Cause) {
		t.Errorf("--verbose stderr does not carry the cause %q:\n%s", payload.Cause, got.stderr)
	}

	// A healthy run gains no key: the addition is a fifth, optional member and
	// an envelope with nothing to explain must serialize as it always did.
	healthy := run(t, newInitializedRepo(t), "status", "--json")
	healthy.requireExit(t, app.ExitSuccess)
	if strings.Contains(healthy.stdout, `"cause"`) {
		t.Errorf("a healthy status envelope carries a cause key:\n%s", healthy.stdout)
	}
}

// TestNoColorProducesNoANSI enforces tech-stack §12 across every human path.
func TestNoColorProducesNoANSI(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	repo := newRepo(t)
	for _, command := range []string{"init", "status", "doctor"} {
		got := run(t, repo, command)
		if bytes.ContainsRune([]byte(got.stdout), 0x1b) {
			t.Errorf("%s emitted an ANSI escape with NO_COLOR set:\n%q", command, got.stdout)
		}
	}
}

// TestJSONGoesToStdoutLogsToStderr is decision D-15. The proof is that stdout
// unmarshals directly, with no filtering and nothing after the object.
func TestJSONGoesToStdoutLogsToStderr(t *testing.T) {
	repo := newInitializedRepo(t)

	got := run(t, repo, "status", "--json", "--verbose")
	got.requireExit(t, app.ExitSuccess)

	decoder := json.NewDecoder(strings.NewReader(got.stdout))
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("stdout is not a single JSON object: %v\n%s", err, got.stdout)
	}
	if _, err := decoder.Token(); err != io.EOF {
		t.Errorf("stdout carries more than one JSON value (next token error %v):\n%s", err, got.stdout)
	}

	if got.stderr == "" {
		t.Fatal("stderr carries no log records at --verbose")
	}
	if !strings.Contains(got.stderr, "step=resolve_repository") {
		t.Errorf("stderr does not carry the startup sequence:\n%s", got.stderr)
	}
	if strings.Contains(got.stdout, "level=") {
		t.Errorf("a log record leaked into stdout:\n%s", got.stdout)
	}
}

// TestExitCodeMatrix is decision D-03's table, asserted end to end. Exit 3
// (denied) has no producer in MR-001: verification arrives with MR-011.
func TestExitCodeMatrix(t *testing.T) {
	tests := []struct {
		name    string
		command []string
		setup   func(t *testing.T) string
		options cli.Options
		want    int
	}{
		{
			name:    "healthy status",
			command: []string{"status"},
			setup:   newInitializedRepo,
			want:    app.ExitSuccess,
		},
		{
			name:    "uninitialized status",
			command: []string{"status"},
			setup:   newRepo,
			want:    app.ExitSuccess,
		},
		{
			name:    "not a git repository",
			command: []string{"status"},
			setup:   func(t *testing.T) string { requireGit(t); return t.TempDir() },
			want:    app.ExitUsage,
		},
		{
			name:    "bare repository",
			command: []string{"status"},
			setup:   newBareRepo,
			want:    app.ExitUsage,
		},
		{
			name:    "unknown configuration key",
			command: []string{"status"},
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeFile(t, filepath.Join(repo, ".mindrail", "config.toml"), []byte("[output]\ncolour = \"always\"\n"))
				return repo
			},
			want: app.ExitUsage,
		},
		{
			name:    "malformed configuration",
			command: []string{"status"},
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeFile(t, filepath.Join(repo, ".mindrail", "config.toml"), []byte("this is not toml\n"))
				return repo
			},
			want: app.ExitUsage,
		},
		{
			name:    "unwritable runtime path",
			command: []string{"init"},
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				denyWrites(t, filepath.Join(repo, ".git"))
				return repo
			},
			want: app.ExitUnavailable,
		},
		{
			name:    "corrupt runtime database",
			command: []string{"status"},
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				corruptDatabase(t, repo)
				return repo
			},
			want: app.ExitUnavailable,
		},
		{
			name:    "schema newer than this binary",
			command: []string{"status"},
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				recordFutureMigration(t, repo)
				return repo
			},
			want: app.ExitFailed,
		},
		{
			// Decision D-30 fixes exit 4 for a git that cannot be run, on every
			// command. The row was missing, and with it the only end-to-end
			// evidence that the exit code and the envelope agree on the class of
			// failure that made them disagree (findings F11, F19).
			name:    "git is not installed",
			command: []string{"status"},
			setup:   func(t *testing.T) string { isolateEnvironment(t); return t.TempDir() },
			options: cli.Options{Runner: unavailableGit()},
			want:    app.ExitUnavailable,
		},
		{
			name:    "git does not answer in time",
			command: []string{"doctor"},
			setup:   func(t *testing.T) string { isolateEnvironment(t); return t.TempDir() },
			options: cli.Options{Runner: hangingGit()},
			want:    app.ExitUnavailable,
		},
		{
			name:    "init cannot run without git",
			command: []string{"init"},
			setup:   func(t *testing.T) string { isolateEnvironment(t); return t.TempDir() },
			options: cli.Options{Runner: unavailableGit()},
			want:    app.ExitUnavailable,
		},
		{
			// Finding F13: MR-001 stores nothing in the cache directory, so an
			// unusable one is reported and costs nothing. init is the command
			// that used to fail here, so init is the command this row runs.
			name:    "unusable cache directory",
			command: []string{"init"},
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				occupyCacheDirectory(t, repo)
				return repo
			},
			want: app.ExitSuccess,
		},
		{
			// Finding F16: a first run another process has not finished is a
			// state, not a failure, and takes decision D-03's zero-exit row.
			name:    "first run still in flight",
			command: []string{"status"},
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				createUnmigratedDatabase(t, repo)
				return repo
			},
			want: app.ExitSuccess,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.setup(t)

			got := runWith(t, repo, tc.options, tc.command...)
			if got.code != tc.want {
				t.Errorf("exit code = %d, want %d (error %v)\n%s", got.code, tc.want, got.err, got.stdout)
			}
		})
	}
}

// TestNoPanicOnHostileInput asserts the boring outcome. Everything here is a
// state a repository can genuinely reach, and each one has to become an error a
// caller can read rather than a stack trace.
func TestNoPanicOnHostileInput(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "corrupt runtime database",
			fn: func(t *testing.T) {
				repo := newInitializedRepo(t)
				corruptDatabase(t, repo)
				requireOrdinaryError(t, run(t, repo, "doctor", "--json"))
			},
		},
		{
			name: "malformed configuration",
			fn: func(t *testing.T) {
				repo := newInitializedRepo(t)
				writeFile(t, filepath.Join(repo, ".mindrail", "config.toml"), []byte("[[[\n"))
				requireOrdinaryError(t, run(t, repo, "status", "--json"))
			},
		},
		{
			name: "unreadable git directory",
			fn: func(t *testing.T) {
				repo := newRepo(t)
				denyAccess(t, filepath.Join(repo, ".git"))
				requireOrdinaryError(t, run(t, repo, "doctor", "--json"))
			},
		},
		{
			name: "zero-byte and malformed migrations",
			fn: func(t *testing.T) {
				repo := newRepo(t)
				application := bootstrap.New(bootstrap.Options{
					StartDir:      repo,
					Mode:          bootstrap.ModeInit,
					Environ:       []string{},
					UserConfigDir: t.TempDir(),
					MigrationFS: fstest.MapFS{
						"000001_empty.sql":  &fstest.MapFile{Data: []byte{}},
						"000002_broken.sql": &fstest.MapFile{Data: []byte("this is not sql;")},
					},
				})
				t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

				err := application.Start(t.Context())
				if err == nil {
					t.Fatal("a malformed migration set started cleanly")
				}
				if _, ok := app.PayloadOf(err); !ok {
					t.Errorf("error %v carries no domain payload", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("panicked: %v", recovered)
				}
			}()
			tc.fn(t)
		})
	}
}

// --- harness ----------------------------------------------------------------

// result is one command invocation, captured the way a caller sees it.
type result struct {
	stdout string
	stderr string
	err    error
	code   int
}

// run drives the real command tree in process against the real git.
func run(t *testing.T, dir string, args ...string) result {
	t.Helper()
	return runWith(t, dir, cli.Options{}, args...)
}

// runWith drives the real command tree in process against explicit seams.
// Building the tree per call is deliberate: cobra flags are stateful, and a
// shared root would let one test's --json leak into the next one's assertions.
//
// The seam is what makes the git-failure branch reachable at all. Every command
// used to construct git.NewExecRunner() itself, so "git is missing" and "git
// hangs" could only be produced by emptying PATH for the whole process — which a
// suite that runs tests in one binary cannot do — and that is why the envelope
// said ok:true while the process exited 4 through a full green suite
// (findings F11, F19).
func runWith(t *testing.T, dir string, options cli.Options, args ...string) result {
	t.Helper()

	root := cli.NewRootCommandWith(options)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append(slices.Clone(args), "-C", dir))

	err := root.ExecuteContext(t.Context())

	return result{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
		code:   app.ExitCode(err),
	}
}

func (r result) requireExit(t *testing.T, want int) {
	t.Helper()
	if r.code != want {
		t.Fatalf("exit code = %d, want %d (error %v)\nstdout:\n%s\nstderr:\n%s", r.code, want, r.err, r.stdout, r.stderr)
	}
}

func (r result) decodeEnvelope(t *testing.T) map[string]any {
	t.Helper()

	var envelope map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &envelope); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\n%s", err, r.stdout)
	}
	return envelope
}

func (r result) errorPayload(t *testing.T) app.ErrorPayload {
	t.Helper()

	var envelope struct {
		OK    bool              `json:"ok"`
		Error *app.ErrorPayload `json:"error"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &envelope); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\n%s", err, r.stdout)
	}
	if envelope.OK {
		t.Fatalf("envelope reports success but an error was expected:\n%s", r.stdout)
	}
	if envelope.Error == nil {
		t.Fatalf("envelope carries no error payload:\n%s", r.stdout)
	}
	return *envelope.Error
}

// decodeData unmarshals the envelope's data member into v.
func decodeData(t *testing.T, stdout string, v any) {
	t.Helper()

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\n%s", err, stdout)
	}
	if len(envelope.Data) == 0 {
		t.Fatalf("envelope carries no data member:\n%s", stdout)
	}
	if err := json.Unmarshal(envelope.Data, v); err != nil {
		t.Fatalf("decode data into %T: %v\n%s", v, err, stdout)
	}
}

// assertFourErrorKeys checks spec §84's promise directly on the wire, rather
// than through the Go type: a struct tag typo would hide the missing key.
func assertFourErrorKeys(t *testing.T, stdout string) {
	t.Helper()

	var envelope struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\n%s", err, stdout)
	}

	for _, key := range []string{"code", "why", "impact", "next_action"} {
		value, present := envelope.Error[key]
		if !present {
			t.Errorf("error payload has no %q key: %v", key, envelope.Error)
			continue
		}
		if isEmptyValue(value) {
			t.Errorf("error payload key %q is empty: %v", key, value)
		}
	}
}

func isEmptyValue(v any) bool {
	switch typed := v.(type) {
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case nil:
		return true
	default:
		return false
	}
}

// requireOrdinaryError insists the command failed the way a command is supposed
// to fail: a coded payload and a mapped exit code.
func requireOrdinaryError(t *testing.T, got result) {
	t.Helper()

	if got.err == nil {
		t.Fatalf("command succeeded on hostile input:\n%s", got.stdout)
	}
	if _, ok := app.PayloadOf(got.err); !ok {
		t.Errorf("error %v carries no domain payload", got.err)
	}
	if got.code == app.ExitSuccess {
		t.Errorf("exit code is 0 despite the error %v", got.err)
	}
}

// --- goldens ----------------------------------------------------------------

// assertShape compares the JSON key set, not the values. Values are paths,
// timings and identifiers that legitimately differ per machine; the shape is
// what another tool binds to.
func assertShape(t *testing.T, name string, value any) {
	t.Helper()

	got := strings.Join(jsonShape("", value), "\n") + "\n"
	path := filepath.Join("testdata", name)

	if *updateGoldens {
		writeGolden(t, path, got)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v\ngot:\n%s", path, err, got)
	}
	if got != string(want) {
		t.Errorf("JSON shape differs from %s\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

// jsonShape renders the sorted dotted key paths of a decoded JSON value. An
// array contributes the shape of its first element under "[]", which is enough
// for a homogeneous payload and keeps the golden independent of how many rows
// a particular run produced.
func jsonShape(prefix string, value any) []string {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		paths := make([]string, 0, len(keys))
		for _, key := range keys {
			paths = append(paths, jsonShape(join(prefix, key), typed[key])...)
		}
		if len(paths) == 0 {
			return []string{prefix + " {}"}
		}
		return paths

	case []any:
		if len(typed) == 0 {
			return []string{prefix + "[]"}
		}
		return jsonShape(prefix+"[]", typed[0])

	default:
		return []string{prefix}
	}
}

func join(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// --- fixtures ---------------------------------------------------------------

// newRepo creates a temporary Git repository with the ambient configuration
// neutralised, so the result does not depend on the developer's home directory
// or exported variables (tech-stack §131).
func newRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	isolateEnvironment(t)

	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

func newBareRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	isolateEnvironment(t)

	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", "--bare", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}
	return dir
}

// newInitializedRepo is a repository `mindrail init` has already succeeded in.
func newInitializedRepo(t *testing.T) string {
	t.Helper()

	repo := newRepo(t)
	if got := run(t, repo, "init"); got.code != app.ExitSuccess {
		t.Fatalf("init exited %d: %v\n%s", got.code, got.err, got.stdout)
	}
	return repo
}

// isolateEnvironment points the user layer at a temporary directory and refuses
// to run against a shell that already exports MINDRAIL_ settings: a stale
// export would change the effective configuration and make the goldens depend
// on whoever ran the suite.
func isolateEnvironment(t *testing.T) {
	t.Helper()

	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "MINDRAIL_") {
			t.Skipf("%s is set in the environment; unset it before running the CLI contract tests", name)
		}
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

func runtimeDBPath(t *testing.T, repo string) string {
	t.Helper()
	return filepath.Join(repo, ".git", "mindrail", "mindrail.db")
}

func configPath(repo string) string {
	return filepath.Join(repo, ".mindrail", "config.toml")
}

// writeKnowledgeRecord drops one record into the decisions directory, which is
// where the §95 reader looks first.
func writeKnowledgeRecord(t *testing.T, repo, name, content string) {
	t.Helper()

	dir := filepath.Join(repo, ".mindrail", "knowledge", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}
	writeFile(t, filepath.Join(dir, name), []byte(content))
}

// newRepoWithCommit is a repository that can carry a linked worktree: `git
// worktree add` needs a commit to check out, and an empty one is enough.
func newRepoWithCommit(t *testing.T) string {
	t.Helper()

	repo := newRepo(t)
	for _, args := range [][]string{
		{"-C", repo, "config", "user.email", "test@example.invalid"},
		{"-C", repo, "config", "user.name", "Mindrail Test"},
		{"-C", repo, "commit", "--allow-empty", "--quiet", "-m", "root"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return repo
}

// addWorktree creates a linked worktree of repo and returns its root.
func addWorktree(t *testing.T, repo, name string) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), name)
	out, err := exec.Command("git", "-C", repo, "worktree", "add", "--quiet", "-b", name, root).CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree add: %v: %s", err, out)
	}
	// Git keeps administrative state for the worktree under the common dir, and
	// leaving it behind makes the parent repository's cleanup fail on some
	// filesystems.
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", root).Run()
	})
	return root
}

// corruptDatabase replaces the runtime database with bytes that are not one.
// The write-ahead sidecars go too: leaving them behind would let SQLite recover
// the file and quietly disprove the scenario.
func corruptDatabase(t *testing.T, repo string) {
	t.Helper()

	dbPath := runtimeDBPath(t, repo)
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s%s: %v", dbPath, suffix, err)
		}
	}
	writeFile(t, dbPath, bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, 1024))
}

// recordFutureMigration writes a ledger row this binary does not know, which is
// what a downgrade looks like from the older binary's side (decision D-25).
func recordFutureMigration(t *testing.T, repo string) {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatalf("open runtime database: %v", err)
	}
	defer func() { _ = db.Close() }()

	err = storage.InTx(t.Context(), db.DB, func(ctx context.Context, tx *sql.Tx) error {
		_, execErr := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
			999999, "from_the_future", "0000", app.FormatTime(time.Unix(0, 0)))
		return execErr
	})
	if err != nil {
		t.Fatalf("record a future migration: %v", err)
	}
}

// unavailableGit is a git that cannot be executed at all, described the way
// internal/git's exec runner describes it: the code, the sentinel underneath,
// and the binary name in the metadata.
//
// It is a scripted runner rather than a mangled PATH because PATH is process
// state and the suite runs every test in one process. The condition was
// therefore unreachable from the CLI tests, and the branch that produced
// finding F11 was never exercised end to end.
func unavailableGit() *git.FakeRunner {
	return &git.FakeRunner{Default: git.FakeResponse{
		Err: app.NewError(
			app.CodeGitUnavailable,
			app.KindUnavailable,
			`the git executable "git" could not be run`,
			"Mindrail drives the system git binary and cannot operate without it",
			"install git and make sure it is on PATH",
			"or verify that the git binary is executable",
		).WithMetadata("binary", "git").
			WithCause(errors.Join(git.ErrGitUnavailable, exec.ErrNotFound)),
	}}
}

// hangingGit is a git that never answers within its deadline (decision D-30).
func hangingGit() *git.FakeRunner {
	return &git.FakeRunner{Default: git.FakeResponse{
		Err: app.NewError(
			app.CodeGitTimeout,
			app.KindUnavailable,
			"git did not answer within "+git.DefaultTimeout.String(),
			"Mindrail cannot determine the repository layout, so no command that needs it can run",
			"check whether the repository is on a slow or unresponsive filesystem",
			"remove a stale .git/index.lock left by an interrupted git command",
		).WithMetadata("timeout", git.DefaultTimeout.String()).
			WithCause(git.ErrGitTimeout),
	}}
}

// occupyCacheDirectory puts a regular file where the cache directory belongs,
// which is a condition neither `mindrail init` nor a mode change can clear.
func occupyCacheDirectory(t *testing.T, repo string) {
	t.Helper()

	cache := filepath.Join(repo, ".git", "mindrail", "cache")
	if err := os.RemoveAll(cache); err != nil {
		t.Fatalf("remove %s: %v", cache, err)
	}
	writeFile(t, cache, []byte("not a directory"))
}

// createUnmigratedDatabase reproduces the window a concurrent first run opens:
// the database file exists and is a valid SQLite database, and the migrations
// that create its tables have not committed yet.
//
// storage.Open with no migrator run is exactly what the winning process has done
// at that instant, and it is reached without any goroutine choreography, so the
// scenario is deterministic rather than a race the suite would have to hope for.
func createUnmigratedDatabase(t *testing.T, repo string) {
	t.Helper()

	runtimeRoot := filepath.Join(repo, ".git", "mindrail")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatalf("create %s: %v", runtimeRoot, err)
	}

	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(runtimeRoot, "mindrail.db")})
	if err != nil {
		t.Fatalf("create the unmigrated database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close the unmigrated database: %v", err)
	}
}

// denyWrites makes a directory readable but not writable.
func denyWrites(t *testing.T, dir string) {
	t.Helper()
	chmodForTest(t, dir, 0o500)
}

// denyAccess makes a directory or file unreachable altogether.
func denyAccess(t *testing.T, path string) {
	t.Helper()
	chmodForTest(t, path, 0o000)
}

// chmodForTest applies a restrictive mode and restores it afterwards so that
// t.TempDir's cleanup can still remove the tree. Root ignores the permission
// bits, so the scenario is unreachable there.
func chmodForTest(t *testing.T, path string, mode os.FileMode) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("permission-denied scenarios are unreachable as root")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, info.Mode().Perm()) })
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// reasonAfter returns the text following marker on the line that carries it.
func reasonAfter(out, marker string) string {
	for line := range strings.SplitSeq(out, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), marker); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// TestHumanOutputGolden pins the layout a person reads.
//
// Everything machine-local is redacted first: absolute paths, opaque ids, the
// installed git version and the elapsed milliseconds all differ per run, and a
// golden that captured them would be a golden of the machine rather than of the
// output. What is left is the part that is actually a contract — the section
// order, the state markers, the labelled blocks and the terminal line.
func TestHumanOutputGolden(t *testing.T) {
	repo := newInitializedRepo(t)

	for _, command := range []string{"status", "doctor"} {
		got := run(t, repo, command, "--no-color")
		got.requireExit(t, app.ExitSuccess)

		assertGolden(t, command+"_human.golden", redact(got.stdout, repo))
	}
}

// assertGolden compares text against testdata/<name>.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)

	if *updateGoldens {
		writeGolden(t, path, got)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v\ngot:\n%s", path, err, got)
	}
	if got != string(want) {
		t.Errorf("output differs from %s\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

// redact replaces every value that legitimately varies between machines and
// runs with a stable placeholder.
func redact(out, repo string) string {
	replacements := []struct{ pattern, placeholder string }{
		{repo, "<REPO>"},
	}
	for _, replacement := range replacements {
		out = strings.ReplaceAll(out, replacement.pattern, replacement.placeholder)
	}

	lines := strings.Split(out, "\n")
	for i, line := range lines {
		lines[i] = redactLine(line)
	}
	return strings.Join(lines, "\n")
}

// redactLine blanks the value of any field whose content is a version, an
// opaque identifier or a duration.
func redactLine(line string) string {
	volatile := []string{
		"Git version:", "git_version:",
		"Workspace id:", "workspace_id:",
		"Project id:", "project_id:",
		"Duration:",
	}
	trimmed := strings.TrimSpace(line)
	for _, label := range volatile {
		if strings.HasPrefix(trimmed, label) {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			return indent + label + " <REDACTED>"
		}
	}
	return line
}
