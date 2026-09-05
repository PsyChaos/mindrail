package doctor

import (
	"slices"
	"strings"

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

	// RepoConfigDir answers the same question about <worktree>/.mindrail, the
	// directory `mindrail init` lays the repository scaffolding down in.
	//
	// It is asked apart from the two above because EnsureDirs does not create it:
	// it is repository content, committed and shared, not machine-local state.
	// That exemption is exactly why nothing ever asked about it, and it is
	// finding D4. On a checkout whose worktree refuses new entries, `doctor`
	// printed `repo_config_dir` as a detail line, reported all seven checks OK,
	// exited 0 and offered `mindrail init` as the remedy — and that init failed
	// with RUNTIME_PATH_UNWRITABLE and exit 4 every single time it was run. The
	// report named the directory and never asked the one question about it that
	// decides whether its own remedy can succeed.
	//
	// RepoConfigDirKnown is the discriminator a zero value cannot supply, with
	// the same meaning it has on the two roots above: an unasked question and an
	// unwritable directory both leave Usable false.
	RepoConfigDir      filesystem.Writability
	RepoConfigDirKnown bool

	// DBPath is what occupies the runtime database path, and DBPathErr the
	// structured obstruction when that is something a database can never be.
	// An empty DBPath means the question was not asked.
	DBPath    storage.Presence
	DBPathErr error

	// DBFileUnwritten marks a zero-length file sitting at the database path: an
	// interrupted first run. It is reported at the same severity as an absent
	// database, because it is the same state (finding H14), but the diagnostic
	// has to say which of the two the reader is looking at — "there is nothing
	// here" sends someone who can see the file looking for a second problem.
	DBFileUnwritten bool

	// DBWrite answers whether the runtime database could be *written*, which is
	// a different question from whether it opened.
	//
	// Every probe before this one stopped at openability, and a mindrail.db whose
	// mode bits refuse writes opens perfectly: doctor read its pragmas back, found
	// WAL and foreign keys in force, printed `Overall: OK` and exited 0 while
	// `mindrail init` in the same directory failed forever (finding W5). A check
	// that has not looked must not report OK, and "opened successfully" is not
	// "usable".
	//
	// DBWriteKnown is the discriminator a zero value cannot supply: an
	// unanswered probe and a refused write both leave Writable false, and reading
	// the first as the second would fail every Subject a test builds by hand.
	DBWrite      storage.WriteAccess
	DBWriteKnown bool
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

	// The repository config directory is not one of the roots EnsureDirs creates,
	// so ProbeRoots has nothing to say about it — and neither did anything else
	// in the report, which is finding D4. It is asked here, beside the roots,
	// rather than by the check that reports it, so status and doctor read the
	// same disk.
	if repoConfig, known := s.Paths.ProbeRepoConfig(); known {
		s.Probes.RepoConfigDir = repoConfig
		s.Probes.RepoConfigDirKnown = true
	}

	// The presence probe only has something to add where bootstrap's own answer
	// was "no database": RuntimePaths.Exists() is a stat that cannot tell an
	// empty path from an occupied one, and the occupied case is precisely the
	// one `mindrail init` cannot fix.
	if s.reached(stepOpenSQLite) && !s.DBPresent && s.DBErr == nil && s.Paths.DBPath != "" {
		s.Probes.DBPath, s.Probes.DBPathErr = storage.Probe(s.Paths.DBPath)
		s.Probes.DBFileUnwritten = s.Probes.DBPath == storage.PresenceFile &&
			storage.IsUnwritten(s.Paths.DBPath)
	}

	// The write-access probe is asked wherever the database path is known and the
	// open step was reached, because the condition it finds is invisible to every
	// other reading in the report: the database opens, its pragmas read back
	// correctly, and nothing else in the sequence attempts a write. Asking costs
	// three access(2) calls and no mutation, so decision D-01 still holds.
	if s.reached(stepOpenSQLite) && s.Paths.DBPath != "" {
		s.Probes.DBWrite = storage.ProbeWriteAccess(s.Paths.DBPath)
		s.Probes.DBWriteKnown = true
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
	if unwritable, blocked := s.unwritableDatabase(); blocked {
		return unwritable, true
	}
	return diagnosis{}, false
}

// repoConfigBlocksInit reports a repository config directory that `mindrail
// init` would fail on, as opposed to one it has no work left to do in.
//
// The distinction was measured rather than assumed, because guessing it in
// either direction is a finding. Against a fully scaffolded `.mindrail` at mode
// 0500, `mindrail init` exits 0: WriteIfAbsent finds config.toml already there
// and writes nothing, EnsureKnowledgeDirs finds both knowledge directories and
// their .gitkeep files already there and creates nothing, and the repository
// keeps working. Grading that as fatal would be finding F13 again, one directory
// over — a fully working repository driven to ERROR and exit 4 over a directory
// this run never needs to write to. Against the same directory with
// `.mindrail/knowledge` removed, init exits 4: the mkdir it has to perform lands
// directly in `.mindrail`.
//
// So the condition is exactly "init still has an entry to create in .mindrail
// itself", and the two entries it creates there are config.toml and knowledge/.
// Both are already answered by the Subject: the config loader records RepoFile
// only for a file it actually found, and the knowledge loader records Present
// only for a directory it actually stat'ed. Re-deriving either from a path
// string here would put a second copy of the scaffold's layout in a package that
// does not own it.
//
// It claims nothing when the knowledge step never ran. A startup that stopped at
// the runtime store leaves Knowledge.Present false because nobody looked, and
// reading that as "the scaffold is missing" would be the report inventing a
// finding out of an unasked question — the defect this whole file exists to
// prevent.
func (s Subject) repoConfigBlocksInit() bool {
	if !s.Probes.RepoConfigDirKnown || s.Probes.RepoConfigDir.Usable {
		return false
	}
	if !s.reached(stepValidateKnowledge) || s.KnowledgeErr != nil {
		return false
	}
	return s.Config.RepoFile == "" || !s.Knowledge.Present
}

// repoConfigObstruction is the repository config directory as a blockage, for
// the readings that would otherwise send the reader to `mindrail init`.
//
// It is deliberately not part of obstruction(). That function answers for the
// runtime store, and its callers name their own summaries from it — a reading
// that took this answer through it printed "Runtime database path unusable" over
// a database path with nothing wrong with it, mislabelling both the state and
// the subject to deliver a remedy about the working tree. supersede is the one
// caller that wants it, because supersede replaces only the remedy and leaves
// the reading's own state and summary alone.
//
// The remedy comes from the filesystem probe, which is the same function
// `init` itself fails through, so doctor, status and the init that fails here
// print one sentence naming one path rather than three.
func (s Subject) repoConfigObstruction() (diagnosis, bool) {
	if !s.repoConfigBlocksInit() {
		return diagnosis{}, false
	}

	return explain(s.Probes.RepoConfigDir.Err, diagnosis{
		code:   app.CodeRuntimePathUnwritable,
		impact: filesystem.RootRepository.Impact() + ".",
		next:   []string{"Make " + s.Probes.RepoConfigDir.Probed + " writable."},
	}), true
}

// unwritableDatabase reports a runtime database that can be read and not
// written, and is the reading finding W5 is about.
//
// It is deliberately narrower than storage.ProbeWriteAccess's own verdict. Two
// of that probe's blockers are already described, better, by the readings above
// it: BlockerDirectory is the runtime root, which RuntimePathCheck names along
// with the MINDRAIL_RUNTIME_DIR override, and BlockerObstruction is the occupied
// path DBPathErr describes in full. Letting either of them speak here would put
// a second, worse-worded remedy for one condition into the same document — the
// defect this package exists to prevent — so only the two conditions nothing
// else in the report can see are claimed: the database file's own mode, and the
// `-shm` index beside it whose mode refuses every write while the database
// file's own mode looks perfectly correct.
//
// The third condition it does claim is space, and it is the one this reading
// was still missing after W5 closed. ProbeWriteAccess used to answer entirely
// from access(2), and mode bits are perfectly correct on a filesystem with zero
// bytes free: with the database held open by another process — so its `-shm`
// already existed and needed no resizing — `doctor` opened it, read WAL and
// foreign keys back, printed `✓ Runtime database healthy` beside
// `db_writable: false` in the same block, exited 0 and left `status` reporting
// READY, while `mindrail init` in the same repository failed with
// RUNTIME_PATH_UNWRITABLE and exit 4 (finding D1). One line of one check
// contradicted the line above it, and the report as a whole contradicted the
// only command that writes. "Permitted" is not "possible", and a blocker no
// chmod can clear must not be dropped on the floor by the check whose whole
// job is to predict whether a write would land.
//
// Narrowing detection is also what keeps it from over-firing. The cache
// directory, the repository config directory and every other path Mindrail
// touches are outside the footprint this probe inspects; a pass that let one of
// those decide the runtime database's verdict bricked a working repository
// once already (finding F13).
func (s Subject) unwritableDatabase() (diagnosis, bool) {
	if !s.Probes.DBWriteKnown || s.Probes.DBWrite.Writable {
		return diagnosis{}, false
	}

	// A database that could not be opened, or that opened and failed its
	// integrity check, is already described by a stronger reading. A file at mode
	// 0000 refuses reads as well as writes, and answering "restore write
	// permission" there replaces "this is not a readable SQLite database" with a
	// remedy that clears half the condition. W5 is about the database that opens
	// perfectly and cannot be written; where the open itself failed, the driver's
	// own diagnosis is the better one and this stays quiet.
	if s.DBErr != nil || s.IntegrityErr != nil {
		return diagnosis{}, false
	}
	switch s.Probes.DBWrite.Blocker {
	case storage.BlockerDatabaseFile, storage.BlockerSidecar, storage.BlockerNoSpace:
	default:
		return diagnosis{}, false
	}

	// The remedy comes from the layer that established the refusal, so `doctor`,
	// `status` and the `mindrail init` that fails here all print the same
	// sentence naming the same files. It is carried out and verified rather than
	// asserted: restoring write permission on those paths, or freeing space on
	// the filesystem under them, is what clears this condition.
	//
	// The fallback below is only reached by a refusal that arrived without a
	// payload, and even there it may not hand a full disk a chmod: that dead-end
	// remedy is the whole reason space is a blocker of its own rather than a
	// fourth spelling of "unwritable".
	d := explain(s.Probes.DBWrite.Err, diagnosis{
		code:   app.CodeRuntimePathUnwritable,
		impact: "Mindrail can read the state already recorded here but cannot record anything new, so `mindrail init` and every other command that writes will fail.",
		next:   []string{fallbackWriteRemedy(s.Probes.DBWrite)},
	})

	// A repository whose database is unwritable *and* not yet established needs
	// both steps, and a remedy that stops at the chmod would leave the reader one
	// silent command short of a working install. The second action is added only
	// where init still has work to do, so a fully registered repository is not
	// told to run a command it does not need.
	if !s.DBPresent {
		d.next = append(slices.Clone(d.next), initCommand)
	}
	return d, true
}

// fallbackWriteRemedy is the remedy for a write refusal that carried no payload
// of its own. It exists so that the fallback is chosen from the blocker rather
// than written once for the common case: a chmod prescribed for a full disk is
// the dead end this classification was split apart to remove, and a fallback is
// still a sentence somebody reads.
func fallbackWriteRemedy(access storage.WriteAccess) string {
	if access.Blocker == storage.BlockerNoSpace {
		return "free space on the filesystem holding " + access.Blocked
	}
	return "restore write permission on " + access.Blocked
}

// unwritableDatabaseSummary names the condition in the one line a human reads
// first.
//
// A full disk and a refused mode bit are one state — no write lands — with
// nothing in common to do about them, and the summary is where a reader decides
// whether to reach for chmod or for df. "Runtime database is not writable" over
// a filesystem with zero bytes free is true and points at the wrong tool.
func unwritableDatabaseSummary(blocker storage.WriteBlocker) string {
	if blocker == storage.BlockerNoSpace {
		return "Runtime database cannot be written: the filesystem is full"
	}
	return "Runtime database is not writable"
}

// supersede replaces a remedy the reader cannot carry out with the obstruction
// that stops them.
//
// The runtime store is described by three checks, and each learns about a
// failure from a different layer: the SQLite driver's refusal, the migrator's,
// the workspace query's. None of those layers can see that the directory holding
// the database is unwritable or that its path is occupied — doctor's own
// read-only probe establishes that, and it is the same probe the document's
// top-level error object is built from. Without this, one rendered document
// carried two remedies for one condition: the error object said "make the
// directory writable" while the component printed beside it said "run
// `mindrail init`", the command that fails there identically every time
// (finding H11).
//
// Two guards keep it from firing where it is not wanted. It touches only a
// reading that actually offers `mindrail init`, so a remedy that already names
// the real blockage is left exactly as the layer that wrote it intended. And the
// obstruction it defers to is the runtime *root* and the database path, never
// the cache directory — a cache Mindrail cannot write costs nothing, and grading
// it like the runtime root once drove a fully working repository to BLOCKED and
// exit 4 (finding F13).
func (s Subject) supersede(result Result) Result {
	if !offersInit(result.NextAction) {
		return result
	}

	obstruction, blocked := s.obstruction()
	if !blocked {
		// The runtime store is sound and `mindrail init` would still fail, because
		// the directory init has to write the repository scaffolding into refuses
		// it. Nothing in the runtime store's own three checks can see that, and
		// before this the whole document — every check OK, exit 0 — recommended
		// the command that could not succeed (finding D4).
		obstruction, blocked = s.repoConfigObstruction()
	}
	if !blocked {
		return result
	}

	superseded := failure(result.State, result.Summary, obstruction)
	superseded.Name = result.Name
	superseded.Section = result.Section
	superseded.Details = result.Details
	return superseded
}

// initAfterObstruction is the step that gets a reader from "the thing in the way
// is gone" back to a working installation. It is a sentence rather than the bare
// command because it is appended to a remedy that already has steps in it, and a
// list whose last line is two words reads as an afterthought.
const initAfterObstruction = "Then run `" + initCommand + "`."

// completeRootRemedy finishes the runtime-root remedy when the blockage is one
// `mindrail init` can follow.
//
// It closes finding W6's half of the disagreement. A regular file standing where
// the runtime root should be is described twice in one document by two layers:
// the filesystem probe, which says to remove it or point MINDRAIL_RUNTIME_DIR
// elsewhere, and the runtime-store probe, which says to remove it and then run
// `mindrail init`. Both are true and only the second is complete — removing the
// file leaves an uninitialised repository — but the first is the one the error
// object is built from, so the document's authority stopped a step short of the
// component printed beside it. The invariant that catches that class reads the
// error object as authoritative, which made the incomplete remedy the one
// everything else had to be cut down to; completing it is the fix that leaves
// both readings able to succeed.
//
// It fires only where the same document already says init is worth running. A
// runtime root whose *mode bits* refuse writes yields an obstruction whose remedy
// is a chmod and no init, so that reading is left exactly as the filesystem layer
// wrote it.
func (s Subject) completeRootRemedy(d diagnosis) diagnosis {
	if offersInit(d.next) {
		return d
	}
	obstruction, blocked := s.obstruction()
	if !blocked || !offersInit(obstruction.next) {
		return d
	}

	d.next = append(slices.Clone(d.next), initAfterObstruction)
	return d
}

// offersInit reports whether any of these actions sends the reader to `mindrail
// init`.
//
// The match is on a mention rather than on the whole action. "Move the file
// aside and run `mindrail init`" is a good remedy when the only thing in the way
// is that file — and it is still the wrong one when the directory holding it
// cannot be written, because the second half fails whatever the reader does
// about the first.
func offersInit(actions []string) bool {
	for _, action := range actions {
		if strings.Contains(action, initCommand) {
			return true
		}
	}
	return false
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
