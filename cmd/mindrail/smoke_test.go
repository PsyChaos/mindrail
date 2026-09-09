//go:build smoke

// Package main's smoke tests drive the compiled binary as a subprocess.
//
// They deliberately import nothing from internal/: the point is to prove that a
// released executable, alone in a directory it has never seen, does the right
// thing. A test that reached into the packages to shortcut any of that would be
// testing the same wiring the unit tests already cover (tech-stack §136).
package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSmokeCleanBinary is tech-stack §159's first-binary contract: init, status
// and doctor in a fresh repository, with nothing installed and nothing
// configured.
func TestSmokeCleanBinary(t *testing.T) {
	binary := buildBinary(t)
	repo := newRepo(t)
	home := t.TempDir()

	// The binary runs from a directory containing only itself, so nothing it
	// needs can be picked up from the source tree by accident.
	if dir := filepath.Dir(binary); len(entriesIn(t, dir)) != 1 {
		t.Fatalf("%s is not alone in its directory: %v", binary, entriesIn(t, dir))
	}

	for _, command := range [][]string{
		{"init"},
		{"status", "--json"},
		{"doctor", "--json"},
		{"version", "--json"},
	} {
		got := runBinary(t, binary, repo, home, command...)
		if got.code != 0 {
			t.Fatalf("%v exited %d\nstdout:\n%s\nstderr:\n%s", command, got.code, got.stdout, got.stderr)
		}

		if len(command) > 1 && command[1] == "--json" {
			envelope := decodeEnvelope(t, got.stdout)
			if envelope["command"] != command[0] {
				t.Errorf("%v envelope command = %v, want %v", command, envelope["command"], command[0])
			}
			if envelope["ok"] != true {
				t.Errorf("%v envelope ok = %v, want true\n%s", command, envelope["ok"], got.stdout)
			}
		}
	}

	dbPath := filepath.Join(repo, ".git", "mindrail", "mindrail.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("runtime database missing after init: %v", err)
	}
}

// TestSmokeLinkedWorktree is acceptance criterion 3 against the released
// executable.
//
// AC-3 is a promise about what the binary *reports*, and the in-process contract
// tests share the command tree with the packages they are asserting on. Driving
// the executable through a real `git worktree add` is the only form of the test
// that would also catch a break in the parts a test binary supplies for itself:
// the embedded assets, the discovery of the common dir from a directory whose
// .git is a file rather than a directory, and the runtime state landing under
// the shared common dir rather than beside each worktree.
func TestSmokeLinkedWorktree(t *testing.T) {
	binary := buildBinary(t)
	home := t.TempDir()
	main := newRepoWithCommit(t)
	linked := addWorktree(t, main, "feature")

	for _, root := range []string{main, linked} {
		if got := runBinary(t, binary, root, home, "init"); got.code != 0 {
			t.Fatalf("init in %s exited %d\nstdout:\n%s\nstderr:\n%s", root, got.code, got.stdout, got.stderr)
		}
	}

	layouts := map[string]map[string]any{}
	for name, root := range map[string]string{"main": main, "linked": linked} {
		got := runBinary(t, binary, root, home, "status", "--json")
		if got.code != 0 {
			t.Fatalf("status in %s exited %d\nstdout:\n%s\nstderr:\n%s", root, got.code, got.stdout, got.stderr)
		}

		data, ok := decodeEnvelope(t, got.stdout)["data"].(map[string]any)
		if !ok {
			t.Fatalf("%s: status envelope carries no data object:\n%s", name, got.stdout)
		}
		repository, ok := data["repository"].(map[string]any)
		if !ok {
			t.Fatalf("%s: status data carries no repository object:\n%s", name, got.stdout)
		}
		layouts[name] = repository
	}

	if layouts["main"]["common_dir"] != layouts["linked"]["common_dir"] {
		t.Errorf("common_dir differs between worktrees: main %v, linked %v",
			layouts["main"]["common_dir"], layouts["linked"]["common_dir"])
	}
	if layouts["main"]["worktree_root"] == layouts["linked"]["worktree_root"] {
		t.Errorf("both worktrees report worktree_root %v", layouts["main"]["worktree_root"])
	}
	if layouts["main"]["is_linked_worktree"] != false {
		t.Errorf("main worktree is_linked_worktree = %v, want false", layouts["main"]["is_linked_worktree"])
	}
	if layouts["linked"]["is_linked_worktree"] != true {
		t.Errorf("linked worktree is_linked_worktree = %v, want true", layouts["linked"]["is_linked_worktree"])
	}

	// One database for the repository, not one per worktree: the runtime store
	// lives under the common dir, which is what makes the two worktrees parts of
	// one project rather than two.
	if _, err := os.Stat(filepath.Join(main, ".git", "mindrail", "mindrail.db")); err != nil {
		t.Errorf("runtime database missing under the common dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(linked, ".git")); err != nil {
		t.Errorf("linked worktree has no .git entry: %v", err)
	}
}

// --- harness ----------------------------------------------------------------

type output struct {
	stdout string
	stderr string
	code   int
}

// buildBinary compiles the executable into a directory of its own.
func buildBinary(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go tool is not on PATH")
	}

	binDir := filepath.Join(t.TempDir(), "installed")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create %s: %v", binDir, err)
	}
	binary := filepath.Join(binDir, "mindrail")

	build := exec.Command("go", "build", "-o", binary, "./cmd/mindrail")
	build.Dir = moduleRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return binary
}

// runBinary executes the compiled binary in dir with a deliberately small
// environment: no MINDRAIL_ settings, and user directories pointed at a
// temporary home, so the result cannot depend on the machine running the suite.
func runBinary(t *testing.T, binary, dir, home string, args ...string) output {
	t.Helper()

	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = subprocessEnv(home)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := cmd.ProcessState.ExitCode()
	if err != nil && cmd.ProcessState == nil {
		t.Fatalf("run %v: %v", args, err)
	}

	return output{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func subprocessEnv(home string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"GIT_CONFIG_NOSYSTEM=1",
	}
}

// decodeEnvelope insists stdout is exactly one JSON object and nothing else,
// which is the parseability promise of decision D-15.
func decodeEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\n%s", err, stdout)
	}
	if _, err := decoder.Token(); err != io.EOF {
		t.Fatalf("stdout carries more than one JSON value:\n%s", stdout)
	}
	return envelope
}

func newRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

// newRepoWithCommit is a repository that can carry a linked worktree: `git
// worktree add` needs a commit to check out, and an empty one is enough. The
// identity is set on the repository rather than taken from the environment so
// the fixture works on a machine with no global git configuration.
func newRepoWithCommit(t *testing.T) string {
	t.Helper()

	repo := newRepo(t)
	for _, args := range [][]string{
		{"-C", repo, "config", "user.email", "smoke@example.invalid"},
		{"-C", repo, "config", "user.name", "Mindrail Smoke"},
		{"-C", repo, "commit", "--allow-empty", "--quiet", "-m", "root"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return repo
}

// addWorktree creates a linked worktree of repo and returns its root.
func addWorktree(t *testing.T, repo, name string) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), name)
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "--quiet", "-b", name, root).CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", root).Run()
	})
	return root
}

// moduleRoot walks up to the directory holding go.mod.
//
// internal/moduletree does this for the three module-wide walkers inside
// internal/, and this is not a fourth caller of it: the package comment above
// keeps this file free of internal/ imports, so the walk is spelled out here on
// purpose.
func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test working directory")
		}
		dir = parent
	}
}

func entriesIn(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// waitFor polls until check reports true or the budget runs out. Polling beats
// a fixed sleep: the test has to be fast on a quick machine and reliable on a
// loaded one.
func waitFor(t *testing.T, budget time.Duration, check func() bool) bool {
	t.Helper()

	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if check() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return check()
}
