package mcp_test

// Warm-path benchmarks (MR-019 TASK-01): the five operations against one
// fixture repo, graded against the STRUCTURAL targets. These measure wall
// clock by construction — benchmarks are the one place timing assertions
// belong (requirements AC-01.4 bans them in unit tests, not here).
//
// Each benchmark connects one client once and reuses it across iterations;
// per-call connects would measure transport setup, not the operation.
// One warmup call runs before measurement so iteration 1 does not price
// cold caches.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/mcp"
	"github.com/PsyChaos/mindrail/internal/perf"
)

// benchRepo builds a ten-file fixture: the ≤10-file shape the after_change
// and reconcile targets are written against. It extends the standard test
// repo (bootstrap init, knowledge, one indexed file) with nine more files
// so discovery, parsing and traversal price a full small change.
func benchRepo(t testing.TB, files int) string {
	t.Helper()
	root := newTestRepo(t)
	pkg := filepath.Join(root, "pkg")
	for i := 1; i < files; i++ {
		body := fmt.Sprintf("def helper_%d():\n    return %d\n", i, i)
		rel := fmt.Sprintf("f%02d.py", i)
		abs := filepath.Join(pkg, rel)
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		indexTestFile(t, root, pkg, abs)
	}
	return root
}

// benchClient holds one connected client across all iterations.
type benchClient struct {
	session *sdk.ClientSession
	b       testing.TB
}

func dialBench(b testing.TB, server *mcp.Server) *benchClient {
	b.Helper()
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.SDK().Connect(context.Background(), serverTransport, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = serverSession.Close() })
	client := sdk.NewClient(&sdk.Implementation{Name: "bench", Version: "v0.0.1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = clientSession.Close() })
	return &benchClient{session: clientSession, b: b}
}

func (c *benchClient) call(tool string, args map[string]any) {
	c.b.Helper()
	c.callCtx(context.Background(), tool, args)
}

func (c *benchClient) callCtx(ctx context.Context, tool string, args map[string]any) {
	c.b.Helper()
	result, err := c.session.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		c.b.Fatalf("tool %s: %v", tool, err)
	}
	if result.IsError {
		c.b.Fatalf("tool %s errored: %+v", tool, result.Content)
	}
}

// gradeOp runs fn b.N times under a fresh ambient timer each iteration,
// grades p95 against the STRUCTURAL target, and fails the benchmark when
// the target is missed (decision D-229). Breakdowns observed on the path
// (parse, sqlite-wait, traversal) ride the timer into the report; phases
// a path never touches stay absent rather than zero-filled.
// gradeOp grades one operation through the shared perf harness. Totals
// cross the transport; breakdowns do not (the wire drops context values)
// — those are measured at the service layer, where the context arrives
// intact (see the changes and impact benchmarks).
func gradeOp(b *testing.B, op string, fn func(ctx context.Context)) {
	b.Helper()
	perf.Bench(b, op, fn)
}

func BenchmarkStatus(b *testing.B) {
	root := benchRepo(b, 10)
	server := newTestServer(b, root)
	client := dialBench(b, server)
	gradeOp(b, "status", func(ctx context.Context) {
		client.callCtx(ctx, mcp.ToolStatus, map[string]any{})
	})
}

func BenchmarkContext(b *testing.B) {
	root := benchRepo(b, 10)
	server := newTestServer(b, root)
	client := dialBench(b, server)
	gradeOp(b, "context", func(ctx context.Context) {
		client.callCtx(ctx, mcp.ToolContext, map[string]any{"detail_level": "focused"})
	})
}

func BenchmarkBeforeChange(b *testing.B) {
	root := benchRepo(b, 10)
	server := newTestServer(b, root)
	coord, db := coordinationStore(b, root)
	workspaceID, projectID := workspaceOf(b, db)
	taskID, _ := openTaskAndSession(b, coord, workspaceID, projectID)
	var paths []string
	for i := range 10 {
		paths = append(paths, filepath.Join(root, "pkg", fmt.Sprintf("f%02d.py", i)))
	}
	client := dialBench(b, server)
	gradeOp(b, "before_change", func(ctx context.Context) {
		client.callCtx(ctx, mcp.ToolBeforeChange, map[string]any{"task_id": taskID, "paths": paths})
	})
}

func BenchmarkAfterChange(b *testing.B) {
	root := benchRepo(b, 10)
	server := newTestServer(b, root)
	coord, db := coordinationStore(b, root)
	workspaceID, projectID := workspaceOf(b, db)
	taskID, _ := openTaskAndSession(b, coord, workspaceID, projectID)
	client := dialBench(b, server)
	var paths []string
	for i := range 10 {
		paths = append(paths, filepath.Join(root, "pkg", fmt.Sprintf("f%02d.py", i)))
	}
	// after_change diffs against a declared baseline: declare once, then
	// every iteration edits first (a warm agent loop calls after_change
	// on fresh edits, so the honest measurement parses and writes each
	// time — which is also what publishes nonzero parse/sqlite-wait
	// breakdowns into the report).
	client.call(mcp.ToolBeforeChange, map[string]any{"task_id": taskID, "paths": paths})
	var n int
	changing := filepath.Join(root, "pkg", "f00.py")
	gradeOp(b, "after_change", func(ctx context.Context) {
		n++
		body := fmt.Sprintf("def helper_0():\n    return %d\n", n)
		if err := os.WriteFile(changing, []byte(body), 0o644); err != nil {
			b.Fatal(err)
		}
		client.callCtx(ctx, mcp.ToolAfterChange, map[string]any{"task_id": taskID})
	})
}

func BenchmarkReconcile(b *testing.B) {
	root := benchRepo(b, 10)
	server := newTestServer(b, root)
	coord, db := coordinationStore(b, root)
	workspaceID, projectID := workspaceOf(b, db)
	taskID, _ := openTaskAndSession(b, coord, workspaceID, projectID)
	client := dialBench(b, server)
	var n int
	changing := filepath.Join(root, "pkg", "f00.py")
	gradeOp(b, "reconcile", func(ctx context.Context) {
		n++
		body := fmt.Sprintf("def helper_0():\n    return %d\n", n)
		if err := os.WriteFile(changing, []byte(body), 0o644); err != nil {
			b.Fatal(err)
		}
		client.callCtx(ctx, mcp.ToolReconcile, map[string]any{"task_id": taskID})
	})
}
