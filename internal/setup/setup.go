// Package setup installs repository-local agent guidance and Git integration.
// It preserves user content and refuses ambiguous or unsafe managed targets.
package setup

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

const Begin = "<!-- BEGIN MINDRAIL MANAGED SECTION -->"
const End = "<!-- END MINDRAIL MANAGED SECTION -->"
const hookBegin = "# mindrail:begin verify-staged"
const hookEnd = "# mindrail:end verify-staged"
const chainMarker = "# mindrail:chain v1"
const backupSuffix = ".mindrail-original"
const claudeHookCommand = "mindrail host claude hook"
const claudeStatusCommand = "mindrail host claude statusline"
const codexEndCommand = "mindrail host codex hook"
const codexMCPServer = "mindrail"
const codexMCPTool = "mindrail_host_event"
const excludeBegin = "# mindrail:begin local-host-adapters"
const excludeEnd = "# mindrail:end local-host-adapters"

const instructions = `## MINDRAIL PROTOCOL

Mindrail is the repository's local engineering gate. The MCP server command is
` + "`mindrail mcp`" + `, started in this repository. Use the existing 15 MCP tools.

At the start of a task, call mindrail_bootstrap with goal, an agent-generated
stable run_key, and paths when known. Keep the same run_key when retrying or
reconnecting. To continue an existing handoff, also pass resume_task_id explicitly.
Do not choose another agent's active task. Sessions, task revisions, operation
identities and leases are managed automatically; do not ask a person to copy them.

Read context through mindrail_status, mindrail_search and mindrail_context.
If bootstrap omitted paths, call mindrail_before_change with paths before the first edit.
Call it again before editing any file outside the declared scope. It may be skipped
only when bootstrap paths already cover every file being edited. Relative
repository file paths are accepted in automatic mode. The task and session may
be omitted when this connection has bootstrapped automatic work. Use
mindrail_after_change or mindrail_reconcile to inspect changes when useful.
Record durable choices through mindrail_decide and mindrail_invariant.

To finish, call mindrail_complete with finalize: true. It reconciles changes,
runs configured validation profiles, applies the shared completion gate, and
completes the task only when allowed. A denial is work to resolve, never success.
No configured profiles means no test profiles ran; do not claim tests passed.
For handoff, call mindrail_checkpoint with a note and handoff: true. The next
run uses a fresh run_key and explicitly resumes that task.

People normally run mindrail init once, and mindrail status, mindrail doctor or
mindrail verify when needed. CI runs mindrail verify --ci. Advanced commands and
explicit-ID MCP payloads remain available for diagnostics and legacy clients.

Before an ambiguous tool, agent, model, or reasoning-effort choice—or another
closed choice explicitly supported by its schema—call the visible ` + "`mindrail_route`" + `
MCP tool exactly once, without waiting for the user to mention Mindrail or JEV.
Pass a required ` + "`goal`" + ` string, optional bounded ` + "`context`" + ` JSON, and one or more
` + "`tools`" + `, ` + "`agents`" + `, ` + "`models`" + `, or ` + "`efforts`" + ` arrays. Every candidate must contain
exactly ` + "`id`" + ` and ` + "`description`" + ` and must already satisfy host permissions,
safety policy, and availability constraints.

Supply only the smallest non-sensitive summary needed for the judgment—never raw
prompts, secrets, logs, source code, diffs, paths, or environment values. Consume
advice only when the top-level status is ` + "`ok`" + ` and every selection is accepted.
If a multi-dimension call returns ` + "`atomic_fallback`" + `, a dimension whose candidate
and confidence are otherwise usable may be retried once in its own single-dimension
call; never treat the rejected bundled result itself as accepted advice.
Missing credentials, disabled/fallback/error/rejected results, telemetry failure, or
an unavailable route tool mean normal reasoning must continue; JEV never blocks
engineering work or grants permission. ` + "`mindrail agent route`" + ` accepts the same
bounded object on stdin only as a compatibility fallback for clients that cannot
discover ` + "`mindrail_route`" + `; do not prefer the hidden CLI bridge.

The route resolves its opt-in credential from Mindrail's OS keyring entry or the
non-blank ` + "`TYPESAFE_API_KEY`" + ` automation override; without either, continue the
normal flow.
Keep the key out of repository config, ` + "`.env`" + ` files, agent settings, and
chat. The user connects it through ` + "`mindrail jev connect`" + `, which opens a
loopback browser form and stores the credential in the operating-system keyring.
Persistent storage is supported through Secret Service on Linux and Credential
Manager on Windows. On macOS, Mindrail refuses persistence until an application-bound
native Keychain backend is available; use the optional environment override.
` + "`TYPESAFE_API_KEY`" + ` remains an optional CI/container override.
`

type Result struct {
	AgentsPath, HookPath, ClaudePath, CodexPath, ExcludePath                string
	AgentsChanged, HookChanged, ClaudeChanged, CodexChanged, ExcludeChanged bool
	CodexTrustRequired                                                      bool
}

type target struct {
	root      *os.Root
	rel, path string
	before    []byte
	mode      os.FileMode
	exists    bool
}

// Install preflights both managed targets before changing either one. HookPath
// must be Git's effective pre-commit path, confined to the worktree/common dir.
func Install(worktree, commonDir, hookPath string) (Result, error) {
	result := Result{
		AgentsPath: filepath.Join(worktree, "AGENTS.md"), HookPath: hookPath,
		ClaudePath:  filepath.Join(worktree, ".claude", "settings.local.json"),
		CodexPath:   filepath.Join(worktree, ".codex", "hooks.json"),
		ExcludePath: filepath.Join(commonDir, "info", "exclude"),
	}
	work, err := os.OpenRoot(worktree)
	if err != nil {
		return result, refuse(worktree, err)
	}
	defer work.Close()
	common, err := os.OpenRoot(commonDir)
	if err != nil {
		return result, refuse(commonDir, err)
	}
	defer common.Close()
	agents, err := readTarget(work, "AGENTS.md", result.AgentsPath)
	if err != nil {
		return result, err
	}
	merged, err := mergeInstructions(agents.before)
	if err != nil {
		return result, refuse(result.AgentsPath, err)
	}
	hookRoot, hookRel, err := hookBoundary(work, common, worktree, commonDir, hookPath)
	if err != nil {
		return result, err
	}
	hook, err := readTarget(hookRoot, hookRel, hookPath)
	if err != nil {
		return result, err
	}
	backup, err := readTarget(hookRoot, hookRel+backupSuffix, hookPath+backupSuffix)
	if err != nil {
		return result, err
	}
	wrapper := hookWrapper()
	claude, err := readTarget(work, filepath.Join(".claude", "settings.local.json"), result.ClaudePath)
	if err != nil {
		return result, err
	}
	worktreeKey := fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.Clean(worktree))))
	statusBackupRel := filepath.Join("mindrail", "host-adapters", worktreeKey, "claude-statusline.json")
	statusBackupPath := filepath.Join(commonDir, statusBackupRel)
	statusBackup, err := readTarget(common, statusBackupRel, statusBackupPath)
	if err != nil {
		return result, err
	}
	claudeBody, backupBody, err := mergeClaudeSettings(claude.before, statusBackup.before)
	if err != nil {
		return result, refuse(result.ClaudePath, err)
	}
	codex, err := readTarget(work, filepath.Join(".codex", "hooks.json"), result.CodexPath)
	if err != nil {
		return result, err
	}
	codexBody, err := mergeCodexHooks(codex.before)
	if err != nil {
		return result, refuse(result.CodexPath, err)
	}
	exclude, err := readTarget(common, filepath.Join("info", "exclude"), result.ExcludePath)
	if err != nil {
		return result, err
	}
	excludeBody, err := mergeLocalExcludes(exclude.before)
	if err != nil {
		return result, refuse(result.ExcludePath, err)
	}
	var foreign []byte
	if !bytes.Equal(hook.before, wrapper) {
		if bytes.Contains(hook.before, []byte(chainMarker)) {
			return result, refuse(hookPath, fmt.Errorf("managed hook wrapper was edited; preserve those edits before reinstalling"))
		}
		foreign, err = foreignHook(hook.before)
		if err != nil {
			return result, refuse(hookPath, err)
		}
		if len(foreign) > 0 && backup.exists && !bytes.Equal(foreign, backup.before) {
			return result, refuse(backup.path, fmt.Errorf("foreign-hook backup is already occupied by different content"))
		}
		if len(foreign) == 0 && backup.exists {
			return result, refuse(backup.path, fmt.Errorf("orphaned foreign-hook backup needs review before installation"))
		}
	}
	if len(foreign) > 0 && !backup.exists {
		if err := writeNew(backup, foreign, hook.mode); err != nil {
			return result, err
		}
	}
	if len(backupBody) > 0 && !bytes.Equal(statusBackup.before, backupBody) {
		if statusBackup.exists {
			if err := replace(statusBackup, backupBody, 0o600); err != nil {
				return result, err
			}
		} else if err := writeNew(statusBackup, backupBody, 0o600); err != nil {
			return result, err
		}
	}
	if !bytes.Equal(hook.before, wrapper) || hook.mode.Perm()&0o111 == 0 {
		if err := replace(hook, wrapper, 0o755); err != nil {
			return result, err
		}
		result.HookChanged = true
	}
	if !bytes.Equal(agents.before, merged) {
		mode := agents.mode
		if !agents.exists {
			mode = 0o644
		}
		if err := replace(agents, merged, mode); err != nil {
			return result, err
		}
		result.AgentsChanged = true
	}
	if !bytes.Equal(claude.before, claudeBody) {
		mode := claude.mode
		if !claude.exists {
			mode = 0o644
		}
		if err := replace(claude, claudeBody, mode); err != nil {
			return result, err
		}
		result.ClaudeChanged = true
	}
	if !bytes.Equal(codex.before, codexBody) {
		mode := codex.mode
		if !codex.exists {
			mode = 0o644
		}
		if err := replace(codex, codexBody, mode); err != nil {
			return result, err
		}
		result.CodexChanged = true
		result.CodexTrustRequired = true
	}
	if !bytes.Equal(exclude.before, excludeBody) {
		mode := exclude.mode
		if !exclude.exists {
			mode = 0o644
		}
		if err := replace(exclude, excludeBody, mode); err != nil {
			return result, err
		}
		result.ExcludeChanged = true
	}
	return result, nil
}

func mergeLocalExcludes(content []byte) ([]byte, error) {
	text := string(content)
	if strings.Count(text, excludeBegin) != strings.Count(text, excludeEnd) || strings.Count(text, excludeBegin) > 1 {
		return nil, errors.New("Git exclude needs at most one matching pair of Mindrail markers")
	}
	block := excludeBegin + "\n/.claude/settings.local.json\n/.codex/hooks.json\n" + excludeEnd
	if strings.Contains(text, excludeBegin) {
		start, end := strings.Index(text, excludeBegin), strings.Index(text, excludeEnd)
		if end < start {
			return nil, errors.New("Git exclude Mindrail markers are out of order")
		}
		return []byte(text[:start] + block + text[end+len(excludeEnd):]), nil
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return []byte(text + block + "\n"), nil
}

func decodeJSONObject(content []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return map[string]any{}, nil
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if object == nil {
		return nil, fmt.Errorf("JSON root must be an object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("JSON contains trailing values")
		}
		return nil, fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return object, nil
}

func encodeJSONObject(object map[string]any) ([]byte, error) {
	body, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func mergeClaudeSettings(content, savedStatus []byte) ([]byte, []byte, error) {
	root, err := decodeJSONObject(content)
	if err != nil {
		return nil, nil, err
	}
	var backup []byte
	managedStatus := map[string]any{"type": "command", "command": claudeStatusCommand}
	current, hasStatus := root["statusLine"]
	currentObject, objectStatus := current.(map[string]any)
	isManaged := objectStatus && currentObject["type"] == "command" && currentObject["command"] == claudeStatusCommand
	if hasStatus && !isManaged {
		backup, err = json.Marshal(current)
		if err != nil {
			return nil, nil, err
		}
		backup = append(backup, '\n')
	} else if len(savedStatus) > 0 {
		backup = append([]byte(nil), savedStatus...)
	}
	if objectStatus {
		managedStatus = make(map[string]any, len(currentObject)+2)
		for key, value := range currentObject {
			managedStatus[key] = value
		}
		managedStatus["type"] = "command"
		managedStatus["command"] = claudeStatusCommand
	}
	root["statusLine"] = managedStatus
	hooks, err := objectField(root, "hooks")
	if err != nil {
		return nil, nil, err
	}
	for _, event := range []string{"SessionStart", "SessionEnd", "PreToolUse", "SubagentStart", "SubagentStop"} {
		matcher := ""
		if event == "SessionStart" {
			matcher = "startup|resume|clear|compact"
		}
		group := commandHookGroup(matcher, claudeHookCommand)
		hooks[event], err = appendManagedGroup(hooks[event], group, func(handler map[string]any) bool {
			return handler["type"] == "command" && handler["command"] == claudeHookCommand
		})
		if err != nil {
			return nil, nil, fmt.Errorf("hooks.%s: %w", event, err)
		}
	}
	root["hooks"] = hooks
	body, err := encodeJSONObject(root)
	return body, backup, err
}

func mergeCodexHooks(content []byte) ([]byte, error) {
	root, err := decodeJSONObject(content)
	if err != nil {
		return nil, err
	}
	hooks, err := objectField(root, "hooks")
	if err != nil {
		return nil, err
	}
	for _, event := range []string{"SessionStart", "SubagentStart", "SubagentStop", "Stop"} {
		matcher := ""
		if event == "SessionStart" {
			matcher = "startup|resume|clear|compact"
		}
		kind := "activity"
		if event == "SessionStart" || event == "SubagentStart" {
			kind = "start"
		}
		if event == "SubagentStop" {
			kind = "stop"
		}
		input := map[string]any{
			"host": "codex", "host_session_id": "${session_id}", "event": kind,
			"model": "${model}",
		}
		if event == "SessionStart" {
			input["source"] = "${source}"
		}
		if strings.HasPrefix(event, "Subagent") {
			input["agent_id"] = "${agent_id}"
			input["agent_type"] = "${agent_type}"
		}
		group := mcpHookGroup(matcher, input)
		hooks[event], err = appendManagedGroup(hooks[event], group, func(handler map[string]any) bool {
			return handler["type"] == "mcp_tool" && handler["server"] == codexMCPServer && handler["tool"] == codexMCPTool
		})
		if err != nil {
			return nil, fmt.Errorf("hooks.%s: %w", event, err)
		}
	}
	claim := mcpHookGroup("^mcp__mindrail__mindrail_bootstrap$", map[string]any{
		"host": "codex", "host_session_id": "${session_id}", "event": "activity",
		"model": "${model}", "claim_pending": true, "run_key": "${tool_input.run_key}",
	})
	hooks["PreToolUse"], err = appendManagedGroup(hooks["PreToolUse"], claim, func(handler map[string]any) bool {
		return handler["type"] == "mcp_tool" && handler["server"] == codexMCPServer && handler["tool"] == codexMCPTool && hasClaimPending(handler)
	})
	if err != nil {
		return nil, fmt.Errorf("hooks.PreToolUse: %w", err)
	}
	end := commandHookGroup("", codexEndCommand)
	hooks["SessionEnd"], err = appendManagedGroup(hooks["SessionEnd"], end, func(handler map[string]any) bool {
		return handler["type"] == "command" && handler["command"] == codexEndCommand
	})
	if err != nil {
		return nil, fmt.Errorf("hooks.SessionEnd: %w", err)
	}
	root["hooks"] = hooks
	if _, ok := root["description"]; !ok {
		root["description"] = "Project hooks including Mindrail runtime telemetry."
	}
	return encodeJSONObject(root)
}

func hasClaimPending(handler map[string]any) bool {
	input, ok := handler["input"].(map[string]any)
	return ok && input["claim_pending"] == true
}

func objectField(root map[string]any, name string) (map[string]any, error) {
	value, ok := root[name]
	if !ok {
		return map[string]any{}, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", name)
	}
	return object, nil
}

func commandHookGroup(matcher, command string) map[string]any {
	group := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command, "timeout": json.Number("3")}}}
	if matcher != "" {
		group["matcher"] = matcher
	}
	return group
}

func mcpHookGroup(matcher string, input map[string]any) map[string]any {
	group := map[string]any{"hooks": []any{map[string]any{"type": "mcp_tool", "server": codexMCPServer, "tool": codexMCPTool, "input": input, "timeout": json.Number("3")}}}
	if matcher != "" {
		group["matcher"] = matcher
	}
	return group
}

func appendManagedGroup(current any, managed map[string]any, identifies func(map[string]any) bool) ([]any, error) {
	var groups []any
	if current != nil {
		var ok bool
		groups, ok = current.([]any)
		if !ok {
			return nil, fmt.Errorf("must be an array")
		}
	}
	filtered := make([]any, 0, len(groups)+1)
	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		handlers, ok := group["hooks"].([]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		remaining := make([]any, 0, len(handlers))
		for _, rawHandler := range handlers {
			if handler, ok := rawHandler.(map[string]any); ok && identifies(handler) {
				continue
			}
			remaining = append(remaining, rawHandler)
		}
		if len(remaining) > 0 {
			kept := make(map[string]any, len(group))
			for key, value := range group {
				kept[key] = value
			}
			kept["hooks"] = remaining
			filtered = append(filtered, kept)
		}
	}
	return append(filtered, managed), nil
}

func sameJSONObject(left any, right map[string]any) bool {
	a, errA := json.Marshal(left)
	b, errB := json.Marshal(right)
	return errA == nil && errB == nil && bytes.Equal(a, b)
}

func hookBoundary(work, common *os.Root, worktree, commonDir, path string) (*os.Root, string, error) {
	for _, candidate := range []struct {
		root *os.Root
		path string
	}{{common, commonDir}, {work, worktree}} {
		rel, err := filepath.Rel(candidate.path, path)
		if err != nil {
			continue
		}
		if rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return candidate.root, rel, nil
		}
	}
	return nil, "", app.NewError(app.CodePathEscapesRoot, app.KindFailed, "effective pre-commit hook is outside this repository: "+path, "Repository-local setup will not modify an external hooks directory.", "Choose a repository-local core.hooksPath, or install the Mindrail guard in the shared hooks directory yourself.").WithMetadata("path", path)
}

func readTarget(root *os.Root, rel, path string) (target, error) {
	t := target{root: root, rel: rel, path: path, mode: 0o644}
	// Reject links even within the boundary: a managed name must not overwrite
	// another user-owned file. os.Root enforces containment during every I/O.
	for p := rel; p != "."; p = filepath.Dir(p) {
		info, err := root.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return t, refuse(path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return t, refuse(path, fmt.Errorf("managed path traverses a symbolic link"))
		}
		if p == rel {
			if !info.Mode().IsRegular() {
				return t, refuse(path, fmt.Errorf("managed target is not a regular file"))
			}
			t.mode = info.Mode().Perm()
			t.exists = true
		}
	}
	if t.exists {
		body, err := root.ReadFile(rel)
		if err != nil {
			return t, refuse(path, err)
		}
		t.before = body
	}
	return t, nil
}

func mergeInstructions(content []byte) ([]byte, error) {
	text := string(content)
	begins, ends := strings.Count(text, Begin), strings.Count(text, End)
	block := Begin + "\n" + instructions + End
	if begins == 0 && ends == 0 {
		separator := ""
		if text != "" {
			if !strings.HasSuffix(text, "\n") {
				separator = "\n"
			}
			separator += "\n"
		}
		return []byte(text + separator + block + "\n"), nil
	}
	if begins != 1 || ends != 1 {
		return nil, fmt.Errorf("AGENTS.md needs exactly one matching pair of Mindrail managed markers")
	}
	start, end := strings.Index(text, Begin), strings.Index(text, End)
	if end < start || !wholeLine(text, start, len(Begin)) || !wholeLine(text, end, len(End)) {
		return nil, fmt.Errorf("Mindrail managed markers must be ordered and occupy their own lines")
	}
	return []byte(text[:start] + block + text[end+len(End):]), nil
}

func wholeLine(text string, start, length int) bool {
	return (start == 0 || text[start-1] == '\n') && (start+length == len(text) || text[start+length] == '\n' || (text[start+length] == '\r' && start+length+1 < len(text) && text[start+length+1] == '\n'))
}

func hookWrapper() []byte {
	return []byte("#!/bin/sh\n" + chainMarker + "\nmindrail_original=\"$(dirname -- \"$0\")/pre-commit" + backupSuffix + "\"\nif [ -x \"$mindrail_original\" ]; then\n  \"$mindrail_original\" \"$@\" || exit $?\nfi\n" + hookBegin + "\nmindrail verify --staged\n" + hookEnd + "\n")
}

func foreignHook(content []byte) ([]byte, error) {
	text := string(content)
	if strings.Contains(text, hookBegin) || strings.Contains(text, hookEnd) {
		legacy := hookBegin + "\nmindrail verify --staged\n" + hookEnd + "\n"
		if strings.Count(text, hookBegin) != 1 || strings.Count(text, hookEnd) != 1 || !strings.Contains(text, legacy) {
			return nil, fmt.Errorf("existing Mindrail hook markers are malformed or modified")
		}
		text = strings.Replace(text, legacy, "", 1)
	}
	if text == "" || text == "#!/bin/sh\n" {
		return nil, nil
	}
	return []byte(text), nil
}

func writeNew(t target, data []byte, mode os.FileMode) error {
	if err := t.root.MkdirAll(filepath.Dir(t.rel), 0o755); err != nil {
		return refuse(t.path, err)
	}
	f, err := t.root.OpenFile(t.rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return refuse(t.path, err)
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	for _, err := range []error{writeErr, syncErr, closeErr} {
		if err != nil {
			return refuse(t.path, err)
		}
	}
	return nil
}

func replace(t target, data []byte, mode os.FileMode) error {
	if err := t.root.MkdirAll(filepath.Dir(t.rel), 0o755); err != nil {
		return refuse(t.path, err)
	}
	temp := filepath.Join(filepath.Dir(t.rel), ".mindrail-setup-"+rand.Text())
	f, err := t.root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return refuse(t.path, err)
	}
	defer t.root.Remove(temp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return refuse(t.path, err)
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return refuse(t.path, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return refuse(t.path, err)
	}
	if err := f.Close(); err != nil {
		return refuse(t.path, err)
	}
	current, err := readTarget(t.root, t.rel, t.path)
	if err != nil {
		return err
	}
	if current.exists != t.exists || !bytes.Equal(current.before, t.before) {
		return refuse(t.path, fmt.Errorf("target changed during setup; retry after reviewing the concurrent edit"))
	}
	if err := t.root.Rename(temp, t.rel); err != nil {
		return refuse(t.path, err)
	}
	return nil
}

func refuse(path string, cause error) error {
	return app.NewError(app.CodeConfigInvalid, app.KindFailed, "repository setup could not update "+path+": "+cause.Error(), "Setup is incomplete; existing user content was preserved.", "Resolve this target and rerun mindrail init.").WithMetadata("path", path).WithCause(cause)
}

func (r Result) RenderHuman(w io.Writer) error {
	_, err := fmt.Fprintf(w, "Agent instructions: %s\nPre-commit guard: %s\nClaude adapter: %s\nCodex adapter: %s\nLocal adapter excludes: %s\n", r.AgentsPath, r.HookPath, r.ClaudePath, r.CodexPath, r.ExcludePath)
	if err == nil && r.CodexTrustRequired {
		_, err = fmt.Fprintln(w, "Codex hook review: required once; open /hooks in Codex and trust the Mindrail project hooks.")
	}
	return err
}
