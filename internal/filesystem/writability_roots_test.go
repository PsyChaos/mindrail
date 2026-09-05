package filesystem

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

func payloadOf(t *testing.T, err error) app.ErrorPayload {
	t.Helper()

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	return payload
}

// TestProbeAndEnsureAgreeAboutADanglingSymlink is finding F6.
//
// os.Stat follows a link and reports the absence at the far end, so the probe
// called a runtime root that is a link to nowhere "not there yet, and
// creatable" and doctor printed `mindrail init`. MkdirAll cannot write through
// such a link, so init then exited 4 with a permissions remedy for a parent
// whose permissions were already correct -- the repeated exit 4 the presence
// probe exists to prevent, reproduced one directory up.
func TestProbeAndEnsureAgreeAboutADanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows needs a privilege this test cannot assume")
	}

	layout := newGitLayout(t)
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "nowhere"), paths.RuntimeRoot); err != nil {
		t.Fatalf("seed a dangling symlink at the runtime root: %v", err)
	}

	probe, asked := paths.ProbeWritable()
	if !asked {
		t.Fatalf("ProbeWritable reported no inputs for %+v", paths)
	}
	if probe.Usable {
		t.Fatalf("ProbeWritable = %+v, want it unusable: `mindrail init` cannot write through a link to nowhere", probe)
	}
	if !errors.Is(probe.Err, ErrDanglingSymlink) {
		t.Errorf("Err = %v, want it to unwrap to ErrDanglingSymlink", probe.Err)
	}

	ensureErr := paths.EnsureDirs()
	if ensureErr == nil {
		t.Fatalf("EnsureDirs = nil, want the same refusal the probe reported")
	}

	probed := payloadOf(t, probe.Err)
	ensured := payloadOf(t, ensureErr)
	if probed.Why != ensured.Why {
		t.Errorf("doctor says %q and init says %q for one condition", probed.Why, ensured.Why)
	}
	if strings.Join(probed.NextAction, "|") != strings.Join(ensured.NextAction, "|") {
		t.Errorf("doctor advises %v and init advises %v for one condition", probed.NextAction, ensured.NextAction)
	}
	if !strings.Contains(probed.NextAction[0], "repoint") {
		t.Errorf("first next action %q does not clear a link that points at nothing", probed.NextAction[0])
	}
	if strings.Contains(probed.NextAction[0], "permissions") {
		t.Errorf("first next action %q sends the user to chmod a link no mode change can fix", probed.NextAction[0])
	}
}

// TestASymlinkedRuntimeRootStaysUsable is the over-fire guard for the check
// above: the condition being detected is a link that resolves to *nothing*, not
// a link. Pointing the runtime root at storage elsewhere is a supported layout,
// and refusing it would break healthy installations to catch a broken one.
func TestASymlinkedRuntimeRootStaysUsable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows needs a privilege this test cannot assume")
	}

	layout := newGitLayout(t)
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}

	elsewhere := filepath.Join(t.TempDir(), "runtime")
	if err := os.MkdirAll(elsewhere, DirMode); err != nil {
		t.Fatalf("create the real runtime directory: %v", err)
	}
	if err := os.Symlink(elsewhere, paths.RuntimeRoot); err != nil {
		t.Fatalf("link the runtime root at %q: %v", elsewhere, err)
	}

	probe, asked := paths.ProbeWritable()
	if !asked || !probe.Usable {
		t.Fatalf("ProbeWritable = (%+v, %v), want a usable answer for a link to a real writable directory", probe, asked)
	}
	if !probe.Exists {
		t.Errorf("Exists = false for a runtime root that resolves to %q", elsewhere)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs = %v, want no error through a resolving symlink", err)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, cacheDirName)); err != nil {
		t.Errorf("EnsureDirs did not create the cache directory through the link: %v", err)
	}
}

// TestHealthyRuntimeRootIsUnaffected is the over-fire guard every new detection
// in this file shares: an ordinary repository, before and after `mindrail
// init`, reports usable roots and no errors.
func TestHealthyRuntimeRootIsUnaffected(t *testing.T) {
	layout := newGitLayout(t)
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}

	for _, stage := range []string{"before init", "after init"} {
		if stage == "after init" {
			if err := paths.EnsureDirs(); err != nil {
				t.Fatalf("EnsureDirs: %v", err)
			}
		}

		probe, asked := paths.ProbeWritable()
		if !asked {
			t.Fatalf("%s: ProbeWritable reported no inputs", stage)
		}
		if !probe.Usable || probe.Err != nil {
			t.Fatalf("%s: ProbeWritable = %+v, want a usable runtime root", stage, probe)
		}
		if probe.Kind != RootRuntime {
			t.Errorf("%s: Kind = %q, want %q", stage, probe.Kind, RootRuntime)
		}
		if probe.Dir != paths.RuntimeRoot {
			t.Errorf("%s: Dir = %q, want the runtime root %q", stage, probe.Dir, paths.RuntimeRoot)
		}

		roots := paths.ProbeRoots()
		if len(roots) != 2 {
			t.Fatalf("%s: ProbeRoots = %d answers, want one per directory EnsureDirs creates", stage, len(roots))
		}
		for _, root := range roots {
			if !root.Usable || root.Err != nil {
				t.Errorf("%s: %s at %q is reported unusable in a healthy repository: %v", stage, root.Kind, root.Dir, root.Err)
			}
		}
	}
}

// TestAnUnusableCacheDirectoryIsNotReportedAsTheRuntimeRoot is finding F7.
//
// ProbeWritable returned whichever root failed first, so a caller labelling the
// answer `runtime_root_exists` / `runtime_root_usable` -- which is what doctor
// does -- published the cache directory's verdict under the runtime root's
// name, and the sentence beside it called the cache "the runtime directory".
func TestAnUnusableCacheDirectoryIsNotReportedAsTheRuntimeRoot(t *testing.T) {
	layout := newGitLayout(t)
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}
	if err := os.MkdirAll(paths.RuntimeRoot, DirMode); err != nil {
		t.Fatalf("create the runtime root: %v", err)
	}
	if err := os.WriteFile(paths.CacheDir, []byte("not a directory"), FileMode); err != nil {
		t.Fatalf("seed a file where the cache directory belongs: %v", err)
	}

	probe, asked := paths.ProbeWritable()
	if !asked {
		t.Fatalf("ProbeWritable reported no inputs for %+v", paths)
	}
	if probe.Dir != paths.RuntimeRoot {
		t.Fatalf("ProbeWritable answered about %q; a caller labelling it runtime_root would be lying", probe.Dir)
	}
	if !probe.Usable {
		t.Fatalf("ProbeWritable = %+v, want the runtime root reported usable: it is", probe)
	}

	roots := paths.ProbeRoots()
	if len(roots) != 2 {
		t.Fatalf("ProbeRoots = %d answers, want 2", len(roots))
	}
	if !roots[0].Usable || roots[0].Kind != RootRuntime {
		t.Errorf("ProbeRoots[0] = %+v, want a usable runtime root first", roots[0])
	}

	cache := roots[1]
	if cache.Kind != RootCache {
		t.Fatalf("ProbeRoots[1].Kind = %q, want %q", cache.Kind, RootCache)
	}
	if cache.Usable {
		t.Fatalf("ProbeRoots[1] = %+v, want the cache directory reported unusable", cache)
	}

	payload := payloadOf(t, cache.Err)
	if strings.Contains(payload.Why, string(RootRuntime)) {
		t.Errorf("payload.Why = %q calls the cache directory the runtime root", payload.Why)
	}
	if !strings.Contains(payload.Why, string(RootCache)) {
		t.Errorf("payload.Why = %q does not name the directory that failed", payload.Why)
	}
	if !strings.Contains(payload.Why, paths.CacheDir) {
		t.Errorf("payload.Why = %q does not name %q", payload.Why, paths.CacheDir)
	}
	if payload.Metadata["root_kind"] != string(RootCache) {
		t.Errorf("metadata root_kind = %q, want %q", payload.Metadata["root_kind"], RootCache)
	}
	if !strings.Contains(strings.Join(payload.NextAction, " | "), "MINDRAIL_CACHE_DIR") {
		t.Errorf("next actions %v offer the override for a different directory", payload.NextAction)
	}
	if cache.Purpose == "" || cache.Purpose == RootRuntime.Purpose() {
		t.Errorf("Purpose = %q, want the cache directory's own purpose", cache.Purpose)
	}
}

// TestRootKindOfNamesTheRootThatStoppedEnsureDirs is the seam a caller needs to
// decide how fatal a failure is. Without it, EnsureDirs' single error forces
// the cache directory and the runtime root to be treated identically.
func TestRootKindOfNamesTheRootThatStoppedEnsureDirs(t *testing.T) {
	layout := newGitLayout(t)
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}
	if err := os.MkdirAll(paths.RuntimeRoot, DirMode); err != nil {
		t.Fatalf("create the runtime root: %v", err)
	}
	if err := os.WriteFile(paths.CacheDir, []byte("not a directory"), FileMode); err != nil {
		t.Fatalf("seed a file where the cache directory belongs: %v", err)
	}

	kind, ok := RootKindOf(paths.EnsureDirs())
	if !ok {
		t.Fatalf("RootKindOf did not identify the root that failed")
	}
	if kind != RootCache {
		t.Errorf("RootKindOf = %q, want %q", kind, RootCache)
	}

	// An error from somewhere else is not silently attributed to a root.
	if kind, ok := RootKindOf(errors.New("unrelated")); ok {
		t.Errorf("RootKindOf(unrelated) = (%q, true), want no answer", kind)
	}
	if kind, ok := RootKindOf(nil); ok {
		t.Errorf("RootKindOf(nil) = (%q, true), want no answer", kind)
	}
}

// TestUnwritablePathRemedyMatchesTheObstruction is finding F8. "Check the
// permissions" was always first, including for obstructions no mode change can
// clear, and the advice a user follows first has to be the one that works.
func TestUnwritablePathRemedyMatchesTheObstruction(t *testing.T) {
	const dir = "/repo/.git/mindrail"

	cases := []struct {
		name        string
		probed      string
		cause       error
		wantFirst   string
		unwantFirst string
	}{
		{
			name: "a file in the way", probed: dir, cause: ErrNotDirectory,
			wantFirst: "remove or move aside", unwantFirst: "permissions",
		},
		{
			name: "a link that points at nothing", probed: dir, cause: ErrDanglingSymlink,
			wantFirst: "repoint", unwantFirst: "permissions",
		},
		{
			name: "the directory itself is read-only", probed: dir, cause: fs.ErrPermission,
			wantFirst: "permissions",
		},
		{
			name: "the parent is read-only", probed: "/repo/.git", cause: fs.ErrPermission,
			wantFirst: "permissions",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := payloadOf(t, unwritablePathError(RootRuntime, dir, tc.probed, tc.cause))
			if len(payload.NextAction) == 0 {
				t.Fatalf("payload %+v is not actionable", payload)
			}
			first := payload.NextAction[0]
			if !strings.Contains(first, tc.wantFirst) {
				t.Errorf("first next action = %q, want it to contain %q", first, tc.wantFirst)
			}
			if tc.unwantFirst != "" && strings.Contains(first, tc.unwantFirst) {
				t.Errorf("first next action = %q, which cannot clear this obstruction", first)
			}
			if !strings.Contains(strings.Join(payload.NextAction, " | "), "MINDRAIL_RUNTIME_DIR") {
				t.Errorf("next actions %v drop the relocation escape hatch", payload.NextAction)
			}
		})
	}
}

// TestEnsureDirsAndProbeDescribeAFileInTheWayIdentically is the other half of
// F8: one condition, two commands, one sentence.
func TestEnsureDirsAndProbeDescribeAFileInTheWayIdentically(t *testing.T) {
	layout := newGitLayout(t)
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}
	if err := os.WriteFile(paths.RuntimeRoot, []byte("not a directory"), FileMode); err != nil {
		t.Fatalf("seed a file where the runtime root belongs: %v", err)
	}

	probe, _ := paths.ProbeWritable()
	if probe.Usable {
		t.Fatalf("ProbeWritable = %+v, want it unusable with a file in the way", probe)
	}
	ensureErr := paths.EnsureDirs()
	if ensureErr == nil {
		t.Fatalf("EnsureDirs = nil, want the same refusal")
	}

	probed := payloadOf(t, probe.Err)
	ensured := payloadOf(t, ensureErr)
	if probed.Why != ensured.Why {
		t.Errorf("doctor says %q and init says %q for one condition", probed.Why, ensured.Why)
	}
	if strings.Join(probed.NextAction, "|") != strings.Join(ensured.NextAction, "|") {
		t.Errorf("doctor advises %v and init advises %v for one condition", probed.NextAction, ensured.NextAction)
	}
	if !errors.Is(ensureErr, ErrNotDirectory) {
		t.Errorf("EnsureDirs error = %v, want it to unwrap to ErrNotDirectory", ensureErr)
	}
}
