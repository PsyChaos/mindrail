package config

import (
	"os"
	"slices"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

// envKeys maps a MINDRAIL_ variable onto the dotted key it sets.
//
// The mapping is an explicit table rather than a mechanical transformation of
// the key name: decision D-07 names the cache override MINDRAIL_CACHE_DIR, not
// MINDRAIL_RUNTIME_CACHE_DIR, and a derived mapping would quietly rename a
// documented variable the day a key moves between tables.
var envKeys = map[string]string{
	EnvPrefix + "PROJECT_NAME": KeyProjectName,
	EnvPrefix + "OUTPUT_COLOR": KeyOutputColor,
	EnvPrefix + "RUNTIME_DIR":  KeyRuntimeDir,
	EnvPrefix + "CACHE_DIR":    KeyRuntimeCacheDir,
}

// applyEnv folds the MINDRAIL_ namespace of environ into cfg.
//
// An unknown variable warns instead of failing — the deliberate opposite of an
// unknown key in config.toml (tech-stack §37). A stale export in a shell
// profile or a CI job should not make the tool unusable, whereas a repository
// file is reviewed, committed and shared, so a typo there is worth stopping
// for.
func applyEnv(environ []string, cfg *Config, provenance Provenance) []app.Warning {
	if environ == nil {
		environ = os.Environ()
	}

	var unknown []string
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, EnvPrefix) {
			continue
		}

		key, known := envKeys[name]
		if !known {
			unknown = append(unknown, name)
			continue
		}

		setKey(cfg, key, value)
		provenance[key] = SourceEnv
	}

	if len(unknown) == 0 {
		return nil
	}

	// os.Environ() has no defined order, and structured output has to be
	// reproducible for the CLI contract goldens.
	slices.Sort(unknown)
	unknown = slices.Compact(unknown)

	warnings := make([]app.Warning, 0, len(unknown))
	for _, name := range unknown {
		warnings = append(warnings, app.Warning{
			Code: app.CodeConfigUnknownEnvVar,
			// The value is withheld on purpose: an unknown MINDRAIL_ variable
			// may hold anything, and spec §19 forbids dumping environment
			// values into diagnostics.
			Message: name + " is not a Mindrail configuration variable and was ignored",
		})
	}
	return warnings
}
