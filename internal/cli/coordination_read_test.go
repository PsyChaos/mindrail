package cli_test

import (
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestADamagedRowIsReportedWithACode is finding F44.
//
// Every write path in internal/coordination was wrapped in a domain error and
// every read path returned a bare fmt.Errorf, which emit published verbatim.
// One unparseable timestamp therefore reached the user as
// `{"code":"","why":"look up the last checkpoint of \"TSK-…\": parsing time
// \"yesterday\" as \"2006\"…","impact":"","next_action":[]}` — a Go parser
// message with a format string in it, at exit 1, with nothing to branch on and
// nothing to do about it. AC-04.6 requires a registered code, a diagnostic, an
// impact and a next action on every error.
//
// The damage is semantic rather than structural on purpose: PRAGMA
// integrity_check runs at startup and would intercept a corrupt file, so a
// column that parses as text and not as a timestamp is what actually reaches
// the read path. It is not exotic — FormatTime writes RFC3339Nano and ParseTime
// is strict, so a host clock outside years 1–9999 writes checkpoints this
// binary can never read back.
func TestADamagedRowIsReportedWithACode(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "a task with a note that will be damaged")
	run(t, repo, "checkpoint", "write", task, "--note", "the note", "--session", session, "--json").
		requireExit(t, app.ExitSuccess)

	execOnRuntimeDB(t, repo, `UPDATE checkpoints SET created_at = 'yesterday'`)

	got := run(t, repo, "task", "show", task, "--json")
	if got.code == app.ExitSuccess {
		t.Fatalf("task show succeeded over a row it cannot parse:\n%s", got.stdout)
	}

	payload := got.errorPayload(t)
	if !app.IsRegistered(payload.Code) {
		t.Errorf("code = %q, which this binary does not own; a caller has nothing to branch on\n%s",
			payload.Code, got.stdout)
	}
	if payload.Code != app.CodeCoordinationReadFailed {
		t.Errorf("code = %q, want %q", payload.Code, app.CodeCoordinationReadFailed)
	}
	assertFourErrorKeys(t, got.stdout)

	// The parser message is the cause, not the diagnosis. A `why` that is a Go
	// format string tells the reader what the code did, not what is wrong.
	if strings.Contains(payload.Why, "parsing time") {
		t.Errorf("why = %q is the parser's message rather than a diagnosis", payload.Why)
	}
}

// TestAReadThatRanAndFailedIsNotPublishedAsNobodyLooked is finding F46.
//
// The same damaged row, seen through `status`. The store was constructed, both
// queries ran and one failed — and the block said "not observed: startup
// stopped before this subsystem was read", about a startup that completed. The
// adjacent Runtime block has had `indeterminate` since MR-001 and means exactly
// what happened here.
func TestAReadThatRanAndFailedIsNotPublishedAsNobodyLooked(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "a task with a note that will be damaged")
	run(t, repo, "checkpoint", "write", task, "--note", "the note", "--session", session, "--json").
		requireExit(t, app.ExitSuccess)

	execOnRuntimeDB(t, repo, `UPDATE checkpoints SET created_at = 'yesterday'`)

	got := run(t, repo, "status", "--json")

	var data struct {
		Coordination struct {
			Observation string `json:"observation"`
		} `json:"coordination"`
	}
	decodeData(t, got.stdout, &data)

	if data.Coordination.Observation != "indeterminate" {
		t.Errorf("observation = %q after a read that ran and failed, want indeterminate:\n%s",
			data.Coordination.Observation, got.stdout)
	}
}

// TestNobodyLookedIsStillSaidWhereNobodyLooked is the over-fire guard for the
// test above, and the condition `not_observed` genuinely describes.
//
// With the database unreadable the startup sequence stops before step 6, so
// knowledge, the workspace and coordination are all things nothing was read
// about. All three have to say so together: a block that had quietly started
// answering `indeterminate` everywhere would pass the test above and mean
// nothing.
func TestNobodyLookedIsStillSaidWhereNobodyLooked(t *testing.T) {
	repo := newInitializedRepo(t)
	denyAccess(t, runtimeDBPath(t, repo))

	got := run(t, repo, "status", "--json")

	var data struct {
		Knowledge struct {
			Observation string `json:"observation"`
		} `json:"knowledge"`
		Workspace struct {
			Observation string `json:"observation"`
		} `json:"workspace"`
		Coordination struct {
			Observation string `json:"observation"`
		} `json:"coordination"`
	}
	decodeData(t, got.stdout, &data)

	for name, observation := range map[string]string{
		"knowledge":    data.Knowledge.Observation,
		"workspace":    data.Workspace.Observation,
		"coordination": data.Coordination.Observation,
	} {
		if observation != "not_observed" {
			t.Errorf("%s says %q where the sequence stopped before it, want not_observed:\n%s",
				name, observation, got.stdout)
		}
	}
}
