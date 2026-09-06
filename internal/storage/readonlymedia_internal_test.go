//go:build unix

package storage

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

// TestConditionBlockerLetsTheConditionOverrideThePartItMet is finding E4 at the
// point the decision is made.
//
// The blockers that name a part of the footprint prescribe a chmod on that part.
// That is right for mode bits and wrong for a mount: on a read-only filesystem
// every mode bit is already correct and `chmod` fails with the same EROFS that
// refused the write, so the remedy cannot be carried out at all. The two
// condition-shaped blockers therefore have to win over whichever part met them.
//
// The rows that keep their part are the over-fire guard. A permission failure on
// the `-shm` must stay a permission failure on the `-shm`; if the condition
// branch widened, a repository with one wrong mode bit would be told to remount
// its filesystem.
func TestConditionBlockerLetsTheConditionOverrideThePartItMet(t *testing.T) {
	rofs := fmt.Errorf("access %q: %w", "/mnt/repo/.git/mindrail/mindrail.db", syscall.EROFS)
	nospc := fmt.Errorf("statfs %q: %w", "/mnt/repo/.git/mindrail", filesystem.ErrNoSpace)
	eacces := fmt.Errorf("access %q: %w", "/repo/.git/mindrail/mindrail.db", syscall.EACCES)

	cases := []struct {
		name  string
		part  WriteBlocker
		cause error
		want  WriteBlocker
	}{
		{name: "the database file on a read-only mount", part: BlockerDatabaseFile, cause: rofs, want: BlockerReadOnlyMedia},
		{name: "the -shm on a read-only mount", part: BlockerSidecar, cause: rofs, want: BlockerReadOnlyMedia},
		{name: "the directory on a read-only mount", part: BlockerDirectory, cause: rofs, want: BlockerReadOnlyMedia},
		{name: "the directory on a full filesystem", part: BlockerDirectory, cause: nospc, want: BlockerNoSpace},

		{name: "the database file's own mode bits", part: BlockerDatabaseFile, cause: eacces, want: BlockerDatabaseFile},
		{name: "the -shm's own mode bits", part: BlockerSidecar, cause: eacces, want: BlockerSidecar},
		{name: "the directory's own mode bits", part: BlockerDirectory, cause: eacces, want: BlockerDirectory},
		{
			// Probe has already described what is standing on the path, and that
			// description is not made less true by the state of the filesystem.
			name: "something else occupying the path, on a read-only mount",
			part: BlockerObstruction, cause: rofs, want: BlockerObstruction,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := conditionBlocker(tc.part, tc.cause); got != tc.want {
				t.Errorf("conditionBlocker(%q, %v) = %q, want %q", tc.part, tc.cause, got, tc.want)
			}
		})
	}
}

// TestWriteRefusalErrorPrescribesTheRemedyThatCanBeCarriedOut is the sentence
// half, and the three rows are the three remedies that have nothing in common.
//
// The read-only row is finding E4 in full: what was printed was "restore write
// permission on <db>, <-wal>, <-shm>" for files whose owner-write bit was
// already set, on a filesystem where the chmod itself fails.
func TestWriteRefusalErrorPrescribesTheRemedyThatCanBeCarriedOut(t *testing.T) {
	const path = "/mnt/repo/.git/mindrail/mindrail.db"
	const dir = "/mnt/repo/.git/mindrail"

	cases := []struct {
		name    string
		blocker WriteBlocker
		blocked string
		cause   error
		want    []string
		refused []string
	}{
		{
			name:    "a read-only mount",
			blocker: BlockerReadOnlyMedia,
			blocked: path,
			cause:   fmt.Errorf("access %q: %w", path, syscall.EROFS),
			want:    []string{"remount", "read-write", "MINDRAIL_RUNTIME_DIR"},
			refused: []string{"restore write permission", "chmod", "free space"},
		},
		{
			name:    "a full filesystem",
			blocker: BlockerNoSpace,
			blocked: dir,
			cause:   fmt.Errorf("statfs %q: %w", dir, filesystem.ErrNoSpace),
			want:    []string{"free space on the filesystem holding " + path},
			refused: []string{"restore write permission", "chmod", "remount"},
		},
		{
			name:    "mode bits that refuse the write",
			blocker: BlockerDatabaseFile,
			blocked: path,
			cause:   fmt.Errorf("access %q: %w", path, syscall.EACCES),
			want:    []string{"restore write permission"},
			refused: []string{"free space", "remount"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := writeRefusalError(path, tc.blocker, tc.blocked, tc.cause)

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("writeRefusalError = %v, which carries no domain payload", err)
			}
			if payload.Code != app.CodeRuntimePathUnwritable {
				t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
			}
			if app.ExitCode(err) != app.ExitUnavailable {
				t.Errorf("ExitCode = %d, want %d (decision D-03)", app.ExitCode(err), app.ExitUnavailable)
			}

			remedy := strings.Join(payload.NextAction, " ")
			for _, want := range tc.want {
				if !strings.Contains(remedy, want) {
					t.Errorf("NextAction = %q, want it to mention %q", payload.NextAction, want)
				}
			}
			for _, refused := range tc.refused {
				if strings.Contains(remedy, refused) {
					t.Errorf("NextAction = %q, which prescribes %q for %s; that cannot clear it",
						payload.NextAction, refused, tc.name)
				}
			}
		})
	}
}

// TestReadOnlyMediaKeepsBothSentinels is what lets the new condition travel
// through code that predates it.
//
// Callers already branch on ErrReadOnly, and a database on a read-only mount is
// read-only by any reading, so that has to keep answering yes. What is added is
// the filesystem's own sentinel, which is how a caller distinguishes the mount
// from the mode bits without parsing a sentence. ErrDiskFull must stay absent:
// freeing space clears nothing here.
func TestReadOnlyMediaKeepsBothSentinels(t *testing.T) {
	const path = "/mnt/repo/.git/mindrail/mindrail.db"
	err := readOnlyMediaDatabaseError(path, path, fmt.Errorf("access %q: %w", path, syscall.EROFS))

	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("readOnlyMediaDatabaseError = %v, want errors.Is(err, ErrReadOnly)", err)
	}
	if !errors.Is(err, filesystem.ErrReadOnlyMedia) {
		t.Errorf("readOnlyMediaDatabaseError = %v, want it to name the mount, not only the database", err)
	}
	if errors.Is(err, ErrDiskFull) {
		t.Errorf("readOnlyMediaDatabaseError = %v, want it not to read as a full disk; "+
			"freeing space does not clear a read-only mount", err)
	}
}
