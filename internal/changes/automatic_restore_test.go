package changes_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/changes"
)

func TestAutomaticReconcileRetiresStaleDirectoryChangeRow(t *testing.T) {
	f := newServiceFixture(t)
	directory := filepath.Join(f.root, "py", "stale-agent-scope")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	change, err := f.store.EnsureOpenChange(t.Context(), "TSK-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.UpsertFileRows(t.Context(), change.ID, []changes.FileChange{{
		Path: directory, Kind: changes.FileAdded, Via: changes.ViaReconcile,
	}}); err != nil {
		t.Fatal(err)
	}

	automatic := f.service.WithAutomaticScope(changes.AutomaticScope{StartedAt: time.Now()})
	if _, err := automatic.Reconcile(t.Context(), changeProject, f.root, "TSK-1", "", reconcileRunner(t, "")); err != nil {
		t.Fatalf("reconcile stale directory row: %v", err)
	}
	rows, err := f.store.ReadChangeFiles(t.Context(), change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("stale rows = %+v, want retired", rows)
	}
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		t.Fatalf("directory was changed: info=%+v err=%v", info, err)
	}
}
