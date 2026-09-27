package changes_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/perf"
)

// TestAfterChangeObservesBreakdowns pins the telemetry wiring: a fresh
// file synced through AfterChange banks parse and sqlite-wait segments on
// the ambient timer. Presence is asserted, never values — durations are
// observed, not graded, in unit tests.
func TestAfterChangeObservesBreakdowns(t *testing.T) {
	fx := newServiceFixture(t)
	abs := filepath.Join(fx.root, "py", "w.py")
	if err := os.WriteFile(abs, []byte("def w():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedTasks(t, fx.db, "TSK-W")
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-W", []string{abs}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("def w():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	timer := perf.Start(app.SystemClock{})
	if _, err := fx.service.AfterChange(perf.WithTimer(t.Context(), timer), changeProject, fx.root, "TSK-W", ""); err != nil {
		t.Fatal(err)
	}
	sample := timer.Stop()
	for _, part := range []string{perf.Parse, perf.SQLiteWait} {
		if _, ok := sample.Breakdowns[part]; !ok {
			t.Fatalf("breakdowns = %v, want %q observed", sample.Breakdowns, part)
		}
	}
}

// TestObserveWithoutTimerIsSilent pins the observer contract: service
// calls without an ambient timer behave exactly as before — no timer,
// no observation, no failure.
func TestObserveWithoutTimerIsSilent(t *testing.T) {
	fx := newServiceFixture(t)
	abs := filepath.Join(fx.root, "py", "q.py")
	if err := os.WriteFile(abs, []byte("def q():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedTasks(t, fx.db, "TSK-Q")
	if _, err := fx.store.CaptureBaseline(t.Context(), "TSK-Q", []string{abs}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("def q():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	change, err := fx.service.AfterChange(t.Context(), changeProject, fx.root, "TSK-Q", "")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := fx.store.ReadChangeSymbols(t.Context(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("symbols = %d, want q", len(rows))
	}
}
