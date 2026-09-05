package doctor

import (
	"slices"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// step names one stage of the tech-stack §87 startup sequence, in the spelling
// bootstrap logs and records.
//
// Doctor needs the vocabulary because §87 is a *sequence*: when it aborts at
// step N, steps N+1 onwards never ran and the Subject fields they would have
// filled are still zero. A check that read those zeros as findings would report
// "there is no database" when the truth is "nobody looked" — the one answer a
// health report must never give, and the shape of findings R1, R4 and R5.
//
// The constants are spelled here rather than imported from bootstrap because
// bootstrap imports doctor; the wire spelling is the contract both halves share.
type step string

const (
	stepResolveRepository   step = "resolve_repository"
	stepLoadConfig          step = "load_config"
	stepResolveRuntimePaths step = "resolve_runtime_paths"
	stepOpenSQLite          step = "open_sqlite"
	stepMigrateDB           step = "migrate_db"
	stepValidateKnowledge   step = "validate_knowledge"
	stepRegisterWorkspace   step = "register_workspace"
)

// stepOrder is the prefix of §87 that fills a Subject, in order. Only these
// seven populate anything a check reads; the three that follow them
// (load_index_state, init_managers, execute_command) contribute no field.
var stepOrder = []step{
	stepResolveRepository,
	stepLoadConfig,
	stepResolveRuntimePaths,
	stepOpenSQLite,
	stepMigrateDB,
	stepValidateKnowledge,
	stepRegisterWorkspace,
}

// halt describes where the startup sequence stopped: the step that did not
// complete, the check that reports that step's failure, and the code of the
// failure itself. The check name is carried so a blocked reading can point at
// the one line in the same report that explains it, instead of guessing a cause.
type halt struct {
	step  step
	check string
	code  app.Code
}

// haltedAt reports the first §87 step that did not complete, mirroring
// bootstrap's own abort-on-first-error control flow.
//
// It is derived from the Subject rather than recorded by bootstrap so that the
// two can never disagree by construction: every condition below is the same
// condition that makes bootstrap return early from the corresponding step.
func (s Subject) haltedAt() (halt, bool) {
	switch {
	case s.RepoErr != nil:
		return halt{stepResolveRepository, checkGit, codeOf(s.RepoErr, app.CodeNotAGitRepository)}, true
	case s.Repo.CommonDir == "" || s.Repo.WorktreeRoot == "":
		// Discovery returned neither a layout nor an error. Nothing downstream
		// could have run, whatever the reason.
		return halt{stepResolveRepository, checkGit, app.CodeStartupIncomplete}, true
	case s.ConfigErr != nil:
		return halt{stepLoadConfig, checkConfig, codeOf(s.ConfigErr, app.CodeConfigInvalid)}, true
	case s.PathsErr != nil:
		return halt{stepResolveRuntimePaths, checkRuntimePaths, codeOf(s.PathsErr, app.CodeRuntimePathUnwritable)}, true
	case s.Paths.RuntimeRoot == "" || s.Paths.DBPath == "":
		return halt{stepResolveRuntimePaths, checkRuntimePaths, app.CodeStartupIncomplete}, true
	case s.DBErr != nil:
		return halt{stepOpenSQLite, checkSQLite, codeOf(s.DBErr, app.CodeRuntimeDBUnavailable)}, true
	case s.IntegrityErr != nil:
		return halt{stepOpenSQLite, checkSQLite, codeOf(s.IntegrityErr, app.CodeRuntimeDBCorrupt)}, true
	case s.MigrateErr != nil:
		return halt{stepMigrateDB, checkMigrations, codeOf(s.MigrateErr, app.CodeMigrationFailed)}, true
	case s.KnowledgeErr != nil:
		return halt{stepValidateKnowledge, checkKnowledge, codeOf(s.KnowledgeErr, app.CodeKnowledgeUnreadable)}, true
	}
	return halt{}, false
}

// reached reports whether startup got far enough for st to have run.
//
// The halting step itself counts as reached: it ran and failed, so its own
// check has a real error to describe. Everything after it did not.
func (s Subject) reached(st step) bool {
	h, halted := s.haltedAt()
	if !halted {
		return true
	}
	return slices.Index(stepOrder, st) <= slices.Index(stepOrder, h.step)
}

// codeOf recovers the code the failing layer attached, falling back to the code
// the step is known for. It never returns the empty string, because a non-OK
// reading with no code is one a consumer cannot branch on (finding R1).
func codeOf(err error, fallback app.Code) app.Code {
	if payload, ok := app.PayloadOf(err); ok && payload.Code != "" {
		return payload.Code
	}
	if fallback == "" {
		return app.CodeStartupIncomplete
	}
	return fallback
}

// unreached renders a reading whose inputs the startup sequence never populated.
//
// It stays inside the §84 vocabulary (decision D-16): UNAVAILABLE already means
// "this could not be reached", which is exactly true here. What makes it honest
// rather than merely polite is the content — the summary asserts nothing about
// the subsystem, the diagnostic names the step that stopped the run and the
// check that explains it, and the remedy points at that check instead of
// presuming a cause nobody established.
func unreached(h halt, subjectPhrase string) Result {
	return Result{
		State:   StateUnavailable,
		Summary: "Not inspected: startup stopped earlier",
		Diagnostic: "Startup stopped at the " + string(h.step) + " step. Nothing was read about " + subjectPhrase +
			", so this reading is a missing observation rather than a finding of absence; the " +
			h.check + " check reports what stopped the run.",
		Impact:     "This report can say nothing about the health of " + subjectPhrase + ".",
		NextAction: []string{"Resolve the failure reported by the " + h.check + " check, then read this report again."},
		Code:       app.CodeStartupIncomplete,
		Metadata: map[string]string{
			"stopped_at_step":  string(h.step),
			"blocked_by_check": h.check,
			"blocking_code":    string(h.code),
		},
	}
}

// Probes are the read-only observations doctor makes for itself.
//
// Bootstrap answers "did opening the database work?", which is a different
// question from "could it ever work?": a runtime directory nothing can write
// and a database path occupied by a directory both look exactly like a
// repository that was never initialised, and both are made worse by the
// `mindrail init` that answer recommends (findings R2 and R3). Establishing the
// difference needs a stat and an access(2), never a write, so decision D-01
// still holds: doctor probes, it does not create.
type Probes struct {
	// Taken marks a Subject whose probes have already been answered. A test
	// fixture sets it so that checks stay pure functions of a value and need no
	// disk; Probe sets it so a second call is free.
	Taken bool

	// RuntimeDir answers whether the runtime root — the directory the database
	// and the workspace state live in — is usable.
	RuntimeDir      filesystem.Writability
	RuntimeDirKnown bool

	// CacheDir answers the same question about the cache directory. It is a
	// separate field rather than a second reading under the first name because
	// the two are worth different amounts: publishing the cache directory's
	// verdict as `runtime_root_usable` is how a healthy repository came to be
	// reported as unusable (finding F13).
	CacheDir      filesystem.Writability
	CacheDirKnown bool

	// DBPath is what occupies the runtime database path, and DBPathErr the
	// structured obstruction when that is something a database can never be.
	// An empty DBPath means the question was not asked.
	DBPath    storage.Presence
	DBPathErr error
}

// Probe answers the read-only questions doctor is allowed to ask the disk and
// returns the enriched Subject. It is idempotent and never writes.
//
// It is a separate step rather than something the individual checks do, so that
// every check stays a pure function of the Subject it is handed: a check that
// reached for the disk on its own could not be tested without one, and could
// quietly disagree with the check beside it about what is there.
func Probe(s Subject) Subject {
	if s.Probes.Taken {
		return s
	}
	s.Probes.Taken = true

	// Nothing to probe until the locations exist as answers. Leaving the fields
	// zero here is the point: a zero Writability must never read as "unwritable".
	if !s.reached(stepResolveRuntimePaths) || s.PathsErr != nil || s.Paths.RuntimeRoot == "" {
		return s
	}

	// Every root is probed, and each answer is filed under the root it is about.
	// filesystem.RootKind is what makes that possible: the probe states which
	// directory it inspected and what that directory is for, and grading the
	// answers is this layer's job, not the filesystem's.
	for _, writability := range s.Paths.ProbeRoots() {
		switch writability.Kind {
		case filesystem.RootRuntime:
			s.Probes.RuntimeDir = writability
			s.Probes.RuntimeDirKnown = true
		case filesystem.RootCache:
			s.Probes.CacheDir = writability
			s.Probes.CacheDirKnown = true
		}
	}

	// The presence probe only has something to add where bootstrap's own answer
	// was "no database": RuntimePaths.Exists() is a stat that cannot tell an
	// empty path from an occupied one, and the occupied case is precisely the
	// one `mindrail init` cannot fix.
	if s.reached(stepOpenSQLite) && !s.DBPresent && s.DBErr == nil && s.Paths.DBPath != "" {
		s.Probes.DBPath, s.Probes.DBPathErr = storage.Probe(s.Paths.DBPath)
	}

	return s
}

// obstruction is a runtime-store blockage that `mindrail init` cannot clear.
// The runtime store is reported by three checks, and all three have to name the
// obstruction rather than recommend the command that would fail on it.
func (s Subject) obstruction() (diagnosis, bool) {
	if s.Probes.DBPathErr != nil {
		return explain(s.Probes.DBPathErr, diagnosis{
			code:   app.CodeRuntimeDBUnavailable,
			impact: "Mindrail can neither open nor create the runtime database while its path is occupied.",
			next:   []string{"Move aside whatever occupies " + s.Paths.DBPath + "."},
		}), true
	}
	if s.Probes.RuntimeDirKnown && !s.Probes.RuntimeDir.Usable {
		return explain(s.Probes.RuntimeDir.Err, diagnosis{
			code:   app.CodeRuntimePathUnwritable,
			impact: "Mindrail cannot store its runtime state, so no command that needs the database can run.",
			next:   []string{"Make " + s.Probes.RuntimeDir.Probed + " writable."},
		}), true
	}
	return diagnosis{}, false
}

// cacheObstruction returns the cache directory failure, from wherever it was
// established, or nil.
//
// init learns it by trying to create the directory and status and doctor learn
// it from the probe, and both have to render the same sentence — one command
// reporting a condition the next one calls healthy is the disagreement this
// package exists to prevent.
func (s Subject) cacheObstruction() error {
	if s.CacheErr != nil {
		return s.CacheErr
	}
	if s.Probes.CacheDirKnown && !s.Probes.CacheDir.Usable {
		return s.Probes.CacheDir.Err
	}
	return nil
}
