package completion_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/bootstrap"
	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/completion"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/testguard"
	"github.com/PsyChaos/mindrail/internal/validation"
)

type fixture struct {
	root, project, task, session string
	service                      *completion.Service
	changes                      *changes.Service
	application                  *bootstrap.App
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	gitCommand(t, root, "init", "--quiet")
	application := bootstrap.New(bootstrap.Options{StartDir: root, Mode: bootstrap.ModeInit})
	if err := application.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := application.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	writeFile(t, filepath.Join(root, "source.txt"), "base\n")
	gitCommand(t, root, "add", ".")
	gitCommand(t, root, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "base")
	registry, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.Close)
	indexes := index.NewStore(application.DB(), app.SystemClock{})
	indexer := index.NewIndexer(indexes, registry, snapshot.New(filesystem.RuntimePaths{CacheDir: application.Paths().CacheDir}))
	store, err := changes.NewStore(application.DB(), app.SystemClock{})
	if err != nil {
		t.Fatal(err)
	}
	changeService, err := changes.New(store, indexes, indexer)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := validation.NewStore(application.DB(), app.SystemClock{})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := testguard.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(guard.Close)
	service, err := completion.New(root, changeService, indexes, evidence, guard)
	if err != nil {
		t.Fatal(err)
	}
	workspace := application.Subject().Workspace
	coord := application.Coordination()
	session, _, err := coord.OpenSession(t.Context(), workspace.ID, "completion-test")
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := coord.OpenTask(t.Context(), workspace.ProjectID, coordination.NamedSession(session.ID), "Evaluate only")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{root: root, project: workspace.ProjectID, task: task.ID, session: session.ID,
		service: service, changes: changeService, application: application}
}

func gitCommand(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateUsesCanonicalReconcileWithoutCompletingTask(t *testing.T) {
	f := newFixture(t)
	before, err := f.application.Coordination().FindTask(t.Context(), f.task)
	if err != nil {
		t.Fatal(err)
	}
	clean, err := f.service.Evaluate(t.Context(), f.project, f.task, nil)
	if err != nil || !clean.Allow {
		t.Fatalf("clean = %+v, %v", clean, err)
	}
	writeFile(t, filepath.Join(f.root, "source.txt"), "undeclared edit\n")
	decision, err := f.service.Evaluate(t.Context(), f.project, f.task, nil)
	if err != nil || decision.Allow || len(decision.Denials) != 1 || decision.Denials[0].Code != app.CodeScopeDrift {
		t.Fatalf("unregistered plain-file edit = %+v, %v", decision, err)
	}
	repeated, err := f.service.Evaluate(t.Context(), f.project, f.task, nil)
	if err != nil || !reflect.DeepEqual(decision, repeated) {
		t.Fatalf("unstable decisions: %+v %+v %v", decision, repeated, err)
	}
	after, err := f.application.Coordination().FindTask(t.Context(), f.task)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("evaluation changed coordination: before=%+v after=%+v err=%v", before, after, err)
	}
}

func TestEvaluatePreservesKnowledgeCodeAndPath(t *testing.T) {
	f := newFixture(t)
	rel := ".mindrail/knowledge/invariants/INV-0001.json"
	writeFile(t, filepath.Join(f.root, filepath.FromSlash(rel)), `{"schema_version":999,"id":"INV-0001"}`)
	for range 2 {
		decision, err := f.service.Evaluate(t.Context(), f.project, f.task, nil)
		payload, ok := app.PayloadOf(err)
		if decision.Allow || !ok || payload.Code != app.CodeKnowledgeSchemaUnsupported || payload.Metadata["path"] != rel || len(payload.NextAction) == 0 {
			t.Fatalf("unactionable refusal: decision=%+v payload=%+v err=%v", decision, payload, err)
		}
	}
	var rows int
	if err := f.application.DB().QueryRow(`SELECT count(*) FROM changes`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("invalid knowledge must stop before discovery writes: changes=%d err=%v", rows, err)
	}
}

func TestEvaluateKeepsOtherTaskChangesInAuthoritativeWorktree(t *testing.T) {
	f := newFixture(t)
	other, _, err := f.application.Coordination().OpenTask(t.Context(), f.project, coordination.NamedSession(f.session), "Concurrent work")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.root, "source.txt")
	if _, err := f.changes.Store().CaptureBaseline(t.Context(), other.ID, []string{path}, ""); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "another task's edit\n")
	if _, err := f.changes.AfterChange(t.Context(), f.project, f.root, other.ID, ""); err != nil {
		t.Fatal(err)
	}
	decision, err := f.service.Evaluate(t.Context(), f.project, f.task, nil)
	if err != nil || decision.Allow || len(decision.Denials) != 1 || decision.Denials[0].Code != app.CodeScopeDrift {
		t.Fatalf("whole-worktree scope must remain visible: %+v %v", decision, err)
	}
}

func TestEvaluateRejectsInvalidInputsAndCancellation(t *testing.T) {
	if _, err := completion.New("", nil, nil, nil, nil); err == nil {
		t.Fatal("accepted absent dependencies")
	}
	f := newFixture(t)
	for _, ids := range [][2]string{{"", f.task}, {f.project, ""}} {
		decision, err := f.service.Evaluate(t.Context(), ids[0], ids[1], nil)
		payload, ok := app.PayloadOf(err)
		if decision.Allow || !ok || payload.Code != app.CodeCommandLineInvalid {
			t.Fatalf("invalid ids = %+v, %v", decision, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if decision, err := f.service.Evaluate(ctx, f.project, f.task, nil); decision.Allow || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %+v %v", decision, err)
	}
}
