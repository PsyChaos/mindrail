package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

// TestDiskFullOpenFailureAsksTheFilesystemBeforeBlamingIt is finding F03's open
// path, which nothing reached.
//
// The write path had a test; this one had none, and it was the half the finding
// was actually reported against. Everything in it could be reverted in silence:
// replacing its four remedies with "free space on the filesystem holding <path>"
// — the exact sentence F03 exists to prevent — left the whole suite green, as
// did stamping the metadata back to "disk_full" and hardcoding the free-space
// reading to zero.
//
// It is a unit test because the condition needs a filesystem that refuses a
// 32 KiB allocation while reporting gigabytes free, and the classification is a
// pure function of a path and a reading. The scenario tests beside it — a real
// full tmpfs, a real RLIMIT_FSIZE — prove the driver still produces the codes
// that arrive here.
func TestDiskFullOpenFailureAsksTheFilesystemBeforeBlamingIt(t *testing.T) {
	cause := errors.New("disk I/O error (4874)")

	t.Run("a filesystem with room names the limit and not the disk", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "mindrail.db")
		requireRoomForTheFixture(t, filepath.Dir(path))

		payload := payloadOf(t, diskFullOpenFailure(path, cause))

		if payload.Code != app.CodeRuntimePathUnwritable {
			t.Errorf("code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
		}
		if payload.Metadata["condition"] != "size_limit" {
			t.Errorf("condition = %q, want %q", payload.Metadata["condition"], "size_limit")
		}
		if strings.Contains(payload.Why, "is full") {
			t.Errorf("why = %q, which asserts a full disk about a filesystem with room", payload.Why)
		}

		remedy := strings.Join(payload.NextAction, "\n")
		if strings.Contains(remedy, "free space") {
			t.Errorf("next_action = %q, which asks for space on a filesystem that has it", payload.NextAction)
		}
		// The three limits that produce this shape. Naming none of them leaves
		// the reader with a condition and no way to look for its cause.
		for _, limit := range []string{"ulimit -f", "quota", "max_page_count"} {
			if !strings.Contains(remedy, limit) {
				t.Errorf("next_action = %q, which never mentions %s", payload.NextAction, limit)
			}
		}

		available, err := strconv.ParseInt(payload.Metadata["available_bytes"], 10, 64)
		if err != nil {
			t.Fatalf("available_bytes = %q, which is not a number", payload.Metadata["available_bytes"])
		}
		if available <= 0 {
			t.Errorf("available_bytes = %d over a temp directory with room; the claim is decided from this reading", available)
		}
		if !strings.Contains(payload.Why, payload.Metadata["available_bytes"]) {
			t.Errorf("why = %q, which does not quote the reading the classification was decided from", payload.Why)
		}
	})

	t.Run("a filesystem that cannot be read is not exonerated", func(t *testing.T) {
		// statfs answers nothing for a directory that is not there, and an
		// unknown reading is not evidence of room: the disk-full sentence stands,
		// which is the branch the doc comment calls load-bearing.
		path := filepath.Join(t.TempDir(), "gone", "mindrail.db")

		payload := payloadOf(t, diskFullOpenFailure(path, cause))
		if payload.Metadata["condition"] != "disk_full" {
			t.Errorf("condition = %q, want %q; an unprobeable filesystem must not be reported as having room",
				payload.Metadata["condition"], "disk_full")
		}
		if !strings.Contains(strings.Join(payload.NextAction, "\n"), "free space") {
			t.Errorf("next_action = %q, want it to ask for space", payload.NextAction)
		}
	})
}

// wantShmFloor is the floor, spelled out rather than referred to.
//
// Writing `want shmSize` compares the function to the constant it reads, which
// is an equation and not an assertion: setting shmSize to four kilobytes — the
// literal over-fire this classification was corrected for — passed the whole
// suite. The number here is SQLite's, and
// TestTheSharedMemoryIndexIsStillTheSizeWeBudgetFor proves it is still SQLite's.
const wantShmFloor = 32 * 1024

// requireRoomForTheFixture keeps the rows below from reporting a defect that is
// really a full disk under the test.
//
// Both of them work by putting a database on the temporary filesystem and asking
// what the classification says about it, which presumes that filesystem has
// room. On one with 24 KiB left the first row fails with `condition = disk_full,
// want size_limit` — the correct answer for that machine, reported as a
// production defect. A test that cannot arrange its own premise says so instead.
func requireRoomForTheFixture(t *testing.T, dir string) {
	t.Helper()

	// The fixture writes a 74 KiB log and needs the shared-memory index's 32 KiB
	// on top; the rest is headroom so the reading is unambiguous.
	const needed = 1 << 20

	free := filesystem.ProbeFreeSpace(dir)
	if free.Known && free.AvailableBytes < needed {
		t.Skipf("the filesystem under %s reports %d bytes free; this row needs at least %d to say anything",
			dir, free.AvailableBytes, needed)
	}
}

// TestRoomSQLiteWasRefusedCountsWhatSQLiteAsksFor pins the floor the exoneration
// above is measured against.
//
// Reading "more than zero bytes" as room was this classification over-firing
// into a new wrong claim: with four kilobytes left the report said a size limit
// was refusing the write and that freeing space would not change it, on a
// filesystem where freeing two megabytes cleared it at once.
func TestRoomSQLiteWasRefusedCountsWhatSQLiteAsksFor(t *testing.T) {
	dir := t.TempDir()
	requireRoomForTheFixture(t, dir)
	path := filepath.Join(dir, "mindrail.db")

	if got := roomSQLiteWasRefused(path); got != wantShmFloor {
		t.Errorf("with no write-ahead log, roomSQLiteWasRefused = %d, want the shared-memory index's %d",
			got, wantShmFloor)
	}

	const logSize = 74 * 1024
	if err := os.WriteFile(path+"-wal", make([]byte, logSize), 0o600); err != nil {
		t.Fatalf("write a fixture log: %v", err)
	}
	if got := roomSQLiteWasRefused(path); got != wantShmFloor+logSize {
		t.Errorf("with a %d-byte log, roomSQLiteWasRefused = %d, want %d",
			logSize, got, wantShmFloor+logSize)
	}
}

// TestTheSharedMemoryIndexIsStillTheSizeWeBudgetFor anchors the constant to the
// driver rather than to a comment.
//
// shmSize is a fact about SQLite, not a policy of this package, and the whole
// exoneration rests on it: a filesystem with less than this much left cannot
// have supplied the allocation SQLITE_IOERR_SHMSIZE is the failure of. A build
// against a SQLite whose `-shm` is a different size would make the floor wrong
// in silence, and the file is on disk to be measured.
func TestTheSharedMemoryIndexIsStillTheSizeWeBudgetFor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")

	db, err := Open(t.Context(), Options{Path: path})
	if err != nil {
		t.Fatalf("Open = %v, want no error", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(t.Context(), `CREATE TABLE probe (x INTEGER)`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}

	info, err := os.Stat(path + "-shm")
	if err != nil {
		t.Skipf("this build keeps no shared-memory file beside the database: %v", err)
	}
	if info.Size() != shmSize {
		t.Errorf("the shared-memory index is %d bytes, but the classification budgets for %d; "+
			"a filesystem is exonerated on the strength of this number", info.Size(), shmSize)
	}
}

func payloadOf(t *testing.T, err error) app.ErrorPayload {
	t.Helper()

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error %v carries no domain payload", err)
	}
	return payload
}
