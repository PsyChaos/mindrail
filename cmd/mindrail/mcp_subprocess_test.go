package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPSubprocessServesFourteenTools is REQ-005's public boundary: it uses
// the SDK's command transport against a built binary, not an in-process
// server. Any diagnostic written to stdout corrupts this client session.
func TestMCPSubprocessServesFourteenTools(t *testing.T) {
	binary := buildMCPBinary(t)
	repo := newMCPRepo(t)
	init := exec.Command(binary, "init")
	init.Dir = repo
	if out, err := init.CombinedOutput(); err != nil {
		t.Fatalf("init binary: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "mindrail-launcher-test", Version: "v0"}, nil)
	command := exec.Command(binary, "mcp")
	command.Dir = repo
	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect stdio launcher: %v", err)
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, &sdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	want := []string{
		"mindrail_after_change", "mindrail_before_change", "mindrail_bootstrap",
		"mindrail_checkpoint", "mindrail_claim", "mindrail_complete", "mindrail_context",
		"mindrail_decide", "mindrail_invariant", "mindrail_reconcile", "mindrail_route",
		"mindrail_search", "mindrail_status", "mindrail_validate",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}

	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "mindrail_bootstrap", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("bootstrap call: %v", err)
	}
	if result.IsError {
		t.Fatalf("bootstrap response is error: %+v", result.Content)
	}
}

// TestZeroCeremonyEndToEndOverStdio exercises the public agent path against a
// persistent real process. Setup uses only init; no coordination command or DB
// fixture creates the session, task, lease, change or terminal transition.
func TestZeroCeremonyEndToEndOverStdio(t *testing.T) {
	testZeroCeremonyEndToEndOverStdio(t, false)
}

func TestZeroCeremonyEndToEndOverStdioFromSubdirectory(t *testing.T) {
	testZeroCeremonyEndToEndOverStdio(t, true)
}

func testZeroCeremonyEndToEndOverStdio(t *testing.T, subdirectory bool) {
	binary := buildMCPBinary(t)
	repo := newMCPRepo(t)
	work := filepath.Join(repo, "work.txt")
	if err := os.WriteFile(work, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitMCP(t, repo, "add", "work.txt")
	gitMCP(t, repo, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-qm", "base")
	init := exec.Command(binary, "init")
	init.Dir = repo
	if out, err := init.CombinedOutput(); err != nil {
		t.Fatalf("init binary: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "zero-ceremony-e2e", Version: "v0"}, nil)
	args := []string{"mcp"}
	if subdirectory {
		dir := filepath.Join(repo, "nested", "work")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		args = []string{"-C", dir, "mcp"}
	}
	command := exec.Command(binary, args...)
	command.Dir = repo
	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect stdio launcher: %v", err)
	}
	defer session.Close()

	started := callMCP(t, ctx, session, "mindrail_bootstrap", map[string]any{
		"goal": "Update the tracked work file", "run_key": "e2e-stable-run", "paths": []string{"work.txt"},
	})
	if started["task_id"] == "" || started["session_id"] == "" || started["state"] != "IN_PROGRESS" || started["worktree_root"] != repo {
		t.Fatalf("automatic bootstrap = %#v", started)
	}
	if err := os.WriteFile(work, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finished := callMCP(t, ctx, session, "mindrail_complete", map[string]any{"finalize": true})
	if finished["allow"] != true || finished["completed"] != true || finished["state"] != "COMPLETED" {
		t.Fatalf("automatic complete = %#v", finished)
	}
	replayed := callMCP(t, ctx, session, "mindrail_complete", map[string]any{"finalize": true})
	if replayed["completed"] != true || replayed["task_id"] != finished["task_id"] || replayed["revision"] != finished["revision"] {
		t.Fatalf("automatic complete replay = %#v, first %#v", replayed, finished)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	restartedClient := sdk.NewClient(&sdk.Implementation{Name: "zero-ceremony-restart", Version: "v0"}, nil)
	restartedCommand := exec.Command(binary, "mcp")
	restartedCommand.Dir = repo
	restarted, err := restartedClient.Connect(ctx, &sdk.CommandTransport{Command: restartedCommand}, nil)
	if err != nil {
		t.Fatalf("restart stdio launcher: %v", err)
	}
	defer restarted.Close()
	afterRestart := callMCP(t, ctx, restarted, "mindrail_complete", map[string]any{
		"finalize": true, "run_key": "e2e-stable-run",
	})
	if afterRestart["completed"] != true || afterRestart["task_id"] != finished["task_id"] || afterRestart["revision"] != finished["revision"] {
		t.Fatalf("restart replay = %#v, first %#v", afterRestart, finished)
	}
}

func TestMCPJSONIsTypedUsageRefusalWithProtocolCleanStdout(t *testing.T) {
	binary := buildMCPBinary(t)
	for _, args := range [][]string{
		{"mcp", "--json"}, {"mcp", "--json", "extra"}, {"mcp", "--json", "--unknown"},
		{"--json", "mcp", "extra"}, {"--json", "mcp", "--unknown"}, {"mcp", "--unknown"},
		{"mcp", "extra", "--json=true"}, {"mcp", "--unknown", "--json"},
		{"--unknown", "mcp", "--json"}, {"--json", "--unknown", "mcp"}, {"--unknown=value", "--json", "mcp"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			command := exec.Command(binary, args...)
			command.Dir = newMCPRepo(t)
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			if err := command.Run(); err == nil {
				t.Fatal("invalid MCP invocation succeeded")
			}
			if command.ProcessState.ExitCode() != 2 {
				t.Fatalf("exit = %d, want 2; stderr: %s", command.ProcessState.ExitCode(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("usage refusal contaminated protocol stdout: %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), "COMMAND_LINE_INVALID") {
				t.Fatalf("stderr has no typed usage code: %s", stderr.String())
			}
		})
	}
}

func buildMCPBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "mindrail")
	build := exec.Command("go", "build", "-o", binary, "./cmd/mindrail")
	build.Dir = mcpModuleRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, out)
	}
	return binary
}

func newMCPRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return repo
}

func mcpModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(dir, "..", ".."))
}

func callMCP(t *testing.T, ctx context.Context, session *sdk.ClientSession, tool string, args map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s call: %v", tool, err)
	}
	if result.IsError {
		var messages []string
		for _, item := range result.Content {
			if text, ok := item.(*sdk.TextContent); ok {
				messages = append(messages, text.Text)
			}
		}
		t.Fatalf("%s tool error: %s", tool, strings.Join(messages, "; "))
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func gitMCP(t *testing.T, repo string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
