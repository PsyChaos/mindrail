package doctor

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/workspace"
)

// Check names. They are the machine-readable identity of a reading: humans read
// the summary, tooling matches on these.
const (
	checkGit          = "git"
	checkRuntimePaths = "runtime_paths"
	checkConfig       = "config"
	checkSQLite       = "sqlite"
	checkMigrations   = "migrations"
	checkKnowledge    = "knowledge"
	checkWorkspace    = "workspace"
)

// Report sections, in the order DefaultChecks emits them. Each section is a
// contiguous run of checks so the renderer prints one header per section
// without reordering the report.
const (
	sectionRepository    = "Repository"
	sectionRuntimePaths  = "Runtime paths"
	sectionConfiguration = "Configuration"
	sectionRuntimeStore  = "Runtime store"
	sectionKnowledge     = "Knowledge"
	sectionWorkspace     = "Workspace"
)

// DefaultChecks returns the MR-001 check set, in report order.
//
// Seven checks, and only seven (decision D-10): one per subsystem this binary
// actually has. Registering a ResolverCheck, ParserCheck, IndexCheck,
// CoverageCheck, HookCheck or CIConfigCheck would put a NOT_APPLICABLE line in
// every report, and §84 reads that as "this project does not use it" — a claim
// about the repository, not about the binary. The list grows additively as the
// subsystems land.
//
// The subject is probed once here rather than in each check, so every check in
// one report describes the same disk (see Probe). A subject that already
// carries its probe answers — every test fixture does — is passed through.
func DefaultChecks(s Subject) []Check {
	s = Probe(s)

	return []Check{
		GitCheck(s),
		RuntimePathCheck(s),
		ConfigCheck(s),
		SQLiteCheck(s),
		MigrationCheck(s),
		KnowledgeCheck(s),
		WorkspaceCheck(s),
	}
}

// GitCheck reports repository discovery: the common dir, the active worktree
// and which of the two this directory belongs to (acceptance criterion 3).
func GitCheck(s Subject) Check {
	return CheckFunc{
		CheckName:    checkGit,
		CheckSection: sectionRepository,
		Fn:           func(context.Context) Result { return gitResult(s) },
	}
}

func gitResult(s Subject) Result {
	if s.RepoErr != nil {
		// A missing or hanging git is an environment condition, not a broken
		// repository: decision D-30 keeps it UNAVAILABLE so a slow network
		// filesystem cannot fail a pipeline that is otherwise correct.
		if errors.Is(s.RepoErr, git.ErrGitUnavailable) {
			return failure(StateUnavailable, "Git is not available", explain(s.RepoErr, diagnosis{
				code:   app.CodeGitUnavailable,
				impact: "Mindrail cannot discover the repository, so no command can run.",
				next: []string{
					"Install git and make sure it is on PATH.",
					"Set the git executable explicitly if it lives outside PATH.",
				},
			}))
		}
		if errors.Is(s.RepoErr, git.ErrGitTimeout) {
			return failure(StateUnavailable, "Git did not answer in time", explain(s.RepoErr, diagnosis{
				code:   app.CodeGitTimeout,
				impact: "Repository discovery is unavailable until git responds.",
				next: []string{
					"Retry once the filesystem hosting this repository responds.",
					"Remove a stale .git/index.lock if one is present.",
				},
			}))
		}

		return failure(StateError, "Repository not resolved", explain(s.RepoErr, diagnosis{
			code:   app.CodeNotAGitRepository,
			impact: "Mindrail cannot locate the repository, so no command can run.",
			next: []string{
				"Run mindrail from inside a Git repository.",
				"Run `git init` if this directory should be one.",
			},
		}))
	}

	if s.Repo.IsBare {
		return failure(StateError, "Repository is bare", diagnosis{
			code:       app.CodeBareRepository,
			diagnostic: "The repository at " + s.Repo.CommonDir + " is bare and has no working tree.",
			impact:     "Mindrail reports an active worktree; a bare repository has none.",
			next: []string{
				"Run mindrail from a checkout of this repository.",
				"Add a worktree with `git worktree add <path>`.",
			},
		})
	}

	if s.Repo.CommonDir == "" || s.Repo.WorktreeRoot == "" {
		return failure(StateUnavailable, "Repository discovery did not complete", diagnosis{
			code:       app.CodeStartupIncomplete,
			diagnostic: "Repository discovery reported neither a layout nor an error for " + describeStartDir(s.StartDir) + ", so this reading has nothing to describe.",
			impact:     "Every later reading in this report is about a repository that was never resolved.",
			next: []string{
				"Run the command again from inside a Git repository worktree.",
			},
			metadata: map[string]string{"start_dir": s.StartDir},
		})
	}

	details := map[string]string{
		"common_dir":         s.Repo.CommonDir,
		"git_dir":            s.Repo.GitDir,
		"worktree_root":      s.Repo.WorktreeRoot,
		"is_linked_worktree": strconv.FormatBool(s.Repo.IsLinkedWorktree),
	}
	if s.GitVersion != "" {
		details["git_version"] = s.GitVersion
	}

	return Result{State: StateOK, Summary: "Git common-dir detected", Details: details}
}

// RuntimePathCheck reports where this repository's machine-local state lives
// and whether Mindrail could actually use it.
//
// Both halves are needed. Derivation is a pure computation that succeeds for a
// repository nothing can write, so a check that reported only the derivation
// called an unwritable installation healthy and left the failure to be
// discovered as a missing database — remedied with the `mindrail init` that
// cannot succeed either (finding R2). Usability is established with a stat and
// an access(2) by Probe; creating the directories to find out remains init's
// job (decision D-01).
func RuntimePathCheck(s Subject) Check {
	return CheckFunc{
		CheckName:    checkRuntimePaths,
		CheckSection: sectionRuntimePaths,
		Fn:           func(context.Context) Result { return runtimePathResult(s) },
	}
}

func runtimePathResult(s Subject) Result {
	if !s.reached(stepResolveRuntimePaths) {
		h, _ := s.haltedAt()
		return unreached(h, "this repository's runtime locations")
	}

	if s.PathsErr != nil {
		return failure(StateError, "Runtime paths unusable", explain(s.PathsErr, diagnosis{
			code:   app.CodeRuntimePathUnwritable,
			impact: "Mindrail cannot store runtime state for this repository.",
			next: []string{
				"Make the Git common directory writable.",
				"Set MINDRAIL_RUNTIME_DIR to a writable directory.",
			},
		}))
	}

	if s.Paths.RuntimeRoot == "" {
		return failure(StateUnavailable, "Runtime paths not derived", diagnosis{
			code: app.CodeStartupIncomplete,
			diagnostic: "Path derivation returned neither a location nor an error for the Git common-dir at " +
				s.Repo.CommonDir + ", so this reading has nothing to describe.",
			impact: "Mindrail cannot say where this repository's runtime state would live.",
			next: []string{
				"Report this as a defect: repository discovery succeeded but runtime path derivation produced nothing.",
			},
			metadata: map[string]string{"common_dir": s.Repo.CommonDir},
		})
	}

	details := map[string]string{
		"runtime_root":    s.Paths.RuntimeRoot,
		"db_path":         s.Paths.DBPath,
		"cache_dir":       s.Paths.CacheDir,
		"repo_config_dir": s.Paths.RepoConfigDir,
	}

	if s.Probes.RuntimeDirKnown {
		details["runtime_root_exists"] = strconv.FormatBool(s.Probes.RuntimeDir.Exists)
		details["runtime_root_usable"] = strconv.FormatBool(s.Probes.RuntimeDir.Usable)

		if !s.Probes.RuntimeDir.Usable {
			result := failure(StateError, "Runtime paths unusable", explain(s.Probes.RuntimeDir.Err, diagnosis{
				code:   app.CodeRuntimePathUnwritable,
				impact: "Mindrail cannot store runtime state for this repository, so `mindrail init` would fail here too.",
				next: []string{
					"Make " + s.Probes.RuntimeDir.Probed + " writable.",
					"Or set MINDRAIL_RUNTIME_DIR to a writable directory.",
				},
			}))
			result.Details = details
			return result
		}
	}
	if s.Probes.CacheDirKnown {
		details["cache_dir_exists"] = strconv.FormatBool(s.Probes.CacheDir.Exists)
		details["cache_dir_usable"] = strconv.FormatBool(s.Probes.CacheDir.Usable)
	}

	// A cache directory Mindrail cannot use is reported and is not fatal.
	// Severity follows what the condition actually prevents, and MR-001 stores
	// nothing in the cache: every command still runs, `mindrail init` still
	// succeeds, and the repository is still ready for targeted work. Grading it
	// like the runtime root — which holds the database every command reads —
	// drove a fully working, registered repository to ERROR, BLOCKED and exit 4
	// (finding F13). DEGRADED is §84's word for exactly this: running with less
	// than it wants.
	if cacheErr := s.cacheObstruction(); cacheErr != nil {
		result := failure(StateDegraded, "Cache directory unusable", explain(cacheErr, diagnosis{
			code:   app.CodeRuntimePathUnwritable,
			impact: "Mindrail cannot store derived cache data for this repository; nothing MR-001 does needs it.",
			next: []string{
				"Make " + s.cacheProbedPath() + " writable.",
				"Or point MINDRAIL_CACHE_DIR at a usable location.",
			},
		}))
		result.Details = details
		return result
	}

	return Result{
		State:   StateOK,
		Summary: "Runtime paths resolved under the Git common-dir",
		Details: details,
	}
}

// cacheProbedPath names the directory a cache remedy has to talk about: the
// cache directory itself when it exists, and otherwise the nearest existing
// ancestor, which is where the permission that blocked the create actually is.
func (s Subject) cacheProbedPath() string {
	if s.Probes.CacheDirKnown && s.Probes.CacheDir.Probed != "" {
		return s.Probes.CacheDir.Probed
	}
	return describePath(s.Paths.CacheDir)
}

// ConfigCheck reports the effective configuration and the layer each value came
// from, which is the question §37 says doctor has to be able to answer.
func ConfigCheck(s Subject) Check {
	return CheckFunc{
		CheckName:    checkConfig,
		CheckSection: sectionConfiguration,
		Fn:           func(context.Context) Result { return configResult(s) },
	}
}

func configResult(s Subject) Result {
	if !s.reached(stepLoadConfig) {
		h, _ := s.haltedAt()
		return unreached(h, "this repository's configuration")
	}

	if s.ConfigErr != nil {
		return failure(StateError, "Configuration rejected", explain(s.ConfigErr, diagnosis{
			code:   app.CodeConfigInvalid,
			impact: "Mindrail refuses to start on a configuration it cannot fully understand.",
			next: []string{
				"Correct or remove the reported key in .mindrail/config.toml.",
			},
		}))
	}

	details := map[string]string{
		config.KeyOutputColor: s.Config.Config.Output.Color,
	}
	if source, ok := s.Config.Provenance[config.KeyOutputColor]; ok {
		details[config.KeyOutputColor+".source"] = string(source)
	}
	if s.Config.RepoFile != "" {
		details["repo_file"] = s.Config.RepoFile
	}
	if s.Config.UserFile != "" {
		details["user_file"] = s.Config.UserFile
	}

	if len(s.Config.Warnings) > 0 {
		messages := make([]string, 0, len(s.Config.Warnings))
		// The first warning's code identifies the reading. A warning that
		// arrived without one still has to leave a code behind: a non-OK result
		// with an empty code is one no consumer can branch on.
		code := app.CodeConfigUnknownEnvVar
		for i, warning := range s.Config.Warnings {
			messages = append(messages, warning.Message)
			if i == 0 && warning.Code != "" {
				code = warning.Code
			}
		}

		result := failure(StateDegraded, "Configuration loaded with warnings", diagnosis{
			code:       code,
			diagnostic: strings.Join(messages, "\n"),
			impact:     "The reported settings are ignored, so the effective configuration is not the one the environment asks for.",
			next: []string{
				"Remove or correct the reported settings.",
			},
		})
		result.Details = details
		return result
	}

	return Result{State: StateOK, Summary: "Configuration loaded", Details: details}
}

// SQLiteCheck reports the runtime database. It never opens or creates one:
// the handle bootstrap managed to obtain, and the error it hit if it did not,
// are both already in the Subject (decision D-01).
//
// Every reading of the runtime store passes through Subject.supersede, which is
// what keeps the three checks that describe it from contradicting the error
// object rendered in the same document (finding H11).
func SQLiteCheck(s Subject) Check {
	return CheckFunc{
		CheckName:    checkSQLite,
		CheckSection: sectionRuntimeStore,
		Fn:           func(context.Context) Result { return s.supersede(sqliteResult(s)) },
	}
}

func sqliteResult(s Subject) Result {
	if !s.reached(stepOpenSQLite) {
		h, _ := s.haltedAt()
		return unreached(h, "the runtime database")
	}

	if s.DBErr != nil {
		return failure(StateError, "Runtime database unusable", explain(s.DBErr, diagnosis{
			code:   app.CodeRuntimeDBUnavailable,
			impact: "Nothing that reads or writes runtime state can run.",
			next: []string{
				"Check that " + describePath(s.Paths.DBPath) + " is a readable SQLite database.",
				"Retry once any other Mindrail process has finished.",
			},
		}))
	}

	if !s.DBPresent {
		// "Nothing is here yet" and "something is here that a database can never
		// be" stop the same commands, but only the first is fixed by `mindrail
		// init`; recommending it for the second sends the user around a loop
		// that fails identically every time (findings R2 and R3).
		if obstruction, blocked := s.obstruction(); blocked {
			return failure(StateError, "Runtime database path unusable", obstruction)
		}

		// Not initialised is a state, not a failure (decision D-01/D-03): it
		// exits 0 and is reported, so the fix is discoverable instead of fatal.
		return failure(StateUnavailable, "Runtime database not initialized", diagnosis{
			code:       app.CodeWorkspaceNotInitialized,
			diagnostic: describeUninitializedDB(s),
			impact:     "Runtime state is unavailable until this repository is initialized.",
			next:       []string{initCommand},
		})
	}

	if s.IntegrityErr != nil {
		return failure(StateError, "Runtime database failed its integrity check", explain(s.IntegrityErr, diagnosis{
			code:   app.CodeRuntimeDBCorrupt,
			impact: "The runtime database is damaged, so nothing it reports can be trusted.",
			next: []string{
				"Move " + describePath(s.Paths.DBPath) + " aside and run `mindrail init` to rebuild it.",
			},
		}))
	}

	return Result{
		State:   StateOK,
		Summary: "Runtime database healthy",
		Details: map[string]string{
			"db_path":      describePath(s.Paths.DBPath),
			"journal_mode": s.Pragmas.JournalMode,
			"foreign_keys": strconv.Itoa(s.Pragmas.ForeignKeys),
			"busy_timeout": strconv.Itoa(s.Pragmas.BusyTimeout),
			"synchronous":  strconv.Itoa(s.Pragmas.Synchronous),
		},
	}
}

// describeUninitializedDB says which of the two uninitialised states the reader
// is looking at.
//
// They are graded the same — both are "not initialised", both exit 0, both are
// cleared by `mindrail init` (finding H14) — but they do not look the same from
// the shell. Telling someone who can see a mindrail.db that the path "is empty"
// sends them hunting for a second problem, so the file an interrupted first run
// left behind is named as what it is.
func describeUninitializedDB(s Subject) string {
	path := describePath(s.Paths.DBPath)
	if s.Probes.DBFileUnwritten {
		return "The file at " + path + " is zero length: a previous `mindrail init` created it and did not get as far as writing a database."
	}
	return "The runtime database path " + path + " was inspected and is empty."
}

// MigrationCheck reports the schema ledger. It reads what bootstrap already
// applied or refused to apply; status and doctor never migrate (decision D-01).
func MigrationCheck(s Subject) Check {
	return CheckFunc{
		CheckName:    checkMigrations,
		CheckSection: sectionRuntimeStore,
		Fn:           func(context.Context) Result { return s.supersede(migrationResult(s)) },
	}
}

func migrationResult(s Subject) Result {
	if !s.reached(stepMigrateDB) {
		h, _ := s.haltedAt()
		return unreached(h, "the runtime schema")
	}

	if s.MigrateErr != nil {
		return failure(StateError, "Schema not usable", explain(s.MigrateErr, diagnosis{
			code:   app.CodeMigrationFailed,
			impact: "The runtime schema is not the one this binary expects.",
			next: []string{
				"Run `mindrail init` to apply the pending migrations.",
			},
		}))
	}

	if !s.DBPresent {
		if obstruction, blocked := s.obstruction(); blocked {
			obstruction.diagnostic = "No migration ledger was read: " + obstruction.diagnostic
			obstruction.impact = "The runtime schema is unavailable until the runtime database path can hold a database."
			return failure(StateUnavailable, "No schema to report", obstruction)
		}

		return failure(StateUnavailable, "No schema to report", diagnosis{
			code:       app.CodeWorkspaceNotInitialized,
			diagnostic: "The runtime database path " + describePath(s.Paths.DBPath) + " is empty, so no migration has ever been applied.",
			impact:     "The runtime schema is unavailable until this repository is initialized.",
			next:       []string{initCommand},
		})
	}

	if s.PendingCount > 0 {
		return failure(StateDegraded, "Schema is behind this binary", diagnosis{
			// The same failure mode as a migration that could not run — the
			// schema is not the one this binary expects — caught earlier and so
			// reported at a lower severity. It shares the code because a
			// consumer branching on "is the schema current?" needs one answer.
			code: app.CodeMigrationFailed,
			diagnostic: fmt.Sprintf("%d %s recorded in this binary %s not been applied.",
				s.PendingCount, plural(s.PendingCount, "migration", "migrations"), plural(s.PendingCount, "has", "have")),
			impact: "Commands that need the newer schema cannot run.",
			next:   []string{initCommand},
		})
	}

	current := int64(0)
	for _, applied := range s.Migrations {
		if applied.Version > current {
			current = applied.Version
		}
	}

	return Result{
		State:   StateOK,
		Summary: fmt.Sprintf("Schema up to date (version %d)", current),
		Details: map[string]string{
			"current_version": strconv.FormatInt(current, 10),
			"applied":         strconv.Itoa(len(s.Migrations)),
			"pending":         strconv.Itoa(s.PendingCount),
		},
	}
}

// KnowledgeCheck reports the repository-owned knowledge store. An absent store
// is healthy (decision D-06): a clean clone of a repository with no records
// must not look broken.
func KnowledgeCheck(s Subject) Check {
	return CheckFunc{
		CheckName:    checkKnowledge,
		CheckSection: sectionKnowledge,
		Fn:           func(context.Context) Result { return knowledgeResult(s) },
	}
}

func knowledgeResult(s Subject) Result {
	if !s.reached(stepValidateKnowledge) {
		h, _ := s.haltedAt()
		result := unreached(h, "the knowledge store")
		// The schema window is a property of this binary, not of the repository
		// (kernel-scope §3), so it is reported even here — a reader must never
		// have to conclude from a partial report that Mindrail reads no schema
		// at all (finding R5).
		result.Details = map[string]string{
			"root":                     loader.StoreRoot,
			"write_schema_version":     strconv.Itoa(KnowledgeWriteSchemaVersion(s.Knowledge)),
			"readable_schema_versions": joinInts(KnowledgeReadableSchemaVersions(s.Knowledge)),
		}
		return result
	}

	window := joinInts(KnowledgeReadableSchemaVersions(s.Knowledge))

	// The schema window is reported on every branch, healthy or not: it is what
	// this binary can read, and a store that failed to load does not change it.
	if s.KnowledgeErr != nil {
		result := failure(StateError, "Knowledge store unreadable", explain(s.KnowledgeErr, diagnosis{
			code:   app.CodeKnowledgeUnreadable,
			impact: "No decision or invariant is visible to any command.",
			next: []string{
				"Make " + loader.StoreRoot + " readable.",
			},
		}))
		result.Details = map[string]string{
			"root":                     storeRootOf(s.Knowledge),
			"write_schema_version":     strconv.Itoa(KnowledgeWriteSchemaVersion(s.Knowledge)),
			"readable_schema_versions": window,
		}
		return result
	}

	details := map[string]string{
		"root":                     storeRootOf(s.Knowledge),
		"present":                  strconv.FormatBool(s.Knowledge.Present),
		"decisions":                strconv.Itoa(len(s.Knowledge.Decisions)),
		"invariants":               strconv.Itoa(len(s.Knowledge.Invariants)),
		"write_schema_version":     strconv.Itoa(KnowledgeWriteSchemaVersion(s.Knowledge)),
		"readable_schema_versions": window,
	}

	fatal, degraded := splitProblems(s.Knowledge.Problems)
	if len(fatal) > 0 {
		result := failure(StateError, "Knowledge store not fully readable by this binary", diagnosis{
			code: app.CodeKnowledgeSchemaUnsupported,
			// The reader window belongs in the diagnostic, not among the
			// remedies: "This binary reads schema 1." is a fact about the
			// binary, and a next_action that cannot be carried out is not a
			// next action (finding R8).
			diagnostic: describeProblems(fatal) + "\nThis binary reads schema " + window + ".",
			impact:     "This binary sees only part of the store, and acting on a partial view of the invariants is worse than refusing to act.",
			next: []string{
				"Upgrade Mindrail to a version whose reader window covers these records.",
			},
		})
		result.Details = details
		return result
	}

	if len(degraded) > 0 {
		result := failure(StateDegraded, "Knowledge store has unreadable records", diagnosis{
			code:       app.CodeKnowledgeUnreadable,
			diagnostic: describeProblems(degraded),
			impact:     "The reported records are invisible to every command; the rest of the store is unaffected.",
			next:       recordRemedies(degraded),
		})
		result.Details = details
		return result
	}

	if !s.Knowledge.Present {
		return Result{
			State:   StateOK,
			Summary: "Knowledge store absent (no records yet)",
			Details: details,
		}
	}

	return Result{
		State: StateOK,
		Summary: fmt.Sprintf("Knowledge store readable (%d %s, %d %s)",
			len(s.Knowledge.Decisions), plural(len(s.Knowledge.Decisions), "decision", "decisions"),
			len(s.Knowledge.Invariants), plural(len(s.Knowledge.Invariants), "invariant", "invariants")),
		Details: details,
	}
}

// WorkspaceCheck reports whether this worktree is recorded in the runtime
// database. An unregistered worktree is a reportable state, not a failure
// (decision D-01).
func WorkspaceCheck(s Subject) Check {
	return CheckFunc{
		CheckName:    checkWorkspace,
		CheckSection: sectionWorkspace,
		Fn:           func(context.Context) Result { return s.supersede(workspaceResult(s)) },
	}
}

func workspaceResult(s Subject) Result {
	if !s.reached(stepRegisterWorkspace) {
		h, _ := s.haltedAt()
		return unreached(h, "this worktree's registration")
	}

	if s.WorkspaceErr != nil && !errors.Is(s.WorkspaceErr, workspace.ErrNotRegistered) {
		return failure(StateError, "Workspace could not be recorded", explain(s.WorkspaceErr, diagnosis{
			code:   app.CodeWorkspaceRegistrationFailed,
			impact: "Runtime state cannot be attributed to this worktree.",
			next: []string{
				"Run `mindrail init` again once the runtime database is usable.",
			},
		}))
	}

	if s.Workspace.ID == "" {
		// Without a database there was no lookup, so there is no row to have
		// found and none to report missing. Saying "no workspace row exists"
		// here asserted the result of a query nobody ran (finding R4).
		if !s.DBPresent {
			if obstruction, blocked := s.obstruction(); blocked {
				obstruction.diagnostic = "No workspace lookup was made: " + obstruction.diagnostic
				obstruction.impact = "Mindrail cannot attribute runtime state to this worktree until the runtime database path can hold a database."
				return failure(StateUnavailable, "Workspace registration not readable", obstruction)
			}

			return failure(StateUnavailable, "Workspace not registered", diagnosis{
				code: app.CodeWorkspaceNotInitialized,
				diagnostic: "There is no runtime database at " + describePath(s.Paths.DBPath) +
					" to hold a workspace row, so no lookup was made for " + describePath(s.Paths.WorktreeRoot) + ".",
				impact: "Mindrail cannot attribute runtime state to this worktree.",
				next:   []string{initCommand},
			})
		}

		// A database file whose migrations have not been applied has no
		// workspaces table to hold a row, which is what a first run looks like
		// from a second process that opened the file between the create and the
		// migration commit. Saying "the database was queried and holds no row"
		// there asserts a query that could not have run (finding F16).
		if s.PendingCount > 0 {
			return failure(StateUnavailable, "Workspace not registered", diagnosis{
				code: app.CodeWorkspaceNotInitialized,
				diagnostic: fmt.Sprintf(
					"The runtime schema is not established — %d %s still pending — so there is no workspace table to query for %s.",
					s.PendingCount, plural(s.PendingCount, "migration is", "migrations are"), describePath(s.Paths.WorktreeRoot)),
				impact: "Mindrail cannot attribute runtime state to this worktree until the runtime schema is in place.",
				next:   []string{initCommand},
			})
		}

		return failure(StateUnavailable, "Workspace not registered", diagnosis{
			code:       app.CodeWorkspaceNotInitialized,
			diagnostic: "The runtime database was queried and holds no workspace row for " + describePath(s.Paths.WorktreeRoot) + ".",
			impact:     "Mindrail cannot attribute runtime state to this worktree.",
			next:       []string{initCommand},
		})
	}

	return Result{
		State:   StateOK,
		Summary: "Workspace registered",
		Details: map[string]string{
			"workspace_id":       s.Workspace.ID,
			"project_id":         s.Workspace.ProjectID,
			"is_linked_worktree": strconv.FormatBool(s.Workspace.IsLinkedWorktree),
		},
	}
}

// initCommand is the one remedy several checks share. It is spelled once so a
// consumer that matches on it — and the tests that assert on it — cannot be
// broken by a stray rewording.
const initCommand = "mindrail init"

// diagnosis is the three things a non-OK result owes its reader, plus the code
// that identifies the failure mode and whatever machine-readable detail the
// layer that detected it attached.
type diagnosis struct {
	code       app.Code
	diagnostic string
	impact     string
	next       []string
	metadata   map[string]string
}

// explain prefers the diagnosis the failing layer attached to its own error.
// The layer that detected the failure knows more about it than the check
// reporting it does; the fallback exists so an error that arrived without a
// payload still produces a complete result rather than an empty block.
func explain(err error, fallback diagnosis) diagnosis {
	out := fallback

	if payload, ok := app.PayloadOf(err); ok {
		if payload.Code != "" {
			out.code = payload.Code
		}
		if payload.Why != "" {
			out.diagnostic = payload.Why
		}
		if payload.Impact != "" {
			out.impact = payload.Impact
		}
		if len(payload.NextAction) > 0 {
			out.next = payload.NextAction
		}
		if len(payload.Metadata) > 0 {
			out.metadata = payload.Metadata
		}
	}

	if out.diagnostic == "" && err != nil {
		out.diagnostic = err.Error()
	}
	return out
}

// failure assembles a non-OK result. It is the single constructor for them so
// that acceptance criterion 4 — diagnostic, impact and a next action on every
// non-OK reading — is structurally hard to forget.
func failure(state State, summary string, d diagnosis) Result {
	return Result{
		State:      state,
		Summary:    summary,
		Diagnostic: d.diagnostic,
		Impact:     d.impact,
		NextAction: d.next,
		Code:       d.code,
		Metadata:   d.metadata,
	}
}

// splitProblems separates the record this binary cannot read at all from the
// records it merely could not parse. The first blocks; the second costs the
// repository one record each (loader.Problem.Fatal).
func splitProblems(problems []loader.Problem) (fatal, nonFatal []loader.Problem) {
	for _, problem := range problems {
		if problem.Fatal {
			fatal = append(fatal, problem)
			continue
		}
		nonFatal = append(nonFatal, problem)
	}
	return fatal, nonFatal
}

func describeProblems(problems []loader.Problem) string {
	lines := make([]string, 0, len(problems))
	for _, problem := range problems {
		lines = append(lines, problem.Path+": "+problem.Message)
	}
	return strings.Join(lines, "\n")
}

// maxNamedRecords bounds how many offending files a remedy lists by name. A
// store with more broken records than this has a systemic problem, and a
// next_action longer than a screen is one nobody reads.
const maxNamedRecords = 10

// recordRemedies names the files to fix, one action per file.
//
// The paths belong in the remedy, not only in the diagnostic: `status` reports
// components without diagnostics, so "Fix or remove the reported record files"
// read on its own never said which files — leaving the one field a consumer is
// meant to act on unactionable (finding R6).
func recordRemedies(problems []loader.Problem) []string {
	actions := make([]string, 0, min(len(problems), maxNamedRecords)+1)
	for i, problem := range problems {
		if i == maxNamedRecords {
			remaining := len(problems) - maxNamedRecords
			actions = append(actions, fmt.Sprintf("Fix or remove the remaining %d unreadable %s under %s.",
				remaining, plural(remaining, "record", "records"), loader.StoreRoot))
			break
		}
		actions = append(actions, "Fix or remove "+problem.Path+".")
	}
	return actions
}

// KnowledgeWriteSchemaVersion reports the schema version this binary stamps on
// the records it writes.
//
// Kernel-scope §3 fixes it at schema.WriteVersion. It is a property of the
// binary and not of the repository, so a store that was never loaded — because
// startup stopped before the knowledge step — must not make it read as zero:
// that is the binary misreporting its own capability (finding R5).
func KnowledgeWriteSchemaVersion(store loader.Store) int {
	if store.WriteSchemaVersion == 0 {
		return schema.WriteVersion
	}
	return store.WriteSchemaVersion
}

// KnowledgeReadableSchemaVersions reports this binary's reader window, for the
// same reason and never empty: a consumer that iterates the field must not have
// to handle a null the healthy path never produces.
func KnowledgeReadableSchemaVersions(store loader.Store) []int {
	if len(store.ReadableSchemaVersions) == 0 {
		return schema.ReadableVersions()
	}
	return slices.Clone(store.ReadableSchemaVersions)
}

// describeStartDir keeps a diagnostic readable when the process could not even
// name the directory it was asked about.
func describeStartDir(dir string) string {
	if dir == "" {
		return "the working directory"
	}
	return dir
}

// storeRootOf reports the knowledge root even for a store that was never
// loaded, so the reader is told where Mindrail looked rather than nowhere.
func storeRootOf(store loader.Store) string {
	if store.Root != "" {
		return store.Root
	}
	return loader.StoreRoot
}

// describePath keeps a diagnostic readable when the path was never derived.
func describePath(p string) string {
	if p == "" {
		return "the runtime path (not derived)"
	}
	return p
}

func joinInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func plural(n int, singular, many string) string {
	if n == 1 {
		return singular
	}
	return many
}
