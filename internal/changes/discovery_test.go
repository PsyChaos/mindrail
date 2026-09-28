package changes_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"

	"github.com/PsyChaos/mindrail/internal/changes"
	"github.com/PsyChaos/mindrail/internal/git"
)

// makeFifo creates a named pipe, or errors where the platform has none;
// callers skip rather than fail there.
func makeFifo(path string) error {
	return syscall.Mkfifo(path, 0o600)
}

func porcelainRunner(t *testing.T, stdout string) *git.FakeRunner {
	t.Helper()
	return &git.FakeRunner{
		Responses: map[string]git.FakeResponse{
			"status --porcelain=v1 -z --untracked-files=all -- .": {Stdout: stdout},
		},
	}
}

func discoveryFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if content == "" {
		return path
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDiscoverFilesGitKinds is AC-03.1 at file level: every porcelain shape
// maps to the D-117 kind with the right paths and hashes.
func TestDiscoverFilesGitKinds(t *testing.T) {
	store := changesFixture(t)
	root := t.TempDir()
	a := discoveryFile(t, root, "a.py", "a = 2\n")
	staged := discoveryFile(t, root, "staged.py", "s = 1\n")
	untracked := discoveryFile(t, root, "new.py", "n = 1\n")
	moved := discoveryFile(t, root, "newname.py", "m = 1\n")
	copied := discoveryFile(t, root, "copy.py", "c = 1\n")
	typed := discoveryFile(t, root, "mode.py", "t = 1\n")
	conflicted := discoveryFile(t, root, "conflict.py", "u = 1\n")
	out := " M a.py\x00A  staged.py\x00?? new.py\x00R  newname.py\x00old.py\x00 D gone.py\x00" + "C  copy.py\x00src.py\x00T  mode.py\x00UU conflict.py\x00"
	files, err := store.DiscoverFilesGit(t.Context(), porcelainRunner(t, out), root)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]changes.FileChange{}
	for _, file := range files {
		byPath[file.Path] = file
	}
	cases := []struct {
		path, kind, old, hash string
	}{
		{a, changes.FileModified, "", shaOf("a = 2\n")},
		{staged, changes.FileAdded, "", shaOf("s = 1\n")},
		{untracked, changes.FileAdded, "", shaOf("n = 1\n")},
		{moved, changes.FileRenamed, filepath.Join(root, "old.py"), shaOf("m = 1\n")},
		{filepath.Join(root, "gone.py"), changes.FileDeleted, "", ""},
		{copied, changes.FileAdded, "", shaOf("c = 1\n")},
		{typed, changes.FileModified, "", shaOf("t = 1\n")},
		{conflicted, changes.FileModified, "", shaOf("u = 1\n")},
	}
	if len(files) != len(cases) {
		t.Fatalf("files = %+v, want %d", files, len(cases))
	}
	for _, tc := range cases {
		got, ok := byPath[tc.path]
		if !ok || got.Kind != tc.kind || got.OldPath != tc.old || got.Hash != tc.hash || got.Via != changes.ViaReconcile {
			t.Errorf("%s = %+v, want kind %s old %q hash %q", tc.path, got, tc.kind, tc.old, tc.hash)
		}
	}
}

// TestDiscoverFilesGitNonRegularRows pins AC-03.2's second half: paths git
// names that parse as nothing (a directory, a fifo) still become rows with
// empty hashes and no symbols — discovery stays complete where parsing
// cannot follow.
func TestDiscoverFilesGitNonRegularRows(t *testing.T) {
	store := changesFixture(t)
	root := t.TempDir()
	dir := discoveryFile(t, root, "pkg", "")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := "M  pkg\x00"
	fifo := filepath.Join(root, "pipe")
	if err := makeFifo(fifo); err != nil {
		t.Skipf("fifo unavailable: %v", err)
	}
	out += "M  pipe\x00"
	files, err := store.DiscoverFilesGit(t.Context(), porcelainRunner(t, out), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %+v, want both non-regular paths", files)
	}
	for _, file := range files {
		if file.Hash != "" || file.Kind != changes.FileModified {
			t.Errorf("file = %+v, want empty-hash modified row", file)
		}
	}
}

// TestDiscoverFilesGitExclusions is AC-03.2's first half: runtime and
// knowledge machinery never become rows, and nothing is statted for them.
func TestDiscoverFilesGitExclusions(t *testing.T) {
	store := changesFixture(t)
	root := t.TempDir()
	out := "M  .git/config\x00M  .mindrail/knowledge/decisions/dec-1.json\x00M  .gitignore\x00"
	discoveryFile(t, root, ".gitignore", "x\n")
	files, err := store.DiscoverFilesGit(t.Context(), porcelainRunner(t, out), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != filepath.Join(root, ".gitignore") {
		t.Fatalf("files = %+v, want only the source file", files)
	}
}

// TestDiscoverFilesGitEscapeRefusesRootBreakout pins fail-closed discovery:
// a path escaping the root fails the whole run instead of joining it
// half-read.
func TestDiscoverFilesGitEscapeRefusesRootBreakout(t *testing.T) {
	store := changesFixture(t)
	root := t.TempDir()
	for _, out := range []string{"M  ../evil.py\x00", "M  a/../../evil.py\x00"} {
		if _, err := store.DiscoverFilesGit(t.Context(), porcelainRunner(t, out), root); err == nil {
			t.Fatalf("escape %q accepted", out)
		}
	}
}

// TestBaselineFileDeltaConverges is AC-03.3: changed hashes and unrecorded
// scope files become rows; quiet scope files do not.
func TestBaselineFileDeltaConverges(t *testing.T) {
	store, db, _ := changesFixtureDB(t)
	seedTaskChain(t, db, "TSK-1")
	dir := t.TempDir()
	a := writeScopeFile(t, dir, "a.py", "a = 1\n")
	b := writeScopeFile(t, dir, "b.py", "b = 1\n")
	if _, err := store.CaptureBaseline(t.Context(), "TSK-1", []string{a, b}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := writeScopeFile(t, dir, "c.py", "c = 1\n")
	delta, err := store.BaselineFileDelta(t.Context(), "TSK-1", []string{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	if len(delta) != 2 {
		t.Fatalf("delta = %+v, want changed b plus unrecorded c", delta)
	}
	byPath := map[string]changes.FileChange{}
	for _, file := range delta {
		byPath[file.Path] = file
	}
	if byPath[b].Kind != changes.FileModified || byPath[b].Hash != shaOf("b = 2\n") || byPath[b].Via != changes.ViaBaseline {
		t.Errorf("b = %+v", byPath[b])
	}
	if byPath[c].Kind != changes.FileAdded {
		t.Errorf("c = %+v, want added", byPath[c])
	}
}

// TestBaselineFileDeltaReportsDeletion pins the deleted shape: a baselined
// file gone from disk becomes a deleted row with an empty hash.
func TestBaselineFileDeltaReportsDeletion(t *testing.T) {
	store, db, _ := changesFixtureDB(t)
	seedTaskChain(t, db, "TSK-1")
	dir := t.TempDir()
	a := writeScopeFile(t, dir, "a.py", "a = 1\n")
	if _, err := store.CaptureBaseline(t.Context(), "TSK-1", []string{a}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(a); err != nil {
		t.Fatal(err)
	}
	delta, err := store.BaselineFileDelta(t.Context(), "TSK-1", []string{a})
	if err != nil || len(delta) != 1 {
		t.Fatalf("delta = %+v, %v", delta, err)
	}
	if delta[0].Kind != changes.FileDeleted || delta[0].Hash != "" {
		t.Fatalf("row = %+v, want deleted with empty hash", delta[0])
	}
}

// TestUpsertFileRowsConverges pins the AC-15 row shape: redelivery updates
// in place, and malformed rows are refused before any write.
func TestUpsertFileRowsConverges(t *testing.T) {
	store := changesFixture(t)
	change, err := store.EnsureOpenChange(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	rows := []changes.FileChange{{Path: "/r/a.py", Kind: changes.FileModified, Hash: "h1", Via: changes.ViaBaseline}}
	if err := store.UpsertFileRows(t.Context(), change.ID, rows); err != nil {
		t.Fatal(err)
	}
	rows[0].Kind, rows[0].Hash, rows[0].Via = changes.FileModified, "h2", changes.ViaReconcile
	if err := store.UpsertFileRows(t.Context(), change.ID, rows); err != nil {
		t.Fatal(err)
	}
	stored, err := store.ReadChangeFiles(t.Context(), change.ID)
	if err != nil || len(stored) != 1 || stored[0].Hash != "h2" || stored[0].Via != changes.ViaReconcile {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	for _, bad := range [][]changes.FileChange{
		{{Path: "/r/a.py", Kind: "vaporized", Via: changes.ViaBaseline}},
		{{Path: "/r/a.py", Kind: changes.FileModified, Via: "telemetry"}},
		{{Kind: changes.FileModified, Via: changes.ViaBaseline}},
	} {
		err := store.UpsertFileRows(t.Context(), change.ID, bad)
		if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeCommandLineInvalid {
			t.Fatalf("malformed rows %+v gave %v, want usage refusal before SQL", bad, err)
		}
	}
	if err := store.UpsertFileRows(t.Context(), "", rows); err == nil {
		t.Fatal("rows accepted without a change")
	}
}

// TestDiscoverUpsertReadComposes pins the TASK-05 seam early: discovery
// output writes through upsert and reads back identical.
func TestDiscoverUpsertReadComposes(t *testing.T) {
	store := changesFixture(t)
	root := t.TempDir()
	a := discoveryFile(t, root, "a.py", "a = 2\n")
	files, err := store.DiscoverFilesGit(t.Context(), porcelainRunner(t, " M a.py\x00"), root)
	if err != nil || len(files) != 1 {
		t.Fatalf("discover = %+v, %v", files, err)
	}
	change, err := store.EnsureOpenChange(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertFileRows(t.Context(), change.ID, files); err != nil {
		t.Fatal(err)
	}
	stored, err := store.ReadChangeFiles(t.Context(), change.ID)
	if err != nil || len(stored) != 1 || stored[0] != files[0] {
		t.Fatalf("stored = %+v, want %+v", stored, files)
	}
	_ = a
}

// TestGitAndBaselineFileConvergence is AC-03.5: one edit, two paths, same
// file rows — provenance may differ, content must not.
func TestGitAndBaselineFileConvergence(t *testing.T) {
	store, db, _ := changesFixtureDB(t)
	seedTaskChain(t, db, "TSK-1")
	root := t.TempDir()
	a := discoveryFile(t, root, "a.py", "a = 1\n")
	if _, err := store.CaptureBaseline(t.Context(), "TSK-1", []string{a}, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a, []byte("a = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseline, err := store.BaselineFileDelta(t.Context(), "TSK-1", []string{a})
	if err != nil || len(baseline) != 1 {
		t.Fatalf("baseline delta = %+v, %v", baseline, err)
	}
	git, err := store.DiscoverFilesGit(t.Context(), porcelainRunner(t, " M a.py\x00"), root)
	if err != nil || len(git) != 1 {
		t.Fatalf("git delta = %+v, %v", git, err)
	}
	if baseline[0].Path != git[0].Path || baseline[0].Kind != git[0].Kind || baseline[0].Hash != git[0].Hash {
		t.Fatalf("baseline %+v vs git %+v: same edit, same rows", baseline[0], git[0])
	}
	if baseline[0].Via == git[0].Via {
		t.Fatal("provenance must differ across paths")
	}
}
