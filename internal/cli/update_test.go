package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

func TestUpdateRefusesUninitializedRepositoryWithoutWrites(t *testing.T) {
	repo := newRepo(t)
	hook := hookPath(t, repo)
	beforeHook, err := os.ReadFile(hook)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	got := run(t, repo, "update", "--json")
	if got.code != app.ExitFailed {
		t.Fatalf("update exited %d, want %d: %s", got.code, app.ExitFailed, got.stdout)
	}
	if command, _ := got.decodeEnvelope(t)["command"].(string); command != "update" {
		t.Fatalf("error command identity = %q, want update: %s", command, got.stdout)
	}
	payload := got.errorPayload(t)
	if !strings.Contains(strings.Join(payload.NextAction, " "), "mindrail init") {
		t.Fatalf("next action does not direct to init: %#v", payload.NextAction)
	}
	for _, path := range []string{
		filepath.Join(repo, ".mindrail"),
		filepath.Join(repo, "AGENTS.md"),
		filepath.Join(repo, ".git", "mindrail"),
	} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("update wrote %s: %v", path, err)
		}
	}
	afterHook, err := os.ReadFile(hook)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if string(afterHook) != string(beforeHook) {
		t.Fatalf("hook changed: before %q after %q", beforeHook, afterHook)
	}
}

func TestUpdateUsesInitPipelinePreservesStateAndIsIdempotent(t *testing.T) {
	repo := newRepo(t)
	foreignHook := "#!/bin/sh\nprintf 'foreign\\n'\n"
	if err := os.WriteFile(hookPath(t, repo), []byte(foreignHook), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := run(t, repo, "init", "--json"); got.code != app.ExitSuccess {
		t.Fatalf("init exited %d: %s", got.code, got.stdout)
	}
	agentsPath := filepath.Join(repo, "AGENTS.md")
	managedAgents, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentsPath, append([]byte("# User rules\nKeep this line.\n\n"), managedAgents...), 0o644); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(repo, ".mindrail", "config.toml")
	configBefore, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	knowledgePath := filepath.Join(repo, ".mindrail", "knowledge", "decisions", "user-note.txt")
	if err := os.WriteFile(knowledgePath, []byte("preserve me\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	statusBefore := run(t, repo, "status", "--json")
	statusBefore.requireExit(t, app.ExitSuccess)
	var before struct {
		Workspace struct {
			ID        string `json:"workspace_id"`
			ProjectID string `json:"project_id"`
		} `json:"workspace"`
	}
	decodeData(t, statusBefore.stdout, &before)

	first := run(t, repo, "update", "--json")
	first.requireExit(t, app.ExitSuccess)
	var report struct {
		Command       string `json:"command"`
		ConfigCreated bool   `json:"config_created"`
	}
	decodeData(t, first.stdout, &report)
	if report.Command != "update" || report.ConfigCreated {
		t.Fatalf("update report = %#v", report)
	}
	agentsAfterFirst, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agentsAfterFirst), "# User rules\nKeep this line.") {
		t.Fatalf("update removed user AGENTS content: %s", agentsAfterFirst)
	}
	hookAfterFirst := readHook(t, repo)

	second := run(t, repo, "update", "--json")
	second.requireExit(t, app.ExitSuccess)
	agentsAfterSecond, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(agentsAfterSecond) != string(agentsAfterFirst) || readHook(t, repo) != hookAfterFirst {
		t.Fatal("repeated update changed managed setup bytes")
	}
	if configAfter, err := os.ReadFile(configPath); err != nil || string(configAfter) != string(configBefore) {
		t.Fatalf("config changed: %q %v", configAfter, err)
	}
	if note, err := os.ReadFile(knowledgePath); err != nil || string(note) != "preserve me\n" {
		t.Fatalf("knowledge changed: %q %v", note, err)
	}
	if backup, err := os.ReadFile(hookPath(t, repo) + ".mindrail-original"); err != nil || string(backup) != foreignHook {
		t.Fatalf("foreign hook changed: %q %v", backup, err)
	}

	statusAfter := run(t, repo, "status", "--json")
	statusAfter.requireExit(t, app.ExitSuccess)
	var after struct {
		Workspace struct {
			ID        string `json:"workspace_id"`
			ProjectID string `json:"project_id"`
		} `json:"workspace"`
	}
	decodeData(t, statusAfter.stdout, &after)
	if after.Workspace != before.Workspace {
		t.Fatalf("identity changed: before %#v after %#v", before.Workspace, after.Workspace)
	}

	human := run(t, repo, "update", "--no-color")
	human.requireExit(t, app.ExitSuccess)
	if !strings.Contains(human.stdout, "Mindrail Update\n") || strings.Contains(human.stdout, "Mindrail Init\n") {
		t.Fatalf("wrong human command identity: %s", human.stdout)
	}
}

func TestUpdateAppliesPendingMigrationsWithoutLosingCoordination(t *testing.T) {
	repo := newInitializedRepo(t)
	run(t, repo, "session", "open", "--json").requireExit(t, app.ExitSuccess)
	opened := run(t, repo, "task", "open", "--title", "survives update", "--json")
	opened.requireExit(t, app.ExitSuccess)
	downgradeToSchemaThree(t, repo)

	updated := run(t, repo, "update", "--json")
	updated.requireExit(t, app.ExitSuccess)
	if listed := run(t, repo, "task", "list", "--json"); listed.code != app.ExitSuccess || !strings.Contains(listed.stdout, "survives update") {
		t.Fatalf("coordination data lost: exit %d output %s", listed.code, listed.stdout)
	}
}
