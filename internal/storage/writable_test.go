package storage_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// requireModeBitsAreEnforced skips the permission cases where they cannot be
// observed. Running as root defeats access(2): every mode is writable to uid 0,
// so a test that chmods a file and expects a refusal would assert nothing.
func requireModeBitsAreEnforced(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root; access(2) reports every path writable")
	}
}

// initialisedDatabase builds what `mindrail init` leaves behind: a real WAL
// database, cleanly closed, so the sidecars are gone and the directory holds the
// single file a healthy repository has.
func initialisedDatabase(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "mindrail.db")
	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		t.Fatalf("storage.Open = %v, want no error", err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE probe (x INTEGER)`); err != nil {
		t.Fatalf("CREATE TABLE = %v, want no error", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close = %v, want no error", err)
	}
	return path
}

// TestProbeWriteAccessPassesAHealthyDatabase is the over-fire guard the whole
// check hangs on. Every condition below is worth detecting only if the ordinary
// case -- an initialised repository nobody has touched -- comes back writable;
// a probe that fired here would brick every working installation, which is
// exactly how an earlier pass at this finding failed.
func TestProbeWriteAccessPassesAHealthyDatabase(t *testing.T) {
	path := initialisedDatabase(t)

	got := storage.ProbeWriteAccess(path)
	if !got.Writable {
		t.Fatalf("ProbeWriteAccess(healthy) = %+v, want Writable", got)
	}
	if got.Err != nil {
		t.Errorf("Err = %v, want nil on a writable database", got.Err)
	}
	if got.Blocker != storage.BlockerNone {
		t.Errorf("Blocker = %q, want %q", got.Blocker, storage.BlockerNone)
	}
}

// TestProbeWriteAccessPassesAnUninitialisedRepository is the second over-fire
// guard: "no database here yet" is decision D-01's reportable state, not an
// unwritable path. A probe that answered "unwritable" for it would turn every
// fresh clone into a broken installation.
func TestProbeWriteAccessPassesAnUninitialisedRepository(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")

	got := storage.ProbeWriteAccess(path)
	if !got.Writable {
		t.Fatalf("ProbeWriteAccess(absent) = %+v, want Writable; init has not run yet", got)
	}
}

// TestProbeWriteAccessIgnoresASiblingDirectory is the adjacent condition this
// check must not detect. A pass-2 fix for an unwritable runtime path also fired
// on the cache directory and bricked a working repository; the answer here is
// about one database file and the directory that holds it, and an unusable
// cache directory beside it is somebody else's finding.
func TestProbeWriteAccessIgnoresASiblingDirectory(t *testing.T) {
	requireModeBitsAreEnforced(t)

	path := initialisedDatabase(t)
	cache := filepath.Join(filepath.Dir(path), "cache")
	if err := os.Mkdir(cache, 0o700); err != nil {
		t.Fatalf("Mkdir = %v, want no error", err)
	}
	if err := os.Chmod(cache, 0o500); err != nil {
		t.Fatalf("Chmod = %v, want no error", err)
	}
	t.Cleanup(func() { _ = os.Chmod(cache, 0o700) })

	if got := storage.ProbeWriteAccess(path); !got.Writable {
		t.Errorf("ProbeWriteAccess = %+v, want Writable; an unwritable cache directory is not this database's fault", got)
	}
}

// TestProbeWriteAccessDetectsAReadOnlyDatabaseFile is finding W2's core. The
// file opens, every pragma reads back correct, and `doctor` called the whole
// installation healthy -- while `mindrail init` in the same directory failed on
// `attempt to write a readonly database (8)` and sent the user back to doctor.
func TestProbeWriteAccessDetectsAReadOnlyDatabaseFile(t *testing.T) {
	requireModeBitsAreEnforced(t)

	path := initialisedDatabase(t)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("Chmod = %v, want no error", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	got := storage.ProbeWriteAccess(path)
	if got.Writable {
		t.Fatalf("ProbeWriteAccess(0444 database) = %+v, want it refused", got)
	}
	if got.Blocker != storage.BlockerDatabaseFile {
		t.Errorf("Blocker = %q, want %q", got.Blocker, storage.BlockerDatabaseFile)
	}
	if got.Blocked != path {
		t.Errorf("Blocked = %q, want %q", got.Blocked, path)
	}

	payload := assertUnwritablePayload(t, got.Err)
	if !strings.Contains(strings.Join(payload.NextAction, " "), path) {
		t.Errorf("NextAction = %q, want a remedy naming %q", payload.NextAction, path)
	}
}

// TestProbeWriteAccessDetectsADirectoryThatCannotHoldTheWAL is the second shape
// of the same fault, and it is genuinely distinct: the database file's own mode
// bits are fine, and SQLite still cannot write because it has nowhere to put
// `-wal` and `-shm`. A remedy aimed at the file would not clear it.
func TestProbeWriteAccessDetectsADirectoryThatCannotHoldTheWAL(t *testing.T) {
	requireModeBitsAreEnforced(t)

	path := initialisedDatabase(t)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod = %v, want no error", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	got := storage.ProbeWriteAccess(path)
	if got.Writable {
		t.Fatalf("ProbeWriteAccess(0500 directory) = %+v, want it refused", got)
	}
	if got.Blocker != storage.BlockerDirectory {
		t.Errorf("Blocker = %q, want %q", got.Blocker, storage.BlockerDirectory)
	}

	payload := assertUnwritablePayload(t, got.Err)
	if !strings.Contains(strings.Join(payload.NextAction, " "), dir) {
		t.Errorf("NextAction = %q, want a remedy naming the directory %q", payload.NextAction, dir)
	}
}

// TestProbeWriteAccessIgnoresWriteAheadLogPermissions is the third over-fire
// guard, and it is here because the probe got this wrong on the first attempt.
//
// A `-wal` whose mode bits refuse writes looks like an obvious blocker: SQLite
// writes the log before it writes the database. It is not one. SQLite brings the
// file's mode into line with the database's and writes through it, and it does
// so even when the directory accepts no new entries. A probe that called either
// of these databases unwritable would fail a repository that works.
func TestProbeWriteAccessIgnoresWriteAheadLogPermissions(t *testing.T) {
	requireModeBitsAreEnforced(t)

	tests := []struct {
		name          string
		lockDirectory bool
	}{
		{name: "writable directory"},
		{name: "read-only directory", lockDirectory: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := initialisedDatabase(t)
			for _, suffix := range []string{"-wal", "-shm"} {
				if err := os.WriteFile(path+suffix, nil, 0o600); err != nil {
					t.Fatalf("WriteFile = %v, want no error", err)
				}
			}
			chmodForTest(t, path+"-wal", 0o444, 0o600)
			if tt.lockDirectory {
				chmodForTest(t, filepath.Dir(path), 0o500, 0o700)
			}

			if got := storage.ProbeWriteAccess(path); !got.Writable {
				t.Errorf("ProbeWriteAccess = %+v, want Writable; SQLite repairs a log it cannot write", got)
			}
		})
	}
}

// TestProbeWriteAccessDetectsAReadOnlySharedMemoryIndex is the sidecar that does
// decide the answer, and the one the first draft of this probe missed.
//
// SQLite maps `-shm` read-write and cannot recover from being refused, so every
// write fails with SQLITE_READONLY while the database file's own mode is
// perfectly correct. It is not a hypothetical shape: a `mindrail init` that fails
// against a read-only database leaves an `-shm` behind carrying the same mode,
// so this is the state the user is in on their second attempt.
func TestProbeWriteAccessDetectsAReadOnlySharedMemoryIndex(t *testing.T) {
	requireModeBitsAreEnforced(t)

	path := initialisedDatabase(t)
	shm := path + "-shm"
	if err := os.WriteFile(shm, nil, 0o600); err != nil {
		t.Fatalf("WriteFile = %v, want no error", err)
	}
	chmodForTest(t, shm, 0o444, 0o600)

	got := storage.ProbeWriteAccess(path)
	if got.Writable {
		t.Fatalf("ProbeWriteAccess(read-only -shm) = %+v, want it refused", got)
	}
	if got.Blocker != storage.BlockerSidecar {
		t.Errorf("Blocker = %q, want %q", got.Blocker, storage.BlockerSidecar)
	}
	if got.Blocked != shm {
		t.Errorf("Blocked = %q, want %q", got.Blocked, shm)
	}

	payload := assertUnwritablePayload(t, got.Err)
	if !strings.Contains(strings.Join(payload.NextAction, " "), shm) {
		t.Errorf("NextAction = %q, want a remedy naming %q", payload.NextAction, shm)
	}
}

// TestTheUnwritableRemedyNamesEveryRefusedPath is the loop guard. A remedy that
// clears half the condition is a slower spelling of the loop it replaced: the
// user chmods the database file, re-runs, and fails on the `-shm` the first run
// created at the same mode. One remedy, one command, condition cleared.
func TestTheUnwritableRemedyNamesEveryRefusedPath(t *testing.T) {
	requireModeBitsAreEnforced(t)

	path := initialisedDatabase(t)
	shm := path + "-shm"
	if err := os.WriteFile(shm, nil, 0o600); err != nil {
		t.Fatalf("WriteFile = %v, want no error", err)
	}
	chmodForTest(t, path, 0o444, 0o644)
	chmodForTest(t, shm, 0o444, 0o600)

	got := storage.ProbeWriteAccess(path)
	payload := assertUnwritablePayload(t, got.Err)
	remedy := strings.Join(payload.NextAction, " ")
	for _, want := range []string{path, shm} {
		if !strings.Contains(remedy, want) {
			t.Errorf("NextAction = %q, want it to name %q; a partial remedy is another failed run", payload.NextAction, want)
		}
	}

	// Carry it out, exactly as written, and confirm the condition is gone.
	for _, candidate := range storage.RefusedWritePaths(path) {
		info, err := os.Stat(candidate)
		if err != nil {
			t.Fatalf("Stat(%q) = %v, want no error", candidate, err)
		}
		if err := os.Chmod(candidate, info.Mode().Perm()|0o200); err != nil {
			t.Fatalf("Chmod(%q) = %v, want no error", candidate, err)
		}
	}
	if after := storage.ProbeWriteAccess(path); !after.Writable {
		t.Fatalf("ProbeWriteAccess after the remedy = %+v, want Writable; the remedy has to end the loop", after)
	}
	if !writeReachesDisk(t, path) {
		t.Error("a real write was still refused after the remedy was carried out")
	}
}

// TestProbeWriteAccessPassesAReadOnlyDirectoryWithOpenSidecars is the over-fire
// guard for the directory case. A database another Mindrail process holds open
// has both sidecars on disk, so SQLite has nothing left to create and writes
// reach the disk even from a directory that accepts no new entries. Reporting
// that as unwritable would fail a healthy repository for being in use.
func TestProbeWriteAccessPassesAReadOnlyDirectoryWithOpenSidecars(t *testing.T) {
	requireModeBitsAreEnforced(t)

	path := initialisedDatabase(t)
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.WriteFile(path+suffix, nil, 0o600); err != nil {
			t.Fatalf("WriteFile = %v, want no error", err)
		}
	}
	chmodForTest(t, filepath.Dir(path), 0o500, 0o700)

	if got := storage.ProbeWriteAccess(path); !got.Writable {
		t.Errorf("ProbeWriteAccess = %+v, want Writable; SQLite has nothing left to create here", got)
	}
}

// TestProbeWriteAccessCreatesNothing is decision D-01 restated for this probe.
// The obvious SQLite-level answer -- BEGIN IMMEDIATE, then roll back -- would
// leave `-wal` and `-shm` behind on a cleanly closed database and hold the write
// lock while it did, so doctor would mutate the installation it was diagnosing
// and block the init it was diagnosing it for.
func TestProbeWriteAccessCreatesNothing(t *testing.T) {
	path := initialisedDatabase(t)
	dir := filepath.Dir(path)

	before := dirEntryNames(t, dir)
	if got := storage.ProbeWriteAccess(path); !got.Writable {
		t.Fatalf("ProbeWriteAccess = %+v, want Writable", got)
	}
	after := dirEntryNames(t, dir)

	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Errorf("directory contents changed from %v to %v; the probe wrote something", before, after)
	}
}

// TestProbeWriteAccessReportsAnObstructionAsItself keeps the two conditions
// apart. A directory standing on the database path is not a permission problem,
// and answering it with "restore write permission" would send the user to chmod
// a path no mode change makes into a database.
func TestProbeWriteAccessReportsAnObstructionAsItself(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindrail.db")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir = %v, want no error", err)
	}

	got := storage.ProbeWriteAccess(path)
	if got.Writable {
		t.Fatalf("ProbeWriteAccess(directory) = %+v, want it refused", got)
	}
	if got.Blocker != storage.BlockerObstruction {
		t.Errorf("Blocker = %q, want %q", got.Blocker, storage.BlockerObstruction)
	}
	if !errors.Is(got.Err, storage.ErrNotDatabaseFile) {
		t.Errorf("Err = %v, want errors.Is(err, ErrNotDatabaseFile)", got.Err)
	}
}

// TestProbeWriteAccessAgreesWithSQLite is what keeps the probe honest rather
// than merely plausible. Every case above asserts what the probe says; this one
// asserts that what it says is true, by opening the database for real and
// attempting the write the probe was predicting.
//
// It is the guard against the probe drifting from SQLite: a check that answers
// "writable" for a database SQLite refuses is the finding all over again, and one
// that answers "unwritable" for a database SQLite accepts is the over-fire that
// bricks a working repository.
func TestProbeWriteAccessAgreesWithSQLite(t *testing.T) {
	requireModeBitsAreEnforced(t)

	tests := []struct {
		name    string
		breakIt func(t *testing.T, path string)
	}{
		{name: "healthy", breakIt: func(*testing.T, string) {}},
		{
			name: "read-only database file",
			breakIt: func(t *testing.T, path string) {
				chmodForTest(t, path, 0o444, 0o644)
			},
		},
		{
			name: "read-only directory",
			breakIt: func(t *testing.T, path string) {
				chmodForTest(t, filepath.Dir(path), 0o500, 0o700)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := initialisedDatabase(t)
			tt.breakIt(t, path)
			assertProbeMatchesSQLite(t, path)
		})
	}
}

// TestProbeWriteAccessAgreesWithSQLiteAcrossTheSidecarMatrix is the same
// agreement over every combination of sidecar and directory mode, because this
// is where guessing costs the most.
//
// Two of these eight are counter-intuitive and both were established by running
// them rather than by reasoning: an unwritable `-wal` never refuses a write, and
// an unwritable `-shm` always does, directory mode notwithstanding. Any future
// change to the probe that "tidies up" either asymmetry fails here.
func TestProbeWriteAccessAgreesWithSQLiteAcrossTheSidecarMatrix(t *testing.T) {
	requireModeBitsAreEnforced(t)

	modes := []os.FileMode{0o644, 0o444}
	dirs := []os.FileMode{0o700, 0o500}

	for _, wal := range modes {
		for _, shm := range modes {
			for _, dir := range dirs {
				name := fmt.Sprintf("wal_%o/shm_%o/dir_%o", wal.Perm(), shm.Perm(), dir.Perm())
				t.Run(name, func(t *testing.T) {
					path := initialisedDatabase(t)
					for _, suffix := range []string{"-wal", "-shm"} {
						if err := os.WriteFile(path+suffix, nil, 0o600); err != nil {
							t.Fatalf("WriteFile = %v, want no error", err)
						}
					}
					chmodForTest(t, path+"-wal", wal, 0o600)
					chmodForTest(t, path+"-shm", shm, 0o600)
					chmodForTest(t, filepath.Dir(path), dir, 0o700)

					assertProbeMatchesSQLite(t, path)
				})
			}
		}
	}
}

// assertProbeMatchesSQLite predicts the verdict and then performs the write.
func assertProbeMatchesSQLite(t *testing.T, path string) {
	t.Helper()

	predicted := storage.ProbeWriteAccess(path).Writable
	actual := writeReachesDisk(t, path)
	if predicted == actual {
		return
	}

	outcome := "was refused"
	if actual {
		outcome = "succeeded"
	}
	t.Errorf("ProbeWriteAccess said Writable=%v, but a real write %s", predicted, outcome)
}

// writeReachesDisk opens the database the way a writing command does and reports
// whether a write actually lands. A refused open counts as a refused write:
// either way the command cannot record anything.
func writeReachesDisk(t *testing.T, path string) bool {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: path})
	if err != nil {
		return false
	}
	defer func() { _ = db.Close() }()

	_, err = db.ExecContext(t.Context(), `INSERT INTO probe (x) VALUES (1)`)
	return err == nil
}

// chmodForTest sets mode for the duration of the test and puts it back
// afterwards, so t.TempDir's own cleanup can still remove what it created.
func chmodForTest(t *testing.T, path string, mode, restore os.FileMode) {
	t.Helper()

	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod(%q, %v) = %v, want no error", path, mode, err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, restore) })
}

// assertUnwritablePayload states the contract every refusal above owes a reader:
// the stable code decision D-03 rates exit 4, an impact, and something to do.
func assertUnwritablePayload(t *testing.T, err error) app.ErrorPayload {
	t.Helper()

	if err == nil {
		t.Fatal("Err is nil on a refused probe; a refusal owes the reader a diagnosis")
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("PayloadOf(%v) = _, false, want a domain payload", err)
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("payload.Code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
	}
	if got := app.ExitCode(err); got != app.ExitUnavailable {
		t.Errorf("ExitCode = %d, want %d (decision D-03: unwritable runtime path)", got, app.ExitUnavailable)
	}
	if payload.Impact == "" {
		t.Error("payload.Impact is empty; a refusal owes the reader one")
	}
	if len(payload.NextAction) == 0 {
		t.Fatal("payload.NextAction is empty; there is nothing for the user to do")
	}
	return payload
}

func dirEntryNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v, want no error", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
