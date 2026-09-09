package doctor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
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
		// completeRootRemedy for the same reason the probe branch below gets it
		// (finding W6): the reader who moves the obstruction aside still has an
		// uninitialised repository, and the sentence that stops at the move is a
		// step short of a working install. It applies here as well because
		// `mindrail init` is the one command that reaches this branch — it is the
		// only mode that creates the roots — and until it did, the same file at
		// the same path produced a complete remedy from `status` and a truncated
		// one from `init`.
		return failure(StateError, "Runtime paths unusable", s.completeRootRemedy(explain(s.PathsErr, diagnosis{
			code:   app.CodeRuntimePathUnwritable,
			impact: "Mindrail cannot store runtime state for this repository.",
			next: []string{
				"Make the Git common directory writable.",
				"Set MINDRAIL_RUNTIME_DIR to a writable directory.",
			},
		})))
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

	// The repository config directory was printed as a location for five audits
	// and never as an answer, which is finding D4: a detail line naming a
	// directory nobody had asked a question about. Both readings are published on
	// every branch below, including the failing ones, because a reader who is
	// being told the runtime root is broken still has to be able to see whether
	// the worktree half of the installation is sound.
	if s.Probes.RepoConfigDirKnown {
		details["repo_config_dir_exists"] = strconv.FormatBool(s.Probes.RepoConfigDir.Exists)
		details["repo_config_dir_usable"] = strconv.FormatBool(s.Probes.RepoConfigDir.Usable)
	}

	if s.Probes.RuntimeDirKnown {
		details["runtime_root_exists"] = strconv.FormatBool(s.Probes.RuntimeDir.Exists)
		details["runtime_root_usable"] = strconv.FormatBool(s.Probes.RuntimeDir.Usable)
	}
	if s.Probes.CacheDirKnown {
		details["cache_dir_exists"] = strconv.FormatBool(s.Probes.CacheDir.Exists)
		details["cache_dir_usable"] = strconv.FormatBool(s.Probes.CacheDir.Usable)
	}

	// A repository config directory `mindrail init` still has to write into and
	// cannot is fatal, and is the other half of finding D4. Severity follows what
	// the condition actually prevents: here it prevents the one command in MR-001
	// that writes, so a report that graded it any lower would be the report that
	// exits 0 and recommends an init which exits 4. repoConfigBlocksInit is what
	// keeps that from over-firing on the adjacent condition — a `.mindrail` that
	// refuses writes but has nothing left to receive, where `mindrail init`
	// exits 0 and the repository works.
	//
	// It is asked before the runtime root, and the order is the fix rather than a
	// detail of it. Two failures can be true at once — a whole filesystem mounted
	// read-only makes both directories unusable — and when they are, whichever
	// this check names first is what the document's error object becomes. `init`
	// meets the repository config directory at §87 step 2 and the runtime root at
	// step 3, so naming the runtime root first published a different `why` and a
	// different path from the `init` that was about to fail on the same disk.
	// Following init's own order is what keeps the two documents describing one
	// condition the same way. Where only one of them is broken the order changes
	// nothing, because only one branch can fire.
	if repoConfig, blocked := s.repoConfigObstruction(); blocked {
		result := failure(StateError, "Repository config directory unusable", repoConfig)
		result.Details = details
		return result
	}

	if s.Probes.RuntimeDirKnown && !s.Probes.RuntimeDir.Usable {
		result := failure(StateError, "Runtime paths unusable", s.completeRootRemedy(explain(s.Probes.RuntimeDir.Err, diagnosis{
			code:   app.CodeRuntimePathUnwritable,
			impact: "Mindrail cannot store runtime state for this repository, so `mindrail init` would fail here too.",
			next: []string{
				"Make " + s.Probes.RuntimeDir.Probed + " writable.",
				"Or set MINDRAIL_RUNTIME_DIR to a writable directory.",
			},
		})))
		result.Details = details
		return result
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

	// The same directory, with the scaffold already in it. Nothing MR-001 does is
	// stopped — `mindrail init` re-run against it exits 0, having nothing to
	// create — so it is reported and it does not block. DEGRADED is §84's word
	// for running with less than it wants, and what is wanted here is the ability
	// to add to the repository's own knowledge tree.
	if s.Probes.RepoConfigDirKnown && !s.Probes.RepoConfigDir.Usable {
		unwritable := explain(s.Probes.RepoConfigDir.Err, diagnosis{
			code: app.CodeRuntimePathUnwritable,
			next: []string{"Make " + s.Probes.RepoConfigDir.Probed + " writable."},
		})
		unwritable.impact = "The repository scaffolding is already in place and every command still runs; " +
			"Mindrail cannot add anything to it while this directory refuses writes."

		result := failure(StateDegraded, "Repository config directory is not writable", unwritable)
		result.Details = details
		return result
	}

	return Result{
		State:   StateOK,
		Summary: describeRuntimeLocation(s),
		Details: details,
	}
}

// describeRuntimeLocation says where the runtime state actually is, rather than
// where it is by default.
//
// The summary used to assert "resolved under the Git common-dir" unconditionally,
// which MINDRAIL_RUNTIME_DIR makes false: a reader looking at a runtime root two
// directories away from any `.git` was told, by the check whose job is to answer
// "which paths did Mindrail actually pick?", that it was somewhere it is not.
// The detail lines carried the truth all along, and a summary that contradicts
// the lines under it is the disagreement this package exists to prevent.
func describeRuntimeLocation(s Subject) string {
	if underCommonDir(s.Repo.CommonDir, s.Paths.RuntimeRoot) {
		return "Runtime paths resolved under the Git common-dir"
	}
	return "Runtime paths resolved outside the Git common-dir, at " + s.Paths.RuntimeRoot
}

// underCommonDir reports whether root is the common dir or lives inside it. Both
// paths are already absolute and cleaned by the time a check sees them, so this
// is a prefix question and not a resolution one.
func underCommonDir(commonDir, root string) bool {
	if commonDir == "" || root == "" {
		return false
	}
	if root == commonDir {
		return true
	}
	return strings.HasPrefix(root, commonDir+string(filepath.Separator))
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

	details := map[string]string{
		"db_path":      describePath(s.Paths.DBPath),
		"journal_mode": s.Pragmas.JournalMode,
		"foreign_keys": strconv.Itoa(s.Pragmas.ForeignKeys),
		"busy_timeout": strconv.Itoa(s.Pragmas.BusyTimeout),
		"synchronous":  strconv.Itoa(s.Pragmas.Synchronous),
	}
	if s.Probes.DBWriteKnown {
		details["db_writable"] = strconv.FormatBool(s.Probes.DBWrite.Writable)
	}

	// Everything above this point established that the database could be *read*.
	// That is not the question `mindrail init` asks of it, and for four audits
	// this check answered the wrong one: a mindrail.db whose mode bits refuse
	// writes opens, reports WAL and foreign keys in force, and was reported
	// `Overall: OK` at exit 0 while every command that writes failed forever
	// (finding W5). A check that has not looked must not report OK, so the
	// writability probe is asked before the healthy verdict is given, not after.
	//
	// It is asked about space as well as about permission, which is what closes
	// finding D1. Detail and verdict were being read from two different probes:
	// `db_writable: false` was printed from the write probe on the line
	// immediately below `✓ Runtime database healthy`, which was decided without
	// consulting it, so a single block of one report asserted both halves of a
	// contradiction over a filesystem with zero bytes free.
	if unwritable, blocked := s.unwritableDatabase(); blocked {
		result := failure(StateError, unwritableDatabaseSummary(s.Probes.DBWrite.Blocker), unwritable)
		result.Details = details
		return result
	}

	return Result{
		State:   StateOK,
		Summary: "Runtime database healthy",
		Details: details,
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
		// The loader says the subtree could not be read. Whether that is a mode
		// bit, an entry standing in the way or a read-only mount is a question it
		// never asks, and its answer — inspect the directory, or delete it — is
		// the one `mindrail init` contradicts one step earlier with the remedy
		// that actually clears the condition (finding E10). Where the probe found
		// a path condition, the probe's answer is the one every command prints.
		reading := explain(s.KnowledgeErr, diagnosis{
			code:   app.CodeKnowledgeUnreadable,
			impact: "No decision or invariant is visible to any command.",
			next: []string{
				"Make " + loader.StoreRoot + " readable.",
			},
		})
		summary := "Knowledge store unreadable"
		if obstruction, blocked := s.knowledgeDirObstruction(); blocked {
			reading, summary = obstruction, "Knowledge directory unusable"
		}

		result := failure(StateError, summary, reading)
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
		reading := unreadableAlso(diagnosis{
			code:       app.CodeKnowledgeSchemaUnsupported,
			diagnostic: describeProblems(fatal),
			impact:     "This binary sees only part of the store, and acting on a partial view of the invariants is worse than refusing to act.",
			next: []string{
				"Upgrade Mindrail to a version whose reader window covers these records.",
			},
		}, degraded)
		// The reader window belongs in the diagnostic, not among the remedies:
		// "This binary reads schema 1." is a fact about the binary, and a
		// next_action that cannot be carried out is not a next action (finding
		// R8). It is appended last so it closes the account whether or not the
		// loader also reported records it could not read for another reason.
		reading.diagnostic += "\nThis binary reads schema " + window + "."

		result := failure(StateError, "Knowledge store not fully readable by this binary", reading)
		result.Details = details
		return result
	}

	// Decision D-44: a fatal loader Problem outranks every finding, which is why
	// the branch above returns before this one is reached. Steps 5-11 judge the
	// records this binary read; a verdict computed over a store the binary has
	// just admitted it cannot fully read would be a claim from incomplete data.
	cycles, invalid := splitFindings(s.KnowledgeFindings)

	// A cycle is the one fatal finding (decision D-39). It is reported ahead of
	// every other finding because it costs the repository a different thing: a
	// malformed record costs the records it names, while a lineage that closes
	// on itself makes "which decision is current" unanswerable for every record
	// in it.
	if len(cycles) > 0 {
		result := failure(StateError, "Knowledge store has a supersede cycle", unreadableAlso(diagnosis{
			code:       app.CodeKnowledgeSupersedeCycle,
			diagnostic: describeFindings(cycles, len(s.KnowledgeFindings)),
			impact:     "No record in the cycle has a newest version, so every answer about which decision is current is wrong rather than merely incomplete.",
			next:       findingRemedies(cycles),
		}, degraded))
		result.Details = details
		return result
	}

	if len(invalid) > 0 {
		result := failure(StateDegraded, "Knowledge store has invalid records", unreadableAlso(diagnosis{
			code:       app.CodeKnowledgeInvalid,
			diagnostic: describeFindings(invalid, len(s.KnowledgeFindings)),
			impact:     invalidRecordsImpact(degraded),
			next:       findingRemedies(invalid),
		}, degraded))
		result.Details = details
		return result
	}

	// Decision D-51. A record that resolves outside the repository was never
	// opened; what happened to it is that Mindrail declined to treat it as
	// repository content, on the path alone. The loader draws that distinction
	// deliberately (loader.escapingRecord) and publishes PATH_ESCAPES_ROOT for it,
	// and this is the only place a consumer can observe it: a reading carries one
	// Code, and until this branch existed every such record arrived under
	// KNOWLEDGE_UNREADABLE with the summary "Knowledge store has unreadable
	// records" and the remedy "Fix or remove <path>" — an account of a read
	// failure that never happened, with a remedy that names nothing to fix
	// (findings R-05, B-A6).
	//
	// This paragraph used to say the record "reads perfectly", and so did
	// recordRemedy's. That is the claim escapingDiagnostic below now calls
	// fabricated, and it was wrong for the same reason KNOWLEDGE_UNREADABLE was:
	// a link pointing at a deleted or unopenable target reaches this branch
	// identically, and nothing here has ever held its bytes. What the reading may
	// say is where the record is, not what is in it (finding R4-M22).
	//
	// It is a homogeneity test and not a per-record one because a reading
	// publishes exactly one Code. A store holding both classes keeps
	// KNOWLEDGE_UNREADABLE — the reading that is wrong about neither, since a
	// record the loader could not read is genuinely unreadable — and its summary
	// and impact then name both classes rather than hiding one behind the other.
	//
	// Its rung is the degraded-problem rung it shares with the branch below: a
	// fatal loader Problem still outranks it (D-44) and so does a cycle
	// (D-39/D-43), and in those readings the escaping record is named by
	// unreadableAlso in the words of its own class.
	if allEscapeRoot(degraded) {
		result := failure(StateDegraded, "Knowledge store has records that resolve outside the repository", diagnosis{
			code:       app.CodePathEscapesRoot,
			diagnostic: escapingDiagnostic(degraded),
			impact:     escapingImpact(len(degraded)),
			next:       recordRemedies(degraded),
		})
		result.Details = details
		return result
	}

	if len(degraded) > 0 {
		result := failure(StateDegraded, degradedSummary(degraded), diagnosis{
			code:       app.CodeKnowledgeUnreadable,
			diagnostic: describeProblems(degraded),
			impact:     degradedImpact(degraded),
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

// escapeRootCount counts the records in a set that Mindrail declined to treat as
// repository content, as opposed to the ones it could not read.
//
// The key is loader.Problem.Code and not the prose in Message, for the reason
// splitFindings keys on Code: the code is the value a consumer branches on, and
// a classification taken from a sentence would disagree with the published code
// the first time the sentence is reworded.
func escapeRootCount(problems []loader.Problem) int {
	escaping := 0
	for _, problem := range problems {
		if problem.Code == app.CodePathEscapesRoot {
			escaping++
		}
	}
	return escaping
}

// allEscapeRoot reports whether every record in a non-empty set resolves outside
// the repository. An empty set is not homogeneously anything, so it is false:
// the branch guarded by this function must not fire on a store with nothing
// wrong with it.
func allEscapeRoot(problems []loader.Problem) bool {
	return len(problems) > 0 && escapeRootCount(problems) == len(problems)
}

// recordNouns names what a set of loader problems is, so that a tail counting
// them counts something the reader can identify.
//
// A set that is homogeneously escaping is not a set of records this binary could
// not read, and "... and 5 further unreadable records" said about five files
// that were never opened is the same false statement the KNOWLEDGE_UNREADABLE
// code was making about them (decision D-51).
//
// A mixed set gets a third noun rather than borrowing one of the two. Borrowing
// the unreadable one is what the mixed reading does with the *code*, and that is
// sound because the code has to be a single value and KNOWLEDGE_UNREADABLE is
// wrong about none of the set — but a noun is not a code. It appears in a
// counting sentence directly below an impact that has just split the same set
// into "N could not be read" and "M resolve outside the repository", so filing
// all of them under one of those two words makes the reading contradict itself
// about records the reader can see listed above.
func recordNouns(problems []loader.Problem) (one, many string) {
	escaping := escapeRootCount(problems)
	switch {
	case escaping == 0:
		return "unreadable record", "unreadable records"
	case escaping == len(problems):
		return "record resolving outside the repository", "records resolving outside the repository"
	default:
		return "unreadable or declined record", "unreadable or declined records"
	}
}

func describeProblems(problems []loader.Problem) string {
	lines := make([]string, 0, len(problems))
	for _, problem := range problems {
		lines = append(lines, problem.Path+": "+problem.Message)
	}
	one, many := recordNouns(problems)
	// The set this sentence counts is the slice it was handed and not every
	// problem in the store, because the reading prints the loader's problems in
	// blocks that partition them: the fatal-branch reading renders the fatal
	// records here, and the degraded ones a few lines below under alsoHeading,
	// which carries their own count. A store-wide total here would say "not
	// listed here" about records listed immediately underneath.
	return describeBounded(lines, len(lines), one, many)
}

// unreadableAlso names the records the loader could not read on a reading the
// precedence ladder has already given to something else.
//
// AC-06.1 fixes that ladder — a fatal loader problem outranks every finding, a
// cycle outranks every other finding, and any finding outranks a merely
// unreadable record — so this function changes no State, no Code and no Summary.
// The ladder decides which condition names the reading. It does not license
// silence about the others, and silence is what was there: a store carrying both
// an unreadable record and a finding reported KNOWLEDGE_INVALID and named the
// unreadable file in no diagnostic and no next_action on any command. `status`
// published the count without the name, `doctor` published neither, so the file
// the reader had to open appeared nowhere at all.
//
// The name goes in both fields because neither is sufficient alone.
// status.componentFrom keeps next_action and drops Diagnostic and Impact (MR-001
// finding R6), so a file named only in the diagnostic is invisible exactly where
// status prints the reading; and a remedy with no account behind it does not say
// why the file is named. recordRemedies supplies the sentence rather than a new
// one, so the phrasing stays the one the CLI's remedy classifier already reads
// and the two renderings of an unreadable record do not drift apart.
func unreadableAlso(d diagnosis, degraded []loader.Problem) diagnosis {
	if len(degraded) == 0 {
		return d
	}

	d.diagnostic += "\n" + alsoHeading(degraded) + "\n" + describeProblems(degraded)
	d.next = slices.Concat(d.next, recordRemedies(degraded))
	return d
}

// alsoHeading introduces the degraded records on a reading the ladder gave to
// something else, in the words of the class they belong to.
//
// "The loader could not read 1 further record" is a false sentence about a
// record that resolves outside the repository: the loader read nothing from it
// because Mindrail declined to treat it as repository content, not because it
// could not (decision D-51). The distinction survives the ladder — a record
// reported below a cycle is still the same record — so the heading has to carry
// it, and a mixed set names both counts rather than filing one class under the
// other's sentence.
func alsoHeading(degraded []loader.Problem) string {
	escaping := escapeRootCount(degraded)
	unread := len(degraded) - escaping

	switch {
	case escaping == 0:
		return fmt.Sprintf("The loader could not read %d further %s:",
			unread, plural(unread, "record", "records"))
	case unread == 0:
		return fmt.Sprintf("Mindrail declined %d further %s as repository content, because %s outside the repository root:",
			escaping, plural(escaping, "record", "records"), plural(escaping, "it resolves", "they resolve"))
	default:
		return fmt.Sprintf("The loader could not read %d further %s, and Mindrail declined %d more that %s outside the repository root:",
			unread, plural(unread, "record", "records"), escaping, plural(escaping, "resolves", "resolve"))
	}
}

// degradedSummary is the one-line reading for a degraded set that is not
// homogeneously escaping.
//
// A mixed set stays under KNOWLEDGE_UNREADABLE (decision D-51) because that code
// is wrong about none of it, but the summary is the only field `status` prints
// beside the code, so a mixed set that read "Knowledge store has unreadable
// records" would hide the escaping half entirely.
func degradedSummary(degraded []loader.Problem) string {
	if escapeRootCount(degraded) > 0 {
		return "Knowledge store has unreadable records and records that resolve outside the repository"
	}
	return "Knowledge store has unreadable records"
}

// degradedImpact says what a degraded set costs. The homogeneous unreadable
// sentence is the one this reading has always printed and is left exactly as it
// was, down to the byte; a set that also holds escaping records earns the
// account that counts both classes, because "the reported records are invisible
// because they could not be read" is false about half of that set.
func degradedImpact(degraded []loader.Problem) string {
	if escapeRootCount(degraded) > 0 {
		return degradedAccount(degraded) + "; the rest of the store is unaffected."
	}
	return "The reported records are invisible to every command; the rest of the store is unaffected."
}

// escapingDiagnostic is this package's own account of a homogeneously escaping
// degraded set.
//
// The lead sentence is written here rather than assembled from the loader's
// Problem.Message alone. The loader's message does say it today, and a
// diagnostic that only said it because that sentence happened to would be a
// property of another package's prose: a fixture carrying a bare message would
// leave this reading claiming nothing at all about what happened. The per-record
// accounts follow it, so the file names AC-06.2 requires are still there.
//
// It says the file was not opened, and deliberately does not say the file is
// readable. The two sound alike and only one of them is something this binary
// knows: PATH_ESCAPES_ROOT is decided by resolving the path, before any read is
// attempted (loader.recordFile), so a link pointing at a target that does not
// exist, or that cannot be opened, arrives here exactly like one pointing at a
// perfect record. Asserting "the file is readable" about that link is the same
// fabricated claim about a file this run never held that decision D-51 exists to
// remove, inverted — the reading would be wrong in the other direction and just
// as confidently.
func escapingDiagnostic(escaping []loader.Problem) string {
	n := len(escaping)
	return fmt.Sprintf(
		"Mindrail declined to treat %d %s under %s as repository content, because %s outside the repository root. %s not opened at all: what is wrong is where the bytes live, not what they say.\n",
		n, plural(n, "record", "records"), loader.StoreRoot,
		plural(n, "it resolves", "they resolve"),
		plural(n, "The file was", "The files were"),
	) + describeProblems(escaping)
}

// escapingImpact says what declining a record costs, which is the record and
// nothing else (decision D-06). It does not say the record could not be read,
// because that is not what happened to it.
//
// Both verbs are pluralised with their subject. That is not fussiness: this
// field is published verbatim in `doctor --json` and shown to a reader, and the
// subject moved to the plural in the D-51 branch while the verbs stayed
// singular, so every store with two or more declined records printed a sentence
// that does not parse. It went unnoticed because no test read this field at all
// — TestTheDeclinedImpactAgreesWithItsOwnSubject is now the one that does.
func escapingImpact(n int) string {
	return fmt.Sprintf("%s no decision and no invariant into this repository, so whatever %s is not in force here; the rest of the store is unaffected.",
		plural(n, "The declined record carries", "The declined records carry"),
		plural(n, "it records", "they record"))
}

// invalidRecordsImpact says what a store of invalid records costs, and does not
// say the rest of it is intact when part of it could not be read.
//
// The clause is computed rather than constant because it is a claim: "the rest
// of the store is unaffected" is true of a store whose only defect is records
// that break their schema, and false of one that also holds records no command
// can see. Asserting it on both was the same shape as the finding above — the
// reading told the reader about one condition and quietly denied the other.
func invalidRecordsImpact(degraded []loader.Problem) string {
	const wrong = "The reported records are readable but wrong, so anything derived from them rests on a record that breaks the contract this repository declares"
	if len(degraded) == 0 {
		return wrong + "; the rest of the store is unaffected."
	}
	return fmt.Sprintf("%s, and the rest of the store is not intact either: %s.", wrong, degradedAccount(degraded))
}

// degradedAccount says what a degraded set costs, counting each class in its own
// words.
//
// "could not be read at all" is a claim about the loader's attempt, and it is
// false of a record that resolves outside the repository: there was no attempt —
// Mindrail declined it on its path, before opening anything (decision D-51, as
// amended after audit round 4). The account is shared by every reading
// that has to describe a degraded set, so two readings of one store cannot drift
// into describing it differently.
func degradedAccount(degraded []loader.Problem) string {
	escaping := escapeRootCount(degraded)
	unread := len(degraded) - escaping

	switch {
	case escaping == 0:
		return fmt.Sprintf("%d %s could not be read at all and %s invisible to every command",
			unread, plural(unread, "record", "records"), plural(unread, "is", "are"))
	case unread == 0:
		return fmt.Sprintf("%d %s outside the repository root and %s declined rather than read, so %s invisible to every command",
			escaping, plural(escaping, "record resolves", "records resolve"),
			plural(escaping, "was", "were"), plural(escaping, "it is", "they are"))
	default:
		return fmt.Sprintf("%d %s could not be read at all, and %d %s outside the repository root and %s declined rather than read, leaving all %d invisible to every command",
			unread, plural(unread, "record", "records"),
			escaping, plural(escaping, "record resolves", "records resolve"),
			plural(escaping, "was", "were"), len(degraded))
	}
}

// maxDiagnosticLines and maxDiagnosticLineBytes bound the account a knowledge
// reading prints. maxNamedRecords bounds the remedies beside it.
//
// AC-06.4 asked for that remedy cap and it was applied where it was asked for;
// this pair is not an extension of it but the bound the diagnostic never had.
// The diagnostic was unbounded in two independent directions, and closing either
// alone leaves the other open.
//
// The first direction is how many accounts are printed: describeProblems and
// describeFindings joined every problem and every finding in the store, so the
// report grew with the repository. Measured on this tree, `doctor --json` over a
// healthy store is 2,164 / 2,175 / 2,186 bytes at 10 / 100 / 1000 records — flat
// — and was 6,009 / 28,140 / 248,652 bytes over the same counts of records
// carrying one short finding each.
//
// The second is how long one account is. The message comes from the package that
// detected the condition, and a finding that names every record it collides with
// is a single line that grows with the store on its own: 1000 records sharing
// one id produced a 45,386,439-byte report from 1,999 lines, 1,000 of which were
// 45,134 bytes each. Capping the number of lines alone would still have printed
// 451 KB of it.
//
// Both bounds sit far above anything this binary writes for a store that is
// merely broken — the longest real account measured is 240 bytes — so an
// ordinary failing repository renders exactly as it did before, and only a
// report that has stopped being readable is abbreviated.
const (
	maxDiagnosticLines     = 10
	maxDiagnosticLineBytes = 1000
)

// describeBounded renders the accounts a reading can print and counts the ones
// it cannot.
//
// Nothing is dropped in silence. Accounts past the line cap are counted in a
// tail that says where they are, and an account past the byte cap keeps its
// head and carries the number of bytes it lost. Keeping the head is what makes
// the truncation safe to do here: both callers write the record path first and
// the rule second, so the part a cut can reach is the end of the prose and never
// the file name AC-06.2 requires the reading to carry.
//
// total is the size of the set the tail sentence names, which is not always
// len(lines). The tail says "and N further <noun> under .mindrail/knowledge" —
// a statement about the store, not about the block — and it was computed from
// the block: a 25-record cycle beside five schema-invalid records rendered the
// ten cycle accounts the cap allows and published "... and 15 further findings",
// while `status` published 30 for the same store (finding B-A2). The number and
// the words now name one set, and every caller states which set that is.
//
// A total below the number of accounts on hand needs no guard of its own: the
// tail is printed on a positive omission, so a caller that understates prints
// every account it has and claims nothing beyond them. The floor that was
// written here for it was a branch nothing could reach and nothing could
// falsify, which is the shape of guard MR-001's second audit round was spent
// deleting.
func describeBounded(lines []string, total int, one, many string) string {
	shown := min(len(lines), maxDiagnosticLines)

	out := make([]string, 0, shown+1)
	for _, line := range lines[:shown] {
		out = append(out, truncateAccount(line))
	}
	if omitted := total - shown; omitted > 0 {
		out = append(out, fmt.Sprintf("... and %d further %s under %s, not listed here.",
			omitted, plural(omitted, one, many), loader.StoreRoot))
	}
	return strings.Join(out, "\n")
}

// truncateAccount cuts one account to maxDiagnosticLineBytes and says how much
// it cut.
//
// The cut lands on a rune boundary: a diagnostic is carried through a JSON
// envelope, and half a rune there is a byte sequence the encoder has to replace
// rather than a shorter sentence.
func truncateAccount(line string) string {
	if len(line) <= maxDiagnosticLineBytes {
		return line
	}

	cut := maxDiagnosticLineBytes
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + fmt.Sprintf("... (%d further bytes not shown)", len(line)-cut)
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
			actions = append(actions, remainingRecordRemedy(problems[maxNamedRecords:]))
			break
		}
		actions = append(actions, recordRemedy(problem))
	}
	return actions
}

// recordRemedy is what one degraded record asks of the reader.
//
// The two classes ask for different things, and that difference is the whole of
// decision D-51. "Fix or remove <path>" is the right sentence for a file this
// binary could not read; it is the wrong one for a file that was declined
// because it resolves outside the repository, where the thing to act on is the
// link and nothing in the file is under discussion at all.
//
// The reason is where the record is, not that its contents are fine. This
// comment used to say the declined file "reads perfectly", which is the claim
// escapingDiagnostic calls fabricated: a link pointing at a deleted target is
// declined identically, and the remedy below is right for it too — replace the
// link, or remove it — precisely because the remedy is about the link.
func recordRemedy(problem loader.Problem) string {
	if problem.Code == app.CodePathEscapesRoot {
		return "Replace the link at " + problem.Path + " with the record itself, or remove it."
	}
	return "Fix or remove " + problem.Path + "."
}

// remainingRecordRemedy closes the remedy list once maxNamedRecords paths have
// been named.
//
// It counts the records it did not name and no others: the set is exactly the
// tail of the slice recordRemedies was handed, so shown + remaining is that
// slice and never the store. That matters on the fatal branch, where the
// degraded records are the only ones this sentence applies to — the fatal ones
// are repaired by upgrading the binary, and counting them into an instruction to
// fix or remove files would ask the reader to delete records that are merely too
// new to read.
//
// The arms match recordRemedy's, one class at a time, because they are the same
// instruction addressed to a group. A mixed tail earns both halves: the ten
// remedies immediately above it were each written in their own record's words,
// and a tail that told the reader to "fix or remove the remaining 4 unreadable
// records" when two of them are links that were never opened asks for an edit to
// a file with nothing in it to edit — which is the exact sentence decision D-51
// exists to stop this package printing.
func remainingRecordRemedy(rest []loader.Problem) string {
	escaping := escapeRootCount(rest)
	unread := len(rest) - escaping

	switch {
	case escaping == 0:
		return fmt.Sprintf("Fix or remove the remaining %d unreadable %s under %s.",
			unread, plural(unread, "record", "records"), loader.StoreRoot)
	case unread == 0:
		return fmt.Sprintf("Replace or remove the remaining %d %s under %s that %s outside the repository root.",
			escaping, plural(escaping, "link", "links"), loader.StoreRoot,
			plural(escaping, "resolves", "resolve"))
	default:
		return fmt.Sprintf("Fix or remove the remaining %d unreadable %s under %s, and replace or remove the %d %s there that %s outside the repository root.",
			unread, plural(unread, "record", "records"), loader.StoreRoot,
			escaping, plural(escaping, "link", "links"),
			plural(escaping, "resolves", "resolve"))
	}
}

// splitFindings separates the one fatal finding class from the rest, keyed on
// the Code the branch above publishes rather than on the Fatal flag beside it.
//
// The code is the right key because it is the value a consumer branches on:
// `status` publishes exactly one Code per component, and a finding filed here as
// a cycle is a finding reported to the world as KNOWLEDGE_SUPERSEDE_CYCLE. Using
// Fatal instead would let the two disagree, and the disagreement would be
// invisible — the state would say ERROR while the code said the records were
// merely invalid (decisions D-39, D-43).
func splitFindings(findings []validate.Finding) (cycles, invalid []validate.Finding) {
	for _, finding := range findings {
		if finding.Code == app.CodeKnowledgeSupersedeCycle {
			cycles = append(cycles, finding)
			continue
		}
		invalid = append(invalid, finding)
	}
	return cycles, invalid
}

// knowledgeStepNames spells spec §95's pipeline steps in the words the
// specification, the design and internal/knowledge/validate's package doc all
// use. A report that says "step 6 (filename <-> kind/id consistency)" names the
// same rule those three documents name, which is the point of the step numbers
// being the specification's.
var knowledgeStepNames = map[validate.Step]string{
	validate.StepSchema:                 "JSON Schema validation",
	validate.StepFilenameConsistency:    "filename <-> kind/id consistency",
	validate.StepUniqueID:               "unique ID",
	validate.StepSupersedeTarget:        "supersede target",
	validate.StepSupersedeCycle:         "supersede DAG/cycle",
	validate.StepDuplicateActiveLineage: "duplicate active lineage",
	validate.StepScopeSyntax:            "scope syntax",
}

// knowledgeStep renders one step for a reader.
//
// A step this binary has no name for is still reported by its number rather
// than dropped or renamed. Steps 12, 13 and 14 exist in the specification and
// are deferred, not absent (AC-12.2), so the day one of them starts producing
// findings the report says "step 12" instead of quietly attributing it to a
// rule this table does know.
func knowledgeStep(step validate.Step) string {
	name, known := knowledgeStepNames[step]
	if !known {
		return fmt.Sprintf("step %d", int(step))
	}
	return fmt.Sprintf("step %d (%s)", int(step), name)
}

// describeFindings renders what steps 5-11 found.
//
// It is deliberately not describeProblems (AC-06.2). That function renders the
// loader's account, every line of which means "this binary could not read the
// record"; every line here means "this binary read it and it is wrong", and the
// two have opposite remedies (decision D-38). The rule that was broken is named
// as well as the file, because "wrong" without "which rule" leaves the reader
// with nothing to correct.
//
// The path is written by this function rather than left to the finding's own
// message. The messages do carry it today, but a diagnostic that names the file
// only when the sentence inside it happens to would be a property of prose
// rather than of the report.
//
// total is every finding steps 5-11 produced for the store, not just the class
// this reading is rendering. The precedence ladder hands one class to the
// diagnostic and says nothing about the other, so the omission tail is the only
// place the rest of them are counted at all; counting the block instead made the
// tail understate the store by the whole of the other class (finding B-A2).
func describeFindings(findings []validate.Finding, total int) string {
	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		lines = append(lines, finding.Path+": "+knowledgeStep(finding.Step)+": "+finding.Message)
	}
	return describeBounded(lines, total, "finding", "findings")
}

// findingRemedyByStep is the action each rule asks of the reader.
//
// One sentence per step rather than one shared "fix the reported records": the
// steps break in different ways and are repaired differently, and a remedy that
// does not say what to change is the same unactionable field finding R6 was
// about. Each carries a %s for the offending path, because status.componentFrom
// drops Diagnostic and Impact and keeps only these sentences (AC-06.4).
var findingRemedyByStep = map[validate.Step]string{
	validate.StepSchema:                 "Correct %s so it satisfies the knowledge schema document for its kind.",
	validate.StepFilenameConsistency:    "Rename %s, or change the id inside it, so the file name and the id agree.",
	validate.StepUniqueID:               "Give %s an id no other record carries, or delete whichever copy is redundant.",
	validate.StepSupersedeTarget:        "Add the record %s supersedes, or drop that id from its supersedes list.",
	validate.StepSupersedeCycle:         "Break the supersede cycle by dropping the superseded id from %s.",
	validate.StepDuplicateActiveLineage: "Set status \"superseded\" on %s if it is not the current record of its lineage.",
	validate.StepScopeSyntax:            "Correct the scope in %s to a level from the enum and a repository-relative forward-slash target.",
}

// findingRemedies names the records to fix, one action per record.
//
// One action per *record*, not per finding: a record that breaks six schema
// rules is one file to open, and printing the same path six times pushes the
// other offending files off the end of the maxNamedRecords cap. The step whose
// remedy is printed is the first one reported against that path, which is the
// lowest-numbered step because Check sorts by (Path, Step, ...) — and the lowest
// step is the right one to lead with, since a record that fails step 5 was never
// asked steps 6-11 (decision D-40).
func findingRemedies(findings []validate.Finding) []string {
	paths := make([]string, 0, len(findings))
	step := make(map[string]validate.Step, len(findings))
	for _, finding := range findings {
		if _, seen := step[finding.Path]; seen {
			continue
		}
		step[finding.Path] = finding.Step
		paths = append(paths, finding.Path)
	}

	actions := make([]string, 0, min(len(paths), maxNamedRecords)+1)
	for i, path := range paths {
		if i == maxNamedRecords {
			remaining := len(paths) - maxNamedRecords
			actions = append(actions, fmt.Sprintf("Correct the remaining %d reported %s under %s.",
				remaining, plural(remaining, "record", "records"), loader.StoreRoot))
			break
		}
		actions = append(actions, fmt.Sprintf(remedyFormatFor(step[path]), path))
	}
	return actions
}

// remedyFormatFor falls back to naming the file alone when a step has no remedy
// of its own. It is reachable the same day knowledgeStep's fallback is — a
// deferred step starting to report — and it exists for the same reason: a
// finding with no sentence for it must still leave the reader a file to open,
// not an empty next_action that assertDiagnosable would be right to reject.
func remedyFormatFor(step validate.Step) string {
	if format, known := findingRemedyByStep[step]; known {
		return format
	}
	return "Correct %s, which this binary read and rejected."
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
