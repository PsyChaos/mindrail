package cli_test

import (
	"slices"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/cli"
)

// coordinationCommands is every command MR-003 added, as the argument list that
// reaches it.
//
// All six, not one. The startup verdict and the coordination refusal both live
// in one shared body, and a command that stopped calling it would be invisible
// to a test over `task list`.
func coordinationCommands() [][]string {
	const absentTask = "TSK-0000000000000000000000000"

	return [][]string{
		{"session", "open"},
		{"task", "open", "--title", "something"},
		{"task", "state", absentTask, "--to", "CLAIMED"},
		{"task", "show", absentTask},
		{"task", "list"},
		{"checkpoint", "write", absentTask, "--note", "something"},
	}
}

// startupHaltedConditions are repository states that stop the startup sequence
// before there is any coordination state to read.
//
// Each is already a row of the MR-001 agreement matrix, and the reference answer
// is taken from `status` on the same bytes rather than written down here, so
// this matrix cannot drift away from the one it extends.
func startupHaltedConditions() []condition {
	return []condition{
		{
			name:   "the directory is not inside a Git repository",
			broken: true,
			setup:  newNonRepositoryDir,
		},
		{
			name:   "a bare repository",
			broken: true,
			setup:  newBareRepo,
		},
		{
			name:   "config.toml cannot be parsed",
			broken: true,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeFile(t, configPath(repo), []byte("output.color = \n"))
				return repo
			},
		},
		{
			name:   "the database was truncated to garbage",
			broken: true,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				corruptDatabase(t, repo)
				return repo
			},
		},
		{
			name:   "the runtime tables were dropped",
			broken: true,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				dropRuntimeTables(t, repo)
				return repo
			},
		},
		{
			name:   "the migration ledger names a version this binary does not have",
			broken: true,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				recordFutureMigration(t, repo)
				return repo
			},
		},
	}
}

// TestACoordinationCommandNamesAHaltedStartupTheWayStatusDoes is finding F09.
//
// bootstrap.App.Run calls the command body even when Start failed, and the
// coordination body was the one caller that never asked what Start decided. It
// went straight to two readings — is the store nil, is the workspace row empty —
// which have two sentences between them for every cause there is. So a
// repository holding a live task, whose config.toml had one unparseable line,
// answered `task list --json` with "this repository has no runtime database, so
// it holds no sessions, tasks or checkpoints" and told the reader to run
// `mindrail init`, which exits 2 with the real code and clears nothing. The true
// code was on stderr in the same run and was dropped before the envelope was
// built.
//
// The reference is `status` on the same bytes, because the property under test
// is agreement rather than any particular code: whatever `status` calls a broken
// startup, the six commands that arrived at the same disk call it that too.
//
// The last assertion is the one that names the defect. COORDINATION_UNAVAILABLE
// describes exactly one thing — a repository whose startup completed and which
// holds no coordination state — and it is a false sentence for every row here.
func TestACoordinationCommandNamesAHaltedStartupTheWayStatusDoes(t *testing.T) {
	for _, tc := range startupHaltedConditions() {
		t.Run(tc.name, func(t *testing.T) {
			reference := answerOf(t, tc.setup(t), tc.options, "status")
			if reference.code == "" {
				t.Fatalf("%s: `status` reported no failure, so the condition did not reproduce", tc.name)
			}

			for _, args := range coordinationCommands() {
				name := commandName(args)
				repo := tc.setup(t)
				got := answerFrom(t, repo, name,
					runWith(t, repo, tc.options, append(slices.Clone(args), "--json")...))

				if got.code != reference.code {
					t.Errorf("%s: `status` calls it %q and `%s` calls it %q; one condition has one code",
						tc.name, reference.code, name, got.code)
				}
				if got.exit != reference.exit {
					t.Errorf("%s: `status` exits %d and `%s` exits %d for the same disk",
						tc.name, reference.exit, name, got.exit)
				}
				if !slices.Equal(got.nextAction, reference.nextAction) {
					t.Errorf("%s: the two commands print different remedies for one condition\n  status: %v\n  %s: %v",
						tc.name, reference.nextAction, name, got.nextAction)
				}
				if got.code == app.CodeCoordinationUnavailable {
					t.Errorf("%s: `%s` reports %q, which says the startup sequence completed and found no coordination state; it did not complete",
						tc.name, name, app.CodeCoordinationUnavailable)
				}
			}
		})
	}
}

// TestTheCoordinationRefusalStillFiresWhereItIsTrue is the other arm.
//
// The rows above would all pass if the six commands had simply stopped refusing
// anything, so the condition COORDINATION_UNAVAILABLE does describe — a startup
// that completed over a repository holding no coordination state — is asserted
// here on the same six commands, along with the remedy that clears it.
//
// `status` legitimately answers this one differently: decision D-03 puts a
// repository that has never been initialised at exit 0, because nothing is
// wrong with it, while a coordination command asked to read or write state it
// does not have has failed to do what it was asked. That divergence is recorded
// in the requirements beside AC-06.7 rather than carried as an exemption here.
func TestTheCoordinationRefusalStillFiresWhereItIsTrue(t *testing.T) {
	for _, args := range coordinationCommands() {
		name := commandName(args)
		repo := newRepo(t)

		got := runWith(t, repo, cli.Options{}, append(slices.Clone(args), "--json")...)
		if got.code != app.ExitFailed {
			t.Errorf("%s exited %d over a repository that has never been initialised, want %d",
				name, got.code, app.ExitFailed)
		}
		if payload := got.errorPayload(t); payload.Code != app.CodeCoordinationUnavailable {
			t.Errorf("%s reports %q over a repository that has never been initialised, want %q",
				name, payload.Code, app.CodeCoordinationUnavailable)
		}

		// And `status`, which is the reference the rows above compare against,
		// says nothing is wrong with the same disk.
		if reference := answerOf(t, repo, cli.Options{}, "status"); reference.exit != app.ExitSuccess {
			t.Errorf("`status` exits %d over a repository that has never been initialised, want %d",
				reference.exit, app.ExitSuccess)
		}
	}
}

// commandName is the argument list read back as the command a reader would type,
// which is the two words before the first operand.
func commandName(args []string) string {
	if len(args) < 2 {
		return args[0]
	}
	return args[0] + " " + args[1]
}
