package config

import (
	_ "embed"
	"errors"
	"path/filepath"
	"slices"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

//go:embed templates/config.toml
var defaultTemplate []byte

// Scaffolded paths are repository content: they are committed, reviewed and
// read by every contributor and by CI, so they take the ordinary repository
// modes rather than the 0700/0600 of the machine-local runtime tree.
var repoModes = filesystem.Modes{Dir: 0o755, File: 0o644}

// knowledgeSubdirs are the two write targets MR-002 will need. They are stored
// slash-separated because that is the form EnsureKnowledgeDirs reports
// (tech-stack §74) and the form filesystem.Root takes.
var knowledgeSubdirs = []string{
	RepoDir + "/knowledge/decisions",
	RepoDir + "/knowledge/invariants",
}

// DefaultTemplate is the embedded .mindrail/config.toml scaffold (tech-stack
// §111). The result is a copy, so a caller that rewrites it cannot corrupt the
// template for the rest of the process.
func DefaultTemplate() []byte {
	return slices.Clone(defaultTemplate)
}

// WriteIfAbsent creates <worktreeRoot>/.mindrail/config.toml from the template.
// It never overwrites (spec §82) and reports which happened.
//
// Every create goes through filesystem.Root, so the containment rule of
// decision D-32 and spec §113 is applied before the bytes exist rather than
// after. Building the paths here with filepath.Join and handing them straight
// to the operating system let a .mindrail symlink pointing out of the worktree
// take the whole scaffold with it, and the only thing that noticed was the
// knowledge reader, one step later, with the files already written.
//
// The file is created with O_EXCL rather than after a stat, so two inits racing
// in the same worktree cannot both decide the file is absent and have the loser
// clobber the winner.
func WriteIfAbsent(worktreeRoot string) (path string, written bool, err error) {
	// The reported path stays in the caller's spelling of the worktree. It is
	// what every other layer prints for the repository config file, and a
	// canonicalized spelling here would make `init` and `status` disagree about
	// the same file on any host whose temporary or home directory is a symlink.
	path = filepath.Join(worktreeRoot, RepoDir, ConfigFileName)
	dir := filepath.Join(worktreeRoot, RepoDir)

	root, rootErr := filesystem.NewRoot(worktreeRoot)
	if rootErr != nil {
		return path, false, scaffoldError(dir, rootErr)
	}

	if _, _, mkErr := root.EnsureDirMode(RepoDir, repoModes.Dir); mkErr != nil {
		return path, false, scaffoldError(dir, mkErr)
	}

	written, _, writeErr := root.WriteFileIfAbsentMode(
		RepoDir+"/"+ConfigFileName, defaultTemplate, repoModes)
	if writeErr != nil {
		return path, false, scaffoldError(path, writeErr)
	}

	return path, written, nil
}

// EnsureKnowledgeDirs creates .mindrail/knowledge/{decisions,invariants}/ each
// with a .gitkeep. Returns the repo-relative dirs it actually created.
//
// The .gitkeep files are what make decision D-05 work: Git tracks no empty
// directory, so without them a clone of an initialised repository would arrive
// with MR-002's write targets missing.
func EnsureKnowledgeDirs(worktreeRoot string) (created []string, err error) {
	root, rootErr := filesystem.NewRoot(worktreeRoot)
	if rootErr != nil {
		return nil, scaffoldError(filepath.Join(worktreeRoot, RepoDir), rootErr)
	}

	created = make([]string, 0, len(knowledgeSubdirs))

	for _, rel := range knowledgeSubdirs {
		dir := filepath.Join(worktreeRoot, filepath.FromSlash(rel))

		_, madeIt, mkErr := root.EnsureDirMode(rel, repoModes.Dir)
		if mkErr != nil {
			return nil, scaffoldError(dir, mkErr)
		}
		if madeIt {
			created = append(created, rel)
		}

		// An existing .gitkeep is left alone: someone may have put a note in it.
		keep := rel + "/.gitkeep"
		if _, _, keepErr := root.WriteFileIfAbsentMode(keep, nil, repoModes); keepErr != nil {
			return nil, scaffoldError(filepath.Join(dir, ".gitkeep"), keepErr)
		}
	}

	return created, nil
}

// scaffoldError reports a failure to write repository scaffolding.
//
// A containment refusal is passed through untouched. PATH_ESCAPES_ROOT already
// says exactly what happened and how to clear it, and re-coding it as an
// unwritable path would send the user to chmod a directory whose permissions
// are fine.
//
// Everything else is KindUnavailable rather than KindUsage: an unwritable
// checkout is an environment condition, not a malformed request (decision
// D-03). It carries the repository config directory's root_kind so a consumer
// grading by root kind can tell it from the runtime and cache roots, which are
// machine-local, relocatable by environment variable, and not the thing that
// failed here.
func scaffoldError(path string, cause error) error {
	if errors.Is(cause, filesystem.ErrEscapesRoot) {
		return cause
	}

	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		"cannot write "+path+": "+reason(cause),
		filesystem.RootRepository.Impact(),
		"Check the permissions and free space on "+path,
	).WithMetadata("path", path).
		WithMetadata("root_kind", string(filesystem.RootRepository)).
		WithCause(cause)
}

// reason is what goes after "cannot write <path>". The filesystem package names
// the path in its own wrapper, so quoting that verbatim printed the same path
// three times in one sentence; one level down is the operating system's own
// complaint, which is the part the reader has to act on. The full chain is still
// attached with WithCause, so nothing is lost to errors.Is or to the payload's
// cause field.
func reason(cause error) string {
	if inner := errors.Unwrap(cause); inner != nil {
		return inner.Error()
	}
	return cause.Error()
}
