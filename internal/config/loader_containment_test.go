package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

const outsideConfig = "[project]\nname = \"from-outside-the-repository\"\n"

// TestLoadRefusesRepoConfigThroughAnEscapingSymlink covers the reading half of
// spec §113, which the escape error itself promises: "Mindrail refuses the
// operation rather than reading or writing outside the repository".
//
// The repository layer was read with a bare os.ReadFile on a joined path, so a
// .mindrail symlink pointing out of the worktree had every command load its
// configuration from wherever the link went — and then report the in-repo path
// as the file it had loaded, so nothing in the output revealed it.
func TestLoadRefusesRepoConfigThroughAnEscapingSymlink(t *testing.T) {
	requireSymlinks(t)

	base := t.TempDir()
	worktree := filepath.Join(base, "worktree")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{worktree, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, config.ConfigFileName), []byte(outsideConfig), 0o644); err != nil {
		t.Fatalf("seed the outside config: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(worktree, config.RepoDir)); err != nil {
		t.Fatalf("symlink %s: %v", config.RepoDir, err)
	}

	_, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  worktree,
		UserConfigDir: filepath.Join(base, "no-user-config"),
		Environ:       []string{},
	}).Load()

	if !errors.Is(err, filesystem.ErrEscapesRoot) {
		t.Fatalf("Load() error = %v, want ErrEscapesRoot", err)
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("Load() error carries no structured payload: %v", err)
	}
	if payload.Code != app.CodePathEscapesRoot {
		t.Errorf("Load() code = %q, want %q", payload.Code, app.CodePathEscapesRoot)
	}
}

// TestLoadAcceptsRepoConfigThroughAnInsideSymlink is the over-fire guard. D-32
// allows inside-root symlinks, and a repository that keeps .mindrail as a link
// to another directory of its own is an ordinary layout, not an escape.
func TestLoadAcceptsRepoConfigThroughAnInsideSymlink(t *testing.T) {
	requireSymlinks(t)

	base := t.TempDir()
	worktree := filepath.Join(base, "worktree")
	real := filepath.Join(worktree, "vendor", "mindrail-config")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("create %s: %v", real, err)
	}
	if err := os.WriteFile(filepath.Join(real, config.ConfigFileName),
		[]byte("[project]\nname = \"inside\"\n"), 0o644); err != nil {
		t.Fatalf("seed the repo config: %v", err)
	}
	if err := os.Symlink(real, filepath.Join(worktree, config.RepoDir)); err != nil {
		t.Fatalf("symlink %s: %v", config.RepoDir, err)
	}

	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  worktree,
		UserConfigDir: filepath.Join(base, "no-user-config"),
		Environ:       []string{},
	}).Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil through an inside-root symlink", err)
	}
	if loaded.Config.Project.Name != "inside" {
		t.Errorf("project.name = %q, want %q: the repository layer was not applied",
			loaded.Config.Project.Name, "inside")
	}
	if want := filepath.Join(worktree, config.RepoDir, config.ConfigFileName); loaded.RepoFile != want {
		t.Errorf("RepoFile = %q, want %q", loaded.RepoFile, want)
	}
}

// TestLoadInAnOrdinaryRepositoryIsUnaffected is the second over-fire guard: the
// containment check must be invisible to a repository with no symlink anywhere
// on the path, which is every repository in practice.
func TestLoadInAnOrdinaryRepositoryIsUnaffected(t *testing.T) {
	base := t.TempDir()
	worktree := filepath.Join(base, "worktree")
	repoDir := filepath.Join(worktree, config.RepoDir)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("create %s: %v", repoDir, err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, config.ConfigFileName),
		[]byte("[project]\nname = \"ordinary\"\n"), 0o644); err != nil {
		t.Fatalf("seed the repo config: %v", err)
	}

	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  worktree,
		UserConfigDir: filepath.Join(base, "no-user-config"),
		Environ:       []string{},
	}).Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if loaded.Config.Project.Name != "ordinary" {
		t.Errorf("project.name = %q, want %q", loaded.Config.Project.Name, "ordinary")
	}

	// A worktree that does not exist at all has no boundary to canonicalize.
	// That is not the escape condition, and refusing it here would fail a
	// command over a directory the read below reports as simply absent.
	absent, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  filepath.Join(base, "never-created"),
		UserConfigDir: filepath.Join(base, "no-user-config"),
		Environ:       []string{},
	}).Load()
	if err != nil {
		t.Fatalf("Load() on a missing worktree error = %v, want nil", err)
	}
	if absent.RepoFile != "" {
		t.Errorf("RepoFile = %q, want empty when no repository file was read", absent.RepoFile)
	}
}
