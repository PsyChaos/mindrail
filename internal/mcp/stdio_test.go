package mcp_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestStdioDiscoversThirteenTools is TASK-02 AC-02.4: one client session
// discovers all thirteen tools over the JSON-framed byte-stream transports
// (the same newIOConn framing stdio serving uses — net.Pipe pairs, no
// subprocess per decision D-209) and smoke-calls every one.
func TestStdioDiscoversThirteenTools(t *testing.T) {
	root := newProfileRepo(t)
	server := newTestServer(t, root)

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
		"mindrail_bootstrap": {},
		"mindrail_status":    {},
		"mindrail_search":    {"query": "auth"},
		"mindrail_context":   {},
		"mindrail_validate":  {"profile": "test"},
		"mindrail_complete":  {"task_id": "TSK-NOPE"},
	}
	for tool, args := range smokes {
		result, err := clientSession.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			t.Fatalf("%s transport error: %v", tool, err)
		}
		if result.IsError && tool != "mindrail_complete" {
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
