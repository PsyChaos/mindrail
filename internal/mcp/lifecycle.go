package mcp

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/coordination"
)

// insideRoot reports whether an absolute path resolves inside the
// repository root. Scope declaration is worktree-bound: the change store
// leaves containment to callers, and this tool is the caller (Breaker B-1).
func insideRoot(root, path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	return clean == root || strings.HasPrefix(clean, root+string(filepath.Separator))
}

func (s *Server) changeStore() *changes.Store {
	return s.store
}

// projectID resolves a task's project for indexing scope. Unknown tasks
// refuse here with the coordination vocabulary, before any change row.
func (s *Server) projectID(ctx context.Context, taskID string) (string, error) {
	task, err := s.app.Coordination().FindTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	return task.ProjectID, nil
}

func (s *Server) registerLifecycle() {
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolClaim, Description: "Claim an open task under a session."}, s.claim)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolBeforeChange, Description: "Declare the scope a change will cover."}, s.beforeChange)
	sdk.AddTool(s.impl, &sdk.Tool{Name: ToolAfterChange, Description: "Record a change with its attribution findings."}, s.afterChange)
}

// ClaimIn claims an OPEN task under a named session. ExpectedRevision
// absent means an unguarded transition; present means revision-guarded
// (conflict on mismatch, nothing written). OperationID absent means no
// idempotency; present replays the first delivery.
type ClaimIn struct {
	TaskID           string  `json:"task_id"`
	Session          string  `json:"session"`
	ExpectedRevision *int64  `json:"expected_revision,omitempty"`
	OperationID      *string `json:"operation_id,omitempty"`
}

// ClaimOut is the claimed task with its revision and replay posture.
type ClaimOut struct {
	TaskID   string `json:"task_id"`
	State    string `json:"state"`
	Revision int64  `json:"revision"`
	Replayed bool   `json:"replayed"`
}

func (s *Server) claim(ctx context.Context, _ *sdk.CallToolRequest, in ClaimIn) (*sdk.CallToolResult, ClaimOut, error) {
	if in.TaskID == "" || in.Session == "" {
		return nil, ClaimOut{}, Invalid("claim needs a task and a session")
	}
	coord := s.app.Coordination()
	if in.OperationID != nil {
		coord = coord.Idempotent(*in.OperationID)
	}
	by := coordination.NamedSession(in.Session)
	var move coordination.Move
	var write coordination.Write
	var err error
	if in.ExpectedRevision != nil {
		move, write, err = coord.TransitionExpecting(ctx, in.TaskID, by,
			coordination.StateClaimed, "claimed through mindrail_claim", *in.ExpectedRevision)
	} else {
		move, write, err = coord.Transition(ctx, in.TaskID, by,
			coordination.StateClaimed, "claimed through mindrail_claim")
	}
	if err != nil {
		return nil, ClaimOut{}, err
	}
	return nil, ClaimOut{
		TaskID:   move.Task.ID,
		State:    string(move.Task.State),
		Revision: move.Task.Revision,
		Replayed: write.Replayed,
	}, nil
}

// BeforeChangeIn declares scope. Claim is never required: the store needs
// the task to exist, not to be claimed (decision D-197).
type BeforeChangeIn struct {
	TaskID      string   `json:"task_id"`
	Paths       []string `json:"paths"`
	OperationID *string  `json:"operation_id,omitempty"`
}

// BeforeChangeOut is the captured baseline summary.
type BeforeChangeOut struct {
	TaskID string   `json:"task_id"`
	Files  int      `json:"files"`
	Scope  []string `json:"scope"`
}

func (s *Server) beforeChange(ctx context.Context, _ *sdk.CallToolRequest, in BeforeChangeIn) (*sdk.CallToolResult, BeforeChangeOut, error) {
	if in.TaskID == "" || len(in.Paths) == 0 {
		return nil, BeforeChangeOut{}, Invalid("before_change needs a task and paths")
	}
	for _, path := range in.Paths {
		if !insideRoot(s.root, path) {
			return nil, BeforeChangeOut{}, Invalid("before_change scope escapes the repository")
		}
	}
	var operationID string
	if in.OperationID != nil {
		operationID = *in.OperationID
	}
	summary, err := s.changeStore().CaptureBaseline(ctx, in.TaskID, in.Paths, operationID)
	if err != nil {
		return nil, BeforeChangeOut{}, err
	}
	baseline, err := s.changeStore().ReadBaseline(ctx, in.TaskID)
	if err != nil {
		return nil, BeforeChangeOut{}, err
	}
	var scope []string
	for path := range baseline {
		scope = append(scope, path)
	}
	sort.Strings(scope)
	return nil, BeforeChangeOut{TaskID: summary.TaskID, Files: summary.Files, Scope: scope}, nil
}

// AfterChangeIn records a change. OperationID absent runs without
// idempotency; present replays the first delivery.
type AfterChangeIn struct {
	TaskID      string  `json:"task_id"`
	OperationID *string `json:"operation_id,omitempty"`
}

// AttributionFinding is one blocking item with its remedy.
type AttributionFinding struct {
	Code       string   `json:"code"`
	Key        string   `json:"key"`
	NextAction []string `json:"next_action"`
}

// AfterChangeOut is the open change plus its attribution posture: pending
// exactly when blocking findings remain. Replay proves by identity — the
// same change id without duplication — because AfterChange emits no replay
// flag (D-199 narrows to services that emit one; recorded in findings).
type AfterChangeOut struct {
	ChangeID string               `json:"change_id"`
	Files    []string             `json:"files"`
	Symbols  []string             `json:"symbols"`
	Findings []AttributionFinding `json:"findings"`
	Pending  bool                 `json:"pending"`
}

func (s *Server) afterChange(ctx context.Context, _ *sdk.CallToolRequest, in AfterChangeIn) (*sdk.CallToolResult, AfterChangeOut, error) {
	if in.TaskID == "" {
		return nil, AfterChangeOut{}, Invalid("after_change needs a task")
	}
	var operationID string
	if in.OperationID != nil {
		operationID = *in.OperationID
	}
	projectID, err := s.projectID(ctx, in.TaskID)
	if err != nil {
		return nil, AfterChangeOut{}, err
	}
	change, err := s.changes.AfterChange(ctx, projectID, s.root, in.TaskID, operationID)
	if err != nil {
		return nil, AfterChangeOut{}, err
	}
	return s.describeChange(ctx, change.ID, in.TaskID)
}

// describeChange reads a change's files, symbol keys and attribution
// findings into the shared discovery answer shape (after_change and
// reconcile differ in provenance, never in shape).
func (s *Server) describeChange(ctx context.Context, changeID, taskID string) (*sdk.CallToolResult, AfterChangeOut, error) {
	store := s.changeStore()
	files, err := store.ReadChangeFiles(ctx, changeID)
	if err != nil {
		return nil, AfterChangeOut{}, err
	}
	symbols, err := store.ReadChangeSymbols(ctx, changeID)
	if err != nil {
		return nil, AfterChangeOut{}, err
	}
	findings, err := s.changes.EvaluateTask(ctx, taskID, nil)
	if err != nil {
		return nil, AfterChangeOut{}, err
	}
	out := AfterChangeOut{ChangeID: changeID}
	for _, file := range files {
		out.Files = append(out.Files, file.Path)
	}
	for _, symbol := range symbols {
		out.Symbols = append(out.Symbols, symbol.Key)
	}
	for _, finding := range findings {
		out.Findings = append(out.Findings, AttributionFinding{
			Code:       string(finding.Code),
			Key:        finding.Provenance.ChangeKey,
			NextAction: finding.NextAction,
		})
	}
	out.Pending = len(out.Findings) > 0
	if out.Files == nil {
		out.Files = []string{}
	}
	if out.Symbols == nil {
		out.Symbols = []string{}
	}
	if out.Findings == nil {
		out.Findings = []AttributionFinding{}
	}
	return nil, out, nil
}
