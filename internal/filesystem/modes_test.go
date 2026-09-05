package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoModes is the pair repository content takes: committed, reviewed and read
// by every contributor and by CI, so not the owner-only runtime pair.
var repoModes = Modes{Dir: 0o755, File: 0o644}

// TestPrivateModesAreTheOwnerOnlyDefaults pins what the unparameterized methods
// still use. Spec §113 makes the runtime tree owner-only by construction, and a
// widened default would relax it everywhere at once and silently.
func TestPrivateModesAreTheOwnerOnlyDefaults(t *testing.T) {
	modes := PrivateModes()
	if modes.Dir != DirMode || modes.File != FileMode {
		t.Fatalf("PrivateModes() = %v, want {%v %v}", modes, DirMode, FileMode)
	}

	// A caller must not be able to widen the default for the rest of the
	// process by editing what it was handed.
	modes.Dir, modes.File = 0o777, 0o666
	if again := PrivateModes(); again.Dir != DirMode || again.File != FileMode {
		t.Fatalf("PrivateModes() = %v after a caller edited its copy, want {%v %v}", again, DirMode, FileMode)
	}
}

// TestEnsureDirModeCreatesWithTheGivenModeAndReportsIt is the seam the
// repository scaffolding needs: containment from Root, repository permissions
// from the caller, and an honest answer about whether this call created the
// directory so nobody has to stat an unvetted path to find out.
func TestEnsureDirModeCreatesWithTheGivenModeAndReportsIt(t *testing.T) {
	root := newTestRoot(t, t.TempDir())

	abs, created, err := root.EnsureDirMode("a/b", repoModes.Dir)
	if err != nil {
		t.Fatalf("EnsureDirMode() error = %v", err)
	}
	if !created {
		t.Error("created = false on the first call, want true")
	}

	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(abs)
		if statErr != nil {
			t.Fatalf("stat %s: %v", abs, statErr)
		}
		if info.Mode().Perm()&0o055 == 0 {
			t.Errorf("mode = %v, want the requested repository mode, not the private default", info.Mode().Perm())
		}
	}

	again, created, err := root.EnsureDirMode("a/b", repoModes.Dir)
	if err != nil {
		t.Fatalf("second EnsureDirMode() error = %v", err)
	}
	if created {
		t.Error("created = true on the second call, want false")
	}
	if again != abs {
		t.Errorf("EnsureDirMode returned %q then %q", abs, again)
	}
}

// TestWriteFileIfAbsentModeUsesTheGivenModes proves the same for files, and
// that the parent it creates on the way takes the caller's directory mode
// rather than the private one.
func TestWriteFileIfAbsentModeUsesTheGivenModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}

	root := newTestRoot(t, t.TempDir())

	written, abs, err := root.WriteFileIfAbsentMode("parent/child.txt", []byte("x"), repoModes)
	if err != nil || !written {
		t.Fatalf("WriteFileIfAbsentMode() = (written %v, err %v), want (true, nil)", written, err)
	}

	file, err := os.Stat(abs)
	if err != nil {
		t.Fatalf("stat %s: %v", abs, err)
	}
	if file.Mode().Perm()&0o044 == 0 {
		t.Errorf("file mode = %v, want the requested repository mode", file.Mode().Perm())
	}

	parent, err := os.Stat(filepath.Dir(abs))
	if err != nil {
		t.Fatalf("stat parent: %v", err)
	}
	if parent.Mode().Perm()&0o055 == 0 {
		t.Errorf("parent mode = %v, want the requested repository mode", parent.Mode().Perm())
	}
}

// TestModeVariantsStillRefuseAnEscape is the guard that matters: a caller may
// choose the permissions, never the boundary.
func TestModeVariantsStillRefuseAnEscape(t *testing.T) {
	requireSymlinks(t)

	base := t.TempDir()
	rootDir := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{rootDir, outside} {
		if err := os.MkdirAll(dir, DirMode); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(rootDir, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	root := newTestRoot(t, rootDir)

	if _, _, err := root.EnsureDirMode("escape/sub", repoModes.Dir); !errors.Is(err, ErrEscapesRoot) {
		t.Errorf("EnsureDirMode() error = %v, want ErrEscapesRoot", err)
	}
	if written, _, err := root.WriteFileIfAbsentMode("escape/f.txt", []byte("pwned"), repoModes); !errors.Is(err, ErrEscapesRoot) || written {
		t.Errorf("WriteFileIfAbsentMode() = (written %v, err %v), want (false, ErrEscapesRoot)", written, err)
	}

	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("read %s: %v", outside, err)
	}
	if len(entries) != 0 {
		t.Fatalf("%s is not empty; the escape was not contained", outside)
	}
}

// TestRepositoryRootIsItsOwnKind is the H10 half on this side: the repository
// config directory is not one of the two machine-local roots, and a report that
// files it under either one names the wrong directory and offers an environment
// variable that does not move it (decision D-07 names two overrides only).
func TestRepositoryRootIsItsOwnKind(t *testing.T) {
	if RootRepository == RootRuntime || RootRepository == RootCache {
		t.Fatalf("RootRepository = %q, which collides with a machine-local root", RootRepository)
	}
	if RootRepository.Purpose() == RootRuntime.Purpose() || RootRepository.Purpose() == RootCache.Purpose() {
		t.Error("RootRepository.Purpose() repeats a machine-local root's purpose")
	}
	if RootRepository.Impact() == RootRuntime.Impact() || RootRepository.Impact() == RootCache.Impact() {
		t.Error("RootRepository.Impact() repeats a machine-local root's impact")
	}

	err := unwritablePathError(RootRepository, "/repo/.mindrail", "/repo/.mindrail", os.ErrPermission)
	payload := payloadOf(t, err)
	if payload.Metadata["root_kind"] != string(RootRepository) {
		t.Errorf("metadata root_kind = %q, want %q", payload.Metadata["root_kind"], RootRepository)
	}
	for _, remedy := range payload.NextAction {
		if strings.Contains(remedy, "MINDRAIL_") {
			t.Errorf("next_action %q offers an environment override; none relocates the repository config directory", remedy)
		}
	}

	kind, known := RootKindOf(err)
	if !known || kind != RootRepository {
		t.Errorf("RootKindOf = (%q, %v), want (%q, true)", kind, known, RootRepository)
	}
}

// TestMachineLocalRootsStillOfferTheirOverride is the over-fire guard for the
// test above: suppressing the override line for the repository root must not
// suppress it for the two roots decision D-07 does give an escape hatch.
func TestMachineLocalRootsStillOfferTheirOverride(t *testing.T) {
	cases := map[RootKind]string{
		RootRuntime: "MINDRAIL_RUNTIME_DIR",
		RootCache:   "MINDRAIL_CACHE_DIR",
	}

	for kind, want := range cases {
		payload := payloadOf(t, unwritablePathError(kind, "/somewhere", "/somewhere", os.ErrPermission))

		var found bool
		for _, remedy := range payload.NextAction {
			if strings.Contains(remedy, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s next_action = %v, want one naming %s", kind, payload.NextAction, want)
		}
		if payload.Metadata["root_kind"] != string(kind) {
			t.Errorf("%s metadata root_kind = %q, want %q", kind, payload.Metadata["root_kind"], kind)
		}
	}
}

// TestRootKindOfStillRefusesWhatItDoesNotKnow keeps the third kind from turning
// RootKindOf into a function that answers for everything. A caller reads a
// "true" here as "this error is about a root I can grade".
func TestRootKindOfStillRefusesWhatItDoesNotKnow(t *testing.T) {
	for _, err := range []error{
		nil,
		errors.New("unrelated"),
		escapeError("some/path"),
	} {
		if kind, ok := RootKindOf(err); ok {
			t.Errorf("RootKindOf(%v) = (%q, true), want no answer", err, kind)
		}
	}
}
