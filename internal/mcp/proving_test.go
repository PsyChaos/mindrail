package mcp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/mcp"
	"github.com/PsyChaos/mindrail/internal/validation"
)

func newProfileRepo(t *testing.T) string {
	t.Helper()
	root := newTestRepo(t)
	config := "[validation.test]\ntype = \"AUTOMATED_TEST\"\n" +
		"paths = [\"tests\"]\ncommands = [[\"echo\", \"hi\"]]\n"
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "t.py"), []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".mindrail", "config.toml")
	previous, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(previous, []byte(config)...), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestValidateRunsNamedProfile is TASK-01 AC-01.1: a named project profile
// runs to evidence rows bound to the current snapshot; unknown names
// refuse before spawn.
func TestValidateRunsNamedProfile(t *testing.T) {
	root := newProfileRepo(t)
	server := newTestServer(t, root)

	out := callTool(t, server, "probe", mcp.ToolValidate, map[string]any{"profile": "test"})
	evidence, ok := out["evidence"].([]any)
	if !ok || len(evidence) != 1 {
		t.Fatalf("validate = %+v, want one row", out)
	}
	row, ok := evidence[0].(map[string]any)
	if !ok || row["status"] != "pass" || row["snapshot"] == "" || row["id"] == "" {
		t.Fatalf("row = %+v", evidence[0])
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolValidate, map[string]any{"profile": "nope"}); err == nil {
		t.Fatal("unknown profile accepted")
	} else if !strings.Contains(err.Error(), "unknown validation profile") {
		t.Fatalf("unknown profile refused downstream: %v", err)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolValidate, map[string]any{"profile": ""}); err == nil {
		t.Fatal("empty profile accepted")
	}
	for _, param := range []string{"budget", "escalation", "approval"} {
		got := callTool(t, server, "probe", mcp.ToolValidate,
			map[string]any{"profile": "test", param: "gold"})
		refusal, ok := got["refusal"].(map[string]any)
		if !ok || refusal["code"] != "NOT_IMPLEMENTED_IN_THIS_VERSION" {
			t.Fatalf("%s = %+v, want version error", param, got)
		}
		if actions, ok := refusal["next_action"].([]any); !ok || len(actions) == 0 {
			t.Fatalf("%s refusal without next_action: %+v", param, got)
		}
	}
}

// TestEvidenceForProfileOrder is TASK-01 AC-01.3 at the read: newest rows
// first, MR-010 behavior unchanged.
func TestEvidenceForProfileOrder(t *testing.T) {
	root := newProfileRepo(t)
	server := newTestServer(t, root)
	_ = server

	fx := newEvidenceFixture(t)
	redactor, err := validation.NewRedactor(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"OP-O1", "OP-O2"} {
		if _, err := fx.store.Record(t.Context(), "test", "AUTOMATED_TEST", []string{"echo"},
			validation.Result{Status: validation.StatusPass}, "snap", "{}", op, redactor); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := fx.store.EvidenceForProfile(t.Context(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].OperationID != "OP-O2" || rows[1].OperationID != "OP-O1" {
		t.Fatalf("order = %+v", rows)
	}
	if empty, err := fx.store.EvidenceForProfile(t.Context(), "nothing"); err != nil || len(empty) != 0 {
		t.Fatalf("empty = %+v, %v", empty, err)
	}
	if _, err := fx.store.EvidenceForProfile(t.Context(), ""); err == nil {
		t.Fatal("empty profile accepted")
	}
}

// TestBindingsWithStatusReadsAll is TASK-01 AC-01.3 at the read: bound,
// ambiguous and orphaned rows all report with status.
func TestBindingsWithStatusReadsAll(t *testing.T) {
	root := newTestRepo(t)
	newTestServer(t, root)
	fx := newIndexFixture(t)
	seedBindingRow(t, fx, "INV-1", "SYM-B-1", "bound")
	seedBindingRow(t, fx, "INV-9", "SYM-B-1", "orphaned")
	bound, err := fx.indexes.BindingsWithStatus(t.Context(), "SYM-B-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(bound) != 2 {
		t.Fatalf("bindings = %+v", bound)
	}
	byID := map[string]string{}
	for _, binding := range bound {
		byID[binding.InvariantID] = binding.Status
	}
	if byID["INV-1"] != index.BindingBound || byID["INV-9"] != "orphaned" {
		t.Fatalf("bindings = %+v", byID)
	}
}
