package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/pelletier/go-toml/v2"

	"github.com/PsyChaos/mindrail/internal/app"
)

// Source names the layer a value came from. It is part of the wire shape of
// doctor's "effective value and its source layer" report (tech-stack §37).
type Source string

const (
	SourceDefault Source = "default"
	SourceUser    Source = "user"
	SourceRepo    Source = "repo"
	SourceEnv     Source = "env"
	SourceFlag    Source = "flag"
)

// Provenance maps a dotted configuration key to the layer that won it.
type Provenance map[string]Source

// Loaded is the result of one resolution pass: the merged configuration, where
// each value came from, anything non-fatal seen on the way, and the files that
// actually contributed.
type Loaded struct {
	Config     Config        `json:"config"`
	Provenance Provenance    `json:"provenance"`
	Warnings   []app.Warning `json:"warnings,omitempty"`
	RepoFile   string        `json:"repo_file,omitempty"`
	UserFile   string        `json:"user_file,omitempty"`
}

// LoaderOptions carries every seam the loader has. Each ambient input — the
// user config directory, the process environment — is injectable so a test
// never depends on the machine it runs on (tech-stack §131).
type LoaderOptions struct {
	WorktreeRoot  string
	UserConfigDir string            // injectable; "" => os.UserConfigDir()
	Environ       []string          // injectable; nil => os.Environ()
	Flags         map[string]string // dotted key => raw value
}

// Loader resolves configuration for one worktree.
type Loader struct {
	opts LoaderOptions
}

// NewLoader captures the options. Nothing is read until Load, so constructing
// a loader is free and cannot fail.
func NewLoader(o LoaderOptions) *Loader {
	return &Loader{opts: o}
}

// ErrUnknownKey marks a configuration key this binary does not own, wherever
// it came from. Callers match it with errors.Is rather than on message text
// (tech-stack §72).
var ErrUnknownKey = errors.New("unknown configuration key")

// Load applies the five layers of tech-stack §37 from lowest precedence to
// highest, so the last writer of a key is the winner and provenance follows
// for free.
//
// The merged result is validated once at the end rather than layer by layer: a
// legal value in one layer can be overridden by an illegal one in the next,
// and only the merged view is what the process will actually use.
func (l *Loader) Load() (Loaded, error) {
	loaded := Loaded{
		Config: Defaults(),
		Provenance: Provenance{
			KeyProjectName:     SourceDefault,
			KeyOutputColor:     SourceDefault,
			KeyRuntimeDir:      SourceDefault,
			KeyRuntimeCacheDir: SourceDefault,
		},
	}

	if userFile := l.userConfigFile(); userFile != "" {
		present, err := applyFile(userFile, &loaded.Config, loaded.Provenance, SourceUser)
		if err != nil {
			return Loaded{}, err
		}
		if present {
			loaded.UserFile = userFile
		}
	}

	if repoFile := l.repoConfigFile(); repoFile != "" {
		present, err := applyFile(repoFile, &loaded.Config, loaded.Provenance, SourceRepo)
		if err != nil {
			return Loaded{}, err
		}
		if present {
			loaded.RepoFile = repoFile
		}
	}

	loaded.Warnings = applyEnv(l.opts.Environ, &loaded.Config, loaded.Provenance)

	if err := applyFlags(l.opts.Flags, &loaded.Config, loaded.Provenance); err != nil {
		return Loaded{}, err
	}

	if err := loaded.Config.Validate(); err != nil {
		return Loaded{}, err
	}

	return loaded, nil
}

// repoConfigFile is <worktreeRoot>/.mindrail/config.toml, or "" when the
// caller has no worktree — status outside a repository still needs defaults.
func (l *Loader) repoConfigFile() string {
	if l.opts.WorktreeRoot == "" {
		return ""
	}
	return filepath.Join(l.opts.WorktreeRoot, RepoDir, ConfigFileName)
}

// userConfigFile is <os.UserConfigDir()>/mindrail/config.toml. A host with no
// discoverable user config directory simply has no user layer; that is not a
// failure worth refusing to start over.
func (l *Loader) userConfigFile() string {
	dir := l.opts.UserConfigDir
	if dir == "" {
		var err error
		if dir, err = os.UserConfigDir(); err != nil {
			return ""
		}
	}
	return filepath.Join(dir, productDir, ConfigFileName)
}

// fileConfig is the decode target for a configuration file.
//
// It shadows Config with pointers instead of decoding into Config directly,
// because provenance needs to distinguish "the file set output.color to the
// empty string" from "the file said nothing about output.color". It also has
// no Runtime field, which is what makes a [runtime] table in a repository file
// an unknown-key error rather than a silently accepted override.
type fileConfig struct {
	Project *fileProject `toml:"project"`
	Output  *fileOutput  `toml:"output"`
}

type fileProject struct {
	Name *string `toml:"name"`
}

type fileOutput struct {
	Color *string `toml:"color"`
}

// applyFile folds one configuration file into cfg and reports whether the file
// existed at all.
func applyFile(path string, cfg *Config, provenance Provenance, source Source) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, configError(path, "cannot be read: "+err.Error(), nil)
	}

	decoder := toml.NewDecoder(bytes.NewReader(data))
	// Strict decoding is the point of this package: an unknown safety-critical
	// field must not be silently ignored (tech-stack §37).
	decoder.DisallowUnknownFields()

	var file fileConfig
	if decodeErr := decoder.Decode(&file); decodeErr != nil {
		var strict *toml.StrictMissingError
		if errors.As(decodeErr, &strict) {
			return true, configError(path, "contains a key this version of Mindrail does not recognise:\n"+strict.String(),
				fmt.Errorf("%w in %s: %s", ErrUnknownKey, path, strict.Error()))
		}
		return true, configError(path, "is not valid TOML: "+decodeErr.Error(), decodeErr)
	}

	if file.Project != nil && file.Project.Name != nil {
		cfg.Project.Name = *file.Project.Name
		provenance[KeyProjectName] = source
	}
	if file.Output != nil && file.Output.Color != nil {
		cfg.Output.Color = *file.Output.Color
		provenance[KeyOutputColor] = source
	}

	return true, nil
}

// applyFlags is the highest-precedence layer. An unrecognised dotted key here
// is a wiring defect in this binary rather than user error, and accepting it
// would make the most authoritative layer the least trustworthy.
func applyFlags(flags map[string]string, cfg *Config, provenance Provenance) error {
	// Sorted so a caller passing two bad keys always sees the same one named.
	for _, key := range slices.Sorted(maps.Keys(flags)) {
		if !setKey(cfg, key, flags[key]) {
			return app.NewError(
				app.CodeConfigInvalid,
				app.KindUsage,
				"unknown configuration key "+quote(key)+" was set by a command-line flag",
				"Mindrail cannot apply the setting, and continuing would run with a configuration the caller did not ask for.",
				"Remove the flag, or check mindrail --help for the supported keys",
			).WithMetadata("key", key).WithCause(fmt.Errorf("%w: %s", ErrUnknownKey, key))
		}
		provenance[key] = SourceFlag
	}

	return nil
}

// setKey assigns one dotted key and reports whether the key is known. It is
// the single place the key vocabulary is turned into struct fields, so the
// env and flag layers cannot drift apart.
func setKey(cfg *Config, key, value string) bool {
	switch key {
	case KeyProjectName:
		cfg.Project.Name = value
	case KeyOutputColor:
		cfg.Output.Color = value
	case KeyRuntimeDir:
		cfg.Runtime.Dir = value
	case KeyRuntimeCacheDir:
		cfg.Runtime.CacheDir = value
	default:
		return false
	}
	return true
}

// configError builds the CONFIG_INVALID carrier. Decision D-03 maps every
// configuration problem onto exit 2: the file is deterministic and the user
// can fix it, which is exactly what "invalid usage or configuration" means.
func configError(path, problem string, cause error) error {
	err := app.NewError(
		app.CodeConfigInvalid,
		app.KindUsage,
		path+" "+problem,
		"Mindrail refuses to run on a configuration it cannot fully interpret, because a silently dropped setting can weaken a repository safety rule.",
		"Fix or remove the offending entry in "+path,
	).WithMetadata("path", path)

	if cause != nil {
		err = err.WithCause(cause)
	}
	return err
}
