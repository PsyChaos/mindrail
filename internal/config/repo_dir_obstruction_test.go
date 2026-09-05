package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

// loadAt runs one resolution pass against a worktree with no user layer and no
// environment, so the only thing under test is the repository layer.
func loadAt(t *testing.T, worktree string) (config.Loaded, error) {
	t.Helper()

	return config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  worktree,
		UserConfigDir: filepath.Join(t.TempDir(), "no-user-config"),
		Environ:       []string{},
	}).Load()
}

// TestARegularFileAtTheRepoConfigDirIsOneConditionWithOneRemedy is finding D5.
//
// With a regular file at <worktree>/.mindrail, reading and writing fail for the
// same reason and neither can succeed until it is moved. They were reported as
// two unrelated faults: the loader turned the ENOTDIR into CONFIG_INVALID at
// exit 2 and advised "Fix or remove the offending entry in
// <repo>/.mindrail/config.toml", a path no editor can open, while the scaffold
// turned it into RUNTIME_PATH_UNWRITABLE at exit 4 and advised checking the
// permissions and free space on a file that is mode 0644, owned by the caller,
// on a disk with room. Two codes, two exit classes, two remedies, neither of
// which clears it.
func TestARegularFileAtTheRepoConfigDirIsOneConditionWithOneRemedy(t *testing.T) {
	worktree := t.TempDir()
	repoDir := filepath.Join(worktree, config.RepoDir)
	if err := os.WriteFile(repoDir, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("seed a file where .mindrail belongs: %v", err)
	}

	_, loadErr := loadAt(t, worktree)
	if loadErr == nil {
		t.Fatal("Load() = nil; a configuration directory that is a file cannot be read")
	}
	_, _, writeErr := config.WriteIfAbsent(worktree)
	if writeErr == nil {
		t.Fatal("WriteIfAbsent() = nil; a configuration directory that is a file cannot be written")
	}

	read := payload(t, loadErr)
	write := payload(t, writeErr)

	if read.Code != write.Code {
		t.Errorf("status reports %q and init reports %q for one condition", read.Code, write.Code)
	}
	if read.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("code = %q, want %q: something is standing in the path, which is not a malformed configuration file (D-03)",
			read.Code, app.CodeRuntimePathUnwritable)
	}
	if read.Why != write.Why {
		t.Errorf("status says %q and init says %q for one condition", read.Why, write.Why)
	}
	if strings.Join(read.NextAction, "|") != strings.Join(write.NextAction, "|") {
		t.Errorf("status advises %v and init advises %v for one condition", read.NextAction, write.NextAction)
	}
	if !errors.Is(loadErr, filesystem.ErrNotDirectory) {
		t.Errorf("Load() error = %v, want it to unwrap to ErrNotDirectory", loadErr)
	}

	first := read.NextAction[0]
	if !strings.Contains(first, repoDir) {
		t.Errorf("first next action = %q does not name %q, the entry in the way", first, repoDir)
	}
	if strings.Contains(first, config.ConfigFileName) {
		t.Errorf("first next action = %q sends the reader to a file that cannot be opened", first)
	}
	if strings.Contains(strings.ToLower(first), "permission") || strings.Contains(strings.ToLower(first), "free space") {
		t.Errorf("first next action = %q blames a mode and a disk that are both fine", first)
	}

	// Carry out the printed remedy exactly, and require that it clears the
	// condition for both commands. A remedy that leaves either one failing is
	// the loop this finding is about.
	if err := os.Remove(repoDir); err != nil {
		t.Fatalf("carry out the printed remedy: %v", err)
	}
	if _, err := loadAt(t, worktree); err != nil {
		t.Fatalf("after the printed remedy Load() = %v, want nil", err)
	}
	if _, written, err := config.WriteIfAbsent(worktree); err != nil || !written {
		t.Fatalf("after the printed remedy WriteIfAbsent() = (written %v, err %v), want it to write", written, err)
	}
}

// TestScaffoldRemedyNamesTheDirectoryThatCarriesThePermission is the init half
// of finding D4. A worktree that refuses new entries stops `init` before
// .mindrail exists, and the remedy printed was "Check the permissions and free
// space on <repo>/.mindrail" -- a chmod on a path that is not there, which
// cannot be carried out at all. The mode that refused belongs to the worktree.
func TestScaffoldRemedyNamesTheDirectoryThatCarriesThePermission(t *testing.T) {
	requireModeEnforcement(t)

	worktree := t.TempDir()
	if err := os.Chmod(worktree, 0o500); err != nil {
		t.Fatalf("chmod the worktree read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(worktree, 0o700) })

	_, written, err := config.WriteIfAbsent(worktree)
	if err == nil || written {
		t.Fatalf("WriteIfAbsent() = (written %v, err %v), want a failure on a worktree that refuses new entries", written, err)
	}

	got := payload(t, err)
	if got.Metadata["root_kind"] != string(filesystem.RootRepository) {
		t.Errorf("metadata root_kind = %q, want %q", got.Metadata["root_kind"], filesystem.RootRepository)
	}
	if len(got.NextAction) == 0 {
		t.Fatalf("payload %+v is not actionable", got)
	}
	first := got.NextAction[0]
	if !strings.Contains(first, worktree) {
		t.Errorf("first next action = %q does not name %q, whose mode refused the create", first, worktree)
	}
	if strings.Contains(first, filepath.Join(worktree, config.RepoDir)) {
		t.Errorf("first next action = %q sends the reader to chmod a directory that does not exist", first)
	}

	// The remedy, carried out.
	if err := os.Chmod(worktree, 0o700); err != nil {
		t.Fatalf("carry out the printed remedy: %v", err)
	}
	if _, written, err := config.WriteIfAbsent(worktree); err != nil || !written {
		t.Fatalf("after the printed remedy WriteIfAbsent() = (written %v, err %v), want it to write", written, err)
	}
}

// TestRepoConfigDirObstructionDoesNotOverFire is the over-fire guard for both
// of the checks above.
//
// The condition being detected is something standing where .mindrail has to be,
// and nothing else. An ordinary initialised repository, a repository that has
// never been initialised, a checkout whose configuration is readable but whose
// directory refuses writes, and the inside-repository symlink decision D-32
// allows must all keep loading exactly as they did.
func TestRepoConfigDirObstructionDoesNotOverFire(t *testing.T) {
	t.Run("an ordinary initialised repository", func(t *testing.T) {
		worktree := t.TempDir()
		if _, _, err := config.WriteIfAbsent(worktree); err != nil {
			t.Fatalf("WriteIfAbsent: %v", err)
		}
		if _, err := config.EnsureKnowledgeDirs(worktree); err != nil {
			t.Fatalf("EnsureKnowledgeDirs: %v", err)
		}

		loaded, err := loadAt(t, worktree)
		if err != nil {
			t.Fatalf("Load() = %v, want nil in an ordinary initialised repository", err)
		}
		if want := filepath.Join(worktree, config.RepoDir, config.ConfigFileName); loaded.RepoFile != want {
			t.Errorf("RepoFile = %q, want %q", loaded.RepoFile, want)
		}
	})

	t.Run("a repository that was never initialised", func(t *testing.T) {
		worktree := t.TempDir()

		loaded, err := loadAt(t, worktree)
		if err != nil {
			t.Fatalf("Load() = %v, want nil: an absent .mindrail is the uninitialised state, not an obstruction", err)
		}
		if loaded.RepoFile != "" {
			t.Errorf("RepoFile = %q, want empty", loaded.RepoFile)
		}
	})

	t.Run("a checkout whose .mindrail refuses writes is still readable", func(t *testing.T) {
		requireModeEnforcement(t)

		worktree := t.TempDir()
		repoDir := filepath.Join(worktree, config.RepoDir)
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			t.Fatalf("create %s: %v", repoDir, err)
		}
		if err := os.WriteFile(filepath.Join(repoDir, config.ConfigFileName),
			[]byte("[project]\nname = \"read-only-checkout\"\n"), 0o644); err != nil {
			t.Fatalf("seed the repo config: %v", err)
		}
		if err := os.Chmod(repoDir, 0o500); err != nil {
			t.Fatalf("chmod %s: %v", repoDir, err)
		}
		t.Cleanup(func() { _ = os.Chmod(repoDir, 0o755) })

		loaded, err := loadAt(t, worktree)
		if err != nil {
			t.Fatalf("Load() = %v, want nil: a directory that cannot be written can still be read", err)
		}
		if loaded.Config.Project.Name != "read-only-checkout" {
			t.Errorf("project.name = %q, want the value from the read-only checkout", loaded.Config.Project.Name)
		}
	})

	t.Run("a .mindrail symlinked inside the repository", func(t *testing.T) {
		requireSymlinks(t)

		worktree := t.TempDir()
		real := filepath.Join(worktree, "vendor", "mindrail-config")
		if err := os.MkdirAll(real, 0o755); err != nil {
			t.Fatalf("create %s: %v", real, err)
		}
		if err := os.WriteFile(filepath.Join(real, config.ConfigFileName),
			[]byte("[project]\nname = \"linked\"\n"), 0o644); err != nil {
			t.Fatalf("seed the repo config: %v", err)
		}
		if err := os.Symlink(real, filepath.Join(worktree, config.RepoDir)); err != nil {
			t.Fatalf("symlink %s: %v", config.RepoDir, err)
		}

		loaded, err := loadAt(t, worktree)
		if err != nil {
			t.Fatalf("Load() = %v, want nil through the inside-root symlink D-32 allows", err)
		}
		if loaded.Config.Project.Name != "linked" {
			t.Errorf("project.name = %q, want %q", loaded.Config.Project.Name, "linked")
		}
	})
}

// payload extracts the structured payload or fails.
func payload(t *testing.T, err error) app.ErrorPayload {
	t.Helper()

	got, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no structured payload: %v", err)
	}
	if len(got.NextAction) == 0 {
		t.Fatalf("payload %+v is not actionable", got)
	}
	return got
}
