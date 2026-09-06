package storage

import (
	"errors"
	"fmt"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

// noSpaceRefusal reports the filesystem holding dir having nothing left to give,
// and nil for every other state including "could not be determined".
//
// The reading itself lives in internal/filesystem, and this is a translation
// rather than a second implementation. The same statfs answer decides whether
// the runtime root can be created, whether `<worktree>/.mindrail` can take a
// config file, and whether this database can take a write; when the reading
// lived here only the last of those could see a full disk, and `doctor` reported
// `runtime_root_usable: true` over a machine where `mindrail init` could not
// create the directory at all (finding E1).
//
// What is added here is the sentinel a storage caller unwraps for. ErrDiskFull
// is what every command, test and classifier in this package asks errors.Is
// about, and it has to keep answering yes for a condition the filesystem layer
// discovered.
func noSpaceRefusal(dir string) error {
	err := filesystem.NoSpaceRefusal(dir)
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrDiskFull, err)
}

// noSpaceDatabaseError reports a runtime database that no permission change can
// make writable, which is why it is kept apart from unwritableDatabaseError
// rather than folded into it as a fourth blocker with the same words.
//
// The two conditions look identical from the outside -- every write fails, the
// database reads perfectly -- and have remedies with nothing in common. Telling
// somebody whose disk is full to restore write permission on a file whose mode
// is already 0644 is the dead-end remedy this package keeps being audited for;
// the point of naming the condition separately is that the sentence changes.
//
// The wording matches diskFullFailure and diskFullOpenFailure exactly, so the
// `mindrail init` that fails on a full disk, the `mindrail status` that cannot
// open the database on the same disk, and the `mindrail doctor` that inspects it
// without opening anything all print one sentence rather than three.
func noSpaceDatabaseError(path, blocked string, cause error) error {
	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		"the filesystem holding the runtime database at "+path+" is full",
		"Mindrail can read the state already recorded here but cannot record anything new, so `mindrail init` and every other command that writes will fail until there is space.",
		"free space on the filesystem holding "+path,
	).
		WithMetadata("path", path).
		WithMetadata("blocked_path", blocked).
		WithMetadata("blocker", string(BlockerNoSpace)).
		WithMetadata("condition", "disk_full").
		WithMetadata("detail", cause.Error()).
		WithCause(errors.Join(ErrDiskFull, cause))
}

// readOnlyMediaDatabaseError reports a runtime database on a filesystem mounted
// read-only, which is the third condition in this family and the one that had no
// remedy of its own.
//
// It is separate from unwritableDatabaseError for the reason that one is
// separate from noSpaceDatabaseError: the sentence has to change, because the
// remedy has nothing in common. "Restore write permission on mindrail.db,
// mindrail.db-wal, mindrail.db-shm" was printed for files whose owner-write bit
// was already set, on which `chmod` itself fails with the same EROFS that
// refused the write (finding E4). Freeing space does not help either. The only
// thing a user can do is make the mount writable or put the runtime state
// somewhere that already is, so those are the two things this says.
func readOnlyMediaDatabaseError(path, blocked string, cause error) error {
	return app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		"the runtime database at "+path+" is on a read-only filesystem",
		"Mindrail can read the state already recorded here but cannot record anything new, and no permission change or free space will alter that while the mount refuses writes.",
		"remount the filesystem holding "+blocked+" read-write, or point MINDRAIL_RUNTIME_DIR at a writable location",
	).
		WithMetadata("path", path).
		WithMetadata("blocked_path", blocked).
		WithMetadata("blocker", string(BlockerReadOnlyMedia)).
		WithMetadata("condition", "read_only_filesystem").
		WithMetadata("detail", cause.Error()).
		WithCause(errors.Join(ErrReadOnly, filesystem.ErrReadOnlyMedia, cause))
}
