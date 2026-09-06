package filesystem

import (
	"errors"
	"io/fs"
)

// Barrier names why Mindrail cannot write to a path, in the terms a remedy has
// to be chosen from.
//
// It exists because "can Mindrail write here?" was being answered separately at
// every place that asked, and each place knew about a different subset of the
// ways a write can fail. Three of those subsets stopped at the mode bits, so a
// filesystem with no room left and a filesystem mounted read-only both arrived
// at the report as a permission problem and were remedied with a chmod that
// cannot clear either. The distinction is not cosmetic: the three conditions
// have no remedy in common.
//
//   - BarrierPermission is cleared by a mode or ownership change, and by nothing
//     else.
//   - BarrierNoSpace is cleared by freeing space, and by nothing else. Every mode
//     bit in the path is already correct.
//   - BarrierReadOnlyMedia is cleared by neither. The mount refuses the write,
//     `chmod` on the path fails with the same EROFS, and the only thing a user
//     can do is remount the filesystem read-write or put the state somewhere
//     else.
//   - BarrierObstruction is cleared by moving whatever is standing in the way.
//   - BarrierMissingParent means the path cannot be created because a directory
//     above it is not there, which is a different sentence from "not writable".
//
// BarrierNone is both "nothing refused" and "this refusal is not one of the five",
// because a classifier that guessed at an unrecognised errno would produce
// exactly the confident wrong remedy this type exists to remove. An unclassified
// cause keeps the generic sentence, which is honest.
type Barrier string

const (
	// BarrierNone means the cause is nil or names no condition this package can
	// choose a remedy from.
	BarrierNone Barrier = ""

	// BarrierPermission means the mode bits or ownership refuse the write.
	BarrierPermission Barrier = "permission"

	// BarrierNoSpace means the filesystem has nothing left to give.
	BarrierNoSpace Barrier = "no_space"

	// BarrierReadOnlyMedia means the filesystem is mounted read-only.
	BarrierReadOnlyMedia Barrier = "read_only_filesystem"

	// BarrierObstruction means something else occupies the path.
	BarrierObstruction Barrier = "obstruction"

	// BarrierMissingParent means a directory above the path does not exist.
	BarrierMissingParent Barrier = "missing_parent"
)

// The conditions this package can name that have no errno of their own on the
// path they are discovered from. They are sentinels so that a caller several
// layers up -- storage, config, doctor -- can ask errors.Is about a refusal it
// only ever sees wrapped, exactly as it can for the errno-backed conditions.
var (
	// ErrNoSpace reports a filesystem with nothing left, observed by reading the
	// filesystem rather than by attempting a write that would fail.
	ErrNoSpace = errors.New("no space left on the filesystem")

	// ErrReadOnlyMedia reports a filesystem mounted read-only. It is not
	// fs.ErrPermission and must never be folded into it: the remedies do not
	// overlap.
	ErrReadOnlyMedia = errors.New("the filesystem is mounted read-only")
)

// ClassifyRefusal names the condition behind a refused write, and is the single
// place that decision is made.
//
// The order is the order of specificity, and it is load-bearing. EROFS and
// ENOSPC are asked about before fs.ErrPermission because a classifier that
// reached the permission branch first would answer "permission" for a read-only
// mount on any platform whose errno mapping ever grew that edge -- which is the
// exact shape of the bug this replaces. The package's own obstruction sentinels
// come first of all, because they describe a path rather than a syscall and no
// errno can contradict them.
//
// A cause it does not recognise is BarrierNone rather than a guess.
func ClassifyRefusal(cause error) Barrier {
	if cause == nil {
		return BarrierNone
	}

	switch {
	case errors.Is(cause, ErrDanglingSymlink), errors.Is(cause, ErrNotDirectory):
		return BarrierObstruction
	case errors.Is(cause, ErrNoSpace):
		return BarrierNoSpace
	case errors.Is(cause, ErrReadOnlyMedia):
		return BarrierReadOnlyMedia
	}

	// The errno half, which is where a real syscall failure is named. It is
	// consulted before the io/fs sentinels for the reason above.
	if barrier := platformBarrier(cause); barrier != BarrierNone {
		return barrier
	}

	switch {
	case errors.Is(cause, fs.ErrPermission):
		return BarrierPermission
	case errors.Is(cause, fs.ErrNotExist):
		return BarrierMissingParent
	}
	return BarrierNone
}
