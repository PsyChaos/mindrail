package storage

import (
	"errors"
	"fmt"

	"github.com/PsyChaos/mindrail/internal/app"
)

// freeSpace is what the filesystem under a path will still hand out, as far as
// this process is allowed to see.
//
// Known is not a formality. Every other question ProbeWriteAccess asks has a
// definite answer on every platform this builds for; this one does not, and an
// unknown answer must read as "no verdict" rather than as "no space". A
// zero-valued freeSpace is therefore the safe value: it says nothing, and
// nothing is what a probe that could not look owes the report.
type freeSpace struct {
	Known bool

	// AvailableBytes is what an unprivileged write could still consume,
	// saturated at the top rather than allowed to wrap. It excludes any reserve
	// the filesystem keeps for the superuser, which is the number that decides
	// whether an ordinary `mindrail init` succeeds.
	AvailableBytes int64
}

// exhausted is the whole detection rule, and it is deliberately the narrowest
// one that can be true: the filesystem was asked, it answered, and the answer
// was that there is nothing left at all.
//
// A threshold above zero was considered and rejected. "Fewer than N bytes free"
// would report a working repository as unwritable for every N a real filesystem
// can dip below during a large checkout, and this verdict is read by `doctor`,
// whose entire value is that it is believed. The cost of the narrow rule is an
// under-fire: a filesystem with one block left cannot really give SQLite the
// 32 KiB of shared-memory index it wants, and this still calls it writable. That
// failure mode ends in the driver's own SQLITE_FULL, which is classified
// (see diskFullCode) and carries the same remedy -- so the narrow rule costs a
// user a slightly later diagnosis, where a wide one would cost them a false one.
func (s freeSpace) exhausted() bool { return s.Known && s.AvailableBytes <= 0 }

// noSpaceRefusal reports the filesystem holding dir having nothing left to give,
// and nil for every other state including "could not be determined".
func noSpaceRefusal(dir string) error {
	space := statFreeSpace(dir)
	if !space.exhausted() {
		return nil
	}
	return fmt.Errorf("statfs %q: %w: the filesystem reports 0 bytes available", dir, ErrDiskFull)
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
