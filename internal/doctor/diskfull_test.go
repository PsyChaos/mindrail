package doctor

import (
	"errors"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// noSpaceRefusal is the answer storage.ProbeWriteAccess returns for a runtime
// database on a filesystem with nothing left to give.
//
// It is built here rather than provoked, because provoking it needs a real
// filesystem filled to zero bytes and that is a mount, not a temp directory —
// internal/storage owns that test and runs it in a user namespace. What this
// package owns is what doctor does with the answer, and that is exercised from
// the answer's shape: the blocker, and the payload the storage layer attaches to
// it. Both are copied from storage's own constructor, so a rewording there that
// this file did not follow shows up as a failure in the wording assertions
// below rather than as silence.
func noSpaceRefusal(path string) storage.WriteAccess {
	dir := "/repo/.git/mindrail"

	err := app.NewError(
		app.CodeRuntimePathUnwritable,
		app.KindUnavailable,
		"the filesystem holding the runtime database at "+path+" is full",
		"Mindrail can read the state already recorded here but cannot record anything new, so `mindrail init` and every other command that writes will fail until there is space.",
		"free space on the filesystem holding "+path,
	).
		WithMetadata("path", path).
		WithMetadata("blocked_path", dir).
		WithMetadata("blocker", string(storage.BlockerNoSpace)).
		WithMetadata("condition", "disk_full").
		WithCause(storage.ErrDiskFull)

	return storage.WriteAccess{
		Path:    path,
		Blocker: storage.BlockerNoSpace,
		Blocked: dir,
		Err:     err,
	}
}

// TestAFullFilesystemIsNotReportedAsAHealthyDatabase is finding D1.
//
// The condition: a runtime database that opens perfectly on a filesystem with
// zero bytes available. It happens whenever another process is holding the
// database open, because then the `-shm` index already exists at the size SQLite
// wants and the open needs no new blocks at all. Every pragma reads back, the
// integrity check passes, and every write fails.
//
// What the report said about it, measured against the compiled binary: the
// sqlite check printed `✓ Runtime database healthy` and, on the line
// immediately below it, `db_writable: false`. `doctor` exited 0, `status`
// reported READY, and `mindrail init` in the same repository exited 4 with
// RUNTIME_PATH_UNWRITABLE. One block of one document asserted both halves of a
// contradiction, because the verdict was decided without consulting the probe
// whose answer was printed underneath it.
func TestAFullFilesystemIsNotReportedAsAHealthyDatabase(t *testing.T) {
	s := healthySubject()
	s.Probes.DBWrite = noSpaceRefusal(s.Paths.DBPath)

	result := SQLiteCheck(s).Run(t.Context())

	if result.State == StateOK {
		t.Fatalf("sqlite check = %s %q on a filesystem with 0 bytes available; "+
			"this is the `✓ Runtime database healthy` doctor printed over a disk that cannot take a byte",
			result.State, result.Summary)
	}
	if result.Code != app.CodeRuntimePathUnwritable {
		t.Errorf("code = %q, want %q", result.Code, app.CodeRuntimePathUnwritable)
	}
	if app.ExitCode(app.NewError(result.Code, app.KindUnavailable, "", "")) != app.ExitUnavailable {
		t.Errorf("code %q does not map to exit %d (decision D-03)", result.Code, app.ExitUnavailable)
	}

	// The detail the check publishes and the verdict it reports have to be the
	// same reading. Printing one from the probe and deciding the other without it
	// is the whole defect.
	if got := result.Details["db_writable"]; got != "false" {
		t.Errorf("db_writable = %q, want %q; the detail and the verdict come from one probe", got, "false")
	}

	remedy := strings.Join(result.NextAction, " ")
	if !strings.Contains(remedy, "free space on the filesystem holding "+s.Paths.DBPath) {
		t.Errorf("next_action = %v, want it to ask for space on the filesystem holding %s",
			result.NextAction, s.Paths.DBPath)
	}
	if strings.Contains(remedy, "permission") || strings.Contains(remedy, "chmod") {
		t.Errorf("next_action = %v, which prescribes a permission change for a full disk", result.NextAction)
	}
	if strings.Contains(remedy, initCommand) {
		t.Errorf("next_action = %v, which sends the reader to the command that fails identically here",
			result.NextAction)
	}

	// The summary is where a reader decides whether to reach for chmod or for df.
	if !strings.Contains(strings.ToLower(result.Summary), "full") {
		t.Errorf("summary = %q, which does not name the condition", result.Summary)
	}
}

// TestAFullFilesystemSupersedesTheInitRemedyEverywhere checks the rest of the
// document, because a report that names the condition in one check and still
// offers `mindrail init` in the next three has not stopped the loop, only moved
// it.
func TestAFullFilesystemSupersedesTheInitRemedyEverywhere(t *testing.T) {
	s := healthySubject()
	s.Probes.DBWrite = noSpaceRefusal(s.Paths.DBPath)
	// A first run that ran out of space partway: the database exists, the ledger
	// does not, and every reading of the runtime store wants to recommend init.
	s.DBPresent = false
	s.Migrations = nil
	s.Workspace.ID = ""

	for _, check := range DefaultChecks(s) {
		result := check.Run(t.Context())
		if !offersInit(result.NextAction) {
			continue
		}
		if !strings.Contains(strings.Join(result.NextAction, " "), "free space") {
			t.Errorf("check %q offers `%s` on a full filesystem without first asking for space: %v",
				result.Name, initCommand, result.NextAction)
		}
	}
}

// TestAHealthyDatabaseIsUnaffectedByTheSpaceReading is the first over-fire
// guard. The reading reaches into every report; on a repository with room on the
// disk there is nothing for it to say.
func TestAHealthyDatabaseIsUnaffectedByTheSpaceReading(t *testing.T) {
	s := healthySubject()

	result := SQLiteCheck(s).Run(t.Context())

	if result.State != StateOK {
		t.Fatalf("sqlite check = %s %q on a healthy repository: %+v", result.State, result.Summary, result)
	}
	if got := result.Details["db_writable"]; got != "true" {
		t.Errorf("db_writable = %q, want %q", got, "true")
	}
	if len(result.NextAction) != 0 {
		t.Errorf("next_action = %v on a healthy repository", result.NextAction)
	}
}

// TestARuntimeDirectoryRefusalIsNotClaimedAsADatabaseRefusal is the adjacent
// condition, and it is the one the widening must not swallow.
//
// storage.ProbeWriteAccess reports "not writable" for four different blockers,
// and only three of them are this reading's business. BlockerDirectory is the
// runtime root — which RuntimePathCheck names, along with the environment
// variable that relocates it — and it is also what an ordinary uninitialised
// repository looks like, because the directory the database would live in does
// not exist yet. Letting it decide the database's verdict would report every
// repository that has never run `mindrail init` as having an unwritable
// database.
func TestARuntimeDirectoryRefusalIsNotClaimedAsADatabaseRefusal(t *testing.T) {
	s := healthySubject()
	s.Probes.DBWrite = storage.WriteAccess{
		Path:    s.Paths.DBPath,
		Blocker: storage.BlockerDirectory,
		Blocked: s.Paths.RuntimeRoot,
		Err:     errors.New("the directory will not accept the write-ahead log files"),
	}

	result := SQLiteCheck(s).Run(t.Context())

	if result.State != StateOK {
		t.Fatalf("sqlite check = %s %q for a blocker that belongs to the runtime paths check: %+v",
			result.State, result.Summary, result)
	}
}

// TestTheSpaceRemedyDoesNotPrescribeAChmodWithoutAPayload covers the fallback
// sentence, which is reached only by a refusal that arrived with no diagnosis of
// its own. It is still a sentence somebody reads, and a chmod for a full disk is
// the dead end this classification was split apart to remove.
func TestTheSpaceRemedyDoesNotPrescribeAChmodWithoutAPayload(t *testing.T) {
	s := healthySubject()
	s.Probes.DBWrite = storage.WriteAccess{
		Path:    s.Paths.DBPath,
		Blocker: storage.BlockerNoSpace,
		Blocked: s.Paths.RuntimeRoot,
		Err:     errors.New("no space left on device"),
	}

	result := SQLiteCheck(s).Run(t.Context())

	remedy := strings.Join(result.NextAction, " ")
	if strings.Contains(remedy, "permission") {
		t.Errorf("next_action = %v, which prescribes a permission change for a full disk", result.NextAction)
	}
	if !strings.Contains(remedy, "free space on the filesystem holding "+s.Paths.RuntimeRoot) {
		t.Errorf("next_action = %v, want it to ask for space", result.NextAction)
	}
}
