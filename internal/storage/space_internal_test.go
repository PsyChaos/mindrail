package storage

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestFreeSpaceExhaustedOnlyWhenTheAnswerIsKnownAndZero pins the detection rule
// itself, on values a test can state exactly rather than on a filesystem it has
// to arrange.
//
// The two directions cost different things and both are here. Firing on an
// unknown answer would report every repository on a platform without a
// free-space call as unwritable; not firing on a known zero is the finding this
// exists to close. The middle rows are the adjacent condition: a filesystem that
// is nearly full is not a filesystem that is full, and `doctor` is read by
// people whose disks are usually fairly full.
func TestFreeSpaceExhaustedOnlyWhenTheAnswerIsKnownAndZero(t *testing.T) {
	cases := []struct {
		name  string
		space freeSpace
		want  bool
	}{
		{name: "no answer at all", space: freeSpace{}, want: false},
		{name: "unknown answer carrying a zero", space: freeSpace{Known: false, AvailableBytes: 0}, want: false},
		{name: "known and empty", space: freeSpace{Known: true, AvailableBytes: 0}, want: true},
		{name: "known and one byte left", space: freeSpace{Known: true, AvailableBytes: 1}, want: false},
		{name: "known and nearly full", space: freeSpace{Known: true, AvailableBytes: 4096}, want: false},
		{name: "known and a terabyte free", space: freeSpace{Known: true, AvailableBytes: 1 << 40}, want: false},
		{
			// Only reachable through a broken statfs answer, and the direction
			// matters: a negative byte count must not read as "plenty".
			name: "known and nonsensical", space: freeSpace{Known: true, AvailableBytes: -1}, want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.space.exhausted(); got != tc.want {
				t.Errorf("freeSpace%+v.exhausted() = %v, want %v", tc.space, got, tc.want)
			}
		})
	}
}

// TestNoSpaceRefusalStaysSilentWhereThereIsRoom is the over-fire guard on the
// probe's entry point: an ordinary directory on an ordinary filesystem must
// produce no refusal at all. Every test in this repository runs in one, so a
// rule that fired here would fail the suite loudly -- which is the point.
func TestNoSpaceRefusalStaysSilentWhereThereIsRoom(t *testing.T) {
	if err := noSpaceRefusal(t.TempDir()); err != nil {
		t.Fatalf("noSpaceRefusal(a temp dir) = %v, want nil; this filesystem has room", err)
	}
}

// TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat keeps "could not look"
// separate from "looked and found nothing". A directory that does not exist is
// somebody else's diagnosis -- ProbeWriteAccess has already reported it as an
// obstruction or a directory that refuses entries -- and answering "the disk is
// full" about it would replace a precise reading with a wrong one.
func TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat(t *testing.T) {
	if err := noSpaceRefusal(filepath.Join(t.TempDir(), "does", "not", "exist")); err != nil {
		t.Fatalf("noSpaceRefusal(absent path) = %v, want nil", err)
	}
}

// TestDiskFullCodeCoversTheCodesThatMeanNoRoomAndNoOthers is the audit in
// executable form.
//
// isDiskFullError deleted its own detection once already and the suite stayed
// green (finding D3), because every case that could have caught it needed a real
// full filesystem to provoke. The policy is separable from the provocation, so
// it is tested separately: these are SQLite's own result codes, and the two that
// mean "there was no room" are the two this returns true for.
func TestDiskFullCodeCoversTheCodesThatMeanNoRoomAndNoOthers(t *testing.T) {
	cases := []struct {
		name string
		code int
		want bool
	}{
		{name: "SQLITE_FULL", code: 13, want: true},
		{name: "SQLITE_FULL in some future extended form", code: 13 | (1 << 8), want: true},
		{name: "SQLITE_IOERR_SHMSIZE, a -shm that could not be grown", code: 4874, want: true},

		{name: "SQLITE_OK", code: 0, want: false},
		{name: "SQLITE_BUSY", code: 5, want: false},
		{name: "SQLITE_READONLY", code: 8, want: false},
		{name: "SQLITE_READONLY_DIRECTORY", code: 1544, want: false},
		{name: "SQLITE_IOERR, an unqualified I/O failure", code: 10, want: false},
		{name: "SQLITE_CORRUPT", code: 11, want: false},
		{name: "SQLITE_CANTOPEN", code: 14, want: false},
		{name: "SQLITE_NOTADB", code: 26, want: false},
		{name: "SQLITE_IOERR_READ", code: 266, want: false},
		{name: "SQLITE_IOERR_WRITE, which the Unix VFS returns only when it was not ENOSPC", code: 778, want: false},
		{name: "SQLITE_IOERR_FSYNC, far more often a failing drive", code: 1034, want: false},
		{name: "SQLITE_IOERR_DIR_FSYNC", code: 1290, want: false},
		{name: "SQLITE_IOERR_TRUNCATE, which frees space rather than needing it", code: 1546, want: false},
		{name: "SQLITE_IOERR_DELETE", code: 2570, want: false},
		{name: "SQLITE_IOERR_NOMEM, which is memory and not disk", code: 3082, want: false},
		{name: "SQLITE_IOERR_SHMOPEN, a -shm that could not be opened", code: 4618, want: false},
		{name: "SQLITE_IOERR_SHMLOCK", code: 5130, want: false},
		{name: "SQLITE_IOERR_SHMMAP, a mapping failure rather than a sizing one", code: 5386, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := diskFullCode(tc.code); got != tc.want {
				t.Errorf("diskFullCode(%d) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

// TestNoSpaceDatabaseErrorPrescribesSpaceAndNotPermission is what keeps the
// fourth blocker from inheriting the other three's remedy.
//
// A chmod cannot clear a full disk. The whole reason space is a separate blocker
// with a separate constructor is that the sentence has to change, and a test
// that only checked the code would not notice if the words drifted back.
func TestNoSpaceDatabaseErrorPrescribesSpaceAndNotPermission(t *testing.T) {
	const path = "/repo/.git/mindrail/mindrail.db"
	err := noSpaceDatabaseError(path, "/repo/.git/mindrail", errors.New("the filesystem reports 0 bytes available"))

	if !errors.Is(err, ErrDiskFull) {
		t.Fatalf("noSpaceDatabaseError = %v, want it to unwrap to ErrDiskFull", err)
	}
	if errors.Is(err, ErrReadOnly) {
		t.Errorf("noSpaceDatabaseError = %v, want it not to read as a permission refusal", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("noSpaceDatabaseError carries no domain payload: %v", err)
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
	}
	if got := app.ExitCode(err); got != app.ExitUnavailable {
		t.Errorf("ExitCode = %d, want %d (decision D-03)", got, app.ExitUnavailable)
	}

	remedy := strings.Join(payload.NextAction, " ")
	if want := "free space on the filesystem holding " + path; !strings.Contains(remedy, want) {
		t.Errorf("NextAction = %q, want it to contain %q", payload.NextAction, want)
	}
	if strings.Contains(remedy, "permission") || strings.Contains(remedy, "chmod") {
		t.Errorf("NextAction = %q, which prescribes a permission change for a full disk", payload.NextAction)
	}
	if strings.Contains(remedy, "mindrail init") {
		t.Errorf("NextAction = %q, which sends the user to the command that fails on this condition", payload.NextAction)
	}
}
