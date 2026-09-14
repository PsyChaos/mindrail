package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/cli"
	"github.com/PsyChaos/mindrail/internal/status"
)

// exitForCode is decision D-03's table, spelled out here as data rather than
// read from the binary.
//
// Duplicating it is the point. doctor.kindForCode is the production mapping, and
// a test that asked the production mapping what the exit code should be would
// agree with any mapping at all — including the one that let GIT_UNAVAILABLE
// exit 4 while the envelope reported success. This is the design document's
// answer, independently written down, and the invariant test compares the two.
//
// ExitSuccess here means something stronger than "exits 0": it means the code
// must never appear in an error object at all. WORKSPACE_NOT_INITIALIZED is
// decision D-03's one zero-exit row — "you have not run init yet" is a state to
// report — and a fatal envelope carrying it would be the opposite regression to
// the one this file exists for.
//
// KNOWLEDGE_INVALID joins that class for the same reason and by decision D-43:
// a record this binary read and found wrong leaves the knowledge component
// DEGRADED, and doctor.Report.Err builds an error object from ERROR readings
// only. A repository with one malformed Decision must stay usable, so the row
// being ExitSuccess is the assertion that it does. Its sibling
// KNOWLEDGE_SUPERSEDE_CYCLE is ExitFailed instead: D-39 makes a lineage with no
// end fatal, because every answer to "which Decision is current" is then wrong.
var exitForCode = map[app.Code]int{
	app.CodeNotAGitRepository:           app.ExitUsage,
	app.CodeBareRepository:              app.ExitUsage,
	app.CodeCommandLineInvalid:          app.ExitUsage,
	app.CodeConfigInvalid:               app.ExitUsage,
	app.CodePathEscapesRoot:             app.ExitUsage,
	app.CodePathNotRepresentable:        app.ExitUsage,
	app.CodeGitUnavailable:              app.ExitUnavailable,
	app.CodeGitTimeout:                  app.ExitUnavailable,
	app.CodeRuntimePathUnwritable:       app.ExitUnavailable,
	app.CodeRuntimeDBUnavailable:        app.ExitUnavailable,
	app.CodeRuntimeDBCorrupt:            app.ExitUnavailable,
	app.CodeStartupIncomplete:           app.ExitUnavailable,
	app.CodeMigrationFailed:             app.ExitFailed,
	app.CodeMigrationChecksumMismatch:   app.ExitFailed,
	app.CodeRuntimeDBSchemaTooNew:       app.ExitFailed,
	app.CodeWorkspaceRegistrationFailed: app.ExitFailed,
	app.CodeKnowledgeUnreadable:         app.ExitFailed,
	app.CodeKnowledgeSchemaUnsupported:  app.ExitFailed,
	app.CodeKnowledgeSupersedeCycle:     app.ExitFailed,
	app.CodeWorkspaceNotInitialized:     app.ExitSuccess,
	app.CodeConfigUnknownEnvVar:         app.ExitSuccess,
	app.CodeKnowledgeInvalid:            app.ExitSuccess,

	// MR-003's seven, all ExitFailed (requirement AC-05.3). Five were added by
	// the milestone; COORDINATION_UNAVAILABLE was the sixth, added during
	// TASK-05 and recorded under AC-06.7, and COORDINATION_READ_FAILED the
	// seventh, added by the round-1 remediation and recorded under AC-05.1.
	//
	// Not all of them are "the id names nothing" — the class description used
	// to say so, and two of the rows are not that. Four are operations that ran
	// against a healthy repository and could not do what they were asked: the
	// id names nothing, or the move is not available from where the task is.
	// COORDINATION_UNAVAILABLE is the repository having no coordination state
	// to operate on at all. The two write/read failures are the runtime
	// database refusing a row for a reason the storage layer does not name.
	//
	// None is ExitUsage. The command line was well formed — `task state TSK-…
	// --to COMPLETED` is a sentence this binary understands — and the refusal
	// depends on rows, not on spelling. A caller that retried after fixing its
	// arguments would meet the same answer. The spelling mistakes are caught
	// earlier and still exit 2 through COMMAND_LINE_INVALID.
	//
	// None is ExitUnavailable either, including the two failure codes: the
	// storage conditions that really are "come back later" are named by
	// storage.WriteFailure before either code is reached, and what survives is
	// a read or write that failed for a reason retrying will not fix.
	app.CodeSessionNotFound:         app.ExitFailed,
	app.CodeTaskNotFound:            app.ExitFailed,
	app.CodeTaskStateInvalid:        app.ExitFailed,
	app.CodeCheckpointNotFound:      app.ExitFailed,
	app.CodeCoordinationUnavailable: app.ExitFailed,
	app.CodeCoordinationWriteFailed: app.ExitFailed,
	app.CodeCoordinationReadFailed:  app.ExitFailed,

	// MR-004's busy code is ExitUnavailable, the class decision D-03 gives a
	// condition that clears on its own: the lock is released when the other
	// command finishes, and the remedy is to run this one again (D-74).
	app.CodeBusyRetryable: app.ExitUnavailable,

	// MR-004's lease codes are ExitFailed like MR-003's task codes (D-76): the
	// command line was well formed and the refusal depends on rows — who holds
	// the target, whether a lease has run out, whether an id names one.
	app.CodeLeaseConflict: app.ExitFailed,
	app.CodeLeaseNotHeld:  app.ExitFailed,
	app.CodeLeaseNotFound: app.ExitFailed,

	// And the revision refusal (D-72): the task exists, the session exists,
	// and the caller's reading of the task is what is stale.
	app.CodeStateRevisionConflict: app.ExitFailed,
}

// TestExitClassTableCoversEveryRegisteredCode makes the table above impossible
// to forget. A new code with no exit class is a code whose envelope and exit
// code have not been reconciled, which is the state finding F11 describes.
func TestExitClassTableCoversEveryRegisteredCode(t *testing.T) {
	for _, code := range app.RegisteredCodes() {
		if _, ok := exitForCode[code]; !ok {
			t.Errorf("code %q has no exit class in exitForCode; decide what it exits with", code)
		}
	}
	for code := range exitForCode {
		if !app.IsRegistered(code) {
			t.Errorf("exitForCode names %q, which this binary does not emit", code)
		}
	}
}

// TestTheKnowledgeExitClassesAreTheOnesTheBinaryTakes ties MR-002's two rows of
// the table above to a repository the binary was actually driven over.
//
// The table is hand-written data, and a row no run ever reads asserts nothing:
// both knowledge rows could be flipped — KNOWLEDGE_INVALID to ExitFailed,
// KNOWLEDGE_SUPERSEDE_CYCLE to ExitSuccess — with `go test ./...` staying green,
// because no envelope scenario produces a knowledge finding and the exit class is
// only consulted when a code reaches the wire inside an error object. That is the
// unfalsifiable-guard class the whole of MR-001's second audit round was spent
// deleting.
//
// So each row here observes an exit from the real command tree and compares the
// table against that, never against a literal. The condition is proved to have
// reproduced first — by the code on the wire for the fatal row, and by the
// knowledge component's own code for the exit-0 one — because a repository where
// nothing went wrong would otherwise satisfy the exit comparison for the wrong
// reason.
func TestTheKnowledgeExitClassesAreTheOnesTheBinaryTakes(t *testing.T) {
	tests := []struct {
		name string
		// records is written into the store before `status` is run. The fixtures
		// are agreement_test.go's, so the two matrices cannot end up describing
		// different repositories under one name.
		records map[string]string
		// wantCode is the code the reading has to carry for the row to be about
		// anything at all.
		wantCode app.Code
		// wantFatal says which carrier publishes it: an error object on the wire,
		// or a component of a report that reports success.
		wantFatal bool
	}{
		{
			name: "a supersede cycle is fatal and exits the class D-03 assigns it",
			records: map[string]string{
				"DEC-0001.json": cyclicSupersededDecision,
				"DEC-0002.json": supersedingDecision,
			},
			wantCode:  app.CodeKnowledgeSupersedeCycle,
			wantFatal: true,
		},
		{
			name:      "a record its schema rejects is a state and exits the class D-03 assigns it",
			records:   map[string]string{"DEC-0001.json": invalidStatusDecision},
			wantCode:  app.CodeKnowledgeInvalid,
			wantFatal: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newInitializedRepo(t)
			for name, content := range tc.records {
				writeKnowledgeRecord(t, repo, name, content)
			}

			got := runWith(t, repo, cli.Options{}, "status", "--json")
			envelope := decodeStrictEnvelope(t, got.stdout)

			if tc.wantFatal {
				if envelope.Error == nil {
					t.Fatalf("the condition did not reproduce: `status` reported ok on a repository carrying %q\n%s",
						tc.wantCode, got.stdout)
				}
				if envelope.Error.Code != tc.wantCode {
					t.Fatalf("`status` reported %q, want %q", envelope.Error.Code, tc.wantCode)
				}
			} else {
				if envelope.Error != nil {
					t.Fatalf("`status` turned %q into a fatal envelope: %+v", tc.wantCode, *envelope.Error)
				}
				var report status.Report
				decodeData(t, got.stdout, &report)
				component, present := report.Components[status.ComponentKnowledge]
				if !present {
					t.Fatalf("the status report has no %q component: %v", status.ComponentKnowledge, report.Components)
				}
				if component.Code != tc.wantCode {
					t.Fatalf("the condition did not reproduce: the knowledge component reports %q, want %q",
						component.Code, tc.wantCode)
				}
			}

			assigned, known := exitForCode[tc.wantCode]
			if !known {
				t.Fatalf("code %q has no exit class in exitForCode", tc.wantCode)
			}
			if got.code != assigned {
				t.Errorf("`status` exited %d carrying %q, but decision D-03's table assigns that code %d",
					got.code, tc.wantCode, assigned)
			}
		})
	}
}

// envelopeScenario is one repository state, reachable from the CLI.
type envelopeScenario struct {
	name    string
	setup   func(t *testing.T) string
	options cli.Options
}

// envelopeScenarios is every failure mode MR-001 can reach, healthy states
// included. The healthy rows are not filler: an invariant that only ever saw
// broken repositories could be satisfied by a binary that reports every
// repository as broken.
func envelopeScenarios() []envelopeScenario {
	return []envelopeScenario{
		{name: "healthy initialized repository", setup: newInitializedRepo},
		{name: "uninitialized repository", setup: newRepo},
		{
			name:  "not a git repository",
			setup: func(t *testing.T) string { requireGit(t); isolateEnvironment(t); return t.TempDir() },
		},
		{name: "bare repository", setup: newBareRepo},
		{
			name:    "git is not installed",
			setup:   func(t *testing.T) string { isolateEnvironment(t); return t.TempDir() },
			options: cli.Options{Runner: unavailableGit()},
		},
		{
			name:    "git does not answer in time",
			setup:   func(t *testing.T) string { isolateEnvironment(t); return t.TempDir() },
			options: cli.Options{Runner: hangingGit()},
		},
		{
			name: "unwritable runtime path",
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				denyWrites(t, filepath.Join(repo, ".git"))
				return repo
			},
		},
		{
			name: "unusable cache directory",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				occupyCacheDirectory(t, repo)
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
			name: "runtime database unreadable",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyAccess(t, runtimeDBPath(t, repo))
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
			name: "runtime directory deleted after init",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				if err := os.RemoveAll(filepath.Join(repo, ".git", "mindrail")); err != nil {
					t.Fatalf("remove the runtime directory: %v", err)
				}
				return repo
			},
		},
		{
			name: "first run still in flight",
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				createUnmigratedDatabase(t, repo)
				return repo
			},
		},
		{
			name: "schema newer than this binary",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				recordFutureMigration(t, repo)
				return repo
			},
		},
		{
			name: "configuration with a syntax error",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeFile(t, configPath(repo), []byte("[[[\n"))
				return repo
			},
		},
		{
			name: "configuration with an unknown key",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeFile(t, configPath(repo), []byte("[output]\ncolour = \"always\"\n"))
				return repo
			},
		},
		{
			name: "knowledge store unreadable",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyAccess(t, filepath.Join(repo, ".mindrail", "knowledge"))
				return repo
			},
		},
		{
			name: "malformed knowledge record",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "broken.json", "{ this is not json")
				return repo
			},
		},
		{
			name: "knowledge record from a future schema",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "future.json",
					`{"schema_version": 999, "id": "D-999", "title": "from the future"}`)
				return repo
			},
		},
	}
}

// TestEnvelopeAndExitCodeAgreeEverywhere is the invariant finding F11 asks for,
// and the reason this file exists.
//
// Before it, `ok`, the presence of the `error` object and the exit code were
// computed in three places from three different inputs, and nothing compared
// them. A missing git produced `{"ok": true}` with no error and no data while
// the process exited 4, and every test in the suite passed: the JSON tests only
// looked at healthy repositories, the exit-code matrix never read the envelope,
// and no row could break git in the first place.
//
// The three assertions are deliberately mechanical, because that is what makes
// them survive the next refactor: ok is false exactly when there is an error
// object, the exit code is what decision D-03 assigns that error's code, and a
// successful envelope exits zero. Every command, every failure mode.
func TestEnvelopeAndExitCodeAgreeEverywhere(t *testing.T) {
	commands := []string{"version", "status", "doctor", "init"}

	for _, scenario := range envelopeScenarios() {
		for _, command := range commands {
			t.Run(scenario.name+"/"+command, func(t *testing.T) {
				// Per command, because init writes: a scenario replayed against
				// an already-repaired repository is a different scenario.
				repo := scenario.setup(t)

				got := runWith(t, repo, scenario.options, command, "--json")
				envelope := decodeStrictEnvelope(t, got.stdout)

				assertEnvelopeCoherent(t, command, got, envelope)
			})
		}
	}
}

// strictEnvelope is the wire shape, decoded without help. `error` is a raw
// message so that "absent" and "present but null" stay distinguishable: a
// consumer branching on the key's presence must not be fooled by either.
type strictEnvelope struct {
	Command string            `json:"command"`
	OK      bool              `json:"ok"`
	Data    json.RawMessage   `json:"data"`
	Error   *app.ErrorPayload `json:"error"`
}

func decodeStrictEnvelope(t *testing.T, stdout string) strictEnvelope {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var envelope strictEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\nstdout:\n%q", err, stdout)
	}
	return envelope
}

// assertEnvelopeCoherent is the invariant itself.
func assertEnvelopeCoherent(t *testing.T, command string, got result, envelope strictEnvelope) {
	t.Helper()

	if envelope.Command != command {
		t.Errorf("envelope command = %q, want %q", envelope.Command, command)
	}

	// One: ok is false exactly when there is an error object. Neither direction
	// is redundant — the shipped binary failed the first, and a command that
	// reported ok:false with nothing to explain would fail the second.
	if envelope.OK != (envelope.Error == nil) {
		t.Fatalf("ok = %v with error = %v: the envelope contradicts itself\n%#v",
			envelope.OK, envelope.Error, envelope)
	}

	if envelope.Error == nil {
		// Two: a successful envelope exits zero, and says something.
		if got.code != app.ExitSuccess {
			t.Errorf("envelope reports ok with no error, but the process exits %d (%v)", got.code, got.err)
		}
		if len(envelope.Data) == 0 {
			t.Errorf("envelope reports ok with neither an error nor any data:\n%s", got.stdout)
		}
		return
	}

	payload := *envelope.Error

	// Three: the exit code is the one decision D-03 assigns this code.
	if !app.IsRegistered(payload.Code) {
		t.Fatalf("error payload carries the unregistered code %q", payload.Code)
	}
	wantExit, known := exitForCode[payload.Code]
	if !known {
		t.Fatalf("error payload carries %q, which has no exit class", payload.Code)
	}
	if wantExit == app.ExitSuccess {
		t.Fatalf("code %q reached the wire as a fatal error; decision D-03 makes it a reportable state", payload.Code)
	}
	if got.code != wantExit {
		t.Errorf("code %q exited %d, want %d", payload.Code, got.code, wantExit)
	}

	// And the error the process returns has to be the one on the wire, or the
	// exit code above agreed with the envelope by coincidence.
	recovered, ok := app.PayloadOf(got.err)
	if !ok {
		t.Fatalf("the returned error %v carries no domain payload while the envelope carries %q", got.err, payload.Code)
	}
	if recovered.Code != payload.Code {
		t.Errorf("the returned error carries %q, the envelope carries %q", recovered.Code, payload.Code)
	}

	// Spec §84's four keys, on every fatal envelope, from every command.
	for label, value := range map[string]string{"why": payload.Why, "impact": payload.Impact} {
		if strings.TrimSpace(value) == "" {
			t.Errorf("error payload for %q has an empty %s", payload.Code, label)
		}
	}
	if len(payload.NextAction) == 0 {
		t.Errorf("error payload for %q carries no next_action", payload.Code)
	}
	for _, action := range payload.NextAction {
		if strings.TrimSpace(action) == "" {
			t.Errorf("error payload for %q carries a blank next_action: %v", payload.Code, payload.NextAction)
		}
	}
}

// TestHumanOutputIsNeverEmpty is finding F12.
//
// The regression it guards is precise: `status` in a repository with no git
// wrote nothing at all to stdout — no report, no structured error, no next
// action — and left the user one bare line on stderr and exit 4. Acceptance
// criterion 4 says in as many words that status reports a missing setup with a
// structured error and a suggested next action, and that is a promise about the
// human surface as much as about `--json`.
func TestHumanOutputIsNeverEmpty(t *testing.T) {
	for _, scenario := range envelopeScenarios() {
		for _, command := range []string{"status", "doctor", "init"} {
			t.Run(scenario.name+"/"+command, func(t *testing.T) {
				repo := scenario.setup(t)

				got := runWith(t, repo, scenario.options, command, "--no-color")

				if strings.TrimSpace(got.stdout) == "" {
					t.Fatalf("%s wrote nothing to stdout (exit %d, error %v)", command, got.code, got.err)
				}
				if got.err == nil {
					return
				}

				// A failure owes the reader the same four things the envelope
				// carries, in the block app.RenderError writes.
				payload, ok := app.PayloadOf(got.err)
				if !ok {
					t.Fatalf("%s failed with %v, which carries no domain payload", command, got.err)
				}
				for _, want := range []string{string(payload.Code), payload.Why, payload.Impact} {
					if want == "" {
						continue
					}
					if !strings.Contains(got.stdout, want) {
						t.Errorf("%s human output does not carry %q:\n%s", command, want, got.stdout)
					}
				}
				if len(payload.NextAction) == 0 {
					t.Fatalf("%s failed with %q and no next action", command, payload.Code)
				}
				for _, action := range payload.NextAction {
					if !strings.Contains(got.stdout, action) {
						t.Errorf("%s human output does not carry the next action %q:\n%s", command, action, got.stdout)
					}
				}
			})
		}
	}
}

// TestHumanErrorNamesTheProbedDirectory is finding F17.
//
// `start_dir` has reached the JSON envelope since the last pass, so a machine
// could answer "which directory did Mindrail actually look at?" and a person
// could not: no human renderer read DomainError.Metadata. The person is the one
// who runs the binary under -C or out of a Git hook, where the shell prompt is
// not the answer — which is the whole reason acceptance criterion 3 asks for it.
func TestHumanErrorNamesTheProbedDirectory(t *testing.T) {
	requireGit(t)
	isolateEnvironment(t)
	dir := t.TempDir()

	for _, command := range []string{"status", "doctor", "init"} {
		t.Run(command, func(t *testing.T) {
			got := run(t, dir, command, "--no-color")
			got.requireExit(t, app.ExitUsage)

			if !strings.Contains(got.stdout, dir) {
				t.Errorf("%s never names the directory it probed (%s):\n%s", command, dir, got.stdout)
			}

			// The JSON and the human block must agree about it, or one of them
			// is naming a directory the other did not probe.
			jsonRun := run(t, dir, command, "--json")
			payload := jsonRun.errorPayload(t)
			if payload.Metadata["start_dir"] != dir {
				t.Errorf("start_dir metadata = %q, want %q", payload.Metadata["start_dir"], dir)
			}
		})
	}
}

// TestUnusableCacheDirectoryLeavesTheRepositoryUsable is the over-fire guard of
// finding F13 at the command layer.
//
// The remediation this replaces fired on a directory MR-001 never writes to and
// drove a fully working, initialised, registered repository to ERROR, BLOCKED
// and exit 4 on all three commands. Every claim below is about the repository
// still working, because that is what the regression took away.
func TestUnusableCacheDirectoryLeavesTheRepositoryUsable(t *testing.T) {
	repo := newInitializedRepo(t)
	occupyCacheDirectory(t, repo)

	for _, command := range []string{"status", "doctor", "init"} {
		got := runWith(t, repo, cli.Options{}, command, "--json")
		got.requireExit(t, app.ExitSuccess)
		assertNoErrorEnvelope(t, command, got.stdout)
	}

	var report status.Report
	statusRun := run(t, repo, "status", "--json")
	decodeData(t, statusRun.stdout, &report)

	if report.Readiness == status.ReadinessBlocked {
		t.Errorf("readiness = BLOCKED on a repository whose only fault is an unusable cache directory")
	}
	if report.BlockingComponent != "" {
		t.Errorf("blocking_component = %q; nothing here blocks work", report.BlockingComponent)
	}
	if !report.Runtime.Observation.Known() || !report.Runtime.Initialized {
		t.Errorf("runtime = %+v, want an observed, initialized store", report.Runtime)
	}
	if !report.Workspace.Registered {
		t.Error("workspace is reported unregistered although init registered it")
	}

	// And the terminal line a CI job greps for still says the repository is
	// ready, which is the sentence the regression replaced with BLOCKED.
	initRun := run(t, repo, "init")
	initRun.requireExit(t, app.ExitSuccess)
	if !strings.Contains(initRun.stdout, terminalReady) {
		t.Errorf("init does not report %q:\n%s", terminalReady, initRun.stdout)
	}
}

// TestUnwritableRuntimeRootIsStillFatalEndToEnd is the condition the fix above
// is *not* trying to relax. Grading the cache down is only safe while the
// runtime root — which holds the database every command reads — still stops the
// commands it has always stopped.
func TestUnwritableRuntimeRootIsStillFatalEndToEnd(t *testing.T) {
	repo := newRepo(t)
	denyWrites(t, filepath.Join(repo, ".git"))

	for _, command := range []string{"status", "doctor", "init"} {
		got := run(t, repo, command, "--json")
		got.requireExit(t, app.ExitUnavailable)

		payload := got.errorPayload(t)
		if payload.Code != app.CodeRuntimePathUnwritable {
			t.Errorf("%s reported %q, want %q", command, payload.Code, app.CodeRuntimePathUnwritable)
		}
	}
}

// TestInitNeverClaimsASchemaItDidNotEstablish is finding F14.
//
// "Migrations applied: none (schema already current)" printed on every run that
// applied nothing, including runs that stopped before a database existed — a
// demonstrably false statement about the schema, and one a golden file had come
// to pin.
func TestInitNeverClaimsASchemaItDidNotEstablish(t *testing.T) {
	const currentClaim = "schema already current"

	t.Run("blocked before a database existed", func(t *testing.T) {
		repo := newRepo(t)
		denyWrites(t, filepath.Join(repo, ".git"))

		got := run(t, repo, "init", "--no-color")

		if strings.Contains(got.stdout, currentClaim) {
			t.Errorf("init claims %q although no database was ever opened:\n%s", currentClaim, got.stdout)
		}

		var report status.InitReport
		jsonRun := run(t, repo, "init", "--json")
		decodeData(t, jsonRun.stdout, &report)
		if report.SchemaCurrent {
			t.Error("schema_current = true although no database was ever opened")
		}
	})

	t.Run("second run on a current schema", func(t *testing.T) {
		repo := newInitializedRepo(t)

		got := run(t, repo, "init", "--no-color")
		got.requireExit(t, app.ExitSuccess)

		// The over-fire guard: the claim is true here and must still be made,
		// or the fix has traded a false statement for a missing one.
		if !strings.Contains(got.stdout, currentClaim) {
			t.Errorf("init does not report %q on a repository whose schema really is current:\n%s",
				currentClaim, got.stdout)
		}

		var report status.InitReport
		jsonRun := run(t, repo, "init", "--json")
		decodeData(t, jsonRun.stdout, &report)
		if !report.SchemaCurrent {
			t.Error("schema_current = false on a repository whose schema is current")
		}
		if len(report.MigrationsApplied) != 0 {
			t.Errorf("a second init applied %d migrations", len(report.MigrationsApplied))
		}
	})
}

// TestStatusDistinguishesLookedAndCouldNotTell is finding F15.
//
// Three states, and the test asserts all three from the CLI because two of them
// used to collapse: `Inspected` was `Code != STARTUP_INCOMPLETE`, so a check
// that ran and failed counted as inspected and its zero values were published as
// findings about the repository.
func TestStatusDistinguishesLookedAndCouldNotTell(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  status.Observation
	}{
		{
			name:  "looked and knows",
			setup: newInitializedRepo,
			want:  status.Observed,
		},
		{
			// "The database path was inspected and is empty" is an observation,
			// and `initialized: false` is exactly what it observed. This row is
			// the over-fire guard: an UNAVAILABLE reading is still a reading.
			name:  "looked and found nothing there",
			setup: newRepo,
			want:  status.Observed,
		},
		{
			name: "looked and could not tell",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyAccess(t, runtimeDBPath(t, repo))
				return repo
			},
			want: status.Indeterminate,
		},
		{
			// The row that isolates the third state. In every other failure the
			// startup sequence also halts, so the block contains a reading that
			// never ran and NOT_OBSERVED would answer for it — which is why a
			// two-value model could still look right. Here the database opened
			// cleanly and only the ledger was refused: one reading ran and
			// succeeded, the other ran and failed, and nothing in the block was
			// left unread. `schema_version: 0` about a database sitting at
			// version 1 is exactly the false finding of F15.
			name: "read the database and could not read its ledger",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				recordFutureMigration(t, repo)
				return repo
			},
			want: status.Indeterminate,
		},
		{
			name: "never looked",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeFile(t, configPath(repo), []byte("[[[\n"))
				return repo
			},
			want: status.NotObserved,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.setup(t)

			got := run(t, repo, "status", "--json")
			if len(got.stdout) == 0 {
				t.Fatalf("status wrote nothing (exit %d, error %v)", got.code, got.err)
			}

			var envelope struct {
				Data *status.Report `json:"data"`
			}
			if err := json.Unmarshal([]byte(got.stdout), &envelope); err != nil {
				t.Fatalf("decode status envelope: %v\n%s", err, got.stdout)
			}
			if envelope.Data == nil {
				// A startup that never derived the runtime locations emits the
				// error alone; the observation question does not arise.
				if tc.want != status.NotObserved {
					t.Fatalf("status emitted no report for %q", tc.name)
				}
				return
			}

			if envelope.Data.Runtime.Observation != tc.want {
				t.Errorf("runtime.observation = %q, want %q", envelope.Data.Runtime.Observation, tc.want)
			}

			// The values are only publishable as facts when the report can vouch
			// for them; the human view has to agree with the JSON about that, and
			// about the value itself.
			//
			// The marker used to be the word "unknown" in place of the value,
			// which made the two views disagree: the human said `Initialized:
			// unknown` while the JSON beside it said `"initialized": false` about
			// the same repository at the same instant. The value is now printed in
			// both, with the observation the JSON carries spelled out beside it in
			// the human one.
			human := run(t, repo, "status", "--no-color")
			runtimeBlock := sectionOf(human.stdout, "Runtime")
			if tc.want == status.Observed {
				for _, marker := range []string{string(status.NotObserved), string(status.Indeterminate)} {
					if strings.Contains(runtimeBlock, "("+marker+")") {
						t.Errorf("an observed runtime block disclaims its own values:\n%s", runtimeBlock)
					}
				}
				return
			}
			for label, want := range map[string]string{
				"Schema version:": strconv.FormatInt(envelope.Data.Runtime.SchemaVersion, 10),
				"Initialized:":    strconv.FormatBool(envelope.Data.Runtime.Initialized),
			} {
				line := lineWithPrefix(runtimeBlock, label)
				if line == "" {
					t.Errorf("runtime block has no %q line:\n%s", label, runtimeBlock)
					continue
				}
				if !strings.HasSuffix(line, "("+string(tc.want)+")") {
					t.Errorf("%q is published as a fact although the report cannot vouch for it: %q", label, line)
				}
				if !strings.Contains(line, want) {
					t.Errorf("%q prints %q while the JSON member of the same report carries %q; "+
						"two renderings of one report may not disagree", label, line, want)
				}
			}
		})
	}
}

// TestFirstRunInFlightIsReportedAsNotInitialized is finding F16.
//
// A second process that opens the database between another process's create and
// its migration commit used to fail hard with SQLite's own
// `no such table: workspaces (1)` as the user-facing why line, at exit 1. The
// condition is "not initialised yet", the ledger already says so, and the remedy
// is the one command that fixes it.
func TestFirstRunInFlightIsReportedAsNotInitialized(t *testing.T) {
	repo := newRepo(t)
	createUnmigratedDatabase(t, repo)

	for _, command := range []string{"status", "doctor"} {
		got := run(t, repo, command, "--json")
		got.requireExit(t, app.ExitSuccess)
		assertNoErrorEnvelope(t, command, got.stdout)

		if strings.Contains(got.stdout, "no such table") {
			t.Errorf("%s surfaces a raw SQLite message:\n%s", command, got.stdout)
		}
		if strings.Contains(got.stdout, string(app.CodeWorkspaceRegistrationFailed)) {
			t.Errorf("%s reports a registration failure for a repository that was never initialised:\n%s",
				command, got.stdout)
		}
	}

	var report status.Report
	statusRun := run(t, repo, "status", "--json")
	decodeData(t, statusRun.stdout, &report)

	if report.Readiness != status.ReadinessBlocked {
		t.Errorf("readiness = %q, want BLOCKED", report.Readiness)
	}
	if !slices.Contains(report.NextAction, "mindrail init") {
		t.Errorf("next_action = %v, want `mindrail init`", report.NextAction)
	}

	// And the advice works, which is the only proof that the reading is right.
	if got := run(t, repo, "init"); got.code != app.ExitSuccess {
		t.Errorf("the report's own next action `mindrail init` exited %d: %v\n%s", got.code, got.err, got.stdout)
	}
	if got := run(t, repo, "status", "--json"); got.code != app.ExitSuccess {
		t.Errorf("status after init exited %d: %v\n%s", got.code, got.err, got.stdout)
	}
}

// sectionOf returns the lines of one titled block of the human status report.
func sectionOf(out, title string) string {
	lines := strings.Split(out, "\n")
	start := slices.Index(lines, title)
	if start < 0 {
		return ""
	}
	for end := start + 1; end < len(lines); end++ {
		if strings.TrimSpace(lines[end]) == "" {
			return strings.Join(lines[start:end], "\n")
		}
	}
	return strings.Join(lines[start:], "\n")
}

// lineWithPrefix returns the trimmed line whose trimmed form starts with prefix.
func lineWithPrefix(block, prefix string) string {
	for line := range strings.SplitSeq(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(trimmed)
		}
	}
	return ""
}
