package cli_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/cli"
	"github.com/PsyChaos/mindrail/internal/coordination"
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

// coordinationRefusal is one of AC-09.1's four conditions, with everything the
// three-way assertion needs: the command line that meets it, the code it must
// be called by, and the remedy carried out rather than read.
type coordinationRefusal struct {
	name string
	// setup builds the repository and the command line that meets the
	// condition in it.
	setup func(t *testing.T) (repo string, args []string)
	code  app.Code
	exit  int
	// clear carries out the printed next_action and returns the command line
	// that must succeed afterwards. Nil where the condition has no remedy a
	// test can perform.
	clear func(t *testing.T, repo string, actions []string) []string
}

// TestTheFourCoordinationRefusalsAgreeAcrossBothRenderings is acceptance
// criterion AC-09.1, which was written and never implemented (finding F13).
//
// The criterion asks for the human rendering, the JSON envelope and the exit
// code to be asserted together for each of four conditions. Every coordination
// failure assertion in the suite ran with `--json`; the only two non-JSON
// invocations were on success paths, so half of what each command prints was
// never read by anything.
//
// The remedy is carried out rather than substring-matched, which is the part
// that catches a sentence that is well-formed and useless: MR-002's §5 rule 3
// is that a remedy that cannot be performed is a defect, and the matrix is the
// machinery this repository built to catch it.
func TestTheFourCoordinationRefusalsAgreeAcrossBothRenderings(t *testing.T) {
	for _, tc := range coordinationRefusals() {
		t.Run(tc.name, func(t *testing.T) {
			repo, args := tc.setup(t)

			structured := run(t, repo, append(slices.Clone(args), "--json")...)
			if structured.code != tc.exit {
				t.Fatalf("exit = %d, want %d\n%s", structured.code, tc.exit, structured.stdout)
			}
			payload := structured.errorPayload(t)
			if payload.Code != tc.code {
				t.Fatalf("code = %q, want %q", payload.Code, tc.code)
			}
			assertFourErrorKeys(t, structured.stdout)

			// The same condition, in the same repository, read the way a person
			// reads it. The two renderings come from one value, and the
			// assertion is that nothing between the value and the terminal
			// drops the half a person sees.
			//
			// Re-running against the same repository is safe because every one
			// of these four refusals writes nothing — which is finding F02's
			// fix, asserted separately — and it is necessary, because the
			// sentences name ids a second repository would mint differently.
			human := run(t, repo, append(slices.Clone(args), "--no-color")...)
			if human.code != structured.code {
				t.Errorf("the human run exits %d and the --json run exits %d for one condition",
					human.code, structured.code)
			}
			if !strings.Contains(human.stdout, string(tc.code)) {
				t.Errorf("the human rendering does not name the code %q:\n%s", tc.code, human.stdout)
			}
			if !strings.Contains(human.stdout, payload.Why) {
				t.Errorf("the human rendering does not carry the reason %q:\n%s", payload.Why, human.stdout)
			}
			for _, action := range payload.NextAction {
				if !strings.Contains(human.stdout, action) {
					t.Errorf("the human rendering drops the remedy %q:\n%s", action, human.stdout)
				}
			}

			if tc.clear == nil {
				return
			}

			// And the sentence works. A fresh repository, the printed remedy
			// carried out, and the command that was refused run again.
			clearRepo, clearArgs := tc.setup(t)
			retry := tc.clear(t, clearRepo, payload.NextAction)
			if retry == nil {
				retry = clearArgs
			}
			if got := run(t, clearRepo, append(slices.Clone(retry), "--json")...); got.code != app.ExitSuccess {
				t.Errorf("after carrying out %v, `%s` exited %d: %s",
					payload.NextAction, commandName(retry), got.code, got.stdout)
			}
		})
	}
}

func coordinationRefusals() []coordinationRefusal {
	const absentTask = "TSK-0000000000000000000000000"

	return []coordinationRefusal{
		{
			name: "an unknown task",
			code: app.CodeTaskNotFound,
			exit: app.ExitFailed,
			setup: func(t *testing.T) (string, []string) {
				repo := newInitializedRepo(t)
				return repo, []string{"task", "show", absentTask}
			},
			// The remedy names `task list`, which is a command rather than a
			// repair: carrying it out is running it and finding the task is
			// genuinely not there.
			clear: func(t *testing.T, repo string, _ []string) []string {
				t.Helper()
				return []string{"task", "list"}
			},
		},
		{
			name: "an unknown session",
			code: app.CodeSessionNotFound,
			exit: app.ExitFailed,
			setup: func(t *testing.T) (string, []string) {
				repo := newInitializedRepo(t)
				return repo, []string{"task", "open", "--title", "work under a session that does not exist",
					"--session", "SES-0000000000000000000000000"}
			},
			// The remedy is to mint a session; the write then succeeds under it.
			clear: func(t *testing.T, repo string, _ []string) []string {
				t.Helper()
				return []string{"task", "open", "--title", "work under a session that does exist",
					"--session", sessionID(t, repo)}
			},
		},
		{
			name: "a move the lifecycle does not have",
			code: app.CodeTaskStateInvalid,
			exit: app.ExitFailed,
			setup: func(t *testing.T) (string, []string) {
				repo := newInitializedRepo(t)
				session := sessionID(t, repo)
				task := openTask(t, repo, session, "a task in OPEN")
				return repo, []string{"task", "state", task, "--to", "COMPLETED", "--session", session}
			},
			// This one's remedy is the interesting case: it lists the moves
			// that *are* available from where the task is, and carrying it out
			// means making one of them.
			clear: func(t *testing.T, repo string, actions []string) []string {
				t.Helper()

				task := onlyTaskIn(t, repo)
				to := firstStateNamedIn(t, actions)
				return []string{"task", "state", task, "--to", to}
			},
		},
		{
			name: "a repository that has never been initialised",
			code: app.CodeCoordinationUnavailable,
			exit: app.ExitFailed,
			setup: func(t *testing.T) (string, []string) {
				return newRepo(t), []string{"task", "list"}
			},
			clear: func(t *testing.T, repo string, actions []string) []string {
				t.Helper()

				if !remedyMentions(actions, "mindrail init") {
					t.Fatalf("the remedy does not name init: %v", actions)
				}
				if got := run(t, repo, "init"); got.code != app.ExitSuccess {
					t.Fatalf("the printed remedy `mindrail init` exited %d: %s", got.code, got.stdout)
				}
				return nil
			},
		},
	}
}

// onlyTaskIn returns the id of the single task in a repository, so a remedy
// closure does not have to be handed one through the row.
func onlyTaskIn(t *testing.T, repo string) string {
	t.Helper()

	listed := run(t, repo, "task", "list", "--json")
	listed.requireExit(t, app.ExitSuccess)

	var data struct {
		Tasks []struct {
			ID string `json:"task_id"`
		} `json:"tasks"`
	}
	decodeData(t, listed.stdout, &data)
	if len(data.Tasks) != 1 {
		t.Fatalf("the repository holds %d tasks, want exactly 1", len(data.Tasks))
	}
	return data.Tasks[0].ID
}

// firstStateNamedIn reads a state out of the remedy the binary printed, rather
// than out of a list this test keeps. A remedy that named a state the lifecycle
// does not allow from here would fail when the move is attempted, which is the
// whole point of carrying it out.
func firstStateNamedIn(t *testing.T, actions []string) string {
	t.Helper()

	for _, action := range actions {
		// Only the part after the colon. The sentence is "From OPEN this task
		// can move to: CLAIMED, ABANDONED." — the state before the colon is
		// where the task already is, and moving there is the self-transition
		// the refusal was about.
		_, offered, found := strings.Cut(action, ":")
		if !found {
			continue
		}
		for _, state := range coordination.States() {
			if strings.Contains(offered, string(state)) {
				return string(state)
			}
		}
	}
	t.Fatalf("no remedy names a state to move to: %v", actions)
	return ""
}

// commandName is the argument list read back as the command a reader would type,
// which is the two words before the first operand.
func commandName(args []string) string {
	if len(args) < 2 {
		return args[0]
	}
	return args[0] + " " + args[1]
}
