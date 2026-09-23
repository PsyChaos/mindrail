package validation_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/validation"
)

func provenanceForTest(t *testing.T, profile string, scope []string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"profile": profile, "command_index": 0, "scope": scope})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func evidenceRow(t *testing.T, id, profile, root string, rels []string) validation.Evidence {
	t.Helper()
	snapshot, err := validation.SnapshotScope(root, rels)
	if err != nil {
		t.Fatal(err)
	}
	return validation.Evidence{
		ID:           id,
		Profile:      profile,
		Type:         "AUTOMATED_TEST",
		Argv:         []string{"echo", "hi"},
		Status:       validation.StatusPass,
		SnapshotHash: snapshot.Hash,
		Provenance:   provenanceForTest(t, profile, snapshot.Scope),
	}
}

// TestCheckKeepsCurrentOnUntouchedScope is TASK-01 AC-01.1: the scope
// re-hashes equal, the row evaluates current with profile, hash and scope
// named in the reason path.
func TestCheckKeepsCurrentOnUntouchedScope(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	row := evidenceRow(t, "EVD-1", "test", root, []string{"tests"})

	verdicts, coverage, err := validation.Check(root, []validation.Evidence{row}, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 1 {
		t.Fatalf("verdicts = %+v", verdicts)
	}
	verdict := verdicts[0]
	if verdict.Status != validation.FreshCurrent {
		t.Fatalf("verdict = %+v", verdict)
	}
	if verdict.Reason == "" || verdict.CurrentHash != row.SnapshotHash {
		t.Fatalf("verdict = %+v", verdict)
	}
	if !coverage.Satisfied["test"] || len(coverage.ReRun) != 0 {
		t.Fatalf("coverage = %+v", coverage)
	}
}

// TestCheckStalesRelevantEdits is TASK-01 AC-01.2: content change, removed
// file and unreadable scope each go stale with the reason naming the gap.
func TestCheckStalesRelevantEdits(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	writeScopeFile(t, root, "tests/b.py", "print(2)\n")
	row := evidenceRow(t, "EVD-1", "test", root, []string{"tests"})

	writeScopeFile(t, root, "tests/a.py", "print(3)\n")
	verdicts, _, err := validation.Check(root, []validation.Evidence{row}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshStale {
		t.Fatalf("content edit = %+v", verdicts[0])
	}
	if verdicts[0].Reason == "" {
		t.Fatal("stale reason empty")
	}

	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	if err := os.Remove(filepath.Join(root, "tests", "b.py")); err != nil {
		t.Fatal(err)
	}
	verdicts, _, err = validation.Check(root, []validation.Evidence{row}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshStale {
		t.Fatalf("removed file = %+v", verdicts[0])
	}

	writeScopeFile(t, root, "tests/b.py", "print(2)\n")
	if err := os.Chmod(filepath.Join(root, "tests", "a.py"), 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(filepath.Join(root, "tests", "a.py"), 0o644) }()
	verdicts, _, err = validation.Check(root, []validation.Evidence{row}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshStale {
		t.Fatalf("unreadable scope = %+v", verdicts[0])
	}
}

// TestCheckIgnoresOutOfScopeEdits is TASK-01 AC-01.3: edits outside the
// row's scope — including added files the scope never listed — leave it
// current. Scope is file-grain: a file the evidence never covered cannot
// expire it (completeness belongs to MR-013's breadth, not to staleness).
func TestCheckIgnoresOutOfScopeEdits(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	row := evidenceRow(t, "EVD-1", "test", root, []string{"tests/a.py"})
	writeScopeFile(t, root, "tests/new.py", "print(9)\n")
	writeScopeFile(t, root, "other/z.py", "print(9)\n")

	verdicts, _, err := validation.Check(root, []validation.Evidence{row}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshCurrent {
		t.Fatalf("out-of-scope edit = %+v", verdicts[0])
	}
}

// TestCheckRequiredCoverage is TASK-01 AC-01.4: satisfied, stale-only and
// missing profiles report correctly, and the re-run list unions stale
// rows' profiles with uncovered required ones, sorted, each with a reason.
func TestCheckRequiredCoverage(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	writeScopeFile(t, root, "lint/b.py", "print(2)\n")
	fresh := evidenceRow(t, "EVD-1", "test", root, []string{"tests"})
	stale := evidenceRow(t, "EVD-2", "lint", root, []string{"lint"})
	writeScopeFile(t, root, "lint/b.py", "print(3)\n")

	_, coverage, err := validation.Check(root,
		[]validation.Evidence{fresh, stale}, []string{"test", "lint", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if !coverage.Satisfied["test"] || coverage.Satisfied["lint"] || coverage.Satisfied["missing"] {
		t.Fatalf("satisfied = %+v", coverage.Satisfied)
	}
	if len(coverage.ReRun) != 2 {
		t.Fatalf("rerun = %+v", coverage.ReRun)
	}
	if coverage.ReRun[0].Profile != "lint" || coverage.ReRun[1].Profile != "missing" {
		t.Fatalf("rerun = %+v, want sorted lint, missing", coverage.ReRun)
	}
	for _, item := range coverage.ReRun {
		if item.Reason == "" {
			t.Fatalf("rerun item without reason: %+v", item)
		}
	}
	// A stale row whose profile is not required still names its profile:
	// re-run is a union, not a filter.
	_, partial, err := validation.Check(root,
		[]validation.Evidence{fresh, stale}, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(partial.ReRun) != 1 || partial.ReRun[0].Profile != "lint" {
		t.Fatalf("partial rerun = %+v, want [lint]", partial.ReRun)
	}
}

// TestCheckMalformedProvenanceStales pins the fail-safe input edge:
// provenance that cannot name a scope never evaluates current.
func TestCheckMalformedProvenanceStales(t *testing.T) {
	root := t.TempDir()
	row := validation.Evidence{ID: "EVD-X", Profile: "test", SnapshotHash: "abc", Provenance: "not-json"}

	verdicts, _, err := validation.Check(root, []validation.Evidence{row}, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshStale {
		t.Fatalf("malformed provenance = %+v", verdicts[0])
	}
	if _, _, err := validation.Check("relative", nil, nil); err == nil {
		t.Fatal("relative root accepted")
	}
	escaped := validation.Evidence{ID: "EVD-E", Profile: "test", SnapshotHash: "abc",
		Provenance: provenanceForTest(t, "test", []string{filepath.Join(root, "..", "outside")})}
	verdicts, _, err = validation.Check(root, []validation.Evidence{escaped}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshStale {
		t.Fatalf("escaped scope = %+v", verdicts[0])
	}
}

// TestCheckReadOnlyPinsNoWrites is TASK-01 AC-01.5 on the letter: the
// evidence table's full contents are identical before and after Check.
func TestCheckReadOnlyPinsNoWrites(t *testing.T) {
	fx := newEvidenceFixture(t)
	redactor, err := validation.NewRedactor(nil)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	snapshot, err := validation.SnapshotScope(root, []string{"tests"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.Record(t.Context(), "test", "AUTOMATED_TEST", []string{"echo", "hi"},
		validation.Result{Status: validation.StatusPass}, snapshot.Hash,
		provenanceForTest(t, "test", snapshot.Scope), "OP-RO", redactor); err != nil {
		t.Fatal(err)
	}
	dump := func() string {
		t.Helper()
		rows, err := fx.db.DB.QueryContext(t.Context(), `SELECT evidence_id, profile, type,
			command_argv, status, exit_code, output, snapshot_hash, provenance,
			created_at, operation_id, request_hash FROM evidence ORDER BY evidence_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var sb strings.Builder
		for rows.Next() {
			var id, profile, typ, argv, status, output, snap, prov, created, op, hash string
			var code int
			if err := rows.Scan(&id, &profile, &typ, &argv, &status, &code, &output,
				&snap, &prov, &created, &op, &hash); err != nil {
				t.Fatal(err)
			}
			sb.WriteString(id + profile + typ + argv + status + output + snap + prov + created + op + hash)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}
	var countBefore int
	if err := fx.db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM evidence`).Scan(&countBefore); err != nil {
		t.Fatal(err)
	}
	before := dump()
	rows := []validation.Evidence{{
		ID: "EVD-1", Profile: "test", SnapshotHash: snapshot.Hash,
		Provenance: provenanceForTest(t, "test", snapshot.Scope),
	}}
	if _, _, err := validation.Check(root, rows, []string{"test"}); err != nil {
		t.Fatal(err)
	}
	var countAfter int
	if err := fx.db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM evidence`).Scan(&countAfter); err != nil {
		t.Fatal(err)
	}
	if countBefore != countAfter || before != dump() {
		t.Fatal("Check wrote to evidence")
	}
}

// TestCheckIsDeterministic pins the no-storage rule behaviorally: Check
// takes values and touches no database, so repeated checks agree exactly.
func TestCheckIsDeterministic(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	row := evidenceRow(t, "EVD-1", "test", root, []string{"tests"})

	firstV, firstC, err := validation.Check(root, []validation.Evidence{row}, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	secondV, secondC, err := validation.Check(root, []validation.Evidence{row}, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstV, secondV) || !reflect.DeepEqual(firstC, secondC) {
		t.Fatalf("check is not deterministic:\n%+v\n%+v", firstV, secondV)
	}
}

// TestCheckDedupesScopesAndCleansRoots pins the Breaker-divergence guards:
// duplicated scope paths hash like SnapshotScope (which dedupes at record),
// and a trailing-slash root checks like its clean form.
func TestCheckDedupesScopesAndCleansRoots(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	row := evidenceRow(t, "EVD-1", "test", root, []string{"tests"})
	dup := row
	dup.ID = "EVD-2"
	dup.Provenance = provenanceForTest(t, "test", append(append([]string{}, rowScope(t, row)...), rowScope(t, row)...))

	for _, checkRoot := range []string{root, root + "/"} {
		verdicts, _, err := validation.Check(checkRoot, []validation.Evidence{row, dup}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, verdict := range verdicts {
			if verdict.Status != validation.FreshCurrent {
				t.Fatalf("root %q verdict = %+v", checkRoot, verdict)
			}
		}
	}
}

// TestCheckEmptyScopeNeedsEmptyHash pins the vacuous-current edge: an
// empty scope is current only beside the hash of nothing.
func TestCheckEmptyScopeNeedsEmptyHash(t *testing.T) {
	root := t.TempDir()
	empty := validation.Evidence{ID: "EVD-E", Profile: "test",
		SnapshotHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Provenance:   provenanceForTest(t, "test", []string{})}
	junk := empty
	junk.ID = "EVD-J"
	junk.SnapshotHash = "junk"

	verdicts, _, err := validation.Check(root, []validation.Evidence{empty, junk}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshCurrent {
		t.Fatalf("recorded-empty = %+v", verdicts[0])
	}
	if verdicts[1].Status != validation.FreshStale {
		t.Fatalf("malformed-empty = %+v", verdicts[1])
	}
}

// TestCheckStaleReasonNamesSnapshot pins the non-vacuous gap: a content
// change names the snapshot it differs from instead of an empty path.
func TestCheckStaleReasonNamesSnapshot(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	row := evidenceRow(t, "EVD-1", "test", root, []string{"tests"})
	writeScopeFile(t, root, "tests/a.py", "print(2)\n")

	verdicts, _, err := validation.Check(root, []validation.Evidence{row}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshStale {
		t.Fatalf("verdict = %+v", verdicts[0])
	}
	if !strings.Contains(verdicts[0].Reason, row.SnapshotHash[:12]) {
		t.Fatalf("reason = %q, want the snapshot named", verdicts[0].Reason)
	}
}

func rowScope(t *testing.T, row validation.Evidence) []string {
	t.Helper()
	var decoded struct {
		Scope []string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(row.Provenance), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded.Scope
}
