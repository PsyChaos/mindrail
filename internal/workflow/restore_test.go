package workflow_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/parser"
	"github.com/PsyChaos/mindrail/internal/index/snapshot"
	"github.com/PsyChaos/mindrail/internal/workflow"
)

func commitRestoreFixture(t *testing.T, f fixture) {
	t.Helper()
	for _, args := range [][]string{{"add", "a.py", "pyproject.toml"}, {"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-qm", "python baseline"}} {
		if out, err := exec.Command("git", append([]string{"-C", f.root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, out)
		}
	}
}

func restoreFixture(t *testing.T, body string) (fixture, index.ProjectUnit, []index.Symbol) {
	t.Helper()
	f := newFixture(t)
	write(t, f.root, "pyproject.toml", "[project]\nname=\"restore\"\n")
	write(t, f.root, "a.py", body)
	commitRestoreFixture(t, f)
	unit, err := f.options.Indexes.UpsertUnit(t.Context(), f.root, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := parser.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reg.Close)
	idx := index.NewIndexer(f.options.Indexes, reg, snapshot.New(filesystem.RuntimePaths{CacheDir: t.TempDir()}))
	if _, err := idx.IndexFile(t.Context(), f.options.ProjectID, unit, filepath.Join(f.root, "a.py")); err != nil {
		t.Fatal(err)
	}
	symbols, err := f.options.Indexes.ListSymbolsInFile(t.Context(), unit.ID, filepath.Join(f.root, "a.py"))
	if err != nil {
		t.Fatal(err)
	}
	return f, unit, symbols
}

func TestReconcileRestoresExactBaseline(t *testing.T) {
	for _, restart := range []bool{false, true} {
		for _, kind := range []string{"modified", "deleted", "renamed", "added"} {
			t.Run(fmt.Sprintf("%s/restart=%v", kind, restart), func(t *testing.T) {
				testReconcileRestoresExactBaseline(t, kind, restart)
			})
		}
	}
}

func testReconcileRestoresExactBaseline(t *testing.T, kind string, restart bool) {
	const original = "def f():\n    return 0\n"
	f, unit, symbols := restoreFixture(t, original)
	r := start(t, f, "restore", "a.py", "new.py")
	switch kind {
	case "modified":
		write(t, f.root, "a.py", "def renamed():\n    return 1\n")
	case "deleted":
		if err := os.Remove(filepath.Join(f.root, "a.py")); err != nil {
			t.Fatal(err)
		}
	case "renamed":
		if err := os.Rename(filepath.Join(f.root, "a.py"), filepath.Join(f.root, "new.py")); err != nil {
			t.Fatal(err)
		}
	case "added":
		write(t, f.root, "new.py", "def added():\n    return 2\n")
	}
	if _, err := f.service.Reconcile(t.Context(), r.RunKey); err != nil {
		t.Fatal(err)
	}
	if restart {
		service, err := workflow.New(f.options)
		if err != nil {
			t.Fatal(err)
		}
		f.service = service
	}
	write(t, f.root, "a.py", original)
	if kind == "renamed" || kind == "added" {
		if err := os.Remove(filepath.Join(f.root, "new.py")); err != nil {
			t.Fatal(err)
		}
	}
	result, err := f.service.Reconcile(t.Context(), r.RunKey)
	if err != nil {
		t.Fatal(err)
	}
	state, err := f.options.Indexes.ReadFileState(t.Context(), filepath.Join(f.root, "a.py"))
	if err != nil || state.State.ContentHash != fmt.Sprintf("%x", sha256.Sum256([]byte(original))) {
		t.Fatalf("restored bytes not indexed: %+v %v", state, err)
	}
	got, err := f.options.Indexes.ListSymbolsInFile(t.Context(), unit.ID, filepath.Join(f.root, "a.py"))
	for i := range got {
		got[i].ID = 0
	}
	for i := range symbols {
		symbols[i].ID = 0
	}
	if err != nil || !reflect.DeepEqual(got, symbols) {
		t.Fatalf("baseline symbols not restored: %+v want %+v: %v", got, symbols, err)
	}
	files, err := f.options.Changes.Store().ReadChangeFiles(t.Context(), result.Change.ID)
	if err != nil || len(files) != 0 {
		t.Fatalf("restored file still attributed: %+v %v", files, err)
	}
	changed, err := f.options.Changes.Store().ReadChangeSymbols(t.Context(), result.Change.ID)
	if err != nil || len(changed) != 0 {
		t.Fatalf("restored symbols still attributed: %+v %v", changed, err)
	}
	attribution, err := f.options.Changes.AttributeTask(t.Context(), r.TaskID)
	if err != nil || len(attribution.Drift) != 0 || len(attribution.Symbols) != 0 {
		t.Fatalf("restored task retains current attribution: %+v %v", attribution, err)
	}
	missing, err := f.options.Indexes.ReadFileState(t.Context(), filepath.Join(f.root, "new.py"))
	if err != nil || missing.Exists {
		t.Fatalf("removed addition still indexed: %+v %v", missing, err)
	}
	ghosts, err := f.options.Indexes.ListSymbolsInFile(t.Context(), unit.ID, filepath.Join(f.root, "new.py"))
	if err != nil || len(ghosts) != 0 {
		t.Fatalf("removed addition retains symbols: %+v %v", ghosts, err)
	}
	out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || !out.Completed {
		t.Fatalf("restored task cannot finish: %+v %v", out, err)
	}
}

func TestRestoredScopeDriftCanFinalizeWithoutAbsorbingInitialDirt(t *testing.T) {
	for _, restart := range []bool{false, true} {
		for _, inherited := range []bool{false, true} {
			t.Run(fmt.Sprintf("restart=%v/inherited=%v", restart, inherited), func(t *testing.T) {
				f := newFixture(t)
				original := "first\n"
				if inherited {
					original = "inherited dirty bytes\n"
					write(t, f.root, "b.txt", original)
				}
				r := start(t, f, "restore-drift", "a.txt")
				write(t, f.root, "b.txt", "accidental out-of-scope change\n")
				out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
				if err != nil || out.Completed || out.Decision.Allow {
					t.Fatalf("expected scope denial: %+v %v", out, err)
				}
				if restart {
					f.service, err = workflow.New(f.options)
					if err != nil {
						t.Fatal(err)
					}
				}
				write(t, f.root, "b.txt", original)
				out, err = f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
				if err != nil || !out.Completed {
					t.Fatalf("restored scope drift blocks finalization: %+v %v", out, err)
				}
				attribution, err := f.options.Changes.AttributeTask(t.Context(), r.TaskID)
				if err != nil || len(attribution.Drift) != 0 {
					t.Fatalf("obsolete drift retained: %+v %v", attribution, err)
				}
				got, err := os.ReadFile(filepath.Join(f.root, "b.txt"))
				if err != nil || string(got) != original {
					t.Fatalf("initial dirt changed: %q %v", got, err)
				}
			})
		}
	}
}

func TestRestorationPreservesOtherCurrentAndHistoricalChanges(t *testing.T) {
	f, _, _ := restoreFixture(t, "def f():\n    return 0\n")
	historical := start(t, f, "historical", "a.py")
	const inherited = "def f():\n    return 1\n"
	write(t, f.root, "a.py", inherited)
	previous, err := f.service.Reconcile(t.Context(), historical.RunKey)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: historical.RunKey})
	if err != nil || !out.Completed {
		t.Fatalf("historical finish: %+v %v", out, err)
	}
	historyFiles, err := f.options.Changes.Store().ReadChangeFiles(t.Context(), previous.Change.ID)
	if err != nil {
		t.Fatal(err)
	}
	historySymbols, err := f.options.Changes.Store().ReadChangeSymbols(t.Context(), previous.Change.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := start(t, f, "restore-with-active", "a.py", "new.py")
	write(t, f.root, "a.py", "def temporary():\n    return 2\n")
	write(t, f.root, "new.py", "def retained():\n    return 3\n")
	changed, err := f.service.Reconcile(t.Context(), r.RunKey)
	if err != nil {
		t.Fatal(err)
	}
	write(t, f.root, "a.py", inherited)
	if _, err := f.service.Reconcile(t.Context(), r.RunKey); err != nil {
		t.Fatal(err)
	}
	files, err := f.options.Changes.Store().ReadChangeFiles(t.Context(), changed.Change.ID)
	if err != nil || len(files) != 1 || files[0].Path != filepath.Join(f.root, "new.py") {
		t.Fatalf("wrong current file ownership: %+v %v", files, err)
	}
	symbols, err := f.options.Changes.Store().ReadChangeSymbols(t.Context(), changed.Change.ID)
	if err != nil || len(symbols) != 1 {
		t.Fatalf("wrong current symbol ownership: %+v %v", symbols, err)
	}
	attribution, err := f.options.Changes.AttributeTask(t.Context(), r.TaskID)
	if err != nil || len(attribution.Symbols) != 1 || attribution.Symbols[0].File != filepath.Join(f.root, "new.py") {
		t.Fatalf("wrong attribution: %+v %v", attribution, err)
	}
	gotFiles, err := f.options.Changes.Store().ReadChangeFiles(t.Context(), previous.Change.ID)
	if err != nil || !reflect.DeepEqual(gotFiles, historyFiles) {
		t.Fatalf("history files changed: %+v %v", gotFiles, err)
	}
	gotSymbols, err := f.options.Changes.Store().ReadChangeSymbols(t.Context(), previous.Change.ID)
	if err != nil || !reflect.DeepEqual(gotSymbols, historySymbols) {
		t.Fatalf("history symbols changed: %+v %v", gotSymbols, err)
	}
	out, err = f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || !out.Completed {
		t.Fatalf("current finish: %+v %v", out, err)
	}
}

func TestRestorationRetryAfterOwnershipWriteFailure(t *testing.T) {
	const original = "def f():\n    return 0\n"
	f, _, _ := restoreFixture(t, original)
	r := start(t, f, "restore-partial", "a.py")
	write(t, f.root, "a.py", "def temporary():\n    return 1\n")
	changed, err := f.service.Reconcile(t.Context(), r.RunKey)
	if err != nil {
		t.Fatal(err)
	}
	write(t, f.root, "a.py", original)
	if _, err := f.options.DB.ExecContext(t.Context(), `CREATE TRIGGER reject_restore BEFORE DELETE ON change_files BEGIN SELECT RAISE(ABORT,'injected restoration failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Reconcile(t.Context(), r.RunKey); err == nil {
		t.Fatal("injected cleanup failure was swallowed")
	}
	files, err := f.options.Changes.Store().ReadChangeFiles(t.Context(), changed.Change.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("partial refresh lost retry path: %+v %v", files, err)
	}
	symbols, err := f.options.Changes.Store().ReadChangeSymbols(t.Context(), changed.Change.ID)
	if err != nil || len(symbols) == 0 {
		t.Fatalf("partial cleanup was not atomic: %+v %v", symbols, err)
	}
	if _, err := f.options.DB.ExecContext(t.Context(), `DROP TRIGGER reject_restore`); err != nil {
		t.Fatal(err)
	}
	f.service, err = workflow.New(f.options)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || !out.Completed {
		t.Fatalf("restart retry failed: %+v %v", out, err)
	}
	files, err = f.options.Changes.Store().ReadChangeFiles(t.Context(), changed.Change.ID)
	if err != nil || len(files) != 0 {
		t.Fatalf("retry failed to retire restored file: %+v %v", files, err)
	}
}

func TestRestorationRetiresCanceledIndexRegistration(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprintf("restart=%v", restart), func(t *testing.T) {
			testRestorationRetiresCanceledIndexRegistration(t, restart)
		})
	}
}

func testRestorationRetiresCanceledIndexRegistration(t *testing.T, restart bool) {
	f, unit, _ := restoreFixture(t, "def f():\n    return 0\n")
	r := start(t, f, "restore-pending", "new.py")
	write(t, f.root, "new.py", "def added():\n    return 1\n")
	if _, err := f.service.Reconcile(t.Context(), r.RunKey); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.root, "new.py")
	observed, err := f.options.Indexes.ReadFileState(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	// A canceled indexing attempt has registered its next bytes, but has
	// not replaced the retained facts from the successful earlier pass.
	target := fmt.Sprintf("%x", sha256.Sum256([]byte("next incomplete attempt")))
	if _, applied, err := f.options.Indexes.RegisterFileCAS(t.Context(), observed, unit.ID, path, "python", target); err != nil || !applied {
		t.Fatalf("register pending: %v %v", applied, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if restart {
		f.service, err = workflow.New(f.options)
		if err != nil {
			t.Fatal(err)
		}
	}
	out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: r.RunKey})
	if err != nil || !out.Completed {
		t.Fatalf("absent baseline retains canceled index generation: %+v %v", out, err)
	}
	state, err := f.options.Indexes.ReadFileState(t.Context(), path)
	if err != nil || state.Exists {
		t.Fatalf("pending state retained for absent file: %+v %v", state, err)
	}
}
