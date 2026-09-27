package status

import (
	"context"
	"fmt"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/index"
)

// RepositoryInfo is the Git layout status reports. CommonDir and WorktreeRoot
// are separate fields because they are separate answers: every linked worktree
// of a repository shares the first and owns the second, and acceptance
// criterion 3 is about reporting both correctly.
//
// Observation qualifies every other field, for the same reason it does in
// RuntimeInfo and for the block acceptance criterion 3 is actually about. When
// discovery fails, `init` still renders a whole report, and the three path
// fields are the empty string while IsLinkedWorktree is false — three unasked
// questions and one flat assertion that this is not a linked worktree, about a
// repository nobody could reach. It was the one block that gained no marker
// (finding H12).
type RepositoryInfo struct {
	Observation      Observation `json:"observation"`
	CommonDir        string      `json:"common_dir"`
	WorktreeRoot     string      `json:"worktree_root"`
	GitDir           string      `json:"git_dir"`
	IsLinkedWorktree bool        `json:"is_linked_worktree"`
	GitVersion       string      `json:"git_version,omitempty"`
}

// Observation says how much a block of the report is worth.
//
// Three values, because two were not enough. A check that never ran and a check
// that ran and could not tell both leave their fields zero, and both used to be
// separated from a real reading by one boolean derived from
// `Code != STARTUP_INCOMPLETE` — so a repository that *has* been initialised but
// whose database will not open published `initialized: false` and
// `schema_version: 0` as inspected facts (finding F15). "Looked and could not
// tell" is its own answer and the report now has a word for it.
//
// It qualifies only the fields that describe the repository. Anything that
// describes this binary — the knowledge schema window — is always known.
type Observation string

const (
	// Observed means the check ran and the values below it are findings.
	Observed Observation = "observed"

	// NotObserved means the tech-stack §87 sequence stopped before the check's
	// inputs were populated. Nobody looked.
	NotObserved Observation = "not_observed"

	// Indeterminate means the check ran, failed, and cannot vouch for the
	// values below it either way.
	Indeterminate Observation = "indeterminate"
)

// Known reports whether the values this observation qualifies are facts.
func (o Observation) Known() bool { return o == Observed }

// RuntimeInfo describes the machine-local state store. Initialized is the
// answer to "has mindrail init run here?", and it is reported rather than acted
// on (decision D-01).
//
// Observation qualifies every other field. Startup is a sequence, and when it
// aborts before the runtime store is opened — or reaches it and cannot open the
// database — these fields are still zero; a report that omitted the distinction
// published `initialized: false` as a fact about a repository it could not read.
type RuntimeInfo struct {
	Observation   Observation `json:"observation"`
	DBPath        string      `json:"db_path"`
	CacheDir      string      `json:"cache_dir"`
	SchemaVersion int64       `json:"schema_version"`
	JournalMode   string      `json:"journal_mode,omitempty"`
	Initialized   bool        `json:"initialized"`
}

// KnowledgeInfo summarises the repository-owned knowledge store, including the
// schema window this binary enforced, so a reader can tell "no records" from
// "records this binary refused to read".
//
// Observation qualifies Present, Decisions, Invariants, Problems and Findings,
// which describe the repository. WriteSchemaVersion and ReadableSchemaVersions
// do not: they describe this binary and are always populated, however far
// startup got (kernel-scope §3).
type KnowledgeInfo struct {
	Observation Observation `json:"observation"`
	Present     bool        `json:"present"`
	Decisions   int         `json:"decisions"`
	Invariants  int         `json:"invariants"`
	// Problems counts the records this binary could not read; Findings counts
	// the ones it read and found wrong. They are two counts rather than one
	// because their remedies are opposite — one is fixed by upgrading Mindrail,
	// the other by editing a file the repository owns (decision D-38) — and a
	// single total would tell a reader neither.
	//
	// Neither carries omitempty. A store with nothing wrong publishes both as
	// zero, because a count a consumer has to infer from an absent key is a
	// count it cannot tell from a report that never took it.
	Problems               int   `json:"problems"`
	Findings               int   `json:"findings"`
	WriteSchemaVersion     int   `json:"write_schema_version"`
	ReadableSchemaVersions []int `json:"readable_schema_versions"`
}

// WorkspaceInfo reports the opaque identity of this worktree. The ids are the
// identity; the paths in RepositoryInfo are only where it happens to live
// (decision D-26). Observation qualifies Registered for the same reason it does
// in RuntimeInfo.
type WorkspaceInfo struct {
	Observation Observation `json:"observation"`
	Registered  bool        `json:"registered"`
	ID          string      `json:"workspace_id,omitempty"`
	ProjectID   string      `json:"project_id,omitempty"`
}

// CoordinationInfo is MR-003's block: what work is in flight and what the last
// agent said about it.
//
// LeasesActive is MR-004's addition (design §11): the leases the project
// holds right now, judged against the clock and marking nothing (D-79).
//
// It is additive beside Knowledge and Workspace rather than a seventh component,
// and it cannot move Readiness (decision D-62). A blocked task is a fact about
// work, not about the installation; a tool that reported BLOCKED — the value
// reserved for "this repository cannot be verified" — because an agent parked a
// task would be unusable in exactly the situation the task was parked for.
//
// The three counts are the states a reader can act on. COMPLETED and ABANDONED
// are deliberately absent: they only grow, so a number that never goes down
// would say nothing about the repository now and would make the block look
// busier every week.
type CoordinationInfo struct {
	Observation     Observation                 `json:"observation"`
	TasksOpen       int                         `json:"tasks_open"`
	TasksInProgress int                         `json:"tasks_in_progress"`
	TasksBlocked    int                         `json:"tasks_blocked"`
	LeasesActive    int                         `json:"leases_active"`
	LastCheckpoint  *coordination.CheckpointRef `json:"last_checkpoint"`
}

// Report is one `mindrail status` answer.
//
// StoppedAtStep is the one field that says the rest of the report is partial:
// when the tech-stack §87 sequence aborts, the subsystems after that step were
// never inspected, and a consumer has to be able to tell a report of findings
// from a report of missing observations without parsing prose.
type Report struct {
	Command           string                      `json:"command"`
	Readiness         Readiness                   `json:"readiness"`
	StoppedAtStep     string                      `json:"stopped_at_step,omitempty"`
	BlockingComponent ComponentName               `json:"blocking_component,omitempty"`
	NextAction        []string                    `json:"next_action,omitempty"`
	Components        map[ComponentName]Component `json:"components"`
	Repository        RepositoryInfo              `json:"repository"`
	Runtime           RuntimeInfo                 `json:"runtime"`
	Knowledge         KnowledgeInfo               `json:"knowledge"`
	Workspace         WorkspaceInfo               `json:"workspace"`
	Coordination      CoordinationInfo            `json:"coordination"`
	DurationMS        int64                       `json:"duration_ms"`
}

// Build derives the readiness report from an already-resolved subject.
//
// The two components MR-001 can actually judge are read through the doctor
// checks rather than re-derived here, so `status` and `doctor` can never
// disagree about whether the runtime store is healthy — the failure mode where
// one command says READY and the other prints an error.
//
// elapsed is passed in rather than measured because the meaningful duration is
// the whole startup sequence, which finished before this function was called
// (decision D-20).
func Build(s doctor.Subject, elapsed time.Duration) Report {
	// The MR-001 checks are pure functions of the subject and never block, so
	// there is nothing for a caller to cancel and Build takes no context.
	ctx := context.Background()

	// The read-only probes doctor is allowed to make, taken once so that status
	// and doctor describe the same disk (decision D-01: probe, never create).
	s = doctor.Probe(s)

	// The git reading is not a component — §107 has no slot for the repository
	// itself — but it is what grades the repository block, and reading it here
	// rather than re-deriving the verdict is what keeps status and doctor from
	// disagreeing about whether the layout was actually resolved.
	repository := doctor.GitCheck(s).Run(ctx)
	knowledge := doctor.KnowledgeCheck(s).Run(ctx)
	runtimePaths := doctor.RuntimePathCheck(s).Run(ctx)
	sqlite := doctor.SQLiteCheck(s).Run(ctx)
	migrations := doctor.MigrationCheck(s).Run(ctx)
	workspace := doctor.WorkspaceCheck(s).Run(ctx)

	components := map[ComponentName]Component{
		ComponentKnowledge: componentFrom(knowledge),
		// The runtime paths reading belongs here: a runtime directory nothing
		// can write stops exactly the same work as a database that will not
		// open, and leaving it out of every component made `status` exit 0 with
		// ok:true on a repository where `mindrail init` cannot succeed.
		//
		// It is listed last so that on a healthy install — where every reading
		// is OK and worst() keeps the first — the component still describes the
		// database it is named after.
		ComponentRuntimeDB: componentFrom(worst(sqlite, migrations, workspace, runtimePaths)),
	}
	for name, summary := range notApplicableComponents {
		// Decision D-02: these four are NOT_APPLICABLE, not degraded. This
		// binary builds no index, so there is no partial progress to report and
		// a healthy install must not sit at PARTIAL_READY forever.
		components[name] = Component{State: doctor.StateNotApplicable, Summary: summary}
	}
	applyInventoryComponents(components, s)

	readiness, blocking, next := classify(components)

	return Report{
		Command:           "status",
		Readiness:         readiness,
		StoppedAtStep:     stoppedAtStep(knowledge, runtimePaths, sqlite, migrations, workspace),
		BlockingComponent: blocking,
		NextAction:        next,
		Components:        components,
		Repository: RepositoryInfo{
			Observation:      repositoryObservation(repository),
			CommonDir:        s.Repo.CommonDir,
			WorktreeRoot:     s.Repo.WorktreeRoot,
			GitDir:           s.Repo.GitDir,
			IsLinkedWorktree: s.Repo.IsLinkedWorktree,
			GitVersion:       s.GitVersion,
		},
		Runtime: RuntimeInfo{
			// The sqlite reading and the migration reading together decide this:
			// a database that opened cleanly but whose ledger could not be read
			// knows `initialized` and not `schema_version`, and the weaker of the
			// two answers is the honest one for the block.
			Observation:   observationOf(sqlite, migrations),
			DBPath:        s.Paths.DBPath,
			CacheDir:      s.Paths.CacheDir,
			SchemaVersion: currentSchemaVersion(s),
			JournalMode:   s.Pragmas.JournalMode,
			Initialized:   s.DBPresent,
		},
		Knowledge: KnowledgeInfo{
			Observation: observationOf(knowledge),
			Present:     s.Knowledge.Present,
			Decisions:   len(s.Knowledge.Decisions),
			Invariants:  len(s.Knowledge.Invariants),
			Problems:    len(s.Knowledge.Problems),
			// Read from the subject, never recomputed here (decisions D-41,
			// D-42). Building a validator in this function would make `status`
			// and `doctor` two places that judge one store, and the day they
			// disagreed the report would carry both answers.
			Findings: len(s.KnowledgeFindings),
			// From the binary, never from the subject: these two are what this
			// binary can do, and a startup that stopped early does not change it.
			WriteSchemaVersion:     doctor.KnowledgeWriteSchemaVersion(s.Knowledge),
			ReadableSchemaVersions: doctor.KnowledgeReadableSchemaVersions(s.Knowledge),
		},
		Workspace: WorkspaceInfo{
			Observation: observationOf(workspace),
			Registered:  s.Workspace.ID != "",
			ID:          s.Workspace.ID,
			ProjectID:   s.Workspace.ProjectID,
		},
		Coordination: coordinationInfo(s),
		DurationMS:   elapsed.Milliseconds(),
	}
}

// applyInventoryComponents projects bootstrap's persisted inventory reading
// into readiness. Build deliberately consumes only Subject fields: status
// never walks the repository, so it cannot race a changing worktree or turn a
// report into filesystem work. The census beside the inventory is the same
// kind of reading — one SQL aggregate, no hashing — which is why the counts
// are honest on the read path while the cold index is still running.
func applyInventoryComponents(components map[ComponentName]Component, s doctor.Subject) {
	switch {
	case s.InventoryErr != nil:
		components[ComponentInventory] = Component{
			State:   doctor.StateDegraded,
			Summary: "Project unit inventory could not be read.",
		}
		components[ComponentSyntax] = Component{
			State:   doctor.StateDegraded,
			Phase:   "INVENTORY",
			Summary: "Syntax index is waiting for a readable inventory.",
		}
	case s.InventoryObserved:
		units := len(s.Inventory)
		components[ComponentInventory] = Component{
			State:   doctor.StateOK,
			Summary: fmt.Sprintf("%d project units discovered", units),
			Units:   intPtr(units),
		}
		applySyntaxComponent(components, s)
	}
}

// applySyntaxComponent derives the syntax slot from the persisted file-state
// census. Phase carries progress (INVENTORY before the first row, INDEXING
// while pending work remains, READY when none does); a failed row degrades
// the component per spec-1.0 §108 while staying pending work for the
// scheduler. An unreadable census degrades too: the read ran and failed, which
// is a different answer from a read that never happened.
func applySyntaxComponent(components map[ComponentName]Component, s doctor.Subject) {
	if s.IndexErr != nil {
		components[ComponentSyntax] = Component{
			State:      doctor.StateDegraded,
			Summary:    "Syntax index state could not be read.",
			Code:       app.CodeIndexStateCorrupt,
			NextAction: []string{"Move the runtime database aside and run `mindrail init` to rebuild the index state."},
		}
		return
	}
	if !s.IndexObserved {
		components[ComponentSyntax] = Component{
			State:   doctor.StateOK,
			Phase:   "INVENTORY",
			Summary: "Syntax index is at INVENTORY; no files have been indexed yet.",
		}
		return
	}
	pending := s.IndexCounts[index.StatePending]
	failed := s.IndexCounts[index.StateFailed]
	indexed := s.IndexCounts[index.StateIndexed]
	switch {
	case failed > 0:
		components[ComponentSyntax] = Component{
			State:      doctor.StateDegraded,
			Phase:      "INDEXING",
			Summary:    fmt.Sprintf("Syntax index is DEGRADED: %d files failed to parse, %d pending.", failed, pending),
			Code:       app.CodeSyntaxParseFailed,
			NextAction: []string{"Fix the syntax errors in the failing source files, then trigger a re-index."},
			Pending:    intPtr(pending),
			Failed:     intPtr(failed),
		}
	case pending > 0:
		components[ComponentSyntax] = Component{
			State:   doctor.StateOK,
			Phase:   "INDEXING",
			Summary: fmt.Sprintf("Syntax index is INDEXING: %d files pending.", pending),
			Pending: intPtr(pending),
			Failed:  intPtr(failed),
		}
	case indexed > 0:
		components[ComponentSyntax] = Component{
			State:   doctor.StateOK,
			Phase:   "READY",
			Summary: fmt.Sprintf("Syntax index is READY: %d files indexed.", indexed),
			Pending: intPtr(pending),
			Failed:  intPtr(failed),
		}
	default:
		components[ComponentSyntax] = Component{
			State:   doctor.StateOK,
			Phase:   "INVENTORY",
			Summary: "Syntax index is at INVENTORY; no files have been indexed yet.",
			Pending: intPtr(pending),
			Failed:  intPtr(failed),
		}
	}
}

// coordinationInfo grades the coordination block.
//
// There is no doctor check behind it, so the observation comes from what
// bootstrap left behind rather than from a reading's code. Three answers, not
// two: the summary was read, the sequence never got that far, or the read ran
// and failed. All three leave the counts at zero, which is precisely why the
// observation has to be published.
//
// The third was missing. Every failure was folded into the same false flag, so
// a damaged `created_at` — a row the database holds and cannot answer for —
// was published as "not observed: startup stopped before this subsystem was
// read", about a startup that completed and a query that ran (finding F46). The
// adjacent Runtime block has had `indeterminate` since MR-001 and means exactly
// this: the subsystem was read and could not answer.
func coordinationInfo(s doctor.Subject) CoordinationInfo {
	if s.CoordinationErr != nil {
		return CoordinationInfo{Observation: Indeterminate}
	}
	if !s.CoordinationObserved {
		return CoordinationInfo{Observation: NotObserved}
	}
	return CoordinationInfo{
		Observation:     Observed,
		TasksOpen:       s.Coordination.Open,
		TasksInProgress: s.Coordination.InProgress,
		TasksBlocked:    s.Coordination.Blocked,
		LeasesActive:    s.Coordination.LeasesActive,
		LastCheckpoint:  s.Coordination.LastCheckpoint,
	}
}

// repositoryObservation grades the Git layout block.
//
// It is deliberately not observationOf(git). That function reads UNAVAILABLE as
// a real finding, which is right for a database path that was inspected and
// found empty, and wrong here: a git that is missing or that timed out reports
// UNAVAILABLE (decision D-30) and leaves every layout field at zero, so
// "is_linked_worktree: false" would be published as an inspected fact about a
// repository nobody reached.
//
// The block is never NotObserved. Discovery is step 1 of the §87 sequence, so it
// always runs; every field of the block comes from that one reading, and a
// reading that is anything but OK leaves all of them at zero. That makes
// Indeterminate — "looked, and cannot vouch for this" — the honest answer for
// every failure, including a bare repository, where the worktree root really is
// unknown rather than absent.
func repositoryObservation(repository doctor.Result) Observation {
	if repository.State == doctor.StateOK {
		return Observed
	}
	return Indeterminate
}

// observationOf grades the readings behind one block of the report, worst
// answer wins.
//
// STARTUP_INCOMPLETE is doctor's code for a reading whose inputs the sequence
// never populated: nobody looked. An ERROR reading is the third case — the
// check ran, the subsystem refused it, and the zero values left behind are not
// findings. Publishing them as findings is how `status` came to report
// `initialized: false` about a repository that had been initialised and whose
// database merely could not be opened (finding F15).
//
// Everything else is a real reading, including UNAVAILABLE: "the database path
// was inspected and is empty" is an observation, and `initialized: false` is
// exactly what it observed.
//
// NOT_OBSERVED is reserved for a block where nothing was read at all. A block
// backed by several readings — the runtime store is described by both the
// database and the ledger — is INDETERMINATE as soon as one of them ran, even
// if the other never did: "nobody looked" would be false, and it is the
// stronger of the two claims.
func observationOf(results ...doctor.Result) Observation {
	looked, known := false, true
	for _, result := range results {
		// Two ways a reading can be no reading: the startup sequence never
		// populated its inputs, or the check named a condition it inferred
		// rather than a value it looked up. Both are "nobody looked".
		if result.Code == app.CodeStartupIncomplete || result.Metadata[doctor.MetadataNothingRead] == "true" {
			known = false
			continue
		}
		looked = true
		if result.State == doctor.StateError {
			known = false
		}
	}

	switch {
	case !looked:
		return NotObserved
	case !known:
		return Indeterminate
	default:
		return Observed
	}
}

// stoppedAtStep recovers the §87 step that aborted the run from whichever
// reading was blocked by it. It is read back out of the check metadata rather
// than re-derived so that status can never name a different step from doctor.
func stoppedAtStep(results ...doctor.Result) string {
	for _, result := range results {
		if step := result.Metadata["stopped_at_step"]; step != "" {
			return step
		}
	}
	return ""
}

// notApplicableComponents are the four slots whose subsystems MR-005 owns. The
// summary names the milestone so a reader is told "not built yet", not left to
// guess whether their project simply does not use the capability.
var notApplicableComponents = map[ComponentName]string{
	ComponentInventory:   "File inventory is not built by this version.",
	ComponentSyntax:      "Syntax index is not built by this version.",
	ComponentSemantic:    "Semantic index is not built by this version.",
	ComponentCoverageMap: "Coverage map is not built by this version.",
}

// componentFrom projects a doctor reading onto a component slot. Diagnostic and
// impact stay in the doctor report: status answers "can I work here and what do
// I do about it", and the full explanation is one `mindrail doctor` away.
func componentFrom(result doctor.Result) Component {
	return Component{
		State:      result.State,
		Summary:    result.Summary,
		Code:       result.Code,
		NextAction: result.NextAction,
	}
}

// worst returns the most severe of several readings. One component slot can be
// backed by several checks, and the reader has to be told about the worst of
// them rather than the first.
func worst(results ...doctor.Result) doctor.Result {
	selected := results[0]
	for _, result := range results[1:] {
		if stateRank(result.State) > stateRank(selected.State) {
			selected = result
		}
	}
	return selected
}

// stateRank mirrors doctor's own severity order. It is duplicated rather than
// exported from doctor because it ranks components here, not checks, and a
// shared knob would tie two independent orderings together.
func stateRank(s doctor.State) int {
	switch s {
	case doctor.StateError:
		return 4
	case doctor.StateUnavailable:
		return 3
	case doctor.StateDegraded:
		return 2
	case doctor.StateOK:
		return 1
	default:
		return 0
	}
}

// classify derives the overall verdict.
//
// Blocking is decided before degradation so the report names the thing that
// stops work rather than the first thing that is merely imperfect, and the
// remedy is copied from that same component so the top-level next_action can
// never contradict the component the report is pointing at.
//
// A component that was inspected wins over one that was not, whatever the
// reporting order says. Otherwise a run that stopped at the runtime store would
// name `knowledge` — the first slot in §107's list, and the one subsystem
// nobody looked at — as the reason work cannot start.
//
// When nothing was inspected at all, no component is named. Readiness is still
// BLOCKED — work cannot start on an installation this report cannot vouch for —
// but blocking_component stays empty, because every candidate for it is a
// subsystem that was never read, and picking the first one asserted that
// `knowledge` stopped a run the error object in the same document attributes to
// the `git` check (finding H15). stopped_at_step carries what is actually known.
func classify(components map[ComponentName]Component) (Readiness, ComponentName, []string) {
	for _, name := range componentOrder {
		if component := components[name]; component.blocks() && component.inspected() {
			return ReadinessBlocked, name, component.NextAction
		}
	}
	for _, name := range componentOrder {
		if component := components[name]; component.blocks() {
			return ReadinessBlocked, "", component.NextAction
		}
	}
	for _, name := range componentOrder {
		if component := components[name]; component.State == doctor.StateDegraded {
			return ReadinessDegraded, "", component.NextAction
		}
	}
	for _, name := range []ComponentName{ComponentInventory, ComponentSyntax} {
		if phase := components[name].Phase; phase != "" && phase != "READY" {
			return ReadinessPartialReady, "", nil
		}
	}
	return ReadinessReady, "", nil
}

// currentSchemaVersion reports the highest applied migration, or zero for a
// database no migration has ever run against.
func currentSchemaVersion(s doctor.Subject) int64 {
	current := int64(0)
	for _, applied := range s.Migrations {
		if applied.Version > current {
			current = applied.Version
		}
	}
	return current
}
