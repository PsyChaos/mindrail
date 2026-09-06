//go:build unix

package filesystem

import (
	"errors"
	"syscall"
)

// platformBarrier maps the kernel's own answer onto the five conditions.
//
// The errno is the only thing that can tell these apart. access(2) and
// mkdir(2) return EACCES, EROFS and ENOSPC for three conditions that look
// identical from every other reading: the directory exists, its mode bits are
// 0755, its owner is the calling user, and the write is refused. Go's own
// mapping only folds EACCES and EPERM into fs.ErrPermission, so every other one
// of them reached a report through the default branch and was printed as a
// permission failure.
//
// EDQUOT is grouped with ENOSPC deliberately. A quota is not the filesystem
// being full, but the remedy is the same shape -- make room, do not chmod -- and
// a user who is told to check permissions on a path they own has been sent
// nowhere. EFBIG is left out: a file that has hit the filesystem's own size
// ceiling is not a repository anybody frees space to fix, and the driver's
// SQLITE_FULL is what names it.
func platformBarrier(cause error) Barrier {
	var errno syscall.Errno
	if !errors.As(cause, &errno) {
		return BarrierNone
	}

	switch errno {
	case syscall.EROFS:
		return BarrierReadOnlyMedia
	case syscall.ENOSPC, syscall.EDQUOT:
		return BarrierNoSpace
	case syscall.ENOTDIR, syscall.EEXIST, syscall.EISDIR:
		return BarrierObstruction
	case syscall.EACCES, syscall.EPERM:
		return BarrierPermission
	case syscall.ENOENT:
		return BarrierMissingParent
	default:
		return BarrierNone
	}
}
