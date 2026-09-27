package mcp_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/mcp"
)

func cleanCompletionRepo(t *testing.T) string {
	t.Helper()
	root := newTestRepo(t)
	gitCommitFile(t, root, ".")
	return root
}

// Knowledge must be read at completion, not only at server startup. A
// refused record must never vanish from the safety decision.
func TestCompleteRefusesBrokenKnowledge(t *testing.T) {
	for _, test := range []struct {
		name, body, code string
		unreadable       bool
	}{
		{"unsupported", `{"schema_version":999,"kind":"invariant","id":"INV-0001"}`, "KNOWLEDGE_SCHEMA_UNSUPPORTED", false},
		{"invalid_json", `{`, "KNOWLEDGE_UNREADABLE", false},
		{"missing_version", `{}`, "KNOWLEDGE_UNREADABLE", false},
		{"invalid_schema", strings.Replace(testInvariant, `"HIGH"`, `"not-a-severity"`, 1), "KNOWLEDGE_INVALID", false},
		{"unreadable_record", "", "KNOWLEDGE_UNREADABLE", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := cleanCompletionRepo(t)
			server := newTestServer(t, root)
			coord, db := coordinationStore(t, root)
			workspaceID, projectID := workspaceOf(t, db)
			taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
			args := map[string]any{"task_id": taskID}
			if got := callTool(t, server, "probe", mcp.ToolComplete, args); got["allow"] != true {
				t.Fatalf("valid control = %+v", got)
			}
			rel := ".mindrail/knowledge/invariants/INV-0001.json"
			path := filepath.Join(root, filepath.FromSlash(rel))
			if test.unreadable {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "missing-record.json"), path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte(test.body), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, reader := range []*mcp.Server{server, newTestServer(t, root)} {
				for range 2 {
					err := callToolRaw(t, reader, "probe", mcp.ToolComplete, args)
					if err == nil || !strings.Contains(err.Error(), test.code) || !strings.Contains(err.Error(), rel) {
						t.Fatalf("completion must refuse with %s and path %s: %v", test.code, rel, err)
					}
				}
			}
			if test.unreadable {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, []byte(testInvariant), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := callTool(t, server, "probe", mcp.ToolComplete, args); got["allow"] != true {
				t.Fatalf("repaired knowledge = %+v", got)
			}
		})
	}
}

func TestCompleteReconcilesSkippedProtocol(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
	args := map[string]any{"task_id": taskID}
	if got := callTool(t, server, "probe", mcp.ToolComplete, args); got["allow"] != true {
		t.Fatalf("clean control = %+v", got)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.py"), []byte("def helper():\n    return 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := callTool(t, server, "probe", mcp.ToolComplete, args)
	codes := denialCodes(t, got)
	if got["allow"] != false || !slices.Contains(codes, "SCOPE_DRIFT") || !slices.Contains(codes, "UNREGISTERED_CHANGE") {
		t.Fatalf("unregistered edit without protocol calls = %+v", got)
	}
	callTool(t, server, "probe", mcp.ToolReconcile, args)
	if explicit := callTool(t, server, "probe", mcp.ToolComplete, args); !reflect.DeepEqual(explicit, got) {
		t.Fatalf("implicit/explicit reconciliation differ: %+v vs %+v", got, explicit)
	}
}

func TestCompleteReconcilesDeclaredEditWithoutAfterChange(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
	path := filepath.Join(root, "pkg", "a.py")
	callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{"task_id": taskID, "paths": []string{path}})
	if err := os.WriteFile(path, []byte("def helper():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if got := callTool(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID}); got["allow"] != true {
			t.Fatalf("honest declared edit = %+v", got)
		}
	}
	var discovered int
	if err := db.QueryRow(`SELECT count(*) FROM change_symbols`).Scan(&discovered); err != nil || discovered == 0 {
		t.Fatalf("completion skipped discovery: symbols=%d error=%v", discovered, err)
	}
}

func TestCompleteRefusesFailedReconcile(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
	gitDir := filepath.Join(root, ".git")
	hidden := filepath.Join(t.TempDir(), "git")
	if err := os.Rename(gitDir, hidden); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Rename(hidden, gitDir); err != nil {
			t.Error(err)
		}
	}()
	if err := callToolRaw(t, server, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID}); err == nil {
		t.Fatal("completion certified a repository Git could not read")
	}
}

func TestCompleteAllowsHonestUnprotectedTestEdit(t *testing.T) {
	root := cleanCompletionRepo(t)
	path := filepath.Join(root, "pkg", "test_plain.py")
	writeScopeFile(t, root, "pkg/test_plain.py", "def test_plain():\n    assert 1\n")
	gitCommitFile(t, root, "pkg/test_plain.py")
	indexTestFile(t, root, filepath.Join(root, "pkg"), path)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
	callTool(t, server, "probe", mcp.ToolBeforeChange, map[string]any{"task_id": taskID, "paths": []string{path}})
	writeScopeFile(t, root, "pkg/test_plain.py", "def test_plain():\n    assert 2\n")
	for _, reader := range []*mcp.Server{server, server, newTestServer(t, root)} {
		if out := callTool(t, reader, "probe", mcp.ToolComplete, map[string]any{"task_id": taskID}); out["allow"] != true {
			t.Fatalf("honest unprotected test edit = %+v", out)
		}
	}
}
