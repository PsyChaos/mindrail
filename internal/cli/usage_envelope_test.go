package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestARejectedLineIsPublishedUnderTheCommandThatWasTyped is finding F33.
//
// The envelope's `command` member is what a consumer keys on, and on the
// usage-rejection path it was `cmd.Name()` — the leaf word. That was invisible
// while every command was top level and became both wrong and ambiguous when
// MR-003 added nested ones: `task open extra --json` published "open", and one
// command contradicted itself under one error class, publishing "task state"
// for a bad `--to` and "state" for a missing id.
//
// Both paths are asserted, because the defect is a disagreement between them
// rather than a wrong value on either.
func TestARejectedLineIsPublishedUnderTheCommandThatWasTyped(t *testing.T) {
	repo := newInitializedRepo(t)
	session := sessionID(t, repo)
	task := openTask(t, repo, session, "a task to name a command with")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "a well-formed line",
			args: []string{"task", "list"},
			want: "task list",
		},
		{
			name: "an argument the command does not take",
			args: []string{"task", "open", "--title", "a title", "extra"},
			want: "task open",
		},
		{
			name: "a flag the command does not have",
			args: []string{"task", "open", "--titel", "a title"},
			want: "task open",
		},
		{
			name: "a positional the command needs and did not get",
			args: []string{"task", "state", "--to", "CLAIMED"},
			want: "task state",
		},
		{
			name: "a value the command does not accept",
			args: []string{"task", "state", task, "--to", "BOGUS"},
			want: "task state",
		},
		{
			name: "a nested group's own subcommand",
			args: []string{"session", "open", "extra"},
			want: "session open",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, repo, append(tc.args, "--json")...)

			var envelope struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal([]byte(got.stdout), &envelope); err != nil {
				t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, got.stdout)
			}
			if envelope.Command != tc.want {
				t.Errorf("command = %q, want %q\n%s", envelope.Command, tc.want, got.stdout)
			}
		})
	}
}

// TestNamingAGroupWithNoSubcommandIsAnEnvelope is finding F34.
//
// A command group with subcommands is runnable and prints help, which is the
// right answer for a person. Under `--json` it put 968 bytes of help on stdout
// and exited 0 — so `mindrail checkpoint --json && echo "handoff recorded"`
// printed that with nothing written, and decision D-15's promise that stdout
// carries a single JSON object was broken by the flag's own help text.
//
// All four groups, including the root, because the branch is shared and a test
// over one of them would pass with the other three broken.
func TestNamingAGroupWithNoSubcommandIsAnEnvelope(t *testing.T) {
	repo := newInitializedRepo(t)

	for _, group := range [][]string{
		{"task"},
		{"session"},
		{"checkpoint"},
		{}, // the root itself
	} {
		name := "mindrail"
		if len(group) > 0 {
			name = group[0]
		}

		got := run(t, repo, append(group, "--json")...)

		if got.code != app.ExitUsage {
			t.Errorf("`%s --json` exited %d, want %d\n%s", name, got.code, app.ExitUsage, got.stdout)
		}
		payload := got.errorPayload(t)
		if payload.Code != app.CodeCommandLineInvalid {
			t.Errorf("`%s --json` reported %q, want %q", name, payload.Code, app.CodeCommandLineInvalid)
		}
		if strings.Contains(got.stdout, "Available Commands") {
			t.Errorf("`%s --json` put help text on stdout:\n%s", name, got.stdout)
		}

		// Human mode is unchanged: help is what a person asked for.
		human := run(t, repo, group...)
		if human.code != app.ExitSuccess {
			t.Errorf("`%s` exited %d for a person, want %d", name, human.code, app.ExitSuccess)
		}
		if !strings.Contains(human.stdout, "Usage:") {
			t.Errorf("`%s` no longer prints help for a person:\n%s", name, human.stdout)
		}
	}
}

// TestASecondClaimNamesTheSessionHoldingIt is finding F22, and decision D-58's
// unamended clause: "the message says the task is already claimed and by which
// session".
//
// Nothing said so. The refusal named the two states and stopped there, which
// tells an arriving agent that the task is taken and not who to hand over to —
// on the one command in the group whose whole purpose is handover.
//
// Since MR-004 the refusal is LEASE_CONFLICT rather than TASK_STATE_INVALID
// (decision D-67: the lease is judged before the state table, because the
// reason the move cannot happen is ownership), and it names the holder, the
// lease and the expiry. The clause F22 asked for holds under the new code.
func TestASecondClaimNamesTheSessionHoldingIt(t *testing.T) {
	repo := newInitializedRepo(t)

	first := sessionID(t, repo)
	task := openTask(t, repo, first, "a task the first agent claims")
	run(t, repo, "task", "state", task, "--to", "CLAIMED", "--session", first, "--json").
		requireExit(t, app.ExitSuccess)

	second := sessionID(t, repo)
	got := run(t, repo, "task", "state", task, "--to", "CLAIMED", "--session", second, "--json")
	if got.code != app.ExitFailed {
		t.Fatalf("a second claim exited %d, want %d\n%s", got.code, app.ExitFailed, got.stdout)
	}

	payload := got.errorPayload(t)
	if payload.Code != app.CodeLeaseConflict {
		t.Fatalf("code = %q, want %q", payload.Code, app.CodeLeaseConflict)
	}
	if payload.Metadata["holder"] != first {
		t.Errorf("metadata holder = %q, want the session holding it, %q",
			payload.Metadata["holder"], first)
	}
	if payload.Metadata["expires_at"] == "" || payload.Metadata["lease_id"] == "" {
		t.Errorf("metadata = %v, want the lease and its expiry named", payload.Metadata)
	}
	if !strings.Contains(payload.Why, first) {
		t.Errorf("why = %q does not name the session holding the task", payload.Why)
	}

	// The over-fire arm: a refused move on an unclaimed task has no holder to
	// name, and inventing one would be worse than saying nothing.
	unclaimed := openTask(t, repo, first, "a task nobody has claimed")
	refused := run(t, repo, "task", "state", unclaimed, "--to", "COMPLETED", "--session", first, "--json")
	refused.requireExit(t, app.ExitFailed)
	if holder := refused.errorPayload(t).Metadata["claimed_by"]; holder != "" {
		t.Errorf("an unclaimed task is reported as claimed by %q", holder)
	}
}
