//go:build linux

package storage_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// fullDiskEnv turns this test binary into the child that runs the scenario on a
// filesystem it fills up.
//
// A full filesystem cannot be faked. access(2) reads mode bits and statfs(2)
// reads the superblock; neither can be persuaded by a temp directory, and the
// entire finding is that mode bits are the wrong question. So the child mounts a
// four-megabyte tmpfs inside a mount namespace of its own, fills it, and asks
// the real probe and the real Open about the real condition.
//
// It is a separate process because the mount has to be. Mounting anything from
// inside the test process would need CAP_SYS_ADMIN in the ambient namespace,
// which no test may assume; `unshare --user --map-root-user --mount` grants it
// inside a namespace that vanishes when the child exits, and takes nothing with
// it. Where the kernel refuses that -- unprivileged user namespaces turned off,
// no `unshare` binary -- the test skips, and the deterministic tests beside it
// are what hold the policy in place.
const fullDiskEnv = "MINDRAIL_TEST_FULL_DISK_MOUNT"

const (
	fullDiskPassed      = 0  // the scenario ran and every assertion held
	fullDiskFailed      = 40 // the scenario ran and something is wrong
	fullDiskUnavailable = 41 // the scenario could not be set up here
)

// tmpfsSize is deliberately small. Everything the scenario needs -- a WAL
// database, its sidecars, a filler file -- fits inside it, and filling four
// megabytes of memory-backed filesystem takes milliseconds.
const tmpfsSize = "size=4m"

// TestAFullFilesystemIsRefusedAndDiagnosed is findings D1 and D2 in one run,
// because they are one condition seen from two places: the probe that answers
// `db_writable` without opening anything, and the open that every command does.
//
// D1: after a full disk aborted the first migration, `mindrail doctor` printed
// `db_writable: true` and `✓ Runtime database healthy`, then prescribed
// `mindrail init` -- which failed with exit 4 every time. The probe answered
// from mode bits, which are correct on a full disk.
//
// D2: on a full filesystem under an already-initialised database, every command
// failed at open with a generic diagnosis whose remedy was to check a directory
// that existed and was writable, and then to run the command that had just
// failed.
func TestAFullFilesystemIsRefusedAndDiagnosed(t *testing.T) {
	if testing.Short() {
		t.Skip("mounts a filesystem in a user namespace")
	}

	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("no unshare(1); a real full filesystem cannot be arranged here")
	}
	if !userNamespacesWork(t, unshare) {
		t.Skip("unprivileged user namespaces are unavailable; a real full filesystem cannot be arranged here")
	}

	binary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable = %v, want no error", err)
	}

	mount := filepath.Join(t.TempDir(), "fullfs")
	if err := os.Mkdir(mount, 0o700); err != nil {
		t.Fatalf("Mkdir = %v, want no error", err)
	}

	cmd := exec.Command(unshare, "--user", "--map-root-user", "--mount", "--", binary)
	cmd.Env = append(os.Environ(), fullDiskEnv+"="+mount)
	out, runErr := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		t.Fatalf("spawn: %v", runErr)
	}

	switch cmd.ProcessState.ExitCode() {
	case fullDiskPassed:
	case fullDiskUnavailable:
		t.Skipf("a capped filesystem could not be mounted here: %s", strings.TrimSpace(string(out)))
	default:
		t.Fatalf("the full-filesystem scenario failed:\n%s", strings.TrimSpace(string(out)))
	}
}

// userNamespacesWork asks the kernel the question rather than guessing from a
// sysctl path that differs between distributions.
func userNamespacesWork(t *testing.T, unshare string) bool {
	t.Helper()

	truth, err := exec.LookPath("true")
	if err != nil {
		return false
	}
	return exec.Command(unshare, "--user", "--map-root-user", "--mount", "--", truth).Run() == nil
}

// runFullDiskChild is the child half, reached from TestMain.
func runFullDiskChild(mount string) int {
	if err := syscall.Mount("tmpfs", mount, "tmpfs", 0, tmpfsSize); err != nil {
		fmt.Fprintf(os.Stderr, "mount tmpfs on %s: %v\n", mount, err)
		return fullDiskUnavailable
	}

	problems := fullDiskScenario(mount)
	if len(problems) == 0 {
		return fullDiskPassed
	}
	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}
	return fullDiskFailed
}

// fullDiskScenario walks one filesystem through the states that matter and
// returns everything it found wrong.
//
// It reports rather than fatals so that one run says everything it can: a probe
// that over-fires on a nearly-full filesystem and a remedy that does not clear
// the condition are separate defects, and finding them one process re-exec at a
// time is how a five-minute check becomes an afternoon.
func fullDiskScenario(mount string) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	ctx := context.Background()
	runtimeDir := filepath.Join(mount, "repo", ".git", "mindrail")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return []string{fmt.Sprintf("MkdirAll %s: %v", runtimeDir, err)}
	}
	path := filepath.Join(runtimeDir, "mindrail.db")

	// A second repository, checked out but never initialised: its Git common-dir
	// exists and its runtime directory does not. That is the shape of every first
	// `mindrail init`, and the shape finding E1 is about -- the probe reached
	// access(2) on a directory that is not there, called the ENOENT a refusal of
	// the write-ahead log files, and never got as far as reading the free space.
	freshCommonDir := filepath.Join(mount, "fresh", ".git")
	if err := os.MkdirAll(freshCommonDir, 0o700); err != nil {
		return []string{fmt.Sprintf("MkdirAll %s: %v", freshCommonDir, err)}
	}
	freshPath := filepath.Join(freshCommonDir, "mindrail", "mindrail.db")

	// A repository that has not been initialised, on a small filesystem with
	// room. This is the first over-fire guard: a probe that answered "no space"
	// here would stop `mindrail init` from ever running.
	if got := storage.ProbeWriteAccess(path); !got.Writable {
		report("before anything was written, ProbeWriteAccess = %+v, want Writable", got)
	}
	// The same, for the repository whose runtime directory does not exist yet.
	// This is the over-fire guard for the absent-directory reading below: a probe
	// that refused here would stop `mindrail init` from ever running on a fresh
	// checkout.
	if got := storage.ProbeWriteAccess(freshPath); !got.Writable {
		report("with room on the disk and the runtime directory not yet created, "+
			"ProbeWriteAccess = %+v, want Writable", got)
	}

	if err := initialiseOnDisk(ctx, path); err != nil {
		return append(problems, err.Error())
	}

	// The healthy initialised repository, still on the small filesystem. This is
	// the over-fire guard the whole change hangs on.
	if got := storage.ProbeWriteAccess(path); !got.Writable {
		report("on a healthy initialised repository, ProbeWriteAccess = %+v, want Writable", got)
	}

	filler := filepath.Join(mount, "filler")
	if err := fillToZero(filler); err != nil {
		return append(problems, err.Error())
	}

	// The adjacent condition that must not trip the detection: a filesystem that
	// is nearly full is not one that is full, and most developers' disks are
	// nearly full most of the time.
	const headroom = 128 << 10
	if err := leaveFree(filler, headroom); err != nil {
		report("%v", err)
	} else if free := availableBytes(mount); free <= 0 {
		report("could not arrange a nearly-full filesystem: %d bytes available", free)
	} else if got := storage.ProbeWriteAccess(path); !got.Writable {
		report("with %d bytes still free, ProbeWriteAccess = %+v, want Writable; "+
			"nearly full is not full", free, got)
	} else if got := storage.ProbeWriteAccess(freshPath); !got.Writable {
		report("with %d bytes still free and the runtime directory not yet created, "+
			"ProbeWriteAccess = %+v, want Writable; nearly full is not full", free, got)
	}

	if err := fillToZero(filler); err != nil {
		return append(problems, err.Error())
	}
	if free := availableBytes(mount); free != 0 {
		return append(problems, fmt.Sprintf("the filesystem reports %d bytes available; "+
			"the scenario needs it genuinely full", free))
	}

	problems = append(problems, probeOnAFullFilesystem(path, runtimeDir)...)
	problems = append(problems, probeBeforeTheRuntimeDirectoryExists(freshPath)...)
	problems = append(problems, openOnAFullFilesystem(ctx, path)...)

	// The remedy, carried out. "Free space on the filesystem holding X" is the
	// sentence every layer prints, and a remedy that does not end the loop is the
	// defect being fixed rather than the fix.
	if err := os.Remove(filler); err != nil {
		return append(problems, fmt.Sprintf("Remove %s: %v", filler, err))
	}
	problems = append(problems, afterTheRemedy(ctx, path)...)

	// The remedy has to end the loop for the fresh repository too, which is the
	// case where "free space" replaced a remedy that could not be carried out at
	// all. `mindrail init` creates the directory; if the probe still refuses
	// after the space is back, the sentence the user was given was wrong.
	if got := storage.ProbeWriteAccess(freshPath); !got.Writable {
		problems = append(problems, fmt.Sprintf(
			"after freeing space, ProbeWriteAccess on a repository with no runtime directory = %+v, "+
				"want Writable; the remedy has to end the loop", got))
	} else if err := os.MkdirAll(filepath.Dir(freshPath), 0o700); err != nil {
		problems = append(problems, fmt.Sprintf(
			"after freeing space, creating the runtime directory the probe said was creatable = %v", err))
	} else if db, err := storage.Open(ctx, storage.Options{Path: freshPath}); err != nil {
		problems = append(problems, fmt.Sprintf(
			"after freeing space, storage.Open on the fresh repository = %v, want no error", err))
	} else {
		_ = db.Close()
	}

	return problems
}

// probeOnAFullFilesystem is finding D1: the reading `doctor` publishes as
// db_writable, taken over a filesystem that cannot accept a byte.
func probeOnAFullFilesystem(path, runtimeDir string) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	before := directoryContents(runtimeDir)
	got := storage.ProbeWriteAccess(path)

	if got.Writable {
		report("on a filesystem with 0 bytes available, ProbeWriteAccess = %+v, want it refused; "+
			"this is the `db_writable: true` doctor printed over a disk that cannot take a byte", got)
		return problems
	}
	if got.Blocker != storage.BlockerNoSpace {
		report("Blocker = %q, want %q; the remedy is chosen from it", got.Blocker, storage.BlockerNoSpace)
	}
	if !errors.Is(got.Err, storage.ErrDiskFull) {
		report("Err = %v, want errors.Is(err, ErrDiskFull)", got.Err)
	}
	if errors.Is(got.Err, storage.ErrReadOnly) {
		report("Err = %v, want it not to read as a permission refusal", got.Err)
	}

	checkRemedy(report, got.Err, path)

	// Decision D-01: doctor reads and never writes. A probe that took the write
	// lock to find out would leave a `-wal` and a `-shm` behind, and on this
	// filesystem it could not even do that.
	if after := directoryContents(runtimeDir); !slices.Equal(before, after) {
		report("the probe changed %s from %v to %v; decision D-01 forbids it", runtimeDir, before, after)
	}
	return problems
}

// probeBeforeTheRuntimeDirectoryExists is finding E1: the reading `doctor`
// publishes as db_writable for a repository `mindrail init` has never run in,
// taken over a filesystem that cannot accept a byte.
//
// It is a separate step from probeOnAFullFilesystem because it is a separate
// code path, and the path it took was the one that never reached the space
// reading at all: access(2) on a directory that does not exist returns ENOENT,
// which was reported as "the directory will not accept the write-ahead log
// files" and remedied with "restore write and search permission on" a path that
// is not there. The reading that closed the previous full-disk finding sat three
// lines below and was unreachable in this shape.
func probeBeforeTheRuntimeDirectoryExists(path string) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	got := storage.ProbeWriteAccess(path)
	if got.Writable {
		report("on a filesystem with 0 bytes available and the runtime directory not yet created, "+
			"ProbeWriteAccess = %+v, want it refused; this is the `db_writable: true` doctor "+
			"published while prescribing the `mindrail init` that exits 4", got)
		return problems
	}
	if got.Blocker != storage.BlockerNoSpace {
		report("Blocker = %q, want %q; the directory is absent because there is no room to create it, "+
			"not because its permissions are wrong", got.Blocker, storage.BlockerNoSpace)
	}
	if !errors.Is(got.Err, storage.ErrDiskFull) {
		report("Err = %v, want errors.Is(err, ErrDiskFull)", got.Err)
	}

	checkRemedy(report, got.Err, path)

	if _, err := os.Stat(filepath.Dir(path)); err == nil {
		report("the probe created %s; decision D-01 forbids it", filepath.Dir(path))
	}
	return problems
}

// openOnAFullFilesystem is finding D2: what every command gets on a full
// filesystem under an already-initialised database.
func openOnAFullFilesystem(ctx context.Context, path string) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	db, err := storage.Open(ctx, storage.Options{Path: path})
	if err == nil {
		_ = db.Close()
		// Not a defect in the diagnosis, but the scenario proves nothing if the
		// open succeeded, so it has to be said out loud.
		return []string{"storage.Open on a full filesystem succeeded; the scenario did not reproduce"}
	}

	if !errors.Is(err, storage.ErrDiskFull) {
		report("storage.Open = %v, want errors.Is(err, ErrDiskFull)", err)
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		report("storage.Open = %v, which carries no domain payload", err)
		return problems
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		report("payload.Code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
	}
	if app.ExitCode(err) != app.ExitUnavailable {
		report("ExitCode = %d, want %d (decision D-03)", app.ExitCode(err), app.ExitUnavailable)
	}

	remedy := strings.Join(payload.NextAction, " ")
	if !strings.Contains(remedy, "free space on the filesystem holding "+path) {
		report("NextAction = %q, want it to ask for space on the filesystem holding %s", payload.NextAction, path)
	}
	if strings.Contains(remedy, "Git common directory") {
		report("NextAction = %q, whose first line is a dead end: the common directory exists and is writable",
			payload.NextAction)
	}
	if strings.Contains(remedy, "mindrail init") {
		report("NextAction = %q, which sends the user to the command that just failed", payload.NextAction)
	}
	return problems
}

// afterTheRemedy carries out the sentence the user is given and checks that it
// ends the condition, rather than trusting that it would.
func afterTheRemedy(ctx context.Context, path string) []string {
	var problems []string

	if got := storage.ProbeWriteAccess(path); !got.Writable {
		problems = append(problems,
			fmt.Sprintf("after freeing space, ProbeWriteAccess = %+v, want Writable; the remedy has to end the loop", got))
	}

	db, err := storage.Open(ctx, storage.Options{Path: path})
	if err != nil {
		return append(problems, fmt.Sprintf("after freeing space, storage.Open = %v, want no error; "+
			"the remedy names something that does not clear the condition", err))
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, `INSERT INTO probe (x) VALUES (1)`); err != nil {
		problems = append(problems, fmt.Sprintf("after freeing space, a write = %v, want no error", err))
	}
	return problems
}

// checkRemedy checks the sentence a refusal hands the user. It is the same set
// of questions for the probe and for the open, and asking them in one place is
// what keeps the two from drifting apart.
func checkRemedy(report func(string, ...any), err error, path string) {
	payload, ok := app.PayloadOf(err)
	if !ok {
		report("the refusal %v carries no domain payload", err)
		return
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		report("payload.Code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
	}

	remedy := strings.Join(payload.NextAction, " ")
	if !strings.Contains(remedy, "free space on the filesystem holding "+path) {
		report("NextAction = %q, want it to ask for space on the filesystem holding %s", payload.NextAction, path)
	}
	if strings.Contains(remedy, "permission") || strings.Contains(remedy, "chmod") {
		report("NextAction = %q, which prescribes a permission change for a full disk", payload.NextAction)
	}
}

// fullDiskChild is TestMain's half of the re-exec: it reports whether this
// process was started as the scenario's child, and what it should exit with.
func fullDiskChild() (int, bool) {
	mount := os.Getenv(fullDiskEnv)
	if mount == "" {
		return 0, false
	}
	return runFullDiskChild(mount), true
}

// initialiseOnDisk is what `mindrail init` leaves behind, built through the same
// storage API the command uses.
func initialiseOnDisk(ctx context.Context, path string) error {
	db, err := storage.Open(ctx, storage.Options{Path: path})
	if err != nil {
		return fmt.Errorf("storage.Open with room on the disk: %w", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, `CREATE TABLE probe (x INTEGER)`); err != nil {
		return fmt.Errorf("CREATE TABLE with room on the disk: %w", err)
	}
	return nil
}

// fillToZero grows path until the filesystem refuses, in shrinking steps so the
// last block really is taken. A filesystem with one page left is a different
// condition from a full one, and this test needs the full one.
func fillToZero(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open filler %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	for _, chunk := range []int{1 << 20, 4096, 1} {
		block := make([]byte, chunk)
		for {
			if _, err := file.Write(block); err != nil {
				break
			}
		}
	}
	return nil
}

// leaveFree gives headroom bytes back to the filesystem, which is how the
// nearly-full state is arranged without guessing at the overhead of everything
// else on the mount.
func leaveFree(path string, headroom int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat filler %s: %w", path, err)
	}
	size := info.Size() - headroom
	if size < 0 {
		size = 0
	}
	if err := os.Truncate(path, size); err != nil {
		return fmt.Errorf("truncate filler %s: %w", path, err)
	}
	return nil
}

// availableBytes is the test's own reading of the filesystem, taken with the
// same syscall the probe uses but through separate code. A probe that agreed
// with itself would prove nothing.
func availableBytes(dir string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}

// directoryContents lists a directory so the probe can be held to D-01.
func directoryContents(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{"unreadable: " + err.Error()}
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names
}
