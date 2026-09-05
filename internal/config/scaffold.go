package config

import (
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/PsyChaos/mindrail/internal/app"
)

//go:embed templates/config.toml
var defaultTemplate []byte

// Scaffolded paths are repository content: they are committed, reviewed and
// read by every contributor and by CI, so they take the ordinary repository
// modes rather than the 0700/0600 of the machine-local runtime tree.
const (
	repoDirMode  os.FileMode = 0o755
	repoFileMode os.FileMode = 0o644
)

// knowledgeSubdirs are the two write targets MR-002 will need. They are stored
// slash-separated because that is the form EnsureKnowledgeDirs reports
// (tech-stack §74).
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
// The file is created with O_EXCL rather than after a stat, so two inits
// racing in the same worktree cannot both decide the file is absent and have
// the loser clobber the winner.
func WriteIfAbsent(worktreeRoot string) (path string, written bool, err error) {
	dir := filepath.Join(worktreeRoot, RepoDir)
	if mkErr := os.MkdirAll(dir, repoDirMode); mkErr != nil {
		return "", false, scaffoldError(dir, mkErr)
	}

	path = filepath.Join(dir, ConfigFileName)
	file, openErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, repoFileMode)
	if errors.Is(openErr, fs.ErrExist) {
		return path, false, nil
	}
	if openErr != nil {
		return path, false, scaffoldError(path, openErr)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = scaffoldError(path, closeErr)
		}
	}()

	if _, writeErr := file.Write(defaultTemplate); writeErr != nil {
		return path, false, scaffoldError(path, writeErr)
	}

	return path, true, nil
}

// EnsureKnowledgeDirs creates .mindrail/knowledge/{decisions,invariants}/ each
// with a .gitkeep. Returns the repo-relative dirs it actually created.
//
// The .gitkeep files are what make decision D-05 work: Git tracks no empty
// directory, so without them a clone of an initialised repository would arrive
// with MR-002's write targets missing.
func EnsureKnowledgeDirs(worktreeRoot string) (created []string, err error) {
	created = make([]string, 0, len(knowledgeSubdirs))

	for _, rel := range knowledgeSubdirs {
		dir := filepath.Join(worktreeRoot, filepath.FromSlash(rel))

		_, statErr := os.Stat(dir)
		switch {
		case statErr == nil:
			// Already there; a second init reports nothing as created.
		case errors.Is(statErr, fs.ErrNotExist):
			if mkErr := os.MkdirAll(dir, repoDirMode); mkErr != nil {
				return nil, scaffoldError(dir, mkErr)
			}
			created = append(created, rel)
		default:
			return nil, scaffoldError(dir, statErr)
		}

		keep := filepath.Join(dir, ".gitkeep")
		file, openErr := os.OpenFile(keep, os.O_WRONLY|os.O_CREATE|os.O_EXCL, repoFileMode)
		if errors.Is(openErr, fs.ErrExist) {
			// Someone may have put a note in it; leave it alone.
			continue
		}
		if openErr != nil {
			return nil, scaffoldError(keep, openErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return nil, scaffoldError(keep, closeErr)
		}
	}

	return created, nil
}

// scaffoldError reports a failure to write repository scaffolding. It is
// KindUnavailable rather than KindUsage: an unwritable checkout is an
// environment condition, not a malformed request (decision D-03).
func scaffoldError(path string, cause error) error {
	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		"cannot write "+path+": "+cause.Error(),
		"Mindrail cannot lay down the repository scaffolding, so knowledge records have nowhere to live.",
		"Check the permissions and free space on "+path,
	).WithMetadata("path", path).WithCause(cause)
}
