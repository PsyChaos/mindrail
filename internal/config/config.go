// Package config resolves Mindrail's effective configuration from the five
// layers of tech-stack §37 and records where each value came from.
//
// Two rules shape everything here. Decoding is strict, because a safety
// setting that is silently ignored because of a typo is worse than a refusal
// to start (§37). Provenance is tracked per key rather than per file, because
// "why is this setting active?" has to be answerable without reading five
// files.
package config

import "github.com/PsyChaos/mindrail/internal/app"

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
	Project ProjectConfig `toml:"project" json:"project"`
	Output  OutputConfig  `toml:"output"  json:"output"`
	Runtime RuntimeConfig `toml:"-"       json:"runtime"` // env/flag only, never from TOML
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

	return nil
}

// quote wraps a value for a diagnostic. Echoing it back is safe only because
// output.color is a closed three-value enum; spec §19 forbids dumping the
// value of an arbitrary environment variable, which is why the unknown-var
// warning in env.go names the variable and not its contents.
func quote(s string) string { return "\"" + s + "\"" }
