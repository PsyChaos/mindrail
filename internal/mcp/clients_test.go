package mcp_test

import (
	"encoding/json"
	"reflect"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestTwoClientsAgree is TASK-02 AC-02.4: two distinct client identities
// over separate in-memory transports see identical tool lists, schemas
// and call results against one server. Client identity never changes
// behavior (spec AC-20).
func TestTwoClientsAgree(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)

	type view struct {
		tools map[string]any
		calls map[string]map[string]any
	}
	views := map[string]view{}
	for _, identity := range []string{"client-alpha", "client-beta"} {
		clientTransport, serverTransport := sdk.NewInMemoryTransports()
		serverSession, err := server.SDK().Connect(t.Context(), serverTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer serverSession.Close()
		client := sdk.NewClient(&sdk.Implementation{Name: identity, Version: "v0.0.1"}, nil)
		clientSession, err := client.Connect(t.Context(), clientTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer clientSession.Close()

		listed, err := clientSession.ListTools(t.Context(), &sdk.ListToolsParams{})
		if err != nil {
			t.Fatal(err)
		}
		tools := map[string]any{}
		for _, tool := range listed.Tools {
			raw, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			var schema map[string]any
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			tools[tool.Name] = schema
		}
		calls := map[string]map[string]any{}
		for tool, args := range map[string]map[string]any{
			"mindrail_bootstrap": {},
			"mindrail_status":    {},
			"mindrail_search":    {"query": "auth"},
			"mindrail_context":   {"detail_level": "focused"},
		} {
			result, err := clientSession.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("%s by %s errored", tool, identity)
			}
			raw, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			delete(decoded, "duration_ms")
			calls[tool] = decoded
		}
		views[identity] = view{tools: tools, calls: calls}
	}
	if !reflect.DeepEqual(views["client-alpha"].tools, views["client-beta"].tools) {
		t.Fatal("tool schemas differ across clients")
	}
	if !reflect.DeepEqual(views["client-alpha"].calls, views["client-beta"].calls) {
		t.Fatal("call results differ across clients")
	}
	if len(views["client-alpha"].tools) != 14 {
		t.Fatalf("tools = %d, want 14", len(views["client-alpha"].tools))
	}
}
