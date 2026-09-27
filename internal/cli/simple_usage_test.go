package cli_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

func TestRootHelpShowsHumanCommandsAndExpertCommandsRemainCallable(t *testing.T) {
	repo := newRepo(t)
	help := run(t, repo, "--help")
	help.requireExit(t, app.ExitSuccess)
	for _, name := range []string{"init", "jev", "update", "status", "doctor", "verify", "version"} {
		if !strings.Contains(help.stdout, "  "+name+" ") {
			t.Fatalf("missing %s:\n%s", name, help.stdout)
		}
	}
	for _, name := range []string{"agent", "session", "task", "lease", "checkpoint", "hook", "knowledge", "mcp", "completion"} {
		if strings.Contains(help.stdout, "  "+name+" ") {
			t.Fatalf("expert command %s in normal help:\n%s", name, help.stdout)
		}
		got := run(t, repo, name, "--help")
		got.requireExit(t, app.ExitSuccess)
	}
}

func TestInitInstallsAgentInstructionsAndHookIdempotently(t *testing.T) {
	repo := newRepo(t)
	path := filepath.Join(repo, "AGENTS.md")
	original := "# Team rules\nKeep the public API stable.\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	first := run(t, repo, "init", "--json")
	first.requireExit(t, app.ExitSuccess)
	agents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hook := readHook(t, repo)
	if !strings.HasPrefix(string(agents), original) || !strings.Contains(string(agents), "mindrail_bootstrap") || !strings.Contains(string(agents), "run_key") || !strings.Contains(string(agents), "finalize") {
		t.Fatalf("bad instructions: %s", agents)
	}
	if !strings.Contains(hook, "mindrail verify --staged") {
		t.Fatalf("bad hook: %s", hook)
	}
	second := run(t, repo, "init", "--json")
	second.requireExit(t, app.ExitSuccess)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(agents) || readHook(t, repo) != hook {
		t.Fatal("second init rewrote content")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode changed: %v %v", info, err)
	}
}

func TestInitChainsForeignHookThatExits(t *testing.T) {
	requireSh(t)
	repo := newRepo(t)
	path := hookPath(t, repo)
	foreign := "#!/bin/sh\nprintf 'FOREIGN\\n'\nexit 0\n"
	if err := os.WriteFile(path, []byte(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	got := run(t, repo, "init", "--json")
	got.requireExit(t, app.ExitSuccess)
	backup, err := os.ReadFile(path + ".mindrail-original")
	if err != nil || string(backup) != foreign {
		t.Fatalf("foreign hook not preserved: %q %v", backup, err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "mindrail"), []byte("#!/bin/sh\necho MINDRAIL\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := exec.Command("sh", path)
	c.Dir = repo
	out, err := c.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "MINDRAIL") || !strings.Contains(string(out), "FOREIGN") {
		t.Fatalf("not chained: %s %v", out, err)
	}
	if strings.Index(string(out), "FOREIGN") > strings.Index(string(out), "MINDRAIL") {
		t.Fatalf("gate ran before foreign hook could finish editing staged files: %s", out)
	}
	if err := os.WriteFile(filepath.Join(bin, "mindrail"), []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c = exec.Command("sh", path)
	c.Dir = repo
	out, err = c.CombinedOutput()
	if err == nil {
		t.Fatalf("gate failure swallowed: %s %v", out, err)
	}
}

func TestInitRespectsRepositoryLocalHooksPathAndRefusesExternal(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(fmt.Sprint(external), func(t *testing.T) {
			repo := newRepo(t)
			dir := filepath.Join(repo, ".team-hooks")
			if external {
				dir = t.TempDir()
			}
			if out, err := exec.Command("git", "-C", repo, "config", "core.hooksPath", dir).CombinedOutput(); err != nil {
				t.Fatalf("config: %s %v", out, err)
			}
			got := run(t, repo, "init", "--json")
			if external {
				if got.code == app.ExitSuccess {
					t.Fatal("external hooks modified")
				}
				if _, err := os.Stat(filepath.Join(dir, "pre-commit")); !os.IsNotExist(err) {
					t.Fatal("external write")
				}
				return
			}
			got.requireExit(t, app.ExitSuccess)
			if _, err := os.Stat(filepath.Join(dir, "pre-commit")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInitRefusesUnsafeOrMalformedManagedTargets(t *testing.T) {
	for _, kind := range []string{"agents-symlink", "hook-symlink", "broken-markers", "duplicate-markers", "hook-directory"} {
		t.Run(kind, func(t *testing.T) {
			repo := newRepo(t)
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("outside unchanged"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "agents-symlink":
				if err := os.Symlink(outside, filepath.Join(repo, "AGENTS.md")); err != nil {
					t.Fatal(err)
				}
			case "hook-symlink":
				if err := os.Symlink(outside, hookPath(t, repo)); err != nil {
					t.Fatal(err)
				}
			case "hook-directory":
				if err := os.Mkdir(hookPath(t, repo), 0o755); err != nil {
					t.Fatal(err)
				}
			case "broken-markers":
				writeRepoFile(t, repo, "AGENTS.md", "before\n<!-- BEGIN MINDRAIL MANAGED SECTION -->\nmissing end\n")
			case "duplicate-markers":
				writeRepoFile(t, repo, "AGENTS.md", strings.Repeat("<!-- BEGIN MINDRAIL MANAGED SECTION -->\nx\n<!-- END MINDRAIL MANAGED SECTION -->\n", 2))
			}
			got := run(t, repo, "init", "--json")
			if got.code == app.ExitSuccess {
				t.Fatalf("accepted %s: %s", kind, got.stdout)
			}
			body, err := os.ReadFile(outside)
			if err != nil || string(body) != "outside unchanged" {
				t.Fatalf("outside modified: %q %v", body, err)
			}
		})
	}
}

func TestVerifyDefaultMatchesStagedAndNamesMode(t *testing.T) {
	repo := newInitializedRepo(t)
	def := run(t, repo, "verify", "--json")
	explicit := run(t, repo, "verify", "--staged", "--json")
	def.requireExit(t, app.ExitSuccess)
	explicit.requireExit(t, app.ExitSuccess)
	if def.stdout != explicit.stdout {
		t.Fatalf("default differs:\n%s\n%s", def.stdout, explicit.stdout)
	}
	human := run(t, repo, "verify")
	human.requireExit(t, app.ExitSuccess)
	if !strings.Contains(strings.ToLower(human.stdout), "staged") {
		t.Fatalf("mode not visible: %s", human.stdout)
	}
}

func TestMCPIntentDoesNotSuppressOtherCLIJSONErrors(t *testing.T) {
	repo := newRepo(t)
	for _, args := range [][]string{
		{"--unknown", "--chdir", "mcp", "status", "--json"},
		{"--unknown", "--chdir=mcp", "status", "--json"},
		{"--unknown", "-C", "mcp", "status", "--json"},
		{"--unknown", "-Cmcp", "status", "--json"},
		{"--unknown", "help", "mcp", "--json"},
		{"--unknown", "status", "mcp", "--json"},
		{"--json", "--unknown", "--", "mcp"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			got := runBare(t, repo, args...)
			got.requireExit(t, app.ExitUsage)
			if !strings.Contains(got.stdout, `"code":"COMMAND_LINE_INVALID"`) {
				t.Fatalf("ordinary CLI JSON usage envelope suppressed: %s", got.stdout)
			}
		})
	}
}
