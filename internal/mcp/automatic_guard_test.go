package mcp_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/mcp"
)

func TestAutomaticGuardBaselinePreservesProtectedTestRemoval(t *testing.T) {
	for _, lateBinding := range []bool{false, true} {
		for _, discovery := range []string{"finalize-only", mcp.ToolAfterChange, mcp.ToolReconcile} {
			t.Run(fmt.Sprintf("late-binding=%t/%s", lateBinding, discovery), func(t *testing.T) {
				root := cleanCompletionRepo(t)
				prod := filepath.Join(root, "pkg", "a.py")
				test := filepath.Join(root, "tests", "test_a.py")
				writeScopeFile(t, root, "tests/test_a.py", "def test_helper():\n    assert helper()\n")
				gitCommitFile(t, root, "tests/test_a.py")
				server := newTestServer(t, root)
				coord, db := coordinationStore(t, root)
				prodUID := indexFile(t, db, filepath.Join(root, "pkg"), prod)
				indexPath(t, db, filepath.Join(root, "tests"), test)
				seedResolvedReference(t, db, test, indexFileKey(t, db, test, "test_helper"), prodUID)
				bind := func() {
					seedBinding(t, db, "INV-0002", prodUID, "bound")
					writeInvariantFile(t, root, "INV-0002", "CRITICAL")
				}
				if !lateBinding {
					bind()
				}
				client := connectPersistent(t, server, "automatic-guard")
				started := client.call(t, mcp.ToolBootstrap, map[string]any{
					"goal": "preserve verification proof", "run_key": "guard", "paths": []string{"tests/test_a.py"},
				})
				if started["started"] != true {
					t.Fatalf("bootstrap: %+v", started)
				}
				if lateBinding {
					// Protection discovered after bootstrap must be captured before
					// discovery replaces the indexed test's resolved references.
					bind()
				}
				writeScopeFile(t, root, "tests/test_a.py", "def test_other():\n    assert True\n")
				if discovery != "finalize-only" {
					client.call(t, discovery, map[string]any{})
				}
				for range 2 {
					out := client.call(t, mcp.ToolComplete, map[string]any{"finalize": true})
					if out["completed"] == true || out["allow"] != false || !slices.Contains(denialCodes(t, out), "TEST_GUARD_WEAKENED") {
						t.Fatalf("protected test removal escaped automatic gate: %+v", out)
					}
				}
				task, err := coord.FindTask(t.Context(), started["task_id"].(string))
				if err != nil || task.State != coordination.StateInProgress {
					t.Fatalf("denial changed task state: %+v %v", task, err)
				}
			})
		}
	}
}
