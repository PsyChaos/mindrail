package cli_test

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/cli"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// coordinationCommands is every command MR-003 added, as the argument list that
// reaches it.
//
// All six, not one. The startup verdict and the coordination refusal both live
// in one shared body, and a command that stopped calling it would be invisible
// to a test over `task list`.
func coordinationCommands() [][]string {
	const (
		absentTask  = "TSK-0000000000000000000000000"
		absentLease = "LSE-0000000000000000000000000"
	)

	return [][]string{
		{"session", "open"},
		{"task", "open", "--title", "something"},
		{"task", "state", absentTask, "--to", "CLAIMED"},
		{"task", "show", absentTask},
		{"task", "list"},
		{"checkpoint", "write", absentTask, "--note", "something"},
		// MR-004's four, through the same shared body.
		{"lease", "acquire", "--file", "src/something.go"},
		{"lease", "renew", absentLease},
		{"lease", "release", absentLease},
		{"lease", "list"},
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

// TestNoCoordinationFailureReachesTheWireUncoded is the guard the brief asked
// emit for, kept where it can be checked.
//
// The rule is that an envelope whose `ok` is false carries a code this binary
// owns. app.WriteJSON deliberately does not drop an uncoded error — dropping it
// would be worse than reporting it — so the rule is enforced by driving the
// commands over every condition the suite can build rather than by a runtime
// branch that should be unreachable.
//
// The first sweep is the auditor's probe from round 1: `task list --json` over
// all of the MR-001 agreement matrix's rows. That is how finding F09 was found.
// It is not how finding F44's empty code would have been: audit round 2 §4.2
// proved that every row agreementConditions() can build is decided by the
// startup Diagnosis before any coordination command opens the database, so
// none of them ever reaches a single one of store.go's read wraps. The second
// sweep below closes that gap: it damages one row of an otherwise healthy
// database, the way TestADamagedRowIsReportedWithACode does for checkpoints,
// and drives the damage through both a single-row read and a list read.
func TestNoCoordinationFailureReachesTheWireUncoded(t *testing.T) {
	for _, tc := range agreementConditions() {
		repo := tc.setup(t)
		got := runWith(t, repo, tc.options, "task", "list", "--json")

		var envelope struct {
			OK    bool              `json:"ok"`
			Error *app.ErrorPayload `json:"error"`
		}
		if err := json.Unmarshal([]byte(got.stdout), &envelope); err != nil {
			t.Fatalf("%s: stdout is not a JSON envelope: %v\n%s", tc.name, err, got.stdout)
		}
		if envelope.OK {
			continue
		}
		if envelope.Error == nil {
			t.Errorf("%s: the envelope reports failure and carries no error object:\n%s", tc.name, got.stdout)
			continue
		}
		if !app.IsRegistered(envelope.Error.Code) {
			t.Errorf("%s: `task list` failed with code %q, which this binary does not own:\n%s",
				tc.name, envelope.Error.Code, got.stdout)
		}
		assertFourErrorKeys(t, got.stdout)
	}

	for _, tc := range semanticDamageRows() {
		repo, taskID := tc.setup(t)
		assertPublishedFailureHasACode(t, tc.name+": `task show`",
			run(t, repo, "task", "show", taskID, "--json"))

		repo, _ = tc.setup(t)
		assertPublishedFailureHasACode(t, tc.name+": `task list`",
			run(t, repo, "task", "list", "--json"))
	}
}

// semanticDamageRow is one row agreementConditions() cannot build: a
// repository whose startup is clean and whose failure is a single row a
// coordination command cannot parse back, rather than a filesystem or
// configuration state the startup Diagnosis rejects before any store read
// runs.
type semanticDamageRow struct {
	name string
	// setup builds a fresh, healthy repository, damages it, and returns the id
	// of the task the damage was done to. The id is read out of `task open`'s
	// own answer, before the damage is done — after it, even `task list`
	// cannot be trusted to read the id back, which is the condition this row
	// exists to prove.
	setup func(t *testing.T) (repo, taskID string)
}

// semanticDamageRows is audit round 2 §4.2's extension of the sweep above: a
// `tasks` row with an unparseable created_at, the shape
// TestADamagedRowIsReportedWithACode already builds for `checkpoints`. Driven
// through `task show` it meets FindTask's read wrap; driven through `task
// list` it meets ListTasks' scan wrap — the two sites item 2's mutation names.
func semanticDamageRows() []semanticDamageRow {
	return []semanticDamageRow{
		{
			name: "a task row with an unparseable created_at",
			setup: func(t *testing.T) (string, string) {
				t.Helper()

				repo := newInitializedRepo(t)
				session := sessionID(t, repo)
				task := openTask(t, repo, session, "a task whose row will be damaged")
				execOnRuntimeDB(t, repo, `UPDATE tasks SET created_at = 'yesterday'`)
				return repo, task
			},
		},
	}
}

// assertPublishedFailureHasACode is item 13's guard, asserted on a row that is
// built to fail rather than swept from a matrix where most rows are healthy.
// It shares no code with the loop above on purpose: that loop's `continue` on
// success is right when a row may legitimately be healthy, and folding the two
// together would make a row that wrongly succeeded here indistinguishable from
// one of those.
func assertPublishedFailureHasACode(t *testing.T, name string, got result) {
	t.Helper()

	payload := got.errorPayload(t)
	if !app.IsRegistered(payload.Code) {
		t.Errorf("%s: failed with code %q, which this binary does not own:\n%s",
			name, payload.Code, got.stdout)
	}
	assertFourErrorKeys(t, got.stdout)
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
// criterion AC-09.1, which was written and never implemented (finding F13) —
// and, since MR-004, its AC-08.7: the six codes that milestone added are
// rows of the same matrix, each with its remedy carried out. The name keeps
// MR-003's "four" because MR-003's records cite it; the matrix is ten rows.
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

			// A short busy budget: the locked row would otherwise wait the
			// production five seconds on each of its two runs, and no other
			// row meets the lock.
			options := cli.Options{BusyTimeout: 200 * time.Millisecond}

			structured := runWith(t, repo, options, append(slices.Clone(args), "--json")...)
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
			human := runWith(t, repo, options, append(slices.Clone(args), "--no-color")...)
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

		// MR-004's six (AC-08.7, decision D-76).
		{
			name: "a target another session holds",
			code: app.CodeLeaseConflict,
			exit: app.ExitFailed,
			setup: func(t *testing.T) (string, []string) {
				repo := newInitializedRepo(t)
				holder := sessionID(t, repo)
				acquireFile(t, repo, holder, "src/held.go")
				return repo, []string{"lease", "acquire", "--file", "src/held.go", "--session", sessionID(t, repo)}
			},
			// The remedy names the holder's release; carried out, the target
			// is free and the refused acquisition succeeds. The id in the
			// printed remedy belongs to the first repository, so the lease is
			// found again in this one, the way onlyTaskIn finds a task.
			clear: func(t *testing.T, repo string, actions []string) []string {
				t.Helper()
				if !remedyMentions(actions, "mindrail lease release LSE-") {
					t.Fatalf("the remedy does not name the holder's release: %v", actions)
				}
				lease := onlyLeaseOn(t, repo, "src/held.go")
				run(t, repo, "lease", "release", lease.ID, "--session", lease.Holder, "--json").requireExit(t, app.ExitSuccess)
				return nil
			},
		},
		{
			name: "a lease this session no longer holds",
			code: app.CodeLeaseNotHeld,
			exit: app.ExitFailed,
			setup: func(t *testing.T) (string, []string) {
				repo := newInitializedRepo(t)
				me := sessionID(t, repo)
				id := acquireFile(t, repo, me, "src/given-up.go")
				run(t, repo, "lease", "release", id, "--session", me, "--json").requireExit(t, app.ExitSuccess)
				return repo, []string{"lease", "renew", id, "--session", me}
			},
			// The remedy is to acquire again, and it names the file.
			clear: func(t *testing.T, repo string, actions []string) []string {
				t.Helper()
				if !remedyMentions(actions, "lease acquire --file=src/given-up.go") {
					t.Fatalf("the remedy does not name the acquisition: %v", actions)
				}
				return []string{"lease", "acquire", "--file", "src/given-up.go", "--session", sessionID(t, repo)}
			},
		},
		{
			name: "an unknown lease",
			code: app.CodeLeaseNotFound,
			exit: app.ExitFailed,
			setup: func(t *testing.T) (string, []string) {
				repo := newInitializedRepo(t)
				return repo, []string{"lease", "renew", "LSE-0000000000000000000000000", "--session", sessionID(t, repo)}
			},
			clear: func(t *testing.T, repo string, actions []string) []string {
				t.Helper()
				if !remedyMentions(actions, "lease list") {
					t.Fatalf("the remedy does not name `lease list`: %v", actions)
				}
				return []string{"lease", "list"}
			},
		},
		{
			name: "a move decided on a stale reading",
			code: app.CodeStateRevisionConflict,
			exit: app.ExitFailed,
			setup: func(t *testing.T) (string, []string) {
				repo := newInitializedRepo(t)
				me := sessionID(t, repo)
				task := openTask(t, repo, me, "a task read at revision 1 and moved since")
				run(t, repo, "task", "state", task, "--to", "CLAIMED", "--session", me, "--json").requireExit(t, app.ExitSuccess)
				return repo, []string{"task", "state", task, "--to", "IN_PROGRESS", "--session", me, "--expect-revision", "1"}
			},
			// The remedy is to re-read and retry with the revision the task is
			// at, which `task show` prints.
			clear: func(t *testing.T, repo string, actions []string) []string {
				t.Helper()
				if !remedyMentions(actions, "mindrail task show") {
					t.Fatalf("the remedy does not send the caller to re-read: %v", actions)
				}
				task := onlyTaskIn(t, repo)
				shown := run(t, repo, "task", "show", task, "--json")
				shown.requireExit(t, app.ExitSuccess)
				var data struct {
					Task struct {
						Revision  int64  `json:"revision"`
						ClaimedBy string `json:"claimed_by"`
					} `json:"task"`
				}
				decodeData(t, shown.stdout, &data)
				return []string{"task", "state", task, "--to", "IN_PROGRESS", "--session", data.Task.ClaimedBy,
					"--expect-revision", strconv.FormatInt(data.Task.Revision, 10)}
			},
		},
		{
			name: "an operation id reused for a different request",
			code: app.CodeOperationIDConflict,
			exit: app.ExitFailed,
			setup: func(t *testing.T) (string, []string) {
				repo := newInitializedRepo(t)
				me := sessionID(t, repo)
				run(t, repo, "task", "open", "--title", "the first request", "--session", me, "--operation-id", "op-1", "--json").requireExit(t, app.ExitSuccess)
				return repo, []string{"task", "open", "--title", "a different request", "--session", me, "--operation-id", "op-1"}
			},
			// The remedy is a new id.
			clear: func(t *testing.T, repo string, actions []string) []string {
				t.Helper()
				if !remedyMentions(actions, "new operation id") {
					t.Fatalf("the remedy does not say to mint a new id: %v", actions)
				}
				return []string{"task", "open", "--title", "a different request", "--session", sessionID(t, repo), "--operation-id", "op-2"}
			},
		},
		{
			name: "a database another process holds past the budget",
			code: app.CodeBusyRetryable,
			exit: app.ExitUnavailable,
			setup: func(t *testing.T) (string, []string) {
				repo := newInitializedRepo(t)
				holdWriteLockOn(t, repo)
				return repo, []string{"session", "open"}
			},
			// The remedy is to wait for the other command to finish and run
			// this one again: carrying it out is letting the holder go.
			clear: func(t *testing.T, repo string, actions []string) []string {
				t.Helper()
				if !remedyMentions(actions, "run this one again") {
					t.Fatalf("the remedy does not say to run the command again: %v", actions)
				}
				releaseWriteLockOn(t, repo)
				return nil
			},
		},
	}
}

// onlyLeaseOn reads the single active lease on a file off `lease list`, so a
// remedy closure does not have to be handed one through the row.
func onlyLeaseOn(t *testing.T, repo, path string) coordination.Lease {
	t.Helper()
	listed := run(t, repo, "lease", "list", "--json")
	listed.requireExit(t, app.ExitSuccess)
	var data struct {
		Leases []coordination.Lease `json:"leases"`
	}
	decodeData(t, listed.stdout, &data)
	var found []coordination.Lease
	for _, lease := range data.Leases {
		if lease.TargetKind == coordination.TargetFile && lease.TargetKey == path {
			found = append(found, lease)
		}
	}
	if len(found) != 1 {
		t.Fatalf("the repository holds %d active leases on %s, want exactly 1", len(found), path)
	}
	return found[0]
}

// heldWriteLocks remembers the lock holdWriteLockOn took on each repository,
// so the busy row's remedy — the other command finishing — can be carried
// out by releasing it.
var heldWriteLocks = struct {
	sync.Mutex
	release map[string]func()
}{release: map[string]func(){}}

// holdWriteLockOn takes the runtime database's write lock through a second
// handle and holds it until the test ends or releaseWriteLockOn is called,
// which is what another Mindrail command in the middle of a write looks like
// to this one.
func holdWriteLockOn(t *testing.T, repo string) {
	t.Helper()
	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatalf("open runtime database: %v", err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("take the write lock: %v", err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			_ = tx.Rollback()
			_ = db.Close()
		})
	}
	heldWriteLocks.Lock()
	heldWriteLocks.release[repo] = release
	heldWriteLocks.Unlock()
	t.Cleanup(release)
}

// releaseWriteLockOn lets the holder go: the other command finished.
func releaseWriteLockOn(t *testing.T, repo string) {
	t.Helper()
	heldWriteLocks.Lock()
	release, held := heldWriteLocks.release[repo]
	delete(heldWriteLocks.release, repo)
	heldWriteLocks.Unlock()
	if !held {
		t.Fatalf("no write lock is held on %s", repo)
	}
	release()
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
