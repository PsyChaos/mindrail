package doctor

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/workspace"
)

// TestVerdictIsFatalExactlyWhenTheInstallationIsUnusable is the check-layer half
// of finding F11.
//
// Report.Err() answers only for ERROR readings, which is decision D-14 and is
// right as far as it goes. What it cannot see is a startup sequence that never
// finished: a missing git binary is UNAVAILABLE by decision D-30, so Err() was
// nil, the envelope said {"ok": true} with no data, and the process still exited
// 4 from the start error a different code path had kept.
//
// The table is the whole point. Promoting every UNAVAILABLE reading would have
// been the obvious fix and would have broken decision D-03's one zero-exit row —
// "you have not run init yet" is a state, not a failure — so the rows that must
// stay non-fatal are here beside the rows that must become fatal.
func TestVerdictIsFatalExactlyWhenTheInstallationIsUnusable(t *testing.T) {
	tests := []struct {
		name     string
		subject  func() Subject
		wantCode app.Code
		wantExit int
	}{
		{
			name:    "healthy repository",
			subject: healthySubject,
		},
		{
			name: "uninitialized repository",
			subject: func() Subject {
				s := healthySubject()
				s.DBPresent = false
				s.Migrations = nil
				s.Workspace = workspace.Workspace{}
				s.Pragmas = healthySubject().Pragmas
				return s
			},
			// Decision D-03's zero-exit row. The sqlite, migrations and
			// workspace readings are all UNAVAILABLE here and none of them may
			// produce an error object or a non-zero exit.
		},
		{
			name: "unusable cache directory",
			subject: func() Subject {
				s := healthySubject()
				s.Probes.CacheDir = unusableCache()
				return s
			},
			// Finding F13: MR-001 writes nothing to the cache, so nothing about
			// this stops a command.
		},
		{
			name: "degraded knowledge store",
			subject: func() Subject {
				s := healthySubject()
				s.Config.Warnings = []app.Warning{{Code: app.CodeConfigUnknownEnvVar, Message: "MINDRAIL_NONSENSE is not a setting"}}
				return s
			},
		},
		{
			name: "git is not installed",
			subject: func() Subject {
				s := haltedSubject(stepResolveRepository)
				s.RepoErr = gitFailure(app.CodeGitUnavailable, git.ErrGitUnavailable)
				return s
			},
			wantCode: app.CodeGitUnavailable,
			wantExit: app.ExitUnavailable,
		},
		{
			name: "git did not answer in time",
			subject: func() Subject {
				s := haltedSubject(stepResolveRepository)
				s.RepoErr = gitFailure(app.CodeGitTimeout, git.ErrGitTimeout)
				return s
			},
			wantCode: app.CodeGitTimeout,
			wantExit: app.ExitUnavailable,
		},
		{
			name: "repository discovery answered nothing at all",
			subject: func() Subject {
				s := haltedSubject(stepResolveRepository)
				s.RepoErr = nil
				return s
			},
			wantCode: app.CodeStartupIncomplete,
			wantExit: app.ExitUnavailable,
		},
		{
			name:     "not a git repository",
			subject:  func() Subject { return haltedSubject(stepResolveRepository) },
			wantCode: app.CodeNotAGitRepository,
			wantExit: app.ExitUsage,
		},
		{
			name:     "configuration rejected",
			subject:  func() Subject { return haltedSubject(stepLoadConfig) },
			wantCode: app.CodeConfigInvalid,
			wantExit: app.ExitUsage,
		},
		{
			name:     "runtime paths unusable",
			subject:  func() Subject { return haltedSubject(stepResolveRuntimePaths) },
			wantCode: app.CodeRuntimePathUnwritable,
			wantExit: app.ExitUnavailable,
		},
		{
			name:     "runtime database corrupt",
			subject:  func() Subject { return haltedSubject(stepOpenSQLite) },
			wantCode: app.CodeRuntimeDBCorrupt,
			wantExit: app.ExitUnavailable,
		},
		{
			name:     "migration ledger drifted",
			subject:  func() Subject { return haltedSubject(stepMigrateDB) },
			wantCode: app.CodeMigrationChecksumMismatch,
			wantExit: app.ExitFailed,
		},
		{
			name:     "knowledge store unreadable",
			subject:  func() Subject { return haltedSubject(stepValidateKnowledge) },
			wantCode: app.CodeKnowledgeUnreadable,
			// Not an environment condition: the store is there and this binary
			// ran and failed to read it, which is decision D-03's "operation
			// failed" class.
			wantExit: app.ExitFailed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subject := Probe(tc.subject())
			report := NewRunner(DefaultChecks(subject)...).Run(t.Context())

			verdict := Verdict(subject, report)

			if tc.wantCode == "" {
				if verdict != nil {
					t.Fatalf("Verdict = %v, want nil: this installation is usable", verdict)
				}
				if got := app.ExitCode(verdict); got != app.ExitSuccess {
					t.Errorf("exit code = %d, want %d", got, app.ExitSuccess)
				}
				return
			}

			if verdict == nil {
				t.Fatalf("Verdict = nil, want %s: nothing on this installation can run", tc.wantCode)
			}
			payload, ok := app.PayloadOf(verdict)
			if !ok {
				t.Fatalf("Verdict = %v, want a domain error", verdict)
			}
			if payload.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", payload.Code, tc.wantCode)
			}
			if got := app.ExitCode(verdict); got != tc.wantExit {
				t.Errorf("exit code = %d, want %d", got, tc.wantExit)
			}
			for label, value := range map[string]string{"why": payload.Why, "impact": payload.Impact} {
				if value == "" {
					t.Errorf("verdict carries no %s", label)
				}
			}
			if len(payload.NextAction) == 0 {
				t.Error("verdict carries no next_action")
			}
		})
	}
}

// TestVerdictNeverContradictsReportErr keeps the two answers layered rather than
// competing: everything Err() calls fatal, Verdict calls fatal too, with the
// same code.
func TestVerdictNeverContradictsReportErr(t *testing.T) {
	for _, st := range stepOrder[:len(stepOrder)-1] {
		subject := Probe(haltedSubject(st))
		report := NewRunner(DefaultChecks(subject)...).Run(t.Context())

		reported := report.Err()
		if reported == nil {
			continue
		}

		verdict := Verdict(subject, report)
		if verdict == nil {
			t.Errorf("halt at %s: Err() = %v but Verdict = nil", st, reported)
			continue
		}

		wantPayload, _ := app.PayloadOf(reported)
		gotPayload, _ := app.PayloadOf(verdict)
		if gotPayload.Code != wantPayload.Code {
			t.Errorf("halt at %s: Verdict code = %q, Err() code = %q", st, gotPayload.Code, wantPayload.Code)
		}
	}
}

// TestUnusableCacheDirectoryDoesNotBrickAHealthyRepository is the over-fire
// guard for finding F13.
//
// The regression it replaces graded every runtime root alike, so a cache
// directory Mindrail never writes to drove runtime_paths to ERROR and every
// command to exit 4 on a repository where the database opened, the schema was
// current and the worktree was registered. Severity has to follow what the
// condition actually prevents, and this one prevents nothing.
func TestUnusableCacheDirectoryDoesNotBrickAHealthyRepository(t *testing.T) {
	subject := healthySubject()
	subject.Probes.CacheDir = unusableCache()

	report := NewRunner(DefaultChecks(subject)...).Run(t.Context())

	result, found := findResult(report, checkRuntimePaths)
	if !found {
		t.Fatalf("no %s reading in the report", checkRuntimePaths)
	}
	if result.State != StateDegraded {
		t.Errorf("runtime_paths state = %q, want %q", result.State, StateDegraded)
	}
	if err := report.Err(); err != nil {
		t.Errorf("Err() = %v, want nil: an unusable cache stops no command", err)
	}
	if verdict := Verdict(subject, report); verdict != nil {
		t.Errorf("Verdict = %v, want nil", verdict)
	}

	// Every other reading has to stay exactly as healthy as it was.
	for _, other := range report.Checks {
		if other.Name == checkRuntimePaths {
			continue
		}
		if other.State != StateOK {
			t.Errorf("check %q reported %q because the cache directory is unusable", other.Name, other.State)
		}
	}

	// And the reading has to be about the cache, with a remedy for the cache.
	if !strings.Contains(result.Diagnostic, "cache") {
		t.Errorf("diagnostic does not say the cache is the problem: %q", result.Diagnostic)
	}
	if !slices.ContainsFunc(result.NextAction, func(a string) bool { return strings.Contains(a, "MINDRAIL_CACHE_DIR") }) {
		t.Errorf("next_action does not offer the cache override: %v", result.NextAction)
	}
	for _, action := range result.NextAction {
		if strings.Contains(action, "MINDRAIL_RUNTIME_DIR") {
			t.Errorf("next_action tells the user to move the runtime root over a cache failure: %v", result.NextAction)
		}
	}
}

// TestUnusableRuntimeRootIsStillFatal is the other half of the pair, and the
// reason the fix above is safe to make. Grading the cache down is only correct
// while the runtime root — which holds the database every command reads — is
// still caught.
func TestUnusableRuntimeRootIsStillFatal(t *testing.T) {
	subject := healthySubject()
	subject.Probes.RuntimeDir = filesystem.Writability{
		Dir:     "/repo/.git/mindrail",
		Kind:    filesystem.RootRuntime,
		Purpose: filesystem.RootRuntime.Purpose(),
		Probed:  "/repo/.git",
		Exists:  false,
		Usable:  false,
		Err: app.NewError(app.CodeRuntimePathUnwritable, app.KindUnavailable,
			`the runtime root "/repo/.git/mindrail" could not be created`,
			"Mindrail cannot store its runtime state, so no command that needs the database can run",
			`check the permissions on "/repo/.git"`,
			"or point MINDRAIL_RUNTIME_DIR at a usable location").
			WithMetadata("root_kind", string(filesystem.RootRuntime)),
	}

	report := NewRunner(DefaultChecks(subject)...).Run(t.Context())

	result, _ := findResult(report, checkRuntimePaths)
	if result.State != StateError {
		t.Fatalf("runtime_paths state = %q, want %q", result.State, StateError)
	}

	verdict := Verdict(subject, report)
	if verdict == nil {
		t.Fatal("Verdict = nil for an unwritable runtime root")
	}
	payload, _ := app.PayloadOf(verdict)
	if payload.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
	}
	if got := app.ExitCode(verdict); got != app.ExitUnavailable {
		t.Errorf("exit code = %d, want %d", got, app.ExitUnavailable)
	}
}

// TestWorkspaceCheckDoesNotAssertAQueryItCouldNotHaveRun is finding F16.
//
// A second process can open the database file another process created and has
// not finished migrating. There is no workspaces table then, and the report used
// to carry SQLite's own `no such table: workspaces (1)` as the user-facing why
// line of a WORKSPACE_REGISTRATION_FAILED error at exit 1. The condition is "not
// initialised yet", and the migration ledger already says so.
func TestWorkspaceCheckDoesNotAssertAQueryItCouldNotHaveRun(t *testing.T) {
	subject := healthySubject()
	subject.Migrations = nil
	subject.PendingCount = 1
	subject.Workspace = workspace.Workspace{}

	report := NewRunner(DefaultChecks(subject)...).Run(t.Context())

	result, found := findResult(report, checkWorkspace)
	if !found {
		t.Fatalf("no %s reading in the report", checkWorkspace)
	}
	if result.State != StateUnavailable {
		t.Errorf("workspace state = %q, want %q", result.State, StateUnavailable)
	}
	if result.Code != app.CodeWorkspaceNotInitialized {
		t.Errorf("workspace code = %q, want %q", result.Code, app.CodeWorkspaceNotInitialized)
	}
	if strings.Contains(result.Diagnostic, "was queried") {
		t.Errorf("diagnostic claims a query that could not have run: %q", result.Diagnostic)
	}
	if !slices.Contains(result.NextAction, initCommand) {
		t.Errorf("next_action = %v, want %q", result.NextAction, initCommand)
	}
	if err := report.Err(); err != nil {
		t.Errorf("Err() = %v, want nil: a first run in flight is a state, not a failure", err)
	}
	if verdict := Verdict(subject, report); verdict != nil {
		t.Errorf("Verdict = %v, want nil", verdict)
	}
}

// TestFullyMigratedDatabaseStillReportsAnUnregisteredWorktree is the over-fire
// guard for the fix above: the pending-migration branch must not swallow the
// case it was carved out of. A database whose schema is complete and whose
// workspaces table holds no row for this worktree really was queried, and the
// report has to keep saying so.
func TestFullyMigratedDatabaseStillReportsAnUnregisteredWorktree(t *testing.T) {
	subject := healthySubject()
	subject.PendingCount = 0
	subject.Workspace = workspace.Workspace{}

	report := NewRunner(DefaultChecks(subject)...).Run(t.Context())

	result, _ := findResult(report, checkWorkspace)
	if result.State != StateUnavailable {
		t.Errorf("workspace state = %q, want %q", result.State, StateUnavailable)
	}
	if !strings.Contains(result.Diagnostic, "was queried") {
		t.Errorf("diagnostic no longer says the database was queried: %q", result.Diagnostic)
	}
}

// producerRemedy is the distinctive next action a lower layer attaches to its
// own error. Nothing in the check bodies produces this sentence, so a check that
// reports it can only have carried the producer's.
const producerRemedy = "Ask the layer that failed, not the check that reported it."

// TestProducerRemedySurvivesToTheOutput is finding F18.
//
// explain() prefers the diagnosis the failing layer attached over the check's
// generic fallback, and deleting that preference — falling back to the
// hardcoded remedy every time — left the entire suite green. Every branch tests
// the *presence* of a next action; none tested whose it was. This does, for
// every check that reports a producer error, and it follows the remedy all the
// way to the error a command returns rather than stopping at the Result.
func TestProducerRemedySurvivesToTheOutput(t *testing.T) {
	tests := []struct {
		check   string
		subject func(err error) Subject
	}{
		{checkGit, func(err error) Subject {
			s := haltedSubject(stepResolveRepository)
			s.RepoErr = err
			return s
		}},
		{checkConfig, func(err error) Subject {
			s := haltedSubject(stepLoadConfig)
			s.ConfigErr = err
			return s
		}},
		{checkRuntimePaths, func(err error) Subject {
			s := haltedSubject(stepResolveRuntimePaths)
			s.PathsErr = err
			return s
		}},
		{checkSQLite, func(err error) Subject {
			s := haltedSubject(stepOpenSQLite)
			s.DBErr = err
			return s
		}},
		{checkMigrations, func(err error) Subject {
			s := haltedSubject(stepMigrateDB)
			s.MigrateErr = err
			return s
		}},
		{checkKnowledge, func(err error) Subject {
			s := haltedSubject(stepValidateKnowledge)
			s.KnowledgeErr = err
			return s
		}},
		{checkWorkspace, func(err error) Subject {
			s := healthySubject()
			s.Workspace = workspace.Workspace{}
			s.WorkspaceErr = err
			return s
		}},
	}

	for _, tc := range tests {
		t.Run(tc.check, func(t *testing.T) {
			producer := app.NewError(
				app.CodeRuntimeDBUnavailable,
				app.KindUnavailable,
				"the producing layer's own sentence",
				"the producing layer's own impact",
				producerRemedy,
			).WithMetadata("producer_detail", "only the failing layer knows this")

			subject := Probe(tc.subject(producer))
			report := NewRunner(DefaultChecks(subject)...).Run(t.Context())

			result, found := findResult(report, tc.check)
			if !found {
				t.Fatalf("no %s reading in the report", tc.check)
			}

			if !slices.Equal(result.NextAction, []string{producerRemedy}) {
				t.Errorf("next_action = %v, want the producer's %q", result.NextAction, producerRemedy)
			}
			if result.Diagnostic != "the producing layer's own sentence" {
				t.Errorf("diagnostic = %q, want the producer's", result.Diagnostic)
			}
			if result.Impact != "the producing layer's own impact" {
				t.Errorf("impact = %q, want the producer's", result.Impact)
			}
			if result.Code != app.CodeRuntimeDBUnavailable {
				t.Errorf("code = %q, want the producer's %q", result.Code, app.CodeRuntimeDBUnavailable)
			}
			if got := result.Metadata["producer_detail"]; got != "only the failing layer knows this" {
				t.Errorf("metadata producer_detail = %q, want the producer's", got)
			}

			// And out the far end: what a command returns, and therefore what
			// the envelope and the human block print, is this remedy and not a
			// reconstruction of it.
			verdict := Verdict(subject, report)
			if verdict == nil {
				t.Fatal("Verdict = nil for a check reporting a producer failure")
			}
			payload, ok := app.PayloadOf(verdict)
			if !ok {
				t.Fatalf("Verdict = %v, want a domain error", verdict)
			}
			if !slices.Equal(payload.NextAction, []string{producerRemedy}) {
				t.Errorf("verdict next_action = %v, want the producer's %q", payload.NextAction, producerRemedy)
			}
			if payload.Metadata["producer_detail"] != "only the failing layer knows this" {
				t.Errorf("verdict dropped the producer's metadata: %v", payload.Metadata)
			}
		})
	}
}

// gitFailure builds the error the git adapter produces for an environment
// condition: a coded payload with the package sentinel underneath, which is what
// gitResult matches on.
func gitFailure(code app.Code, sentinel error) error {
	return app.NewError(code, app.KindUnavailable,
		"git could not answer",
		"Mindrail drives the system git binary and cannot operate without it",
		"install git and make sure it is on PATH").
		WithMetadata("binary", "git").
		WithCause(errors.Join(sentinel, errors.New("exec: \"git\": executable file not found in $PATH")))
}

// unusableCache is a cache directory Mindrail cannot write, described the way
// the filesystem probe describes it.
func unusableCache() filesystem.Writability {
	return filesystem.Writability{
		Dir:     "/repo/.git/mindrail/cache",
		Kind:    filesystem.RootCache,
		Purpose: filesystem.RootCache.Purpose(),
		Probed:  "/repo/.git/mindrail/cache",
		Exists:  true,
		Usable:  false,
		Err: app.NewError(app.CodeRuntimePathUnwritable, app.KindUnavailable,
			`the cache directory "/repo/.git/mindrail/cache" is not writable`,
			"Mindrail cannot store derived cache data for this repository",
			`check the permissions on "/repo/.git/mindrail/cache"`,
			"or point MINDRAIL_CACHE_DIR at a usable location").
			WithMetadata("root_kind", string(filesystem.RootCache)),
	}
}

func findResult(report Report, name string) (Result, bool) {
	for _, result := range report.Checks {
		if result.Name == name {
			return result, true
		}
	}
	return Result{}, false
}
