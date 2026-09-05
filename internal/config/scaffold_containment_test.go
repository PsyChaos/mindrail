package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

// TestScaffoldRefusesToWriteThroughAnEscapingSymlink is spec §113 and decision
// D-32 applied where they have to be applied: before the bytes exist.
//
// The scaffolding used to be laid down with filepath.Join and bare os calls, so
// a .mindrail (or .mindrail/knowledge) symlink pointing out of the worktree took
// the entire scaffold with it. Nothing noticed until the knowledge reader ran
// one step later, by which time config.toml, two directories and two .gitkeep
// files were already sitting outside the repository.
func TestScaffoldRefusesToWriteThroughAnEscapingSymlink(t *testing.T) {
	requireSymlinks(t)

	cases := map[string]struct {
		// link is the repo-relative path replaced by a symlink to outside.
		link string
	}{
		"the whole .mindrail directory": {link: config.RepoDir},
		"only the knowledge directory":  {link: filepath.Join(config.RepoDir, "knowledge")},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			worktree := filepath.Join(base, "worktree")
			outside := filepath.Join(base, "outside")
			for _, dir := range []string{worktree, outside} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("create %s: %v", dir, err)
				}
			}

			link := filepath.Join(worktree, tc.link)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatalf("create parent of %s: %v", link, err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatalf("symlink %s -> %s: %v", link, outside, err)
			}

			// Both entry points are exercised: init calls them in sequence and
			// either one on its own is enough to land files outside the repo.
			_, written, writeErr := config.WriteIfAbsent(worktree)
			created, dirsErr := config.EnsureKnowledgeDirs(worktree)

			if len(created) != 0 {
				t.Errorf("EnsureKnowledgeDirs created = %v, want none", created)
			}

			// The config file only escapes when .mindrail itself is the link;
			// with only knowledge/ linked it is written legitimately inside the
			// worktree, so that entry point is allowed to succeed there.
			if tc.link == config.RepoDir {
				requireEscape(t, "WriteIfAbsent", writeErr)
				if written {
					t.Error("WriteIfAbsent reported a write through an escaping symlink")
				}
			} else if writeErr != nil {
				t.Errorf("WriteIfAbsent() error = %v, want nil: the config file is inside the repository", writeErr)
			}
			requireEscape(t, "EnsureKnowledgeDirs", dirsErr)

			requireEmptyTree(t, outside)
		})
	}
}

// TestScaffoldAllowsSymlinkInsideTheRepository is the over-fire guard for the
// test above. D-32 permits inside-root symlinks on purpose — the macOS
// /tmp -> /private/tmp fixture path depends on it — so a .mindrail that is a
// link to another directory *in the same worktree* has to keep working.
func TestScaffoldAllowsSymlinkInsideTheRepository(t *testing.T) {
	requireSymlinks(t)

	worktree := t.TempDir()
	real := filepath.Join(worktree, "vendor", "mindrail-config")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("create %s: %v", real, err)
	}
	if err := os.Symlink(real, filepath.Join(worktree, config.RepoDir)); err != nil {
		t.Fatalf("symlink %s: %v", config.RepoDir, err)
	}

	path, written, err := config.WriteIfAbsent(worktree)
	if err != nil {
		t.Fatalf("WriteIfAbsent() error = %v, want nil through an inside-root symlink", err)
	}
	if !written {
		t.Fatal("written = false, want true")
	}
	if want := filepath.Join(worktree, config.RepoDir, config.ConfigFileName); path != want {
		t.Errorf("path = %q, want %q: the reported spelling must stay the caller's", path, want)
	}

	created, err := config.EnsureKnowledgeDirs(worktree)
	if err != nil {
		t.Fatalf("EnsureKnowledgeDirs() error = %v, want nil through an inside-root symlink", err)
	}
	if len(created) != 2 {
		t.Fatalf("created = %v, want both knowledge directories", created)
	}

	// The bytes have to be at the link's real target, not merely reachable.
	for _, rel := range []string{
		config.ConfigFileName,
		filepath.Join("knowledge", "decisions", ".gitkeep"),
		filepath.Join("knowledge", "invariants", ".gitkeep"),
	} {
		if _, statErr := os.Lstat(filepath.Join(real, rel)); statErr != nil {
			t.Errorf("stat %s under the link target: %v", rel, statErr)
		}
	}
}

// TestScaffoldKeepsRepositoryModes is the other over-fire guard: routing the
// writes through filesystem.Root must not hand repository content the 0700/0600
// of the machine-local runtime tree. .mindrail/ is committed and read by every
// contributor and by CI.
func TestScaffoldKeepsRepositoryModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}

	worktree := t.TempDir()
	// A umask stricter than 022 would mask the bits this test is about, and the
	// process umask is not something a test may safely change while other tests
	// run in parallel. Compare against what the umask actually allows.
	if _, _, err := config.WriteIfAbsent(worktree); err != nil {
		t.Fatalf("WriteIfAbsent() error = %v", err)
	}
	if _, err := config.EnsureKnowledgeDirs(worktree); err != nil {
		t.Fatalf("EnsureKnowledgeDirs() error = %v", err)
	}

	dirs := []string{
		config.RepoDir,
		filepath.Join(config.RepoDir, "knowledge"),
		filepath.Join(config.RepoDir, "knowledge", "decisions"),
		filepath.Join(config.RepoDir, "knowledge", "invariants"),
	}
	for _, rel := range dirs {
		info, err := os.Stat(filepath.Join(worktree, rel))
		if err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		// The group and other read/execute bits are the point: 0700 would make a
		// checked-out repository unreadable to anything but the user who ran init.
		if info.Mode().Perm()&0o055 == 0 {
			t.Errorf("%s mode = %v, want repository modes (group/other readable), not the private 0700", rel, info.Mode().Perm())
		}
	}

	files := []string{
		filepath.Join(config.RepoDir, config.ConfigFileName),
		filepath.Join(config.RepoDir, "knowledge", "decisions", ".gitkeep"),
		filepath.Join(config.RepoDir, "knowledge", "invariants", ".gitkeep"),
	}
	for _, rel := range files {
		info, err := os.Stat(filepath.Join(worktree, rel))
		if err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		if info.Mode().Perm()&0o044 == 0 {
			t.Errorf("%s mode = %v, want repository modes (group/other readable), not the private 0600", rel, info.Mode().Perm())
		}
	}
}

// TestScaffoldFailureNamesTheRepositoryConfigRoot is the H10 half: a write
// failure under .mindrail/ is not a runtime-path failure. It used to be coded as
// one and to carry no root_kind at all, so a consumer grading by root kind could
// not classify it.
func TestScaffoldFailureNamesTheRepositoryConfigRoot(t *testing.T) {
	requireModeEnforcement(t)

	worktree := t.TempDir()
	repoDir := filepath.Join(worktree, config.RepoDir)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("create %s: %v", repoDir, err)
	}
	if err := os.Chmod(repoDir, 0o500); err != nil {
		t.Fatalf("chmod %s: %v", repoDir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(repoDir, 0o755) })

	_, written, err := config.WriteIfAbsent(worktree)
	if err == nil || written {
		t.Fatalf("WriteIfAbsent() = (written %v, err %v), want a failure on an unwritable .mindrail", written, err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error %v carries no structured payload", err)
	}
	if payload.Metadata["root_kind"] != string(filesystem.RootRepository) {
		t.Errorf("metadata root_kind = %q, want %q",
			payload.Metadata["root_kind"], filesystem.RootRepository)
	}

	kind, known := filesystem.RootKindOf(err)
	if !known {
		t.Fatal("RootKindOf did not classify a .mindrail write failure")
	}
	if kind != filesystem.RootRepository {
		t.Errorf("RootKindOf = %q, want %q", kind, filesystem.RootRepository)
	}

	// The adjacent condition this must not be mistaken for: bootstrap downgrades
	// a cache-root failure to a warning and carries on. A repository whose
	// scaffolding cannot be written is not that, and misfiling it there would
	// let `init` report success on a repository it never wrote to.
	if kind == filesystem.RootCache || kind == filesystem.RootRuntime {
		t.Errorf("RootKindOf = %q; the repository config directory is neither of the machine-local roots", kind)
	}
}

// requireEscape asserts an error is the containment refusal, not some other
// failure that happens to be non-nil.
func requireEscape(t *testing.T, label string, err error) {
	t.Helper()

	if !errors.Is(err, filesystem.ErrEscapesRoot) {
		t.Fatalf("%s() error = %v, want ErrEscapesRoot", label, err)
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("%s() error carries no structured payload", label)
	}
	if payload.Code != app.CodePathEscapesRoot {
		t.Errorf("%s() code = %q, want %q", label, payload.Code, app.CodePathEscapesRoot)
	}
}

// requireEmptyTree fails when anything at all was created under dir.
func requireEmptyTree(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("%s contains %v; the scaffold was written outside the repository", dir, names)
	}
}

// requireSymlinks skips where creating a symlink needs a privilege the runner
// may not hold.
func requireSymlinks(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("symbolic links require elevated privileges on Windows")
	}
}

// requireModeEnforcement skips where permission bits do not stop a write: root
// ignores them, and so does Windows.
func requireModeEnforcement(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test depends on")
	}
}
