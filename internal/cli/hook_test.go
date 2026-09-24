package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// hookPath returns the pre-commit path for a repo.
func hookPath(t *testing.T, repo string) string {
	t.Helper()
	return filepath.Join(repo, ".git", "hooks", "pre-commit")
}

func readHook(t *testing.T, repo string) string {
	t.Helper()
	raw, err := os.ReadFile(hookPath(t, repo))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestHookInstallCreatesMissing is TASK-02 AC-02.1: no hook file grows one
// with shebang, block and executable bit.
func TestHookInstallCreatesMissing(t *testing.T) {
	repo := newInitializedRepo(t)
	path := hookPath(t, repo)
	os.Remove(path)

	got := run(t, repo, "hook", "install", "--json")
	got.requireExit(t, app.ExitSuccess)
	content := readHook(t, repo)
	if !strings.HasPrefix(content, "#!/bin/sh\n") {
		t.Fatalf("hook = %q, want shebang", content)
	}
	if !strings.Contains(content, "# mindrail:begin verify-staged") ||
		!strings.Contains(content, "mindrail verify --staged") ||
		!strings.Contains(content, "# mindrail:end verify-staged") {
		t.Fatalf("hook = %q, want guarded block", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatal("hook not executable")
	}
}

// TestHookInstallAppendsForeign Untouched is TASK-02 AC-02.1 + AC-02.2:
// foreign content survives byte-identical around the appended block, and a
// second install changes nothing.
func TestHookInstallAppendsForeignUntouched(t *testing.T) {
	repo := newInitializedRepo(t)
	path := hookPath(t, repo)
	foreign := "#!/bin/sh\n# team lint gate\nexit 0\n"
	if err := os.WriteFile(path, []byte(foreign), 0o755); err != nil {
		t.Fatal(err)
	}

	first := run(t, repo, "hook", "install", "--json")
	first.requireExit(t, app.ExitSuccess)
	afterFirst := readHook(t, repo)
	if !strings.HasPrefix(afterFirst, foreign) {
		t.Fatalf("foreign rewritten:\n%s", afterFirst)
	}
	if !strings.Contains(afterFirst, "# mindrail:begin verify-staged") {
		t.Fatalf("block missing:\n%s", afterFirst)
	}

	second := run(t, repo, "hook", "install", "--json")
	second.requireExit(t, app.ExitSuccess)
	if afterSecond := readHook(t, repo); afterSecond != afterFirst {
		t.Fatalf("second install changed the file:\n%s", afterSecond)
	}
	// Foreign content without a trailing newline still joins cleanly: no
	// line is glued to the marker.
	noNewline := "#!/bin/sh\n# team gate"
	if err := os.WriteFile(path, []byte(noNewline), 0o755); err != nil {
		t.Fatal(err)
	}
	third := run(t, repo, "hook", "install", "--json")
	third.requireExit(t, app.ExitSuccess)
	joined := readHook(t, repo)
	if !strings.Contains(joined, "# team gate\n# mindrail:begin verify-staged") {
		t.Fatalf("glued lines:\n%s", joined)
	}
}

// TestHookBlockExecutesVerify is TASK-02 AC-02.4: running the installed
// block dispatches `verify --staged` and propagates its exit. A fixture
// binary stands in for mindrail on PATH — the product never shells; the
// hook is a shell script by definition, so executing it is the only proof
// and the fixture is test-only.
func TestHookBlockExecutesVerify(t *testing.T) {
	repo := newInitializedRepo(t)
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"CALLED: $@\"\nexit 3\n"
	if err := os.WriteFile(filepath.Join(bin, "mindrail"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	got := run(t, repo, "hook", "install", "--json")
	got.requireExit(t, app.ExitSuccess)

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := exec.Command("sh", hookPath(t, repo)).CombinedOutput()
	if err == nil {
		t.Fatalf("block swallowed exit 3: %s", out)
	}
	if !strings.Contains(string(out), "CALLED: verify --staged") {
		t.Fatalf("block called: %s", out)
	}
}
