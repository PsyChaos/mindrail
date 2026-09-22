package changes_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
)

func codesOf(findings []changes.Finding) []app.Code {
	var codes []app.Code
	for _, finding := range findings {
		codes = append(codes, finding.Code)
	}
	return codes
}

// TestOverrideClearsOneSymbolWhileSiblingsBlock is TASK-03 AC-03.1.
// Overrides resolve by logical key, globally (decision D-137): recording
// one for a::f clears it on both tasks' evaluations, while TSK-B's second
// symbol — same overlap, no override — stays blocked.
func TestOverrideClearsOneSymbolWhileSiblingsBlock(t *testing.T) {
	fx := newServiceFixture(t)
	attributionSetup(t, fx, "TSK-A", "/r/a.py", "/r/a.py", "a::f", "SYM-A-1")
	attributionSetup(t, fx, "TSK-B", "/r/a.py", "/r/a.py", "a::f", "SYM-A-1")
	changeB := taskChangeID(t, fx, "TSK-B")
	if _, err := fx.db.ExecContext(t.Context(), `INSERT OR IGNORE INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES ('SYM-H', 'PRJ-1', ?, 'python', 'a::h', '[]', '2026-09-23T10:00:00Z')`,
		fx.units["py"].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO symbols
		(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col,
		signature_hash, body_hash, structure_hash, symbol_uid)
		VALUES (?, '/r/a.py', 'a::h', 'function', 'h', 3, 0, 4, 0, 's', 'b', 't', 'SYM-H')`,
		fx.units["py"].ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertSymbolRows(t.Context(), changeB, []changes.SymbolChange{
		{Key: "a::h", UID: "SYM-H", Kind: changes.SymbolModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}

	before, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || before[0].Code != app.CodeReconcileAmbiguous {
		t.Fatalf("before = %+v", before)
	}
	if err := fx.store.RecordAttribution(t.Context(), "a::f",
		taskChangeID(t, fx, "TSK-A"), "SES-1", "A owns it"); err != nil {
		t.Fatal(err)
	}
	after, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("after = %+v, want empty", after)
	}
	sibling, err := fx.service.EvaluateTask(t.Context(), "TSK-B", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sibling) != 1 || sibling[0].Code != app.CodeReconcileAmbiguous ||
		sibling[0].Provenance.ChangeKey != "a::h" {
		t.Fatalf("sibling = %+v, want the un-overridden a::h ambiguity", sibling)
	}
}

// TestOverrideBindsUnregisteredSymbols is TASK-03 AC-03.1 on the other
// track: an unregistered symbol binds to the recorded change the same way.
func TestOverrideBindsUnregisteredSymbols(t *testing.T) {
	fx := newServiceFixture(t)
	attributionSetup(t, fx, "TSK-A", "/r/a.py", "/r/a.py", "a::f", "SYM-A-1")
	if _, err := fx.db.ExecContext(t.Context(),
		`UPDATE symbols SET path = '/r/elsewhere.py' WHERE symbol_uid = 'SYM-A-1'`); err != nil {
		t.Fatal(err)
	}
	before, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || before[0].Code != app.CodeUnregisteredChange {
		t.Fatalf("before = %+v", before)
	}
	if err := fx.store.RecordAttribution(t.Context(), "a::f",
		taskChangeID(t, fx, "TSK-A"), "SES-1", "declared late"); err != nil {
		t.Fatal(err)
	}
	after, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("after = %+v, want empty", after)
	}
}

// TestRecordAttributionRefusesBeforeAnyWrite is TASK-03 AC-03.2 and closes
// Breaker B-1: empty deciders, unknown changes and malformed keys are
// refused, and the refusal leaves no row behind.
func TestRecordAttributionRefusesBeforeAnyWrite(t *testing.T) {
	fx := newServiceFixture(t)
	attributionSetup(t, fx, "TSK-A", "/r/a.py", "/r/a.py", "", "")
	change := taskChangeID(t, fx, "TSK-A")

	refusals := []struct {
		name            string
		key, change, by string
	}{
		{"empty key", "", change, "SES-1"},
		{"empty change", "a::f", "", "SES-1"},
		{"unknown change", "a::f", "CHG-NOPE", "SES-1"},
		{"empty decider", "a::f", change, ""},
	}
	for _, refusal := range refusals {
		if err := fx.store.RecordAttribution(t.Context(), refusal.key, refusal.change, refusal.by, "x"); err == nil {
			t.Fatalf("%s accepted", refusal.name)
		}
	}
	read, err := fx.store.ReadAttributions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 0 {
		t.Fatalf("refused writes left rows: %+v", read)
	}
}

// TestEvaluateTaskReturnsTheBlockingSet is TASK-03 AC-03.3. Ambiguous,
// unregistered and drifted items arrive together, each with code,
// provenance and next_action; a clean task returns empty.
func TestEvaluateTaskReturnsTheBlockingSet(t *testing.T) {
	fx := newServiceFixture(t)
	attributionSetup(t, fx, "TSK-A", "/r/a.py", "/r/a.py", "a::f", "SYM-A-1")
	attributionSetup(t, fx, "TSK-B", "/r/a.py", "/r/a.py", "a::g", "SYM-B-1")
	// TSK-A's change additionally drifts a third file and holds an
	// unregistered symbol (its live file is covered nowhere).
	changeA := taskChangeID(t, fx, "TSK-A")
	if err := fx.store.UpsertFileRows(t.Context(), changeA, []changes.FileChange{
		{Path: "/r/c.py", Kind: changes.FileAdded, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT OR IGNORE INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES ('SYM-LONE', 'PRJ-1', ?, 'python', 'c::lone', '[]', '2026-09-23T10:00:00Z')`,
		fx.units["py"].ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertSymbolRows(t.Context(), changeA, []changes.SymbolChange{
		{Key: "c::lone", UID: "SYM-LONE", Kind: changes.SymbolAdded, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}

	blocked, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	codes := codesOf(blocked)
	want := []app.Code{app.CodeScopeDrift, app.CodeReconcileAmbiguous, app.CodeUnregisteredChange}
	if len(codes) != len(want) {
		t.Fatalf("codes = %q, want %q", codes, want)
	}
	seen := map[app.Code]bool{}
	for _, finding := range blocked {
		seen[finding.Code] = true
		if !finding.Blocking {
			t.Fatalf("%q does not block", finding.Code)
		}
		if finding.Provenance.TaskID != "TSK-A" {
			t.Fatalf("provenance = %+v", finding.Provenance)
		}
		if len(finding.NextAction) == 0 {
			t.Fatalf("%q names no remedy", finding.Code)
		}
	}
	for _, code := range want {
		if !seen[code] {
			t.Fatalf("codes = %q, missing %q", codes, code)
		}
	}

	if err := fx.store.RecordAttribution(t.Context(), "a::f", changeA, "SES-1", "A"); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.RecordAttribution(t.Context(), "c::lone", changeA, "SES-1", "A"); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.ClearBaseline(t.Context(), "TSK-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO change_baselines
		(task_id, path, content_hash, captured_at) VALUES
		('TSK-A', '/r/a.py', 'h', '2026-09-23T10:00:00Z'),
		('TSK-A', '/r/c.py', 'h', '2026-09-23T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	// Resolution paths, one per finding: the ambiguity and the unregistered
	// symbol clear through overrides (even while TSK-B still baselines
	// /r/a.py), and the drift clears by extending the baseline to cover
	// /r/c.py.
	rest, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 0 {
		t.Fatalf("rest = %+v, want empty: overrides clear their symbols", rest)
	}
}

// TestLeaseGuidanceNamesExactlyOneHolder is TASK-03 AC-03.4, pinned both
// directions: one covering lease is named, zero or several name none — and
// attribution never follows the lease either way.
func TestLeaseGuidanceNamesExactlyOneHolder(t *testing.T) {
	fx := newServiceFixture(t)
	attributionSetup(t, fx, "TSK-A", "/r/a.py", "/r/a.py", "a::f", "SYM-A-1")
	attributionSetup(t, fx, "TSK-B", "/r/a.py", "/r/a.py", "a::f", "SYM-A-1")
	file := changes.LeaseView{Holder: "SES-1", Kind: "file", Key: "/r/a.py"}

	one, err := fx.service.EvaluateTask(t.Context(), "TSK-A", []changes.LeaseView{file})
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].Code != app.CodeReconcileAmbiguous {
		t.Fatalf("one = %+v, want a still-blocked ambiguity", one)
	}
	if len(one[0].NextAction) != 2 {
		t.Fatalf("next_action = %q, want remedy plus holder", one[0].NextAction)
	}

	none, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 1 || len(none[0].NextAction) != 1 {
		t.Fatalf("none = %+v", none)
	}
	several, err := fx.service.EvaluateTask(t.Context(), "TSK-A", []changes.LeaseView{file,
		{Holder: "SES-2", Kind: "task", Key: "TSK-B"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(several) != 1 || len(several[0].NextAction) != 1 {
		t.Fatalf("several = %+v, want no holder named", several)
	}
}

// TestOwnerlessWorkEvaluatesUnregistered is TASK-03 AC-03.5. NULL-task
// changes ride the unregistered track, never an error path; a named task
// with no change at all evaluates empty the same way.
func TestOwnerlessWorkEvaluatesUnregistered(t *testing.T) {
	fx := newServiceFixture(t)
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO changes
		(change_id, task_id, created_at, updated_at)
		VALUES ('CHG-OWNERLESS', NULL, '2026-09-23T10:00:00Z', '2026-09-23T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT OR IGNORE INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES ('SYM-ORPH', 'PRJ-1', ?, 'python', 'o::f', '[]', '2026-09-23T10:00:00Z')`,
		fx.units["py"].ID); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertSymbolRows(t.Context(), "CHG-OWNERLESS", []changes.SymbolChange{
		{Key: "o::f", UID: "SYM-ORPH", Kind: changes.SymbolAdded, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}

	blocked, err := fx.service.EvaluateTask(t.Context(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 1 || blocked[0].Code != app.CodeUnregisteredChange {
		t.Fatalf("blocked = %+v", blocked)
	}
	if blocked[0].Provenance.ChangeID != "CHG-OWNERLESS" {
		t.Fatalf("provenance = %+v", blocked[0].Provenance)
	}
	seedTasks(t, fx.db, "TSK-Q")
	empty, err := fx.service.EvaluateTask(t.Context(), "TSK-Q", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty = %+v", empty)
	}
}
