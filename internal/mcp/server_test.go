package mcp_test

import (
	"encoding/json"
	"reflect"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/mcp"
	"github.com/PsyChaos/mindrail/internal/status"
)

// TestServerRegistersFourReadTools is TASK-01 AC-01.1: the SDK, the
// server and all six tools with derived schemas — four serving, two
// refusing stubs until TASK-02.
func TestServerRegistersFourReadTools(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)

	names := listToolNames(t, server, "probe")
	want := map[string]bool{
		mcp.ToolBootstrap: false, mcp.ToolStatus: false,
		mcp.ToolSearch: false, mcp.ToolContext: false,
		mcp.ToolDecide: false, mcp.ToolInvariant: false,
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
	for _, tool := range []string{mcp.ToolDecide, mcp.ToolInvariant} {
		args := map[string]any{"title": "t", "decision": "d"}
		if tool == mcp.ToolInvariant {
			args = map[string]any{"mode": "active", "statement": "s"}
		}
		got := callTool(t, server, "probe", tool, args)
		refusal, ok := got["refusal"].(map[string]any)
		if !ok || refusal["code"] != "NOT_IMPLEMENTED_IN_THIS_VERSION" {
			t.Fatalf("%s = %+v, want refusing stub", tool, got)
		}
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
// pinned by byte-equality with status.Build on the same subject, modulo
// the measured duration.
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
	expected := buildStatusDirectly(t, root)
	delete(status, "duration_ms")
	delete(expected, "duration_ms")
	if !reflect.DeepEqual(status, expected) {
		t.Fatal("status tool output differs from status.Build on the same subject")
	}
}

func buildStatusDirectly(t *testing.T, root string) map[string]any {
	t.Helper()
	application := bootstrap.New(bootstrap.Options{StartDir: root, Mode: bootstrap.ModeReadOnly})
	if err := application.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = application.Shutdown(t.Context()) }()
	report := status.Build(application.Subject(), 0)
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
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
