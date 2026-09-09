package cli_test

import (
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestAnUnregisteredWorktreeIsRefusedByName is finding F32.
//
// coordinationScope has two refusals: the runtime database is not there, and it
// is there but this worktree has no row in it. Only the first was ever driven.
// Deleting the second branch — the one that keeps a linked worktree from
// reading another worktree's project — left the whole suite green, and the
// mutant does not fail loudly: `task list` answers `ok:true` with an empty list
// over a project that holds three tasks, which is the answer a reader is least
// able to question.
//
// The linked worktree shares the repository's common directory and therefore
// its runtime database, so the store is present and the workspace row is not.
// That is the only arrangement in which the two branches are distinguishable.
func TestAnUnregisteredWorktreeIsRefusedByName(t *testing.T) {
	repo := newRepoWithCommit(t)
	if got := run(t, repo, "init"); got.code != app.ExitSuccess {
		t.Fatalf("init exited %d: %s", got.code, got.stdout)
	}

	session := sessionID(t, repo)
	task := openTask(t, repo, session, "work the other worktree must not see")

	linked := addWorktree(t, repo, "mode-b")

	for _, args := range coordinationCommands() {
		name := commandName(args)
		got := run(t, linked, append(argsFor(args, task), "--json")...)

		if got.code != app.ExitFailed {
			t.Errorf("%s exited %d from an unregistered worktree, want %d\n%s",
				name, got.code, app.ExitFailed, got.stdout)
			continue
		}

		payload := got.errorPayload(t)
		if payload.Code != app.CodeCoordinationUnavailable {
			t.Errorf("%s reported %q from an unregistered worktree, want %q",
				name, payload.Code, app.CodeCoordinationUnavailable)
		}
		// The branch, not just the code. Both refusals carry
		// COORDINATION_UNAVAILABLE, and a test that stopped at the code would
		// pass with the two sentences swapped — which is the difference between
		// "run init here" and "this repository has never been initialised".
		if !strings.Contains(payload.Why, "not registered in the runtime database") {
			t.Errorf("%s explains it with %q, which is the other branch's sentence", name, payload.Why)
		}
	}

	// The over-fire guard: the registered worktree still sees its own task.
	listed := run(t, repo, "task", "list", "--json")
	listed.requireExit(t, app.ExitSuccess)
	if !strings.Contains(listed.stdout, task) {
		t.Errorf("the registered worktree cannot see its own task:\n%s", listed.stdout)
	}
}

// argsFor substitutes the real task id into an argument list that carries the
// placeholder coordinationCommands uses.
func argsFor(args []string, task string) []string {
	substituted := make([]string, len(args))
	for i, arg := range args {
		if strings.HasPrefix(arg, "TSK-0000") {
			substituted[i] = task
			continue
		}
		substituted[i] = arg
	}
	return substituted
}
