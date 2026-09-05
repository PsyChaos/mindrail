package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// requireModeEnforcement skips where permission bits do not stop a write.
func requireModeEnforcement(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test depends on")
	}
}

// TestProbeRepoConfigReportsAWorktreeThatRefusesTheScaffold is finding D4.
//
// Only the runtime root and the cache directory were ever probed, and both of
// those live under .git, which stays perfectly writable when the worktree above
// it does not. So a checkout whose scaffold can never be laid down answered
// "runtime paths fine, database absent", and every command printed `mindrail
// init` as the whole remedy while init failed with exit 4 each time it was run.
// The probe has to be able to answer for the third root too.
func TestProbeRepoConfigReportsAWorktreeThatRefusesTheScaffold(t *testing.T) {
	requireModeEnforcement(t)

	layout := newGitLayout(t)
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}
	if err := os.Chmod(layout.worktree, 0o500); err != nil {
		t.Fatalf("chmod the worktree read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(layout.worktree, 0o700) })

	// The condition is invisible to every probe that existed: .git is untouched.
	if probe, asked := paths.ProbeWritable(); !asked || !probe.Usable {
		t.Fatalf("ProbeWritable = (%+v, %v); this test is not exercising what it claims", probe, asked)
	}

	repo, asked := paths.ProbeRepoConfig()
	if !asked {
		t.Fatalf("ProbeRepoConfig reported no inputs for %+v", paths)
	}
	if repo.Usable {
		t.Fatalf("ProbeRepoConfig = %+v, want it unusable: `mindrail init` cannot create %q", repo, paths.RepoConfigDir)
	}
	if repo.Kind != RootRepository {
		t.Errorf("Kind = %q, want %q", repo.Kind, RootRepository)
	}
	if repo.Dir != paths.RepoConfigDir {
		t.Errorf("Dir = %q, want the repository config directory %q", repo.Dir, paths.RepoConfigDir)
	}
	if repo.Purpose != RootRepository.Purpose() {
		t.Errorf("Purpose = %q, want the repository config directory's own purpose", repo.Purpose)
	}
	if repo.Exists {
		t.Errorf("Exists = true for a .mindrail that was never created")
	}
	if repo.Probed != filepath.Clean(layout.worktree) {
		t.Errorf("Probed = %q, want the worktree %q: that is where the create has to happen",
			repo.Probed, layout.worktree)
	}

	payload := payloadOf(t, repo.Err)
	if payload.Metadata["root_kind"] != string(RootRepository) {
		t.Errorf("metadata root_kind = %q, want %q", payload.Metadata["root_kind"], RootRepository)
	}
	if !strings.Contains(payload.Why, paths.RepoConfigDir) {
		t.Errorf("payload.Why = %q does not name the directory that failed", payload.Why)
	}
	if len(payload.NextAction) == 0 {
		t.Fatalf("payload %+v is not actionable", payload)
	}
	if !strings.Contains(payload.NextAction[0], layout.worktree) {
		t.Errorf("first next action = %q, which does not name %q -- the directory whose mode refused the create",
			payload.NextAction[0], layout.worktree)
	}
	// No environment variable relocates repository content, and offering one
	// would send the reader to move a root that is not the one that failed.
	if joined := strings.Join(payload.NextAction, " | "); strings.Contains(joined, "MINDRAIL_") {
		t.Errorf("next actions %v offer an override for a root that is not this one", payload.NextAction)
	}

	// Carry out the remedy the probe printed, and confirm it clears the
	// condition: an advice that does not is not a remedy.
	if err := os.Chmod(layout.worktree, 0o700); err != nil {
		t.Fatalf("carry out the printed remedy: %v", err)
	}
	if repo, asked := paths.ProbeRepoConfig(); !asked || !repo.Usable {
		t.Fatalf("after the printed remedy ProbeRepoConfig = (%+v, %v), want it usable", repo, asked)
	}
}

// TestProbeRepoConfigOnAHealthyRepositoryIsUsable is the over-fire guard: an
// ordinary repository, with .mindrail absent and again once it exists, reports
// a usable repository config root.
func TestProbeRepoConfigOnAHealthyRepositoryIsUsable(t *testing.T) {
	layout := newGitLayout(t)
	if err := os.MkdirAll(layout.worktree, DirMode); err != nil {
		t.Fatalf("create the worktree: %v", err)
	}
	paths, err := ResolveRuntimePaths(layout.options())
	if err != nil {
		t.Fatalf("ResolveRuntimePaths: %v", err)
	}

	for _, stage := range []string{"before init", "after init"} {
		if stage == "after init" {
			if err := os.MkdirAll(paths.RepoConfigDir, 0o755); err != nil {
				t.Fatalf("create %q: %v", paths.RepoConfigDir, err)
			}
		}

		repo, asked := paths.ProbeRepoConfig()
		if !asked {
			t.Fatalf("%s: ProbeRepoConfig reported no inputs", stage)
		}
		if !repo.Usable || repo.Err != nil {
			t.Fatalf("%s: ProbeRepoConfig = %+v, want a usable repository config directory", stage, repo)
		}
		if want := stage == "after init"; repo.Exists != want {
			t.Errorf("%s: Exists = %v, want %v", stage, repo.Exists, want)
		}
	}

	// The adjacent condition this must not be confused with: an absent
	// .mindrail is the normal uninitialised state, not an unusable root. It is
	// asserted above by the "before init" stage reporting Usable with
	// Exists=false -- and EnsureDirs must still leave it alone.
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.worktree, ".mindrail-never")); err == nil {
		t.Errorf("EnsureDirs invented repository content")
	}
}

// TestProbeRepoConfigSeparatesNoInputFromNoPermission keeps the distinction a
// zero value cannot carry: a probe whose input was never derived has not run,
// and reading its zero Writability as "unwritable" would report every
// unreached step as a broken checkout.
func TestProbeRepoConfigSeparatesNoInputFromNoPermission(t *testing.T) {
	got, asked := RuntimePaths{}.ProbeRepoConfig()
	if asked {
		t.Fatalf("ProbeRepoConfig reported a real answer %+v for paths that were never derived", got)
	}
	if got != (Writability{}) {
		t.Errorf("ProbeRepoConfig = %+v, want the zero value when there was nothing to probe", got)
	}
}

// TestRemedyNamesAPathTheUserCanActuallyChmod covers the other way a printed
// remedy turns out to be uncarryable: a directory at mode 0000 answers no stat
// about anything inside it, so naming a path below it sends the reader to chmod
// something no chmod can reach. The mode that refused belongs to the directory
// that is still answerable.
func TestRemedyNamesAPathTheUserCanActuallyChmod(t *testing.T) {
	requireModeEnforcement(t)

	base := t.TempDir()
	sealed := filepath.Join(base, "sealed")
	if err := os.MkdirAll(sealed, 0o755); err != nil {
		t.Fatalf("create %q: %v", sealed, err)
	}
	inside := filepath.Join(sealed, "config.toml")
	if err := os.WriteFile(inside, []byte("[project]\n"), FileMode); err != nil {
		t.Fatalf("seed a file inside: %v", err)
	}
	if err := os.Chmod(sealed, 0o000); err != nil {
		t.Fatalf("seal %q: %v", sealed, err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o755) })

	payload := payloadOf(t, UnwritablePath(RootRepository, "repository configuration file", inside, os.ErrPermission))
	if payload.Metadata["probed_path"] != sealed {
		t.Errorf("probed_path = %q, want %q: the mode that refused is the sealed directory's",
			payload.Metadata["probed_path"], sealed)
	}
	if !strings.Contains(payload.NextAction[0], sealed) {
		t.Errorf("first next action = %q does not name %q", payload.NextAction[0], sealed)
	}
	if strings.Contains(payload.NextAction[0], inside) {
		t.Errorf("first next action = %q names a path no chmod can reach through a 0000 directory", payload.NextAction[0])
	}

	// The remedy, carried out: chmod the named path and the file is reachable.
	if err := os.Chmod(sealed, 0o700); err != nil {
		t.Fatalf("carry out the printed remedy: %v", err)
	}
	if _, err := os.Stat(inside); err != nil {
		t.Fatalf("after the printed remedy the file is still unreachable: %v", err)
	}
}

// TestObstructedDirNamesOnlyWhatCanNeverBeADirectory pins the read-side
// question and, just as importantly, what it refuses to answer. The loader asks
// it before reading configuration, so a false positive here would refuse to run
// in a repository that works today.
func TestObstructedDirNamesOnlyWhatCanNeverBeADirectory(t *testing.T) {
	base := t.TempDir()

	t.Run("a regular file in the way is an obstruction", func(t *testing.T) {
		path := filepath.Join(base, "file")
		if err := os.WriteFile(path, []byte("not a directory"), FileMode); err != nil {
			t.Fatalf("seed a file: %v", err)
		}

		err := ObstructedDir(RootRepository, path)
		if err == nil {
			t.Fatal("ObstructedDir = nil for a regular file standing where a directory has to be")
		}
		if !errors.Is(err, ErrNotDirectory) {
			t.Errorf("error = %v, want it to unwrap to ErrNotDirectory", err)
		}
		payload := payloadOf(t, err)
		if !strings.Contains(payload.NextAction[0], "remove or move aside") {
			t.Errorf("first next action = %q, which does not clear a file in the way", payload.NextAction[0])
		}
	})

	t.Run("a link to nothing is an obstruction", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("creating symlinks on Windows needs a privilege this test cannot assume")
		}
		path := filepath.Join(base, "dangling")
		if err := os.Symlink(filepath.Join(base, "nowhere"), path); err != nil {
			t.Fatalf("seed a dangling symlink: %v", err)
		}

		err := ObstructedDir(RootRepository, path)
		if !errors.Is(err, ErrDanglingSymlink) {
			t.Errorf("ObstructedDir = %v, want it to unwrap to ErrDanglingSymlink", err)
		}
	})

	t.Run("absent, read-only and symlinked directories are not", func(t *testing.T) {
		readOnly := filepath.Join(base, "read-only")
		if err := os.MkdirAll(readOnly, 0o500); err != nil {
			t.Fatalf("seed a read-only directory: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })

		target := filepath.Join(base, "target")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("seed a link target: %v", err)
		}
		link := filepath.Join(base, "link")
		if runtime.GOOS != "windows" {
			if err := os.Symlink(target, link); err != nil {
				t.Fatalf("seed a symlink: %v", err)
			}
		} else {
			link = target
		}

		for name, dir := range map[string]string{
			"absent":                filepath.Join(base, "not-there"),
			"present but read-only": readOnly,
			"a link to a real dir":  link,
		} {
			if err := ObstructedDir(RootRepository, dir); err != nil {
				t.Errorf("%s: ObstructedDir = %v, want nil: nothing is standing in the way", name, err)
			}
		}
	})
}
