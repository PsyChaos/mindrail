package workflow_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
	"github.com/PsyChaos/mindrail/internal/workflow"
)

type fixture struct {
	service *workflow.Service
	options workflow.Options
	app     *bootstrap.App
	root    string
}

type rejectingResumeAuthorizer struct{ calls atomic.Int32 }

func (a *rejectingResumeAuthorizer) AuthorizeResume(context.Context, string, string) error {
	a.calls.Add(1)
	return fmt.Errorf("reserved")
}
func (a *rejectingResumeAuthorizer) BindResume(context.Context, string, string, string) error {
	return fmt.Errorf("reserved")
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, out)
		}
	}
	git("init", "-q")
	write(t, root, "a.txt", "first\n")
	write(t, root, "b.txt", "first\n")
	git("add", ".")
	git("-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-qm", "base")
	a := bootstrap.New(bootstrap.Options{StartDir: root, Mode: bootstrap.ModeInit})
	if err := a.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })
	clock := app.SystemClock{}
	is := index.NewStore(a.DB(), clock)
	reg, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reg.Close)
	cs, err := changes.NewStore(a.DB(), clock)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := changes.New(cs, is, index.NewIndexer(is, reg, snapshot.New(filesystem.RuntimePaths{CacheDir: t.TempDir()})))
	if err != nil {
		t.Fatal(err)
	}
	es, err := validation.NewStore(a.DB(), clock)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := validation.NewRunner(root, time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	vs, err := validation.NewService(runner, es)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := testguard.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(guard.Close)
	o := workflow.Options{DB: a.DB(), Root: root, ProjectID: a.Subject().Workspace.ProjectID, WorkspaceID: a.Subject().Workspace.ID, Coordination: a.Coordination(), Changes: svc, Indexes: is, Validation: vs, Evidence: es, Guard: guard}
	s, err := workflow.New(o)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{s, o, a, root}
}

func write(t *testing.T, root, path, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
func start(t *testing.T, f fixture, key string, paths ...string) workflow.Run {
	t.Helper()
	r, err := f.service.Start(t.Context(), workflow.StartInput{Goal: "Change files", RunKey: key, Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestStartRetryRestartAndFreshRuns(t *testing.T) {
	f := newFixture(t)
	one := start(t, f, "retry", "a.txt")
	s, err := workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.service = s
	two := start(t, f, "retry", "a.txt")
	if one.TaskID != two.TaskID || one.SessionID != two.SessionID || one.Revision != two.Revision {
		t.Fatalf("retry changed identity/revision: %+v %+v", one, two)
	}
	other := start(t, f, "other", "b.txt")
	if other.SessionID == one.SessionID {
		t.Fatal("new run reused session")
	}
	if _, err := s.Start(t.Context(), workflow.StartInput{Goal: "Different goal", RunKey: "retry", Paths: []string{"a.txt"}}); err == nil {
		t.Fatal("conflicting run key accepted")
	}
}

func TestResumeAuthorizationRunsBeforeSuccessorIdentityIsCreated(t *testing.T) {
	f := newFixture(t)
	first := start(t, f, "predecessor", "a.txt")
	if _, err := f.service.Checkpoint(t.Context(), first.RunKey, "handoff", true); err != nil {
		t.Fatal(err)
	}
	authorizer := &rejectingResumeAuthorizer{}
	f.options.ResumeAuthorizer = authorizer
	service, err := workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(t.Context(), workflow.StartInput{Goal: "resume", RunKey: "successor", ResumeTaskID: first.TaskID}); err == nil {
		t.Fatal("reserved continuity task was resumed without authorization")
	}
	if authorizer.calls.Load() != 1 {
		t.Fatalf("authorization calls=%d", authorizer.calls.Load())
	}
}

func TestCompleteIsScopedRepeatableAndReleasesFiles(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "b.txt", "pre-existing dirt\n")
	r := start(t, f, "one", "a.txt")
	write(t, f.root, "a.txt", "task one\n")
	out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || !out.Completed {
		t.Fatalf("finish: %+v %v", out, err)
	}
	if !out.NoProfilesConfigured {
		t.Fatal("missing no-profile explanation")
	}
	again, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || !again.Completed || again.Run.Revision != out.Run.Revision {
		t.Fatalf("repeat: %+v %v", again, err)
	}
	second := start(t, f, "two", "a.txt")
	write(t, f.root, "a.txt", "task two\n")
	out, err = f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: second.RunKey})
	if err != nil || !out.Completed {
		t.Fatalf("second finish: %+v %v", out, err)
	}
	files, err := f.options.Changes.Store().ListTaskChanges(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("historical changes lost: %v", files)
	}
}

func TestScopeExtensionPreservesEarlierBaseline(t *testing.T) {
	f := newFixture(t)
	r := start(t, f, "extend", "a.txt")
	write(t, f.root, "a.txt", "changed\n")
	if _, err := f.service.ExtendScope(t.Context(), r.RunKey, []string{"b.txt"}); err != nil {
		t.Fatal(err)
	}
	write(t, f.root, "b.txt", "also changed\n")
	delta, err := f.service.Reconcile(t.Context(), r.RunKey)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.options.Changes.Store().ReadChangeFiles(t.Context(), delta.Change.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("lost earlier delta: %v %v", rows, err)
	}
}

func TestConflictingStartReportsRecoverableTask(t *testing.T) {
	f := newFixture(t)
	start(t, f, "owner", "a.txt")
	r, err := f.service.Start(t.Context(), workflow.StartInput{Goal: "Other", RunKey: "contender", Paths: []string{"a.txt"}})
	if err == nil || r.TaskID == "" || r.SessionID == "" {
		t.Fatalf("missing partial context: %+v %v", r, err)
	}
}

func TestRenewDoesNotChangeRevisionAndLossBlocksCompletion(t *testing.T) {
	f := newFixture(t)
	r := start(t, f, "renew", "a.txt")
	if err := f.service.Renew(t.Context(), r.RunKey); err != nil {
		t.Fatal(err)
	}
	after, err := f.service.Resolve(t.Context(), r.RunKey)
	if err != nil || after.Revision != r.Revision {
		t.Fatalf("renew changed task: %+v %v", after, err)
	}
	leases, err := f.options.Coordination.ListLeases(t.Context(), f.options.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range leases {
		if l.Holder == r.SessionID && l.TargetKind == coordination.TargetFile {
			if _, _, err := f.options.Coordination.ReleaseLease(t.Context(), l.ID, coordination.NamedSession(r.SessionID)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := f.service.Renew(t.Context(), r.RunKey); err == nil {
		t.Fatal("lost file lease accepted")
	}
	if out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey}); err == nil || out.Completed {
		t.Fatalf("lease loss completed: %+v %v", out, err)
	}
}

func TestProfilesSortedAndFailedEvidenceDenies(t *testing.T) {
	f := newFixture(t)
	f.options.Profiles = map[string]config.ValidationProfile{"z": {Type: "unit", Paths: []string{"a.txt"}, Commands: [][]string{{"false"}}}, "a": {Type: "unit", Paths: []string{"a.txt"}, Commands: [][]string{{"true"}}}}
	s, err := workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.service = s
	r := start(t, f, "profiles", "a.txt")
	out, err := s.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || out.Completed || out.Decision.Allow || !reflect.DeepEqual(out.Profiles, []string{"a", "z"}) {
		t.Fatalf("failure: %+v %v", out, err)
	}
}

func TestFinalizationReplayPreservesHistoricallyExecutedProfiles(t *testing.T) {
	for _, withProfile := range []bool{false, true} {
		t.Run(fmt.Sprint(withProfile), func(t *testing.T) {
			f := newFixture(t)
			if withProfile {
				f.options.Profiles = map[string]config.ValidationProfile{"original": {Type: "unit", Paths: []string{"a.txt"}, Commands: [][]string{{"true"}}}}
			}
			var err error
			f.service, err = workflow.New(f.options)
			if err != nil {
				t.Fatal(err)
			}
			run := start(t, f, "historical", "a.txt")
			first, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: run.RunKey})
			if err != nil || !first.Completed {
				t.Fatalf("first finalization: %+v %v", first, err)
			}
			f.options.Profiles = map[string]config.ValidationProfile{"new-failing-check": {Type: "unit", Paths: []string{"a.txt"}, Commands: [][]string{{"false"}}}}
			restarted, err := workflow.New(f.options)
			if err != nil {
				t.Fatal(err)
			}
			for _, service := range []*workflow.Service{f.service, restarted} {
				replay, err := service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: run.RunKey})
				if err != nil || !replay.Completed || replay.Run.Revision != first.Run.Revision || !reflect.DeepEqual(replay.Profiles, first.Profiles) || replay.NoProfilesConfigured != first.NoProfilesConfigured {
					t.Fatalf("historical validation report changed: first=%+v replay=%+v err=%v", first, replay, err)
				}
			}
			var newEvidence int
			if err := f.app.DB().QueryRow(`SELECT count(*) FROM evidence WHERE profile='new-failing-check'`).Scan(&newEvidence); err != nil || newEvidence != 0 {
				t.Fatalf("terminal replay ran a newly configured profile: %d %v", newEvidence, err)
			}
		})
	}
}

type renewalBarrierClock struct {
	entered, release chan struct{}
	blocked          atomic.Bool
}

func (c *renewalBarrierClock) Now() time.Time {
	if c.blocked.CompareAndSwap(false, true) {
		close(c.entered)
		<-c.release
	}
	return time.Now()
}

func TestRenewalAfterConcurrentCompletionDoesNotPoisonReplay(t *testing.T) {
	f := newFixture(t)
	run := start(t, f, "renewal-barrier", "a.txt")
	barrier := &renewalBarrierClock{entered: make(chan struct{}), release: make(chan struct{})}
	options := f.options
	options.Coordination = coordination.NewStore(f.app.DB(), barrier)
	renewer, err := workflow.New(options)
	if err != nil {
		t.Fatal(err)
	}
	var release sync.Once
	unblock := func() { release.Do(func() { close(barrier.release) }) }
	defer unblock()
	result := make(chan error, 1)
	go func() { result <- renewer.Renew(t.Context(), run.RunKey) }()
	select {
	case <-barrier.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("renewal did not pause after resolving active state")
	}
	finished, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: run.RunKey})
	if err != nil || !finished.Completed {
		t.Fatalf("completion: %+v %v", finished, err)
	}
	unblock()
	if err := <-result; err != nil {
		t.Fatalf("normal completion was mistaken for lease loss: %v", err)
	}
	restarted, err := workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []*workflow.Service{renewer, restarted} {
		replayed, err := service.Start(t.Context(), workflow.StartInput{Goal: "Change files", RunKey: run.RunKey, Paths: []string{"a.txt"}})
		if err != nil || replayed.State != coordination.StateCompleted {
			t.Fatalf("completion poisoned bootstrap replay: %+v %v", replayed, err)
		}
	}
}

func TestHeartbeatConcurrentFinalizationStressPreservesReplay(t *testing.T) {
	f := newFixture(t)
	for i := range 30 {
		key := fmt.Sprintf("heartbeat-finish-%d", i)
		start(t, f, key, "a.txt")
		heartbeat, err := f.service.StartHeartbeat(t.Context(), key, time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		finished, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: key})
		heartbeat.Stop()
		if err != nil || !finished.Completed || heartbeat.Health() != nil {
			t.Fatalf("iteration %d completion=%+v error=%v heartbeat=%v", i, finished, err, heartbeat.Health())
		}
		restarted, err := workflow.New(f.options)
		if err != nil {
			t.Fatal(err)
		}
		for _, service := range []*workflow.Service{f.service, restarted} {
			out, err := service.Start(t.Context(), workflow.StartInput{Goal: "Change files", RunKey: key, Paths: []string{"a.txt"}})
			if err != nil || out.State != coordination.StateCompleted {
				t.Fatalf("iteration %d terminal replay poisoned: %+v %v", i, out, err)
			}
		}
	}
}

func TestConcurrentDuplicateStartConverges(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	results := make(chan workflow.Run, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := f.service.Start(t.Context(), workflow.StartInput{Goal: "same", RunKey: "concurrent", Paths: []string{"a.txt"}})
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first workflow.Run
	for r := range results {
		if first.TaskID != "" && r.TaskID != first.TaskID {
			t.Fatal("duplicate task")
		}
		first = r
	}
}

func TestDisjointRunsCanFinishAfterEachOther(t *testing.T) {
	f := newFixture(t)
	one := start(t, f, "parallel-one", "a.txt")
	two := start(t, f, "parallel-two", "b.txt")
	write(t, f.root, "a.txt", "first agent\n")
	write(t, f.root, "b.txt", "second agent\n")
	for _, run := range []workflow.Run{one, two} {
		out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: run.RunKey})
		if err != nil || !out.Completed {
			t.Fatalf("disjoint finish %s: %+v %v", run.RunKey, out, err)
		}
	}
}

func TestHandoffResumeAndCompletedBootstrapRetry(t *testing.T) {
	f := newFixture(t)
	r := start(t, f, "departing", "a.txt")
	note, err := f.service.Checkpoint(t.Context(), r.RunKey, "continue this work", true)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.service.Checkpoint(t.Context(), r.RunKey, "continue this work", true)
	if err != nil || again.Checkpoint.ID != note.Checkpoint.ID {
		t.Fatalf("checkpoint replay: %+v %v", again, err)
	}
	if _, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey}); err == nil {
		t.Fatal("handed off run finalized")
	}
	in := workflow.StartInput{RunKey: "arriving", Goal: "Continue work", ResumeTaskID: r.TaskID}
	next, err := f.service.Start(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	if next.TaskID != r.TaskID || next.SessionID == r.SessionID {
		t.Fatalf("bad handoff identity: %+v", next)
	}
	write(t, f.root, "a.txt", "resumed\n")
	out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: next.RunKey})
	if err != nil || !out.Completed {
		t.Fatalf("resumed finish: %+v %v", out, err)
	}
	if _, err := f.service.Start(t.Context(), in); err != nil {
		t.Fatalf("completed bootstrap retry: %v", err)
	}
}

func TestSequentialTasksSameParsedSymbolRemainUnambiguous(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "pyproject.toml", "[project]\nname = \"test\"\n")
	write(t, f.root, "a.py", "def f():\n    return 1\n")
	if _, err := f.options.Indexes.UpsertUnit(t.Context(), f.root, index.UnitPython); err != nil {
		t.Fatal(err)
	}
	for i, key := range []string{"symbol-one", "symbol-two"} {
		r := start(t, f, key, "a.py")
		write(t, f.root, "a.py", fmt.Sprintf("def f():\n    return %d\n", i+2))
		out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
		if err != nil || !out.Completed {
			t.Fatalf("symbol finish: %+v %v", out, err)
		}
	}
}

func TestPreexistingDirtyChangedOutsideScopeIsDenied(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "b.txt", "old dirt\n")
	r := start(t, f, "drift", "a.txt")
	write(t, f.root, "b.txt", "new unscheduled dirt\n")
	out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || out.Completed || out.Decision.Allow {
		t.Fatalf("scope drift accepted: %+v %v", out, err)
	}
}

func TestPartialStartRecoversAfterConflictEnds(t *testing.T) {
	f := newFixture(t)
	owner := start(t, f, "owner-finish", "a.txt")
	in := workflow.StartInput{RunKey: "waiting", Goal: "Later work", Paths: []string{"a.txt"}}
	partial, err := f.service.Start(t.Context(), in)
	if err == nil {
		t.Fatal("expected conflict")
	}
	if _, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: owner.RunKey}); err != nil {
		t.Fatal(err)
	}
	s, err := workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Start(t.Context(), in)
	if err != nil || r.TaskID != partial.TaskID || r.SessionID != partial.SessionID {
		t.Fatalf("partial recovery: %+v %v", r, err)
	}
}

func TestHeartbeatStopsAndRenewalFailureSurvivesRestart(t *testing.T) {
	f := newFixture(t)
	r := start(t, f, "beat", "a.txt")
	h, err := f.service.StartHeartbeat(t.Context(), r.RunKey, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	h.Stop()
	if err := h.Health(); err != nil {
		t.Fatal(err)
	}
	before, err := f.options.Coordination.Handover(t.Context(), r.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Lease == nil {
		t.Fatal("missing lease")
	}
	if _, _, err := f.options.Coordination.ReleaseLease(t.Context(), before.Lease.ID, coordination.NamedSession(r.SessionID)); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Renew(t.Context(), r.RunKey); err == nil {
		t.Fatal("renew accepted released task lease")
	}
	restarted, err := workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := restarted.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey}); err == nil || out.Completed {
		t.Fatalf("restart cleared failure: %+v %v", out, err)
	}
}

func TestIndependentServicesDuplicateStartConverges(t *testing.T) {
	f := newFixture(t)
	other, err := workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan workflow.Run, 2)
	errs := make(chan error, 2)
	for _, service := range []*workflow.Service{f.service, other} {
		wg.Add(1)
		go func(service *workflow.Service) {
			defer wg.Done()
			r, e := service.Start(t.Context(), workflow.StartInput{Goal: "concurrent", RunKey: "cross-service", Paths: []string{"a.txt"}})
			results <- r
			errs <- e
		}(service)
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first workflow.Run
	for r := range results {
		if first.TaskID != "" && (first.TaskID != r.TaskID || first.SessionID != r.SessionID) {
			t.Fatal("identity diverged")
		}
		first = r
	}
}

func TestScopeRejectsEscapesDirectoriesAndSymlinksWithoutRows(t *testing.T) {
	f := newFixture(t)
	if err := os.Symlink(filepath.Join(f.root, "a.txt"), filepath.Join(f.root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside", ".", ".git/config", ".mindrail/config.toml", "link.txt"} {
		if _, err := f.service.Start(t.Context(), workflow.StartInput{Goal: "bad scope", RunKey: "bad-" + path, Paths: []string{path}}); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	var count int
	if err := f.options.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM sessions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid scope minted %d sessions", count)
	}
}

func TestRestoringPreexistingDirtOutsideScopeIsDenied(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "b.txt", "old dirt\n")
	r := start(t, f, "restore-drift", "a.txt")
	write(t, f.root, "b.txt", "first\n") // no longer appears in git status
	out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || out.Completed || out.Decision.Allow {
		t.Fatalf("restoration outside scope accepted: %+v %v", out, err)
	}
}

func TestHistoricalCompletedBytesDoNotHideNewScopeDrift(t *testing.T) {
	f := newFixture(t)
	old := start(t, f, "historical", "b.txt")
	write(t, f.root, "b.txt", "historical change\n")
	out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: old.RunKey})
	if err != nil || !out.Completed {
		t.Fatalf("old finish: %+v %v", out, err)
	}
	write(t, f.root, "b.txt", "first\n")
	next := start(t, f, "new-drift", "a.txt")
	write(t, f.root, "b.txt", "historical change\n")
	out, err = f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: next.RunKey})
	if err != nil || out.Completed || out.Decision.Allow {
		t.Fatalf("historic bytes hid new drift: %+v %v", out, err)
	}
}

func TestStaleRevisionDuringValidationCannotComplete(t *testing.T) {
	f := newFixture(t)
	f.options.Profiles = map[string]config.ValidationProfile{"blocking": {Type: "unit", Paths: []string{"a.txt"}, Commands: [][]string{{"sh", "-c", "touch .mindrail/validation-started; while [ ! -e .mindrail/validation-release ]; do sleep 0.01; done"}}}}
	s, err := workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	f.service = s
	r := start(t, f, "stale", "a.txt")
	type result struct {
		out workflow.Finalization
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := s.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
		done <- result{out, err}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(f.root, ".mindrail", "validation-started")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("validation did not start")
		}
		time.Sleep(time.Millisecond)
	}
	if _, _, err := f.options.Coordination.TransitionExpecting(t.Context(), r.TaskID, coordination.NamedSession(r.SessionID), coordination.StateBlocked, "concurrent intervention", r.Revision); err != nil {
		t.Fatal(err)
	}
	write(t, f.root, ".mindrail/validation-release", "")
	got := <-done
	if got.err == nil || got.out.Completed {
		t.Fatalf("stale revision completed: %+v %v", got.out, got.err)
	}
	payload, ok := app.PayloadOf(got.err)
	if !ok || payload.Code != app.CodeStateRevisionConflict {
		t.Fatalf("expected stale CAS: %v", got.err)
	}
	task, err := f.options.Coordination.FindTask(t.Context(), r.TaskID)
	if err != nil || task.State != coordination.StateBlocked {
		t.Fatalf("concurrent state overwritten: %+v %v", task, err)
	}
}

func TestFinalizeRetryRepairsPostCommitLeaseReleaseFailure(t *testing.T) {
	f := newFixture(t)
	r := start(t, f, "release-failure", "a.txt")
	if _, err := f.options.DB.ExecContext(t.Context(), `CREATE TRIGGER reject_file_release BEFORE UPDATE OF released_at ON leases WHEN OLD.target_kind='file' BEGIN SELECT RAISE(ABORT,'test release failure'); END`); err != nil {
		t.Fatal(err)
	}
	out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err == nil || out.Completed || out.Run.State != coordination.StateCompleted {
		t.Fatalf("partial completion not reported: %+v %v", out, err)
	}
	if _, err := f.options.DB.ExecContext(t.Context(), `DROP TRIGGER reject_file_release`); err != nil {
		t.Fatal(err)
	}
	restarted, err := workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	again, err := restarted.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || !again.Completed || again.Run.Revision != out.Run.Revision {
		t.Fatalf("recovery: %+v %v", again, err)
	}
	leases, err := f.options.Coordination.ListLeases(t.Context(), f.options.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range leases {
		if l.Holder == r.SessionID {
			t.Fatalf("leaked lease: %+v", l)
		}
	}
}

func TestReconcileUsesDurableSnapshotPhases(t *testing.T) {
	f := newFixture(t)
	r := start(t, f, "reconcile-phases", "a.txt")
	write(t, f.root, "a.txt", "first edit\n")
	first, err := f.service.Reconcile(t.Context(), r.RunKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Reconcile(t.Context(), r.RunKey); err != nil {
		t.Fatal(err)
	}
	restarted, err := workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := restarted.Reconcile(t.Context(), r.RunKey)
	if err != nil || replay.Change.ID != first.Change.ID {
		t.Fatalf("restart replay: %+v %v", replay, err)
	}
	count := func() int {
		t.Helper()
		var n int
		if err := f.options.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM operations WHERE command='workflow reconcile'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(); n != 1 {
		t.Fatalf("identical replay recorded %d phases", n)
	}
	write(t, f.root, "a.txt", "second edit without task revision\n")
	next, err := restarted.Reconcile(t.Context(), r.RunKey)
	if err != nil || next.Change.ID != first.Change.ID {
		t.Fatalf("next edit: %+v %v", next, err)
	}
	if n := count(); n != 2 {
		t.Fatalf("new bytes did not create a new phase: %d", n)
	}
	out, err := restarted.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || !out.Completed {
		t.Fatalf("finish: %+v %v", out, err)
	}
	if n := count(); n != 2 {
		t.Fatalf("finalize repeated same discovery phase: %d", n)
	}
}

func TestReconcileReappliesEarlierBytesAfterInterveningSnapshot(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprint(restart), func(t *testing.T) {
			f := newFixture(t)
			write(t, f.root, "pyproject.toml", "[project]\nname=\"replay\"\n")
			write(t, f.root, "a.py", "def f():\n    return 0\n")
			if _, err := f.options.Indexes.UpsertUnit(t.Context(), f.root, index.UnitPython); err != nil {
				t.Fatal(err)
			}
			r := start(t, f, "snapshot-return", "a.py")
			write(t, f.root, "a.py", "def f():\n    return 1\n")
			first, err := f.service.Reconcile(t.Context(), r.RunKey)
			if err != nil {
				t.Fatal(err)
			}
			readHashes := func() (string, string) {
				t.Helper()
				var file, symbol string
				if err := f.options.DB.QueryRowContext(t.Context(), `SELECT content_hash FROM change_files WHERE change_id=? AND path=?`, first.Change.ID, filepath.Join(f.root, "a.py")).Scan(&file); err != nil {
					t.Fatal(err)
				}
				if err := f.options.DB.QueryRowContext(t.Context(), `SELECT body_hash FROM symbols WHERE path=? AND name='f'`, filepath.Join(f.root, "a.py")).Scan(&symbol); err != nil {
					t.Fatal(err)
				}
				return file, symbol
			}
			aFile, aSymbol := readHashes()
			if _, err := f.service.Reconcile(t.Context(), r.RunKey); err != nil {
				t.Fatal(err)
			}
			write(t, f.root, "a.py", "def f():\n    return 2\n")
			if _, err := f.service.Reconcile(t.Context(), r.RunKey); err != nil {
				t.Fatal(err)
			}
			bFile, bSymbol := readHashes()
			if aFile == bFile || aSymbol == bSymbol {
				t.Fatal("test did not create distinct snapshots")
			}
			if restart {
				s, err := workflow.New(f.options)
				if err != nil {
					t.Fatal(err)
				}
				f.service = s
			}
			write(t, f.root, "a.py", "def f():\n    return 1\n")
			if _, err := f.service.Reconcile(t.Context(), r.RunKey); err != nil {
				t.Fatal(err)
			}
			lastFile, lastSymbol := readHashes()
			if lastFile != aFile || lastSymbol != aSymbol {
				t.Fatalf("historical replay left B applied: files %s vs %s; symbols %s vs %s", lastFile, aFile, lastSymbol, aSymbol)
			}
		})
	}
}
