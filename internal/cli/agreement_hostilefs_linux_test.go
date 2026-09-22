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
	spawnHostileChild(t, hostileMountEnv)
}

// spawnHostileChild re-runs the calling test inside a user and mount namespace.
//
// It re-executes the test binary rather than doing the work here because the
// states only exist inside a namespace this process cannot enter: unshare(2)
// applies to the calling process, and a Go test binary is already multi-threaded
// by the time a test runs.
func spawnHostileChild(t *testing.T, mountEnv string) {
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
	cmd.Env = append(os.Environ(), mountEnv+"="+mount)
	out, runErr := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if runErr != nil && !errors.As(runErr, &exitErr) {
		t.Fatalf("spawn: %v", runErr)
	}
	if cmd.ProcessState.ExitCode() != 0 {
		t.Fatalf("the hostile-filesystem rows failed:\n%s", strings.TrimSpace(string(out)))
	}

	// A child that never ran the rows also exits 0, and three separate ways of
	// not running them — a mount the kernel refused, a -test.run that matches
	// nothing, a `go test -short` — all reported success. That is the worst
	// failure a test can have: this is the only place the F01 condition is
	// reproduced at all, and it was reporting that it had reproduced it while
	// doing nothing. The child says out loud how many rows it ran, and the
	// parent refuses an answer that does not include the sentence.
	if !strings.Contains(string(out), hostileRowsRanMarker) {
		t.Fatalf("the child exited 0 without running any rows; it never reported %q:\n%s",
			hostileRowsRanMarker, strings.TrimSpace(string(out)))
	}
}

// hostileRowsRanMarker is the child's proof of work. It is a fixed string rather
// than a count so the parent's check cannot drift as rows are added.
const hostileRowsRanMarker = "MINDRAIL_HOSTILE_ROWS_RAN"

// reportHostileRowsRan is called by a child once it has actually driven the
// commands, on the far side of every skip.
func reportHostileRowsRan(t *testing.T, rows *int) {
	t.Helper()
	if *rows == 0 {
		t.Fatal("no rows ran, so the scenario proves nothing")
	}
	fmt.Printf("%s %d\n", hostileRowsRanMarker, *rows)
}

// rowRan is the LAST statement of a row body, and it is deliberately not
// deferred.
//
// Counting at the top of the loop counted iterations rather than work: a row
// that skipped as its first statement still reported itself as having run.
// Deferring the count did not fix that — `t.Skip` unwinds through Goexit, so
// deferred calls run — and only a plain call on the far side of the assertions
// is reached by a row that actually made them. A row that fails aborts before it
// too, which costs nothing: a failing row already fails the test.
func rowRan(counter *int) { *counter++ }

// readyBandMountEnv is the second mount handed to a namespace child, for the
// rows below. It is a separate variable so that one child runs one test.
const readyBandMountEnv = "MINDRAIL_TEST_CLI_READY_BAND_MOUNT"

// TestInitNeverCallsARepositoryReadyTheNextCommandRefuses is finding F01.
//
// `mindrail init` used to take the reading its terminal line is derived from
// while its own last write was still ahead of it: the write-ahead log was on
// disk and the checkpoint that folds it back into the database file had not
// happened yet. On a filesystem with room for the log and not for the
// checkpoint, that reading was taken over a few kilobytes that were about to be
// spent, and init printed READY FOR TARGETED WORK at exit 0 over a repository
// `status`, run a moment later on bytes nothing had touched in between, refused
// at exit 4. A CI step gated on `mindrail init` passed on a machine where every
// later Mindrail command failed.
//
// The invariant is deliberately weaker than "init succeeds": on a filesystem
// this close to full, init failing is a correct answer and so is init
// succeeding. What it may not do is disagree with the command that runs next
// over a disk neither of them changed.
//
// The headroom is swept rather than named. The band's edges depend on the page
// size, on how many migrations ship in this binary and on how much log SQLite
// decides to keep, so a single number would be a row that stops reproducing the
// first time any of the three moves. The wide end is the over-fire guard: at
// a megabyte free everything has to work.
//
// The edge has been measured twice, on this tmpfs with 4 KiB pages, by
// sweeping in 16 KiB steps: with two migrations `init` fits at 240 KiB and
// not at 224; with three (MR-004's 000003: two tables, two indexes, one ADD
// COLUMN) it fits at 288 KiB and not at 272. The wide end was a quarter of a
// megabyte until the third migration crossed it (MR-004, TASK-02), then half
// a megabyte until the sixth migration crossed it (MR-007, TASK-01: five
// tables, three indexes); the edge now lies past 512 KiB, so the wide end
// moves to a megabyte with margin rather than to a freshly swept edge.
func TestInitNeverCallsARepositoryReadyTheNextCommandRefuses(t *testing.T) {
	if mount := os.Getenv(readyBandMountEnv); mount != "" {
		if err := os.Unsetenv(readyBandMountEnv); err != nil {
			t.Fatalf("Unsetenv %s = %v, want no error", readyBandMountEnv, err)
		}
		runReadyBand(t, mount)
		return
	}
	spawnHostileChild(t, readyBandMountEnv)
}

func runReadyBand(t *testing.T, mount string) {
	if err := syscall.Mount("tmpfs", mount, "tmpfs", 0, hostileMountSize); err != nil {
		// Fatal rather than Skip: the parent cannot tell a skip from a pass, and
		// a child that could not mount has not answered the question.
		t.Fatalf("mount tmpfs on %s: %v", mount, err)
	}

	ran := 0
	t.Cleanup(func() { reportHostileRowsRan(t, &ran) })

	for _, headroom := range []int64{8, 24, 44, 48, 80, 112, 120, 124, 128, 132, 140, 148, 224, 240, 272, 288, 512, 1024} {
		t.Run(fmt.Sprintf("%d KiB free", headroom), func(t *testing.T) {
			repo := hostileRepo(t, mount, hostileCondition{}, fmt.Sprintf("ready-band-%d", headroom))
			leaveHostileHeadroom(t, mount, headroom<<10)
			t.Cleanup(func() { emptyHostileMount(t, mount) })

			gotInit := runWith(t, repo, cli.Options{}, "init", "--json")
			initAnswer := answerFrom(t, repo, "init", gotInit)

			gotStatus := runWith(t, repo, cli.Options{}, "status", "--json")
			statusAnswer := answerFrom(t, repo, "status", gotStatus)

			if initAnswer.exit == app.ExitSuccess && statusAnswer.exit != app.ExitSuccess {
				t.Fatalf("with %d KiB free, `init` exited 0 and `status` exited %d (%s) on the same disk;\n"+
					"init said: %s\nstatus said: %s",
					headroom, statusAnswer.exit, statusAnswer.code, gotInit.stdout, gotStatus.stdout)
			}
			if initAnswer.exit != app.ExitSuccess && statusAnswer.exit != app.ExitSuccess {
				assertSameAnswer(t, fmt.Sprintf("%d KiB free", headroom),
					"init", initAnswer, "status", statusAnswer)
			}

			// The other direction, which was unasserted: `init` refused and
			// `status` content. It is allowed — `status` observes a repository
			// that has genuinely not been initialised, and decision D-03 makes
			// that exit 0 — but only if its remedy is one a reader can act on.
			// "Run `mindrail init`" is the command that just failed, so the row
			// insists the reader be told what will stop it.
			if initAnswer.exit != app.ExitSuccess && statusAnswer.exit == app.ExitSuccess {
				assertTheLoopTerminates(t, headroom, repo, initAnswer)
			}

			// The wide end is the over-fire guard: a megabyte is room
			// enough for everything six migrations write, measured above,
			// and a change that made init pessimistic would show up here
			// rather than in production.
			if headroom == 1024 && initAnswer.exit != app.ExitSuccess {
				t.Fatalf("with %d KiB free, `init` exited %d (%s); there is room for everything it writes\n%s",
					headroom, initAnswer.exit, initAnswer.code, gotInit.stdout)
			}

			// The remedy, carried out: freeing the space has to end the
			// condition for a repository init has already been run in.
			if initAnswer.exit != app.ExitSuccess {
				emptyHostileMount(t, mount)
				for _, command := range commandsUnderTest {
					if got := run(t, repo, command); got.code != app.ExitSuccess {
						t.Fatalf("after carrying out the remedy %v, `%s` exited %d: %v\n%s",
							initAnswer.nextAction, command, got.code, got.err, got.stdout)
					}
				}
			}

			rowRan(&ran)
		})
	}
}

// assertTheLoopTerminates holds the one case where `init` and `status` are
// allowed to disagree.
//
// On a filesystem too full for `init` and not full enough for the space probe to
// see it, `status` reports an uninitialised repository at exit 0 and offers
// `mindrail init` — the command that just failed. That is the shape of finding
// F12, and the thing that makes it survivable rather than a dead end is that
// running it says something the reader can act on. The claim is therefore not
// "the three commands always agree", which is false here; it is that following
// what they say gets the reader out, and that is what this checks: run init,
// read what it says, do it, and require the repository to work.
func assertTheLoopTerminates(t *testing.T, headroom int64, repo string, initAnswer answer) {
	t.Helper()

	if len(initAnswer.nextAction) == 0 {
		t.Fatalf("with %d KiB free, `init` refused with no remedy at all while `status` "+
			"sent the reader to `mindrail init`; there is nowhere to go from here", headroom)
	}
	if initAnswer.code == "" {
		t.Fatalf("with %d KiB free, `init` refused with no code", headroom)
	}

	// The remedy `init` printed, carried out. Everything the band can produce
	// here is a space condition, so freeing the space is the action; a remedy
	// that named something else would be caught by the class assertion in the
	// agreement matrix rather than here.
	emptyHostileMount(t, filepath.Dir(repo))
	for _, command := range commandsUnderTest {
		if got := run(t, repo, command); got.code != app.ExitSuccess {
			t.Fatalf("with %d KiB free, after carrying out `init`'s remedy %v, `%s` exited %d: %v\n%s",
				headroom, initAnswer.nextAction, command, got.code, got.err, got.stdout)
		}
	}
}

// leaveHostileHeadroom fills the mount and then gives exactly headroom bytes
// back, which is how a precise amount of free space is arranged without having
// to know the overhead of everything else already on it.
func leaveHostileHeadroom(t *testing.T, mount string, headroom int64) {
	t.Helper()
	fillHostileMountTo(t, mount, 0)

	info, err := os.Stat(hostileFillerPath(mount))
	if err != nil {
		t.Fatalf("stat the filler: %v", err)
	}
	if err := os.Truncate(hostileFillerPath(mount), max(info.Size()-headroom, 0)); err != nil {
		t.Fatalf("give headroom back: %v", err)
	}
	if free := hostileAvailableBytes(mount); free <= 0 {
		t.Fatalf("wanted %d bytes free, the filesystem reports %d", headroom, free)
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
		// Fatal rather than Skip, for the reason in runReadyBand.
		t.Fatalf("mount tmpfs on %s: %v", mount, err)
	}

	ran := 0
	t.Cleanup(func() { reportHostileRowsRan(t, &ran) })

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

			rowRan(&ran)
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
