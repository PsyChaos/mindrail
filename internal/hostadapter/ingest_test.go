package hostadapter

import (
	"encoding/json"
	"testing"

	"github.com/PsyChaos/mindrail/internal/agent"
)

func TestTranslateClaudeStatusLineUsesDocumentedFields(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"session_id":"session-1","agent":{"name":"Build"},"model":{"id":"claude-sonnet-4-6"},"effort":{"level":"high"},"context_window":{"context_window_size":1000000,"current_usage":{"input_tokens":400000,"cache_creation_input_tokens":100000,"cache_read_input_tokens":50000}}}`), &payload); err != nil {
		t.Fatal(err)
	}
	event, runKey, err := translate("claude", "statusline", payload)
	if err != nil {
		t.Fatal(err)
	}
	if event.Host != agent.HostClaudeCode || event.Event != agent.HostEventTelemetry || event.AgentType != "Build" || event.ModelKey != "claude-sonnet-4-6" ||
		event.ContextUsed == nil || *event.ContextUsed != 550000 || event.ContextLimit == nil || *event.ContextLimit != 1000000 || runKey != "" {
		t.Fatalf("event=%#v run=%q", event, runKey)
	}
}

func TestTranslatePreToolUseCarriesOnlyRunBinding(t *testing.T) {
	payload := map[string]any{"session_id": "session-1", "hook_event_name": "PreToolUse", "tool_input": map[string]any{"run_key": "run-1"}}
	event, runKey, err := translate("claude", "hook", payload)
	if err != nil || event.Event != agent.HostEventActivity || runKey != "run-1" {
		t.Fatalf("event=%#v run=%q err=%v", event, runKey, err)
	}
}

func TestTranslateClaudeResumeAndCompactKeepCurrentGeneration(t *testing.T) {
	for _, source := range []string{"resume", "compact"} {
		event, _, err := translate("claude", "hook", map[string]any{
			"session_id": "session-1", "hook_event_name": "SessionStart", "source": source,
		})
		if err != nil || event.Event != agent.HostEventActivity {
			t.Fatalf("source=%s event=%#v err=%v", source, event, err)
		}
	}
}
