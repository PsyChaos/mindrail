package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/mcp"
	schemas "github.com/PsyChaos/mindrail/schemas"
)

func loadKnowledge(t *testing.T, root string) loader.Store {
	t.Helper()
	fsRoot, err := filesystem.NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatal(err)
	}
	store, err := loader.New(fsRoot, reg).Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// TestDecideRoundTrips is TASK-02 AC-02.1: validate + persist + loader
// read-back with filename==id, and ids allocating max+1.
func TestDecideRoundTrips(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)

	first := callTool(t, server, "probe", mcp.ToolDecide, map[string]any{
		"title": "Ship the gate", "decision": "Gate on current evidence.",
	})
	if first["id"] != "DEC-0002" {
		t.Fatalf("id = %+v, want DEC-0002", first)
	}
	if first["path"] != "knowledge/decisions/DEC-0002.json" {
		t.Fatalf("path = %+v", first)
	}
	store := loadKnowledge(t, root)
	found := false
	for _, ref := range store.Decisions {
		if ref.ID == "DEC-0002" {
			found = true
		}
	}
	if !found {
		t.Fatalf("loader decisions = %+v", store.Decisions)
	}
	second := callTool(t, server, "probe", mcp.ToolDecide, map[string]any{
		"title": "Again", "decision": "More.",
	})
	if second["id"] != "DEC-0003" {
		t.Fatalf("id = %+v, want DEC-0003", second)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolDecide,
		map[string]any{"title": "", "decision": "s3cr3t-decision-body"}); err == nil {
		t.Fatal("empty title accepted")
	}
}

// TestInvariantActiveRoundTrips is TASK-02 AC-02.2: mode=active persists
// readable-back; candidate and empty modes refuse loudly.
func TestInvariantActiveRoundTrips(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)

	made := callTool(t, server, "probe", mcp.ToolInvariant, map[string]any{
		"mode": "active", "statement": "Logs never carry tokens.",
		"severity": "CRITICAL", "scope_level": "PROJECT",
	})
	if made["id"] != "INV-0002" {
		t.Fatalf("id = %+v, want INV-0002", made)
	}
	store := loadKnowledge(t, root)
	found := false
	for _, ref := range store.Invariants {
		if ref.ID == "INV-0002" {
			found = true
		}
	}
	if !found {
		t.Fatalf("loader invariants = %+v", store.Invariants)
	}
	candidate := callTool(t, server, "probe", mcp.ToolInvariant, map[string]any{
		"mode": "candidate", "statement": "Maybe.",
		"severity": "LOW", "scope_level": "PROJECT",
	})
	refusal, ok := candidate["refusal"].(map[string]any)
	if !ok || refusal["code"] != "NOT_IMPLEMENTED_IN_THIS_VERSION" {
		t.Fatalf("candidate = %+v, want loud refusal", candidate)
	}
	actions, ok := refusal["next_action"].([]any)
	if !ok || len(actions) == 0 {
		t.Fatalf("refusal without next_action: %+v", refusal)
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolInvariant, map[string]any{
		"mode": "", "statement": "s",
	}); err == nil {
		t.Fatal("empty mode accepted")
	}
	if err := callToolRaw(t, server, "probe", mcp.ToolInvariant, map[string]any{
		"mode": "active", "statement": "s",
	}); err == nil {
		t.Fatal("missing severity/scope accepted")
	}
}

// TestDiagnosticsEchoNamesOnly is TASK-02 AC-02.5: tool diagnostics name
// parameters, never the values callers sent.
func TestDiagnosticsEchoNamesOnly(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)

	err := callToolRaw(t, server, "probe", mcp.ToolDecide,
		map[string]any{"title": "", "decision": "s3cr3t-decision-body"})
	if err == nil {
		t.Fatal("empty title accepted")
	}
	if strings.Contains(err.Error(), "s3cr3t-decision-body") {
		t.Fatalf("diagnostic echoes values: %v", err)
	}
	candidate := callTool(t, server, "probe", mcp.ToolInvariant, map[string]any{
		"mode": "candidate", "statement": "s3cr3t-statement",
		"severity": "LOW", "scope_level": "PROJECT",
	})
	raw, _ := json.Marshal(candidate)
	if strings.Contains(string(raw), "s3cr3t-statement") {
		t.Fatalf("refusal echoes values: %s", raw)
	}
}
