//go:build unix

package filesystem_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/PsyChaos/mindrail/internal/filesystem"
)

// TestClassifyRefusalNamesEachConditionAndGuessesAtNoOther is the whole
// detection rule for "why can Mindrail not write here?", stated over the errnos
// the kernel actually returns.
//
// Every row that must not be BarrierPermission is a finding in its own right.
// EROFS and ENOSPC arrived at three separate reports as permission failures and
// were remedied with a chmod that cannot clear either -- on a read-only mount
// `chmod` itself fails with the same EROFS. The rows that must be BarrierNone
// are the other direction: a classifier that guessed at an unrecognised errno
// would produce a confident wrong remedy where the generic sentence is at least
// honest.
func TestClassifyRefusalNamesEachConditionAndGuessesAtNoOther(t *testing.T) {
	cases := []struct {
		name  string
		cause error
		want  filesystem.Barrier
	}{
		{name: "nothing refused", cause: nil, want: filesystem.BarrierNone},

		{name: "EACCES, the mode bits", cause: syscall.EACCES, want: filesystem.BarrierPermission},
		{name: "EPERM, the operation is not permitted", cause: syscall.EPERM, want: filesystem.BarrierPermission},

		{name: "ENOSPC, the filesystem is full", cause: syscall.ENOSPC, want: filesystem.BarrierNoSpace},
		{name: "EDQUOT, the quota is spent", cause: syscall.EDQUOT, want: filesystem.BarrierNoSpace},
		{
			name: "the probe's own no-space refusal, read from statfs rather than met at a write",
			cause: fmt.Errorf("statfs %q: %w: the filesystem reports 0 bytes available",
				"/full", filesystem.ErrNoSpace),
			want: filesystem.BarrierNoSpace,
		},

		{name: "EROFS, the mount refuses writes", cause: syscall.EROFS, want: filesystem.BarrierReadOnlyMedia},

		{name: "ENOTDIR, something else is on the path", cause: syscall.ENOTDIR, want: filesystem.BarrierObstruction},
		{name: "the package's own not-a-directory", cause: filesystem.ErrNotDirectory, want: filesystem.BarrierObstruction},
		{name: "a link to nowhere", cause: filesystem.ErrDanglingSymlink, want: filesystem.BarrierObstruction},

		{name: "ENOENT, a directory above is missing", cause: syscall.ENOENT, want: filesystem.BarrierMissingParent},
		{name: "the io/fs spelling of the same", cause: fs.ErrNotExist, want: filesystem.BarrierMissingParent},

		{name: "EIO, a failing drive", cause: syscall.EIO, want: filesystem.BarrierNone},
		{name: "ENOMEM, which is not the disk at all", cause: syscall.ENOMEM, want: filesystem.BarrierNone},
		{name: "an error with no errno in it", cause: errors.New("something went wrong"), want: filesystem.BarrierNone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := filesystem.ClassifyRefusal(tc.cause); got != tc.want {
				t.Errorf("ClassifyRefusal(%v) = %q, want %q", tc.cause, got, tc.want)
			}
		})
	}
}

// TestClassifyRefusalReadsThroughTheWrappingEveryCallerAdds keeps the
// classification working on the shape it is actually handed. Nothing in this
// repository passes a bare errno: access(2) failures arrive wrapped in a
// fmt.Errorf naming the path, and os.MkdirAll wraps its own in a *PathError.
func TestClassifyRefusalReadsThroughTheWrappingEveryCallerAdds(t *testing.T) {
	cases := []struct {
		name  string
		cause error
		want  filesystem.Barrier
	}{
		{
			name:  "the wrapping accessWritable adds",
			cause: fmt.Errorf("access %q: %w", "/mnt/repo/.git", syscall.EROFS),
			want:  filesystem.BarrierReadOnlyMedia,
		},
		{
			name:  "the *PathError os.MkdirAll returns",
			cause: &os.PathError{Op: "mkdir", Path: "/mnt/repo/.git/mindrail", Err: syscall.ENOSPC},
			want:  filesystem.BarrierNoSpace,
		},
		{
			name:  "the *PathError os.WriteFile returns",
			cause: &os.PathError{Op: "write", Path: "/mnt/repo/.mindrail/config.toml", Err: syscall.ENOSPC},
			want:  filesystem.BarrierNoSpace,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := filesystem.ClassifyRefusal(tc.cause); got != tc.want {
				t.Errorf("ClassifyRefusal(%v) = %q, want %q", tc.cause, got, tc.want)
			}
		})
	}
}

// TestTheThreeUnwritableConditionsAreNotConfusable is the over-fire guard the
// whole classification hangs on, stated as the pairs that must never collapse.
//
// Each of the three has a remedy the other two cannot be cleared by. If any pair
// were to merge, the report would still be produced, still be confident, and
// still send the user somewhere that cannot work -- which is how all four of the
// findings this closes stayed alive through six audits.
func TestTheThreeUnwritableConditionsAreNotConfusable(t *testing.T) {
	permission := filesystem.ClassifyRefusal(syscall.EACCES)
	noSpace := filesystem.ClassifyRefusal(syscall.ENOSPC)
	readOnly := filesystem.ClassifyRefusal(syscall.EROFS)

	if permission == noSpace {
		t.Error("a permission failure and a full filesystem classify the same; " +
			"one of them would be told to free space or to chmod, and only one of those works")
	}
	if permission == readOnly {
		t.Error("a permission failure and a read-only mount classify the same; " +
			"chmod on a read-only mount fails with the same EROFS that refused the write")
	}
	if noSpace == readOnly {
		t.Error("a full filesystem and a read-only mount classify the same; " +
			"freeing space on a read-only mount changes nothing")
	}
}

// TestFsErrPermissionDoesNotSwallowTheOtherTwo pins the ordering inside
// ClassifyRefusal rather than trusting it.
//
// Go maps EACCES and EPERM onto fs.ErrPermission and nothing else, which is
// correct -- but a future edge, or a caller that wrapped its own
// fs.ErrPermission around an EROFS, must not be able to turn a read-only mount
// back into a chmod. The specific condition wins.
func TestFsErrPermissionDoesNotSwallowTheOtherTwo(t *testing.T) {
	cases := map[string]struct {
		cause error
		want  filesystem.Barrier
	}{
		"a read-only mount wrapped in a permission error": {
			cause: fmt.Errorf("%w: %w", fs.ErrPermission, syscall.EROFS),
			want:  filesystem.BarrierReadOnlyMedia,
		},
		"a full filesystem wrapped in a permission error": {
			cause: fmt.Errorf("%w: %w", fs.ErrPermission, syscall.ENOSPC),
			want:  filesystem.BarrierNoSpace,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := filesystem.ClassifyRefusal(tc.cause); got != tc.want {
				t.Errorf("ClassifyRefusal(%v) = %q, want %q", tc.cause, got, tc.want)
			}
		})
	}
}
