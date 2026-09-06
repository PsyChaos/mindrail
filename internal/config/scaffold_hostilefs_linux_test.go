//go:build linux

package config_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/config"
)

// hostileEnv turns this test binary into the child that lays the repository
// scaffold down on a filesystem that will not take it.
//
// A full filesystem and a read-only mount cannot be faked: access(2) reads mode
// bits and mount flags, and the whole finding is that the mode bits are the
// wrong question. `<worktree>/.mindrail` is mode 0755, owned by the caller, and
// `chmod 777` on it changes nothing -- yet the failure was rendered as "check
// the permissions on <worktree>/.mindrail" and `init` exited 4 every time
// (finding E2).
//
// The child mounts a small tmpfs inside a mount namespace of its own, so the
// namespace and everything in it vanish when it exits.
const hostileEnv = "MINDRAIL_TEST_HOSTILE_SCAFFOLD_MOUNT"

const (
	hostilePassed      = 0  // the scenario ran and every assertion held
	hostileFailed      = 44 // the scenario ran and something is wrong
	hostileUnavailable = 45 // the scenario could not be set up here
)

// tmpfsSize is deliberately small: the scaffold, a filler file and the mount
// overhead all fit, and filling four megabytes of memory takes milliseconds.
const tmpfsSize = "size=4m"

// TestMain dispatches the re-exec before it runs anything, because the states
// this scenario needs exist only inside a mount namespace this process cannot
// enter.
func TestMain(m *testing.M) {
	if mount := os.Getenv(hostileEnv); mount != "" {
		os.Exit(runHostileChild(mount))
	}
	os.Exit(m.Run())
}

// TestTheScaffoldNamesTheConditionThatStoppedIt is finding E2, plus the
// read-only mount beside it.
//
// Writing `.mindrail/config.toml` fails with ENOSPC on a full disk and with
// EROFS on a read-only checkout. Neither is a permission problem, and both were
// rendered as one: the directory is 0755 and owned by the running user, `chmod
// 777` changes nothing, and `mindrail init` exits 4 on the next run exactly as
// it did on the last.
func TestTheScaffoldNamesTheConditionThatStoppedIt(t *testing.T) {
	if testing.Short() {
		t.Skip("mounts a filesystem in a user namespace")
	}

	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("no unshare(1); a real full filesystem cannot be arranged here")
	}
	if !userNamespacesWork(unshare) {
		t.Skip("unprivileged user namespaces are unavailable; a real full filesystem cannot be arranged here")
	}

	binary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable = %v, want no error", err)
	}

	mount := filepath.Join(t.TempDir(), "hostilefs")
	if err := os.Mkdir(mount, 0o700); err != nil {
		t.Fatalf("Mkdir = %v, want no error", err)
	}

	cmd := exec.Command(unshare, "--user", "--map-root-user", "--mount", "--", binary)
	cmd.Env = append(os.Environ(), hostileEnv+"="+mount)
	out, runErr := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		t.Fatalf("spawn: %v", runErr)
	}

	switch cmd.ProcessState.ExitCode() {
	case hostilePassed:
	case hostileUnavailable:
		t.Skipf("a hostile filesystem could not be mounted here: %s", strings.TrimSpace(string(out)))
	default:
		t.Fatalf("the hostile-filesystem scaffold scenario failed:\n%s", strings.TrimSpace(string(out)))
	}
}

// userNamespacesWork asks the kernel the question rather than guessing from a
// sysctl path that differs between distributions.
func userNamespacesWork(unshare string) bool {
	truth, err := exec.LookPath("true")
	if err != nil {
		return false
	}
	return exec.Command(unshare, "--user", "--map-root-user", "--mount", "--", truth).Run() == nil
}

func runHostileChild(mount string) int {
	if err := syscall.Mount("tmpfs", mount, "tmpfs", 0, tmpfsSize); err != nil {
		fmt.Fprintf(os.Stderr, "mount tmpfs on %s: %v\n", mount, err)
		return hostileUnavailable
	}

	problems, unavailable := hostileScaffoldScenario(mount)
	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}
	switch {
	case unavailable:
		return hostileUnavailable
	case len(problems) > 0:
		return hostileFailed
	default:
		return hostilePassed
	}
}

// hostileScaffoldScenario walks one worktree through the states that matter.
func hostileScaffoldScenario(mount string) (problems []string, unavailable bool) {
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	// The over-fire guard first, on the same small filesystem with room on it: a
	// classification that fired here would stop every `mindrail init` in the
	// world. It is deliberately taken before anything is filled or remounted, so
	// it is the same code answering about the same directory.
	healthy := filepath.Join(mount, "healthy")
	if err := os.MkdirAll(healthy, 0o755); err != nil {
		return []string{fmt.Sprintf("MkdirAll %s: %v", healthy, err)}, true
	}
	if _, written, err := config.WriteIfAbsent(healthy); err != nil || !written {
		return []string{fmt.Sprintf("WriteIfAbsent with room on the disk = (%v, %v), want (true, nil)",
			written, err)}, true
	}
	if _, err := config.EnsureKnowledgeDirs(healthy); err != nil {
		return []string{fmt.Sprintf("EnsureKnowledgeDirs with room on the disk = %v, want no error", err)}, true
	}

	worktree := filepath.Join(mount, "repo")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		return []string{fmt.Sprintf("MkdirAll %s: %v", worktree, err)}, true
	}

	filler := filepath.Join(mount, "filler")
	if err := fillToZero(filler); err != nil {
		return []string{err.Error()}, true
	}
	if free := availableBytes(mount); free != 0 {
		return []string{fmt.Sprintf("the filesystem reports %d bytes available; "+
			"the scenario needs it genuinely full", free)}, true
	}

	problems = append(problems, scaffoldOnAFullFilesystem(worktree)...)

	// The remedy, carried out. The sentence a user is handed has to end the loop,
	// and the sentence it replaced could not: `chmod 777` on a directory whose
	// mode was never the problem leaves `init` failing exactly as before.
	if err := os.Remove(filler); err != nil {
		return append(problems, fmt.Sprintf("Remove %s: %v", filler, err)), false
	}
	if path, written, err := config.WriteIfAbsent(worktree); err != nil || !written {
		report("after freeing space, WriteIfAbsent(%s) = (%q, %v, %v), want it written; "+
			"the remedy has to end the loop", worktree, path, written, err)
	}
	if _, err := config.EnsureKnowledgeDirs(worktree); err != nil {
		report("after freeing space, EnsureKnowledgeDirs = %v, want no error", err)
	}

	return problems, false
}

// scaffoldOnAFullFilesystem is finding E2: the sentence `mindrail init` prints
// when `<worktree>/.mindrail/config.toml` cannot be written for want of space.
func scaffoldOnAFullFilesystem(worktree string) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	_, written, err := config.WriteIfAbsent(worktree)
	if err == nil {
		return []string{fmt.Sprintf("WriteIfAbsent on a filesystem with 0 bytes available = "+
			"(written %v, nil); the scenario did not reproduce", written)}
	}

	payload, ok := app.PayloadOf(err)
	if !ok {
		return []string{fmt.Sprintf("WriteIfAbsent = %v, which carries no domain payload", err)}
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		report("payload.Code = %q, want %q", payload.Code, app.CodeRuntimePathUnwritable)
	}
	if app.ExitCode(err) != app.ExitUnavailable {
		report("ExitCode = %d, want %d (decision D-03)", app.ExitCode(err), app.ExitUnavailable)
	}

	if !strings.Contains(payload.Why, "full") {
		report("Why = %q, want it to say the filesystem is full; the directory's mode bits "+
			"are 0755 and owned by the caller", payload.Why)
	}

	remedy := strings.Join(payload.NextAction, " ")
	if !strings.Contains(remedy, "free space") {
		report("NextAction = %q, want it to ask for space", payload.NextAction)
	}
	if strings.Contains(remedy, "permission") || strings.Contains(remedy, "chmod") {
		report("NextAction = %q, which prescribes a permission change for a full disk; "+
			"`chmod 777` on this directory changes nothing and init still exits 4", payload.NextAction)
	}
	return problems
}

// fillToZero grows path until the filesystem refuses, in shrinking steps so the
// last block really is taken. A filesystem with one page left is a different
// condition from a full one, and this scenario needs the full one.
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

// availableBytes is the scenario's own reading of the filesystem, taken through
// separate code from the probe's so that a probe agreeing with itself proves
// nothing.
func availableBytes(dir string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}
