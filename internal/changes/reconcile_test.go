package changes_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/index"
)

func reconcileRunner(t *testing.T, stdout string) *git.FakeRunner {
	t.Helper()
	return &git.FakeRunner{
		Responses: map[string]git.FakeResponse{
			"status --porcelain=v1 -z --untracked-files=normal -- .": {Stdout: stdout},
			"diff --no-color --name-status -z -M HEAD -- .":          {Stdout: ""},
		},
	}
}

// TestReconcileConvergesWithBaselinePath is AC-05.1: one edit, two paths,
// same discovery content. The flows run on identical twin fixtures because
// discovery consumes: whoever indexes first leaves nothing for the second
// to find in a shared database. Each flow must independently produce the
// full rows; provenance may differ, content must not (uids compared by
// presence — separate databases mint separate lineages by design).
func TestReconcileConvergesWithBaselinePath(t *testing.T) {
	v1 := "def f():\n    return 1\n"
	v2 := "def f():\n    return 2\n"
	build := func(t *testing.T) serviceFixture {
		t.Helper()
		f := newServiceFixture(t)
		path := filepath.Join(f.root, "py", "a.py")
		if err := os.WriteFile(path, []byte(v1), 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}
	// Flow A: declare, edit, after_change.
	fa := build(t)
	seedTasks(t, fa.db, "TSK-1")
	pa := filepath.Join(fa.root, "py", "a.py")
	if _, err := fa.store.CaptureBaseline(t.Context(), "TSK-1", []string{pa}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pa, []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}
	changeA, err := fa.service.AfterChange(t.Context(), changeProject, fa.root, "TSK-1", "")
	if err != nil {
		t.Fatal(err)
	}
	// Flow B: no baseline, reconcile-only over the same edit.
	fb := build(t)
	seedTasks(t, fb.db, "TSK-1")
	pb := filepath.Join(fb.root, "py", "a.py")
	if err := os.WriteFile(pb, []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}
	resultB, err := fb.service.Reconcile(t.Context(), changeProject, fb.root, "TSK-1", "", reconcileRunner(t, " M py/a.py\x00"))
	if err != nil {
		t.Fatal(err)
	}
	aFiles, err := fa.store.ReadChangeFiles(t.Context(), changeA.ID)
	if err != nil {
		t.Fatal(err)
	}
	bFiles, err := fb.store.ReadChangeFiles(t.Context(), resultB.Change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aFiles) != 1 || len(bFiles) != 1 {
		t.Fatalf("files %+v vs %+v", aFiles, bFiles)
	}
	for _, field := range [][2]string{{aFiles[0].Kind, bFiles[0].Kind}, {aFiles[0].Hash, bFiles[0].Hash}} {
		if field[0] != field[1] {
			t.Fatalf("file rows differ: %+v vs %+v", aFiles[0], bFiles[0])
		}
	}
	if aFiles[0].Via == bFiles[0].Via {
		t.Fatal("provenance must differ across paths")
	}
	aSymbols, err := fa.store.ReadChangeSymbols(t.Context(), changeA.ID)
	if err != nil {
		t.Fatal(err)
	}
	bSymbols, err := fb.store.ReadChangeSymbols(t.Context(), resultB.Change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aSymbols) != 1 || len(bSymbols) != 1 {
		t.Fatalf("symbols %+v vs %+v", aSymbols, bSymbols)
	}
	aRow, bRow := aSymbols[0], bSymbols[0]
	if aRow.Kind != bRow.Kind || aRow.Body != bRow.Body || aRow.Signature != bRow.Signature || aRow.Structure != bRow.Structure ||
		aRow.SigHash != bRow.SigHash || aRow.BodyHash != bRow.BodyHash || aRow.StructHash != bRow.StructHash {
		t.Fatalf("symbol rows differ: %+v vs %+v", aRow, bRow)
	}
	if aRow.UID == "" || bRow.UID == "" {
		t.Fatalf("symbol rows lack uids: %+v vs %+v", aRow, bRow)
	}
	if aRow.Key != bRow.Key {
		t.Fatalf("symbol keys differ: %q vs %q", aRow.Key, bRow.Key)
	}
}

// TestReconcileDivergenceListsOutOfScopeEdits is AC-05.2: an edit outside
// the baseline scope appears with path, kind and reason — reported, never
// absorbed, never blocking this milestone.
func TestReconcileDivergenceListsOutOfScopeEdits(t *testing.T) {
	f := newServiceFixture(t)
	a := filepath.Join(f.root, "py", "a.py")
	b := filepath.Join(f.root, "py", "b.py")
	if err := os.WriteFile(a, []byte("a = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedTasks(t, f.db, "TSK-1")
	if _, err := f.store.CaptureBaseline(t.Context(), "TSK-1", []string{a}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a, []byte("a = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := f.service.Reconcile(t.Context(), changeProject, f.root, "TSK-1", "", reconcileRunner(t, " M py/a.py\x00 M py/b.py\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Divergence) != 1 {
		t.Fatalf("divergence = %+v, want exactly the out-of-scope file", result.Divergence)
	}
	entry := result.Divergence[0]
	if entry.Path != b || entry.Kind != changes.FileModified || entry.Reason == "" {
		t.Fatalf("entry = %+v, want path+kind+reason", entry)
	}
	// Divergence does not block: both files land in the change.
	files, err := f.store.ReadChangeFiles(t.Context(), result.Change.ID)
	if err != nil || len(files) != 2 {
		t.Fatalf("files = %+v, %v", files, err)
	}
}

// TestReconcileWithoutTaskCreatesRetroactiveChange is AC-05.3: untasked work
// lands in a NULL-task Change instead of vanishing.
func TestReconcileWithoutTaskCreatesRetroactiveChange(t *testing.T) {
	f := newServiceFixture(t)
	a := filepath.Join(f.root, "py", "a.py")
	if err := os.WriteFile(a, []byte("def f():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := f.service.Reconcile(t.Context(), changeProject, f.root, "", "", reconcileRunner(t, " M py/a.py\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.TaskID != "" {
		t.Fatalf("change = %+v, want NULL-task retroactive", result.Change)
	}
	files, err := f.store.ReadChangeFiles(t.Context(), result.Change.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("files = %+v, %v", files, err)
	}
	symbols, err := f.store.ReadChangeSymbols(t.Context(), result.Change.ID)
	if err != nil || len(symbols) != 1 || symbols[0].Kind != changes.SymbolAdded || symbols[0].UID == "" {
		t.Fatalf("symbols = %+v, %v", symbols, err)
	}
	if len(result.Divergence) != 0 {
		t.Fatalf("divergence = %+v, want none without a baseline", result.Divergence)
	}
}

// realGitRepo builds a committed repository for worktree fixtures. It
// assumes the git binary the repository itself requires.
func realGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	for rel, content := range files {
		path := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "-A")
	run("commit", "-qm", "base")
	return repo
}

func gitWorktree(t *testing.T, repo, name string) string {
	t.Helper()
	linked := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("git", "-C", repo, "worktree", "add", "--quiet", linked)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v\n%s", err, out)
	}
	return linked
}

// TestReconcileDiscoversStagedAndWorktreeChanges pins the Git fixture half:
// staged edits, unstaged edits and a separate linked worktree all surface
// through the real binary.
func TestReconcileDiscoversStagedAndWorktreeChanges(t *testing.T) {
	f := newServiceFixture(t)
	repo := realGitRepo(t, map[string]string{
		"pkg/pyproject.toml": "[project]\nname = 'pkg'\n",
		"pkg/a.py":           "def f():\n    return 1\n",
	})
	linked := gitWorktree(t, repo, "linked")
	unit, err := f.indexes.UpsertUnit(t.Context(), filepath.Join(linked, "pkg"), index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	_ = unit
	seedTasks(t, f.db, "TSK-W")
	// Staged edit in the main checkout, unstaged edit in the linked one.
	if err := os.WriteFile(filepath.Join(repo, "pkg", "a.py"), []byte("def f():\n    return 10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	staged := exec.Command("git", "-C", repo, "add", "-A")
	if out, err := staged.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	linkedFile := filepath.Join(linked, "pkg", "a.py")
	if err := os.WriteFile(linkedFile, []byte("def f():\n    return 20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main, err := f.service.Reconcile(t.Context(), changeProject, repo, "TSK-W", "", &git.ExecRunner{})
	if err != nil {
		t.Fatal(err)
	}
	mainFiles, err := f.store.ReadChangeFiles(t.Context(), main.Change.ID)
	if err != nil || len(mainFiles) != 1 || mainFiles[0].Path != filepath.Join(repo, "pkg", "a.py") {
		t.Fatalf("main files = %+v, %v", mainFiles, err)
	}
	mainHash := mainFiles[0].Hash
	other, err := f.service.Reconcile(t.Context(), changeProject, linked, "TSK-W", "", &git.ExecRunner{})
	if err != nil {
		t.Fatal(err)
	}
	otherFiles, err := f.store.ReadChangeFiles(t.Context(), other.Change.ID)
	if err != nil || len(otherFiles) != 2 {
		t.Fatalf("linked files = %+v, %v", otherFiles, err)
	}
	found := false
	for _, file := range otherFiles {
		if file.Path == linkedFile {
			found = true
		}
	}
	if !found {
		t.Fatalf("shared change lacks the linked file: %+v", otherFiles)
	}
	var linkedHash string
	for _, file := range otherFiles {
		if file.Path == linkedFile {
			linkedHash = file.Hash
		}
	}
	if linkedHash == "" || linkedHash == mainHash {
		t.Fatalf("linked hash = %q, main hash = %q: staged and worktree edits must differ", linkedHash, mainHash)
	}
}

// TestWarmPathBudgetsReading times the §5 STRUCTURAL budgets on a ten-file
// fixture: after_change under 1.5 s, reconcile under 2 s. Bounds are the
// requirement itself; the suite passing anywhere slower is the signal.
func TestWarmPathBudgetsReading(t *testing.T) {
	f := newServiceFixture(t)
	seedTasks(t, f.db, "TSK-SLO")
	var scope []string
	for i := 0; i < 10; i++ {
		name := "f" + itoa(i) + ".py"
		path := filepath.Join(f.root, "py", name)
		if err := os.WriteFile(path, []byte("def f"+itoa(i)+"():\n    return "+itoa(i)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		scope = append(scope, path)
	}
	if _, err := f.store.CaptureBaseline(t.Context(), "TSK-SLO", scope, ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		path := filepath.Join(f.root, "py", "f"+itoa(i)+".py")
		if err := os.WriteFile(path, []byte("def f"+itoa(i)+"():\n    return "+itoa(i*2)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now()
	if _, err := f.service.AfterChange(t.Context(), changeProject, f.root, "TSK-SLO", ""); err != nil {
		t.Fatal(err)
	}
	afterElapsed := time.Since(started)
	porcelain := ""
	for i := 0; i < 10; i++ {
		porcelain += " M py/f" + itoa(i) + ".py\x00"
	}
	started = time.Now()
	if _, err := f.service.Reconcile(t.Context(), changeProject, f.root, "TSK-SLO", "", reconcileRunner(t, porcelain)); err != nil {
		t.Fatal(err)
	}
	reconcileElapsed := time.Since(started)
	t.Logf("after_change 10 files: %v (budget 1.5s); reconcile 10 files: %v (budget 2s)", afterElapsed, reconcileElapsed)
	if afterElapsed > 1500*time.Millisecond {
		t.Fatalf("after_change %v exceeds the 1.5s STRUCTURAL budget", afterElapsed)
	}
	if reconcileElapsed > 2*time.Second {
		t.Fatalf("reconcile %v exceeds the 2s STRUCTURAL budget", reconcileElapsed)
	}
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

// TestReconcileHintsPartitionByTarget pins fileHintsFor: two simultaneous
// staged renames corroborate only their own heir — a leaked hint would
// migrate the wrong lineage silently.
func TestReconcileHintsPartitionByTarget(t *testing.T) {
	f := newServiceFixture(t)
	repo := realGitRepo(t, map[string]string{
		"pkg/pyproject.toml": "[project]\nname = 'pkg'\n",
		"pkg/a.py":           "def f():\n    return 1\n",
		"pkg/c.py":           "def g():\n    return 2\n",
	})
	pkg := filepath.Join(repo, "pkg")
	unit, err := f.indexes.UpsertUnit(t.Context(), pkg, index.UnitPython)
	if err != nil {
		t.Fatal(err)
	}
	seedTasks(t, f.db, "TSK-P")
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	uidNamed := func(path, name string) string {
		t.Helper()
		symbols, err := f.indexes.ListSymbolsInFile(t.Context(), unit.ID, path)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range symbols {
			if row.Name == name {
				return row.UID
			}
		}
		t.Fatalf("no %s in %s", name, path)
		return ""
	}
	// Establish both old uids through content edits discovered first.
	if err := os.WriteFile(filepath.Join(pkg, "a.py"), []byte("def f():\n    return 10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "c.py"), []byte("def g():\n    return 20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Reconcile(t.Context(), changeProject, repo, "TSK-P", "", &git.ExecRunner{}); err != nil {
		t.Fatal(err)
	}
	oldF, oldG := uidNamed(filepath.Join(pkg, "a.py"), "f"), uidNamed(filepath.Join(pkg, "c.py"), "g")
	if oldF == "" || oldG == "" || oldF == oldG {
		t.Fatalf("old uids = %q %q", oldF, oldG)
	}
	gitRun("add", "-A")
	gitRun("commit", "-qm", "edit")
	gitRun("mv", "pkg/a.py", "pkg/b.py")
	gitRun("mv", "pkg/c.py", "pkg/d.py")
	if _, err := f.service.Reconcile(t.Context(), changeProject, repo, "TSK-P", "", &git.ExecRunner{}); err != nil {
		t.Fatal(err)
	}
	// The change accumulates across runs by design; assert per moved file
	// against the index store: each heir file carries exactly its own old
	// uid, proving hints did not leak across targets.
	for path, want := range map[string]string{filepath.Join(pkg, "b.py"): oldF, filepath.Join(pkg, "d.py"): oldG} {
		rows, err := f.indexes.ListSymbolsInFile(t.Context(), unit.ID, path)
		if err != nil || len(rows) != 1 || rows[0].UID != want {
			t.Fatalf("%s rows = %+v, %v: want the single carried %s", path, rows, err, want)
		}
	}
	if got := uidNamed(filepath.Join(pkg, "b.py"), "f"); got != oldF {
		t.Fatalf("b.py heir = %q, want carried %q", got, oldF)
	}
	if got := uidNamed(filepath.Join(pkg, "d.py"), "g"); got != oldG {
		t.Fatalf("d.py heir = %q, want carried %q", got, oldG)
	}
}

// TestReconcileStagedMoveMigrates proves the hint path end to end: a staged
// Git rename corroborates the move, so the heir rows carry the indexed
// file's uid instead of minting. The file is indexed first so the old uid
// exists to carry.
func TestReconcileStagedMoveMigrates(t *testing.T) {
	f := newServiceFixture(t)
	repo := realGitRepo(t, map[string]string{
		"pkg/pyproject.toml": "[project]\nname = 'pkg'\n",
		"pkg/a.py":           "def f():\n    return 1\n",
	})
	if _, err := f.indexes.UpsertUnit(t.Context(), filepath.Join(repo, "pkg"), index.UnitPython); err != nil {
		t.Fatal(err)
	}
	seedTasks(t, f.db, "TSK-M")
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// Establish the old uid through a content edit discovered by reconcile.
	if err := os.WriteFile(filepath.Join(repo, "pkg", "a.py"), []byte("def f():\n    return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := f.service.Reconcile(t.Context(), changeProject, repo, "TSK-M", "", &git.ExecRunner{})
	if err != nil {
		t.Fatal(err)
	}
	firstSymbols, err := f.store.ReadChangeSymbols(t.Context(), first.Change.ID)
	if err != nil || len(firstSymbols) != 1 || firstSymbols[0].UID == "" {
		t.Fatalf("first symbols = %+v, %v", firstSymbols, err)
	}
	oldUID := firstSymbols[0].UID
	gitRun("add", "-A")
	gitRun("commit", "-qm", "edit")
	// Staged move: the heir must carry, not mint.
	gitRun("mv", "pkg/a.py", "pkg/b.py")
	second, err := f.service.Reconcile(t.Context(), changeProject, repo, "TSK-M", "", &git.ExecRunner{})
	if err != nil {
		t.Fatal(err)
	}
	files, err := f.store.ReadChangeFiles(t.Context(), second.Change.ID)
	if err != nil {
		t.Fatal(err)
	}
	var moved *changes.FileChange
	for i := range files {
		if files[i].Kind == changes.FileRenamed {
			moved = &files[i]
		}
	}
	if moved == nil {
		t.Fatalf("no renamed file row in %+v", files)
	}
	symbols, err := f.store.ReadChangeSymbols(t.Context(), second.Change.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range symbols {
		if row.UID != oldUID {
			t.Fatalf("heir row = %+v, want carried %s", row, oldUID)
		}
	}
}
