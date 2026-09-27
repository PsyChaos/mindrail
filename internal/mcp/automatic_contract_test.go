package mcp_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/mcp"
)

// TestBootstrapSchemaAddsAutomaticModeWithoutAddingATool pins the additive
// 0.1 contract: automatic work is selected by optional bootstrap arguments,
// never by a fourteenth tool.
func TestBootstrapSchemaAddsAutomaticModeWithoutAddingATool(t *testing.T) {
	server := newTestServer(t, newTestRepo(t))
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.SDK().Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "auto-schema", Version: "v0"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	listed, err := clientSession.ListTools(t.Context(), &sdk.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 13 {
		t.Fatalf("tool count = %d, want 13", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if tool.Name != mcp.ToolBootstrap {
			continue
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("bootstrap schema = %#v", tool.InputSchema)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("bootstrap properties = %#v", schema["properties"])
		}
		for _, field := range []string{"goal", "run_key", "paths", "resume_task_id"} {
			if _, ok := properties[field]; !ok {
				t.Errorf("bootstrap schema is missing %q: %#v", field, properties)
			}
		}
		return
	}
	t.Fatal("bootstrap tool not listed")
}

// TestLegacyBootstrapRemainsReadOnly ensures the additive automatic mode does
// not turn the historical empty payload into a session/task creating call.
func TestLegacyBootstrapRemainsReadOnly(t *testing.T) {
	root := newTestRepo(t)
	server := newTestServer(t, root)
	_, db := coordinationStore(t, root)

	beforeSessions := countRows(t, db, "sessions")
	beforeTasks := countRows(t, db, "tasks")
	got := callTool(t, server, "legacy-bootstrap", mcp.ToolBootstrap, map[string]any{})
	if got["worktree_root"] != root {
		t.Fatalf("bootstrap = %#v", got)
	}
	if after := countRows(t, db, "sessions"); after != beforeSessions {
		t.Fatalf("sessions = %d, want %d", after, beforeSessions)
	}
	if after := countRows(t, db, "tasks"); after != beforeTasks {
		t.Fatalf("tasks = %d, want %d", after, beforeTasks)
	}
	if err := callToolRaw(t, server, "invalid-auto-bootstrap", mcp.ToolBootstrap, map[string]any{
		"goal": "", "run_key": "",
	}); err == nil {
		t.Fatal("present but empty automatic fields fell back to legacy bootstrap")
	}
	if countRows(t, db, "sessions") != beforeSessions || countRows(t, db, "tasks") != beforeTasks {
		t.Fatal("invalid automatic bootstrap wrote coordination rows")
	}
}

// TestCompleteSchemaAddsFinalizeMode pins the compatibility discriminator:
// finalize is explicit and additive, while the old task_id request remains an
// evaluation call.
func TestCompleteSchemaAddsFinalizeMode(t *testing.T) {
	server := newTestServer(t, newTestRepo(t))
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.SDK().Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "complete-schema", Version: "v0"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	listed, err := clientSession.ListTools(t.Context(), &sdk.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != mcp.ToolComplete {
			continue
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("complete schema = %#v", tool.InputSchema)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("complete properties = %#v", schema["properties"])
		}
		for _, field := range []string{"finalize", "run_key"} {
			if _, ok := properties[field]; !ok {
				t.Errorf("complete schema is missing %q: %#v", field, properties)
			}
		}
		return
	}
	t.Fatal("complete tool not listed")
}

// BRK-003: each manual field must independently refuse automatic finalize.
// The same ready-to-finish run succeeds when only that forbidden field is
// removed, so another gate or connection error cannot mask this protection.
func TestAutomaticFinalizeRejectsManualFields(t *testing.T) {
	for _, field := range []string{"task_id", "required"} {
		for _, reconnect := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reconnect=%v", field, reconnect), func(t *testing.T) {
				root := cleanCompletionRepo(t)
				server := newTestServer(t, root)
				coord, db := coordinationStore(t, root)
				client := connectPersistent(t, server, "mixed-finalize")
				started := client.call(t, mcp.ToolBootstrap, map[string]any{
					"goal": "finish without manual fields", "run_key": "mixed-finalize", "paths": []string{"pkg/a.py"},
				})
				if reconnect {
					client.close()
					client = connectPersistent(t, server, "mixed-finalize-reconnected")
				}
				defer client.close()
				taskID := started["task_id"].(string)
				before, err := coord.FindTask(t.Context(), taskID)
				if err != nil {
					t.Fatal(err)
				}
				counts := map[string]int{}
				for _, table := range []string{"sessions", "tasks", "leases", "operations", "changes", "evidence"} {
					counts[table] = countRows(t, db, table)
				}
				args := map[string]any{"finalize": true, "run_key": "mixed-finalize"}
				if field == "task_id" {
					args[field] = taskID
				} else {
					args[field] = []string{"unit"}
				}
				result, err := client.client.CallTool(t.Context(), &sdk.CallToolParams{Name: mcp.ToolComplete, Arguments: args})
				if err != nil {
					t.Fatalf("transport failed instead of a tool refusal: %v", err)
				}
				if !result.IsError {
					t.Fatalf("automatic finalize accepted manual %s: %+v", field, result)
				}
				message := toolError(result).Error()
				if !strings.Contains(message, string(app.CodeCommandLineInvalid)) || !strings.Contains(message, "does not accept task_id or required") {
					t.Fatalf("wrong refusal masked mixed-mode guard: %s", message)
				}
				after, err := coord.FindTask(t.Context(), taskID)
				if err != nil || after.State != before.State || after.Revision != before.Revision {
					t.Fatalf("rejected mixed mode changed task: before=%+v after=%+v err=%v", before, after, err)
				}
				for table, beforeCount := range counts {
					if afterCount := countRows(t, db, table); afterCount != beforeCount {
						t.Errorf("mixed-mode refusal wrote %s: %d -> %d", table, beforeCount, afterCount)
					}
				}
				delete(args, field)
				finished := client.call(t, mcp.ToolComplete, args)
				if finished["completed"] != true || finished["allow"] != true || finished["task_id"] != taskID {
					t.Fatalf("valid control finalization failed: %#v", finished)
				}
			})
		}
	}
}

func TestLegacyCompleteAllowsButDoesNotTransitionTask(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	coord, db := coordinationStore(t, root)
	workspaceID, projectID := workspaceOf(t, db)
	taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)

	got := callTool(t, server, "legacy-complete", mcp.ToolComplete, map[string]any{"task_id": taskID})
	if got["allow"] != true {
		t.Fatalf("complete = %#v, want allow", got)
	}
	task, err := coord.FindTask(t.Context(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "OPEN" || task.Revision != 1 {
		t.Fatalf("legacy complete mutated task: state=%s revision=%d", task.State, task.Revision)
	}
}

func TestAutomaticConnectionsAreIsolatedAndRetryStable(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	_, db := coordinationStore(t, root)
	one := connectPersistent(t, server, "automatic-one")
	defer one.close()
	two := connectPersistent(t, server, "automatic-two")
	defer two.close()

	first := one.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "first", "run_key": "connection-one", "paths": []string{"pkg/a.py"},
	})
	second := two.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "second", "run_key": "connection-two", "paths": []string{"pkg/b.py"},
	})
	if first["started"] != true || second["started"] != true {
		t.Fatalf("starts = %#v / %#v", first, second)
	}
	if first["task_id"] == second["task_id"] || first["session_id"] == second["session_id"] {
		t.Fatalf("connections shared identity: %#v / %#v", first, second)
	}
	retry := one.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "first", "run_key": "connection-one", "paths": []string{"pkg/a.py"},
	})
	if retry["task_id"] != first["task_id"] || retry["session_id"] != first["session_id"] || retry["revision"] != first["revision"] {
		t.Fatalf("retry changed run: %#v / %#v", first, retry)
	}
	beforeSessions, beforeTasks := countRows(t, db, "sessions"), countRows(t, db, "tasks")
	if err := one.callError(t, mcp.ToolBootstrap, map[string]any{
		"goal": "wrong connection", "run_key": "must-not-start", "paths": []string{"pkg/c.py"},
	}); err == nil || !strings.Contains(err.Error(), "already has") {
		t.Fatalf("second run on one connection accepted: %v", err)
	}
	if countRows(t, db, "sessions") != beforeSessions || countRows(t, db, "tasks") != beforeTasks {
		t.Fatal("rejected second run created durable identities")
	}
	if err := one.callError(t, mcp.ToolComplete, map[string]any{"finalize": true, "run_key": "connection-two"}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("cross-connection run_key accepted: %v", err)
	}
}

func TestAutomaticMCPFromRepositorySubdirectoryUsesWorktreeRoot(t *testing.T) {
	root := cleanCompletionRepo(t)
	writeScopeFile(t, root, "work.txt", "before\n")
	gitCommitFile(t, root, "work.txt")
	rootServer := newTestServer(t, root)
	rootState := callTool(t, rootServer, "root-probe", mcp.ToolBootstrap, map[string]any{})
	server := newTestServer(t, filepath.Join(root, "pkg"))
	client := connectPersistent(t, server, "subdirectory")
	defer client.close()
	started := client.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "edit from subdirectory", "run_key": "subdir", "paths": []string{"work.txt"},
	})
	if started["started"] != true || started["worktree_root"] != root || started["db_path"] != rootState["db_path"] {
		t.Fatalf("subdirectory bootstrap chose wrong root/database: root=%+v subdir=%+v", rootState, started)
	}
	writeScopeFile(t, root, "work.txt", "after\n")
	finished := client.call(t, mcp.ToolComplete, map[string]any{"finalize": true})
	if finished["allow"] != true || finished["completed"] != true {
		t.Fatalf("subdirectory finalization: %+v", finished)
	}
}

func TestAutomaticConflictReportsPartialStateAndRetryRecovers(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	owner := connectPersistent(t, server, "lease-owner")
	defer owner.close()
	contender := connectPersistent(t, server, "lease-contender")
	defer contender.close()

	owner.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "owner", "run_key": "owner-run", "paths": []string{"pkg/a.py"},
	})
	partial := contender.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "contender", "run_key": "contender-run", "paths": []string{"pkg/a.py"},
	})
	failure, ok := partial["failure"].(map[string]any)
	if partial["started"] == true || !ok || failure["message"] == "" || failure["code"] == "" || partial["task_id"] == "" || partial["session_id"] == "" {
		t.Fatalf("conflict did not expose recoverable partial state: %#v", partial)
	}
	completed := owner.call(t, mcp.ToolComplete, map[string]any{"finalize": true})
	replayed := owner.call(t, mcp.ToolComplete, map[string]any{"finalize": true})
	if replayed["task_id"] != completed["task_id"] || replayed["revision"] != completed["revision"] {
		t.Fatalf("ID-free finalize retry changed terminal result: %#v / %#v", completed, replayed)
	}
	next := owner.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "owner next", "run_key": "owner-next-run", "paths": []string{"pkg/b.py"},
	})
	if next["started"] != true || next["task_id"] == completed["task_id"] || next["session_id"] == completed["session_id"] {
		t.Fatalf("terminal binding was not replaceable: %#v / %#v", completed, next)
	}
	recovered := contender.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "contender", "run_key": "contender-run", "paths": []string{"pkg/a.py"},
	})
	if recovered["started"] != true || recovered["task_id"] != partial["task_id"] || recovered["session_id"] != partial["session_id"] {
		t.Fatalf("retry did not recover partial run: %#v / %#v", partial, recovered)
	}
}

func TestAutomaticRestartReusesRunAndFinalizeReconciles(t *testing.T) {
	root := cleanCompletionRepo(t)
	server, err := mcp.New(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	firstClient := connectPersistent(t, server, "before-restart")
	first := firstClient.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "restart-safe", "run_key": "restart-run", "paths": []string{"pkg/a.py"},
	})
	firstClient.close()
	if err := server.Close(t.Context()); err != nil {
		t.Fatal(err)
	}

	server, err = mcp.New(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close(t.Context())
	secondClient := connectPersistent(t, server, "after-restart")
	defer secondClient.close()
	resumed := secondClient.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "restart-safe", "run_key": "restart-run", "paths": []string{"pkg/a.py"},
	})
	if resumed["task_id"] != first["task_id"] || resumed["session_id"] != first["session_id"] || resumed["revision"] != first["revision"] {
		t.Fatalf("restart duplicated run: %#v / %#v", first, resumed)
	}
	path := filepath.Join(root, "pkg", "a.py")
	if err := os.WriteFile(path, []byte("def helper():\n    return 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finished := secondClient.call(t, mcp.ToolComplete, map[string]any{"finalize": true})
	if finished["completed"] != true || finished["state"] != "COMPLETED" {
		t.Fatalf("finalize = %#v", finished)
	}
	_, db := coordinationStore(t, root)
	var discovered int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM change_files f
		JOIN changes c ON c.change_id = f.change_id WHERE c.task_id = ? AND f.path = ?`,
		first["task_id"], path).Scan(&discovered); err != nil {
		t.Fatal(err)
	}
	if discovered != 1 {
		t.Fatalf("finalize discovered %d scoped file rows, want 1", discovered)
	}
}

func TestAutomaticLifecycleCallsOmitMechanicalIDs(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	client := connectPersistent(t, server, "id-free-lifecycle")
	defer client.close()
	started := client.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "id free", "run_key": "id-free-run", "paths": []string{"pkg/a.py"},
	})
	extended := client.call(t, mcp.ToolBeforeChange, map[string]any{"paths": []string{"pkg/b.py"}})
	if extended["task_id"] != started["task_id"] {
		t.Fatalf("before_change lost automatic task: %#v", extended)
	}
	written := client.call(t, mcp.ToolCheckpoint, map[string]any{"note": "halfway"})
	if written["task_id"] != started["task_id"] || written["note"] != "halfway" {
		t.Fatalf("checkpoint = %#v", written)
	}
	read := client.call(t, mcp.ToolCheckpoint, map[string]any{})
	if read["note"] != "halfway" {
		t.Fatalf("checkpoint read = %#v", read)
	}
	path := filepath.Join(root, "pkg", "b.py")
	if err := os.WriteFile(path, []byte("value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reconciled := client.call(t, mcp.ToolReconcile, map[string]any{})
	if reconciled["change_id"] == "" {
		t.Fatalf("reconcile = %#v", reconciled)
	}
	finished := client.call(t, mcp.ToolComplete, map[string]any{"finalize": true})
	if finished["completed"] != true {
		t.Fatalf("complete = %#v", finished)
	}
}

func TestAutomaticFinalizeDenialIsExplicitAndRecoverable(t *testing.T) {
	root := cleanCompletionRepo(t)
	configPath := filepath.Join(root, ".mindrail", "config.toml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config = append(config, []byte("\n[validation.fail]\ntype = \"AUTOMATED_TEST\"\npaths = [\"pkg\"]\ncommands = [[\"false\"]]\n")...)
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitFile(t, root, ".")
	server := newTestServer(t, root)
	client := connectPersistent(t, server, "denied-finalize")
	defer client.close()
	started := client.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "must deny", "run_key": "denied-run", "paths": []string{"pkg/a.py"},
	})
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.py"), []byte("def helper():\n    return 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	denied := client.call(t, mcp.ToolComplete, map[string]any{"finalize": true})
	if denied["allow"] != false || denied["completed"] != false || denied["state"] != "IN_PROGRESS" {
		t.Fatalf("denied finalize = %#v", denied)
	}
	if denials, ok := denied["denials"].([]any); !ok || len(denials) == 0 {
		t.Fatalf("denied finalize has no actionable denials: %#v", denied)
	}
	coord, db := coordinationStore(t, root)
	task, err := coord.FindTask(t.Context(), started["task_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "IN_PROGRESS" {
		t.Fatalf("denial moved task to %s", task.State)
	}
	_, projectID := workspaceOf(t, db)
	leases, err := coord.ListLeases(t.Context(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	holder := started["session_id"].(string)
	for _, lease := range leases {
		if lease.Holder == holder && lease.Status == "active" {
			active++
		}
	}
	if active < 2 {
		t.Fatalf("denial retained %d active task/file leases, want at least 2", active)
	}
}

func TestAutomaticConcurrentLifecycleUpdatesAreRaceFree(t *testing.T) {
	root := cleanCompletionRepo(t)
	server := newTestServer(t, root)
	client := connectPersistent(t, server, "concurrent-lifecycle")
	defer client.close()
	client.call(t, mcp.ToolBootstrap, map[string]any{
		"goal": "concurrent", "run_key": "concurrent-run", "paths": []string{"pkg/a.py"},
	})

	var wait sync.WaitGroup
	transportErrors := make(chan error, 24)
	call := func(tool string, args map[string]any) {
		defer wait.Done()
		result, err := client.client.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			transportErrors <- err
			return
		}
		// Domain refusals caused by another concurrent call reaching the
		// terminal state first are valid. This regression targets shared-memory
		// safety and transport integrity.
		_ = result
	}
	for i := 0; i < 8; i++ {
		wait.Add(3)
		go call(mcp.ToolBootstrap, map[string]any{
			"goal": "concurrent", "run_key": "concurrent-run", "paths": []string{"pkg/a.py"},
		})
		go call(mcp.ToolBeforeChange, map[string]any{
			"paths": []string{filepath.Join("pkg", fmt.Sprintf("concurrent-%d.py", i))},
		})
		go call(mcp.ToolComplete, map[string]any{"finalize": true})
	}
	wait.Wait()
	close(transportErrors)
	for err := range transportErrors {
		t.Errorf("concurrent MCP transport error: %v", err)
	}
	final := client.call(t, mcp.ToolComplete, map[string]any{"finalize": true})
	if final["completed"] != true || final["state"] != "COMPLETED" {
		t.Fatalf("terminal replay after concurrent updates = %#v", final)
	}
}

type persistentClient struct {
	server *sdk.ServerSession
	client *sdk.ClientSession
}

func connectPersistent(t *testing.T, server *mcp.Server, name string) persistentClient {
	t.Helper()
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.SDK().Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: name, Version: "v0"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		t.Fatal(err)
	}
	return persistentClient{server: serverSession, client: clientSession}
}

func (c persistentClient) close() {
	_ = c.client.Close()
	_ = c.server.Close()
}

func (c persistentClient) call(t *testing.T, tool string, args map[string]any) map[string]any {
	t.Helper()
	result, err := c.client.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("%s: %v", tool, toolError(result))
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

func (c persistentClient) callError(t *testing.T, tool string, args map[string]any) error {
	t.Helper()
	result, err := c.client.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return err
	}
	if result.IsError {
		return toolError(result)
	}
	return nil
}

func toolError(result *sdk.CallToolResult) error {
	var messages []string
	for _, item := range result.Content {
		if text, ok := item.(*sdk.TextContent); ok {
			messages = append(messages, text.Text)
		}
	}
	return &toolCallError{message: strings.Join(messages, "; ")}
}

type toolCallError struct{ message string }

func (e *toolCallError) Error() string { return e.message }

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
