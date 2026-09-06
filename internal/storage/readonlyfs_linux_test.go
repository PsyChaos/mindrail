//go:build linux

package storage_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// readOnlyEnv turns this test binary into the child that runs the scenario on a
// filesystem it has mounted read-only.
//
// A read-only mount cannot be faked, for the same reason a full filesystem
// cannot. access(2) reads the mount flags as well as the mode bits, and the
// entire finding is that the mode bits are the wrong question: every one of them
// is already correct, the write is refused anyway, and `chmod` fails with the
// same EROFS. So the child mounts a tmpfs inside a mount namespace of its own,
// initialises a database on it, and then remounts it read-only underneath the
// finished repository.
//
// The remount is a read-only bind of the mount over itself rather than
// `remount,ro` on the tmpfs, because a tmpfs mounted inside a user namespace
// carries uid options the plain remount refuses to re-parse. The bind produces
// the same EROFS on every write and the same failure from `chmod`.
const readOnlyEnv = "MINDRAIL_TEST_READONLY_MOUNT"

const (
	readOnlyPassed      = 0  // the scenario ran and every assertion held
	readOnlyFailed      = 42 // the scenario ran and something is wrong
	readOnlyUnavailable = 43 // the scenario could not be set up here
)

// TestAReadOnlyFilesystemIsNotAPermissionProblem is finding E4.
//
// `status` and `doctor` printed "restore write permission on <db>, <-wal>,
// <-shm>" over a repository whose files are mode 0600 and owned by the caller,
// on a filesystem where `chmod` itself fails with EROFS. The remedy could not be
// carried out, and the same document carried a different, working one further
// down -- so one condition had two remedies and the headline one was a dead end.
func TestAReadOnlyFilesystemIsNotAPermissionProblem(t *testing.T) {
	if testing.Short() {
		t.Skip("mounts a filesystem in a user namespace")
	}

	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("no unshare(1); a real read-only mount cannot be arranged here")
	}
	if !userNamespacesWork(t, unshare) {
		t.Skip("unprivileged user namespaces are unavailable; a real read-only mount cannot be arranged here")
	}

	binary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable = %v, want no error", err)
	}

	mount := filepath.Join(t.TempDir(), "readonlyfs")
	if err := os.Mkdir(mount, 0o700); err != nil {
		t.Fatalf("Mkdir = %v, want no error", err)
	}

	cmd := exec.Command(unshare, "--user", "--map-root-user", "--mount", "--", binary)
	cmd.Env = append(os.Environ(), readOnlyEnv+"="+mount)
	out, runErr := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		t.Fatalf("spawn: %v", runErr)
	}

	switch cmd.ProcessState.ExitCode() {
	case readOnlyPassed:
	case readOnlyUnavailable:
		t.Skipf("a read-only filesystem could not be mounted here: %s", strings.TrimSpace(string(out)))
	default:
		t.Fatalf("the read-only-filesystem scenario failed:\n%s", strings.TrimSpace(string(out)))
	}
}

// readOnlyChild is TestMain's half of the re-exec.
func readOnlyChild() (int, bool) {
	mount := os.Getenv(readOnlyEnv)
	if mount == "" {
		return 0, false
	}
	return runReadOnlyChild(mount), true
}

func runReadOnlyChild(mount string) int {
	if err := syscall.Mount("tmpfs", mount, "tmpfs", 0, tmpfsSize); err != nil {
		fmt.Fprintf(os.Stderr, "mount tmpfs on %s: %v\n", mount, err)
		return readOnlyUnavailable
	}

	problems, unavailable := readOnlyScenario(mount)
	if unavailable {
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, problem)
		}
		return readOnlyUnavailable
	}
	if len(problems) == 0 {
		return readOnlyPassed
	}
	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}
	return readOnlyFailed
}

// remountReadOnly makes the whole mount refuse writes, and undoes it again.
func remountReadOnly(mount string) error {
	if err := syscall.Mount(mount, mount, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind %s over itself: %w", mount, err)
	}
	if err := syscall.Mount("", mount, "",
		syscall.MS_REMOUNT|syscall.MS_BIND|syscall.MS_RDONLY, ""); err != nil {
		return fmt.Errorf("remount %s read-only: %w", mount, err)
	}
	return nil
}

func remountReadWrite(mount string) error {
	if err := syscall.Mount("", mount, "", syscall.MS_REMOUNT|syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("remount %s read-write: %w", mount, err)
	}
	return nil
}

// readOnlyScenario walks one repository from healthy to read-only and back, and
// returns everything it found wrong.
func readOnlyScenario(mount string) (problems []string, unavailable bool) {
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	ctx := context.Background()
	runtimeDir := filepath.Join(mount, "repo", ".git", "mindrail")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return []string{fmt.Sprintf("MkdirAll %s: %v", runtimeDir, err)}, true
	}
	path := filepath.Join(runtimeDir, "mindrail.db")

	if err := initialiseOnDisk(ctx, path); err != nil {
		return []string{err.Error()}, true
	}

	// The over-fire guard, taken on the same filesystem an instant before it is
	// remounted: a healthy repository on ordinary media is untouched by any of
	// this.
	if got := storage.ProbeWriteAccess(path); !got.Writable {
		report("on a healthy initialised repository, ProbeWriteAccess = %+v, want Writable", got)
	}

	if err := remountReadOnly(mount); err != nil {
		return []string{err.Error()}, true
	}

	// The premise, checked rather than assumed. Every assertion below is about a
	// remedy being a dead end, and that only means anything if the dead end is
	// real.
	if err := os.Chmod(path, 0o666); err == nil {
		return []string{fmt.Sprintf("chmod on %s succeeded; the mount is not read-only "+
			"and the scenario did not reproduce", path)}, true
	} else if !errors.Is(err, syscall.EROFS) {
		return []string{fmt.Sprintf("chmod on %s = %v, want EROFS; "+
			"the scenario did not reproduce", path, err)}, true
	}

	problems = append(problems, probeOnAReadOnlyFilesystem(path)...)
	problems = append(problems, openOnAReadOnlyFilesystem(ctx, path)...)

	// The remedy, carried out. It is the only one of the three that works here,
	// and a remedy that does not end the loop is the defect rather than the fix.
	if err := remountReadWrite(mount); err != nil {
		return append(problems, err.Error()), false
	}
	problems = append(problems, afterTheRemedy(ctx, path)...)

	return problems, false
}

// probeOnAReadOnlyFilesystem is the reading doctor and status publish, taken
// over a mount that refuses every write.
func probeOnAReadOnlyFilesystem(path string) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	got := storage.ProbeWriteAccess(path)
	if got.Writable {
		report("on a read-only filesystem, ProbeWriteAccess = %+v, want it refused", got)
		return problems
	}
	if got.Blocker != storage.BlockerReadOnlyMedia {
		report("Blocker = %q, want %q; the remedy is chosen from it, and no part of the "+
			"footprint is at fault here", got.Blocker, storage.BlockerReadOnlyMedia)
	}
	if !errors.Is(got.Err, filesystem.ErrReadOnlyMedia) {
		report("Err = %v, want errors.Is(err, filesystem.ErrReadOnlyMedia)", got.Err)
	}
	if errors.Is(got.Err, storage.ErrDiskFull) {
		report("Err = %v, want it not to read as a full disk; freeing space clears nothing here", got.Err)
	}

	checkReadOnlyRemedy(report, got.Err, path)
	return problems
}

// openOnAReadOnlyFilesystem is what every command gets: SQLite cannot create the
// shared-memory index a WAL database needs, and the result code it returns --
// SQLITE_CANTOPEN -- is the same one a missing path produces, so the diagnosis
// cannot come from the driver.
func openOnAReadOnlyFilesystem(ctx context.Context, path string) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	db, err := storage.Open(ctx, storage.Options{Path: path})
	if err == nil {
		_ = db.Close()
		return []string{"storage.Open on a read-only filesystem succeeded; the scenario did not reproduce"}
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		return []string{fmt.Sprintf("storage.Open = %v, which carries no domain payload", err)}
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		report("payload.Code = %q, want %q; the database is intact and readable, "+
			"it is the mount that refuses", payload.Code, app.CodeRuntimePathUnwritable)
	}
	if app.ExitCode(err) != app.ExitUnavailable {
		report("ExitCode = %d, want %d (decision D-03)", app.ExitCode(err), app.ExitUnavailable)
	}

	checkReadOnlyRemedy(report, err, path)
	return problems
}

// checkReadOnlyRemedy asks the same questions of every sentence this condition
// produces, which is what keeps the probe and the open from drifting apart.
func checkReadOnlyRemedy(report func(string, ...any), err error, path string) {
	payload, ok := app.PayloadOf(err)
	if !ok {
		report("the refusal %v carries no domain payload", err)
		return
	}

	remedy := strings.Join(payload.NextAction, " ")
	if !strings.Contains(remedy, "remount") {
		report("NextAction = %q, want it to offer the one action that works", payload.NextAction)
	}
	if strings.Contains(remedy, "permission") || strings.Contains(remedy, "chmod") {
		report("NextAction = %q, which prescribes a permission change on %s; "+
			"chmod on this filesystem fails with the same EROFS that refused the write",
			payload.NextAction, path)
	}
	if strings.Contains(remedy, "free space") {
		report("NextAction = %q, which prescribes freeing space for a read-only mount", payload.NextAction)
	}
	if strings.Contains(remedy, "Git common directory") {
		report("NextAction = %q, whose first line is a dead end: the common directory "+
			"exists and its mode bits are correct", payload.NextAction)
	}
	if strings.Contains(remedy, "mindrail init") {
		report("NextAction = %q, which sends the user to a command that fails identically",
			payload.NextAction)
	}
}
