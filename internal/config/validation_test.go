package config_test

import (
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
)

func loadRepoConfig(t *testing.T, contents string) config.Config {
	t.Helper()
	worktree := t.TempDir()
	writeRepoConfig(t, worktree, contents)
	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  worktree,
		UserConfigDir: t.TempDir(),
		Environ:       []string{},
	}).Load()
	if err != nil {
		t.Fatal(err)
	}
	return loaded.Config
}

// TestValidationProfilesParse is TASK-01 AC-01.1: profiles parse type, paths
// and argv commands from TOML with strict decoding.
func TestValidationProfilesParse(t *testing.T) {
	got := loadRepoConfig(t, `
[validation.test]
type = "AUTOMATED_TEST"
paths = ["tests/"]
commands = [["pytest", "-q"], ["go", "test", "./..."]]

[secrets]
env = ["API_KEY", "DB_PASSWORD"]
`)
	profile, ok := got.Validation["test"]
	if !ok {
		t.Fatalf("profiles = %+v", got.Validation)
	}
	if profile.Type != "AUTOMATED_TEST" || len(profile.Paths) != 1 || len(profile.Commands) != 2 {
		t.Fatalf("profile = %+v", profile)
	}
	if len(got.Secrets.Env) != 2 || got.Secrets.Env[0] != "API_KEY" {
		t.Fatalf("secrets = %+v", got.Secrets)
	}
}

// TestValidationProfilesRefuse is TASK-01 AC-01.1's refusal table: unknown
// types, missing scopes, missing commands, empty argv and invalid secret
// names are refused with usage errors — and diagnostics never echo command
// contents, which may carry secret values.
func TestValidationProfilesRefuse(t *testing.T) {
	bodies := map[string]string{
		"unknown type":   "[validation.t]\ntype = \"VIBES\"\npaths = [\"x/\"]\ncommands = [[\"go\", \"test\"]]\n",
		"no paths":       "[validation.t]\ntype = \"BUILD\"\ncommands = [[\"go\", \"build\"]]\n",
		"no commands":    "[validation.t]\ntype = \"BUILD\"\npaths = [\"x/\"]\n",
		"empty argv":     "[validation.t]\ntype = \"BUILD\"\npaths = [\"x/\"]\ncommands = [[\"go\", \"build\"], []]\n",
		"blank exe":      "[validation.t]\ntype = \"BUILD\"\npaths = [\"x/\"]\ncommands = [[\"  \"]]\n",
		"bad secret":     "[secrets]\nenv = [\"API KEY\"]\n",
		"empty secret":   "[secrets]\nenv = [\"\"]\n",
		"secret command": "[validation.t]\ntype = \"BUILD\"\npaths = [\"x/\"]\ncommands = [[\"deploy\", \"s3cr3t-value\"]]\n",
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			worktree := t.TempDir()
			if name == "secret command" {
				// Valid config: proves the refusal table above is what
				// rejects, not the shape itself.
				cfg := loadRepoConfig(t, body)
				if len(cfg.Validation["t"].Commands[0]) != 2 {
					t.Fatalf("profile = %+v", cfg.Validation["t"])
				}
				return
			}
			writeRepoConfig(t, worktree, body)
			_, err := config.NewLoader(config.LoaderOptions{
				WorktreeRoot:  worktree,
				UserConfigDir: t.TempDir(),
				Environ:       []string{},
			}).Load()
			if err == nil {
				t.Fatal("Load() error = nil, want refusal")
			}
			if got := app.ExitCode(err); got != app.ExitUsage {
				t.Fatalf("ExitCode = %d, want %d", got, app.ExitUsage)
			}
			// Rejected *values* must never appear: the unknown type and any
			// argv word. Names (profile, key, env var) are safe to echo —
			// spec §19 forbids dumping values, with codebase precedent
			// naming unknown variables.
			if msg := err.Error(); strings.Contains(msg, "VIBES") ||
				strings.Contains(msg, "s3cr3t-value") {
				t.Fatalf("diagnostic echoes rejected values: %q", msg)
			}
		})
	}
}

func TestContinuityPolicyParsesAndValidatesOrdering(t *testing.T) {
	got := loadRepoConfig(t, `[continuity]
enabled = true
warn_used_percent = 50
handoff_used_percent = 62
hard_used_percent = 80
consecutive_observations = 3
`)
	if !got.Continuity.Enabled || got.Continuity.HandoffUsedPercent != 62 || got.Continuity.ConsecutiveObservations != 3 {
		t.Fatalf("continuity = %#v", got.Continuity)
	}
	worktree := t.TempDir()
	writeRepoConfig(t, worktree, `[continuity]
warn_used_percent = 70
handoff_used_percent = 60
`)
	_, err := config.NewLoader(config.LoaderOptions{WorktreeRoot: worktree, UserConfigDir: t.TempDir(), Environ: []string{}}).Load()
	if err == nil || app.ExitCode(err) != app.ExitUsage {
		t.Fatalf("invalid ordering error = %v", err)
	}
}
