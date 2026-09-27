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
// re-run and required unsatisfied; unrelated edit preserves the stale
// verdict down to its hash (proving the verdict's basis is the row's own
// scope, not the whole tree); second run → current again with both rows
// coexisting (append-only visible in the table, not just in memory).
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
	staleHash := verdicts[0].CurrentHash

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
	if verdicts[0].CurrentHash != staleHash {
		t.Fatalf("unrelated edit moved the verdict hash: %q vs %q",
			verdicts[0].CurrentHash, staleHash)
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

// TestProfileRunMustBeWhollySuccessful covers REQ-001 through the public
// service/check seam. Only the latest complete run may satisfy a required
// profile; fail, spawn error, and a mixed pass/fail run must not. A later
// wholly successful run recovers without deleting append-only history.
func TestProfileRunMustBeWhollySuccessful(t *testing.T) {
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
	run := func(operation string, commands ...[]string) []validation.Evidence {
		t.Helper()
		rows, err := service.RunProfile(t.Context(), "test", config.ValidationProfile{
			Type: "AUTOMATED_TEST", Paths: []string{"tests"}, Commands: commands,
		}, root, nil, operation)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	assertSatisfied := func(name string, rows []validation.Evidence, want bool) {
		t.Helper()
		_, coverage, err := validation.Check(root, rows, []string{"test"})
		if err != nil {
			t.Fatal(err)
		}
		if got := coverage.Satisfied["test"]; got != want {
			t.Fatalf("%s satisfied = %v, want %v; coverage=%+v rows=%+v", name, got, want, coverage, rows)
		}
	}

	pass := run("OP-PASS", []string{"true"})
	assertSatisfied("pass", pass, true)

	failed := run("OP-FAIL", []string{"false"})
	assertSatisfied("failed latest run", append(pass, failed...), false)

	spawnError := run("OP-ERROR", []string{"mindrail-no-such-validation-command"})
	assertSatisfied("spawn error", spawnError, false)

	mixed := run("OP-MIXED", []string{"true"}, []string{"false"})
	assertSatisfied("mixed pass/fail", mixed, false)

	incomplete := run("OP-INCOMPLETE", []string{"true"}, []string{"true"})
	assertSatisfied("missing command row", incomplete[:1], false)

	recovered := run("OP-RECOVERED", []string{"true"}, []string{"true"})
	assertSatisfied("successful rerun", append(append(failed, mixed...), recovered...), true)
}

// TestDeclaredScopeMembershipChangesStaleEvidence covers REQ-002. Directory
// and recursive-glob declarations are re-enumerated; add/edit/remove within
// them stales evidence, while a sibling outside the declaration does not.
func TestDeclaredScopeMembershipChangesStaleEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{name: "directory", paths: []string{"tests"}},
		{name: "recursive glob", paths: []string{"tests/**/*.py"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			writeScopeFile(t, root, "tests/unit/a.py", "print(1)\n")
			profile := config.ValidationProfile{
				Type: "AUTOMATED_TEST", Paths: tc.paths, Commands: [][]string{{"true"}},
			}
			rows, err := service.RunProfile(t.Context(), "test", profile, root, nil, "OP-SCOPE-"+tc.name)
			if err != nil {
				t.Fatal(err)
			}
			assertFreshness := func(stage, want string) {
				t.Helper()
				verdicts, _, err := validation.Check(root, rows, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(verdicts) != 1 || verdicts[0].Status != want {
					t.Fatalf("%s verdicts = %+v, want %s", stage, verdicts, want)
				}
			}

			assertFreshness("untouched", validation.FreshCurrent)
			writeScopeFile(t, root, "outside/z.py", "print(9)\n")
			assertFreshness("outside add", validation.FreshCurrent)
			if tc.name == "recursive glob" {
				writeScopeFile(t, root, "tests/unit/ignored.txt", "outside the glob\n")
				assertFreshness("inside directory but outside glob", validation.FreshCurrent)
			}
			writeScopeFile(t, root, "tests/unit/added.py", "assert False\n")
			assertFreshness("inside add", validation.FreshStale)

			if err := os.Remove(filepath.Join(root, "tests", "unit", "added.py")); err != nil {
				t.Fatal(err)
			}
			writeScopeFile(t, root, "tests/unit/a.py", "print(2)\n")
			assertFreshness("inside edit", validation.FreshStale)
			if err := os.Remove(filepath.Join(root, "tests", "unit", "a.py")); err != nil {
				t.Fatal(err)
			}
			assertFreshness("inside remove", validation.FreshStale)
		})
	}
}

func TestZeroMatchDeclaredScopeIsCurrentUntilMembershipChanges(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{name: "empty literal directory", paths: []string{"empty"}},
		{name: "zero-match recursive glob", paths: []string{"empty/**/*.py"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
				t.Fatal(err)
			}
			snapshot, err := validation.SnapshotScope(root, tc.paths)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Scope == nil || len(snapshot.Scope) != 0 {
				t.Fatalf("empty declaration scope = %#v, want non-nil empty slice", snapshot.Scope)
			}
			rows, err := service.RunProfile(t.Context(), "test", config.ValidationProfile{
				Type: "AUTOMATED_TEST", Paths: tc.paths, Commands: [][]string{{"true"}},
			}, root, nil, "OP-ZERO-"+tc.name)
			if err != nil {
				t.Fatal(err)
			}
			verdicts, coverage, err := validation.Check(root, rows, []string{"test"})
			if err != nil {
				t.Fatal(err)
			}
			if len(verdicts) != 1 || verdicts[0].Status != validation.FreshCurrent || !coverage.Satisfied["test"] {
				t.Fatalf("zero-match run = verdicts %#v coverage %#v", verdicts, coverage)
			}

			writeScopeFile(t, root, "empty/added.py", "assert False\n")
			verdicts, coverage, err = validation.Check(root, rows, []string{"test"})
			if err != nil {
				t.Fatal(err)
			}
			if verdicts[0].Status != validation.FreshStale || coverage.Satisfied["test"] {
				t.Fatalf("membership addition = verdicts %#v coverage %#v", verdicts, coverage)
			}
		})
	}
}
