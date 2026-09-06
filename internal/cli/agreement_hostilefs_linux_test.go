//go:build linux

package cli_test

import (
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
	"github.com/PsyChaos/mindrail/internal/cli"
)

// hostileMountEnv carries the mount point from the parent process to the child
// that actually runs the rows. Its presence is also how the test knows which of
// the two it is.
const hostileMountEnv = "MINDRAIL_TEST_CLI_HOSTILE_MOUNT"

// hostileMountSize is small on purpose: the rows have to fill it, and filling a
// large tmpfs is slow and hostile to whoever else is on the machine.
const hostileMountSize = "size=8m"

// TestAgreementOnAHostileFilesystem is the rest of the broken-setup matrix: the
// four conditions that cannot be arranged with a chmod.
//
// A full filesystem and a read-only mount are the two states where the mode bits
// are *correct* and the write is refused anyway, which is exactly why they were
// missing from the matrix for six audits: every other row is a permission or an
// obstruction, and both of those can be set up in a temporary directory. These
// need a filesystem of their own, so the test mounts one — in a mount namespace,
// so nothing outside this process sees it — and drives the real command tree
// against a repository living on it.
//
// The nearly-full row is the over-fire guard, and it is the important half. Most
// developers' disks are nearly full most of the time, and a space check that
// refused there would report every one of them as broken.
func TestAgreementOnAHostileFilesystem(t *testing.T) {
	if mount := os.Getenv(hostileMountEnv); mount != "" {
		// Removed as soon as it has been read. It is spelled with the MINDRAIL_
		// prefix because it belongs to this program, and every fixture in this
		// package refuses to run against a shell that exports one — a stale
		// export would change the effective configuration and make the
		// assertions depend on whoever ran the suite. Leaving it set would have
		// made every row skip, which is what it did.
		if err := os.Unsetenv(hostileMountEnv); err != nil {
			t.Fatalf("Unsetenv %s = %v, want no error", hostileMountEnv, err)
		}
		runHostileAgreement(t, mount)
		return
	}
	spawnHostileAgreement(t)
}

// spawnHostileAgreement re-runs this one test inside a user and mount namespace.
//
// It re-executes the test binary rather than doing the work here because the
// states only exist inside a namespace this process cannot enter: unshare(2)
// applies to the calling process, and a Go test binary is already multi-threaded
// by the time a test runs.
func spawnHostileAgreement(t *testing.T) {
	if testing.Short() {
		t.Skip("mounts a filesystem in a user namespace")
	}
	requireGit(t)

	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("no unshare(1); a real full filesystem cannot be arranged here")
	}
	if !hostileNamespacesWork(unshare) {
		t.Skip("unprivileged user namespaces are unavailable; a real full filesystem cannot be arranged here")
	}

	binary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable = %v, want no error", err)
	}

	mount := filepath.Join(t.TempDir(), "hostilefs")
	if err := os.Mkdir(mount, 0o755); err != nil {
		t.Fatalf("Mkdir = %v, want no error", err)
	}

	cmd := exec.Command(unshare, "--user", "--map-root-user", "--mount", "--",
		binary, "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), hostileMountEnv+"="+mount)
	out, runErr := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		t.Fatalf("spawn: %v", runErr)
	}
	if cmd.ProcessState.ExitCode() != 0 {
		t.Fatalf("the hostile-filesystem agreement rows failed:\n%s", strings.TrimSpace(string(out)))
	}
}

func hostileNamespacesWork(unshare string) bool {
	truth, err := exec.LookPath("true")
	if err != nil {
		return false
	}
	return exec.Command(unshare, "--user", "--map-root-user", "--mount", "--", truth).Run() == nil
}

// hostileCondition is one state of the *filesystem* rather than of a path in it.
//
// It is shaped differently from the ordinary matrix rows for one reason: the
// mount is shared by the three commands and each of them still needs its own
// repository, so the state has to be armed after the repository is built and
// relaxed before the next one is. Relaxing it is also the remedy the report
// prints, which is what the last phase carries out.
type hostileCondition struct {
	name string
	// initialised says whether `mindrail init` runs on the repository while the
	// filesystem is still healthy.
	initialised bool
	arm         func(t *testing.T, mount string)
	relax       func(t *testing.T, mount string)
	broken      bool
	separately  map[string]string
}

func runHostileAgreement(t *testing.T, mount string) {
	if err := syscall.Mount("tmpfs", mount, "tmpfs", 0, hostileMountSize); err != nil {
		t.Skipf("mount tmpfs on %s: %v", mount, err)
	}

	for _, tc := range hostileConditions() {
		t.Run(tc.name, func(t *testing.T) {
			answers := make(map[string]answer, len(commandsUnderTest))
			for i, command := range commandsUnderTest {
				repo := hostileRepo(t, mount, tc, fmt.Sprintf("%s-%d", strings.ReplaceAll(tc.name, " ", "-"), i))
				tc.arm(t, mount)
				got := runWith(t, repo, cli.Options{}, command, "--json")
				answers[command] = answerFrom(t, repo, command, got)
				// The other invariant, on the same document: these four
				// conditions are the ones whose remedy no chmod can carry out, so
				// they are exactly where a component quietly prescribing one
				// would cost the reader the most.
				assertDocumentIsCoherent(t, command, got.stdout)
				tc.relax(t, mount)
			}

			agreeing := agreeingHostileCommands(tc)
			first := agreeing[0]
			for _, command := range agreeing[1:] {
				assertSameAnswer(t, tc.name, first, answers[first], command, answers[command])
			}
			assertExemptionsAreStillEarned(t, tc.name, answers[first], answers, tc.separately)

			if tc.broken && answers[first].code == "" {
				t.Fatalf("%s: no command reported a failure, so the condition did not reproduce", tc.name)
			}
			if !tc.broken {
				for command, got := range answers {
					if got.code != "" || got.exit != app.ExitSuccess {
						t.Errorf("%s: %s reported %q at exit %d on a filesystem nothing is wrong with",
							tc.name, command, got.code, got.exit)
					}
				}
			}

			// The sentence all three printed, carried out: relaxing the mount is
			// "free space on the filesystem holding X" and "remount the
			// filesystem holding X read-write".
			repo := hostileRepo(t, mount, tc, strings.ReplaceAll(tc.name, " ", "-")+"-remedy")
			tc.arm(t, mount)
			tc.relax(t, mount)
			for _, command := range commandsUnderTest {
				if got := run(t, repo, command); got.code != app.ExitSuccess {
					t.Fatalf("%s: after carrying out the remedy %v, `%s` exited %d: %v\n%s",
						tc.name, answers[first].nextAction, command, got.code, got.err, got.stdout)
				}
			}
		})
	}
}

func hostileConditions() []hostileCondition {
	return []hostileCondition{
		{
			name:   "a filesystem with no space left, before init",
			broken: true,
			arm:    fillHostileMount,
			relax:  emptyHostileMount,
			separately: map[string]string{
				// `init` acts where the other two observe, and on this
				// filesystem the act succeeds one directory further than the
				// probe could see: the mkdir of `.mindrail` costs no data block,
				// so it lands, and the write of config.toml inside it is what
				// meets ENOSPC. Both remedies name the same filesystem and the
				// same instruction — free space — and they name it about
				// different directories because the disk really was different by
				// the time init looked at it. The row keeps the exemption rather
				// than comparing something weaker, because "same filesystem" is
				// not a property this test can read off two strings.
				"init": "init creates .mindrail before the write that fails, so it names the deeper directory it created",
			},
		},
		{
			name:        "a filesystem with no space left, after init",
			initialised: true,
			broken:      true,
			arm:         fillHostileMount,
			relax:       emptyHostileMount,
		},
		{
			// The over-fire guard, and the state most machines are in.
			name:        "a nearly full filesystem",
			initialised: true,
			arm:         nearlyFillHostileMount,
			relax:       emptyHostileMount,
		},
		{
			name:   "a read-only mount, before init",
			broken: true,
			arm:    remountHostileReadOnly,
			relax:  remountHostileReadWrite,
		},
		{
			name:        "a read-only mount, after init",
			initialised: true,
			broken:      true,
			arm:         remountHostileReadOnly,
			relax:       remountHostileReadWrite,
		},
	}
}

func agreeingHostileCommands(tc hostileCondition) []string {
	agreeing := make([]string, 0, len(commandsUnderTest))
	for _, command := range commandsUnderTest {
		if _, exempt := tc.separately[command]; !exempt {
			agreeing = append(agreeing, command)
		}
	}
	if len(agreeing) == 0 {
		return slices.Clone(commandsUnderTest)
	}
	return agreeing
}

// hostileRepo builds one repository on the mount while it is still healthy.
func hostileRepo(t *testing.T, mount string, tc hostileCondition, name string) string {
	t.Helper()
	isolateEnvironment(t)

	repo := filepath.Join(mount, name)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("create %s: %v", repo, err)
	}
	if out, err := exec.Command("git", "init", "--quiet", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if tc.initialised {
		if got := run(t, repo, "init"); got.code != app.ExitSuccess {
			t.Fatalf("preparing %q: init exited %d: %v\n%s", tc.name, got.code, got.err, got.stdout)
		}
	}
	return repo
}

// hostileFiller is the file that takes up the space. It is one path so that
// emptying the mount is a single remove, which is what makes "free space on the
// filesystem" a remedy this test can carry out.
func hostileFillerPath(mount string) string { return filepath.Join(mount, "filler") }

func fillHostileMount(t *testing.T, mount string) {
	t.Helper()
	fillHostileMountTo(t, mount, 0)

	if free := hostileAvailableBytes(mount); free != 0 {
		t.Fatalf("the filesystem reports %d bytes available; the row needs it genuinely full", free)
	}
}

// nearlyFillHostileMount leaves a little room, which is the adjacent condition:
// nearly full is not full, and a probe that cannot tell them apart reports every
// working machine as broken.
func nearlyFillHostileMount(t *testing.T, mount string) {
	t.Helper()
	fillHostileMountTo(t, mount, 0)

	const headroom = 512 << 10
	info, err := os.Stat(hostileFillerPath(mount))
	if err != nil {
		t.Fatalf("stat the filler: %v", err)
	}
	if err := os.Truncate(hostileFillerPath(mount), max(info.Size()-headroom, 0)); err != nil {
		t.Fatalf("give headroom back: %v", err)
	}
	if free := hostileAvailableBytes(mount); free <= 0 {
		t.Fatalf("could not arrange a nearly-full filesystem: %d bytes available", free)
	}
}

func fillHostileMountTo(t *testing.T, mount string, _ int64) {
	t.Helper()

	file, err := os.OpenFile(hostileFillerPath(mount), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open the filler: %v", err)
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
}

func emptyHostileMount(t *testing.T, mount string) {
	t.Helper()
	if err := os.Remove(hostileFillerPath(mount)); err != nil && !os.IsNotExist(err) {
		t.Fatalf("carry out the remedy (free space): %v", err)
	}
}

// remountHostileReadOnly binds the mount over itself before remounting, because
// a tmpfs mounted inside a user namespace carries uid options a plain
// `remount,ro` refuses to re-parse. The bind produces the same EROFS on every
// write, and the same EROFS from chmod — which is the whole point of the row.
func remountHostileReadOnly(t *testing.T, mount string) {
	t.Helper()
	if err := syscall.Mount(mount, mount, "", syscall.MS_BIND, ""); err != nil {
		t.Fatalf("bind %s over itself: %v", mount, err)
	}
	if err := syscall.Mount("", mount, "", syscall.MS_REMOUNT|syscall.MS_BIND|syscall.MS_RDONLY, ""); err != nil {
		t.Fatalf("remount %s read-only: %v", mount, err)
	}
}

func remountHostileReadWrite(t *testing.T, mount string) {
	t.Helper()
	if err := syscall.Mount("", mount, "", syscall.MS_REMOUNT|syscall.MS_BIND, ""); err != nil {
		t.Fatalf("carry out the remedy (remount read-write): %v", err)
	}
	if err := syscall.Unmount(mount, 0); err != nil {
		t.Fatalf("drop the read-only bind on %s: %v", mount, err)
	}
}

// hostileAvailableBytes is the row's own reading, taken through separate code
// from the probe's so that a probe agreeing with itself proves nothing.
func hostileAvailableBytes(dir string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}
