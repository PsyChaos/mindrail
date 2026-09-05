//go:build smoke

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
)

// signalSlack is how much longer than the budget the test is willing to wait
// before calling the shutdown unbounded. It absorbs process start-up and the
// scheduler, not a slow shutdown.
const signalSlack = 3 * time.Second

// TestGracefulShutdownOnSIGINT covers tech-stack §88.
//
// The slow step is injected by putting a git on PATH that never answers, which
// is what a hung network filesystem looks like from the binary's side. That
// keeps the production code free of a test hook while still exercising the
// path that matters: an interrupt has to cancel the subprocess, close SQLite
// and return, all within the bounded time.
func TestGracefulShutdownOnSIGINT(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals and process groups are not available on Windows")
	}

	binary := buildBinary(t)
	repo := newRepo(t)
	home := t.TempDir()

	if got := runBinary(t, binary, repo, home, "init"); got.code != 0 {
		t.Fatalf("init exited %d\n%s\n%s", got.code, got.stdout, got.stderr)
	}

	marker := filepath.Join(t.TempDir(), "git-started")
	slowGitDir := writeSlowGit(t, marker)

	cmd := exec.Command(binary, "status")
	cmd.Dir = repo
	cmd.Env = append(subprocessEnv(home), "PATH="+slowGitDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Its own process group, so an orphaned grandchild is observable after the
	// binary is gone rather than being adopted invisibly by the test runner.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start the binary: %v", err)
	}
	pgid := cmd.Process.Pid

	if !waitFor(t, 20*time.Second, func() bool { _, err := os.Stat(marker); return err == nil }) {
		_ = cmd.Process.Kill()
		t.Fatalf("the binary never reached the git step\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}

	// Without this the test would pass just as happily against a binary that
	// exited on its own: the whole point is that it was blocked when the signal
	// arrived.
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("the binary exited before it was interrupted (%v)\nstdout:\n%s\nstderr:\n%s",
			err, stdout.String(), stderr.String())
	}

	interruptedAt := time.Now()
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal the binary: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	budget := app.ShutdownTimeout + signalSlack
	select {
	case <-done:
	case <-time.After(budget):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		t.Fatalf("the binary did not exit within %s of SIGINT\nstdout:\n%s\nstderr:\n%s",
			budget, stdout.String(), stderr.String())
	}

	if elapsed := time.Since(interruptedAt); elapsed > budget {
		t.Errorf("shutdown took %s, want at most %s", elapsed, budget)
	}

	// The interrupted git must be gone with it: a hung subprocess that outlives
	// its parent is exactly what WaitDelay and CommandContext exist to prevent.
	if !waitFor(t, 2*time.Second, func() bool { return syscall.Kill(-pgid, 0) != nil }) {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		t.Error("a process from the binary's process group outlived it")
	}

	// The database has to survive the interrupt intact; a shutdown that dropped
	// SQLite mid-write would show up here and nowhere else.
	got := runBinary(t, binary, repo, home, "doctor", "--json")
	if got.code != 0 {
		t.Fatalf("doctor after the interrupt exited %d\n%s\n%s", got.code, got.stdout, got.stderr)
	}
	assertSQLiteCheckHealthy(t, got.stdout)
}

// writeSlowGit puts a git on PATH that records that it ran and then never
// answers.
func writeSlowGit(t *testing.T, marker string) string {
	t.Helper()

	dir := t.TempDir()
	script := "#!/bin/sh\ntouch " + shellQuote(marker) + "\nexec sleep 300\n"
	path := filepath.Join(dir, "git")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write the slow git: %v", err)
	}
	return dir
}

// shellQuote is safe here because the only value it ever sees is a path this
// test just created under t.TempDir(); nothing user-supplied reaches it.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func assertSQLiteCheckHealthy(t *testing.T, stdout string) {
	t.Helper()

	var envelope struct {
		Data struct {
			Checks []struct {
				Name  string `json:"name"`
				State string `json:"state"`
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode doctor output: %v\n%s", err, stdout)
	}

	for _, check := range envelope.Data.Checks {
		if check.Name != "sqlite" {
			continue
		}
		if check.State != "OK" {
			t.Errorf("sqlite check = %q after the interrupt, want OK\n%s", check.State, stdout)
		}
		return
	}
	t.Errorf("doctor reported no sqlite check:\n%s", stdout)
}
