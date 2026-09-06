package cli_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/cli"
	"github.com/PsyChaos/mindrail/internal/git"
	"github.com/PsyChaos/mindrail/internal/storage"
)

// commandsUnderTest is every MR-001 command that reports on the repository it
// was run in. `version` is not one: it answers about the binary and never looks
// at the disk.
var commandsUnderTest = []string{"status", "doctor", "init"}

// answer is what one command said about one repository, reduced to the three
// things a consumer and a human both act on.
type answer struct {
	exit       int
	code       app.Code
	nextAction []string
}

// condition is one repository state, plus the remedy the report is supposed to
// print for it, so the test can carry that remedy out instead of trusting it.
type condition struct {
	name string
	// setup builds a fresh repository in the state. It is called once per
	// command, because `init` writes and the next command must not inherit what
	// it did.
	setup func(t *testing.T) string
	// options is the process seam the row needs. The zero value is the real git;
	// the rows about a git that is missing, hanging or failing for a reason of
	// its own cannot be arranged any other way, because PATH is process state
	// and this suite runs every row in one process.
	options cli.Options
	// broken says whether every command should report a failure. The over-fire
	// rows set it false and then assert that none of them reports one.
	broken bool
	// clear carries out the remedy on disk. Nil where there is nothing to undo,
	// or where the condition is a process seam rather than a state of the disk.
	clear func(t *testing.T, repo string)
	// remedy is the class of action the report is supposed to print, asserted
	// against next_action before clear runs.
	//
	// It exists because the final phase claimed to carry out "the sentence all
	// three printed" and never read that sentence: it ran this row's own clear
	// closure, so a remedy that was wrong, useless or impossible passed as long
	// as all three commands printed it identically. Replacing the config
	// loader's remedy with "Reinstall Mindrail from your package manager" left
	// the whole suite green (finding F05).
	remedy remedyClass
	// carriedOutDifferently states why clear does something other than what the
	// report printed, for the rows where the printed remedy is real and no test
	// can perform it. Declaring it is how such a row stays honest; the
	// alternative — silently substituting a different action — is the defect.
	//
	// It does NOT excuse the row from declaring its remedy class. It used to:
	// the field was free text and setting it skipped the assertion entirely, so
	// any future row could silence the check with a sentence, and applying
	// finding F05's own mutation alongside it left the suite green. The class is
	// asserted either way; this only explains the divergence between the printed
	// remedy and what clear does about it.
	carriedOutDifferently string
	// separately names a command whose answer legitimately differs from the ones
	// that agree, and states why in the row.
	//
	// It exists because `init` acts where `status` and `doctor` only observe, so
	// a handful of conditions really do end differently depending on which
	// command met them — and loosening the whole assertion to accommodate them
	// would give back exactly the coverage this test was written for. An
	// exemption is checked in both directions: the row must still differ, or the
	// exemption is stale and has to be deleted rather than left standing.
	separately map[string]string
}

// TestOneConditionIsNamedTheSameWayByEveryCommand is finding D5, generalised.
//
// A regular file at <worktree>/.mindrail was one condition with two diagnoses:
// `status` and `doctor` called it CONFIG_INVALID and exited 2, remedying it with
// "fix or remove the offending entry in .mindrail/config.toml" — a path that
// cannot be opened, because a file is standing where its directory has to be —
// while `init` called the identical disk RUNTIME_PATH_UNWRITABLE and exited 4.
// A reader who ran the diagnostic command got a remedy that could not be carried
// out; a reader who ran the writing command got a different code for the same
// bytes on the same disk.
//
// The invariant from pass 3 could not catch it. That one reads one document and
// asks whether it contradicts its own error object, and here the two disagreeing
// answers are in two different documents produced by two different commands.
// This is the missing half: one condition, one code, one exit class, one remedy,
// whichever command the reader happened to run.
//
// The remedy is then carried out, because a set of three identical sentences
// that do not clear the condition is three commands agreeing on the wrong
// answer.
//
// The matrix is the whole broken-setup matrix and not a sample of it. For five
// audits it held three rows, and finding E10 was found by adding a fourth — an
// unreadable `.mindrail/knowledge`, which `status` and `doctor` called
// KNOWLEDGE_UNREADABLE at exit 1 with an inspection-only remedy while `init`
// called it RUNTIME_PATH_UNWRITABLE at exit 4 with the chmod that actually
// clears it — and watching all three assertions fail at once. A matrix with a
// gap in it is a matrix that reports on the rows somebody happened to think of.
func TestOneConditionIsNamedTheSameWayByEveryCommand(t *testing.T) {
	for _, tc := range agreementConditions() {
		t.Run(tc.name, func(t *testing.T) {
			answers := make(map[string]answer, len(commandsUnderTest))
			for _, command := range commandsUnderTest {
				repo := tc.setup(t)
				answers[command] = answerOf(t, repo, tc.options, command)
			}

			agreeing := agreeingCommands(tc)
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
						t.Errorf("%s: %s reported %q at exit %d on a repository nothing is wrong with",
							tc.name, command, got.code, got.exit)
					}
				}
			}

			if tc.clear == nil {
				return
			}

			// The sentence all three printed, read before it is carried out.
			assertRemedyIsWhatTheRowExpects(t, tc, answers[first].nextAction)

			repo := tc.setup(t)
			tc.clear(t, repo)
			for _, command := range commandsUnderTest {
				if got := runWith(t, repo, tc.options, command); got.code != app.ExitSuccess {
					t.Fatalf("%s: after carrying out the remedy %v, `%s` exited %d: %v\n%s",
						tc.name, answers[first].nextAction, command, got.code, got.err, got.stdout)
				}
			}
		})
	}
}

// The remedy classes only this matrix asks about.
//
// coherence_test.go's classifier reads the sentences that name a path, because
// that is what a path-level contradiction is made of. These four name no path —
// they send the reader into the configuration file, back to `mindrail init`, or
// to a newer binary — so they are invisible to it and would all read as
// classUnknown, which asserts nothing.
const (
	classEditConfig remedyClass = "correct the entry the report named"
	classRebuild    remedyClass = "move the runtime database aside and run mindrail init"
	classUpgrade    remedyClass = "upgrade to a binary that reads these records"
	classContain    remedyClass = "put the path back inside the repository"
)

// agreementRemedyPhrases extends coherence_test.go's table with the four above,
// most specific first. A phrase that appears in neither table classifies as
// nothing, which is the honest failure: an unrecognised remedy is not read as
// agreeing with the row, it makes the row fail until somebody writes it down.
var agreementRemedyPhrases = []struct {
	phrase string
	class  remedyClass
}{
	{"to rebuild it", classRebuild},
	{"then run `mindrail init`", classRebuild},
	{"fix or remove the offending entry", classEditConfig},
	{"upgrade mindrail", classUpgrade},
	{"use a path inside the repository", classContain},
}

// agreementRemedyClass reads a next_action list the way this matrix needs it:
// the shared classifier first, so that a condition both tests know is named the
// same way by both, then the four remedies only this one asks about.
func agreementRemedyClass(actions []string) remedyClass {
	for _, action := range actions {
		if class := classOf(action); class != classUnknown {
			return class
		}
		lowered := strings.ToLower(action)
		for _, candidate := range agreementRemedyPhrases {
			if strings.Contains(lowered, candidate.phrase) {
				return candidate.class
			}
		}
	}
	return classUnknown
}

// assertRemedyIsWhatTheRowExpects holds the printed remedy to the class the row
// declared before the row's own clear closure is allowed to run.
//
// A row that declares neither a class nor a reason it cannot is a row that has
// not been reconciled with the binary, and it fails rather than passing quietly:
// that silence is what let five mutation survivors through (finding F05).
func assertRemedyIsWhatTheRowExpects(t *testing.T, tc condition, actions []string) {
	t.Helper()

	if tc.remedy == classUnknown {
		t.Fatalf("%s: the row carries out a remedy but does not say which one it expects the report to print; "+
			"set remedy. The report printed %q", tc.name, actions)
	}

	if got := agreementRemedyClass(actions); got != tc.remedy {
		t.Fatalf("%s: the report tells the reader to %q, but the row expects to %q\nnext_action: %q",
			tc.name, got, tc.remedy, actions)
	}
}

// agreementConditions is the matrix: every condition six audits have produced,
// grouped by the subsystem whose failure it is.
func agreementConditions() []condition {
	return slices.Concat(
		discoveryConditions(),
		repositoryConfigConditions(),
		knowledgeConditions(),
		runtimeRootConditions(),
		runtimeDatabaseConditions(),
		healthyConditions(),
	)
}

// discoveryConditions are the §87 step-1 failures: no repository, no git, or a
// git that will not answer. None of them is a state of a repository, so none of
// them has a remedy this test can carry out on disk.
func discoveryConditions() []condition {
	return []condition{
		{
			name:    "git is not installed",
			broken:  true,
			setup:   newRepo,
			options: cli.Options{Runner: unavailableGit()},
		},
		{
			name:    "git does not answer in time",
			broken:  true,
			setup:   newRepo,
			options: cli.Options{Runner: hangingGit()},
		},
		{
			// Finding F1's condition, kept in the matrix because the remedy for
			// it is the one that must never be `git init`: git ran, found a
			// repository, and refused to read it.
			name:    "git exits 128 for a reason other than a missing repository",
			broken:  true,
			setup:   newRepo,
			options: cli.Options{Runner: unreadableRepositoryGit()},
		},
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
			name:   "-C names a directory that does not exist",
			broken: true,
			setup: func(t *testing.T) string {
				t.Helper()
				requireGit(t)
				isolateEnvironment(t)
				return filepath.Join(t.TempDir(), "gone")
			},
		},
	}
}

// repositoryConfigConditions are the states of <worktree>/.mindrail, which is
// where the two disagreeing answers of finding D5 came from: the config loader
// reads that directory and `init` writes it.
func repositoryConfigConditions() []condition {
	return []condition{
		{
			name:   "a regular file where the repository config directory belongs",
			broken: true,
			remedy: classObstruction,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				writeFile(t, repoConfigDir(repo), []byte("not a directory"))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, repoConfigDir(repo))
			},
		},
		{
			name:   "a worktree that will not accept the repository config directory",
			broken: true,
			remedy: classPermission,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				denyWrites(t, repo)
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				chmodForRemedy(t, repo, 0o700)
			},
		},
		{
			name:   "a dangling symlink where the repository config directory belongs",
			broken: true,
			remedy: classRelink,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				if err := os.Symlink(filepath.Join(repo, "gone"), repoConfigDir(repo)); err != nil {
					t.Fatalf("create a dangling symlink: %v", err)
				}
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, repoConfigDir(repo))
			},
		},
		{
			// The containment boundary of decision D-32 and spec §113, met from
			// the one direction that is not an obstruction: the link resolves
			// perfectly, to a directory outside the repository. It is a usage
			// error rather than an environment one, and all three commands have
			// to say so with the same words.
			name:   "a symlink pointing outside the worktree where the repository config directory belongs",
			broken: true,
			remedy: classContain,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				outside := t.TempDir()
				if err := os.Symlink(outside, repoConfigDir(repo)); err != nil {
					t.Fatalf("point .mindrail out of the worktree: %v", err)
				}
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, repoConfigDir(repo))
			},
		},
		{
			// The other half of finding D4: a `.mindrail` that refuses writes and
			// still has an entry `mindrail init` has to create in it. The
			// over-fire guard for it — the same mode with nothing left to create
			// — is in healthyConditions.
			name:   "the repository config directory refuses writes and the scaffold is incomplete",
			broken: true,
			remedy: classPermission,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				removeForSetup(t, filepath.Join(repoConfigDir(repo), "knowledge"))
				denyWrites(t, repoConfigDir(repo))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				chmodForRemedy(t, repoConfigDir(repo), 0o755)
			},
		},
		{
			name:   "config.toml cannot be parsed",
			broken: true,
			remedy: classEditConfig,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeFile(t, configPath(repo), []byte("output.color = \n"))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, configPath(repo))
			},
		},
		{
			// A key this binary does not know is fatal rather than ignored: §37
			// makes the effective configuration answerable, and a setting that
			// was silently dropped is one nobody can answer for. The remedy is
			// "correct or remove the reported key", and it is carried out as
			// written rather than by deleting the whole file.
			name:   "config.toml carries a key this binary does not know",
			broken: true,
			remedy: classEditConfig,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeFile(t, configPath(repo), []byte("[output]\ncolour = \"always\"\n"))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, configPath(repo), []byte("[output]\ncolor = \"auto\"\n"))
			},
		},
		{
			// Finding F06's first gap. The matrix had rows for a `.mindrail`
			// that is a file, unwritable, dangling or escaping the root, and
			// rows for config.toml's *contents* being wrong — and none for the
			// file being unopenable, which is the shape that hid finding F02's
			// remedy: an entry-level instruction printed for a file with no
			// readable entry in it.
			name:   "config.toml cannot be opened",
			broken: true,
			remedy: classPermission,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyAccess(t, configPath(repo))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				chmodForRemedy(t, configPath(repo), 0o644)
			},
		},
		{
			// F06's second gap, and the one a repository can reach without
			// anybody's permissions being unusual: a tree may legitimately
			// commit a path named .mindrail/config.toml, and then every command
			// in every fresh clone meets a directory where the file belongs.
			name:   "a directory where config.toml belongs",
			broken: true,
			remedy: classObstruction,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				removeForSetup(t, configPath(repo))
				if err := os.Mkdir(configPath(repo), 0o755); err != nil {
					t.Fatalf("occupy config.toml with a directory: %v", err)
				}
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, configPath(repo))
			},
		},
	}
}

// knowledgeConditions are the states of <worktree>/.mindrail/knowledge, which
// finding E10 is about.
func knowledgeConditions() []condition {
	return []condition{
		{
			name:   "the knowledge directory is unreadable",
			broken: true,
			remedy: classPermission,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyAccess(t, knowledgeDir(repo))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				chmodForRemedy(t, knowledgeDir(repo), 0o755)
			},
		},
		{
			name:   "a regular file where the knowledge directory belongs",
			broken: true,
			remedy: classObstruction,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				mkdirForSetup(t, repoConfigDir(repo))
				writeFile(t, knowledgeDir(repo), []byte("not a directory"))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, knowledgeDir(repo))
			},
		},
		{
			// One directory further down than the row above. The loader names
			// the record bucket it could not walk, `init` fails on the .gitkeep
			// it has to write inside it, and both have to name that directory.
			name:   "a record bucket under the knowledge directory is unreadable",
			broken: true,
			remedy: classPermission,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyAccess(t, filepath.Join(knowledgeDir(repo), "decisions"))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				chmodForRemedy(t, filepath.Join(knowledgeDir(repo), "decisions"), 0o755)
			},
		},
		{
			// The containment boundary met one directory deeper than the
			// `.mindrail` row, and the gap that hid a second D5: the loader
			// walks `.mindrail/knowledge` and reported a link out of the
			// repository as KNOWLEDGE_UNREADABLE at exit 1 with "check that it
			// is a readable directory" — an instruction already satisfied,
			// because the link resolves to a directory that reads perfectly —
			// while `init` resolved the same path through the boundary and said
			// PATH_ESCAPES_ROOT at exit 2.
			name:   "the knowledge directory points outside the worktree",
			broken: true,
			remedy: classContain,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				removeForSetup(t, knowledgeDir(repo))
				if err := os.Symlink(t.TempDir(), knowledgeDir(repo)); err != nil {
					t.Fatalf("point .mindrail/knowledge out of the worktree: %v", err)
				}
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, knowledgeDir(repo))
			},
		},
		{
			// The over-fire guard for both rows above: one record file nobody
			// can read is a problem with that record, not with the directory
			// holding it, and the path remedy must not fire on it.
			name: "a single knowledge record that cannot be read",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "DEC-0001.json", "{}")
				denyAccess(t, filepath.Join(knowledgeDir(repo), "decisions", "DEC-0001.json"))
				return repo
			},
		},
		{
			// Decision D-06: a clone with no records is healthy, and `init`
			// re-creates the scaffold it laid down. Every command has to say so.
			name: "the knowledge directory was removed after init",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				removeForSetup(t, knowledgeDir(repo))
				return repo
			},
		},
		{
			// One record this binary cannot parse costs the repository that
			// record and nothing else, so no command may fail on it.
			name: "a knowledge record that is not valid JSON",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "DEC-0001.json", "{not json")
				return repo
			},
		},
		{
			name:   "a knowledge record written by a newer schema than this binary reads",
			broken: true,
			remedy: classUpgrade,
			// The report tells the reader to upgrade the binary, which is the
			// right instruction and one no test can carry out. Deleting the
			// record clears the condition by removing what the newer schema
			// wrote, which is a different action, and finding F05 is that the
			// matrix used to make that substitution silently.
			carriedOutDifferently: "upgrading the binary is not an action a test can perform; " +
				"clear deletes the record the newer schema wrote instead",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "DEC-0001.json", futureSchemaRecord)
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, filepath.Join(knowledgeDir(repo), "decisions", "DEC-0001.json"))
			},
		},
	}
}

// runtimeRootConditions are the states of the machine-local runtime tree under
// the Git common directory.
func runtimeRootConditions() []condition {
	return []condition{
		{
			name:   "the runtime root cannot be created",
			broken: true,
			remedy: classPermission,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				denyWrites(t, filepath.Join(repo, ".git"))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				chmodForRemedy(t, filepath.Join(repo, ".git"), 0o755)
			},
		},
		{
			name:   "the runtime root refuses writes after init",
			broken: true,
			remedy: classPermission,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyWrites(t, runtimeRoot(repo))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				chmodForRemedy(t, runtimeRoot(repo), 0o700)
			},
		},
		{
			name:   "a regular file where the runtime root belongs",
			broken: true,
			remedy: classObstruction,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				writeFile(t, runtimeRoot(repo), []byte("not a directory"))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, runtimeRoot(repo))
			},
		},
		{
			// Decision D-01's one zero-exit row, reached from the other side: the
			// repository was initialised and the runtime tree was then deleted.
			// It is the same state as never having run init, and every command
			// has to grade it that way.
			name: "the runtime directory was deleted after init",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				removeForSetup(t, runtimeRoot(repo))
				return repo
			},
		},
		{
			// Finding F13: MR-001 stores nothing in the cache, so an unusable
			// cache directory is reported and stops nothing.
			name: "the cache directory refuses writes",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyWrites(t, filepath.Join(runtimeRoot(repo), "cache"))
				return repo
			},
		},
	}
}

// runtimeDatabaseConditions are the states of the runtime database itself.
func runtimeDatabaseConditions() []condition {
	return []condition{
		{
			name:   "the database was truncated to garbage",
			broken: true,
			remedy: classRebuild,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				corruptDatabase(t, repo)
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, runtimeDBPath(t, repo))
			},
		},
		{
			// Finding H14: an interrupted first run leaves a zero-length file at
			// the database path. It is the uninitialised state, not a corrupt
			// database, and it exits 0.
			name: "a zero-length database file",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				truncateForSetup(t, runtimeDBPath(t, repo))
				return repo
			},
		},
		{
			name:   "the database path is occupied by a directory",
			broken: true,
			remedy: classObstruction,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				mkdirForSetup(t, runtimeDBPath(t, repo))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, runtimeDBPath(t, repo))
			},
		},
		{
			// Finding W5: the file opens, reports WAL and foreign keys, and
			// refuses every write.
			name:   "the database file refuses writes",
			broken: true,
			remedy: classPermission,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				chmodForTest(t, runtimeDBPath(t, repo), 0o444)
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				chmodForRemedy(t, runtimeDBPath(t, repo), 0o600)
			},
		},
		{
			name:   "the runtime tables were dropped",
			broken: true,
			remedy: classRebuild,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				dropRuntimeTables(t, repo)
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, runtimeDBPath(t, repo))
			},
		},
		{
			name:   "the migration ledger rows were deleted",
			broken: true,
			remedy: classRebuild,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				emptyMigrationLedger(t, repo)
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				removeForRemedy(t, runtimeDBPath(t, repo))
			},
		},
	}
}

// healthyConditions are the over-fire guards. Every new detection has to leave
// them alone, and every one of them is the adjacent state of a row above.
func healthyConditions() []condition {
	return []condition{
		{name: "a healthy initialised repository", setup: newInitializedRepo},
		{name: "a repository that has never been initialised", setup: newRepo},
		{
			// The adjacent condition to "the repository config directory refuses
			// writes and the scaffold is incomplete": the same mode with nothing
			// left to create, where `mindrail init` exits 0.
			name: "a repository config directory with nothing left to receive",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyWrites(t, repoConfigDir(repo))
				return repo
			},
		},
		{
			// The adjacent condition to "the knowledge directory is unreadable":
			// a knowledge tree that can be read and not added to, which stops
			// nothing MR-001 does.
			name: "a knowledge directory that refuses writes with nothing left to receive",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyWrites(t, knowledgeDir(repo))
				return repo
			},
		},
	}
}

// futureSchemaRecord is a decision stamped with a schema version outside this
// binary's reader window (kernel-scope §3), which is the one knowledge problem
// that is fatal rather than costing a single record.
const futureSchemaRecord = `{"schema_version": 99, "id": "DEC-0001", "title": "from the future", "status": "accepted"}`

// answerOf runs one command in JSON mode and reduces its envelope.
func answerOf(t *testing.T, repo string, options cli.Options, command string) answer {
	t.Helper()
	return answerFrom(t, repo, command, runWith(t, repo, options, command, "--json"))
}

// answerFrom reduces an envelope that has already been rendered, for a caller
// that needs the raw document as well — the coherence invariant reads the same
// run rather than provoking a second one, because a condition that is a state of
// the *filesystem* cannot be arranged twice cheaply.
func answerFrom(t *testing.T, repo, command string, got result) answer {
	t.Helper()

	var envelope struct {
		Error *app.ErrorPayload `json:"error"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &envelope); err != nil {
		t.Fatalf("%s: stdout is not a JSON envelope: %v\n%s", command, err, got.stdout)
	}

	reduced := answer{exit: got.code}
	if envelope.Error != nil {
		reduced.code = envelope.Error.Code
		// Each command gets its own freshly built repository, because `init`
		// writes and the next command must not inherit what it did. That makes
		// the absolute paths differ between the three answers while the sentence
		// around them is the thing under test, so the repository root is folded
		// out before they are compared.
		reduced.nextAction = make([]string, 0, len(envelope.Error.NextAction))
		for _, action := range envelope.Error.NextAction {
			reduced.nextAction = append(reduced.nextAction, strings.ReplaceAll(action, repo, "<REPO>"))
		}
	}
	return reduced
}

// agreeingCommands is the commands a row expects to answer identically.
func agreeingCommands(tc condition) []string {
	agreeing := make([]string, 0, len(commandsUnderTest))
	for _, command := range commandsUnderTest {
		if _, exempt := tc.separately[command]; !exempt {
			agreeing = append(agreeing, command)
		}
	}
	if len(agreeing) == 0 {
		return commandsUnderTest
	}
	return agreeing
}

// assertExemptionsAreStillEarned checks an exemption in both directions.
//
// An exemption is narrow by construction: it excuses the *remedy* and nothing
// else. A command that names a different condition or exits in a different class
// is the disagreement this whole test exists to catch, and no row may write that
// off as "init acts where status observes" — that sentence explains a remedy
// naming a directory init created, not two different diagnoses.
//
// And it has to still be earned. When the divergence goes away — because the
// code was fixed, or because the condition changed — the exemption stops
// describing anything and starts hiding the assertion it was carved out of, so
// an exempted command that now agrees is an error too.
func assertExemptionsAreStillEarned(t *testing.T, name string, agreed answer, answers map[string]answer, separately map[string]string) {
	t.Helper()

	for command, why := range separately {
		got := answers[command]
		if got.code != agreed.code {
			t.Errorf("%s: `%s` is exempted from the remedy because %q, and it also calls the condition %q where the rest call it %q; an exemption does not cover the code",
				name, command, why, got.code, agreed.code)
		}
		if got.exit != agreed.exit {
			t.Errorf("%s: `%s` is exempted from the remedy because %q, and it also exits %d where the rest exit %d; an exemption does not cover the exit class",
				name, command, why, got.exit, agreed.exit)
		}
		if slices.Equal(got.nextAction, agreed.nextAction) {
			t.Errorf("%s: `%s` is exempted from the agreement because %q, and it now prints the same remedy as the rest; delete the exemption",
				name, command, why)
		}
	}
}

// assertSameAnswer compares two commands' answers about one condition.
//
// The exit code is compared rather than only its class, because decision D-03
// maps each code to exactly one exit status: two commands that agree on the code
// and disagree on the status would mean the map itself had been broken.
func assertSameAnswer(t *testing.T, condition, leftName string, left answer, rightName string, right answer) {
	t.Helper()

	if left.code != right.code {
		t.Errorf("%s: `%s` calls it %q and `%s` calls it %q; one condition has one code",
			condition, leftName, left.code, rightName, right.code)
	}
	if left.exit != right.exit {
		t.Errorf("%s: `%s` exits %d and `%s` exits %d for the same disk",
			condition, leftName, left.exit, rightName, right.exit)
	}
	if !slices.Equal(left.nextAction, right.nextAction) {
		t.Errorf("%s: the two commands print different remedies for one condition\n  %s: %v\n  %s: %v",
			condition, leftName, left.nextAction, rightName, right.nextAction)
	}
}

// --- matrix fixtures ---------------------------------------------------------

func repoConfigDir(repo string) string { return filepath.Join(repo, ".mindrail") }

func knowledgeDir(repo string) string { return filepath.Join(repoConfigDir(repo), "knowledge") }

func runtimeRoot(repo string) string { return filepath.Join(repo, ".git", "mindrail") }

// newNonRepositoryDir is an ordinary directory with no repository above it.
func newNonRepositoryDir(t *testing.T) string {
	t.Helper()
	requireGit(t)
	isolateEnvironment(t)
	return t.TempDir()
}

// unreadableRepositoryGit is a git that found a repository and refused to read
// it, which is the one exit-128 shape whose remedy must never be `git init`
// (finding F1). The stderr is git 2.55's own, under the LC_ALL=C the runner
// forces.
func unreadableRepositoryGit() *git.FakeRunner {
	return &git.FakeRunner{Default: git.FakeResponse{
		Stderr: "fatal: detected dubious ownership in repository at '/srv/repo'\n" +
			"To add an exception for this directory, call:\n\n" +
			"\tgit config --global --add safe.directory /srv/repo\n",
		Err: errors.New("exit status 128"),
	}}
}

// dropRuntimeTables removes the tables the migrations created and leaves the
// ledger claiming they are there, which is what a hand-edited database looks
// like from the next run's side.
func dropRuntimeTables(t *testing.T, repo string) {
	t.Helper()

	execOnRuntimeDB(t, repo,
		`DROP INDEX IF EXISTS idx_workspaces_project`,
		`DROP TABLE IF EXISTS workspaces`,
		`DROP TABLE IF EXISTS projects`)
}

// emptyMigrationLedger deletes every applied-migration row without touching the
// tables those migrations created, which is the other half of the same hazard:
// the schema is there and the record of it is gone.
func emptyMigrationLedger(t *testing.T, repo string) {
	t.Helper()
	execOnRuntimeDB(t, repo, `DELETE FROM schema_migrations`)
}

func execOnRuntimeDB(t *testing.T, repo string, statements ...string) {
	t.Helper()

	db, err := storage.Open(t.Context(), storage.Options{Path: runtimeDBPath(t, repo)})
	if err != nil {
		t.Fatalf("open runtime database: %v", err)
	}
	defer func() { _ = db.Close() }()

	err = storage.InTx(t.Context(), db.DB, func(ctx context.Context, tx *sql.Tx) error {
		for _, statement := range statements {
			if _, execErr := tx.ExecContext(ctx, statement); execErr != nil {
				return execErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("rewrite the runtime database: %v", err)
	}
}

// mkdirForSetup, removeForSetup and truncateForSetup arrange a condition;
// chmodForRemedy and removeForRemedy carry a printed one out. They are spelled
// apart so a failure says which half of the row failed.
func mkdirForSetup(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("arrange %s: %v", dir, err)
	}
}

func removeForSetup(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("arrange %s: %v", path, err)
	}
}

func truncateForSetup(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatalf("arrange %s%s: %v", path, suffix, err)
		}
	}
	if err := os.Truncate(path, 0); err != nil {
		t.Fatalf("arrange %s: %v", path, err)
	}
}

func removeForRemedy(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("carry out the remedy on %s: %v", path, err)
	}
}

func chmodForRemedy(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("carry out the remedy on %s: %v", path, err)
	}
}
