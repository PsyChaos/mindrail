//go:build linux

package filesystem_test

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
	"github.com/PsyChaos/mindrail/internal/filesystem"
)

// hostileEnv turns this test binary into the child that probes Mindrail's roots
// on a filesystem that will not take them.
//
// Neither condition can be faked. access(2) answers from mode bits and mount
// flags, and the finding is precisely that the mode bits are correct in both
// states: on a full filesystem `<git-common-dir>` is 0755 and owned by the
// caller and the directory still cannot be created, and on a read-only mount
// `chmod` on it fails with the same EROFS that refused the create. So the child
// mounts a small tmpfs in a mount namespace of its own and puts it into each
// state in turn.
const hostileEnv = "MINDRAIL_TEST_HOSTILE_ROOTS_MOUNT"

const (
	hostilePassed      = 0  // the scenario ran and every assertion held
	hostileFailed      = 46 // the scenario ran and something is wrong
	hostileUnavailable = 47 // the scenario could not be set up here
)

const tmpfsSize = "size=4m"

// TestMain dispatches the re-exec before it runs anything: the states this
// scenario needs exist only inside a mount namespace this process cannot enter.
func TestMain(m *testing.M) {
	if mount := os.Getenv(hostileEnv); mount != "" {
		os.Exit(runHostileChild(mount))
	}
	os.Exit(m.Run())
}

// TestProbingARootAnswersAboutTheFilesystemAndNotOnlyItsModeBits is finding E1
// at the layer that publishes `runtime_root_usable`, with the read-only mount
// beside it.
//
// On a full disk with the runtime directory not yet created, this probe reported
// the root usable: it walked up to the nearest existing ancestor, asked access(2)
// about its mode bits, got the right answer to the wrong question and said yes.
// `doctor` printed `runtime_root_usable: true`, exited 0, and prescribed the
// `mindrail init` that exits 4 -- so a CI health gate built on it passed on a
// machine where Mindrail could not be installed at all.
func TestProbingARootAnswersAboutTheFilesystemAndNotOnlyItsModeBits(t *testing.T) {
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
		t.Fatalf("the hostile-filesystem root scenario failed:\n%s", strings.TrimSpace(string(out)))
	}
}

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

	problems, unavailable := hostileRootScenario(mount)
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

// hostileRootScenario walks one repository's roots through the states that
// matter and returns everything it found wrong.
func hostileRootScenario(mount string) (problems []string, unavailable bool) {
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	worktree := filepath.Join(mount, "repo")
	common := filepath.Join(worktree, ".git")
	if err := os.MkdirAll(common, 0o755); err != nil {
		return []string{fmt.Sprintf("MkdirAll %s: %v", common, err)}, true
	}

	paths, err := filesystem.ResolveRuntimePaths(filesystem.PathOptions{
		CommonDir: common, WorktreeRoot: worktree,
	})
	if err != nil {
		return []string{fmt.Sprintf("ResolveRuntimePaths = %v", err)}, true
	}

	// The over-fire guard, on the same small filesystem with room on it and with
	// nothing created yet. This is the state of every fresh checkout, and a probe
	// that refused here would stop `mindrail init` from ever running.
	for _, w := range paths.ProbeRoots() {
		if !w.Usable {
			report("with room on the disk and nothing created yet, %s = %+v, want usable", w.Kind, w)
		}
	}
	if repo, known := paths.ProbeRepoConfig(); !known || !repo.Usable {
		report("with room on the disk, ProbeRepoConfig = %+v (known=%v), want usable", repo, known)
	}

	filler := filepath.Join(mount, "filler")
	if err := fillToZero(filler); err != nil {
		return []string{err.Error()}, true
	}

	// The adjacent condition that must not trip the detection: nearly full is not
	// full, and most developers' disks are nearly full most of the time.
	const headroom = 128 << 10
	if err := leaveFree(filler, headroom); err != nil {
		report("%v", err)
	} else if free := availableBytes(mount); free <= 0 {
		report("could not arrange a nearly-full filesystem: %d bytes available", free)
	} else {
		for _, w := range paths.ProbeRoots() {
			if !w.Usable {
				report("with %d bytes still free, %s = %+v, want usable; nearly full is not full",
					free, w.Kind, w)
			}
		}
	}

	if err := fillToZero(filler); err != nil {
		return append(problems, err.Error()), false
	}
	if free := availableBytes(mount); free != 0 {
		return append(problems, fmt.Sprintf("the filesystem reports %d bytes available; "+
			"the scenario needs it genuinely full", free)), false
	}

	problems = append(problems, rootsOnAFullFilesystem(paths)...)

	// The remedy, carried out.
	if err := os.Remove(filler); err != nil {
		return append(problems, fmt.Sprintf("Remove %s: %v", filler, err)), false
	}
	for _, w := range paths.ProbeRoots() {
		if !w.Usable {
			report("after freeing space, %s = %+v, want usable; the remedy has to end the loop", w.Kind, w)
		}
	}
	if err := paths.EnsureDirs(); err != nil {
		report("after freeing space, EnsureDirs = %v, want no error; "+
			"the probe promised the directories were creatable", err)
	}

	problems = append(problems, rootsOnAReadOnlyFilesystem(mount, paths)...)
	return problems, false
}

// rootsOnAFullFilesystem is the reading `doctor` publishes as
// `runtime_root_usable`, taken over a filesystem that cannot accept a byte with
// the runtime directory not yet created.
func rootsOnAFullFilesystem(paths filesystem.RuntimePaths) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	answers := paths.ProbeRoots()
	if repo, known := paths.ProbeRepoConfig(); known {
		answers = append(answers, repo)
	}

	for _, w := range answers {
		if w.Usable {
			report("on a filesystem with 0 bytes available, %s = %+v, want it refused; "+
				"this is the `runtime_root_usable: true` doctor published while exiting 0 "+
				"and prescribing the `mindrail init` that exits 4", w.Kind, w)
			continue
		}
		if w.Exists {
			report("%s: Exists = true; the scenario needs the directory absent", w.Kind)
		}
		checkSpaceRemedy(report, w.Kind, w.Err)
	}

	// The verdict, checked against the ground truth rather than trusted. The
	// probe's claim is "Mindrail cannot store its runtime state here", and that
	// is about the bytes, not about the directory entry: a tmpfs with zero pages
	// left still accepts a mkdir, and on a real full disk it does not. So the
	// creates are allowed to succeed, and what must fail is the write they exist
	// to make room for. A probe refusing a root that can still take a database
	// file would be the over-fire this whole rule is written narrowly to avoid.
	if err := paths.EnsureDirs(); err != nil {
		// It refused, so it has to refuse in the same words the probe used:
		// `init` and `doctor` disagreeing about one condition is what this
		// classification exists to stop.
		checkSpaceRemedy(report, filesystem.RootRuntime, err)
	} else if err := os.WriteFile(paths.DBPath, []byte("x"), 0o600); err == nil {
		report("a write into %s succeeded on a filesystem reporting 0 bytes available; "+
			"the probe refused a root that is in fact usable", paths.RuntimeRoot)
	} else if !errors.Is(err, syscall.ENOSPC) {
		report("a write into %s = %v, want ENOSPC; the scenario did not reproduce",
			paths.RuntimeRoot, err)
	}
	return problems
}

// rootsOnAReadOnlyFilesystem is the same question asked of a mount that refuses
// every write. The directories exist by now and their mode bits are correct, so
// nothing but the mount flags can produce the right answer.
func rootsOnAReadOnlyFilesystem(mount string, paths filesystem.RuntimePaths) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if err := remountReadOnly(mount); err != nil {
		return []string{err.Error()}
	}
	defer func() {
		if err := remountReadWrite(mount); err != nil {
			problems = append(problems, err.Error())
		}
	}()

	// The premise, checked rather than assumed: every assertion below is about a
	// chmod being impossible, which means nothing unless it really is.
	if err := os.Chmod(paths.RuntimeRoot, 0o777); err == nil {
		return []string{fmt.Sprintf("chmod on %s succeeded; the mount is not read-only "+
			"and the scenario did not reproduce", paths.RuntimeRoot)}
	} else if !errors.Is(err, syscall.EROFS) {
		return []string{fmt.Sprintf("chmod on %s = %v, want EROFS", paths.RuntimeRoot, err)}
	}

	for _, w := range paths.ProbeRoots() {
		if w.Usable {
			report("on a read-only filesystem, %s = %+v, want it refused", w.Kind, w)
			continue
		}
		checkReadOnlyRemedy(report, w.Kind, w.Err)
	}

	if err := paths.EnsureDirs(); err != nil {
		// The directories already exist, so EnsureDirs succeeds here -- it creates
		// nothing. That is correct and worth stating: a create that has nothing to
		// do is not a failure, and reporting one would fail every command run
		// against a repository on read-only media that only ever reads.
		report("EnsureDirs over directories that already exist = %v, want no error", err)
	}
	return problems
}

// checkSpaceRemedy asks the same questions of every sentence the full-disk
// condition produces, which is what keeps the probe and the create from drifting
// apart.
func checkSpaceRemedy(report func(string, ...any), kind filesystem.RootKind, err error) {
	payload, ok := app.PayloadOf(err)
	if !ok {
		report("%s: the refusal %v carries no domain payload", kind, err)
		return
	}
	if payload.Code != app.CodeRuntimePathUnwritable {
		report("%s: payload.Code = %q, want %q", kind, payload.Code, app.CodeRuntimePathUnwritable)
	}
	if !strings.Contains(payload.Why, "full") {
		report("%s: Why = %q, want it to say the filesystem is full", kind, payload.Why)
	}

	remedy := strings.Join(payload.NextAction, " ")
	if !strings.Contains(remedy, "free space") {
		report("%s: NextAction = %q, want it to ask for space", kind, payload.NextAction)
	}
	if strings.Contains(remedy, "check the permissions") || strings.Contains(remedy, "chmod") {
		report("%s: NextAction = %q, which prescribes a permission change for a full disk",
			kind, payload.NextAction)
	}
	if strings.Contains(remedy, "mindrail init") {
		report("%s: NextAction = %q, which sends the user to the command that fails on this condition",
			kind, payload.NextAction)
	}
}

// checkReadOnlyRemedy is the same, for the mount that refuses writes.
func checkReadOnlyRemedy(report func(string, ...any), kind filesystem.RootKind, err error) {
	payload, ok := app.PayloadOf(err)
	if !ok {
		report("%s: the refusal %v carries no domain payload", kind, err)
		return
	}

	remedy := strings.Join(payload.NextAction, " ")
	if !strings.Contains(remedy, "remount") {
		report("%s: NextAction = %q, want it to offer the one action that works", kind, payload.NextAction)
	}
	if strings.Contains(remedy, "check the permissions") || strings.Contains(remedy, "chmod") {
		report("%s: NextAction = %q, which prescribes a permission change on a filesystem "+
			"where chmod fails with the same EROFS that refused the write", kind, payload.NextAction)
	}
	if strings.Contains(remedy, "free space") {
		report("%s: NextAction = %q, which prescribes freeing space for a read-only mount",
			kind, payload.NextAction)
	}
}

// remountReadOnly makes the whole mount refuse writes. It binds the mount over
// itself first because a tmpfs mounted inside a user namespace carries uid
// options a plain `remount,ro` refuses to re-parse; the bind produces the same
// EROFS on every write and the same failure from chmod.
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

// fillToZero grows path until the filesystem refuses, in shrinking steps so the
// last block really is taken.
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

// leaveFree gives headroom bytes back, which is how the nearly-full state is
// arranged without guessing at the overhead of everything else on the mount.
func leaveFree(path string, headroom int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat filler %s: %w", path, err)
	}
	size := max(info.Size()-headroom, 0)
	if err := os.Truncate(path, size); err != nil {
		return fmt.Errorf("truncate filler %s: %w", path, err)
	}
	return nil
}

// availableBytes is the scenario's own reading, taken through separate code from
// the probe's so that a probe agreeing with itself proves nothing.
func availableBytes(dir string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}
