package mcp_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestStdioDiscoversThirteenTools is TASK-02 AC-02.4: one client session
// discovers all thirteen tools over the JSON-framed byte-stream transports
// (net.Pipe pairs through the SDK's own framing — the wire encoding stdio
// serving uses, without a subprocess per decision D-209) and smoke-calls
// every one with valid arguments.
func TestStdioDiscoversThirteenTools(t *testing.T) {
	root := newProfileRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, session := openTaskAndSession(t, coord, workspaceID, projectID)

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.SDK().Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "stdio-probe", Version: "v0.0.1"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	listed, err := clientSession.ListTools(t.Context(), &sdk.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		if tool.Description == "" {
			t.Fatalf("tool %q without description", tool.Name)
		}
	}
	sort.Strings(names)
	want := []string{"mindrail_after_change", "mindrail_before_change", "mindrail_bootstrap",
		"mindrail_checkpoint", "mindrail_claim", "mindrail_complete", "mindrail_context",
		"mindrail_decide", "mindrail_invariant", "mindrail_reconcile", "mindrail_search",
		"mindrail_status", "mindrail_validate"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tools = %+v", names)
	}
	smokes := map[string]map[string]any{
		"mindrail_bootstrap":     {},
		"mindrail_status":        {},
		"mindrail_search":        {"query": "auth"},
		"mindrail_context":       {},
		"mindrail_validate":      {"profile": "test"},
		"mindrail_claim":         {"task_id": taskID, "session": session},
		"mindrail_before_change": {"task_id": taskID, "paths": []string{filepath.Join(root, "tests", "t.py")}},
		"mindrail_after_change":  {"task_id": taskID},
		"mindrail_reconcile":     {"task_id": taskID},
		"mindrail_checkpoint":    {"task_id": taskID, "session": session, "note": "smoke"},
		"mindrail_decide":        {"title": "smoke", "decision": "smoke"},
		"mindrail_invariant":     {"mode": "active", "statement": "Smoke holds.", "severity": "LOW", "scope_level": "PROJECT"},
		"mindrail_complete":      {"task_id": taskID},
	}
	for tool, args := range smokes {
		result, err := clientSession.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			t.Fatalf("%s transport error: %v", tool, err)
		}
		if result.IsError {
			t.Fatalf("%s errored: %+v", tool, result.Content)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s unmarshal: %v", tool, err)
		}
	}
}
