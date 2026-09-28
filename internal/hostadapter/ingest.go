// Package hostadapter translates documented host hook/status-line payloads
// into privacy-bounded pending runtime events. It never selects a task.
package hostadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
)

const maxPayloadBytes = 64 << 10

func IngestHostEvent(ctx context.Context, host, kind, startDir string, payload []byte) error {
	if len(payload) == 0 || len(payload) > maxPayloadBytes {
		return errors.New("host payload is empty or too large")
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil || object == nil {
		return errors.New("host payload must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("host payload has trailing JSON")
	}
	event, runKey, err := translate(host, kind, object)
	if err != nil {
		return err
	}
	application := bootstrap.New(bootstrap.Options{StartDir: startDir, Mode: bootstrap.ModeWrite})
	if err := application.Start(ctx); err != nil {
		return err
	}
	defer application.Shutdown(ctx)
	store, err := agent.NewHostRuntimeStore(application.DB(), app.SystemClock{})
	if err != nil {
		return err
	}
	if err := store.ObservePendingLifecycle(ctx, event); err != nil {
		// A status line or pre-tool event can be the first event observed after
		// install/restart. Establish the generation without guessing a task.
		if !errors.Is(err, agent.ErrPendingHostGeneration) || (event.Event != agent.HostEventActivity && event.Event != agent.HostEventTelemetry) {
			return err
		}
		event.Event = agent.HostEventStart
		if err := store.ObservePendingLifecycle(ctx, event); err != nil {
			return err
		}
	}
	if runKey != "" {
		return store.AssociatePendingRun(ctx, event.Host, event.HostSessionID, event.AgentID, runKey)
	}
	return nil
}

func translate(host, kind string, object map[string]any) (agent.HostLifecycleInput, string, error) {
	canonicalHost := map[string]string{"claude": agent.HostClaudeCode, "claude-code": agent.HostClaudeCode, "codex": agent.HostCodex}[host]
	if canonicalHost == "" {
		return agent.HostLifecycleInput{}, "", errors.New("unsupported host adapter")
	}
	sessionID := stringField(object, "session_id")
	if sessionID == "" {
		return agent.HostLifecycleInput{}, "", errors.New("host payload has no session_id")
	}
	hook := stringField(object, "hook_event_name")
	event := agent.HostEventActivity
	switch strings.ToLower(hook) {
	case "sessionstart", "subagentstart":
		event = agent.HostEventStart
	case "sessionend", "subagentstop":
		event = agent.HostEventStop
	case "stop", "pretooluse", "posttooluse", "":
		if kind == "statusline" {
			event = agent.HostEventTelemetry
		}
	default:
		return agent.HostLifecycleInput{}, "", fmt.Errorf("unsupported host event %q", hook)
	}
	if strings.EqualFold(hook, "SessionStart") && (strings.EqualFold(stringField(object, "source"), "resume") || strings.EqualFold(stringField(object, "source"), "compact")) {
		event = agent.HostEventActivity
	}
	model := stringField(object, "model")
	if nested, ok := object["model"].(map[string]any); ok {
		model = stringField(nested, "id")
	}
	effort := ""
	if nested, ok := object["effort"].(map[string]any); ok {
		effort = stringField(nested, "level")
	}
	agentType := stringField(object, "agent_type")
	if kind == "statusline" && stringField(object, "agent_id") == "" {
		if nested, ok := object["agent"].(map[string]any); ok {
			agentType = stringField(nested, "name")
		}
	}
	var used, limit *int64
	if window, ok := object["context_window"].(map[string]any); ok {
		if value, ok := integerField(window, "context_window_size"); ok {
			limit = &value
		}
		if usage, ok := window["current_usage"].(map[string]any); ok {
			value := int64(0)
			for _, name := range []string{"input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens"} {
				part, _ := integerField(usage, name)
				value += part
			}
			used = &value
		}
		if used == nil || limit == nil {
			used, limit = nil, nil
		}
	}
	input := agent.HostLifecycleInput{Host: canonicalHost, HostSessionID: sessionID,
		AgentID: stringField(object, "agent_id"), AgentType: agentType,
		Event: event, Source: stringField(object, "source"), ModelKey: model, Effort: effort,
		ContextUsed: used, ContextLimit: limit}
	runKey := ""
	if hook == "PreToolUse" {
		if toolInput, ok := object["tool_input"].(map[string]any); ok {
			runKey = stringField(toolInput, "run_key")
		}
	}
	return input, runKey, nil
}

func stringField(object map[string]any, name string) string {
	value, _ := object[name].(string)
	return value
}

func integerField(object map[string]any, name string) (int64, bool) {
	switch value := object[name].(type) {
	case json.Number:
		n, err := value.Int64()
		return n, err == nil
	case float64:
		return int64(value), value == float64(int64(value))
	default:
		return 0, false
	}
}
