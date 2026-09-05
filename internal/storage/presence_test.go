package storage_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// TestProbeSeparatesAbsentFromOccupied is finding S3's storage half. Both
// conditions stop every command that needs the database, but only one of them
// is what `mindrail init` fixes, and a caller that cannot tell them apart
// recommends init for a directory that init will trip over on the next run too.
func TestProbeSeparatesAbsentFromOccupied(t *testing.T) {
	dir := t.TempDir()

	absent := filepath.Join(dir, "absent.db")
	file := filepath.Join(dir, "present.db")
	occupied := filepath.Join(dir, "occupied.db")

	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("WriteFile = %v, want no error", err)
	}
	if err := os.Mkdir(occupied, 0o700); err != nil {
		t.Fatalf("Mkdir = %v, want no error", err)
	}

	tests := []struct {
		name     string
		path     string
		want     storage.Presence
		wantErr  bool
		wantSaid string
	}{
		{name: "nothing there", path: absent, want: storage.PresenceAbsent},
		{name: "a regular file", path: file, want: storage.PresenceFile},
		{
			name:     "a directory on the path",
			path:     occupied,
			want:     storage.PresenceUnusable,
			wantErr:  true,
			wantSaid: "directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := storage.Probe(tt.path)
			if got != tt.want {
				t.Errorf("Probe(%q) = %q, want %q", tt.path, got, tt.want)
			}
			if tt.wantErr != (err != nil) {
				t.Fatalf("Probe(%q) error = %v, want error: %v", tt.path, err, tt.wantErr)
			}
			if !tt.wantErr {
				return
			}

			if !errors.Is(err, storage.ErrNotDatabaseFile) {
				t.Errorf("Probe(%q) = %v, want errors.Is(err, ErrNotDatabaseFile)", tt.path, err)
			}
			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
			}
			if !strings.Contains(payload.Why, tt.wantSaid) {
				t.Errorf("Why = %q, want it to mention %q", payload.Why, tt.wantSaid)
			}
			if payload.Metadata["path_state"] != string(storage.PresenceUnusable) {
				t.Errorf("metadata path_state = %q, want %q",
					payload.Metadata["path_state"], storage.PresenceUnusable)
			}
		})
	}
}

// TestOpenOnAnOccupiedPathReportsTheObstruction is the same distinction seen
// through Open, in both modes. The remedy has to name the obstruction: a
// next_action of "mindrail init" alone is the loop the audit found, where init
// exits 4, tells the user to run init, and exits 4 again.
func TestOpenOnAnOccupiedPathReportsTheObstruction(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		name := "writable"
		if readOnly {
			name = "read-only"
		}

		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mindrail.db")
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatalf("Mkdir = %v, want no error", err)
			}

			db, err := storage.Open(t.Context(), storage.Options{Path: path, ReadOnly: readOnly})
			if err == nil {
				_ = db.Close()
				t.Fatal("Open(directory) = nil error, want ErrNotDatabaseFile")
			}
			if !errors.Is(err, storage.ErrNotDatabaseFile) {
				t.Errorf("Open(directory) = %v, want errors.Is(err, ErrNotDatabaseFile)", err)
			}
			if !errors.Is(err, storage.ErrOpenFailed) {
				t.Errorf("Open(directory) = %v, want errors.Is(err, ErrOpenFailed)", err)
			}

			payload, ok := app.PayloadOf(err)
			if !ok {
				t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
			}
			if payload.Code != app.CodeRuntimeDBUnavailable {
				t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeRuntimeDBUnavailable)
			}
			if len(payload.NextAction) == 0 {
				t.Fatal("payload.NextAction is empty, want a remedy")
			}
			for _, next := range payload.NextAction {
				if !strings.Contains(next, path) {
					continue
				}
				return // a remedy that names the obstructed path
			}
			t.Errorf("NextAction = %q, want a remedy that names %q rather than init alone",
				payload.NextAction, path)
		})
	}
}

// TestOpenReadOnlyDistinguishesAbsenceFromObstruction checks the two errors do
// not collapse into one another: absence is the initialisable state and carries
// the init remedy, obstruction is not and does not.
func TestOpenReadOnlyDistinguishesAbsenceFromObstruction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")

	db, err := storage.Open(t.Context(), storage.Options{Path: path, ReadOnly: true})
	if err == nil {
		_ = db.Close()
		t.Fatal("Open(read-only, absent) = nil error, want ErrOpenFailed")
	}
	if errors.Is(err, storage.ErrNotDatabaseFile) {
		t.Errorf("Open(read-only, absent) = %v, want it not to claim an obstruction", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
	}
	if payload.Metadata["path_state"] != string(storage.PresenceAbsent) {
		t.Errorf("metadata path_state = %q, want %q", payload.Metadata["path_state"], storage.PresenceAbsent)
	}
	if !strings.Contains(strings.Join(payload.NextAction, " "), "mindrail init") {
		t.Errorf("NextAction = %q, want `mindrail init`; an absent database is what init creates",
			payload.NextAction)
	}
}

// TestProbeDoesNotCreateAnything is decision D-01 restated for the probe: the
// question "is there a database here?" must never be answered by making one.
func TestProbeDoesNotCreateAnything(t *testing.T) {
	dir := t.TempDir()

	if got, err := storage.Probe(filepath.Join(dir, "mindrail.db")); got != storage.PresenceAbsent || err != nil {
		t.Fatalf("Probe = %q, %v; want %q, nil", got, err, storage.PresenceAbsent)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir = %v, want no error", err)
	}
	if len(entries) != 0 {
		t.Errorf("directory contains %d entries after Probe, want 0", len(entries))
	}
}

// TestProbeReportsADanglingSymlinkAsOccupied covers the one case where "the
// path resolves to nothing" and "nothing is at the path" come apart. Reporting
// it as absent would recommend init for a link init cannot write through.
func TestProbeReportsADanglingSymlinkAsOccupied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mindrail.db")
	if err := os.Symlink(filepath.Join(dir, "gone.db"), path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := storage.Probe(path)
	if got != storage.PresenceUnusable {
		t.Errorf("Probe(dangling symlink) = %q, want %q", got, storage.PresenceUnusable)
	}
	if !errors.Is(err, storage.ErrNotDatabaseFile) {
		t.Errorf("Probe(dangling symlink) = %v, want errors.Is(err, ErrNotDatabaseFile)", err)
	}
}

// TestProbeFollowsAWorkingSymlink is the other side of it: decision D-32 allows
// symlinks inside the root, so a link to a real database file is a database.
func TestProbeFollowsAWorkingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.db")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatalf("WriteFile = %v, want no error", err)
	}

	path := filepath.Join(dir, "mindrail.db")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := storage.Probe(path)
	if got != storage.PresenceFile || err != nil {
		t.Errorf("Probe(symlink to a file) = %q, %v; want %q, nil", got, err, storage.PresenceFile)
	}
}

func TestProbeRejectsRelativePath(t *testing.T) {
	got, err := storage.Probe("mindrail.db")
	if got != storage.PresenceUnusable {
		t.Errorf("Probe(relative) = %q, want %q", got, storage.PresenceUnusable)
	}
	if !errors.Is(err, storage.ErrOpenFailed) {
		t.Errorf("Probe(relative) = %v, want errors.Is(err, ErrOpenFailed)", err)
	}
}
