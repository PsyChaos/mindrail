package status

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/workspace"
)

// TestBuildReadyOnHealthySubject pins decision D-02. With no index, the four
// index components are NOT_APPLICABLE and a healthy install reports READY --
// not a permanent PARTIAL_READY, which would teach every user to ignore the
// field before MR-005 gives it meaning.
func TestBuildReadyOnHealthySubject(t *testing.T) {
	report := Build(healthySubject(), 7*time.Millisecond)

	if report.Command != "status" {
		t.Errorf("command = %q, want %q", report.Command, "status")
	}
	if report.Readiness != ReadinessReady {
		t.Fatalf("readiness = %q, want %q (%+v)", report.Readiness, ReadinessReady, report.Components)
	}
	if report.BlockingComponent != "" {
		t.Errorf("blocking_component = %q, want none", report.BlockingComponent)
	}
	if len(report.NextAction) != 0 {
		t.Errorf("next_action = %v, want none", report.NextAction)
	}
	if report.DurationMS != 7 {
		t.Errorf("duration_ms = %d, want 7", report.DurationMS)
	}

	wantStates := map[ComponentName]doctor.State{
		ComponentKnowledge:   doctor.StateOK,
		ComponentRuntimeDB:   doctor.StateOK,
		ComponentInventory:   doctor.StateNotApplicable,
		ComponentSyntax:      doctor.StateNotApplicable,
		ComponentSemantic:    doctor.StateNotApplicable,
		ComponentCoverageMap: doctor.StateNotApplicable,
	}
	for name, want := range wantStates {
		if got := report.Components[name].State; got != want {
			t.Errorf("component %q = %q, want %q", name, got, want)
		}
	}

	if report.Repository.CommonDir != "/repo/.git" {
		t.Errorf("repository.common_dir = %q", report.Repository.CommonDir)
	}
	if report.Repository.WorktreeRoot != "/repo" {
		t.Errorf("repository.worktree_root = %q", report.Repository.WorktreeRoot)
	}
	if !report.Runtime.Initialized {
		t.Error("runtime.initialized = false on a healthy subject")
	}
	if report.Runtime.SchemaVersion != 1 {
		t.Errorf("runtime.schema_version = %d, want 1", report.Runtime.SchemaVersion)
	}
	if report.Runtime.JournalMode != "wal" {
		t.Errorf("runtime.journal_mode = %q, want %q", report.Runtime.JournalMode, "wal")
	}
	if !report.Knowledge.Present || report.Knowledge.Decisions != 2 || report.Knowledge.Invariants != 1 {
		t.Errorf("knowledge = %+v", report.Knowledge)
	}
	if !report.Workspace.Registered || report.Workspace.ID == "" || report.Workspace.ProjectID == "" {
		t.Errorf("workspace = %+v", report.Workspace)
	}
}

func TestBuildReportsInventoryPhaseWithoutRescanning(t *testing.T) {
	subject := healthySubject()
	subject.InventoryObserved = true

	report := Build(subject, time.Millisecond)
	if report.Readiness != ReadinessPartialReady {
		t.Fatalf("readiness = %q, want %q when syntax has only reached inventory", report.Readiness, ReadinessPartialReady)
	}
	inventory := report.Components[ComponentInventory]
	if inventory.State != doctor.StateOK || inventory.Summary != "0 project units discovered" {
		t.Errorf("inventory component = %+v, want observed zero-unit discovery", inventory)
	}
	syntax := report.Components[ComponentSyntax]
	if syntax.State != doctor.StateOK || syntax.Phase != "INVENTORY" {
		t.Errorf("syntax component = %+v, want the explicit INVENTORY phase", syntax)
	}
}

func indexedSubject() doctor.Subject {
	subject := healthySubject()
	subject.InventoryObserved = true
	subject.Inventory = []index.ProjectUnit{{ID: "UNT-01", Path: "/repo/pkg", Kind: index.UnitPython}}
	return subject
}

func assertComponentRemedy(t *testing.T, name ComponentName, component Component) {
	t.Helper()
	if component.Code == "" {
		t.Errorf("component %q reported %q with no code", name, component.State)
	}
	if component.Code != "" && !app.IsRegistered(component.Code) {
		t.Errorf("component %q reported unregistered code %q", name, component.Code)
	}
	if len(component.NextAction) == 0 {
		t.Errorf("component %q reported %q with no next_action", name, component.State)
	}
	for i, action := range component.NextAction {
		assertActionable(t, string(name), i, action)
	}
}

// TestBuildReportsIndexingPartialReady is the task list's fifth acceptance
// criterion: a running cold index is explicit pending work with a count, not a
// silent gap and not a blocker.
func TestBuildReportsIndexingPartialReady(t *testing.T) {
	subject := indexedSubject()
	subject.IndexObserved = true
	subject.IndexCounts = map[index.FileState]int{index.StatePending: 3, index.StateIndexed: 5}

	report := Build(subject, time.Millisecond)
	if report.Readiness != ReadinessPartialReady {
		t.Fatalf("readiness = %q, want %q while files are pending", report.Readiness, ReadinessPartialReady)
	}
	syntax := report.Components[ComponentSyntax]
	if syntax.State != doctor.StateOK || syntax.Phase != "INDEXING" {
		t.Errorf("syntax component = %+v, want OK INDEXING", syntax)
	}
	if syntax.Pending == nil || *syntax.Pending != 3 {
		t.Errorf("syntax pending = %v, want 3", syntax.Pending)
	}
	if syntax.Failed == nil || *syntax.Failed != 0 {
		t.Errorf("syntax failed = %v, want explicit 0", syntax.Failed)
	}
	inventory := report.Components[ComponentInventory]
	if inventory.Units == nil || *inventory.Units != 1 {
		t.Errorf("inventory units = %v, want 1", inventory.Units)
	}
}

// TestSyntaxComponentCarriesNoTimingKeys is REQ-12 at the status surface:
// the census adds counts, never timings. The golden key list pins this
// globally; this test names the reason beside the wire shape.
func TestSyntaxComponentCarriesNoTimingKeys(t *testing.T) {
	subject := indexedSubject()
	subject.IndexObserved = true
	subject.IndexCounts = map[index.FileState]int{index.StatePending: 2, index.StateIndexed: 4}

	encoded, err := json.Marshal(Build(subject, time.Millisecond).Components[ComponentSyntax])
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatal(err)
	}
	for key := range keys {
		switch key {
		case "state", "phase", "summary", "code", "next_action", "pending", "failed":
		default:
			t.Errorf("syntax component carries unexpected key %q", key)
		}
	}
	for _, timing := range []string{"timing", "parse", "extract", "duration", "waited", "held"} {
		if strings.Contains(string(encoded), `"`+timing) {
			t.Errorf("syntax component carries timing key %q: %s", timing, encoded)
		}
	}
}

// TestBuildInventoryPhaseWithEmptyCensus pins the observed-but-empty census:
// units discovered, no file rows yet — still INVENTORY, with explicit zero
// counts rather than absent keys.
func TestBuildInventoryPhaseWithEmptyCensus(t *testing.T) {
	subject := indexedSubject()
	subject.IndexObserved = true
	subject.IndexCounts = map[index.FileState]int{}

	report := Build(subject, time.Millisecond)
	if report.Readiness != ReadinessPartialReady {
		t.Fatalf("readiness = %q, want %q before the first file row", report.Readiness, ReadinessPartialReady)
	}
	syntax := report.Components[ComponentSyntax]
	if syntax.State != doctor.StateOK || syntax.Phase != "INVENTORY" {
		t.Errorf("syntax component = %+v, want OK INVENTORY", syntax)
	}
	if syntax.Pending == nil || *syntax.Pending != 0 || syntax.Failed == nil || *syntax.Failed != 0 {
		t.Errorf("syntax counts = %v/%v, want explicit zeros", syntax.Pending, syntax.Failed)
	}
}

// TestBuildReadyWhenIndexDrained proves READY returns once the cold remainder
// is gone: PARTIAL_READY must not stick after the work finishes.
func TestBuildReadyWhenIndexDrained(t *testing.T) {
	subject := indexedSubject()
	subject.IndexObserved = true
	subject.IndexCounts = map[index.FileState]int{index.StateIndexed: 8}

	report := Build(subject, time.Millisecond)
	if report.Readiness != ReadinessReady {
		t.Fatalf("readiness = %q, want %q when nothing is pending", report.Readiness, ReadinessReady)
	}
	if phase := report.Components[ComponentSyntax].Phase; phase != "READY" {
		t.Errorf("syntax phase = %q, want %q", phase, "READY")
	}
}

// TestBuildDegradedOnFailedIndexRows is spec-1.0 §108: any failed row degrades
// syntax health while the row stays pending work for the scheduler.
func TestBuildDegradedOnFailedIndexRows(t *testing.T) {
	subject := indexedSubject()
	subject.IndexObserved = true
	subject.IndexCounts = map[index.FileState]int{index.StateFailed: 1, index.StatePending: 2}

	report := Build(subject, time.Millisecond)
	if report.Readiness != ReadinessDegraded {
		t.Fatalf("readiness = %q, want %q with failed index rows", report.Readiness, ReadinessDegraded)
	}
	if report.BlockingComponent != "" {
		t.Errorf("blocking_component = %q, want none: DEGRADED still works", report.BlockingComponent)
	}
	syntax := report.Components[ComponentSyntax]
	if syntax.State != doctor.StateDegraded || syntax.Phase != "INDEXING" {
		t.Errorf("syntax component = %+v, want DEGRADED INDEXING", syntax)
	}
	if syntax.Code != app.CodeSyntaxParseFailed {
		t.Errorf("syntax code = %q, want %q", syntax.Code, app.CodeSyntaxParseFailed)
	}
	if syntax.Pending == nil || *syntax.Pending != 2 || syntax.Failed == nil || *syntax.Failed != 1 {
		t.Errorf("syntax counts = %v/%v, want pending 2 failed 1", syntax.Pending, syntax.Failed)
	}
	assertComponentRemedy(t, ComponentSyntax, syntax)
}

// TestBuildDegradedOnUnreadableIndexCensus keeps "looked and failed" apart
// from "nobody looked": a census that ran and failed degrades the component
// instead of publishing uninspected zeros as findings.
func TestBuildDegradedOnUnreadableIndexCensus(t *testing.T) {
	subject := indexedSubject()
	subject.IndexErr = errors.New("index census unavailable")

	report := Build(subject, time.Millisecond)
	if report.Readiness != ReadinessDegraded {
		t.Fatalf("readiness = %q, want %q with an unreadable census", report.Readiness, ReadinessDegraded)
	}
	syntax := report.Components[ComponentSyntax]
	if syntax.State != doctor.StateDegraded || syntax.Code != app.CodeIndexStateCorrupt {
		t.Errorf("syntax component = %+v, want DEGRADED INDEX_STATE_CORRUPT", syntax)
	}
	assertComponentRemedy(t, ComponentSyntax, syntax)
}

// TestBuildBlockedOnUnopenableDB is acceptance criterion 4 for the runtime
// store: a database that cannot be opened has to name itself as the blocker and
// say what to do, not merely report an unhappy overall state.
func TestBuildBlockedOnUnopenableDB(t *testing.T) {
	subject := healthySubject()
	subject.DBErr = app.NewError(
		app.CodeRuntimeDBUnavailable,
		app.KindUnavailable,
		"database is locked",
		"Nothing that reads or writes runtime state can run.",
		"Retry once the other Mindrail process has finished.",
	).WithCause(storage.ErrOpenFailed)

	report := Build(subject, time.Millisecond)

	if report.Readiness != ReadinessBlocked {
		t.Fatalf("readiness = %q, want %q", report.Readiness, ReadinessBlocked)
	}
	if report.BlockingComponent != ComponentRuntimeDB {
		t.Errorf("blocking_component = %q, want %q", report.BlockingComponent, ComponentRuntimeDB)
	}
	if len(report.NextAction) == 0 {
		t.Error("next_action is empty on a blocked report")
	}
	component := report.Components[ComponentRuntimeDB]
	if component.State != doctor.StateError {
		t.Errorf("runtime_db state = %q, want %q", component.State, doctor.StateError)
	}
	if component.Code != app.CodeRuntimeDBUnavailable {
		t.Errorf("runtime_db code = %q, want %q", component.Code, app.CodeRuntimeDBUnavailable)
	}
	if !slices.Equal(component.NextAction, report.NextAction) {
		t.Errorf("report next_action %v does not come from the blocking component %v", report.NextAction, component.NextAction)
	}
}

// TestBuildBlockedOnUnsupportedKnowledgeSchema pins the fail-closed branch of
// kernel-scope §3: a record this binary cannot read means it sees only part of
// the invariants, and acting on a partial view is worse than refusing to act.
func TestBuildBlockedOnUnsupportedKnowledgeSchema(t *testing.T) {
	subject := healthySubject()
	subject.Knowledge.Problems = []loader.Problem{{
		Path:    ".mindrail/knowledge/invariants/inv-9.json",
		Code:    app.CodeKnowledgeSchemaUnsupported,
		Message: "schema_version 3 is outside the reader window",
		Fatal:   true,
	}}

	report := Build(subject, time.Millisecond)

	if report.Readiness != ReadinessBlocked {
		t.Fatalf("readiness = %q, want %q", report.Readiness, ReadinessBlocked)
	}
	if report.BlockingComponent != ComponentKnowledge {
		t.Errorf("blocking_component = %q, want %q", report.BlockingComponent, ComponentKnowledge)
	}
	component := report.Components[ComponentKnowledge]
	if component.Code != app.CodeKnowledgeSchemaUnsupported {
		t.Errorf("knowledge code = %q, want %q", component.Code, app.CodeKnowledgeSchemaUnsupported)
	}
	if len(component.NextAction) == 0 {
		t.Error("blocking knowledge component carries no next_action")
	}
	if report.Components[ComponentRuntimeDB].State != doctor.StateOK {
		t.Error("an unreadable knowledge record must not implicate the runtime store")
	}
}

// TestBuildBlockedOnUninitializedRuntime pins decision D-01 and D-03: status
// never creates the database, it reports that there is none, and the report
// says exactly how to fix it.
func TestBuildBlockedOnUninitializedRuntime(t *testing.T) {
	report := Build(uninitializedSubject(), time.Millisecond)

	if report.Readiness != ReadinessBlocked {
		t.Fatalf("readiness = %q, want %q", report.Readiness, ReadinessBlocked)
	}
	if report.BlockingComponent != ComponentRuntimeDB {
		t.Fatalf("blocking_component = %q, want %q", report.BlockingComponent, ComponentRuntimeDB)
	}

	component := report.Components[ComponentRuntimeDB]
	if component.Code != app.CodeWorkspaceNotInitialized {
		t.Errorf("runtime_db code = %q, want %q", component.Code, app.CodeWorkspaceNotInitialized)
	}
	if want := []string{"mindrail init"}; !slices.Equal(component.NextAction, want) {
		t.Errorf("runtime_db next_action = %v, want %v", component.NextAction, want)
	}
	if want := []string{"mindrail init"}; !slices.Equal(report.NextAction, want) {
		t.Errorf("report next_action = %v, want %v", report.NextAction, want)
	}
	if report.Runtime.Initialized {
		t.Error("runtime.initialized = true without a database")
	}
	if report.Runtime.SchemaVersion != 0 {
		t.Errorf("runtime.schema_version = %d, want 0", report.Runtime.SchemaVersion)
	}
	if report.Workspace.Registered {
		t.Error("workspace.registered = true without a database")
	}
	if report.Components[ComponentKnowledge].State != doctor.StateOK {
		t.Error("an uninitialized runtime must not implicate the knowledge store")
	}
}

// TestBuildDegradedOnRecoverableProblem proves DEGRADED is reachable and that
// it is not treated as a blocker: the installation still works, so nothing
// names a blocking component.
func TestBuildDegradedOnRecoverableProblem(t *testing.T) {
	subject := healthySubject()
	subject.Knowledge.Problems = []loader.Problem{{
		Path:    ".mindrail/knowledge/decisions/dec-9.json",
		Code:    app.CodeKnowledgeUnreadable,
		Message: "invalid character '}' looking for beginning of object key string",
	}}

	report := Build(subject, time.Millisecond)

	if report.Readiness != ReadinessDegraded {
		t.Fatalf("readiness = %q, want %q", report.Readiness, ReadinessDegraded)
	}
	if report.BlockingComponent != "" {
		t.Errorf("blocking_component = %q, want none: DEGRADED still works", report.BlockingComponent)
	}
	if len(report.NextAction) == 0 {
		t.Error("a degraded report carries no next_action")
	}
}

// buildSubjects is every shape Build has to answer for, including the ones the
// startup sequence produces when it aborts partway. The halted rows are what
// the per-field fixtures could not express, and so what the suite could not see.
func buildSubjects() map[string]doctor.Subject {
	return map[string]doctor.Subject{
		"healthy":             healthySubject(),
		"uninitialized":       uninitializedSubject(),
		"unreadable record":   subjectWithKnowledgeProblem(false),
		"unsupported schema":  subjectWithKnowledgeProblem(true),
		"invalid record":      subjectWithKnowledgeFinding(false),
		"supersede cycle":     subjectWithKnowledgeFinding(true),
		"repository unusable": subjectWithoutRepository(),
		"halted at sqlite":    haltedAtSQLiteSubject(),
	}
}

// TestEveryNonOKComponentCarriesARemedy is acceptance criterion 4 applied to the
// readiness model: whatever drove a component off OK, the report has to say what
// to do about it — with a code, and with a remedy that is an action the reader
// can take and is not the command that produced the report.
//
// The earlier version allowed a component to report a non-OK state with no code
// and accepted any non-blank string as a remedy, which is why a component whose
// only advice was `mindrail status` or a statement of fact would have passed.
func TestEveryNonOKComponentCarriesARemedy(t *testing.T) {
	for name, subject := range buildSubjects() {
		t.Run(name, func(t *testing.T) {
			report := Build(subject, time.Millisecond)

			for _, componentName := range Components() {
				component := report.Components[componentName]
				switch component.State {
				case doctor.StateOK, doctor.StateNotApplicable:
					continue
				}
				if strings.TrimSpace(component.Summary) == "" {
					t.Errorf("component %q reported %q with no summary", componentName, component.State)
				}
				if component.Code == "" {
					t.Errorf("component %q reported %q with no code", componentName, component.State)
				}
				if component.Code != "" && !app.IsRegistered(component.Code) {
					t.Errorf("component %q reported unregistered code %q", componentName, component.Code)
				}
				if len(component.NextAction) == 0 {
					t.Errorf("component %q reported %q with no next_action", componentName, component.State)
				}
				for i, action := range component.NextAction {
					assertActionable(t, string(componentName), i, action)
				}
			}

			if report.Readiness != ReadinessReady && len(report.NextAction) == 0 {
				t.Errorf("readiness %q with no next_action", report.Readiness)
			}
			for i, action := range report.NextAction {
				assertActionable(t, "report", i, action)
			}
			if !report.Readiness.Valid() {
				t.Errorf("readiness %q is outside the enum", report.Readiness)
			}
		})
	}
}

// declarativeOpeners begin a sentence that states a fact rather than asking for
// one. "This binary reads schema 1." is true, useful, and not a next action.
var declarativeOpeners = []string{"This ", "There ", "It ", "Mindrail cannot ", "No "}

// assertActionable is §84's next_action contract for the readiness model: a
// remedy has to be something the reader can do, and it must never be the
// command that produced the report.
func assertActionable(t *testing.T, owner string, index int, action string) {
	t.Helper()

	if strings.TrimSpace(action) == "" {
		t.Errorf("%s next_action[%d] is blank", owner, index)
		return
	}
	if strings.Contains(action, "mindrail status") {
		t.Errorf("%s next_action[%d] = %q, which is the command that produced this report", owner, index, action)
	}
	for _, opener := range declarativeOpeners {
		if strings.HasPrefix(action, opener) {
			t.Errorf("%s next_action[%d] = %q is a statement, not an action", owner, index, action)
		}
	}
}

// TestReportNeverPublishesAnUninspectedSubsystemAsAFact is the status half of
// the systemic finding. When startup aborts, the blocks describing subsystems
// after the halt carry zero values, and the report used to publish them —
// `present: false`, `initialized: false`, `registered: false`,
// `write_schema_version: 0`, `readable_schema_versions: null` — as findings
// about a repository nobody had looked at.
func TestReportNeverPublishesAnUninspectedSubsystemAsAFact(t *testing.T) {
	report := Build(haltedAtSQLiteSubject(), time.Millisecond)

	if report.StoppedAtStep != "open_sqlite" {
		t.Errorf("stopped_at_step = %q, want %q", report.StoppedAtStep, "open_sqlite")
	}
	if report.Knowledge.Observation != NotObserved {
		t.Errorf("knowledge.observation = %q for a store the startup sequence never read, want %q",
			report.Knowledge.Observation, NotObserved)
	}
	if report.Workspace.Observation != NotObserved {
		t.Errorf("workspace.observation = %q for a lookup that never happened, want %q",
			report.Workspace.Observation, NotObserved)
	}

	// The schema window describes this binary, not the repository, so it stays
	// populated however far startup got (kernel-scope §3, finding R5).
	if report.Knowledge.WriteSchemaVersion != schema.WriteVersion {
		t.Errorf("knowledge.write_schema_version = %d, want %d",
			report.Knowledge.WriteSchemaVersion, schema.WriteVersion)
	}
	if !slices.Equal(report.Knowledge.ReadableSchemaVersions, schema.ReadableVersions()) {
		t.Errorf("knowledge.readable_schema_versions = %v, want %v",
			report.Knowledge.ReadableSchemaVersions, schema.ReadableVersions())
	}
	if report.Knowledge.ReadableSchemaVersions == nil {
		t.Error("knowledge.readable_schema_versions is null; the healthy path emits an array")
	}

	// The blocker is the subsystem that stopped the run, not the first slot in
	// §107's list that was left unread because of it.
	if report.BlockingComponent != ComponentRuntimeDB {
		t.Errorf("blocking_component = %q, want %q", report.BlockingComponent, ComponentRuntimeDB)
	}
	knowledge := report.Components[ComponentKnowledge]
	if knowledge.Code != app.CodeStartupIncomplete {
		t.Errorf("knowledge component code = %q, want %q", knowledge.Code, app.CodeStartupIncomplete)
	}
	if slices.Contains(knowledge.NextAction, "mindrail init") {
		t.Errorf("knowledge component advises %v for a cause nobody established", knowledge.NextAction)
	}
}

// TestSchemaWindowSurvivesEverySubject is the other half of finding R5: the two
// fields that describe this binary must never be zero-valued, whatever the
// repository looks like.
func TestSchemaWindowSurvivesEverySubject(t *testing.T) {
	for name, subject := range buildSubjects() {
		t.Run(name, func(t *testing.T) {
			knowledge := Build(subject, time.Millisecond).Knowledge

			if knowledge.WriteSchemaVersion != schema.WriteVersion {
				t.Errorf("write_schema_version = %d, want %d", knowledge.WriteSchemaVersion, schema.WriteVersion)
			}
			if len(knowledge.ReadableSchemaVersions) == 0 {
				t.Errorf("readable_schema_versions = %v, want %v",
					knowledge.ReadableSchemaVersions, schema.ReadableVersions())
			}
		})
	}
}

// TestUnwritableRuntimePathBlocksStatus is the status half of finding R2. The
// runtime path reading was in no component, so a repository whose runtime
// directory nothing can write reported ok:true and exited 0 — while telling the
// user to run an init that fails on the same permission.
func TestUnwritableRuntimePathBlocksStatus(t *testing.T) {
	subject := uninitializedSubject()
	subject.Probes.RuntimeDir = filesystem.Writability{
		Dir:    "/repo/.git/mindrail",
		Probed: "/repo/.git",
		Exists: false,
		Usable: false,
		Err: app.NewError(app.CodeRuntimePathUnwritable, app.KindUnavailable,
			`the runtime directory "/repo/.git/mindrail" cannot be created because "/repo/.git" is not writable`,
			"Mindrail cannot store its runtime state.",
			`Check the permissions on "/repo/.git".`),
	}

	report := Build(subject, time.Millisecond)

	if report.Readiness != ReadinessBlocked {
		t.Fatalf("readiness = %q, want %q", report.Readiness, ReadinessBlocked)
	}
	component := report.Components[ComponentRuntimeDB]
	if component.State != doctor.StateError {
		t.Errorf("runtime_db state = %q, want %q", component.State, doctor.StateError)
	}
	if component.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("runtime_db code = %q, want %q", component.Code, app.CodeRuntimePathUnwritable)
	}
	if slices.Contains(report.NextAction, "mindrail init") {
		t.Errorf("report advises %v on a repository where init cannot succeed", report.NextAction)
	}
}

// TestBuildNeverReportsPartialReady pins the other half of decision D-02:
// PARTIAL_READY stays in the vocabulary for MR-005, and MR-001 never emits it.
func TestBuildNeverReportsPartialReady(t *testing.T) {
	for name, subject := range buildSubjects() {
		if got := Build(subject, time.Millisecond).Readiness; got == ReadinessPartialReady {
			t.Errorf("subject %q reported %q; no MR-001 component can be partially ready", name, got)
		}
	}
}

var fixedInstant = time.Date(2026, 3, 14, 9, 26, 53, 0, time.UTC)

// healthySubject is a fully initialized repository. Doctor's checks -- and so
// this package -- are pure functions of a Subject, which is why no test here
// needs a disk.
func healthySubject() doctor.Subject {
	return doctor.Subject{
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
			Config:     config.Defaults(),
			Provenance: config.Provenance{config.KeyOutputColor: config.SourceDefault},
			RepoFile:   "/repo/.mindrail/config.toml",
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
		// A healthy repository is one where the coordination summary was read,
		// and the counts being zero is the reading rather than the absence of
		// one. Leaving the flag false published `not_observed` on a report the
		// observation tests require to carry no caveat at all — which is exactly
		// what those tests are for, and they caught it.
		//
		// For a while the flag was true here and unreachable from `init`, which
		// returned from step 7 before reading the summary (finding F10): the
		// fixture described a world the command could not produce. Both commands
		// read it now, and the two fixtures that genuinely have not observed it —
		// uninitializedSubject and haltedAtSQLiteSubject — say so.
		CoordinationObserved: true,
		// Declaring the probe answers keeps this fixture a value: doctor.Probe
		// passes an already-probed subject through, so no test here needs the
		// /repo tree to exist on the machine running it.
		Probes: doctor.Probes{
			Taken:           true,
			RuntimeDirKnown: true,
			RuntimeDir: filesystem.Writability{
				Dir:    "/repo/.git/mindrail",
				Probed: "/repo/.git/mindrail",
				Exists: true,
				Usable: true,
			},
		},
	}
}

// uninitializedSubject is a Git repository mindrail init has never run in: the
// state decision D-01 requires status to report rather than repair.
func uninitializedSubject() doctor.Subject {
	s := healthySubject()
	s.DBPresent = false
	s.Pragmas = storage.Pragmas{}
	s.Migrations = nil
	s.Workspace = workspace.Workspace{}
	s.WorkspaceErr = workspace.ErrNotRegistered
	// Nobody looked at coordination here, and this is the case that sentence
	// describes: there is no database, so step 7 never reached the summary. The
	// flag was inherited from healthySubject, where it is true because `init`
	// and `status` both do read it (finding F10) — leaving it true here made
	// "observed, zero tasks" the reading for a repository with nothing to read.
	s.CoordinationObserved = false
	// The path was inspected and is genuinely empty, which is the one condition
	// `mindrail init` actually fixes.
	s.Probes.DBPath = storage.PresenceAbsent
	return s
}

// haltedAtSQLiteSubject is what bootstrap leaves behind when the runtime store
// cannot be opened: the steps after it never ran, so the knowledge and
// workspace fields are still zero. It is the shape the per-field fixtures above
// cannot produce, and the one that used to be published as fact.
func haltedAtSQLiteSubject() doctor.Subject {
	s := healthySubject()
	s.DBPresent = false
	s.Pragmas = storage.Pragmas{}
	s.Migrations = nil
	s.Knowledge = loader.Store{}
	s.Workspace = workspace.Workspace{}
	// The sequence stopped at step 5, so steps 6 and 7 never ran and nobody
	// looked at coordination. This is the fixture the `not_observed` rendering
	// exists for.
	s.CoordinationObserved = false
	s.DBErr = app.NewError(app.CodeRuntimeDBCorrupt, app.KindUnavailable,
		"the runtime database file is not a valid SQLite database",
		"All recorded workspace state is unreadable.",
		"Move the file aside and run `mindrail init` to rebuild it.")
	return s
}

func subjectWithKnowledgeProblem(fatal bool) doctor.Subject {
	s := healthySubject()
	code := app.CodeKnowledgeUnreadable
	if fatal {
		code = app.CodeKnowledgeSchemaUnsupported
	}
	s.Knowledge.Problems = []loader.Problem{{
		Path:    ".mindrail/knowledge/decisions/dec-9.json",
		Code:    code,
		Message: "record rejected",
		Fatal:   fatal,
	}}
	return s
}

// subjectWithKnowledgeFinding is a store this binary read whole and found
// wrong. It is deliberately not subjectWithKnowledgeProblem: that one is a
// record nothing could read, and decision D-38 keeps the two apart because
// their remedies are opposite.
func subjectWithKnowledgeFinding(cycle bool) doctor.Subject {
	s := healthySubject()
	s.KnowledgeFindings = []validate.Finding{findingOn(".mindrail/knowledge/decisions/dec-1.json", cycle)}
	return s
}

func subjectWithoutRepository() doctor.Subject {
	s := healthySubject()
	s.Repo = git.Repository{}
	s.RepoErr = app.NewError(app.CodeNotAGitRepository, app.KindUsage,
		"No Git repository contains /tmp/elsewhere.",
		"Mindrail cannot locate the repository, so no command can run.",
		"Run mindrail from inside a Git repository.")
	s.Paths = filesystem.RuntimePaths{}
	s.DBPresent = false
	s.Migrations = nil
	s.Pragmas = storage.Pragmas{}
	s.Workspace = workspace.Workspace{}
	s.WorkspaceErr = workspace.ErrNotRegistered
	return s
}
