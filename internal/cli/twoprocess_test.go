package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/cli"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// The races in this file are run by two `mindrail` processes — this test
// binary re-executed as the command tree — and not by two goroutines, for
// the reason open_contention_test.go gives: the contention this test is about
// only exists between processes. SQLite's locks are held per process, and
// two goroutines in one process share the winner's connection state and
// never see the SQLITE_BUSY that a second `mindrail` on the same repository
// sees. Two `run` calls in goroutines would prove that one process serialises
// two of its own handles, which the coordination tests prove already, and
// not what REQ-10 says: two agents, each its own process, on one repository.
//
// Every child is started blocked on its standard input and says `ready` on
// its standard error once it is; the parent closes every input back to back,
// which is as close to one instant as two processes get, and each child runs
// its command line the moment its input closes.
const (
	childModeEnv = "MINDRAIL_TEST_CLI_CHILD"       // `command` runs the tree over the arguments; `hold` holds the write lock
	childBusyEnv = "MINDRAIL_TEST_CLI_BUSY_BUDGET" // the command child's busy budget, a Go duration; empty is the production budget
	childDBEnv   = "MINDRAIL_TEST_CLI_HOLD_DB"     // the database the hold child locks
)

// TestMain dispatches the re-execs before it runs anything, as
// open_contention_test.go's does.
func TestMain(m *testing.M) {
	switch os.Getenv(childModeEnv) {
	case "command":
		os.Exit(runCommandChild())
	case "hold":
		os.Exit(runHoldChild())
	}
	os.Exit(m.Run())
}

// runCommandChild is one racer: the command tree over the process's
// arguments, started when standard input closes, exiting with the command's
// exit code. Its own variables are dropped from the environment first,
// because the tree warns about any MINDRAIL_ variable it does not know.
func runCommandChild() int {
	budget, err := time.ParseDuration(os.Getenv(childBusyEnv))
	if err != nil {
		budget = 0
	}
	for _, name := range []string{childModeEnv, childBusyEnv, childDBEnv} {
		_ = os.Unsetenv(name)
	}
	fmt.Fprintln(os.Stderr, "ready")
	_, _ = io.Copy(io.Discard, os.Stdin)

	root := cli.NewRootWith(cli.Options{BusyTimeout: budget})
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetArgs(os.Args[1:])
	return app.ExitCode(root.ExecuteContext(context.Background()))
}

// runHoldChild takes the write lock on the database the environment names
// and keeps it until standard input closes, then commits: a process in the
// middle of a write, which is what AC-10.5's contender has to wait behind.
func runHoldChild() int {
	db, err := storage.Open(context.Background(), storage.Options{Path: os.Getenv(childDBEnv)})
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		return 1
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "begin:", err)
		return 1
	}
	fmt.Fprintln(os.Stderr, "ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	if err := tx.Commit(); err != nil {
		fmt.Fprintln(os.Stderr, "commit:", err)
		return 1
	}
	return 0
}

// child is one re-executed process the parent is holding at its barrier.
type child struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   bytes.Buffer
	stderr   readiness
	released time.Time
}

// readiness is the child's standard error: it releases the parent on the
// `ready` line and keeps everything the child said.
type readiness struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	once  sync.Once
	ready chan struct{}
}

func (r *readiness) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf.Write(p)
	if bytes.Contains(r.buf.Bytes(), []byte("ready\n")) {
		r.once.Do(func() { close(r.ready) })
	}
	return len(p), nil
}

func (r *readiness) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

// startChild re-executes this binary in the mode given, with `env` added,
// and returns once the child is at its barrier.
func startChild(t *testing.T, mode string, env []string, args ...string) *child {
	t.Helper()
	if testing.Short() {
		t.Skip("spawns processes")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable = %v, want no error", err)
	}

	c := &child{cmd: exec.Command(binary, args...)}
	c.stderr.ready = make(chan struct{})
	c.cmd.Env = append(append(os.Environ(), childModeEnv+"="+mode), env...)
	c.cmd.Stdout = &c.stdout
	c.cmd.Stderr = &c.stderr
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe = %v, want no error", err)
	}
	if err := c.cmd.Start(); err != nil {
		t.Fatalf("starting the child: %v", err)
	}
	t.Cleanup(func() {
		_ = c.stdin.Close()
		if c.cmd.ProcessState == nil {
			_ = c.cmd.Process.Kill()
			_ = c.cmd.Wait()
		}
	})

	select {
	case <-c.stderr.ready:
	case <-time.After(30 * time.Second):
		t.Fatalf("the child never said ready:\n%s", c.stderr.String())
	}
	return c
}

// commandChild is a racer over one mindrail command line in `repo`, under
// the busy budget given (zero is the production budget).
func commandChild(t *testing.T, repo string, budget time.Duration, args ...string) *child {
	t.Helper()
	env := []string{}
	if budget > 0 {
		env = append(env, childBusyEnv+"="+budget.String())
	}
	return startChild(t, "command", env, append(args, "-C", repo)...)
}

// release lets the child run.
func (c *child) release() {
	c.released = time.Now()
	_ = c.stdin.Close()
}

// wait collects the child's result the way `run` reports one.
func (c *child) wait(t *testing.T) result {
	t.Helper()
	err := c.cmd.Wait()
	return result{
		stdout: c.stdout.String(),
		stderr: c.stderr.String(),
		err:    err,
		code:   c.cmd.ProcessState.ExitCode(),
	}
}

// race releases every child in one go and returns their results in order.
func race(t *testing.T, children ...*child) []result {
	t.Helper()
	for _, c := range children {
		c.release()
	}
	results := make([]result, len(children))
	for i, c := range children {
		results[i] = c.wait(t)
	}
	return results
}

// countWhere is one COUNT over the runtime database, read-only.
func countWhere(t *testing.T, repo, query string, args ...any) int {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo), ReadOnly: true})
	if err != nil {
		t.Fatalf("opening the runtime database: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// racedTask is the part of `task state --json` the races read.
type racedTask struct {
	Task struct {
		ID        string `json:"task_id"`
		State     string `json:"state"`
		ClaimedBy string `json:"claimed_by"`
		Revision  int64  `json:"revision"`
	} `json:"task"`
	Lease *struct {
		ID     string `json:"lease_id"`
		Holder string `json:"holder"`
		Status string `json:"status"`
	} `json:"lease"`
	Superseded *struct {
		ID            string `json:"lease_id"`
		Holder        string `json:"holder"`
		Status        string `json:"status"`
		ReleaseReason string `json:"release_reason"`
	} `json:"superseded"`
	Replayed bool `json:"replayed"`
}

// winnerAndLoser sorts two racers' results into the one that exited 0 and
// the one that did not, and fails unless there is exactly one of each.
func winnerAndLoser(t *testing.T, what string, results []result) (winner, loser result) {
	t.Helper()
	var won, lost []result
	for _, r := range results {
		if r.code == app.ExitSuccess {
			won = append(won, r)
		} else {
			lost = append(lost, r)
		}
	}
	if len(won) != 1 || len(lost) != 1 {
		t.Fatalf("%s: %d processes succeeded and %d were refused, want one of each:\n%s\n%s",
			what, len(won), len(lost), results[0].stdout+results[0].stderr, results[1].stdout+results[1].stderr)
	}
	return won[0], lost[0]
}

// TestTwoProcessesRaceToClaimOneTask is AC-10.1: two `mindrail` processes
// move one OPEN task to CLAIMED at once; one gets the task and its lease,
// the other is refused with the winner named, and the table holds one
// unreleased lease for the task.
func TestTwoProcessesRaceToClaimOneTask(t *testing.T) {
	repo := newInitializedRepo(t)
	first, second := sessionID(t, repo), sessionID(t, repo)
	task := openTask(t, repo, first, "raced")
	sessionsBefore := rowCount(t, repo, "sessions")

	results := race(t,
		commandChild(t, repo, 0, "task", "state", task, "--to", "CLAIMED", "--session", first, "--json"),
		commandChild(t, repo, 0, "task", "state", task, "--to", "CLAIMED", "--session", second, "--json"),
	)
	winner, loser := winnerAndLoser(t, "task state --to CLAIMED", results)

	var won racedTask
	decodeData(t, winner.stdout, &won)
	if won.Task.State != string(coordination.StateClaimed) || won.Lease == nil || won.Lease.Holder != won.Task.ClaimedBy || won.Lease.Status != "active" {
		t.Errorf("the winner's answer = %+v, want CLAIMED with an active lease held by the claimant", won)
	}
	if won.Task.ClaimedBy != first && won.Task.ClaimedBy != second {
		t.Errorf("claimed_by = %q, want one of the two racing sessions", won.Task.ClaimedBy)
	}

	if loser.code != app.ExitFailed {
		t.Errorf("the loser exited %d, want %d", loser.code, app.ExitFailed)
	}
	payload := loser.errorPayload(t)
	if payload.Code != app.CodeLeaseConflict {
		t.Errorf("the loser's code = %s, want %s: the winner's lease is judged before the state table (D-67)", payload.Code, app.CodeLeaseConflict)
	}
	if payload.Metadata["holder"] != won.Task.ClaimedBy {
		t.Errorf("the loser was told holder=%q, want the winner %q", payload.Metadata["holder"], won.Task.ClaimedBy)
	}

	if n := countWhere(t, repo, `SELECT count(*) FROM leases WHERE target_kind = 'task' AND target_key = ? AND released_at IS NULL`, task); n != 1 {
		t.Errorf("%d unreleased lease rows for the task, want exactly 1", n)
	}
	if n := countWhere(t, repo, `SELECT count(*) FROM leases WHERE holder = ?`, won.Task.ClaimedBy); n != 1 {
		t.Errorf("%d lease rows held by the winner, want 1", n)
	}
	if n := countWhere(t, repo, `SELECT count(*) FROM tasks WHERE task_id = ? AND claimed_by = ? AND state = 'CLAIMED' AND revision = 2`, task, won.Task.ClaimedBy); n != 1 {
		t.Errorf("the task row does not carry the winner at revision 2")
	}
	if n := rowCount(t, repo, "sessions"); n != sessionsBefore {
		t.Errorf("the race minted %d sessions; both racers named theirs", n-sessionsBefore)
	}
}

// TestTwoProcessesRaceForOneFileAndShareTwo is AC-10.2: `lease acquire
// --file` on one path from two processes admits one; on two different paths
// both are admitted — different-file concurrency, at file grain.
func TestTwoProcessesRaceForOneFileAndShareTwo(t *testing.T) {
	repo := newInitializedRepo(t)
	first, second := sessionID(t, repo), sessionID(t, repo)

	results := race(t,
		commandChild(t, repo, 0, "lease", "acquire", "--file", "src/same.go", "--session", first, "--json"),
		commandChild(t, repo, 0, "lease", "acquire", "--file", "src/same.go", "--session", second, "--json"),
	)
	winner, loser := winnerAndLoser(t, "lease acquire --file src/same.go", results)

	var won struct {
		Lease struct {
			ID     string `json:"lease_id"`
			Holder string `json:"holder"`
			Key    string `json:"target_key"`
		} `json:"lease"`
		Renewed bool `json:"renewed"`
	}
	decodeData(t, winner.stdout, &won)
	if won.Lease.Key != "src/same.go" || won.Renewed || (won.Lease.Holder != first && won.Lease.Holder != second) {
		t.Errorf("the winner's lease = %+v, want a fresh lease on src/same.go held by a racer", won)
	}
	payload := loser.errorPayload(t)
	if payload.Code != app.CodeLeaseConflict || payload.Metadata["holder"] != won.Lease.Holder || payload.Metadata["lease_id"] != won.Lease.ID {
		t.Errorf("the loser got %s %v, want %s naming the winner's session and lease", payload.Code, payload.Metadata, app.CodeLeaseConflict)
	}
	if n := countWhere(t, repo, `SELECT count(*) FROM leases WHERE target_kind = 'file' AND target_key = 'src/same.go' AND released_at IS NULL`); n != 1 {
		t.Errorf("%d unreleased lease rows for src/same.go, want exactly 1", n)
	}

	both := race(t,
		commandChild(t, repo, 0, "lease", "acquire", "--file", "src/left.go", "--session", first, "--json"),
		commandChild(t, repo, 0, "lease", "acquire", "--file", "src/right.go", "--session", second, "--json"),
	)
	for i, r := range both {
		if r.code != app.ExitSuccess {
			t.Errorf("racer %d on its own file exited %d, want 0:\n%s%s", i, r.code, r.stdout, r.stderr)
		}
	}
	if n := countWhere(t, repo, `SELECT count(*) FROM leases WHERE target_kind = 'file' AND target_key IN ('src/left.go', 'src/right.go') AND released_at IS NULL`); n != 2 {
		t.Errorf("%d unreleased lease rows for the two different files, want 2", n)
	}
}

// TestAThirdProcessTakesOverAnExpiredTenure is AC-10.3: once the clock is
// past a claim's TTL — the row back-dated by SQL, since the binary runs on
// the system clock — a third session's move takes the task over, and its
// answer names the tenure it superseded.
func TestAThirdProcessTakesOverAnExpiredTenure(t *testing.T) {
	repo := newInitializedRepo(t)
	first, second, third := sessionID(t, repo), sessionID(t, repo), sessionID(t, repo)
	task := openTask(t, repo, first, "handed on")
	_, _ = winnerAndLoser(t, "task state --to CLAIMED", race(t,
		commandChild(t, repo, 0, "task", "state", task, "--to", "CLAIMED", "--session", first, "--json"),
		commandChild(t, repo, 0, "task", "state", task, "--to", "CLAIMED", "--session", second, "--json"),
	))

	// Before the TTL the third session is refused like the loser was.
	early := commandChild(t, repo, 0, "task", "state", task, "--to", "IN_PROGRESS", "--session", third, "--json")
	if refused := race(t, early)[0]; refused.code != app.ExitFailed || refused.errorPayload(t).Code != app.CodeLeaseConflict {
		t.Fatalf("a move inside the TTL exited %d, want %s at exit 1:\n%s", refused.code, app.CodeLeaseConflict, refused.stdout)
	}

	execOnRuntimeDB(t, repo, `UPDATE leases SET expires_at = '2020-01-01T00:00:00Z' WHERE target_kind = 'task' AND target_key = '`+task+`' AND released_at IS NULL`)
	late := race(t, commandChild(t, repo, 0, "task", "state", task, "--to", "IN_PROGRESS", "--session", third, "--json"))[0]
	late.requireExit(t, app.ExitSuccess)

	var took racedTask
	decodeData(t, late.stdout, &took)
	if took.Task.State != string(coordination.StateInProgress) || took.Task.ClaimedBy != third || took.Lease == nil || took.Lease.Holder != third {
		t.Errorf("the takeover's answer = %+v, want IN_PROGRESS claimed and held by the third session", took)
	}
	// The superseded tenure is closed by the takeover, so it reads as
	// released, and the reason says why: it had expired.
	if took.Superseded == nil || took.Superseded.Holder == third || took.Superseded.Status != "released" || took.Superseded.ReleaseReason != "expired" {
		t.Errorf("superseded = %+v, want the tenure of the session that held the task, closed as expired", took.Superseded)
	}
	if n := countWhere(t, repo, `SELECT count(*) FROM leases WHERE target_kind = 'task' AND target_key = ? AND released_at IS NULL AND holder = ?`, task, third); n != 1 {
		t.Errorf("%d unreleased lease rows held by the third session, want 1", n)
	}
	if took.Superseded != nil {
		if n := countWhere(t, repo, `SELECT count(*) FROM leases WHERE lease_id = ? AND released_at IS NOT NULL AND release_reason = 'expired'`, took.Superseded.ID); n != 1 {
			t.Errorf("the superseded row is not closed as expired")
		}
	}
}

// TestOneOperationIDTwiceConcurrentlyAndTwiceSequentially is AC-10.4: the
// same `task open --operation-id` from two processes at once, and then twice
// in a row, is one task and one operation row each time, the second answer
// marked as a replay.
func TestOneOperationIDTwiceConcurrentlyAndTwiceSequentially(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)

	results := race(t,
		commandChild(t, repo, 0, "task", "open", "--title", "once", "--session", session, "--operation-id", "op-concurrent", "--json"),
		commandChild(t, repo, 0, "task", "open", "--title", "once", "--session", session, "--operation-id", "op-concurrent", "--json"),
	)
	var replays int
	var ids []string
	for i, r := range results {
		if r.code != app.ExitSuccess {
			t.Fatalf("process %d exited %d, want 0:\n%s%s", i, r.code, r.stdout, r.stderr)
		}
		var got racedTask
		decodeData(t, r.stdout, &got)
		if got.Replayed {
			replays++
		}
		ids = append(ids, got.Task.ID)
	}
	if replays != 1 || ids[0] != ids[1] || ids[0] == "" {
		t.Errorf("concurrent deliveries: %d replays, tasks %v; want one replay and one task", replays, ids)
	}
	if tasks, ops := rowCount(t, repo, "tasks"), rowCount(t, repo, "operations"); tasks != 1 || ops != 1 {
		t.Errorf("%d task rows and %d operation rows after two concurrent deliveries, want 1 and 1", tasks, ops)
	}

	first := run(t, repo, "task", "open", "--title", "again", "--session", session, "--operation-id", "op-sequential", "--json")
	first.requireExit(t, app.ExitSuccess)
	second := run(t, repo, "task", "open", "--title", "again", "--session", session, "--operation-id", "op-sequential", "--json")
	second.requireExit(t, app.ExitSuccess)
	var a, b racedTask
	decodeData(t, first.stdout, &a)
	decodeData(t, second.stdout, &b)
	if a.Replayed || !b.Replayed || a.Task.ID != b.Task.ID {
		t.Errorf("sequential deliveries: first replayed=%v, second replayed=%v, tasks %s and %s; want a write then a replay of it",
			a.Replayed, b.Replayed, a.Task.ID, b.Task.ID)
	}
	if tasks, ops := rowCount(t, repo, "tasks"), rowCount(t, repo, "operations"); tasks != 2 || ops != 2 {
		t.Errorf("%d task rows and %d operation rows after both pairs, want 2 and 2", tasks, ops)
	}
}

// TestAContenderGivesUpWithinOneLadderStepOfItsBudget is AC-10.5: with
// another process holding `BEGIN IMMEDIATE` past the contender's budget,
// `task open` is refused as MINDRAIL_BUSY_RETRYABLE at exit 4 within the
// budget plus one ladder step, and the same command succeeds once the holder
// has committed.
func TestAContenderGivesUpWithinOneLadderStepOfItsBudget(t *testing.T) {
	const budget = 300 * time.Millisecond
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)

	holder := startChild(t, "hold", []string{childDBEnv + "=" + runtimeDBPath(t, repo)})
	contender := commandChild(t, repo, budget, "task", "open", "--title", "behind a writer", "--session", session, "--json")
	refused := race(t, contender)[0]
	elapsed := time.Since(contender.released)

	if refused.code != app.ExitUnavailable {
		t.Fatalf("the contender exited %d, want %d:\n%s%s", refused.code, app.ExitUnavailable, refused.stdout, refused.stderr)
	}
	payload := refused.errorPayload(t)
	if payload.Code != app.CodeBusyRetryable {
		t.Errorf("code = %s, want %s", payload.Code, app.CodeBusyRetryable)
	}
	waited, err := strconv.Atoi(payload.Metadata["waited_ms"])
	if err != nil {
		t.Fatalf("waited_ms = %q, want a number: %v", payload.Metadata["waited_ms"], err)
	}
	step := storage.DefaultBackoff.Cap
	if waited < int(budget/time.Millisecond) || waited > int((budget+step)/time.Millisecond) {
		t.Errorf("waited_ms = %d, want within [%d, %d]: the budget, plus at most one ladder step", waited, budget/time.Millisecond, (budget+step)/time.Millisecond)
	}
	if elapsed > budget+step+2*time.Second {
		t.Errorf("the contender took %v from release to exit, want the budget plus a ladder step and a process start", elapsed)
	}
	t.Logf("busy budget %v: waited_ms %d, %v from release to exit", budget, waited, elapsed.Round(time.Millisecond))
	if n := rowCount(t, repo, "tasks"); n != 0 {
		t.Errorf("the refused contender left %d task rows", n)
	}

	holder.release()
	if done := holder.wait(t); done.code != 0 {
		t.Fatalf("the holder exited %d: %s", done.code, done.stderr)
	}
	after := run(t, repo, "task", "open", "--title", "behind a writer", "--session", session, "--json")
	after.requireExit(t, app.ExitSuccess)
	if n := rowCount(t, repo, "tasks"); n != 1 {
		t.Errorf("%d task rows after the holder committed and the command was run again, want 1", n)
	}
}
