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
	"github.com/PsyChaos/mindrail/internal/filesystem"
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

	repoFile, repoErr := l.repoConfigFile()
	if repoErr != nil {
		return Loaded{}, repoErr
	}
	if repoFile != "" {
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
//
// The repository layer is the only one that is supposed to come from inside the
// worktree, so it is the one that has to be containment-checked. Without the
// check, a .mindrail symlink pointing out of the repository had Mindrail read
// its configuration from wherever the link went and then report the in-repo
// path as the file it had loaded, which is the reading half of what spec §113
// forbids and what escapeError already promises not to do.
//
// Only an escape refuses. A boundary that could not be evaluated at all is a
// different condition, and turning it into a refusal here would take down every
// command in a worktree git itself was happy to resolve; the read below then
// fails, or succeeds, exactly as it did before.
//
// The second refusal is an obstruction at .mindrail itself, and it is asked
// before the read because the read cannot describe it. A regular file there
// fails the open with ENOTDIR, which arrived here as "config.toml cannot be
// read", coded CONFIG_INVALID at exit 2, remedied with "fix or remove the
// offending entry in <repo>/.mindrail/config.toml" — a path that cannot be
// reached at all — while `init` called the identical disk RUNTIME_PATH_UNWRITABLE
// at exit 4 (finding D5). filesystem.ObstructedDir is the single answer both now
// use, so one condition has one code, one exit class and one remedy that clears
// it.
func (l *Loader) repoConfigFile() (string, error) {
	if l.opts.WorktreeRoot == "" {
		return "", nil
	}

	path := filepath.Join(l.opts.WorktreeRoot, RepoDir, ConfigFileName)

	root, err := filesystem.NewRoot(l.opts.WorktreeRoot)
	if err != nil {
		return path, nil
	}
	if _, err := root.Resolve(RepoDir + "/" + ConfigFileName); errors.Is(err, filesystem.ErrEscapesRoot) {
		return "", err
	}

	repoDir := filepath.Join(l.opts.WorktreeRoot, RepoDir)
	if obstruction := filesystem.ObstructedDir(filesystem.RootRepository, repoDir); obstruction != nil {
		return "", obstruction
	}
	return path, nil
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
	Project    *fileProject               `toml:"project"`
	Output     *fileOutput                `toml:"output"`
	Validation map[string]*fileValidation `toml:"validation"`
	Secrets    *fileSecrets               `toml:"secrets"`
	Continuity *fileContinuity            `toml:"continuity"`
}

type fileProject struct {
	Name *string `toml:"name"`
}

type fileOutput struct {
	Color *string `toml:"color"`
}

// fileValidation shadows ValidationProfile with pointers so presence is
// detectable per layer: a layer that says nothing about a field leaves the
// lower layer's value alone.
type fileValidation struct {
	Type     *string     `toml:"type"`
	Paths    *[]string   `toml:"paths"`
	Commands *[][]string `toml:"commands"`
}

type fileSecrets struct {
	Env *[]string `toml:"env"`
}

type fileContinuity struct {
	Enabled                 *bool `toml:"enabled"`
	WarnUsedPercent         *int  `toml:"warn_used_percent"`
	HandoffUsedPercent      *int  `toml:"handoff_used_percent"`
	HardUsedPercent         *int  `toml:"hard_used_percent"`
	ConsecutiveObservations *int  `toml:"consecutive_observations"`
}

// applyFile folds one configuration file into cfg and reports whether the file
// existed at all.
func applyFile(path string, cfg *Config, provenance Provenance, source Source) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, configOpenError(path, err)
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
	for _, name := range slices.Sorted(maps.Keys(file.Validation)) {
		layer := file.Validation[name]
		if layer == nil {
			continue
		}
		if cfg.Validation == nil {
			cfg.Validation = map[string]ValidationProfile{}
		}
		profile := cfg.Validation[name]
		if layer.Type != nil {
			profile.Type = *layer.Type
		}
		if layer.Paths != nil {
			profile.Paths = *layer.Paths
		}
		if layer.Commands != nil {
			profile.Commands = *layer.Commands
		}
		cfg.Validation[name] = profile
		provenance["validation."+name] = source
	}
	if file.Secrets != nil && file.Secrets.Env != nil {
		cfg.Secrets.Env = *file.Secrets.Env
		provenance["secrets.env"] = source
	}
	if layer := file.Continuity; layer != nil {
		changed := false
		if layer.Enabled != nil {
			cfg.Continuity.Enabled = *layer.Enabled
			changed = true
		}
		if layer.WarnUsedPercent != nil {
			cfg.Continuity.WarnUsedPercent = *layer.WarnUsedPercent
			changed = true
		}
		if layer.HandoffUsedPercent != nil {
			cfg.Continuity.HandoffUsedPercent = *layer.HandoffUsedPercent
			changed = true
		}
		if layer.HardUsedPercent != nil {
			cfg.Continuity.HardUsedPercent = *layer.HardUsedPercent
			changed = true
		}
		if layer.ConsecutiveObservations != nil {
			cfg.Continuity.ConsecutiveObservations = *layer.ConsecutiveObservations
			changed = true
		}
		if changed {
			provenance["continuity"] = source
		}
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

// configOpenError reports a configuration file that could not be opened at all.
//
// This is not the condition CONFIG_INVALID usually describes, and it must not
// borrow its remedy. A file whose contents are wrong is fixed by editing an
// entry in it; a file at mode 0000, or a directory standing where the file
// belongs, has no entry to edit and cannot be opened to look for one. Printing
// "Fix or remove the offending entry in <path>" for those told the reader to
// carry out an action that fails with the very errno that produced the report,
// and it did so on all three of init, status and doctor, so the agreement matrix
// saw three commands agreeing on a remedy none of them could clear (finding F02).
//
// The code stays CONFIG_INVALID and the kind stays Usage: this is still the
// configuration refusing to load, and decision D-03 puts that at exit 2. What
// changes is that the sentence and the remedy come from
// filesystem.ClassifyRefusal — the same classifier that already names the
// condition for .mindrail itself, for .mindrail/knowledge and for the record
// buckets under it — so the one path in the tree that still guessed now asks.
//
// An unrecognised cause keeps the generic sentence and the permission remedy,
// which is the honest default: EACCES is what almost every unreadable file is,
// and the cause string travels on the wire beside it.
func configOpenError(path string, cause error) error {
	why := path + " cannot be read: " + cause.Error()
	next := []string{"Check the permissions on " + path}

	switch filesystem.ClassifyRefusal(cause) {
	case filesystem.BarrierObstruction:
		why = path + " is not a readable file: " + cause.Error()
		next = []string{"Remove or move aside " + path + ", so that Mindrail can write a configuration file there"}
	case filesystem.BarrierPermission:
		why = path + " cannot be read: " + cause.Error()
		next = []string{"Check the permissions on " + path}
	case filesystem.BarrierReadOnlyMedia:
		why = path + " is on a read-only filesystem and could not be read: " + cause.Error()
		next = []string{"Remount the filesystem holding " + path + " read-write, or move this repository to a writable location"}
	}

	return app.NewError(
		app.CodeConfigInvalid,
		app.KindUsage,
		why,
		"Mindrail refuses to run on a configuration it cannot fully interpret, because a silently dropped setting can weaken a repository safety rule.",
		next...,
	).WithMetadata("path", path).
		WithMetadata("barrier", string(filesystem.ClassifyRefusal(cause))).
		WithCause(cause)
}
