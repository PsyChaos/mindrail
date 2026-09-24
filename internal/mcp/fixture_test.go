package mcp_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/mcp"
	"github.com/PsyChaos/mindrail/internal/migration"
	"github.com/PsyChaos/mindrail/internal/storage"
	"github.com/PsyChaos/mindrail/internal/validation"
	"github.com/PsyChaos/mindrail/migrations"
)

const (
	testDecision = `{"schema_version":1,"kind":"decision","id":"DEC-0001","status":"active",` +
		`"created_at":"2026-09-23T10:00:00Z","title":"Test the auth flow",` +
		`"decision":"Authenticate every request.","scope":{"level":"PROJECT"}}`
	testInvariant = `{"schema_version":1,"kind":"invariant","id":"INV-0001","status":"active",` +
		`"created_at":"2026-09-23T10:00:00Z","statement":"Sessions never cross users.",` +
		`"severity":"HIGH","scope":{"level":"PROJECT"}}`
)

func newTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	init := bootstrap.New(bootstrap.Options{StartDir: root, Mode: bootstrap.ModeInit})
	if err := init.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := init.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"decisions/DEC-0001.json":  testDecision,
		"invariants/INV-0001.json": testInvariant,
	}
	for rel, content := range files {
		abs := filepath.Join(root, ".mindrail", "knowledge", rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pkg := filepath.Join(root, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "pyproject.toml"), []byte("[project]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "a.py"), []byte("def helper():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexTestFile(t, root, pkg, filepath.Join(pkg, "a.py"))
	return root
}

func indexTestFile(t *testing.T, root, pkg, path string) {
	t.Helper()
	dbPath := filepath.Join(root, ".git", "mindrail", "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	clock := app.FixedClock{Instant: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)}
	indexes := index.NewStore(db.DB, clock)
	unit, err := indexes.UpsertUnit(t.Context(), pkg, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	indexer := index.NewIndexer(indexes, registry, snapshot.New(filesystem.RuntimePaths{CacheDir: t.TempDir()}))
	if _, err := indexer.IndexFile(t.Context(), "PRJ-MCP", unit, path); err != nil {
		t.Fatal(err)
	}
}

func newTestServer(t *testing.T, root string) *mcp.Server {
	t.Helper()
	server, err := mcp.New(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	return server
}

// callTool connects one client over an in-memory transport, calls one tool
// and returns the structured output re-marshalled through JSON.
func callTool(t *testing.T, server *mcp.Server, clientName string, tool string, args map[string]any) map[string]any {
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
	result, err := clientSession.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool %s errored: %+v", tool, result.Content)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

// callToolRaw calls one tool and reports whether the call failed at the
// transport level or answered a tool error, for refusal paths that fail
// the call instead of answering refusal output.
func callToolRaw(t *testing.T, server *mcp.Server, clientName string, tool string, args map[string]any) error {
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
	result, err := clientSession.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return err
	}
	if result.IsError {
		raw, _ := json.Marshal(result.Content)
		return fmt.Errorf("tool errored: %s", raw)
	}
	return nil
}

type indexFixture struct {
	indexes *index.Store
	db      *sql.DB
	unit    index.ProjectUnit
}

func newIndexFixture(t *testing.T) indexFixture {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	indexes := index.NewStore(db.DB, clock)
	unit, err := indexes.UpsertUnit(t.Context(), t.TempDir(), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	return indexFixture{indexes: indexes, db: db.DB, unit: unit}
}

func seedBindingRow(t *testing.T, fx indexFixture, invariant, uid, status string) {
	t.Helper()
	if _, err := fx.db.ExecContext(t.Context(), `INSERT OR IGNORE INTO symbol_identities
		(symbol_uid, project_id, unit_id, language, logical_key, previous_keys, created_at)
		VALUES (?, 'PRJ-1', ?, 'python', ?, '[]', '2026-09-23T10:00:00Z')`,
		uid, fx.unit.ID, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO invariant_symbol_bindings
		(invariant_id, symbol_uid, status, updated_at)
		VALUES (?, ?, ?, '2026-09-23T10:00:00Z')`, invariant, uid, status); err != nil {
		t.Fatal(err)
	}
}

type evidenceFixture struct {
	store *validation.Store
}

func newEvidenceFixture(t *testing.T) evidenceFixture {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: filepath.Join(t.TempDir(), "mindrail.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	set, err := migration.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	clock := app.FixedClock{Instant: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)}
	if _, err := migration.New(db.DB, set, clock).Up(t.Context()); err != nil {
		t.Fatal(err)
	}
	store, err := validation.NewStore(db.DB, clock)
	if err != nil {
		t.Fatal(err)
	}
	return evidenceFixture{store: store}
}
