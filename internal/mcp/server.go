package mcp

import (
	"context"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/status"
)

// Tool names on the 0.1 wire.
const (
	ToolBootstrap = "mindrail_bootstrap"
	ToolStatus    = "mindrail_status"
	ToolSearch    = "mindrail_search"
	ToolContext   = "mindrail_context"
	ToolDecide    = "mindrail_decide"
	ToolInvariant = "mindrail_invariant"
)

// Server binds six tools to one read-only application over a repository
// root. The application starts once at construction — the same startup the
// CLI runs — and every handler reads from it, so behavior cannot fork
// (decision D-186). Close shuts the application down.
type Server struct {
	impl    *sdk.Server
	app     *bootstrap.App
	root    string
	started time.Time
}

// New starts a read-only application over root and registers the read
// tools. Write tools (decide, invariant) arrive in TASK-02; the six-tool
// registry completes there.
func New(ctx context.Context, root string) (*Server, error) {
	if root == "" {
		return nil, Invalid("server needs a repository root")
	}
	application := bootstrap.New(bootstrap.Options{
		StartDir: root,
		Mode:     bootstrap.ModeReadOnly,
	})
	if err := application.Start(ctx); err != nil {
		return nil, err
	}
	server := &Server{
		impl:    sdk.NewServer(&sdk.Implementation{Name: "mindrail", Version: "0.1"}, nil),
		app:     application,
		root:    root,
		started: time.Now(),
	}
	server.registerReads()
	return server, nil
}

// Close shuts the application down.
func (s *Server) Close(ctx context.Context) error {
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
