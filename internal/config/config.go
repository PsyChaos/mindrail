// Package config resolves Mindrail's effective configuration from the five
// layers of tech-stack §37 and records where each value came from.
//
// Two rules shape everything here. Decoding is strict, because a safety
// setting that is silently ignored because of a typo is worse than a refusal
// to start (§37). Provenance is tracked per key rather than per file, because
// "why is this setting active?" has to be answerable without reading five
// files.
package config

import (
	"sort"
	"strconv"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

const (
	// RepoDir is the repository-owned configuration directory (spec §8).
	RepoDir = ".mindrail"
	// ConfigFileName is used for both the repository and the user layer, so a
	// user can copy one to the other without renaming.
	ConfigFileName = "config.toml"
	// EnvPrefix namespaces every configuration variable (tech-stack §37).
	EnvPrefix = "MINDRAIL_"
)

// productDir is the single directory name Mindrail owns under a user
// directory. Tech-stack §112 requires one product name everywhere, with no
// alternative prefix.
const productDir = "mindrail"

// Dotted configuration keys. They are the vocabulary of Provenance, of the
// Flags map and of every diagnostic that has to name a setting, so they exist
// once rather than as string literals in three packages.
const (
	KeyProjectName     = "project.name"
	KeyOutputColor     = "output.color"
	KeyRuntimeDir      = "runtime.dir"
	KeyRuntimeCacheDir = "runtime.cache_dir"
)

// Accepted values for output.color. "auto" defers to terminal detection; see
// app.ColorEnabled for the rule itself.
const (
	ColorAuto   = "auto"
	ColorAlways = "always"
	ColorNever  = "never"
)

// Config is the effective, fully merged configuration.
type Config struct {
	Project    ProjectConfig                `toml:"project" json:"project"`
	Output     OutputConfig                 `toml:"output"  json:"output"`
	Runtime    RuntimeConfig                `toml:"-"       json:"runtime"` // env/flag only, never from TOML
	Validation map[string]ValidationProfile `toml:"validation" json:"validation"`
	Secrets    SecretsConfig                `toml:"secrets"    json:"secrets"`
	Continuity ContinuityConfig             `toml:"continuity" json:"continuity"`
}

// ProjectConfig carries repository-scoped identity.
type ProjectConfig struct {
	Name string `toml:"name" json:"name"`
}

// OutputConfig carries presentation preferences, which tech-stack §37 lists
// among the machine-local settings the user layer is allowed to hold.
type OutputConfig struct {
	Color string `toml:"color" json:"color"` // auto | always | never
}

// ValidationProfile is one project-defined validation profile (spec §49):
// a named whitelist of commands over a declared path scope. Commands are
// argv arrays — a shell string never exists, not even in configuration.
type ValidationProfile struct {
	Type     string     `toml:"type"     json:"type"`
	Paths    []string   `toml:"paths"    json:"paths"`
	Commands [][]string `toml:"commands" json:"commands"`
}

// SecretsConfig names secret-bearing environment variables (spec §78).
// Only names are configured; values are read at redaction time and never
// stored, logged or echoed — not even in validation diagnostics.
type SecretsConfig struct {
	Env []string `toml:"env" json:"env"`
}

// ContinuityConfig controls repository policy thresholds only. Permission to
// create a successor conversation is deliberately host-local and has no field
// here, so a cloned repository cannot authorize host side effects.
type ContinuityConfig struct {
	Enabled                 bool `toml:"enabled" json:"enabled"`
	WarnUsedPercent         int  `toml:"warn_used_percent" json:"warn_used_percent"`
	HandoffUsedPercent      int  `toml:"handoff_used_percent" json:"handoff_used_percent"`
	HardUsedPercent         int  `toml:"hard_used_percent" json:"hard_used_percent"`
	ConsecutiveObservations int  `toml:"consecutive_observations" json:"consecutive_observations"`
}

// Evidence types (spec §45). Profiles declare one; anything else is refused
// at validation — no default classification is invented (decision D-164).
var evidenceTypes = map[string]bool{
	"AUTOMATED_TEST": true, "TYPECHECK": true, "BUILD": true, "LINT": true,
	"INTEGRATION_TEST": true, "RUNTIME_PROBE": true, "MANUAL_VERIFICATION": true,
	"HUMAN_APPROVAL": true, "CI_VERIFICATION": true, "EXTERNAL_SYSTEM": true,
}

// RuntimeConfig holds the test- and isolation-only root overrides of decision
// D-07. They are deliberately unreachable from TOML: a repository that pinned
// another checkout's runtime directory would corrupt it for every clone.
type RuntimeConfig struct {
	Dir      string `json:"dir"`       // MINDRAIL_RUNTIME_DIR
	CacheDir string `json:"cache_dir"` // MINDRAIL_CACHE_DIR
}

// Defaults is the bottom layer of §37's precedence chain: the values that
// apply when nothing else has an opinion.
func Defaults() Config {
	return Config{
		Output: OutputConfig{Color: ColorAuto},
		Continuity: ContinuityConfig{Enabled: true, WarnUsedPercent: 55, HandoffUsedPercent: 60,
			HardUsedPercent: 75, ConsecutiveObservations: 2},
	}
}

// Validate checks the merged result rather than any single layer, so a value
// cannot slip in through the highest-precedence layer unchecked.
func (c Config) Validate() error {
	switch c.Output.Color {
	case ColorAuto, ColorAlways, ColorNever:
	default:
		return app.NewError(
			app.CodeConfigInvalid,
			app.KindUsage,
			"output.color must be one of auto, always or never, not "+quote(c.Output.Color),
			"Mindrail cannot decide whether to emit colour, so it refuses to start rather than guess.",
			"Set output.color to auto, always or never in .mindrail/config.toml",
			"Or unset MINDRAIL_OUTPUT_COLOR",
		).WithMetadata("key", KeyOutputColor)
	}

	if err := validateProfiles(c.Validation); err != nil {
		return err
	}
	if err := validateSecretNames(c.Secrets.Env); err != nil {
		return err
	}
	if c.Continuity.WarnUsedPercent < 1 || c.Continuity.WarnUsedPercent >= c.Continuity.HandoffUsedPercent ||
		c.Continuity.HandoffUsedPercent >= c.Continuity.HardUsedPercent || c.Continuity.HardUsedPercent > 100 ||
		c.Continuity.ConsecutiveObservations < 1 || c.Continuity.ConsecutiveObservations > 20 {
		return app.NewError(app.CodeConfigInvalid, app.KindUsage,
			"continuity thresholds are invalid",
			"Context continuity requires 0 < warn < handoff < hard <= 100 and 1..20 consecutive observations.",
			"Correct the [continuity] policy in .mindrail/config.toml",
		).WithMetadata("key", "continuity")
	}

	return nil
}

// validateProfiles refuses profiles that cannot run safely before anything
// reads them: unknown types (no invented classification, decision D-164),
// pathless scopes (a snapshot of nothing binds nothing), and empty or
// blank-headed argv (a shell string never exists, so there is nothing to
// fall back to). Command contents are never echoed: argv may carry secret
// values, and diagnostics name the profile, never its words.
func validateProfiles(profiles map[string]ValidationProfile) error {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profile := profiles[name]
		if !evidenceTypes[profile.Type] {
			return app.NewError(
				app.CodeConfigInvalid,
				app.KindUsage,
				"validation profile "+quote(name)+" has unknown evidence type, not a known spec §45 type",
				"Mindrail cannot classify the evidence this profile would produce, so it refuses the profile rather than guess.",
				"Set a spec §45 type on the profile in .mindrail/config.toml",
			).WithMetadata("key", "validation."+name+".type")
		}
		if len(profile.Paths) == 0 {
			return app.NewError(
				app.CodeConfigInvalid,
				app.KindUsage,
				"validation profile "+quote(name)+" declares no paths",
				"Evidence must bind a source snapshot, and an empty scope hashes to nothing.",
				"Add the profile scope to paths in .mindrail/config.toml",
			).WithMetadata("key", "validation."+name+".paths")
		}
		if len(profile.Commands) == 0 {
			return app.NewError(
				app.CodeConfigInvalid,
				app.KindUsage,
				"validation profile "+quote(name)+" declares no commands",
				"A profile with nothing to run would record vacuous evidence.",
				"Add argv commands to the profile in .mindrail/config.toml",
			).WithMetadata("key", "validation."+name+".commands")
		}
		for i, argv := range profile.Commands {
			if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
				return app.NewError(
					app.CodeConfigInvalid,
					app.KindUsage,
					"validation profile "+quote(name)+" command "+strconv.Itoa(i)+" has no executable",
					"An empty executable cannot run without a shell to interpret it, and there is no shell.",
					"Give the command an executable argv head in .mindrail/config.toml",
				).WithMetadata("key", "validation."+name+".commands")
			}
		}
	}
	return nil
}

// validateSecretNames refuses secret env names that cannot name a variable.
// Names are echoed (a name is not a value); values are never read here, let
// alone echoed (spec §19).
func validateSecretNames(names []string) error {
	for _, name := range names {
		if !validEnvName(name) {
			return app.NewError(
				app.CodeConfigInvalid,
				app.KindUsage,
				"secrets.env names an invalid environment variable: "+quote(name),
				"Redaction cannot watch a name the process environment cannot hold.",
				"Name secret-bearing variables in [secrets] env in .mindrail/config.toml",
			).WithMetadata("key", "secrets.env")
		}
	}
	return nil
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

// quote wraps a value for a diagnostic. Echoing it back is safe only because
// output.color is a closed three-value enum; spec §19 forbids dumping the
// value of an arbitrary environment variable, which is why the unknown-var
// warning in env.go names the variable and not its contents.
func quote(s string) string { return "\"" + s + "\"" }
