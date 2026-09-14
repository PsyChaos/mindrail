package cli_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
)

// leaseOf decodes the `lease` object of a writer's result.
func leaseOf(t *testing.T, stdout string) *coordination.Lease {
	t.Helper()
	var data struct {
		Lease *coordination.Lease `json:"lease"`
	}
	decodeData(t, stdout, &data)
	return data.Lease
}

// acquireFile takes a file lease through the real command tree and returns
// its id.
func acquireFile(t *testing.T, repo, session, path string) string {
	t.Helper()
	got := run(t, repo, "lease", "acquire", "--file", path, "--session", session, "--json")
	got.requireExit(t, app.ExitSuccess)
	lease := leaseOf(t, got.stdout)
	if lease == nil || lease.Status != coordination.LeaseActive {
		t.Fatalf("lease acquire published %+v, want an active lease", lease)
	}
	return lease.ID
}

// TestTheLeaseGroupHasFourVerbsAndTwoTargets is AC-08.1: the four subcommands
// of design §10, `lease acquire` over a file or a task, and no fifth verb.
func TestTheLeaseGroupHasFourVerbsAndTwoTargets(t *testing.T) {
	repo := newInitializedRepo(t)
	me := sessionID(t, repo)

	help := run(t, repo, "lease", "--help")
	help.requireExit(t, app.ExitSuccess)
	for _, verb := range []string{"acquire", "renew", "release", "list"} {
		if !strings.Contains(help.stdout, "\n  "+verb) {
			t.Errorf("`lease --help` does not list %s:\n%s", verb, help.stdout)
		}
	}
	if got := run(t, repo, "lease", "break", "LSE-x", "--json"); got.code != app.ExitUsage {
		t.Errorf("`lease break` exited %d, want %d: there is no fifth verb", got.code, app.ExitUsage)
	}

	// A file, then the holder renews and releases it.
	id := acquireFile(t, repo, me, "src/auth.go")
	renewed := run(t, repo, "lease", "renew", id, "--session", me, "--json")
	renewed.requireExit(t, app.ExitSuccess)
	released := run(t, repo, "lease", "release", id, "--session", me, "--json")
	released.requireExit(t, app.ExitSuccess)
	if lease := leaseOf(t, released.stdout); lease.Status != coordination.LeaseReleased || lease.ReleaseReason != coordination.ReleaseReasonReleased {
		t.Errorf("lease release published %+v, want released (released)", lease)
	}

	// A task where it stands: the claim without a move (D-66 as amended).
	task := openTask(t, repo, me, "a task to take where it stands")
	run(t, repo, "task", "state", task, "--to", "CLAIMED", "--session", me, "--json").requireExit(t, app.ExitSuccess)
	run(t, repo, "task", "state", task, "--to", "IN_PROGRESS", "--session", me, "--json").requireExit(t, app.ExitSuccess)
	run(t, repo, "checkpoint", "write", task, "--note", "leaving", "--handoff", "--session", me, "--json").requireExit(t, app.ExitSuccess)

	next := sessionID(t, repo)
	taken := run(t, repo, "lease", "acquire", "--task", task, "--session", next, "--json")
	taken.requireExit(t, app.ExitSuccess)
	var acquisition struct {
		Lease coordination.Lease `json:"lease"`
		Task  *coordination.Task `json:"task"`
	}
	decodeData(t, taken.stdout, &acquisition)
	if acquisition.Lease.Holder != next || acquisition.Task == nil || acquisition.Task.ClaimedBy != next || acquisition.Task.State != coordination.StateInProgress {
		t.Errorf("lease acquire --task published %+v / %+v; want the next session holding an IN_PROGRESS task", acquisition.Lease, acquisition.Task)
	}

	// Both targets, or neither, is a mistake in the command line.
	for _, args := range [][]string{
		{"lease", "acquire", "--session", me},
		{"lease", "acquire", "--file", "a.go", "--task", task, "--session", me},
	} {
		got := run(t, repo, append(slices.Clone(args), "--json")...)
		if got.code != app.ExitUsage {
			t.Errorf("%v exited %d, want %d", args, got.code, app.ExitUsage)
		}
		if payload := got.errorPayload(t); !strings.Contains(payload.Impact, "Mindrail did not run") {
			t.Errorf("%v: impact = %q, want the refusal before starting", args, payload.Impact)
		}
	}
}

// TestEveryWriterTakesAnOperationIDAndJudgesItFirst is AC-08.2's first half:
// all seven writers take --operation-id, and a malformed one is refused
// before the application starts.
func TestEveryWriterTakesAnOperationIDAndJudgesItFirst(t *testing.T) {
	repo := newInitializedRepo(t)
	me := sessionID(t, repo)
	task := openTask(t, repo, me, "a task")
	lease := acquireFile(t, repo, me, "src/x.go")

	writers := [][]string{
		{"session", "open"},
		{"task", "open", "--title", "under an id", "--session", me},
		{"task", "state", task, "--to", "CLAIMED", "--session", me},
		{"checkpoint", "write", task, "--note", "a note", "--session", me},
		{"lease", "acquire", "--file", "src/y.go", "--session", me},
		{"lease", "renew", lease, "--session", me},
		{"lease", "release", lease, "--session", me},
	}
	for _, args := range writers {
		name := commandName(args)
		bad := run(t, repo, append(slices.Clone(args), "--operation-id", "has space", "--json")...)
		if bad.code != app.ExitUsage {
			t.Errorf("%s with a malformed --operation-id exited %d, want %d\n%s", name, bad.code, app.ExitUsage, bad.stdout)
			continue
		}
		payload := bad.errorPayload(t)
		if payload.Code != app.CodeCommandLineInvalid || !strings.Contains(payload.Impact, "Mindrail did not run") {
			t.Errorf("%s: %s / %q, want COMMAND_LINE_INVALID before starting", name, payload.Code, payload.Impact)
		}
	}
	if n := rowCount(t, repo, "operations"); n != 0 {
		t.Errorf("malformed ids recorded %d operation row(s)", n)
	}

	// And a well-formed one is carried: the first run records, the second
	// replays (AC-08.4).
	first := run(t, repo, "task", "open", "--title", "opened once", "--session", me, "--operation-id", "op:open-1", "--json")
	first.requireExit(t, app.ExitSuccess)
	second := run(t, repo, "task", "open", "--title", "opened once", "--session", me, "--operation-id", "op:open-1", "--json")
	second.requireExit(t, app.ExitSuccess)

	var a, b map[string]json.RawMessage
	decodeData(t, first.stdout, &a)
	decodeData(t, second.stdout, &b)
	if string(a["replayed"]) != "false" || string(b["replayed"]) != "true" {
		t.Errorf("replayed = %s then %s, want false then true", a["replayed"], b["replayed"])
	}
	if string(a["task"]) != string(b["task"]) || string(a["session"]) != string(b["session"]) {
		t.Errorf("the replay's task or session differs from the first delivery's:\n%s\n%s", a["task"], b["task"])
	}
	if string(a["operation_id"]) != `"op:open-1"` {
		t.Errorf("operation_id = %s, want the id given", a["operation_id"])
	}
	if n := rowCount(t, repo, "tasks"); n != 2 {
		t.Errorf("tasks = %d after one task opened twice under one id (plus the fixture's), want 2", n)
	}

	human := run(t, repo, "task", "open", "--title", "opened once", "--session", me, "--operation-id", "op:open-1", "--no-color")
	human.requireExit(t, app.ExitSuccess)
	if !strings.Contains(human.stdout, "recorded result of operation op:open-1") {
		t.Errorf("the human rendering of a replay does not say so:\n%s", human.stdout)
	}
}

// TestExpectRevisionIsJudgedBeforeStartingAndThenByTheStore is AC-08.2's
// second half and AC-05.4 through the command tree.
func TestExpectRevisionIsJudgedBeforeStartingAndThenByTheStore(t *testing.T) {
	repo := newInitializedRepo(t)
	me := sessionID(t, repo)
	task := openTask(t, repo, me, "a task at revision 1")

	for _, raw := range []string{"0", "-1", "x", "1.5"} {
		got := run(t, repo, "task", "state", task, "--to", "CLAIMED", "--session", me, "--expect-revision", raw, "--json")
		if got.code != app.ExitUsage {
			t.Errorf("--expect-revision %s exited %d, want %d", raw, got.code, app.ExitUsage)
			continue
		}
		if payload := got.errorPayload(t); !strings.Contains(payload.Impact, "Mindrail did not run") {
			t.Errorf("--expect-revision %s: impact = %q", raw, payload.Impact)
		}
	}

	moved := run(t, repo, "task", "state", task, "--to", "CLAIMED", "--session", me, "--expect-revision", "1", "--json")
	moved.requireExit(t, app.ExitSuccess)
	var data struct {
		Task struct {
			Revision int64 `json:"revision"`
		} `json:"task"`
	}
	decodeData(t, moved.stdout, &data)
	if data.Task.Revision != 2 {
		t.Errorf("revision = %d after the move, want 2", data.Task.Revision)
	}

	stale := run(t, repo, "task", "state", task, "--to", "IN_PROGRESS", "--session", me, "--expect-revision", "1", "--json")
	if stale.code != app.ExitFailed {
		t.Fatalf("a stale expectation exited %d, want %d", stale.code, app.ExitFailed)
	}
	payload := stale.errorPayload(t)
	if payload.Code != app.CodeStateRevisionConflict || payload.Metadata["current_revision"] != "2" {
		t.Errorf("stale expectation = %s %v, want STATE_REVISION_CONFLICT at revision 2", payload.Code, payload.Metadata)
	}
}

// TestTaskShowPrintsTheLeaseInEveryStatus is AC-08.5: active with the
// expiry, expired with the time, released with the time — and no lease line
// for a task never claimed.
func TestTaskShowPrintsTheLeaseInEveryStatus(t *testing.T) {
	repo := newInitializedRepo(t)
	me := sessionID(t, repo)

	fresh := openTask(t, repo, me, "never claimed")
	shown := run(t, repo, "task", "show", fresh, "--no-color")
	shown.requireExit(t, app.ExitSuccess)
	if strings.Contains(shown.stdout, "Lease") {
		t.Errorf("a task never claimed prints a lease line:\n%s", shown.stdout)
	}

	task := openTask(t, repo, me, "claimed")
	claimed := run(t, repo, "task", "state", task, "--to", "CLAIMED", "--session", me, "--json")
	claimed.requireExit(t, app.ExitSuccess)
	lease := leaseOf(t, claimed.stdout)

	active := run(t, repo, "task", "show", task, "--no-color")
	active.requireExit(t, app.ExitSuccess)
	if !strings.Contains(active.stdout, "Lease "+lease.ID) || !strings.Contains(active.stdout, "Held by session "+me+" until "+app.FormatTime(lease.ExpiresAt)) {
		t.Errorf("an active lease is not printed with its holder and expiry:\n%s", active.stdout)
	}

	// Expiry is a matter of the clock the binary runs on, so the row is
	// back-dated rather than waited for.
	execOnRuntimeDB(t, repo, `UPDATE leases SET expires_at = '2020-01-01T00:00:00Z' WHERE lease_id = '`+lease.ID+`'`)
	expired := run(t, repo, "task", "show", task, "--no-color")
	expired.requireExit(t, app.ExitSuccess)
	if !strings.Contains(expired.stdout, "expired at 2020-01-01T00:00:00Z") || !strings.Contains(expired.stdout, "Claimed by: "+me) {
		t.Errorf("an expired lease is not printed as expired beside the claimant:\n%s", expired.stdout)
	}
	if n := rowCount(t, repo, "leases"); n != 1 {
		t.Errorf("task show changed the leases table to %d rows; reads write nothing", n)
	}

	// Released, through the holder's own release.
	execOnRuntimeDB(t, repo, `UPDATE leases SET expires_at = '2099-01-01T00:00:00Z' WHERE lease_id = '`+lease.ID+`'`)
	run(t, repo, "lease", "release", lease.ID, "--session", me, "--json").requireExit(t, app.ExitSuccess)
	released := run(t, repo, "task", "show", task, "--no-color")
	released.requireExit(t, app.ExitSuccess)
	if !strings.Contains(released.stdout, "released at") || !strings.Contains(released.stdout, "(released)") {
		t.Errorf("a released lease is not printed with its end:\n%s", released.stdout)
	}

	structured := run(t, repo, "task", "show", task, "--json")
	structured.requireExit(t, app.ExitSuccess)
	if got := leaseOf(t, structured.stdout); got == nil || got.Status != coordination.LeaseReleased {
		t.Errorf("task show --json publishes %+v, want the released tenure", got)
	}
}

// TestLeaseListAndTaskShowWriteNoRow is AC-08.6 and AC-08.3's `[]`.
func TestLeaseListAndTaskShowWriteNoRow(t *testing.T) {
	repo := newInitializedRepo(t)
	empty := run(t, repo, "lease", "list", "--json")
	empty.requireExit(t, app.ExitSuccess)
	var listed struct {
		Leases []coordination.Lease `json:"leases"`
	}
	decodeData(t, empty.stdout, &listed)
	if listed.Leases == nil {
		t.Errorf("lease list --json says null for a project with no leases, want []:\n%s", empty.stdout)
	}

	me := sessionID(t, repo)
	acquireFile(t, repo, me, "a.go")
	acquireFile(t, repo, me, "b.go")
	task := openTask(t, repo, me, "a task")

	before := map[string]int{}
	for _, table := range []string{"sessions", "leases", "operations", "tasks"} {
		before[table] = rowCount(t, repo, table)
	}
	run(t, repo, "lease", "list", "--json").requireExit(t, app.ExitSuccess)
	run(t, repo, "task", "show", task, "--json").requireExit(t, app.ExitSuccess)
	human := run(t, repo, "lease", "list", "--no-color")
	human.requireExit(t, app.ExitSuccess)
	for table, n := range before {
		if got := rowCount(t, repo, table); got != n {
			t.Errorf("a read changed %s from %d to %d rows", table, n, got)
		}
	}
	if !strings.Contains(human.stdout, "a.go") || !strings.Contains(human.stdout, "b.go") {
		t.Errorf("lease list does not name both leases:\n%s", human.stdout)
	}
}

// TestAMoveReportsTheLeaseAndATakeover is D-67 on `task state`'s wire: the
// lease the move left active, and the tenure it took over.
func TestAMoveReportsTheLeaseAndATakeover(t *testing.T) {
	repo := newInitializedRepo(t)
	first := sessionID(t, repo)
	task := openTask(t, repo, first, "a task to take over")
	claimed := run(t, repo, "task", "state", task, "--to", "CLAIMED", "--session", first, "--json")
	claimed.requireExit(t, app.ExitSuccess)
	held := leaseOf(t, claimed.stdout)
	if held == nil || held.Holder != first {
		t.Fatalf("task state published %+v, want the lease it acquired", held)
	}

	execOnRuntimeDB(t, repo, `UPDATE leases SET expires_at = '2020-01-01T00:00:00Z' WHERE lease_id = '`+held.ID+`'`)
	second := sessionID(t, repo)
	took := run(t, repo, "task", "state", task, "--to", "IN_PROGRESS", "--session", second, "--json")
	took.requireExit(t, app.ExitSuccess)
	var data struct {
		Lease      *coordination.Lease `json:"lease"`
		Superseded *coordination.Lease `json:"superseded"`
	}
	decodeData(t, took.stdout, &data)
	if data.Lease == nil || data.Lease.Holder != second || data.Superseded == nil || data.Superseded.ID != held.ID {
		t.Errorf("the takeover published lease %+v superseded %+v; want the second session's tenure and the first's named", data.Lease, data.Superseded)
	}

	execOnRuntimeDB(t, repo, `UPDATE leases SET expires_at = '2020-01-01T00:00:00Z' WHERE holder = '`+second+`'`)
	third := sessionID(t, repo)
	human := run(t, repo, "task", "state", task, "--to", "BLOCKED", "--reason", "waiting", "--session", third, "--no-color")
	human.requireExit(t, app.ExitSuccess)
	if !strings.Contains(human.stdout, "Took over from session "+second) {
		t.Errorf("the human rendering of a takeover does not name the session it took over from:\n%s", human.stdout)
	}
}
