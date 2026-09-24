package mcp

import (
	"context"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/status"
)

// Tool names on the 0.1 wire.
const (
	ToolBootstrap    = "mindrail_bootstrap"
	ToolStatus       = "mindrail_status"
	ToolSearch       = "mindrail_search"
	ToolContext      = "mindrail_context"
	ToolDecide       = "mindrail_decide"
	ToolInvariant    = "mindrail_invariant"
	ToolClaim        = "mindrail_claim"
	ToolBeforeChange = "mindrail_before_change"
	ToolAfterChange  = "mindrail_after_change"
	ToolReconcile    = "mindrail_reconcile"
	ToolCheckpoint   = "mindrail_checkpoint"
)

// Server binds eleven tools to one read-write application over a
// repository root. The application starts once at construction in ModeWrite
// (existing database, no creation or migration) — the same startup the CLI
// runs — and every handler reads from it, so behavior cannot fork
// (decision D-186, mode D-195). Close shuts the application down.
type Server struct {
	impl     *sdk.Server
	app      *bootstrap.App
	changes  *changes.Service
	store    *changes.Store
	registry *parser.Registry
	root     string
	started  time.Time
}

// New starts a read-write application over root and registers the tools.
func New(ctx context.Context, root string) (*Server, error) {
	if root == "" {
		return nil, Invalid("server needs a repository root")
	}
	application := bootstrap.New(bootstrap.Options{
		StartDir: root,
		Mode:     bootstrap.ModeWrite,
	})
	if err := application.Start(ctx); err != nil {
		return nil, err
	}
	registry, err := parser.NewRegistry()
	if err != nil {
		return nil, err
	}
	indexes := index.NewStore(application.DB(), app.SystemClock{})
	indexer := index.NewIndexer(indexes, registry, snapshot.New(filesystem.RuntimePaths{
		CacheDir: application.Paths().CacheDir,
	}))
	changeStore, err := changes.NewStore(application.DB(), app.SystemClock{})
	if err != nil {
		registry.Close()
		return nil, err
	}
	changeService, err := changes.New(changeStore, indexes, indexer)
	if err != nil {
		registry.Close()
		return nil, err
	}
	server := &Server{
		impl:     sdk.NewServer(&sdk.Implementation{Name: "mindrail", Version: "0.1"}, nil),
		app:      application,
		changes:  changeService,
		store:    changeStore,
		registry: registry,
		root:     root,
		started:  time.Now(),
	}
	server.registerReads()
	server.registerLifecycle()
	return server, nil
}

// Close shuts the application down and releases parser resources.
func (s *Server) Close(ctx context.Context) error {
	if s.registry != nil {
		s.registry.Close()
	}
	return s.app.Shutdown(ctx)
}

// SDK exposes the underlying server for transports the milestone does not
// own (later process wiring owns serving; tests own in-memory sessions).
func (s *Server) SDK() *sdk.Server {
	return s.impl
}

func (s *Server) registerReads() {
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolBootstrap, Description: "Open a repository session: runtime locations and readiness."}, s.bootstrap)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolStatus, Description: "Readiness report for the repository."}, s.status)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolSearch, Description: "Search knowledge records and declarations."}, s.search)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolContext, Description: "Aggregated repository context at a detail level."}, s.context)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolDecide, Description: "Record a decision."}, s.decide)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolInvariant, Description: "Record an active invariant."}, s.invariant)
}

// BootstrapOut is session-start state: where everything lives and whether
// the repository is ready.
type BootstrapOut struct {
	WorktreeRoot    string   `json:"worktree_root"`
	RuntimeRoot     string   `json:"runtime_root"`
	DBPath          string   `json:"db_path"`
	Readiness       string   `json:"readiness"`
	KnowledgeIssues []string `json:"knowledge_issues,omitempty"`
	Refusal         *Refusal `json:"refusal,omitempty"`
}

func (s *Server) bootstrap(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, BootstrapOut, error) {
	paths := s.app.Paths()
	report := status.Build(s.app.Subject(), time.Since(s.started))
	return nil, BootstrapOut{
		WorktreeRoot: paths.WorktreeRoot,
		RuntimeRoot:  paths.RuntimeRoot,
		DBPath:       paths.DBPath,
		Readiness:    string(report.Readiness),
	}, nil
}

// StatusOut is the CLI's own report struct: identical shape by
// construction, not by parallel implementation.
func (s *Server) status(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, status.Report, error) {
	report := status.Build(s.app.Subject(), time.Since(s.started))
	return nil, report, nil
}
