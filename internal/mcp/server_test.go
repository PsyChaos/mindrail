package mcp_test

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/mcp"
)

// TestServerRegistersFourReadTools is TASK-01 AC-01.1 at this stage: the
// SDK, the server and the four read tools with derived schemas. Decide and
// invariant complete the six in TASK-02.
func TestServerRegistersFourReadTools(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)

	names := listToolNames(t, server, "probe")
	want := map[string]bool{
		mcp.ToolBootstrap: false, mcp.ToolStatus: false,
		mcp.ToolSearch: false, mcp.ToolContext: false,
	}
	for _, name := range names {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected tool %q", name)
		}
		want[name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("tool %q not registered", name)
		}
	}
	out := callTool(t, server, "probe", mcp.ToolBootstrap, map[string]any{})
	if out["worktree_root"] == "" || out["readiness"] == "" {
		t.Fatalf("bootstrap = %+v", out)
	}
}

func listToolNames(t *testing.T, server *mcp.Server, clientName string) []string {
	t.Helper()
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.SDK().Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: clientName, Version: "v0.0.1"}, nil)
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
	}
	return names
}

// TestBootstrapAndStatusShareServices is TASK-01 AC-01.2: the status tool
// returns the CLI's report shape from the same application services —
// readiness plus the report body the CLI prints.
func TestBootstrapAndStatusShareServices(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)

	boot := callTool(t, server, "probe", mcp.ToolBootstrap, map[string]any{})
	status := callTool(t, server, "probe", mcp.ToolStatus, map[string]any{})
	if boot["readiness"] != status["readiness"] {
		t.Fatalf("bootstrap readiness %q != status readiness %q", boot["readiness"], status["readiness"])
	}
	if _, ok := status["components"]; !ok {
		t.Fatalf("status = %+v, want the CLI report shape", status)
	}
}

// TestSearchBindsRecords is TASK-01 AC-01.3: knowledge text matches and
// over-limit refuses instead of truncating.
func TestSearchBindsRecords(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)

	hits := callTool(t, server, "probe", mcp.ToolSearch, map[string]any{"query": "auth"})
	results, ok := hits["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatalf("search = %+v, want knowledge hits", hits)
	}
	symbolHits := callTool(t, server, "probe", mcp.ToolSearch, map[string]any{"query": "helper"})
	symbols, ok := symbolHits["results"].([]any)
	if !ok || len(symbols) == 0 {
		t.Fatalf("symbol search = %+v, want declaration hits", symbolHits)
	}
	first, ok := symbols[0].(map[string]any)
	if !ok || first["source"] != "symbol" {
		t.Fatalf("symbol hit = %+v", symbols[0])
	}
	limited := callTool(t, server, "probe", mcp.ToolSearch, map[string]any{"query": "e", "limit": 100})
	refusal, ok := limited["refusal"].(map[string]any)
	if !ok || refusal["code"] != "NOT_IMPLEMENTED_IN_THIS_VERSION" {
		t.Fatalf("over-limit = %+v, want loud refusal", limited)
	}
	if _, ok := refusal["next_action"]; !ok {
		t.Fatalf("refusal without next_action: %+v", refusal)
	}
	empty := callTool(t, server, "probe", mcp.ToolSearch, map[string]any{"query": "no-such-thing-xyz"})
	if results, ok := empty["results"].([]any); !ok || len(results) != 0 {
		t.Fatalf("empty search = %+v", empty)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolSearch, map[string]any{"query": "  "}); err == nil {
		t.Fatal("blank query accepted")
	}
}

// TestContextLevels is TASK-01 AC-01.4 + AC-01.5: summary and focused
// shapes, and every present-but-deferred parameter refuses loudly.
func TestContextLevels(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)

	summary := callTool(t, server, "probe", mcp.ToolContext, map[string]any{})
	if summary["readiness"] == nil || summary["decisions"] == nil {
		t.Fatalf("summary = %+v", summary)
	}
	focused := callTool(t, server, "probe", mcp.ToolContext,
		map[string]any{"detail_level": "focused"})
	if _, ok := focused["excerpts"]; !ok {
		t.Fatalf("focused = %+v, want excerpts", focused)
	}
	for name, args := range map[string]map[string]any{
		"full":         {"detail_level": "full"},
		"unknown":      {"detail_level": "panoramic"},
		"impact_group": {"target_type": "impact_group"},
		"target_id":    {"target_id": "IG-42"},
		"cursor":       {"cursor": "abc"},
		"page_size":    {"page_size": 10},
	} {
		got := callTool(t, server, "probe", mcp.ToolContext, args)
		refusal, ok := got["refusal"].(map[string]any)
		if !ok || refusal["code"] != "NOT_IMPLEMENTED_IN_THIS_VERSION" {
			t.Fatalf("%s = %+v, want loud refusal", name, got)
		}
	}
}
