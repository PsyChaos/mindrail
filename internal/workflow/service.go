// Package workflow composes the automatic agent lifecycle independently of its
// transport. The existing stores remain the authority for leases and revisions.
package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/gate"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
)

type Options struct {
	DB                           *sql.DB
	Root, ProjectID, WorkspaceID string
	Coordination                 *coordination.Store
	Changes                      *changes.Service
	Indexes                      *index.Store
	Validation                   *validation.Service
	Evidence                     *validation.Store
	Guard                        *testguard.Service
	Profiles                     map[string]config.ValidationProfile
	SecretEnv                    []string
	LoadValidationConfig         func() (ValidationConfig, error)
	Clock                        app.Clock
}

// ValidationConfig is the policy input that may change while a long-lived
// agent connection remains open.
type ValidationConfig struct {
	Profiles  map[string]config.ValidationProfile
	SecretEnv []string
}

type Service struct {
	options   Options
	runLocks  sync.Map
	failureMu sync.Mutex
	failures  map[string]error
}

type StartInput struct {
	Goal         string   `json:"goal"`
	RunKey       string   `json:"run_key"`
	Paths        []string `json:"paths,omitempty"`
	ResumeTaskID string   `json:"resume_task_id,omitempty"`
}

type Run struct {
	RunKey    string             `json:"run_key"`
	SessionID string             `json:"session_id"`
	TaskID    string             `json:"task_id"`
	Paths     []string           `json:"paths"`
	State     coordination.State `json:"state"`
	Revision  int64              `json:"revision"`
}

type FinalizeInput struct {
	RunKey string `json:"run_key"`
}
type Finalization struct {
	Run                  Run
	Decision             gate.Decision
	Completed            bool
	Profiles             []string
	NoProfilesConfigured bool
}

func New(o Options) (*Service, error) {
	if o.DB == nil || o.Root == "" || o.ProjectID == "" || o.WorkspaceID == "" || o.Coordination == nil || o.Changes == nil || o.Indexes == nil || o.Validation == nil || o.Evidence == nil || o.Guard == nil {
		return nil, invalid("automatic workflow needs a repository, workspace and all lifecycle services")
	}
	abs, err := filepath.Abs(o.Root)
	if err != nil {
		return nil, err
	}
	o.Root = filepath.Clean(abs)
	if o.Clock == nil {
		o.Clock = app.SystemClock{}
	}
	o.Profiles = cloneProfiles(o.Profiles)
	o.SecretEnv = append([]string(nil), o.SecretEnv...)
	return &Service{options: o, failures: map[string]error{}}, nil
}

func cloneProfiles(source map[string]config.ValidationProfile) map[string]config.ValidationProfile {
	profiles := make(map[string]config.ValidationProfile, len(source))
	for name, p := range source {
		p.Paths = append([]string(nil), p.Paths...)
		p.Commands = append([][]string(nil), p.Commands...)
		for i := range p.Commands {
			p.Commands[i] = append([]string(nil), p.Commands[i]...)
		}
		profiles[name] = p
	}
	return profiles
}

func (s *Service) validationConfig() (ValidationConfig, error) {
	current := ValidationConfig{Profiles: s.options.Profiles, SecretEnv: s.options.SecretEnv}
	if s.options.LoadValidationConfig != nil {
		loaded, err := s.options.LoadValidationConfig()
		if err != nil {
			return ValidationConfig{}, err
		}
		current = loaded
	}
	current.Profiles = cloneProfiles(current.Profiles)
	current.SecretEnv = append([]string(nil), current.SecretEnv...)
	return current, nil
}

func invalid(why string) error {
	return app.NewError(app.CodeCommandLineInvalid, app.KindUsage, why, "The automatic workflow did not complete.", "Correct the request and retry with the same run_key; resume interrupted work explicitly when needed.")
}

func (s *Service) normalizePaths(paths []string) ([]string, error) {
	set := map[string]bool{}
	for _, raw := range paths {
		if strings.TrimSpace(raw) == "" {
			return nil, invalid("scope contains an empty path")
		}
		abs := raw
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(s.options.Root, filepath.FromSlash(raw))
		}
		abs = filepath.Clean(abs)
		rel, err := filepath.Rel(s.options.Root, abs)
		if err != nil {
			return nil, err
		}
		target, err := coordination.FileTarget(filepath.ToSlash(rel))
		if err != nil {
			return nil, err
		}
		if target.Key == ".git" || strings.HasPrefix(target.Key, ".git/") || target.Key == ".mindrail" || strings.HasPrefix(target.Key, ".mindrail/") {
			return nil, invalid("scope cannot contain Mindrail or Git metadata")
		}
		for p := abs; p != s.options.Root; p = filepath.Dir(p) {
			info, err := os.Lstat(p)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, invalid("automatic scope cannot traverse symbolic links: " + raw)
			}
			if p == abs && !info.Mode().IsRegular() {
				return nil, invalid("automatic scope must name files: " + raw)
			}
		}
		set[abs] = true
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// Start records immutable start intent before creating any identity. Every
// subsequent phase uses a deterministic operation key, so a retry continues a
// partial start and never mints an extra session or task for the logical run.
func (s *Service) Start(ctx context.Context, in StartInput) (Run, error) {
	unlock := s.lockRun(in.RunKey)
	defer unlock()
	in.Goal = strings.TrimSpace(in.Goal)
	if in.Goal == "" || strings.TrimSpace(in.RunKey) == "" || len(in.RunKey) > 1024 {
		return Run{}, invalid("automatic bootstrap needs a goal and a run_key of at most 1024 bytes")
	}
	paths, err := s.normalizePaths(in.Paths)
	if err != nil {
		return Run{}, err
	}
	in.Paths = paths
	if in.ResumeTaskID != "" {
		_, _, replay, err := s.lookupStart(ctx, in.RunKey)
		if err != nil {
			return Run{}, err
		}
		t, err := s.options.Coordination.FindTask(ctx, in.ResumeTaskID)
		if err != nil {
			return Run{}, err
		}
		if t.ProjectID != s.options.ProjectID || (t.State.Terminal() && !replay) {
			return Run{}, invalid("resume_task_id must name a non-terminal task in this project")
		}
	}
	j, err := s.rememberStart(ctx, in)
	if err != nil {
		return Run{}, err
	}
	r := Run{RunKey: in.RunKey, Paths: append([]string(nil), in.Paths...)}
	if err := s.health(ctx, in.RunKey); err != nil {
		if recovered, recoverErr := s.resolve(ctx, j); recoverErr == nil {
			r = recovered
		}
		return r, err
	}
	session, _, err := s.options.Coordination.Idempotent(s.operation(in.RunKey, "session")).OpenSession(ctx, s.options.WorkspaceID, "automatic: "+in.Goal)
	if err != nil {
		return r, err
	}
	r.SessionID = session.ID
	by := coordination.NamedSession(session.ID)
	var task coordination.Task
	if in.ResumeTaskID != "" {
		task, err = s.options.Coordination.FindTask(ctx, in.ResumeTaskID)
	} else {
		task, _, err = s.options.Coordination.Idempotent(s.operation(in.RunKey, "task")).OpenTask(ctx, s.options.ProjectID, by, in.Goal)
	}
	if err != nil {
		return r, err
	}
	r.TaskID = task.ID
	// OpenTask replay contains the original OPEN row, not current task state.
	task, err = s.options.Coordination.FindTask(ctx, task.ID)
	if err != nil {
		return r, err
	}
	r.State, r.Revision = task.State, task.Revision
	if task.State == coordination.StateCompleted {
		return s.resolve(ctx, j)
	}
	if task.State.Terminal() {
		return r, invalid("automatic task is terminal")
	}
	if _, err := s.options.Changes.Store().EnsureGuardBaseline(ctx, s.options.ProjectID, s.options.Root, git.NewExecRunner()); err != nil {
		return r, err
	}
	if task.State == coordination.StateOpen {
		move, _, err := s.options.Coordination.Idempotent(s.operation(in.RunKey, "claim")).TransitionExpecting(ctx, task.ID, by, coordination.StateClaimed, "automatic start", task.Revision)
		if err != nil {
			return r, err
		}
		task = move.Task
	} else {
		acquired, _, err := s.options.Coordination.AcquireLease(ctx, s.options.ProjectID, by, coordination.TaskTarget(task.ID))
		if err != nil {
			return r, err
		}
		task = *acquired.Task
	}
	r.State, r.Revision = task.State, task.Revision
	if task.State == coordination.StateClaimed || task.State == coordination.StateBlocked || task.State == coordination.StateReadyToComplete {
		move, _, err := s.options.Coordination.Idempotent(s.operation(in.RunKey, fmt.Sprintf("working:%d", task.Revision))).TransitionExpecting(ctx, task.ID, by, coordination.StateInProgress, "automatic work", task.Revision)
		if err != nil {
			return r, err
		}
		r.State, r.Revision = move.Task.State, move.Task.Revision
	}
	baseline, err := s.options.Changes.Store().ReadBaseline(ctx, r.TaskID)
	if err != nil {
		return r, err
	}
	for p := range baseline {
		paths = append(paths, p)
	}
	paths, err = s.normalizePaths(paths)
	if err != nil {
		return r, err
	}
	if err := s.acquireFiles(ctx, r, paths); err != nil {
		return r, err
	}
	if _, err := s.options.Changes.Store().ExtendBaseline(ctx, r.TaskID, paths); err != nil {
		return r, err
	}
	return s.resolve(ctx, j)
}

func (s *Service) Resolve(ctx context.Context, key string) (Run, error) {
	j, err := s.loadStart(ctx, key)
	if err != nil {
		return Run{}, err
	}
	return s.resolve(ctx, j)
}

func (s *Service) resolve(ctx context.Context, j journal) (Run, error) {
	r := Run{RunKey: j.Input.RunKey, TaskID: j.Input.ResumeTaskID, Paths: []string{}}
	var session coordination.Session
	if found, err := s.readPhase(ctx, r.RunKey, "session", &session); err != nil {
		return r, err
	} else if !found {
		return r, invalid("automatic start is incomplete; retry bootstrap with the same run_key")
	}
	r.SessionID = session.ID
	if r.TaskID == "" {
		var task coordination.Task
		if found, err := s.readPhase(ctx, r.RunKey, "task", &task); err != nil {
			return r, err
		} else if !found {
			return r, invalid("automatic task creation is incomplete; retry bootstrap")
		}
		r.TaskID = task.ID
	}
	task, err := s.options.Coordination.FindTask(ctx, r.TaskID)
	if err != nil {
		return r, err
	}
	if task.ProjectID != s.options.ProjectID {
		return r, invalid("automatic task belongs to another project")
	}
	r.State, r.Revision = task.State, task.Revision
	baseline, err := s.options.Changes.Store().ReadBaseline(ctx, r.TaskID)
	if err != nil {
		return r, err
	}
	for p := range baseline {
		r.Paths = append(r.Paths, p)
	}
	sort.Strings(r.Paths)
	return r, nil
}

func (s *Service) acquireFiles(ctx context.Context, r Run, paths []string) error {
	for _, p := range paths {
		rel, err := filepath.Rel(s.options.Root, p)
		if err != nil {
			return err
		}
		target, err := coordination.FileTarget(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if _, _, err := s.options.Coordination.AcquireLease(ctx, s.options.ProjectID, coordination.NamedSession(r.SessionID), target); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ExtendScope(ctx context.Context, key string, paths []string) (Run, error) {
	unlock := s.lockRun(key)
	defer unlock()
	r, err := s.Resolve(ctx, key)
	if err != nil {
		return r, err
	}
	if err := s.health(ctx, key); err != nil {
		return r, err
	}
	if r.State.Terminal() {
		return r, invalid("cannot extend a terminal task")
	}
	paths, err = s.normalizePaths(paths)
	if err != nil {
		return r, err
	}
	if err := s.Renew(ctx, key); err != nil {
		return r, err
	}
	if err := s.acquireFiles(ctx, r, paths); err != nil {
		return r, err
	}
	if _, err := s.options.Changes.Store().ExtendBaseline(ctx, r.TaskID, paths); err != nil {
		return r, err
	}
	return s.Resolve(ctx, key)
}

func (s *Service) scoped(ctx context.Context, r Run) (*changes.Service, error) {
	j, err := s.loadStart(ctx, r.RunKey)
	if err != nil {
		return nil, err
	}
	leases, err := s.options.Coordination.ListLeases(ctx, s.options.ProjectID)
	if err != nil {
		return nil, err
	}
	scope := changes.AutomaticScope{Initial: j.Initial, StartedAt: j.StartedAt}
	for _, l := range leases {
		if l.Holder != r.SessionID && l.TargetKind == coordination.TargetFile {
			scope.ForeignPaths = append(scope.ForeignPaths, filepath.Join(s.options.Root, filepath.FromSlash(l.TargetKey)))
		}
	}
	return s.options.Changes.WithAutomaticScope(scope), nil
}

func (s *Service) Reconcile(ctx context.Context, key string) (changes.ReconcileResult, error) {
	unlock := s.lockRun(key)
	defer unlock()
	r, err := s.Resolve(ctx, key)
	if err != nil {
		return changes.ReconcileResult{}, err
	}
	if r.State.Terminal() {
		return changes.ReconcileResult{}, invalid("cannot reconcile a terminal automatic task")
	}
	if err := s.Renew(ctx, key); err != nil {
		return changes.ReconcileResult{}, err
	}
	view, err := s.scoped(ctx, r)
	if err != nil {
		return changes.ReconcileResult{}, err
	}
	return s.reconcile(ctx, r, view)
}

func (s *Service) lockRun(key string) func() {
	value, _ := s.runLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
