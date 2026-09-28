package mcp

import (
	"context"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/agent"
	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/credential"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/status"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
	"github.com/PsyChaos/mindrail/internal/workflow"
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

// Server binds the fourteen tools to one read-write application over a
// repository root. The application starts once at construction in ModeWrite
// (existing database, no creation or migration) — the same startup the CLI
// runs — and every handler reads from it, so behavior cannot fork
// (decision D-186, mode D-195). Close shuts the application down.
type Server struct {
	impl                             *sdk.Server
	app                              *bootstrap.App
	changes                          *changes.Service
	store                            *changes.Store
	valid                            *validation.Service
	evidence                         *validation.Store
	indexes                          *index.Store
	guard                            *testguard.Service
	registry                         *parser.Registry
	root                             string
	started                          time.Time
	workflow                         workflowPort
	autoMu                           sync.RWMutex
	autoStartMu                      sync.Mutex
	automatic                        map[*sdk.ServerSession]*automaticContext
	router                           routeRunner
	routes                           routeRecorder
	presence                         presenceRecorder
	clock                            app.Clock
	projectIDValue, workspaceIDValue string
	routeMu                          sync.Mutex
	routeSessions                    map[string]*routeSessionState
	presenceHeartbeatTestInterval    time.Duration
}

// ValidationTimeout bounds one profile run through the tool. Generous by
// design: profiles run real test suites, and the timeout is a hang guard,
// never a performance assertion.
const ValidationTimeout = 10 * time.Minute

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
	// Startup accepts any directory within a worktree. Every downstream
	// filesystem, validation and workflow service uses the discovered root.
	root = application.Paths().WorktreeRoot
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
	runner, err := validation.NewRunner(root, ValidationTimeout, 0)
	if err != nil {
		registry.Close()
		return nil, err
	}
	evidenceStore, err := validation.NewStore(application.DB(), app.SystemClock{})
	if err != nil {
		registry.Close()
		return nil, err
	}
	valid, err := validation.NewService(runner, evidenceStore)
	if err != nil {
		registry.Close()
		return nil, err
	}
	guard, err := testguard.New()
	if err != nil {
		registry.Close()
		return nil, err
	}
	routeStore, err := agent.NewRouteStore(application.DB())
	if err != nil {
		guard.Close()
		registry.Close()
		return nil, err
	}
	presenceStore, err := agent.NewPresenceStore(application.DB(), app.SystemClock{})
	if err != nil {
		guard.Close()
		registry.Close()
		return nil, err
	}
	space := application.Subject().Workspace
	flow, err := workflow.New(workflow.Options{
		DB:           application.DB(),
		Root:         root,
		ProjectID:    space.ProjectID,
		WorkspaceID:  space.ID,
		Coordination: application.Coordination(),
		Changes:      changeService,
		Indexes:      indexes,
		Validation:   valid,
		Evidence:     evidenceStore,
		Guard:        guard,
		Profiles:     application.Config().Config.Validation,
		SecretEnv:    application.Config().Config.Secrets.Env,
		LoadValidationConfig: func() (workflow.ValidationConfig, error) {
			loaded, err := application.CurrentConfig()
			if err != nil {
				return workflow.ValidationConfig{}, err
			}
			return workflow.ValidationConfig{
				Profiles: loaded.Config.Validation, SecretEnv: loaded.Config.Secrets.Env,
			}, nil
		},
	})
	if err != nil {
		guard.Close()
		registry.Close()
		return nil, err
	}
	server := &Server{
		impl:      sdk.NewServer(&sdk.Implementation{Name: "mindrail", Version: "0.1"}, nil),
		app:       application,
		changes:   changeService,
		store:     changeStore,
		valid:     valid,
		evidence:  evidenceStore,
		indexes:   indexes,
		guard:     guard,
		registry:  registry,
		root:      root,
		started:   time.Now(),
		workflow:  workflowAdapter{flow},
		automatic: make(map[*sdk.ServerSession]*automaticContext),
		router:    agent.NewRouter(credential.NewOSStore()), routes: routeStore, presence: presenceStore,
		clock: app.SystemClock{}, projectIDValue: space.ProjectID, workspaceIDValue: space.ID,
		routeSessions: make(map[string]*routeSessionState),
	}
	server.impl.AddReceivingMiddleware(server.presenceActivityMiddleware)
	server.registerReads()
	server.registerLifecycle()
	server.registerDiscovery()
	server.registerProving()
	server.registerRoute()
	return server, nil
}

// Close shuts the application down and releases parser and guard resources.
func (s *Server) Close(ctx context.Context) error {
	s.closeAutomatic()
	if s.registry != nil {
		s.registry.Close()
	}
	if s.guard != nil {
		s.guard.Close()
	}
	return s.app.Shutdown(ctx)
}

// SDK exposes the underlying server for transports the milestone does not
// own (later process wiring owns serving; tests own in-memory sessions).
func (s *Server) SDK() *sdk.Server {
	return s.impl
}

func (s *Server) registerReads() {
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolBootstrap, Description: "Read repository readiness, or start/recover automatic work with goal and run_key. Declare paths now, or call before_change before the first edit."}, s.bootstrap)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolStatus, Description: "Readiness report for the repository."}, s.status)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolSearch, Description: "Search knowledge records and declarations."}, s.search)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolContext, Description: "Aggregated repository context at a detail level."}, s.context)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolDecide, Description: "Record a decision."}, s.decide)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolInvariant, Description: "Record an active invariant."}, s.invariant)
}

// BootstrapOut is session-start state: where everything lives and whether
// the repository is ready.
type BootstrapOut struct {
	WorktreeRoot    string            `json:"worktree_root"`
	RuntimeRoot     string            `json:"runtime_root"`
	DBPath          string            `json:"db_path"`
	Readiness       string            `json:"readiness"`
	KnowledgeIssues []string          `json:"knowledge_issues,omitempty"`
	Refusal         *Refusal          `json:"refusal,omitempty"`
	Automatic       bool              `json:"automatic,omitempty"`
	Started         bool              `json:"started,omitempty"`
	RunKey          string            `json:"run_key,omitempty"`
	SessionID       string            `json:"session_id,omitempty"`
	TaskID          string            `json:"task_id,omitempty"`
	State           string            `json:"state,omitempty"`
	Revision        int64             `json:"revision,omitempty"`
	Paths           []string          `json:"paths,omitempty"`
	Failure         *AutomaticFailure `json:"failure,omitempty"`
}

// AutomaticFailure preserves the actionable domain error alongside any
// durable run state an interrupted automatic start already created.
type AutomaticFailure struct {
	Code       string            `json:"code,omitempty"`
	Message    string            `json:"message"`
	Why        string            `json:"why,omitempty"`
	Impact     string            `json:"impact,omitempty"`
	NextAction []string          `json:"next_action"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// BootstrapIn selects the additive automatic mode when any field is present.
// The historical empty object remains the read-only bootstrap operation.
type BootstrapIn struct {
	Goal         *string   `json:"goal,omitempty"`
	RunKey       *string   `json:"run_key,omitempty"`
	Paths        *[]string `json:"paths,omitempty"`
	ResumeTaskID *string   `json:"resume_task_id,omitempty"`
}

func (in BootstrapIn) automatic() bool {
	return in.Goal != nil || in.RunKey != nil || in.Paths != nil || in.ResumeTaskID != nil
}

func (s *Server) bootstrap(ctx context.Context, req *sdk.CallToolRequest, in BootstrapIn) (*sdk.CallToolResult, BootstrapOut, error) {
	if in.automatic() {
		if in.Goal == nil || in.RunKey == nil || *in.Goal == "" || *in.RunKey == "" {
			return nil, BootstrapOut{}, Invalid("automatic bootstrap needs goal and run_key")
		}
		out, err := s.automaticStart(ctx, req, in)
		return nil, out, err
	}
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
