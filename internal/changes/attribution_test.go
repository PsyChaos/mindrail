package changes_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
)

func TestPreferredTaskOwnersLifecyclePrecedence(t *testing.T) {
	older := time.Date(2026, 9, 23, 10, 1, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	scopes := map[string][]string{
		"TSK-A": {"/r/a.py"},
		"TSK-B": {"/r/a.py"},
	}
	for _, tc := range []struct {
		name      string
		ownership map[string]changes.TaskOwnership
		want      []string
	}{
		{
			name: "unique latest completed",
			ownership: map[string]changes.TaskOwnership{
				"TSK-A": {State: "COMPLETED", UpdatedAt: older},
				"TSK-B": {State: "COMPLETED", UpdatedAt: newer},
			},
			want: []string{"TSK-B"},
		},
		{
			name: "latest completed tie",
			ownership: map[string]changes.TaskOwnership{
				"TSK-A": {State: "COMPLETED", UpdatedAt: newer},
				"TSK-B": {State: "COMPLETED", UpdatedAt: newer},
			},
			want: []string{"TSK-A", "TSK-B"},
		},
		{
			name: "current beats newer history",
			ownership: map[string]changes.TaskOwnership{
				"TSK-A": {State: "IN_PROGRESS", UpdatedAt: older},
				"TSK-B": {State: "COMPLETED", UpdatedAt: newer},
			},
			want: []string{"TSK-A"},
		},
		{
			name: "missing owner does not collapse overlap",
			ownership: map[string]changes.TaskOwnership{
				"TSK-A": {State: "IN_PROGRESS", UpdatedAt: older},
			},
			want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := changes.PreferredTaskOwners(scopes, tc.ownership, "/r/a.py")
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("owners = %v, want %v", got, tc.want)
			}
		})
	}

	if got := changes.PreferredTaskOwners(
		map[string][]string{"TSK-A": {"/r/a.py"}},
		map[string]changes.TaskOwnership{},
		"/r/a.py",
	); got != nil {
		t.Fatalf("sole missing owner = %v, want no authorization", got)
	}
}

// attributionSetup seeds tasks with baselines, open changes, file rows and
// symbol rows, plus the index rows uid→path resolution reads. Baselines go
// in as SQL: the tests pin attribution, not capture.
func attributionSetup(t *testing.T, fx serviceFixture, task, scope, file, key, uid string) {
	t.Helper()
	seedTasks(t, fx.db, task)
	for _, path := range []string{scope} {
		if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO change_baselines
			(task_id, path, content_hash, captured_at) VALUES (?, ?, 'h', '2026-09-23T10:00:00Z')`,
			task, path); err != nil {
			t.Fatal(err)
		}
	}
	change, err := fx.store.EnsureOpenChange(t.Context(), task, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertFileRows(t.Context(), change.ID, []changes.FileChange{
		{Path: file, Kind: changes.FileModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
	if key != "" && uid != "" {
		if _, err := fx.db.ExecContext(t.Context(), `INSERT OR IGNORE INTO symbol_identities
			(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
			VALUES (?, 'PRJ-1', ?, 'python', ?, '[]', '2026-09-23T10:00:00Z')`,
			uid, fx.units["py"].ID, key); err != nil {
			t.Fatal(err)
		}
	}
	if key != "" {
		if err := fx.store.UpsertSymbolRows(t.Context(), change.ID, []changes.SymbolChange{
			{Key: key, UID: uid, Kind: changes.SymbolModified, Via: changes.ViaReconcile},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if uid != "" {
		if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO symbols
			(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col,
			signature_hash, body_hash, structure_hash, symbol_uid)
			VALUES (?, ?, ?, 'function', 'f', 1, 0, 2, 0, 's', 'b', 't', ?)`,
			fx.units["py"].ID, file, key, uid); err != nil {
			t.Fatal(err)
		}
	}
}

func taskChangeID(t *testing.T, fx serviceFixture, task string) string {
	t.Helper()
	change, err := fx.store.EnsureOpenChange(t.Context(), task, "")
	if err != nil {
		t.Fatal(err)
	}
	return change.ID
}

// TestAttributeTaskSingleCandidateAttributes is TASK-02 AC-02.1. One
// candidate attributes: the outcome names the change, no finding, and
// repeated evaluation is stable.
func TestAttributeTaskSingleCandidateAttributes(t *testing.T) {
	fx := newServiceFixture(t)
	attributionSetup(t, fx, "TSK-A", "/r/a.py", "/r/a.py", "a::f", "SYM-A-1")

	first, err := fx.service.AttributeTask(t.Context(), "TSK-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Symbols) != 1 {
		t.Fatalf("symbols = %d, want 1", len(first.Symbols))
	}
	symbol := first.Symbols[0]
	if symbol.Outcome != changes.AttributedOutcome {
		t.Fatalf("outcome = %q", symbol.Outcome)
	}
	if symbol.AttributedTo != taskChangeID(t, fx, "TSK-A") {
		t.Fatalf("attributed to %q", symbol.AttributedTo)
	}
	if symbol.Finding != nil {
		t.Fatalf("attributed symbol carries a finding: %+v", symbol.Finding)
	}
	if len(first.Drift) != 0 {
		t.Fatalf("drift = %d, want 0", len(first.Drift))
	}
	second, err := fx.service.AttributeTask(t.Context(), "TSK-A")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("evaluation is not stable:\n%+v\n%+v", first, second)
	}
}

// TestAttributeTaskZeroCandidatesIsUnregistered is TASK-02 AC-02.2. The
// symbol resolves to a file no baseline covers: UNREGISTERED_CHANGE with
// provenance and next_action, and it blocks.
func TestAttributeTaskZeroCandidatesIsUnregistered(t *testing.T) {
	fx := newServiceFixture(t)
	attributionSetup(t, fx, "TSK-A", "/r/a.py", "/r/a.py", "a::f", "SYM-A-1")
	// Move the symbol's live file outside every baseline without touching
	// the change rows: b.py is covered nowhere.
	if _, err := fx.db.ExecContext(t.Context(),
		`UPDATE symbols SET path = '/r/b.py' WHERE symbol_uid = 'SYM-A-1'`); err != nil {
		t.Fatal(err)
	}

	answer, err := fx.service.AttributeTask(t.Context(), "TSK-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Symbols) != 1 {
		t.Fatalf("symbols = %d, want 1", len(answer.Symbols))
	}
	symbol := answer.Symbols[0]
	if symbol.Outcome != changes.UnregisteredOutcome {
		t.Fatalf("outcome = %q", symbol.Outcome)
	}
	if symbol.Finding == nil {
		t.Fatal("unregistered symbol carries no finding")
	}
	finding := *symbol.Finding
	if finding.Code != app.CodeUnregisteredChange {
		t.Fatalf("code = %q", finding.Code)
	}
	if !finding.Blocking {
		t.Fatal("unregistered finding does not block")
	}
	if finding.Provenance.TaskID != "TSK-A" || finding.Provenance.ChangeKey != "a::f" ||
		finding.Provenance.Via != changes.ViaReconcile {
		t.Fatalf("provenance = %+v", finding.Provenance)
	}
	if len(finding.NextAction) == 0 {
		t.Fatal("unregistered finding names no remedy")
	}
}

// TestAttributeTaskTwoCandidatesIsAmbiguous is TASK-02 AC-02.3. Two tasks
// baseline the same file: its symbol ambiguates naming both changes, and no
// candidate is chosen.
func TestAttributeTaskTwoCandidatesIsAmbiguous(t *testing.T) {
	fx := newServiceFixture(t)
	attributionSetup(t, fx, "TSK-A", "/r/a.py", "/r/a.py", "a::f", "SYM-A-1")
	attributionSetup(t, fx, "TSK-B", "/r/a.py", "/r/a.py", "a::f", "SYM-A-1")

	answer, err := fx.service.AttributeTask(t.Context(), "TSK-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Symbols) != 1 {
		t.Fatalf("symbols = %d, want 1", len(answer.Symbols))
	}
	symbol := answer.Symbols[0]
	if symbol.Outcome != changes.AmbiguousOutcome {
		t.Fatalf("outcome = %q", symbol.Outcome)
	}
	if symbol.AttributedTo != "" {
		t.Fatalf("ambiguous symbol chose %q", symbol.AttributedTo)
	}
	want := []string{taskChangeID(t, fx, "TSK-A"), taskChangeID(t, fx, "TSK-B")}
	if !reflect.DeepEqual(symbol.Candidates, want) {
		t.Fatalf("candidates = %q, want %q", symbol.Candidates, want)
	}
	if symbol.Finding == nil || symbol.Finding.Code != app.CodeReconcileAmbiguous {
		t.Fatalf("finding = %+v", symbol.Finding)
	}
	if !symbol.Finding.Blocking {
		t.Fatal("ambiguous finding does not block")
	}
}

// TestAttributeTaskDriftFiresPerFile is TASK-02 AC-02.4. Files outside the
// baseline scope drift and block; in-scope files never drift.
func TestAttributeTaskDriftFiresPerFile(t *testing.T) {
	fx := newServiceFixture(t)
	attributionSetup(t, fx, "TSK-A", "/r/a.py", "/r/a.py", "", "")
	change, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertFileRows(t.Context(), change.ID, []changes.FileChange{
		{Path: "/r/c.py", Kind: changes.FileAdded, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}

	answer, err := fx.service.AttributeTask(t.Context(), "TSK-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Drift) != 1 {
		t.Fatalf("drift = %d, want 1", len(answer.Drift))
	}
	drift := answer.Drift[0]
	if drift.Code != app.CodeScopeDrift {
		t.Fatalf("code = %q", drift.Code)
	}
	if drift.Provenance.ChangeKey != "/r/c.py" {
		t.Fatalf("drift path = %q", drift.Provenance.ChangeKey)
	}
	if !drift.Blocking {
		t.Fatal("drift finding does not block")
	}
	if len(drift.NextAction) == 0 {
		t.Fatal("drift finding names no remedy")
	}
	if !reflect.DeepEqual(drift.Provenance.BaselineScope, []string{"/r/a.py"}) {
		t.Fatalf("drift baseline scope = %q", drift.Provenance.BaselineScope)
	}
}

// TestAttributeTaskNeverInventsPaths is TASK-02 AC-02.5. A symbol whose uid
// resolves nowhere is unregistered with an empty file — the finding judges
// the row it read, never a path it guessed.
func TestAttributeTaskNeverInventsPaths(t *testing.T) {
	fx := newServiceFixture(t)
	attributionSetup(t, fx, "TSK-A", "/r/a.py", "/r/a.py", "a::f", "")
	if _, err := fx.db.ExecContext(t.Context(),
		`UPDATE change_symbols SET symbol_uid = NULL
		WHERE logical_key = 'a::f'`); err != nil {
		t.Fatal(err)
	}

	answer, err := fx.service.AttributeTask(t.Context(), "TSK-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Symbols) != 1 {
		t.Fatalf("symbols = %d, want 1", len(answer.Symbols))
	}
	symbol := answer.Symbols[0]
	if symbol.File != "" {
		t.Fatalf("file = %q, want empty", symbol.File)
	}
	if symbol.Outcome != changes.UnregisteredOutcome || symbol.Finding == nil {
		t.Fatalf("outcome = %+v", symbol)
	}
}

func attributeMissingLiveProjectedSymbol(t *testing.T, mode, key, kind string) changes.SymbolAttribution {
	t.Helper()
	fx := newServiceFixture(t)
	file := "/r/pkg/a.py"
	uid := "SYM-MISSING-LIVE"
	attributionSetup(t, fx, "TSK-A", file, file, "", "")
	changeTask := "TSK-A"
	operation := ""
	if mode == "staged" {
		changeTask = ""
		operation = "verify-staged-v2"
	}
	change, err := fx.store.EnsureOpenChange(t.Context(), changeTask, operation)
	if err != nil {
		t.Fatal(err)
	}
	if mode == "staged" {
		if err := fx.store.UpsertFileRows(t.Context(), change.ID, []changes.FileChange{
			{Path: file, Kind: changes.FileModified, Via: changes.ViaReconcile},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES (?, 'PRJ-1', ?, 'python', ?, '[]', '2026-09-23T10:00:00Z')`,
		uid, fx.units["py"].ID, key); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertSymbolRows(t.Context(), change.ID, []changes.SymbolChange{
		{Key: key, UID: uid, Kind: kind, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}

	var answer changes.TaskAttribution
	if mode == "task" {
		answer, err = fx.service.AttributeTask(t.Context(), "TSK-A")
	} else {
		answer, err = fx.service.AttributeChanges(t.Context(), []string{change.ID})
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Symbols) != 1 {
		t.Fatalf("symbols = %d, want 1", len(answer.Symbols))
	}
	return answer.Symbols[0]
}

func TestProjectedAttributionRejectsMalformedOuterLogicalKey(t *testing.T) {
	for _, mode := range []string{"task", "staged"} {
		for _, tc := range []struct {
			name string
			key  string
		}{
			{name: "missing local", key: `["pkg/a.py"]`},
			{name: "empty local", key: `["pkg/a.py",""]`},
			{name: "extra element", key: `["pkg/a.py","function:check_boundary","extra"]`},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				symbol := attributeMissingLiveProjectedSymbol(t, mode, tc.key, changes.SymbolRemoved)
				if symbol.File != "" || symbol.Outcome != changes.UnregisteredOutcome || symbol.AttributedTo != "" || symbol.Finding == nil {
					t.Fatalf("symbol = %+v, want malformed key to stay unregistered", symbol)
				}
			})
		}
	}
}

func TestAttributeChangesDoesNotProjectMissingLiveNonRemovedSymbol(t *testing.T) {
	for _, kind := range []string{changes.SymbolAdded, changes.SymbolModified} {
		t.Run(kind, func(t *testing.T) {
			symbol := attributeMissingLiveProjectedSymbol(t, "staged",
				`["pkg/a.py","function:check_boundary"]`, kind)
			if symbol.File != "" || symbol.Outcome != changes.UnregisteredOutcome || symbol.AttributedTo != "" || symbol.Finding == nil {
				t.Fatalf("symbol = %+v, want unresolved %s to stay unregistered", symbol, kind)
			}
		})
	}
}

// A symbol removed from a still-present scoped file has no live symbols row.
// Task completion recovers that source path from the qualified key and fixed
// Change projection, just as staged verification does.
func TestAttributeTaskResolvesRemovedSymbolFromProjection(t *testing.T) {
	fx := newServiceFixture(t)
	file := "/r/pkg/a.py"
	key := `["pkg/a.py","function:check_boundary"]`
	attributionSetup(t, fx, "TSK-A", file, file, "", "")
	change, err := fx.store.EnsureOpenChange(t.Context(), "TSK-A", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES (?, 'PRJ-1', ?, 'python', ?, '[]', '2026-09-23T10:00:00Z')`,
		"SYM-TASK-REMOVED", fx.units["py"].ID, key); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertSymbolRows(t.Context(), change.ID, []changes.SymbolChange{
		{Key: key, UID: "SYM-TASK-REMOVED", Kind: changes.SymbolRemoved, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}

	answer, err := fx.service.AttributeTask(t.Context(), "TSK-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Symbols) != 1 {
		t.Fatalf("symbols = %d, want 1", len(answer.Symbols))
	}
	symbol := answer.Symbols[0]
	if symbol.File != file || symbol.Outcome != changes.AttributedOutcome || symbol.AttributedTo != change.ID {
		t.Fatalf("symbol = %+v, want removed symbol attributed through %s", symbol, file)
	}
}

// Only removals are expected to lack a live symbols row. ADDED and MODIFIED
// rows with an unresolved identity must stay fail-closed even when their
// qualified key happens to match this Change's file projection.
func TestAttributeTaskDoesNotProjectMissingLiveNonRemovedSymbol(t *testing.T) {
	for _, kind := range []string{changes.SymbolAdded, changes.SymbolModified} {
		t.Run(kind, func(t *testing.T) {
			symbol := attributeMissingLiveProjectedSymbol(t, "task",
				`["pkg/a.py","function:check_boundary"]`, kind)
			if symbol.File != "" || symbol.Outcome != changes.UnregisteredOutcome || symbol.AttributedTo != "" || symbol.Finding == nil {
				t.Fatalf("symbol = %+v, want unresolved %s to stay unregistered", symbol, kind)
			}
		})
	}
}

// A removed staged symbol has no live symbols row after indexing. Global
// commit attribution recovers its exact file from the qualified logical key
// and the fixed staged projection, so normal test deletion remains owned.
func TestAttributeChangesResolvesRemovedSymbolFromProjection(t *testing.T) {
	fx := newServiceFixture(t)
	file := "/r/pkg/a.py"
	key := `["pkg/a.py","function:test_a"]`
	attributionSetup(t, fx, "TSK-A", file, file, "", "")
	change, err := fx.store.EnsureOpenChange(t.Context(), "", "verify-staged-v2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES (?, 'PRJ-1', ?, 'python', ?, '[]', '2026-09-23T10:00:00Z')`,
		"SYM-REMOVED", fx.units["py"].ID, key); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertFileRows(t.Context(), change.ID, []changes.FileChange{
		{Path: file, Kind: changes.FileModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertSymbolRows(t.Context(), change.ID, []changes.SymbolChange{
		{Key: key, UID: "SYM-REMOVED", Kind: changes.SymbolRemoved, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}

	answer, err := fx.service.AttributeChanges(t.Context(), []string{change.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Symbols) != 1 {
		t.Fatalf("symbols = %d, want 1", len(answer.Symbols))
	}
	symbol := answer.Symbols[0]
	if symbol.File != file || symbol.Outcome != changes.AttributedOutcome {
		t.Fatalf("symbol = %+v, want removed symbol attributed through %s", symbol, file)
	}
}

// TestAttributeTaskWithoutChangeIsEmpty pins the no-discovery rule: a task
// with a baseline but no open Change evaluates empty, never an error.
func TestAttributeTaskWithoutChangeIsEmpty(t *testing.T) {
	fx := newServiceFixture(t)
	seedTasks(t, fx.db, "TSK-Q")
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO change_baselines
		(task_id, path, content_hash, captured_at)
		VALUES ('TSK-Q', '/r/a.py', 'h', '2026-09-23T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	answer, err := fx.service.AttributeTask(t.Context(), "TSK-Q")
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Symbols) != 0 || len(answer.Drift) != 0 {
		t.Fatalf("answer = %+v", answer)
	}
	if !reflect.DeepEqual(answer.BaselineScope, []string{"/r/a.py"}) {
		t.Fatalf("scope = %q", answer.BaselineScope)
	}
}
