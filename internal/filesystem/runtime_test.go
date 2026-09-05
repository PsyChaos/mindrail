package filesystem

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// gitLayout mirrors what the git adapter hands over: an absolute common-dir and
// an absolute worktree root.
type gitLayout struct {
	worktree  string
	commonDir string
}

func newGitLayout(t *testing.T) gitLayout {
	t.Helper()

	base := t.TempDir()
	layout := gitLayout{
		worktree:  filepath.Join(base, "repo"),
		commonDir: filepath.Join(base, "repo", ".git"),
	}
	if err := os.MkdirAll(layout.commonDir, DirMode); err != nil {
		t.Fatalf("create fake git layout: %v", err)
	}
	return layout
}

func (l gitLayout) options() PathOptions {
	return PathOptions{CommonDir: l.commonDir, WorktreeRoot: l.worktree}
}

func TestResolveRuntimePathsUnderCommonDir(t *testing.T) {
	layout := newGitLayout(t)

	t.Run("derived from the common dir", func(t *testing.T) {
		got, err := ResolveRuntimePaths(layout.options())
		if err != nil {
			t.Fatalf("ResolveRuntimePaths: %v", err)
		}

		wantRuntimeRoot := filepath.Join(layout.commonDir, ProductName)
		want := RuntimePaths{
			CommonDir:     layout.commonDir,
			WorktreeRoot:  layout.worktree,
			RuntimeRoot:   wantRuntimeRoot,
			DBPath:        filepath.Join(wantRuntimeRoot, ProductName+".db"),
			CacheDir:      filepath.Join(wantRuntimeRoot, "cache"),
			RepoConfigDir: filepath.Join(layout.worktree, "."+ProductName),
		}
		if got != want {
			t.Fatalf("ResolveRuntimePaths = %+v, want %+v", got, want)
		}
	})

	t.Run("runtime dir override relocates the db and cache", func(t *testing.T) {
		override := filepath.Join(t.TempDir(), "isolated")
		opts := layout.options()
		opts.RuntimeDirOverride = override

		got, err := ResolveRuntimePaths(opts)
		if err != nil {
			t.Fatalf("ResolveRuntimePaths with a runtime override: %v", err)
		}
		if got.RuntimeRoot != override {
			t.Fatalf("RuntimeRoot = %q, want %q", got.RuntimeRoot, override)
		}
		if want := filepath.Join(override, ProductName+".db"); got.DBPath != want {
			t.Fatalf("DBPath = %q, want %q", got.DBPath, want)
		}
		if want := filepath.Join(override, "cache"); got.CacheDir != want {
			t.Fatalf("CacheDir = %q, want %q", got.CacheDir, want)
		}
		if got.CommonDir != layout.commonDir {
			t.Fatalf("CommonDir = %q, want it untouched at %q", got.CommonDir, layout.commonDir)
		}
	})

	t.Run("cache dir override is independent", func(t *testing.T) {
		override := filepath.Join(t.TempDir(), "cache-only")
		opts := layout.options()
		opts.CacheDirOverride = override

		got, err := ResolveRuntimePaths(opts)
		if err != nil {
			t.Fatalf("ResolveRuntimePaths with a cache override: %v", err)
		}
		if got.CacheDir != override {
			t.Fatalf("CacheDir = %q, want %q", got.CacheDir, override)
		}
		if want := filepath.Join(layout.commonDir, ProductName); got.RuntimeRoot != want {
			t.Fatalf("RuntimeRoot = %q, want %q; a cache override must not move it", got.RuntimeRoot, want)
		}
	})

	t.Run("relative override stays under the common dir", func(t *testing.T) {
		opts := layout.options()
		opts.RuntimeDirOverride = "nested/runtime"

		got, err := ResolveRuntimePaths(opts)
		if err != nil {
			t.Fatalf("ResolveRuntimePaths with a relative override: %v", err)
		}
		if want := filepath.Join(layout.commonDir, "nested", "runtime"); got.RuntimeRoot != want {
			t.Fatalf("RuntimeRoot = %q, want %q", got.RuntimeRoot, want)
		}
	})

	t.Run("json keys are the wire contract", func(t *testing.T) {
		paths, err := ResolveRuntimePaths(layout.options())
		if err != nil {
			t.Fatalf("ResolveRuntimePaths: %v", err)
		}
		encoded, err := json.Marshal(paths)
		if err != nil {
			t.Fatalf("marshal RuntimePaths: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("unmarshal RuntimePaths: %v", err)
		}
		for _, key := range []string{"common_dir", "worktree_root", "runtime_root", "db_path", "cache_dir", "repo_config_dir"} {
			if _, ok := decoded[key]; !ok {
				t.Errorf("RuntimePaths JSON is missing key %q; got %s", key, encoded)
			}
		}
	})

	t.Run("missing inputs are rejected", func(t *testing.T) {
		for _, opts := range []PathOptions{
			{WorktreeRoot: layout.worktree},
			{CommonDir: layout.commonDir},
			{CommonDir: "relative/.git", WorktreeRoot: layout.worktree},
			{CommonDir: layout.commonDir, WorktreeRoot: "relative"},
		} {
			if _, err := ResolveRuntimePaths(opts); err == nil {
				t.Errorf("ResolveRuntimePaths(%+v) = nil error, want a rejection", opts)
			}
		}
	})
}

func TestResolveRuntimePathsRejectsTraversalOverride(t *testing.T) {
	layout := newGitLayout(t)

	tests := []struct {
		name    string
		mutate  func(*PathOptions)
		wantErr error
	}{
		{
			name:    "relative runtime traversal",
			mutate:  func(o *PathOptions) { o.RuntimeDirOverride = filepath.Join("..", "..", "..", "etc") },
			wantErr: ErrEscapesRoot,
		},
		{
			// Spelled by hand rather than with filepath.Join, which would clean
			// the traversal away before the code under test ever sees it. An
			// environment variable arrives uncleaned.
			name: "absolute runtime traversal",
			mutate: func(o *PathOptions) {
				o.RuntimeDirOverride = strings.Join([]string{layout.commonDir, "..", "..", "escape"}, string(os.PathSeparator))
			},
			wantErr: ErrEscapesRoot,
		},
		{
			name:    "relative cache traversal",
			mutate:  func(o *PathOptions) { o.CacheDirOverride = filepath.Join("..", "outside") },
			wantErr: ErrEscapesRoot,
		},
		{
			name:    "slash form traversal",
			mutate:  func(o *PathOptions) { o.RuntimeDirOverride = "../outside" },
			wantErr: ErrEscapesRoot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := layout.options()
			tt.mutate(&opts)

			got, err := ResolveRuntimePaths(opts)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ResolveRuntimePaths(%+v) error = %v, want %v", opts, err, tt.wantErr)
			}
			if got != (RuntimePaths{}) {
				t.Fatalf("ResolveRuntimePaths returned %+v alongside an error", got)
			}
			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("rejection carries no domain payload: %v", err)
			}
			if payload.Code != app.CodePathEscapesRoot {
				t.Fatalf("payload code = %q, want %q", payload.Code, app.CodePathEscapesRoot)
			}
		})
	}
}

func TestEnsureDirsAndWriteFileUseRestrictiveModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not carry Unix permission bits")
	}

	layout := newGitLayout(t)
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}

	if paths.Exists() {
		t.Fatalf("Exists() = true before anything was created")
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	// Idempotence matters: init runs on repositories that were already set up.
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("second EnsureDirs: %v", err)
	}

	for _, dir := range []string{paths.RuntimeRoot, paths.CacheDir} {
		assertMode(t, dir, DirMode)
	}

	// The repository config directory is repository content, not runtime state:
	// status and doctor must not conjure it (spec §83, decision D-01).
	if _, err := os.Stat(paths.RepoConfigDir); !os.IsNotExist(err) {
		t.Fatalf("EnsureDirs created %q; only init may write repository content", paths.RepoConfigDir)
	}

	if paths.Exists() {
		t.Fatalf("Exists() = true with no database file present")
	}
	if err := os.WriteFile(paths.DBPath, []byte("db"), FileMode); err != nil {
		t.Fatalf("seed database file: %v", err)
	}
	if !paths.Exists() {
		t.Fatalf("Exists() = false with a database file present")
	}

	root := newTestRoot(t, layout.worktree)

	written, path, err := root.WriteFileIfAbsent(".mindrail/config.toml", []byte("first"))
	if err != nil {
		t.Fatalf("WriteFileIfAbsent: %v", err)
	}
	if !written {
		t.Fatalf("WriteFileIfAbsent reported no write for an absent file")
	}
	assertMode(t, path, FileMode)
	assertMode(t, filepath.Dir(path), DirMode)

	written, path2, err := root.WriteFileIfAbsent(".mindrail/config.toml", []byte("second"))
	if err != nil {
		t.Fatalf("second WriteFileIfAbsent: %v", err)
	}
	if written {
		t.Fatalf("WriteFileIfAbsent overwrote an existing file")
	}
	if path2 != path {
		t.Fatalf("second call reported path %q, want %q", path2, path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back config: %v", err)
	}
	if string(content) != "first" {
		t.Fatalf("config content = %q, want %q", content, "first")
	}

	dir, err := root.EnsureDir(".mindrail/knowledge/decisions")
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	assertMode(t, dir, DirMode)
	assertMode(t, filepath.Dir(dir), DirMode)
}

func TestEnsureDirsReportsUnwritableRuntimePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix directory permissions this way")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions, so there is nothing to observe")
	}

	layout := newGitLayout(t)
	if err := os.Chmod(layout.commonDir, 0o500); err != nil {
		t.Fatalf("make the common dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(layout.commonDir, DirMode) })

	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}

	err = paths.EnsureDirs()
	if err == nil {
		t.Fatalf("EnsureDirs succeeded under a read-only common dir")
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		t.Fatalf("payload code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
	}
	if payload.Impact == "" || len(payload.NextAction) == 0 {
		t.Fatalf("payload %q is not actionable: %+v", payload.Code, payload)
	}
	// Decision D-03: an unwritable runtime path is an environment condition,
	// not a usage mistake.
	if got := app.ExitCode(err); got != app.ExitUnavailable {
		t.Fatalf("ExitCode = %d, want %d (unavailable)", got, app.ExitUnavailable)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("the underlying permission error was not preserved: %v", err)
	}
}

func assertMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%q mode = %04o, want %04o", path, got, want)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("%q is readable or writable by group or other (%04o)", path, info.Mode().Perm())
	}
}

// TestProbeWritableReportsAnUnwritableRuntimePathWithoutCreatingIt is the
// read-only half of the question EnsureDirs answers by writing. Doctor and
// status may not create anything (decision D-01), so if the only way to learn
// that a runtime path is unusable were to try, they would have to report an
// unwritable repository as a merely uninitialized one.
func TestProbeWritableReportsAnUnwritableRuntimePathWithoutCreatingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix directory permissions this way")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions, so there is nothing to observe")
	}

	layout := newGitLayout(t)
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}

	t.Run("a writable common dir is usable before anything exists", func(t *testing.T) {
		got, probed := paths.ProbeWritable()
		if !probed {
			t.Fatalf("ProbeWritable reported no inputs for %+v", paths)
		}
		if !got.Usable {
			t.Fatalf("ProbeWritable = %+v, want it usable under a writable common dir: %v", got, got.Err)
		}
		if got.Exists {
			t.Errorf("Exists = true for a runtime root that was never created")
		}
		if got.Err != nil {
			t.Errorf("Err = %v alongside Usable", got.Err)
		}
		if _, statErr := os.Stat(paths.RuntimeRoot); !os.IsNotExist(statErr) {
			t.Fatalf("the probe created %q; it must answer without mutating anything", paths.RuntimeRoot)
		}
	})

	t.Run("a read-only common dir is reported as unwritable", func(t *testing.T) {
		if err := os.Chmod(layout.commonDir, 0o500); err != nil {
			t.Fatalf("make the common dir read-only: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(layout.commonDir, DirMode) })

		got, probed := paths.ProbeWritable()
		if !probed {
			t.Fatalf("ProbeWritable reported no inputs for %+v", paths)
		}
		if got.Usable {
			t.Fatalf("ProbeWritable = %+v, want it unusable under a read-only common dir", got)
		}
		if got.Probed != layout.commonDir {
			t.Errorf("Probed = %q, want the nearest existing ancestor %q, which is where the create would land", got.Probed, layout.commonDir)
		}
		payload, ok := app.PayloadOf(got.Err)
		if !ok {
			t.Fatalf("Err carries no domain payload: %v", got.Err)
		}
		if payload.Code != app.CodeRuntimePathUnwritable {
			t.Fatalf("payload code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
		}
		if payload.Impact == "" || len(payload.NextAction) == 0 {
			t.Fatalf("payload %q is not actionable: %+v", payload.Code, payload)
		}
		if got := app.ExitCode(got.Err); got != app.ExitUnavailable {
			t.Fatalf("ExitCode = %d, want %d (unavailable, decision D-03)", got, app.ExitUnavailable)
		}
		if !errors.Is(got.Err, fs.ErrPermission) {
			t.Errorf("the underlying permission error was not preserved: %v", got.Err)
		}
		if _, statErr := os.Stat(paths.RuntimeRoot); !os.IsNotExist(statErr) {
			t.Fatalf("the probe created %q", paths.RuntimeRoot)
		}
	})

	t.Run("an existing writable runtime root is usable and reported as existing", func(t *testing.T) {
		if err := paths.EnsureDirs(); err != nil {
			t.Fatalf("EnsureDirs: %v", err)
		}

		got, probed := paths.ProbeWritable()
		if !probed || !got.Usable {
			t.Fatalf("ProbeWritable = (%+v, %v), want a usable answer", got, probed)
		}
		if !got.Exists {
			t.Errorf("Exists = false for a runtime root that is on disk at %q", paths.RuntimeRoot)
		}
	})
}

// TestProbeWritableSeparatesNoInputFromNoPermission guards the distinction the
// startup sequence keeps getting wrong: a check whose inputs were never
// populated has not run, and must not be rendered as one that ran and found a
// problem.
func TestProbeWritableSeparatesNoInputFromNoPermission(t *testing.T) {
	got, probed := RuntimePaths{}.ProbeWritable()
	if probed {
		t.Fatalf("ProbeWritable reported a real answer %+v for paths that were never derived", got)
	}
	if got != (Writability{}) {
		t.Fatalf("ProbeWritable = %+v, want the zero value when there was nothing to probe", got)
	}
	if got.Err != nil {
		t.Fatalf("Err = %v; an unprobed path is not an unwritable one", got.Err)
	}
}

// TestProbeWritableRejectsAFileInThePathOfTheRuntimeRoot covers the other way a
// derived location can be unusable: something that is not a directory already
// occupies it.
func TestProbeWritableRejectsAFileInThePathOfTheRuntimeRoot(t *testing.T) {
	layout := newGitLayout(t)
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}
	if err := os.WriteFile(paths.RuntimeRoot, []byte("not a directory"), FileMode); err != nil {
		t.Fatalf("seed a file where the runtime root belongs: %v", err)
	}

	got, probed := paths.ProbeWritable()
	if !probed {
		t.Fatalf("ProbeWritable reported no inputs for %+v", paths)
	}
	if got.Usable {
		t.Fatalf("ProbeWritable = %+v, want it unusable with a file in the way", got)
	}
	if !errors.Is(got.Err, ErrNotDirectory) {
		t.Fatalf("Err = %v, want it to unwrap to ErrNotDirectory", got.Err)
	}
	payload, ok := app.PayloadOf(got.Err)
	if !ok || payload.Code != app.CodeRuntimePathUnwritable {
		t.Fatalf("payload = %+v, want %q", payload, app.CodeRuntimePathUnwritable)
	}
}
