package filesystem

import "fmt"

// FreeSpace is what the filesystem under a path will still hand out, as far as
// this process is allowed to see.
//
// Known is not a formality. Every other question a writability probe asks has a
// definite answer on every platform this builds for; this one does not, and an
// unknown answer must read as "no verdict" rather than as "no space". A
// zero-valued FreeSpace is therefore the safe value: it says nothing, and
// nothing is what a probe that could not look owes the report.
//
// It lives here rather than in the package that first needed it because the
// answer is about a directory, not about a database. When only the runtime
// database asked, the probe that answers for the runtime root, the cache
// directory and `<worktree>/.mindrail` could not see a full disk at all: it
// reported `runtime_root_usable: true` for a directory that could not be
// created, and `doctor` exited 0 over a machine where `mindrail init` fails
// every time (finding E1).
type FreeSpace struct {
	Known bool

	// AvailableBytes is what an unprivileged write could still consume,
	// saturated at the top rather than allowed to wrap. It excludes any reserve
	// the filesystem keeps for the superuser, which is the number that decides
	// whether an ordinary `mindrail init` succeeds.
	AvailableBytes int64
}

// Exhausted is the whole detection rule, and it is deliberately the narrowest
// one that can be true: the filesystem was asked, it answered, and the answer
// was that there is nothing left at all.
//
// A threshold above zero was considered and rejected. "Fewer than N bytes free"
// would report a working repository as unwritable for every N a real filesystem
// can dip below during a large checkout, and this verdict is read by `doctor`,
// whose entire value is that it is believed. The cost of the narrow rule is an
// under-fire: a filesystem with one block left cannot really give SQLite the
// 32 KiB of shared-memory index it wants, and this still calls it writable. That
// failure mode ends in the driver's own SQLITE_FULL, which storage classifies
// and remedies with the same sentence -- so the narrow rule costs a user a
// slightly later diagnosis, where a wide one would cost them a false one.
func (s FreeSpace) Exhausted() bool { return s.Known && s.AvailableBytes <= 0 }

// ProbeFreeSpace reads what the filesystem holding dir has left. It creates
// nothing and opens nothing, so decision D-01's "doctor mutates nothing"
// survives it.
func ProbeFreeSpace(dir string) FreeSpace { return statFreeSpace(dir) }

// NoSpaceRefusal reports the filesystem holding dir having nothing left to give,
// and nil for every other state including "could not be determined".
//
// dir has to be a directory that exists. A caller asking about a path that does
// not exist yet must ask about its nearest existing ancestor instead: that is
// the filesystem a create would land on, and statfs on the absent path answers
// nothing at all. Passing the absent path is how the space reading became
// unreachable in the one shape that mattered -- a first `mindrail init` on a
// full disk (finding E1).
func NoSpaceRefusal(dir string) error {
	if !ProbeFreeSpace(dir).Exhausted() {
		return nil
	}
	return fmt.Errorf("statfs %q: %w: the filesystem reports 0 bytes available", dir, ErrNoSpace)
}
