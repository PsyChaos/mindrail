package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
)

// TestUnknownMindrailEnvVarWarnsNotFails pins the asymmetry tech-stack §37
// asks for: an unknown key in config.toml is an error, but an unknown
// MINDRAIL_* variable only warns. A stale variable left in a shell profile or
// a CI job must not make the tool unusable, yet it must not vanish silently
// either.
func TestUnknownMindrailEnvVarWarnsNotFails(t *testing.T) {
	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  t.TempDir(),
		UserConfigDir: t.TempDir(),
		Environ:       []string{"MINDRAIL_NOT_A_KEY=1"},
	}).Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil: an unknown env var must not fail", err)
	}

	if len(loaded.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", loaded.Warnings)
	}
	warning := loaded.Warnings[0]
	if warning.Code != app.CodeConfigUnknownEnvVar {
		t.Errorf("warning code = %q, want %q", warning.Code, app.CodeConfigUnknownEnvVar)
	}
	if !strings.Contains(warning.Message, "MINDRAIL_NOT_A_KEY") {
		t.Errorf("warning message = %q, want it to name the variable", warning.Message)
	}
	if strings.Contains(warning.Message, "=1") {
		t.Errorf("warning message = %q, must not echo the value (spec §19)", warning.Message)
	}

	if !reflect.DeepEqual(loaded.Config, config.Defaults()) {
		t.Errorf("Config = %+v, want defaults untouched", loaded.Config)
	}
}

// TestKnownMindrailEnvVarsMapToTypedFields is the mapping table itself. Every
// entry here is a promise to whoever exports the variable.
func TestKnownMindrailEnvVarsMapToTypedFields(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		want    config.Config
		wantKey string
	}{
		{
			name:    "project name",
			environ: []string{"MINDRAIL_PROJECT_NAME=checkout"},
			want:    withProjectName(config.Defaults(), "checkout"),
			wantKey: config.KeyProjectName,
		},
		{
			name:    "output colour",
			environ: []string{"MINDRAIL_OUTPUT_COLOR=never"},
			want:    withColor(config.Defaults(), "never"),
			wantKey: config.KeyOutputColor,
		},
		{
			name:    "runtime dir override",
			environ: []string{"MINDRAIL_RUNTIME_DIR=/var/tmp/mindrail-runtime"},
			want:    withRuntimeDir(config.Defaults(), "/var/tmp/mindrail-runtime"),
			wantKey: config.KeyRuntimeDir,
		},
		{
			name:    "cache dir override",
			environ: []string{"MINDRAIL_CACHE_DIR=/var/tmp/mindrail-cache"},
			want:    withCacheDir(config.Defaults(), "/var/tmp/mindrail-cache"),
			wantKey: config.KeyRuntimeCacheDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded, err := config.NewLoader(config.LoaderOptions{
				WorktreeRoot:  t.TempDir(),
				UserConfigDir: t.TempDir(),
				Environ:       tt.environ,
			}).Load()
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}
			if len(loaded.Warnings) != 0 {
				t.Errorf("Warnings = %v, want none for a known variable", loaded.Warnings)
			}
			if !reflect.DeepEqual(loaded.Config, tt.want) {
				t.Errorf("Config = %+v, want %+v", loaded.Config, tt.want)
			}
			if got := loaded.Provenance[tt.wantKey]; got != config.SourceEnv {
				t.Errorf("provenance[%s] = %q, want %q", tt.wantKey, got, config.SourceEnv)
			}
		})
	}
}

// TestForeignEnvVarsAreIgnoredEntirely keeps the warning specific to the
// MINDRAIL_ namespace: warning about every unrelated variable in the process
// environment would make the channel worthless.
func TestForeignEnvVarsAreIgnoredEntirely(t *testing.T) {
	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  t.TempDir(),
		UserConfigDir: t.TempDir(),
		Environ: []string{
			"PATH=/usr/bin",
			"HOME=/home/nobody",
			"MINDRAILNOTPREFIXED=1",
			"NO_COLOR=1",
		},
	}).Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(loaded.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", loaded.Warnings)
	}
}

// TestUnknownEnvVarWarningsAreDeterministic keeps structured output stable:
// os.Environ() has no defined order, so the warning list must be sorted.
func TestUnknownEnvVarWarningsAreDeterministic(t *testing.T) {
	environ := []string{"MINDRAIL_ZULU=1", "MINDRAIL_ALPHA=1", "MINDRAIL_MIKE=1"}

	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  t.TempDir(),
		UserConfigDir: t.TempDir(),
		Environ:       environ,
	}).Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(loaded.Warnings) != 3 {
		t.Fatalf("Warnings = %v, want three", loaded.Warnings)
	}

	previous := ""
	for _, warning := range loaded.Warnings {
		if warning.Message <= previous {
			t.Fatalf("warnings are not sorted: %q followed %q", warning.Message, previous)
		}
		previous = warning.Message
	}
}

// TestEnvVarWithoutValueIsStillSeen covers the malformed entry: an environ
// element with no "=" still occupies the MINDRAIL_ namespace, and dropping it
// silently is the behaviour §37 rules out.
func TestEnvVarWithoutValueIsStillSeen(t *testing.T) {
	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  t.TempDir(),
		UserConfigDir: t.TempDir(),
		Environ:       []string{"MINDRAIL_MYSTERY"},
	}).Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(loaded.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want one", loaded.Warnings)
	}
	if !strings.Contains(loaded.Warnings[0].Message, "MINDRAIL_MYSTERY") {
		t.Errorf("warning = %q, want it to name MINDRAIL_MYSTERY", loaded.Warnings[0].Message)
	}
}

func withProjectName(c config.Config, name string) config.Config {
	c.Project.Name = name
	return c
}

func withColor(c config.Config, color string) config.Config {
	c.Output.Color = color
	return c
}

func withRuntimeDir(c config.Config, dir string) config.Config {
	c.Runtime.Dir = dir
	return c
}

func withCacheDir(c config.Config, dir string) config.Config {
	c.Runtime.CacheDir = dir
	return c
}
