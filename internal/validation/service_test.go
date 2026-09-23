package validation_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/validation"
)

// TestRunProfileFlowEndToEnd is TASK-02 AC-02.5: profile → run → redact →
// snapshot → store in one flow. A leaked secret arrives redacted in the
// stored row, the snapshot hash reproduces from the tree, and the same
// operation id replays instead of duplicating.
func TestRunProfileFlowEndToEnd(t *testing.T) {
	t.Setenv("MR010_FLOW_SECRET", "flow-secret-1")
	fx := newEvidenceFixture(t)
	runner, err := validation.NewRunner(t.TempDir(), 10*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	store := fx.store
	service, err := validation.NewService(runner, store)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "a.py"), []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := config.ValidationProfile{
		Type:  "AUTOMATED_TEST",
		Paths: []string{"tests"},
		Commands: [][]string{
			{"echo", "leaked flow-secret-1"},
			{"echo", "clean"},
		},
	}

	first, err := service.RunProfile(t.Context(), "test", profile, root,
		[]string{"MR010_FLOW_SECRET"}, "OP-FLOW")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("rows = %d, want 2", len(first))
	}
	if first[0].Status != validation.StatusPass || first[0].Output != "leaked [REDACTED]\n" {
		t.Fatalf("row = %+v", first[0])
	}
	if first[1].Output != "clean\n" {
		t.Fatalf("row = %+v", first[1])
	}
	want, err := validation.SnapshotScope(root, []string{"tests"})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].SnapshotHash != want.Hash {
		t.Fatalf("snapshot = %q, want %q", first[0].SnapshotHash, want.Hash)
	}
	second, err := service.RunProfile(t.Context(), "test", profile, root,
		[]string{"MR010_FLOW_SECRET"}, "OP-FLOW")
	if err != nil {
		t.Fatal(err)
	}
	if second[0].ID != first[0].ID || second[1].ID != first[1].ID {
		t.Fatalf("replay duplicated rows: %+v vs %+v", second, first)
	}
	if _, err := service.RunProfile(t.Context(), "", profile, root, nil, ""); err == nil {
		t.Fatal("nameless profile accepted")
	}
}
