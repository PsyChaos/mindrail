package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
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

// TestRoomSQLiteWasRefusedCountsWhatSQLiteAsksFor pins the floor the exoneration
// above is measured against.
//
// Reading "more than zero bytes" as room was this classification over-firing
// into a new wrong claim: with four kilobytes left the report said a size limit
// was refusing the write and that freeing space would not change it, on a
// filesystem where freeing two megabytes cleared it at once.
func TestRoomSQLiteWasRefusedCountsWhatSQLiteAsksFor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mindrail.db")

	if got := roomSQLiteWasRefused(path); got != shmSize {
		t.Errorf("with no write-ahead log, roomSQLiteWasRefused = %d, want the shared-memory index's %d", got, shmSize)
	}

	const logSize = 74 * 1024
	if err := os.WriteFile(path+"-wal", make([]byte, logSize), 0o600); err != nil {
		t.Fatalf("write a fixture log: %v", err)
	}
	if got := roomSQLiteWasRefused(path); got != shmSize+logSize {
		t.Errorf("with a %d-byte log, roomSQLiteWasRefused = %d, want %d", logSize, got, shmSize+logSize)
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
