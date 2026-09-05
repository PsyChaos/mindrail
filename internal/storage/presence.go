package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/PsyChaos/mindrail/internal/app"
)

// Presence is what is sitting at a runtime database path.
//
// The three values exist because collapsing them is how a report ends up
// stating something untrue. "Nothing is here yet" and "something is here that
// is not a database" stop the same commands, but only the first is fixed by
// `mindrail init`; recommending init for the second sends the user around a
// loop that fails the same way every time. A caller that only ever asked
// "does a database file exist?" has no way to tell the two apart, so the
// question is answered here once, in the layer that owns the file.
type Presence string

const (
	// PresenceAbsent means the path was looked at and nothing is there.
	PresenceAbsent Presence = "absent"

	// PresenceFile means a regular file is there. It may still turn out to be
	// corrupt or locked; that is Open's answer, not this one.
	PresenceFile Presence = "file"

	// PresenceUnusable means something occupies the path that can never be a
	// database file, or that the path could not be inspected at all.
	PresenceUnusable Presence = "unusable"
)

// ErrNotDatabaseFile means the database path is occupied by something that is
// not a regular file -- most often a directory left behind by a botched copy or
// an aborted container mount.
var ErrNotDatabaseFile = errors.New("runtime database path is not a regular file")

// Probe reports what is at path without opening or creating anything, so a
// read-only command can ask the question decision D-01 forbids it from
// answering by side effect.
//
// The returned error is non-nil exactly when the presence is PresenceUnusable,
// and it carries the remedy for the specific obstruction; PresenceAbsent is a
// state, not a failure, and is reported with a nil error.
func Probe(path string) (Presence, error) {
	if err := validatePath(path); err != nil {
		return PresenceUnusable, err
	}

	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return probeUnresolved(path)
	case err != nil:
		return probeUninspectable(path, err)
	default:
		return classify(path, info)
	}
}

// probeUninspectable is reached when the path could not be looked at, which is
// two conditions wearing one errno.
//
// A regular file standing where the runtime root should be is the common one: a
// stray `.git/mindrail` file makes os.Stat of `.git/mindrail/mindrail.db` fail
// with ENOTDIR, and the generic answer -- "the runtime database path could not be
// inspected", remedied with "Check the permissions on the directory that holds
// it" -- was wrong twice over. Permissions are not the problem, and the file is
// not a directory whose permissions could be checked. The runtime-paths check
// spotted the same obstruction and printed the real remedy, so human output
// carried two disagreeing `Next:` blocks one screen apart while the status
// `next_action` published the wrong one (finding W3).
//
// The obstruction is found by walking up to the nearest path component that
// exists, which is the same answer without the errno: a component that is on
// disk and is not a directory is the thing in the way, and naming it is what
// makes the remedy actionable. Anything else really is a permission or an I/O
// problem, and keeps the remedy that fits it -- now naming the directory it is
// about rather than "the directory that holds it".
func probeUninspectable(path string, cause error) (Presence, error) {
	obstruction, found := nonDirectoryAncestor(path)
	if found {
		return PresenceUnusable, unusableFailure(path,
			obstruction+" is in the way of the runtime database path and is not a directory",
			"Remove or move aside "+obstruction+", then run `mindrail init`.",
			errors.Join(ErrNotDatabaseFile, cause))
	}

	return PresenceUnusable, unusableFailure(path,
		"the runtime database path could not be inspected",
		"Check the permissions on "+filepath.Dir(path)+".", cause)
}

// nonDirectoryAncestor returns the nearest existing ancestor of path when that
// ancestor is not a directory, which is the component blocking every lookup
// below it.
//
// The nearest *existing* ancestor is the only one worth inspecting: everything
// below it is absent, and everything above it was walked through successfully.
// Resolution is deliberate -- decision D-32 allows symlinks inside the runtime
// root, so a link to a real directory is a supported layout and must not be
// reported as an obstruction. The answer is claimed only when the component is
// confirmed to be something other than a directory; every other reason a stat can
// fail keeps the permission remedy, which is the one that fits it.
func nonDirectoryAncestor(path string) (string, bool) {
	current := filepath.Dir(filepath.Clean(path))
	for {
		if _, err := os.Lstat(current); err == nil {
			info, statErr := os.Stat(current)
			return current, statErr == nil && !info.IsDir()
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

// probeUnresolved is reached when the path resolves to nothing. Almost always
// that means nothing is there, but a dangling symlink resolves to nothing while
// very much being there, and reporting it as absent would recommend `mindrail
// init` for a link init cannot write through. The extra syscall is paid only on
// the uninitialised path, never on a warm start.
func probeUnresolved(path string) (Presence, error) {
	link, err := os.Lstat(path)
	switch {
	case err != nil:
		return PresenceAbsent, nil
	case link.Mode()&fs.ModeSymlink != 0:
		return PresenceUnusable, unusableFailure(path,
			"the runtime database path is a symlink that points at nothing",
			"Remove or repoint the link at "+path+", then run `mindrail init`.", ErrNotDatabaseFile)
	default:
		// The path came into existence between the two calls, which is exactly
		// what a second `mindrail init` on the same repository looks like from
		// here. Describe what is now there rather than the emptiness that was
		// there a syscall ago.
		return classify(path, link)
	}
}

// classify turns a stat result into the three-way answer. It is shared by both
// probe paths so a file that appeared mid-probe is described the same way as
// one that was there all along.
func classify(path string, info fs.FileInfo) (Presence, error) {
	switch {
	case info.Mode().IsRegular():
		return PresenceFile, nil
	case info.IsDir():
		return PresenceUnusable, unusableFailure(path,
			"a directory occupies the runtime database path",
			"Remove or move aside the directory at "+path+", then run `mindrail init`.", ErrNotDatabaseFile)
	default:
		return PresenceUnusable, unusableFailure(path,
			"the runtime database path is occupied by "+describeMode(info.Mode())+", not a database file",
			"Remove or move aside "+path+", then run `mindrail init`.", ErrNotDatabaseFile)
	}
}

// unusableFailure describes an obstruction rather than an absence. The next
// action names the obstruction first on purpose: `mindrail init` alone cannot
// clear it, and offering it as the whole remedy is what turns a one-line fix
// into a repeated exit 4.
func unusableFailure(path, why, next string, cause error) error {
	return app.NewError(
		app.CodeRuntimeDBUnavailable,
		app.KindUnavailable,
		why,
		"Mindrail can neither open nor create the runtime database while its path is occupied.",
		next,
	).
		WithMetadata("path", path).
		WithMetadata("path_state", string(PresenceUnusable)).
		WithCause(errors.Join(ErrOpenFailed, cause))
}

// absentFailure is the other half of the distinction: the path was inspected
// and is genuinely empty, which for a read-only command is the reportable
// "never initialised" state of decision D-01.
func absentFailure(path string) error {
	return app.NewError(
		app.CodeRuntimeDBUnavailable,
		app.KindUnavailable,
		"there is no runtime database at "+path,
		"Mindrail has no recorded workspace state for this repository yet.",
		"Run `mindrail init` to create it.",
	).
		WithMetadata("path", path).
		WithMetadata("path_state", string(PresenceAbsent)).
		WithCause(errors.Join(ErrOpenFailed, fs.ErrNotExist))
}

// describeMode names the non-regular, non-directory cases in words a user can
// act on, because "mode 0xC000019F" tells them nothing about what to delete.
// A symlink never reaches it: os.Stat resolves them, and the one case os.Lstat
// sees is answered by probeUnresolved before it gets here.
func describeMode(mode fs.FileMode) string {
	switch {
	case mode&fs.ModeSocket != 0:
		return "a socket"
	case mode&fs.ModeNamedPipe != 0:
		return "a named pipe"
	case mode&fs.ModeDevice != 0:
		return "a device node"
	default:
		return "a special file"
	}
}
