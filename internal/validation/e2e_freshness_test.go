package validation_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/validation"
)

// TestFreshnessLifecycleEndToEnd is TASK-02 AC-02.1. One real tree through
// the whole lifecycle: run → current; relevant edit → stale with named
// re-run and required unsatisfied; unrelated edit → current; second run →
// current again with both rows coexisting (append-only visible in the
// table, not just in memory).
func TestFreshnessLifecycleEndToEnd(t *testing.T) {
	fx := newEvidenceFixture(t)
	runner, err := validation.NewRunner(t.TempDir(), 10*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	service, err := validation.NewService(runner, fx.store)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeScopeFile(t, root, "tests/a.py", "print(1)\n")
	profile := config.ValidationProfile{
		Type:     "AUTOMATED_TEST",
		Paths:    []string{"tests"},
		Commands: [][]string{{"echo", "hi"}},
	}

	first, err := service.RunProfile(t.Context(), "test", profile, root, nil, "OP-E2E")
	if err != nil {
		t.Fatal(err)
	}
	verdicts, coverage, err := validation.Check(root, first, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 1 || verdicts[0].Status != validation.FreshCurrent {
		t.Fatalf("fresh run = %+v", verdicts)
	}
	if !coverage.Satisfied["test"] {
		t.Fatalf("coverage = %+v", coverage)
	}

	writeScopeFile(t, root, "tests/a.py", "print(2)\n")
	verdicts, coverage, err = validation.Check(root, first, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshStale {
		t.Fatalf("edited run = %+v", verdicts[0])
	}
	if verdicts[0].Reason == "" {
		t.Fatal("stale reason empty")
	}
	if len(coverage.ReRun) != 1 || coverage.ReRun[0].Profile != "test" {
		t.Fatalf("rerun = %+v", coverage.ReRun)
	}
	if coverage.Satisfied["test"] {
		t.Fatalf("stale satisfies required: %+v", coverage)
	}

	if err := os.MkdirAll(filepath.Join(root, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeScopeFile(t, root, "other/z.py", "print(9)\n")
	verdicts, _, err = validation.Check(root, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Status != validation.FreshStale {
		t.Fatalf("unrelated edit cleared staleness: %+v", verdicts[0])
	}

	second, err := service.RunProfile(t.Context(), "test", profile, root, nil, "OP-E2E-2")
	if err != nil {
		t.Fatal(err)
	}
	verdicts, coverage, err = validation.Check(root, append(first, second...), []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 2 {
		t.Fatalf("verdicts = %+v", verdicts)
	}
	if verdicts[0].Status != validation.FreshStale || verdicts[1].Status != validation.FreshCurrent {
		t.Fatalf("verdicts = %+v, want stale then current", verdicts)
	}
	if !coverage.Satisfied["test"] {
		t.Fatalf("coverage = %+v", coverage)
	}
	var rows int
	if err := fx.db.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM evidence`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("evidence rows = %d, want both coexisting", rows)
	}
}
