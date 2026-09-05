//go:build linux || darwin

package storage

import (
	"path/filepath"
	"testing"
)

// TestStatFreeSpaceAnswersForARealDirectory is the wiring test the decision
// tests cannot be: freeSpace.exhausted() is exact arithmetic, and it is worth
// nothing if the field it reads is always zero.
//
// A syscall that came back wrong -- a field read at the wrong width, a block
// size multiplied the wrong way -- would show up here as an unknown answer or as
// zero bytes free on the filesystem this test is running from, and zero bytes
// free is the condition that reports a healthy repository unwritable.
func TestStatFreeSpaceAnswersForARealDirectory(t *testing.T) {
	space := statFreeSpace(t.TempDir())

	if !space.Known {
		t.Fatal("statFreeSpace(a temp dir) = unknown; statfs is supposed to work on this platform")
	}
	if space.AvailableBytes <= 0 {
		t.Fatalf("statFreeSpace(a temp dir).AvailableBytes = %d, want a positive count; "+
			"the filesystem the tests run on is not full", space.AvailableBytes)
	}
	if space.exhausted() {
		t.Fatal("statFreeSpace(a temp dir).exhausted() = true on a working filesystem")
	}
}

// TestStatFreeSpaceLeavesAnUnstattableDirectoryUnknown keeps the failure of the
// syscall out of the answer. An error from statfs means the question was not
// answered, and an unanswered question must not arrive at the report as "the
// disk is full".
func TestStatFreeSpaceLeavesAnUnstattableDirectoryUnknown(t *testing.T) {
	space := statFreeSpace(filepath.Join(t.TempDir(), "absent"))

	if space.Known {
		t.Errorf("statFreeSpace(absent) = %+v, want an unknown answer", space)
	}
	if space.exhausted() {
		t.Error("statFreeSpace(absent).exhausted() = true; a failed syscall is not a full disk")
	}
}
