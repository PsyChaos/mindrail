package setup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/setup"
)

func fixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	common := filepath.Join(root, ".git")
	hook := filepath.Join(common, "hooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, common, hook
}

func TestManagedSectionReplacementPreservesSurroundingBytes(t *testing.T) {
	root, common, hook := fixture(t)
	path := filepath.Join(root, "AGENTS.md")
	prefix := "# Private team rules\r\nDo not change this line.\r\n\r\n"
	suffix := "\r\n\r\n## Additional instructions\r\nKeep tabs:\tvalue\r\n"
	old := prefix + setup.Begin + "\r\noutdated instructions\r\n" + setup.End + suffix
	if err := os.WriteFile(path, []byte(old), 0o640); err != nil {
		t.Fatal(err)
	}
	first, err := setup.Install(root, common, hook)
	if err != nil {
		t.Fatal(err)
	}
	if !first.AgentsChanged || !first.HookChanged {
		t.Fatalf("missing change result: %+v", first)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), prefix) || !strings.HasSuffix(string(body), suffix) || strings.Contains(string(body), "outdated instructions") {
		t.Fatalf("surrounding content changed: %q", body)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := setup.Install(root, common, hook)
	if err != nil {
		t.Fatal(err)
	}
	if second.AgentsChanged || second.HookChanged {
		t.Fatalf("repeat changed files: %+v", second)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.ModTime() != after.ModTime() || after.Mode().Perm() != 0o640 {
		t.Fatal("repeat rewrote metadata")
	}
}

func TestLegacyHookMigrationPreservesForeignBytes(t *testing.T) {
	root, common, hook := fixture(t)
	foreign := "#!/bin/sh\n# Team-owned script\nexit 0\n"
	legacy := foreign + "# mindrail:begin verify-staged\nmindrail verify --staged\n# mindrail:end verify-staged\n"
	if err := os.WriteFile(hook, []byte(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.Install(root, common, hook); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(hook + ".mindrail-original")
	if err != nil || string(body) != foreign {
		t.Fatalf("lost original: %q %v", body, err)
	}
	if _, err := setup.Install(root, common, hook); err != nil {
		t.Fatal(err)
	}
}

func TestExternalHookDirectoryAndOccupiedBackupRefuseWithoutUserEdits(t *testing.T) {
	t.Run("external", func(t *testing.T) {
		root, common, _ := fixture(t)
		outside := filepath.Join(t.TempDir(), "pre-commit")
		if err := os.WriteFile(outside, []byte("outside"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := setup.Install(root, common, outside); err == nil {
			t.Fatal("external hook accepted")
		}
		body, err := os.ReadFile(outside)
		if err != nil || string(body) != "outside" {
			t.Fatal("external hook changed")
		}
		if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
			t.Fatal("partial instruction write before failed preflight")
		}
	})
	t.Run("backup", func(t *testing.T) {
		root, common, hook := fixture(t)
		original := "#!/bin/sh\nexit 0\n"
		if err := os.WriteFile(hook, []byte(original), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(hook+".mindrail-original", []byte("other user content"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := setup.Install(root, common, hook); err == nil {
			t.Fatal("occupied backup accepted")
		}
		body, err := os.ReadFile(hook)
		if err != nil || string(body) != original {
			t.Fatal("foreign hook overwritten")
		}
	})
}

func TestManagedWrapperEditIsNotOverwritten(t *testing.T) {
	root, common, hook := fixture(t)
	if _, err := setup.Install(root, common, hook); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(hook)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("# local edit\n")...)
	if err := os.WriteFile(hook, body, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.Install(root, common, hook); err == nil {
		t.Fatal("modified managed wrapper overwritten")
	}
	after, err := os.ReadFile(hook)
	if err != nil || string(body) != string(after) {
		t.Fatal("local edits lost")
	}
}

func TestHookParentSymlinkRefused(t *testing.T) {
	root, common, _ := fixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "custom-hooks")); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.Install(root, common, filepath.Join(root, "custom-hooks", "pre-commit")); err == nil {
		t.Fatal("symlink hooks dir accepted")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside writes: %v %v", entries, err)
	}
}

func TestRepositoryLocalHookParentSymlinkRefusedWithoutWrites(t *testing.T) {
	root, common, hook := fixture(t)
	original := []byte("#!/bin/sh\n# repository-owned hook\nexit 0\n")
	if err := os.WriteFile(hook, original, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(".git", "hooks"), filepath.Join(root, "custom-hooks")); err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(root, "custom-hooks", "pre-commit")
	if _, err := setup.Install(root, common, alias); err == nil {
		t.Fatal("repository-local symlink hooks dir accepted")
	}
	body, err := os.ReadFile(hook)
	if err != nil || string(body) != string(original) {
		t.Fatalf("repository-owned hook changed: %q %v", body, err)
	}
	if _, err := os.Stat(hook + ".mindrail-original"); !os.IsNotExist(err) {
		t.Fatalf("foreign-hook backup created through symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("partial instruction write before failed preflight: %v", err)
	}
}

func TestManagedInstructionsRequireScopeBeforeEditing(t *testing.T) {
	root, common, hook := fixture(t)
	if _, err := setup.Install(root, common, hook); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"If bootstrap omitted paths", "before the first edit", "only when bootstrap paths already cover every file"} {
		if !strings.Contains(string(body), required) {
			t.Errorf("managed scope guidance missing %q: %s", required, body)
		}
	}
}

func TestExistingManagedInstructionsUpgradeJEVAndRemainStable(t *testing.T) {
	root, common, hook := fixture(t)
	agentsPath := filepath.Join(root, "AGENTS.md")
	userPrefix := "# Team rules\nKeep this exact.\n\n"
	userSuffix := "\n## Local workflow\nRun the repository tests.\n"
	oldBlock := setup.Begin + "\n## MINDRAIL PROTOCOL\n\nOld initialized guidance.\n" + setup.End
	if err := os.WriteFile(agentsPath, []byte(userPrefix+oldBlock+userSuffix), 0o640); err != nil {
		t.Fatal(err)
	}
	foreignHook := []byte("#!/bin/sh\n# user-owned hook\nexit 0\n")
	if err := os.WriteFile(hook, foreignHook, 0o755); err != nil {
		t.Fatal(err)
	}

	first, err := setup.Install(root, common, hook)
	if err != nil {
		t.Fatal(err)
	}
	if !first.AgentsChanged || !first.HookChanged {
		t.Fatalf("upgrade did not change managed files: %+v", first)
	}
	agentsBody, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(agentsBody)
	for _, required := range []string{
		"mindrail agent route", "TYPESAFE_API_KEY", "top-level status is `ok`",
		"mindrail jev connect", "operating-system keyring", "optional CI/container override",
		"never raw secrets, logs, or source code",
		"ambiguous tool, agent, model, or reasoning-effort choice",
		"required `goal` string", "optional", "`context` JSON", "one or more",
		"`tools`, `agents`, `models`, or `efforts`", "exactly `id` and `description`",
		`{"goal":"select a search tool","tools":[{"id":"rg","description":"search repository text"}]}`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("upgraded guidance missing %q", required)
		}
	}
	for _, forbidden := range []string{"environment that launches the agent", "Use it only when"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("upgraded guidance retained client-specific activation text %q", forbidden)
		}
	}
	if !strings.HasPrefix(text, userPrefix) || !strings.HasSuffix(text, userSuffix) || strings.Contains(text, "Old initialized guidance") {
		t.Fatalf("user content changed during upgrade: %q", text)
	}
	backup, err := os.ReadFile(hook + ".mindrail-original")
	if err != nil || string(backup) != string(foreignHook) {
		t.Fatalf("foreign hook was not preserved: %q %v", backup, err)
	}
	firstHook, err := os.ReadFile(hook)
	if err != nil {
		t.Fatal(err)
	}
	agentsInfo, err := os.Stat(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	hookInfo, err := os.Stat(hook)
	if err != nil {
		t.Fatal(err)
	}

	second, err := setup.Install(root, common, hook)
	if err != nil {
		t.Fatal(err)
	}
	if second.AgentsChanged || second.HookChanged {
		t.Fatalf("repeat install changed managed files: %+v", second)
	}
	secondAgents, _ := os.ReadFile(agentsPath)
	secondHook, _ := os.ReadFile(hook)
	secondAgentsInfo, _ := os.Stat(agentsPath)
	secondHookInfo, _ := os.Stat(hook)
	if string(secondAgents) != string(agentsBody) || string(secondHook) != string(firstHook) {
		t.Fatal("repeat install changed managed bytes")
	}
	if secondAgentsInfo.ModTime() != agentsInfo.ModTime() || secondHookInfo.ModTime() != hookInfo.ModTime() {
		t.Fatal("repeat install changed managed mtimes")
	}
}
