package changes_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
)

// TestOverlappingBaselinesResolveEndToEnd is TASK-04 AC-04.1. Two tasks
// declare overlapping baselines over one real file and discover the same
// edit — TSK-A through the real AfterChange path, TSK-B row-for-row with
// the same lineage, the way an independent discovery of identical content
// would. A third file outside both scopes drifts. The beats: shared symbols
// ambiguate naming both changes and block; a human override clears one
// symbol while the other stays blocked; the drifted file blocks; resolving
// all three clears the evaluation.
func TestOverlappingBaselinesResolveEndToEnd(t *testing.T) {
	fx := newServiceFixture(t)
	v1 := "def f():\n    return 1\n\ndef g():\n    return 1\n"
	v2 := "def f():\n    return 2\n\ndef g():\n    return 2\n"
	shared := filepath.Join(fx.root, "py", "a.py")
	if err := os.WriteFile(shared, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}
	seedTasks(t, fx.db, "TSK-A", "TSK-B")
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A", []string{shared}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-B", []string{shared}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shared, []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}
	changeA, err := fx.service.AfterChange(t.Context(), changeProject, fx.root, "TSK-A", "")
	if err != nil {
		t.Fatal(err)
	}
	aSymbols, err := fx.store.ReadChangeSymbols(t.Context(), changeA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aSymbols) != 2 {
		t.Fatalf("TSK-A symbols = %d, want f and g", len(aSymbols))
	}
	// TSK-B's independent discovery of the same edit: same file, same
	// lineage, its own change.
	changeB, err := fx.store.EnsureOpenChange(t.Context(), "TSK-B", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertFileRows(t.Context(), changeB.ID, []changes.FileChange{
		{Path: shared, Kind: changes.FileModified, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertSymbolRows(t.Context(), changeB.ID, aSymbols); err != nil {
		t.Fatal(err)
	}
	// The third file: outside both scopes, drifted under TSK-A with a
	// symbol no baseline covers.
	drifted := filepath.Join(fx.root, "py", "c.py")
	if _, err := fx.db.ExecContext(t.Context(), `INSERT OR IGNORE INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES ('SYM-E2E-C', 'PRJ-1', ?, 'python', 'c::h', '[]', '2026-09-23T10:00:00Z')`,
		fx.units["py"].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO symbols
		(unit_id, path, logical_key, kind, name, start_line, start_col, end_line, end_col,
		signature_hash, body_hash, structure_hash, symbol_uid)
		VALUES (?, ?, 'c::h', 'function', 'h', 1, 0, 2, 0, 's', 'b', 't', 'SYM-E2E-C')`,
		fx.units["py"].ID, drifted); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertFileRows(t.Context(), changeA.ID, []changes.FileChange{
		{Path: drifted, Kind: changes.FileAdded, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.UpsertSymbolRows(t.Context(), changeA.ID, []changes.SymbolChange{
		{Key: "c::h", UID: "SYM-E2E-C", Kind: changes.SymbolAdded, Via: changes.ViaReconcile},
	}); err != nil {
		t.Fatal(err)
	}

	// Beat 1: shared symbols ambiguate naming both changes and block.
	blocked, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]app.Code{}
	for _, finding := range blocked {
		byKey[finding.Provenance.ChangeKey] = finding.Code
		if !finding.Blocking {
			t.Fatalf("%q does not block", finding.Code)
		}
	}
	if byKey[aSymbols[0].Key] != app.CodeReconcileAmbiguous ||
		byKey[aSymbols[1].Key] != app.CodeReconcileAmbiguous {
		t.Fatalf("shared symbols = %+v, want both ambiguous", byKey)
	}
	for _, finding := range blocked {
		if finding.Code != app.CodeReconcileAmbiguous {
			continue
		}
		for _, id := range []string{changeA.ID, changeB.ID} {
			if !strings.Contains(finding.Detail, id) {
				t.Fatalf("ambiguous detail names no %q: %q", id, finding.Detail)
			}
		}
	}
	if byKey[drifted] != app.CodeScopeDrift {
		t.Fatalf("drifted file = %+v, want drift", byKey)
	}
	if byKey["c::h"] != app.CodeUnregisteredChange {
		t.Fatalf("drifted symbol = %+v, want unregistered", byKey)
	}

	// Beat 2: the human assigns one shared symbol; the other stays blocked.
	if err := fx.store.RecordAttribution(t.Context(), aSymbols[0].Key,
		changeA.ID, "SES-1", "A owns f"); err != nil {
		t.Fatal(err)
	}
	half, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	halfKeys := map[string]bool{}
	for _, finding := range half {
		halfKeys[finding.Provenance.ChangeKey] = true
	}
	if halfKeys[aSymbols[0].Key] {
		t.Fatalf("overridden symbol still blocked: %+v", half)
	}
	if !halfKeys[aSymbols[1].Key] {
		t.Fatalf("sibling symbol cleared without an override: %+v", half)
	}

	// Beat 3: resolve everything — override the sibling and the drifted
	// symbol first: only the drift finding may remain, which proves each
	// override is load-bearing. Then extend the baseline over the drifted
	// file, and the evaluation clears.
	if err := fx.store.RecordAttribution(t.Context(), aSymbols[1].Key,
		changeA.ID, "SES-1", "A owns g"); err != nil {
		t.Fatal(err)
	}
	if err := fx.store.RecordAttribution(t.Context(), "c::h",
		changeA.ID, "SES-1", "A owns h"); err != nil {
		t.Fatal(err)
	}
	overridden, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(overridden) != 1 || overridden[0].Code != app.CodeScopeDrift {
		t.Fatalf("overridden = %+v, want only the drift", overridden)
	}
	if err := fx.store.ClearBaseline(t.Context(), "TSK-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-A",
		[]string{shared, drifted}, ""); err != nil {
		t.Fatal(err)
	}
	clear, err := fx.service.EvaluateTask(t.Context(), "TSK-A", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(clear) != 0 {
		t.Fatalf("clear = %+v, want empty", clear)
	}
}
