package config_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
)

// layerColors gives every overridable layer a value the assertion can tell
// apart from the default. Only three colours are legal, so the test pins the
// winning source as well as the winning value.
const (
	defaultColor = "auto"
	userColor    = "always"
	repoColor    = "never"
	envColor     = "always"
	flagColor    = "never"
)

// TestPrecedenceAcrossFiveLayers walks every combination of the four
// overridable layers and asserts the tech-stack §37 order:
// flag > env > repository file > user file > defaults.
func TestPrecedenceAcrossFiveLayers(t *testing.T) {
	type layer struct {
		name   string
		color  string
		source config.Source
	}

	// Ordered lowest precedence first; the last enabled layer must win.
	layers := []layer{
		{name: "user", color: userColor, source: config.SourceUser},
		{name: "repo", color: repoColor, source: config.SourceRepo},
		{name: "env", color: envColor, source: config.SourceEnv},
		{name: "flag", color: flagColor, source: config.SourceFlag},
	}

	for mask := 0; mask < 1<<len(layers); mask++ {
		enabled := make([]layer, 0, len(layers))
		names := make([]string, 0, len(layers))
		for i, l := range layers {
			if mask&(1<<i) != 0 {
				enabled = append(enabled, l)
				names = append(names, l.name)
			}
		}

		caseName := "defaults-only"
		if len(names) > 0 {
			caseName = strings.Join(names, "+")
		}

		t.Run(caseName, func(t *testing.T) {
			worktree := t.TempDir()
			userDir := t.TempDir()

			opts := config.LoaderOptions{
				WorktreeRoot:  worktree,
				UserConfigDir: userDir,
				Environ:       []string{},
			}

			for _, l := range enabled {
				switch l.source {
				case config.SourceUser:
					writeUserConfig(t, userDir, "[output]\ncolor = \""+l.color+"\"\n")
				case config.SourceRepo:
					writeRepoConfig(t, worktree, "[output]\ncolor = \""+l.color+"\"\n")
				case config.SourceEnv:
					opts.Environ = append(opts.Environ, "MINDRAIL_OUTPUT_COLOR="+l.color)
				case config.SourceFlag:
					opts.Flags = map[string]string{config.KeyOutputColor: l.color}
				}
			}

			wantColor, wantSource := defaultColor, config.SourceDefault
			if len(enabled) > 0 {
				winner := enabled[len(enabled)-1]
				wantColor, wantSource = winner.color, winner.source
			}

			loaded, err := config.NewLoader(opts).Load()
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}
			if got := loaded.Config.Output.Color; got != wantColor {
				t.Errorf("output.color = %q, want %q", got, wantColor)
			}
			if got := loaded.Provenance[config.KeyOutputColor]; got != wantSource {
				t.Errorf("provenance[output.color] = %q, want %q", got, wantSource)
			}
		})
	}
}

// TestProvenanceReportsSourceLayer proves every key reports its own winning
// layer independently: tech-stack §37 wants "why is this setting active?"
// answerable per setting, not per file.
func TestProvenanceReportsSourceLayer(t *testing.T) {
	worktree := t.TempDir()
	userDir := t.TempDir()

	writeUserConfig(t, userDir, "[project]\nname = \"from-user\"\n[output]\ncolor = \"always\"\n")
	writeRepoConfig(t, worktree, "[project]\nname = \"from-repo\"\n")

	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  worktree,
		UserConfigDir: userDir,
		Environ:       []string{"MINDRAIL_RUNTIME_DIR=/tmp/injected-runtime"},
		Flags:         map[string]string{config.KeyOutputColor: "never"},
	}).Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	want := config.Provenance{
		config.KeyProjectName:     config.SourceRepo,
		config.KeyOutputColor:     config.SourceFlag,
		config.KeyRuntimeDir:      config.SourceEnv,
		config.KeyRuntimeCacheDir: config.SourceDefault,
	}
	if !maps.Equal(loaded.Provenance, want) {
		t.Errorf("Provenance = %v, want %v", loaded.Provenance, want)
	}

	if got := loaded.Config.Project.Name; got != "from-repo" {
		t.Errorf("project.name = %q, want %q (repository outranks user, §37)", got, "from-repo")
	}
	if got := loaded.Config.Output.Color; got != "never" {
		t.Errorf("output.color = %q, want %q", got, "never")
	}
	if got := loaded.Config.Runtime.Dir; got != "/tmp/injected-runtime" {
		t.Errorf("runtime.dir = %q, want %q", got, "/tmp/injected-runtime")
	}

	wantRepo := filepath.Join(worktree, config.RepoDir, config.ConfigFileName)
	if loaded.RepoFile != wantRepo {
		t.Errorf("RepoFile = %q, want %q", loaded.RepoFile, wantRepo)
	}
	wantUser := filepath.Join(userDir, "mindrail", config.ConfigFileName)
	if loaded.UserFile != wantUser {
		t.Errorf("UserFile = %q, want %q", loaded.UserFile, wantUser)
	}
}

// TestStrictDecodeRejectsUnknownKey pins decision D-03: a configuration file
// this binary does not fully understand is a usage error (exit 2), never a
// silently ignored key. A silently dropped safety setting is the failure this
// guards against (tech-stack §37).
func TestStrictDecodeRejectsUnknownKey(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		inRepo   bool
		fragment string
	}{
		{
			name:     "unknown top-level key",
			toml:     "unknown_key = 1\n",
			inRepo:   true,
			fragment: "unknown_key",
		},
		{
			name:     "unknown key inside a known table",
			toml:     "[output]\ncolor = \"auto\"\ntheme = \"dark\"\n",
			inRepo:   true,
			fragment: "theme",
		},
		{
			name:     "misspelled table",
			toml:     "[outputs]\ncolor = \"auto\"\n",
			inRepo:   true,
			fragment: "outputs",
		},
		{
			// Named without the offending word so the assertion below cannot
			// pass on the t.TempDir() path, which embeds the subtest name.
			name:     "table reserved for env and flag layers",
			toml:     "[runtime]\ndir = \"/somewhere\"\n",
			inRepo:   true,
			fragment: "runtime",
		},
		{
			name:     "user file is strict too",
			toml:     "[project]\nnmae = \"typo\"\n",
			inRepo:   false,
			fragment: "nmae",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worktree := t.TempDir()
			userDir := t.TempDir()
			if tt.inRepo {
				writeRepoConfig(t, worktree, tt.toml)
			} else {
				writeUserConfig(t, userDir, tt.toml)
			}

			_, err := config.NewLoader(config.LoaderOptions{
				WorktreeRoot:  worktree,
				UserConfigDir: userDir,
				Environ:       []string{},
			}).Load()
			if err == nil {
				t.Fatal("Load() error = nil, want a configuration error")
			}
			if !errors.Is(err, config.ErrUnknownKey) {
				t.Errorf("errors.Is(err, ErrUnknownKey) = false, err = %v", err)
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("error carries no domain payload: %v", err)
			}
			if payload.Code != app.CodeConfigInvalid {
				t.Errorf("code = %q, want %q", payload.Code, app.CodeConfigInvalid)
			}
			if len(payload.NextAction) == 0 {
				t.Error("next_action is empty; spec §84 promises a remedy")
			}
			if got := app.ExitCode(err); got != app.ExitUsage {
				t.Errorf("ExitCode = %d, want %d (D-03)", got, app.ExitUsage)
			}
			if !strings.Contains(err.Error()+payload.Why, tt.fragment) {
				t.Errorf("error does not name the offending key %q: %v / %q", tt.fragment, err, payload.Why)
			}
		})
	}
}

// TestMalformedTOMLIsAUsageError separates "syntactically broken" from
// "unknown key": both are exit 2, but only the second is ErrUnknownKey.
func TestMalformedTOMLIsAUsageError(t *testing.T) {
	worktree := t.TempDir()
	writeRepoConfig(t, worktree, "[output\ncolor = \"auto\"\n")

	_, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  worktree,
		UserConfigDir: t.TempDir(),
		Environ:       []string{},
	}).Load()
	if err == nil {
		t.Fatal("Load() error = nil, want a parse error")
	}
	if got := app.ExitCode(err); got != app.ExitUsage {
		t.Errorf("ExitCode = %d, want %d (D-03)", got, app.ExitUsage)
	}
	payload, ok := app.PayloadOf(err)
	if !ok || payload.Code != app.CodeConfigInvalid {
		t.Errorf("payload = %+v (ok=%v), want code %q", payload, ok, app.CodeConfigInvalid)
	}
}

// TestInvalidColorValueIsRejected proves Validate runs on the merged result,
// so an illegal value cannot enter through the highest layer unchecked.
func TestInvalidColorValueIsRejected(t *testing.T) {
	tests := []struct {
		name string
		opts config.LoaderOptions
	}{
		{
			name: "from environment",
			opts: config.LoaderOptions{Environ: []string{"MINDRAIL_OUTPUT_COLOR=rainbow"}},
		},
		{
			name: "from flag",
			opts: config.LoaderOptions{Environ: []string{}, Flags: map[string]string{config.KeyOutputColor: "rainbow"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			opts.WorktreeRoot = t.TempDir()
			opts.UserConfigDir = t.TempDir()

			if _, err := config.NewLoader(opts).Load(); err == nil {
				t.Fatal("Load() error = nil, want an invalid-value error")
			} else if got := app.ExitCode(err); got != app.ExitUsage {
				t.Errorf("ExitCode = %d, want %d", got, app.ExitUsage)
			}
		})
	}
}

// TestUnknownFlagKeyIsRejected keeps the flag layer honest: a dotted key the
// binary does not own is a wiring defect, and swallowing it would make the
// highest-precedence layer the least trustworthy.
func TestUnknownFlagKeyIsRejected(t *testing.T) {
	_, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  t.TempDir(),
		UserConfigDir: t.TempDir(),
		Environ:       []string{},
		Flags:         map[string]string{"output.theme": "dark"},
	}).Load()
	if err == nil {
		t.Fatal("Load() error = nil, want an unknown-key error")
	}
	if !errors.Is(err, config.ErrUnknownKey) {
		t.Errorf("errors.Is(err, ErrUnknownKey) = false, err = %v", err)
	}
	if got := app.ExitCode(err); got != app.ExitUsage {
		t.Errorf("ExitCode = %d, want %d", got, app.ExitUsage)
	}
}

// TestLoadWithNoFilesUsesDefaults is the clean-clone case: a repository that
// has never run init must load without error.
func TestLoadWithNoFilesUsesDefaults(t *testing.T) {
	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  t.TempDir(),
		UserConfigDir: t.TempDir(),
		Environ:       []string{},
	}).Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if loaded.Config != config.Defaults() {
		t.Errorf("Config = %+v, want %+v", loaded.Config, config.Defaults())
	}
	if loaded.RepoFile != "" || loaded.UserFile != "" {
		t.Errorf("RepoFile = %q, UserFile = %q, want both empty", loaded.RepoFile, loaded.UserFile)
	}
	if len(loaded.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", loaded.Warnings)
	}
}

// TestEmbeddedTemplateLoadsStrictly closes the loop between the two halves of
// this package: the file init writes must survive the loader init later runs.
func TestEmbeddedTemplateLoadsStrictly(t *testing.T) {
	worktree := t.TempDir()
	if _, _, err := config.WriteIfAbsent(worktree); err != nil {
		t.Fatalf("WriteIfAbsent() error = %v", err)
	}

	loaded, err := config.NewLoader(config.LoaderOptions{
		WorktreeRoot:  worktree,
		UserConfigDir: t.TempDir(),
		Environ:       []string{},
	}).Load()
	if err != nil {
		t.Fatalf("Load() after WriteIfAbsent error = %v, want nil", err)
	}

	// The scaffold must not pin values: a repository file that sets output.color
	// would permanently outrank the user's machine-local preference (§37).
	for key, source := range loaded.Provenance {
		if source != config.SourceDefault {
			t.Errorf("provenance[%s] = %q after scaffolding, want %q", key, source, config.SourceDefault)
		}
	}
}

func writeRepoConfig(t *testing.T, worktreeRoot, contents string) {
	t.Helper()
	dir := filepath.Join(worktreeRoot, config.RepoDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	writeTestFile(t, filepath.Join(dir, config.ConfigFileName), contents)
}

func writeUserConfig(t *testing.T, userConfigDir, contents string) {
	t.Helper()
	dir := filepath.Join(userConfigDir, "mindrail")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	writeTestFile(t, filepath.Join(dir, config.ConfigFileName), contents)
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
