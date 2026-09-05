package doctor

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/workspace"
)

// TestDefaultChecksNamesAndSections pins decision D-10: seven checks, in report
// order, and not one more. A check registered for a subsystem this binary does
// not have would report NOT_APPLICABLE forever and tell the reader that the
// capability exists but is unused, which is a different and false claim.
func TestDefaultChecksNamesAndSections(t *testing.T) {
	want := []struct {
		name    string
		section string
	}{
		{name: "git", section: "Repository"},
		{name: "runtime_paths", section: "Runtime paths"},
		{name: "config", section: "Configuration"},
		{name: "sqlite", section: "Runtime store"},
		{name: "migrations", section: "Runtime store"},
		{name: "knowledge", section: "Knowledge"},
		{name: "workspace", section: "Workspace"},
	}

	checks := DefaultChecks(healthySubject())
	if len(checks) != len(want) {
		t.Fatalf("DefaultChecks returned %d checks, want exactly %d", len(checks), len(want))
	}

	report := NewRunner(checks...).Run(t.Context())
	for i, wc := range want {
		if got := checks[i].Name(); got != wc.name {
			t.Errorf("check %d is named %q, want %q", i, got, wc.name)
		}
		if got := report.Checks[i].Name; got != wc.name {
			t.Errorf("result %d is named %q, want %q", i, got, wc.name)
		}
		if got := report.Checks[i].Section; got != wc.section {
			t.Errorf("check %q is in section %q, want %q", wc.name, got, wc.section)
		}
	}

	// Sections must stay contiguous: the human renderer prints one header per
	// run of checks, and a section that reappeared later would print twice.
	seen := map[string]bool{}
	previous := ""
	for _, result := range report.Checks {
		if result.Section == previous {
			continue
		}
		if seen[result.Section] {
			t.Errorf("section %q is not contiguous in report order", result.Section)
		}
		seen[result.Section] = true
		previous = result.Section
	}
}

// TestDefaultChecksRegisterNoAbsentSubsystem is the other half of D-10: the
// checks that belong to milestones MR-001 does not implement must be absent,
// not present-and-skipped.
func TestDefaultChecksRegisterNoAbsentSubsystem(t *testing.T) {
	forbidden := []string{"resolver", "parser", "index", "coverage", "hook", "ci", "unit", "validation"}

	for _, check := range DefaultChecks(healthySubject()) {
		for _, word := range forbidden {
			if strings.Contains(check.Name(), word) {
				t.Errorf("check %q claims the %q subsystem, which MR-001 does not implement", check.Name(), word)
			}
		}
	}
}

// TestGitCheckBranches walks the states repository discovery can produce.
// Acceptance criterion 3 lives in the healthy row: the common dir has to be
// reported, not merely known.
func TestGitCheckBranches(t *testing.T) {
	notARepo := app.NewError(
		app.CodeNotAGitRepository,
		app.KindUsage,
		"No Git repository contains /tmp/elsewhere.",
		"Mindrail cannot locate the repository, so no command can run.",
		"Run mindrail from inside a Git repository.",
	).WithCause(git.ErrNotARepository)

	tests := []struct {
		name      string
		subject   Subject
		wantState State
		wantCode  app.Code
	}{
		{
			name:      "healthy repository",
			subject:   healthySubject(),
			wantState: StateOK,
		},
		{
			name: "not a repository",
			subject: func() Subject {
				s := healthySubject()
				s.Repo = git.Repository{}
				s.RepoErr = notARepo
				return s
			}(),
			wantState: StateError,
			wantCode:  app.CodeNotAGitRepository,
		},
		{
			name: "bare repository",
			subject: func() Subject {
				s := healthySubject()
				s.Repo.IsBare = true
				s.Repo.WorktreeRoot = ""
				return s
			}(),
			wantState: StateError,
			wantCode:  app.CodeBareRepository,
		},
		{
			name: "git binary missing",
			subject: func() Subject {
				s := healthySubject()
				s.Repo = git.Repository{}
				s.RepoErr = fmt.Errorf("locate git: %w", git.ErrGitUnavailable)
				return s
			}(),
			wantState: StateUnavailable,
			wantCode:  app.CodeGitUnavailable,
		},
		{
			name: "git timed out",
			subject: func() Subject {
				s := healthySubject()
				s.Repo = git.Repository{}
				s.RepoErr = fmt.Errorf("rev-parse: %w", git.ErrGitTimeout)
				return s
			}(),
			wantState: StateUnavailable,
			wantCode:  app.CodeGitTimeout,
		},
		{
			name: "discovery never ran",
			subject: func() Subject {
				s := healthySubject()
				s.Repo = git.Repository{}
				return s
			}(),
			wantState: StateUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GitCheck(tc.subject).Run(t.Context())

			if got.State != tc.wantState {
				t.Fatalf("state = %q, want %q (%+v)", got.State, tc.wantState, got)
			}
			if tc.wantCode != "" && got.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", got.Code, tc.wantCode)
			}

			if tc.wantState == StateOK {
				if got.Summary != "Git common-dir detected" {
					t.Errorf("summary = %q, want %q", got.Summary, "Git common-dir detected")
				}
				if got.Details["common_dir"] != tc.subject.Repo.CommonDir {
					t.Errorf("common_dir detail = %q, want %q", got.Details["common_dir"], tc.subject.Repo.CommonDir)
				}
				if got.Details["worktree_root"] != tc.subject.Repo.WorktreeRoot {
					t.Errorf("worktree_root detail = %q, want %q", got.Details["worktree_root"], tc.subject.Repo.WorktreeRoot)
				}
				return
			}
			assertDiagnosable(t, got)
		})
	}
}

// TestSQLiteFailureDoesNotSpreadUpstream proves the checks are independent
// readings of one subject rather than a cascade: a single broken subsystem that
// painted the checks above it red would bury the one diagnostic that matters.
//
// It used to require the checks *below* sqlite to report OK as well. That was
// the finding, not the contract: bootstrap stops at the first hard failure, so
// those subsystems were never inspected, and calling them healthy is the same
// fabrication as calling them broken. They must be explicitly unreported.
func TestSQLiteFailureDoesNotSpreadUpstream(t *testing.T) {
	subject := haltedSubject(stepOpenSQLite)
	subject.DBErr = app.NewError(
		app.CodeRuntimeDBCorrupt,
		app.KindUnavailable,
		"file is not a database",
		"Runtime state cannot be read or written.",
		"Move the runtime database aside and rebuild it.",
	).WithCause(storage.ErrCorrupt)

	report := NewRunner(DefaultChecks(subject)...).Run(t.Context())

	upstream := map[string]bool{checkGit: true, checkConfig: true, checkRuntimePaths: true}
	for _, result := range report.Checks {
		switch {
		case result.Name == checkSQLite:
			if result.State != StateError {
				t.Errorf("sqlite reported %q, want %q", result.State, StateError)
			}
			if result.Code != app.CodeRuntimeDBCorrupt {
				t.Errorf("sqlite code = %q, want %q", result.Code, app.CodeRuntimeDBCorrupt)
			}
		case upstream[result.Name]:
			if result.State != StateOK {
				t.Errorf("check %q reported %q, want %q: a broken database must not spread upstream",
					result.Name, result.State, StateOK)
			}
		default:
			if result.Code != app.CodeStartupIncomplete {
				t.Errorf("check %q reported %q on a subsystem startup never reached, want %q",
					result.Name, result.Code, app.CodeStartupIncomplete)
			}
		}
	}
}

// TestKnowledgeCheckAbsentTreeIsOK pins decision D-06. A clean clone of a
// repository that has no records is a bare install, not a broken one.
func TestKnowledgeCheckAbsentTreeIsOK(t *testing.T) {
	subject := healthySubject()
	subject.Knowledge = loader.Store{
		Present:                false,
		Root:                   loader.StoreRoot,
		WriteSchemaVersion:     1,
		ReadableSchemaVersions: []int{1},
	}

	got := KnowledgeCheck(subject).Run(t.Context())

	if got.State != StateOK {
		t.Fatalf("absent knowledge tree reported %q, want %q (%+v)", got.State, StateOK, got)
	}
	if got.Code != "" {
		t.Errorf("absent knowledge tree carries code %q, want none", got.Code)
	}
}

// TestOptionalCapabilitiesAreNotErrors pins §84 through the report as a whole:
// a healthy repository with no index, no resolver, no hook and no CI config
// must exit clean.
func TestOptionalCapabilitiesAreNotErrors(t *testing.T) {
	subject := healthySubject()
	subject.Knowledge = loader.Store{
		Present:                false,
		Root:                   loader.StoreRoot,
		WriteSchemaVersion:     1,
		ReadableSchemaVersions: []int{1},
	}

	report := NewRunner(DefaultChecks(subject)...).Run(t.Context())

	for _, result := range report.Checks {
		if result.State == StateError {
			t.Errorf("check %q reported ERROR on a healthy bare install: %+v", result.Name, result)
		}
	}
	if report.WorstState != StateOK {
		t.Errorf("WorstState = %q, want %q", report.WorstState, StateOK)
	}
	if err := report.Err(); err != nil {
		t.Errorf("Err() = %v, want nil", err)
	}
	if got := app.ExitCode(report.Err()); got != app.ExitSuccess {
		t.Errorf("exit code = %d, want %d", got, app.ExitSuccess)
	}
}

// TestEveryNonOKResultCarriesDiagnosticImpactNextAction is MR-001 acceptance
// criterion 4. Each row drives one check into one of its non-OK branches, and
// the assertion walks every result the runner produced: a report that says
// something is wrong without saying what it costs or what to do about it is the
// failure mode this whole package exists to prevent.
func TestEveryNonOKResultCarriesDiagnosticImpactNextAction(t *testing.T) {
	uninitialized := func() Subject {
		s := healthySubject()
		s.DBPresent = false
		s.Pragmas = storage.Pragmas{}
		s.Migrations = nil
		s.Workspace = workspace.Workspace{}
		s.WorkspaceErr = workspace.ErrNotRegistered
		return s
	}

	tests := []struct {
		name    string
		subject Subject
		// wantNonOK names the checks this row is designed to push off OK.
		wantNonOK []string
	}{
		{
			name: "repository not resolved",
			subject: func() Subject {
				s := healthySubject()
				s.Repo = git.Repository{}
				s.RepoErr = app.NewError(app.CodeNotAGitRepository, app.KindUsage,
					"No Git repository contains /tmp/elsewhere.",
					"Mindrail cannot locate the repository.",
					"Run mindrail from inside a Git repository.")
				s.Paths = filesystem.RuntimePaths{}
				return s
			}(),
			wantNonOK: []string{"git", "runtime_paths"},
		},
		{
			name: "bare repository",
			subject: func() Subject {
				s := healthySubject()
				s.Repo.IsBare = true
				return s
			}(),
			wantNonOK: []string{"git"},
		},
		{
			name: "git unavailable",
			subject: func() Subject {
				s := healthySubject()
				s.Repo = git.Repository{}
				s.RepoErr = fmt.Errorf("run git: %w", git.ErrGitUnavailable)
				s.Paths = filesystem.RuntimePaths{}
				return s
			}(),
			wantNonOK: []string{"git", "runtime_paths"},
		},
		{
			name: "runtime path unwritable",
			subject: func() Subject {
				s := healthySubject()
				s.PathsErr = app.NewError(app.CodeRuntimePathUnwritable, app.KindUnavailable,
					"mkdir /repo/.git/mindrail: permission denied",
					"Mindrail cannot store runtime state for this repository.",
					"Make the Git common directory writable.")
				return s
			}(),
			wantNonOK: []string{"runtime_paths"},
		},
		{
			name: "config rejected",
			subject: func() Subject {
				s := healthySubject()
				s.ConfigErr = app.NewError(app.CodeConfigInvalid, app.KindUsage,
					`unknown configuration key "output.colour"`,
					"Mindrail refuses to start on a configuration it cannot fully understand.",
					"Remove or correct the reported key in .mindrail/config.toml.")
				return s
			}(),
			wantNonOK: []string{"config"},
		},
		{
			name: "config warnings",
			subject: func() Subject {
				s := healthySubject()
				s.Config.Warnings = []app.Warning{{
					Code:    app.CodeConfigUnknownEnvVar,
					Message: `unknown environment variable "MINDRAIL_NOT_A_KEY"`,
				}}
				return s
			}(),
			wantNonOK: []string{"config"},
		},
		{
			name: "database unopenable",
			subject: func() Subject {
				s := healthySubject()
				s.DBErr = app.NewError(app.CodeRuntimeDBUnavailable, app.KindUnavailable,
					"database is locked",
					"Nothing that reads or writes runtime state can run.",
					"Retry once the other Mindrail process has finished.")
				return s
			}(),
			wantNonOK: []string{"sqlite"},
		},
		{
			name: "database fails its integrity check",
			subject: func() Subject {
				s := healthySubject()
				s.IntegrityErr = fmt.Errorf("integrity_check: %w", storage.ErrCorrupt)
				return s
			}(),
			wantNonOK: []string{"sqlite"},
		},
		{
			name: "migration failed",
			subject: func() Subject {
				s := healthySubject()
				s.MigrateErr = app.NewError(app.CodeMigrationChecksumMismatch, app.KindFailed,
					"migration 000001_initial.sql changed after it was applied",
					"The runtime schema no longer matches the migration that produced it.",
					"Add a new migration instead of editing an applied one.")
				return s
			}(),
			wantNonOK: []string{"migrations"},
		},
		{
			name: "migrations pending",
			subject: func() Subject {
				s := healthySubject()
				s.PendingCount = 2
				return s
			}(),
			wantNonOK: []string{"migrations"},
		},
		{
			name: "knowledge unreadable",
			subject: func() Subject {
				s := healthySubject()
				s.KnowledgeErr = app.NewError(app.CodeKnowledgeUnreadable, app.KindFailed,
					"open .mindrail/knowledge: permission denied",
					"No decision or invariant is visible to any command.",
					"Make .mindrail/knowledge readable.")
				return s
			}(),
			wantNonOK: []string{"knowledge"},
		},
		{
			name: "knowledge record written by a newer Mindrail",
			subject: func() Subject {
				s := healthySubject()
				s.Knowledge.Problems = []loader.Problem{{
					Path:    ".mindrail/knowledge/decisions/dec-9.json",
					Code:    app.CodeKnowledgeSchemaUnsupported,
					Message: "schema_version 3 is outside the reader window",
					Fatal:   true,
				}}
				return s
			}(),
			wantNonOK: []string{"knowledge"},
		},
		{
			name: "one unreadable knowledge record",
			subject: func() Subject {
				s := healthySubject()
				s.Knowledge.Problems = []loader.Problem{{
					Path:    ".mindrail/knowledge/decisions/dec-9.json",
					Code:    app.CodeKnowledgeUnreadable,
					Message: "invalid character '}' looking for beginning of object key string",
				}}
				return s
			}(),
			wantNonOK: []string{"knowledge"},
		},
		{
			name: "workspace registration failed",
			subject: func() Subject {
				s := healthySubject()
				s.Workspace = workspace.Workspace{}
				s.WorkspaceErr = app.NewError(app.CodeWorkspaceRegistrationFailed, app.KindFailed,
					"insert workspace: constraint failed",
					"Runtime state cannot be attributed to this worktree.",
					"Run mindrail init again.")
				return s
			}(),
			wantNonOK: []string{"workspace"},
		},
		{
			name:      "runtime never initialized",
			subject:   uninitialized(),
			wantNonOK: []string{"sqlite", "migrations", "workspace"},
		},
		// The rows above mutate one field of a healthy subject, so every check
		// downstream still sees populated inputs. These drive the shape
		// bootstrap actually produces — a halt, with everything after it zero —
		// which is the branch none of the rows above can reach.
		{
			name:      "startup halted at load_config",
			subject:   haltedSubject(stepLoadConfig),
			wantNonOK: []string{"config", "runtime_paths", "sqlite", "migrations", "knowledge", "workspace"},
		},
		{
			name:      "startup halted at resolve_runtime_paths",
			subject:   haltedSubject(stepResolveRuntimePaths),
			wantNonOK: []string{"runtime_paths", "sqlite", "migrations", "knowledge", "workspace"},
		},
		{
			name:      "startup halted at migrate_db",
			subject:   haltedSubject(stepMigrateDB),
			wantNonOK: []string{"migrations", "knowledge", "workspace"},
		},
	}

	// Coverage bookkeeping: the table has to reach every check and every non-OK
	// state, or a branch could rot untested behind a green suite.
	coveredChecks := map[string]bool{}
	coveredStates := map[State]bool{}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := NewRunner(DefaultChecks(tc.subject)...).Run(t.Context())

			nonOK := map[string]bool{}
			for _, result := range report.Checks {
				if result.State == StateOK {
					continue
				}
				nonOK[result.Name] = true
				coveredChecks[result.Name] = true
				coveredStates[result.State] = true
				assertDiagnosable(t, result)
			}

			for _, name := range tc.wantNonOK {
				if !nonOK[name] {
					t.Errorf("check %q stayed OK; this row is meant to drive it off the happy path", name)
				}
			}
		})
	}

	for _, check := range DefaultChecks(healthySubject()) {
		if !coveredChecks[check.Name()] {
			t.Errorf("no table row drives check %q into a non-OK branch", check.Name())
		}
	}
	for _, state := range []State{StateDegraded, StateError, StateUnavailable} {
		if !coveredStates[state] {
			t.Errorf("no table row produces state %q", state)
		}
	}
}

// assertDiagnosable is acceptance criterion 4 expressed once: a result that is
// not OK owes its reader a code, a diagnostic, an impact and at least one next
// action that can actually be carried out.
//
// The earlier version of this helper allowed an empty code and accepted any
// non-blank string as a next action, which is how a reading with no code at all
// and a remedy that was a statement of fact both passed a green suite.
func assertDiagnosable(t *testing.T, result Result) {
	t.Helper()

	if result.State == StateOK {
		return
	}
	if strings.TrimSpace(result.Summary) == "" {
		t.Errorf("check %q reported %q with no summary", result.Name, result.State)
	}
	if strings.TrimSpace(result.Diagnostic) == "" {
		t.Errorf("check %q reported %q with no diagnostic", result.Name, result.State)
	}
	if strings.TrimSpace(result.Impact) == "" {
		t.Errorf("check %q reported %q with no impact", result.Name, result.State)
	}
	if result.Code == "" {
		t.Errorf("check %q reported %q with no code; a consumer cannot branch on prose", result.Name, result.State)
	}
	if result.Code != "" && !app.IsRegistered(result.Code) {
		t.Errorf("check %q reported unregistered code %q", result.Name, result.Code)
	}
	if len(result.NextAction) == 0 {
		t.Errorf("check %q reported %q with no next_action", result.Name, result.State)
	}
	for i, action := range result.NextAction {
		assertActionable(t, result.Name, i, action, "mindrail doctor")
	}
}

// declarativeOpeners begin a sentence that states a fact rather than asking for
// one. "This binary reads schema 1." is true, useful and not a next action:
// there is nothing in it for the reader to do.
var declarativeOpeners = []string{"This ", "There ", "It ", "Mindrail cannot ", "No "}

// assertActionable is the other half of §84's next_action contract: a remedy
// has to be something the reader can carry out, and it must not be the command
// that produced the report — advising `mindrail doctor` inside a doctor report
// is a loop, not a remedy.
func assertActionable(t *testing.T, owner string, index int, action, runningCommand string) {
	t.Helper()

	if strings.TrimSpace(action) == "" {
		t.Errorf("%s next_action[%d] is blank", owner, index)
		return
	}
	if strings.Contains(action, runningCommand) {
		t.Errorf("%s next_action[%d] = %q, which is the command that produced this report", owner, index, action)
	}
	for _, opener := range declarativeOpeners {
		if strings.HasPrefix(action, opener) {
			t.Errorf("%s next_action[%d] = %q is a statement, not an action", owner, index, action)
		}
	}
}

// fabricatedClaims are the sentences that made the original report untrue: each
// one asserts the outcome of an inspection that never happened. They are
// matched as literals so a reworded regression still fails on the shape rather
// than passing on new prose.
var fabricatedClaims = []string{
	"No workspace row exists",
	"Knowledge store absent",
	"not derived",
	"the repository was never resolved",
	"There is no runtime database",
	"no migration has ever been applied",
	"Runtime database not initialized",
}

// TestNoCheckAssertsAFactAboutAStepThatNeverRan is the systemic assertion the
// per-field tables could not make.
//
// Startup is a sequence: when it aborts at step N, the subsystems after it were
// never inspected, and every check downstream of the halt has to say so. It
// walks every halt point and every check, and demands of each blocked reading
// that it name the step that stopped the run, carry STARTUP_INCOMPLETE, and
// claim nothing at all about the subsystem it is named after.
func TestNoCheckAssertsAFactAboutAStepThatNeverRan(t *testing.T) {
	halts := []step{
		stepResolveRepository,
		stepLoadConfig,
		stepResolveRuntimePaths,
		stepOpenSQLite,
		stepMigrateDB,
		stepValidateKnowledge,
	}

	for _, halted := range halts {
		t.Run(string(halted), func(t *testing.T) {
			subject := haltedSubject(halted)
			report := NewRunner(DefaultChecks(subject)...).Run(t.Context())
			haltIndex := slices.Index(stepOrder, halted)

			blocked := 0
			for _, result := range report.Checks {
				assertDiagnosable(t, result)

				owned, known := stepOfCheck[result.Name]
				if !known {
					t.Fatalf("check %q is not mapped to a startup step", result.Name)
				}

				if slices.Index(stepOrder, owned) <= haltIndex {
					// This step ran. Its check has a real observation to report,
					// so it must not be excused as never having run.
					if result.Code == app.CodeStartupIncomplete {
						t.Errorf("check %q ran but reported %q", result.Name, app.CodeStartupIncomplete)
					}
					continue
				}

				blocked++
				if result.State != StateUnavailable {
					t.Errorf("check %q was never run but reported %q, want %q",
						result.Name, result.State, StateUnavailable)
				}
				if result.Code != app.CodeStartupIncomplete {
					t.Errorf("check %q was never run but reported code %q, want %q",
						result.Name, result.Code, app.CodeStartupIncomplete)
				}
				if got := result.Metadata["stopped_at_step"]; got != string(halted) {
					t.Errorf("check %q reports stopped_at_step %q, want %q", result.Name, got, halted)
				}
				if !strings.Contains(result.Diagnostic, string(halted)) {
					t.Errorf("check %q does not name the step that stopped the run: %q", result.Name, result.Diagnostic)
				}
				if slices.Contains(result.NextAction, initCommand) {
					t.Errorf("check %q advises %q for a cause nobody established: %v",
						result.Name, initCommand, result.NextAction)
				}
				claim := result.Summary + "\n" + result.Diagnostic
				for _, fabricated := range fabricatedClaims {
					if strings.Contains(claim, fabricated) {
						t.Errorf("check %q states %q about a subsystem it never inspected", result.Name, fabricated)
					}
				}
			}

			if blocked == 0 {
				t.Fatalf("halting at %q blocked no check; this row proves nothing", halted)
			}
		})
	}
}

// TestBrokenConfigDoesNotUnresolveTheRepository is finding R1 in its reported
// form: on an initialised repository with a syntactically broken config, doctor
// used to print `git: OK` and, one line below it, claim the repository had
// never been resolved.
func TestBrokenConfigDoesNotUnresolveTheRepository(t *testing.T) {
	report := NewRunner(DefaultChecks(haltedSubject(stepLoadConfig))...).Run(t.Context())

	byName := map[string]Result{}
	for _, result := range report.Checks {
		byName[result.Name] = result
	}

	if byName[checkGit].State != StateOK {
		t.Fatalf("git reported %q, want %q", byName[checkGit].State, StateOK)
	}
	if byName[checkConfig].Code != app.CodeConfigInvalid {
		t.Errorf("config code = %q, want %q", byName[checkConfig].Code, app.CodeConfigInvalid)
	}

	// Everything downstream of the config failure is unreported, and says which
	// check to look at instead of naming a cause of its own.
	for _, name := range []string{checkRuntimePaths, checkSQLite, checkMigrations, checkKnowledge, checkWorkspace} {
		result := byName[name]
		if result.Metadata["blocked_by_check"] != checkConfig {
			t.Errorf("check %q points at %q, want %q", name, result.Metadata["blocked_by_check"], checkConfig)
		}
		if result.Metadata["blocking_code"] != string(app.CodeConfigInvalid) {
			t.Errorf("check %q reports blocking_code %q, want %q",
				name, result.Metadata["blocking_code"], app.CodeConfigInvalid)
		}
	}
}

// TestKnowledgeSchemaWindowIsAPropertyOfTheBinary is finding R5. The two
// schema fields describe what this binary can read, so a startup that never got
// as far as the knowledge store must not make them read as zero and empty:
// that is the binary misstating its own capability.
func TestKnowledgeSchemaWindowIsAPropertyOfTheBinary(t *testing.T) {
	subjects := map[string]Subject{
		"healthy":                healthySubject(),
		"halted before the load": haltedSubject(stepOpenSQLite),
		"store never loaded":     haltedSubject(stepValidateKnowledge),
	}

	for name, subject := range subjects {
		t.Run(name, func(t *testing.T) {
			if got := KnowledgeWriteSchemaVersion(subject.Knowledge); got != schema.WriteVersion {
				t.Errorf("write schema version = %d, want %d", got, schema.WriteVersion)
			}
			if got := KnowledgeReadableSchemaVersions(subject.Knowledge); !slices.Equal(got, schema.ReadableVersions()) {
				t.Errorf("readable schema versions = %v, want %v", got, schema.ReadableVersions())
			}

			result := KnowledgeCheck(subject).Run(t.Context())
			if got := result.Details["write_schema_version"]; got != strconv.Itoa(schema.WriteVersion) {
				t.Errorf("write_schema_version detail = %q, want %q", got, strconv.Itoa(schema.WriteVersion))
			}
			if got := result.Details["readable_schema_versions"]; got == "" || got == "none" || got == "0" {
				t.Errorf("readable_schema_versions detail = %q, want the reader window", got)
			}
		})
	}
}

// TestSchemaUnsupportedRemediesAreActions is finding R8: the reader window is a
// fact about the binary and belongs in the diagnostic, not among the things the
// reader is asked to do.
func TestSchemaUnsupportedRemediesAreActions(t *testing.T) {
	subject := healthySubject()
	subject.Knowledge.Problems = []loader.Problem{{
		Path:    ".mindrail/knowledge/decisions/dec-9.json",
		Code:    app.CodeKnowledgeSchemaUnsupported,
		Message: "schema_version 3 is outside the reader window",
		Fatal:   true,
	}}

	result := KnowledgeCheck(subject).Run(t.Context())

	assertDiagnosable(t, result)
	if !strings.Contains(result.Diagnostic, "reads schema") {
		t.Errorf("diagnostic does not report the reader window: %q", result.Diagnostic)
	}
}

// TestUnreadableRecordRemediesNameTheFiles is finding R6. `status` reports
// components without their diagnostics, so a remedy that says "fix the reported
// record files" without naming one is not actionable where it is read.
func TestUnreadableRecordRemediesNameTheFiles(t *testing.T) {
	subject := healthySubject()
	paths := []string{
		".mindrail/knowledge/decisions/dec-8.json",
		".mindrail/knowledge/invariants/inv-3.json",
	}
	for _, path := range paths {
		subject.Knowledge.Problems = append(subject.Knowledge.Problems, loader.Problem{
			Path:    path,
			Code:    app.CodeKnowledgeUnreadable,
			Message: "invalid character '}' looking for beginning of object key string",
		})
	}

	result := KnowledgeCheck(subject).Run(t.Context())

	assertDiagnosable(t, result)
	if result.State != StateDegraded {
		t.Fatalf("state = %q, want %q", result.State, StateDegraded)
	}
	for _, path := range paths {
		named := false
		for _, action := range result.NextAction {
			if strings.Contains(action, path) {
				named = true
			}
		}
		if !named {
			t.Errorf("next_action %v never names %q", result.NextAction, path)
		}
	}
}

// TestReportErrPreservesProducerMetadata is finding R7. The git adapter records
// the directory it was asked about; rebuilding the error from the Result used
// to replace that with {"check": "git"} alone, leaving the user unable to learn
// which directory Mindrail actually probed — the question acceptance criterion
// 3 exists to answer, and one the shell prompt cannot answer under -C or in a
// Git hook.
func TestReportErrPreservesProducerMetadata(t *testing.T) {
	subject := healthySubject()
	subject.Repo = git.Repository{}
	subject.RepoErr = app.NewError(app.CodeNotAGitRepository, app.KindUsage,
		"the directory is not inside a Git repository worktree",
		"Mindrail scopes all of its state to a repository.",
		"Change into a Git repository and run the command again.").
		WithMetadata("start_dir", "/tmp/elsewhere").
		WithCause(git.ErrNotARepository)

	err := NewRunner(DefaultChecks(subject)...).Run(t.Context()).Err()
	if err == nil {
		t.Fatal("Err() = nil, want the repository failure")
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("Err() = %v, want a domain error", err)
	}
	if got := payload.Metadata["start_dir"]; got != "/tmp/elsewhere" {
		t.Errorf("start_dir metadata = %q, want %q: the producer's detail was discarded", got, "/tmp/elsewhere")
	}
	if got := payload.Metadata["check"]; got != checkGit {
		t.Errorf("check metadata = %q, want %q", got, checkGit)
	}
}

// fixedInstant keeps every golden and every timestamp in the suite deterministic.
var fixedInstant = time.Date(2026, 3, 14, 9, 26, 53, 0, time.UTC)

// healthySubject is the fully-initialized repository every branch test departs
// from. It is a pure value: doctor opens nothing, so no test here needs a disk.
func healthySubject() Subject {
	return Subject{
		StartDir:   "/repo/services/api",
		GitVersion: "git version 2.55.0",
		Repo: git.Repository{
			CommonDir:    "/repo/.git",
			GitDir:       "/repo/.git",
			WorktreeRoot: "/repo",
		},
		Paths: filesystem.RuntimePaths{
			CommonDir:     "/repo/.git",
			WorktreeRoot:  "/repo",
			RuntimeRoot:   "/repo/.git/mindrail",
			DBPath:        "/repo/.git/mindrail/mindrail.db",
			CacheDir:      "/repo/.git/mindrail/cache",
			RepoConfigDir: "/repo/.mindrail",
		},
		Config: config.Loaded{
			Config: config.Defaults(),
			Provenance: config.Provenance{
				config.KeyOutputColor: config.SourceDefault,
				config.KeyProjectName: config.SourceRepo,
			},
			RepoFile: "/repo/.mindrail/config.toml",
		},
		DBPresent: true,
		Pragmas:   storage.ExpectedPragmas(storage.DefaultBusyTimeout),
		Migrations: []migration.Applied{{
			Version:   1,
			Name:      "initial",
			Checksum:  "3c0d6b1f",
			AppliedAt: fixedInstant,
		}},
		Knowledge: loader.Store{
			Present: true,
			Root:    loader.StoreRoot,
			Decisions: []loader.RecordRef{
				{Kind: loader.KindDecision, ID: "DEC-1", Path: ".mindrail/knowledge/decisions/dec-1.json", SchemaVersion: 1},
				{Kind: loader.KindDecision, ID: "DEC-2", Path: ".mindrail/knowledge/decisions/dec-2.json", SchemaVersion: 1},
			},
			Invariants: []loader.RecordRef{
				{Kind: loader.KindInvariant, ID: "INV-1", Path: ".mindrail/knowledge/invariants/inv-1.json", SchemaVersion: 1},
			},
			WriteSchemaVersion:     1,
			ReadableSchemaVersions: []int{1},
		},
		Workspace: workspace.Workspace{
			ID:           "WS-01JQ8N6X5T0000000000000000",
			ProjectID:    "PRJ-01JQ8N6X5T0000000000000000",
			RootPath:     "/repo",
			GitDir:       "/repo/.git",
			RegisteredAt: fixedInstant,
			LastSeenAt:   fixedInstant,
		},
		// The probe answers are part of the fixture for the same reason every
		// other resolved fact is: a Subject that declares them is a value, and
		// DefaultChecks leaves an already-probed subject alone, so no test here
		// needs the /repo tree to exist.
		Probes: Probes{
			Taken:           true,
			RuntimeDirKnown: true,
			RuntimeDir: filesystem.Writability{
				Dir:     "/repo/.git/mindrail",
				Kind:    filesystem.RootRuntime,
				Purpose: filesystem.RootRuntime.Purpose(),
				Probed:  "/repo/.git/mindrail",
				Exists:  true,
				Usable:  true,
			},
			// Both roots, because a real healthy repository has both and the
			// two are graded differently: leaving the cache out of the fixture
			// is what let a check that treats an unusable cache as fatal look
			// healthy here (finding F13).
			CacheDirKnown: true,
			CacheDir: filesystem.Writability{
				Dir:     "/repo/.git/mindrail/cache",
				Kind:    filesystem.RootCache,
				Purpose: filesystem.RootCache.Purpose(),
				Probed:  "/repo/.git/mindrail/cache",
				Exists:  true,
				Usable:  true,
			},
		},
	}
}

// haltedSubject is the Subject bootstrap really leaves behind when the §87
// sequence aborts at st: the failing step's error is set, and every field a
// later step would have filled is still zero.
//
// It exists because the branch tables in this file mutate one field of an
// otherwise healthy subject, which is precisely why none of them could catch a
// check reporting a zero value as a finding — in those fixtures the downstream
// fields are populated, so the "we never looked" path is never reached.
func haltedSubject(st step) Subject {
	s := healthySubject()
	at := slices.Index(stepOrder, st)
	fromHere := func(other step) bool { return slices.Index(stepOrder, other) >= at }

	// Unwound in reverse order, so a halt early in the sequence clears
	// everything every later step would have written.
	if fromHere(stepRegisterWorkspace) {
		s.Workspace = workspace.Workspace{}
		s.WorkspaceErr = nil
	}
	if fromHere(stepValidateKnowledge) {
		s.Knowledge = loader.Store{}
	}
	if fromHere(stepMigrateDB) {
		s.Migrations = nil
		s.PendingCount = 0
	}
	if fromHere(stepOpenSQLite) {
		s.DBPresent = false
		s.Pragmas = storage.Pragmas{}
	}
	if fromHere(stepResolveRuntimePaths) {
		s.Paths = filesystem.RuntimePaths{}
		s.Probes = Probes{Taken: true}
	}
	if fromHere(stepLoadConfig) {
		s.Config = config.Loaded{}
	}
	if fromHere(stepResolveRepository) {
		s.Repo = git.Repository{}
		s.GitVersion = ""
	}

	switch st {
	case stepResolveRepository:
		s.RepoErr = app.NewError(app.CodeNotAGitRepository, app.KindUsage,
			"No Git repository contains /tmp/elsewhere.",
			"Mindrail cannot locate the repository.",
			"Run mindrail from inside a Git repository.").WithMetadata("start_dir", "/tmp/elsewhere")
	case stepLoadConfig:
		s.ConfigErr = app.NewError(app.CodeConfigInvalid, app.KindUsage,
			"/repo/.mindrail/config.toml is not valid TOML",
			"Mindrail refuses to start on a configuration it cannot fully understand.",
			"Correct or remove the offending entry in /repo/.mindrail/config.toml.")
	case stepResolveRuntimePaths:
		s.PathsErr = app.NewError(app.CodeRuntimePathUnwritable, app.KindUnavailable,
			`the runtime directory "/repo/.git/mindrail" is not writable`,
			"Mindrail cannot store runtime state for this repository.",
			"Make /repo/.git writable.")
	case stepOpenSQLite:
		s.DBErr = app.NewError(app.CodeRuntimeDBCorrupt, app.KindUnavailable,
			"the runtime database file is not a valid SQLite database",
			"All recorded workspace state is unreadable.",
			"Move the file aside and run `mindrail init` to rebuild it.")
	case stepMigrateDB:
		s.DBPresent = true
		s.Pragmas = storage.ExpectedPragmas(storage.DefaultBusyTimeout)
		s.MigrateErr = app.NewError(app.CodeMigrationChecksumMismatch, app.KindFailed,
			"migration 000001_initial.sql changed after it was applied",
			"The runtime schema no longer matches the migration that produced it.",
			"Add a new migration instead of editing an applied one.")
	case stepValidateKnowledge:
		s.DBPresent = true
		s.Pragmas = storage.ExpectedPragmas(storage.DefaultBusyTimeout)
		s.Migrations = healthySubject().Migrations
		s.KnowledgeErr = app.NewError(app.CodeKnowledgeUnreadable, app.KindUnavailable,
			"open .mindrail/knowledge: permission denied",
			"No decision or invariant is visible to any command.",
			"Make .mindrail/knowledge readable.")
	}

	return s
}

// stepOfCheck maps a check onto the startup step that fills its inputs. It is
// what lets a test say "this check could not possibly have run" without
// repeating the sequence.
var stepOfCheck = map[string]step{
	checkGit:          stepResolveRepository,
	checkConfig:       stepLoadConfig,
	checkRuntimePaths: stepResolveRuntimePaths,
	checkSQLite:       stepOpenSQLite,
	checkMigrations:   stepMigrateDB,
	checkKnowledge:    stepValidateKnowledge,
	checkWorkspace:    stepRegisterWorkspace,
}

// TestHealthySubjectIsHealthy keeps the shared fixture honest: if it ever drifts
// into a non-OK state the branch tests above would stop proving anything.
func TestHealthySubjectIsHealthy(t *testing.T) {
	report := NewRunner(DefaultChecks(healthySubject())...).Run(t.Context())

	for _, result := range report.Checks {
		if result.State != StateOK {
			t.Errorf("check %q reported %q on the healthy fixture: %+v", result.Name, result.State, result)
		}
	}
	if !slices.Contains([]State{StateOK}, report.WorstState) {
		t.Errorf("WorstState = %q, want %q", report.WorstState, StateOK)
	}
	if err := report.Err(); err != nil {
		t.Errorf("Err() = %v, want nil", err)
	}
	if errors.Is(report.Err(), git.ErrNotARepository) {
		t.Error("healthy fixture leaked a repository error")
	}
}
