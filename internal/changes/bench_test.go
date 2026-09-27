package changes_test

// Service-layer benchmarks (MR-019 TASK-01): after_change and reconcile
// driven directly, so the ambient timer reaches the service layer intact
// and parse/sqlite-wait breakdowns publish into the report. (The MCP
// transport drops context values, so transport benchmarks carry totals
// only — see the mcp benchmarks.) Graded against the same STRUCTURAL
// targets.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/perf"
)

// benchFiles writes ten small modules and returns their paths.
func benchFiles(t *testing.B, fx serviceFixture) []string {
	t.Helper()
	var paths []string
	for i := range 10 {
		abs := filepath.Join(fx.root, "py", fmt.Sprintf("b%02d.py", i))
		if err := os.WriteFile(abs, []byte(fmt.Sprintf("def b%d():\n    return 1\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, abs)
	}
	return paths
}

func BenchmarkAfterChangeSvc(b *testing.B) {
	fx := newServiceFixture(b)
	paths := benchFiles(b, fx)
	seedTasks(b, fx.db, "TSK-BENCH")
	if _, err := fx.store.CaptureBaseline(b.Context(), "TSK-BENCH", paths, ""); err != nil {
		b.Fatal(err)
	}
	var n int
	perf.Bench(b, "after_change", func(ctx context.Context) {
		n++
		if err := os.WriteFile(paths[0], []byte(fmt.Sprintf("def b0():\n    return %d\n", n)), 0o644); err != nil {
			b.Fatal(err)
		}
		if _, err := fx.service.AfterChange(ctx, changeProject, fx.root, "TSK-BENCH", ""); err != nil {
			b.Fatal(err)
		}
	})
}

func BenchmarkReconcileSvc(b *testing.B) {
	fx := newServiceFixture(b)
	paths := benchFiles(b, fx)
	seedTasks(b, fx.db, "TSK-BENCH-R")
	gitBenchInit(b, fx.root)
	gitBenchCommit(b, fx.root, "base")
	// Every iteration edits first so discovery is never empty and each
	// pass parses and writes — the breakdowns publish honestly.
	var n int
	perf.Bench(b, "reconcile", func(ctx context.Context) {
		n++
		if err := os.WriteFile(paths[0], []byte(fmt.Sprintf("def b0():\n    return %d\n", n)), 0o644); err != nil {
			b.Fatal(err)
		}
		if _, err := fx.service.Reconcile(ctx, changeProject, fx.root, "TSK-BENCH-R", "", git.NewExecRunner()); err != nil {
			b.Fatal(err)
		}
	})
}

func gitBenchInit(b *testing.B, root string) {
	b.Helper()
	if out, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		b.Fatalf("git init: %v: %s", err, out)
	}
	for _, args := range [][]string{
		{"config", "user.email", "bench@bench"},
		{"config", "user.name", "bench"},
		{"config", "commit.gpgsign", "false"},
	} {
		full := append([]string{"-C", root}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			b.Fatalf("git %v: %v: %s", full, err, out)
		}
	}
}

func gitBenchCommit(b *testing.B, root, message string) {
	b.Helper()
	if out, err := exec.Command("git", "-C", root, "add", ".").CombinedOutput(); err != nil {
		b.Fatalf("git add: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", root, "commit", "--quiet", "-m", message).CombinedOutput(); err != nil {
		b.Fatalf("git commit: %v: %s", err, out)
	}
}
