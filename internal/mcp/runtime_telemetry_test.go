package mcp

import (
	"strings"
	"testing"
)

func TestDecodeRuntimeTelemetryAcceptsBoundedStrictShape(t *testing.T) {
	input, ok := decodeRuntimeTelemetry(map[string]any{
		"model": "gpt-6-astra", "effort": "high",
		"context":  map[string]any{"used": 600_000, "limit": 1_000_000},
		"sequence": 9, "observed_at": "2026-09-28T11:00:21Z",
	})
	if !ok || input.ModelKey != "gpt-6-astra" || input.Effort != "high" ||
		input.ContextUsed == nil || *input.ContextUsed != 600_000 || *input.ContextLimit != 1_000_000 ||
		input.ProducerSequence != 9 || input.ObservedAt.IsZero() {
		t.Fatalf("input=%#v ok=%v", input, ok)
	}
}

func TestDecodeRuntimeTelemetryDropsMalformedWithoutBlockingTool(t *testing.T) {
	cases := []any{
		map[string]any{"model": "gpt-6-astra", "unknown": true},
		map[string]any{"context": map[string]any{"used": 1, "limit": 2, "extra": 3}},
		map[string]any{"context": map[string]any{"used": 600_000, "limit": 1_000_000}},
		map[string]any{"context": map[string]any{"used": 600_000, "limit": 1_000_000}, "sequence": 1},
		map[string]any{"model": strings.Repeat("x", runtimeTelemetryMaxBytes+1)},
		func() {},
	}
	for _, raw := range cases {
		if input, ok := decodeRuntimeTelemetry(raw); ok {
			t.Fatalf("accepted %#v as %#v", raw, input)
		}
	}
}
