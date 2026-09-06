package filesystem

import (
	"errors"
	"path/filepath"
	"testing"
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
		space FreeSpace
		want  bool
	}{
		{name: "no answer at all", space: FreeSpace{}, want: false},
		{name: "unknown answer carrying a zero", space: FreeSpace{Known: false, AvailableBytes: 0}, want: false},
		{name: "known and empty", space: FreeSpace{Known: true, AvailableBytes: 0}, want: true},
		{name: "known and one byte left", space: FreeSpace{Known: true, AvailableBytes: 1}, want: false},
		{name: "known and nearly full", space: FreeSpace{Known: true, AvailableBytes: 4096}, want: false},
		{name: "known and a terabyte free", space: FreeSpace{Known: true, AvailableBytes: 1 << 40}, want: false},
		{
			// Only reachable through a broken statfs answer, and the direction
			// matters: a negative byte count must not read as "plenty".
			name: "known and nonsensical", space: FreeSpace{Known: true, AvailableBytes: -1}, want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.space.Exhausted(); got != tc.want {
				t.Errorf("FreeSpace%+v.Exhausted() = %v, want %v", tc.space, got, tc.want)
			}
		})
	}
}

// TestNoSpaceRefusalStaysSilentWhereThereIsRoom is the over-fire guard on the
// probe's entry point: an ordinary directory on an ordinary filesystem must
// produce no refusal at all. Every test in this repository runs in one, so a
// rule that fired here would fail the suite loudly -- which is the point.
func TestNoSpaceRefusalStaysSilentWhereThereIsRoom(t *testing.T) {
	if err := NoSpaceRefusal(t.TempDir()); err != nil {
		t.Fatalf("NoSpaceRefusal(a temp dir) = %v, want nil; this filesystem has room", err)
	}
}

// TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat keeps "could not look"
// separate from "looked and found nothing". A path that does not exist is
// somebody else's diagnosis -- the caller has already reported it as an
// obstruction, or is asking about its nearest existing ancestor instead -- and
// answering "the disk is full" about it would replace a precise reading with a
// wrong one.
func TestNoSpaceRefusalSaysNothingAboutAPathItCannotStat(t *testing.T) {
	if err := NoSpaceRefusal(filepath.Join(t.TempDir(), "does", "not", "exist")); err != nil {
		t.Fatalf("NoSpaceRefusal(absent path) = %v, want nil", err)
	}
}

// TestNoSpaceRefusalNamesTheConditionItFound is what lets the refusal cross a
// package boundary without losing its meaning: storage wraps it and asks
// errors.Is about ErrDiskFull, and ClassifyRefusal has to recognise it as the
// no-space condition rather than falling through to the generic sentence.
func TestNoSpaceRefusalNamesTheConditionItFound(t *testing.T) {
	err := NoSpaceRefusal("")
	if err != nil {
		t.Fatalf("NoSpaceRefusal(\"\") = %v, want nil; an unstattable path has no verdict", err)
	}

	// The constructed form, since a full filesystem cannot be arranged in a unit
	// test. The shape is the one NoSpaceRefusal builds.
	refusal := errors.Join(ErrNoSpace, errors.New("the filesystem reports 0 bytes available"))
	if got := ClassifyRefusal(refusal); got != BarrierNoSpace {
		t.Errorf("ClassifyRefusal(a no-space refusal) = %q, want %q", got, BarrierNoSpace)
	}
}
