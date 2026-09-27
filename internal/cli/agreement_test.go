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
	// A row whose clear does something other than what the report printed says
	// so in a comment on the row. There was a field for it, and the field was
	// worse than nothing: free text that skipped the class assertion entirely,
	// so any future row could silence the check with a sentence — applying
	// finding F05's own mutation alongside it left the suite green. Once the
	// class is asserted unconditionally there is nothing left for it to carry
	// that a comment does not carry better.
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

// agreementRemedyClass reads a next_action list as the class of its first
// classifiable sentence.
//
// It used to consult a private table of its own, holding the four remedies the
// shared classifier did not know. That split was the defect: this side could
// tell a rebuild from an upgrade while coherence_test.go's path comparison could
// tell neither from nothing, so an entire vocabulary of real remedies was
// invisible to the invariant that compares what two documents say about one
// path. There is one table now, in coherence_test.go, and both read it.
func agreementRemedyClass(actions []string) remedyClass {
	for _, action := range actions {
		if class := classOf(action); class != classUnknown {
			return class
		}
	}
	return classUnknown
}

// TestEveryRemedyThatNamesAPathIsClassified is the harvest that stops the
// classifier drifting away from the binary.
//
// The table in coherence_test.go is only worth what its coverage of the real
// sentences is worth, and it was tested with invented ones: rewording all three
// `Make <path> writable.` sites passed the whole suite, and five real
// path-naming remedies classified as nothing at all. A remedy the table does not
// recognise is not read as disagreeing — it is read as silence — so the
// contradiction check simply stops looking at those paths.
//
// This drives every condition the matrix knows, collects what the binary
// actually printed, and requires each sentence that names a path to be one the
// table can read. A new remedy either gets a class or fails here.
func TestEveryRemedyThatNamesAPathIsClassified(t *testing.T) {
	seen := 0

	// Deliberately not subtests: the setups are ordinary helpers on the test
	// they are handed, and a harvest that counted in a subtest and checked the
	// count in the parent would be reading it before the rows had finished.
	//
	// The envelope is read raw rather than through answerOf, which folds the
	// repository root out to `<REPO>` so three commands in three fresh
	// repositories can be compared. That normalisation is right for the
	// comparison and wrong here: the question is which remedies name an
	// absolute path, and the answer has to be about the paths the binary
	// actually printed.
	for _, tc := range agreementConditions() {
		if !tc.broken {
			continue
		}

		repo := tc.setup(t)
		for _, command := range commandsUnderTest {
			got := runWith(t, repo, tc.options, command, "--json")

			var envelope struct {
				Error *app.ErrorPayload `json:"error"`
			}
			if err := json.Unmarshal([]byte(got.stdout), &envelope); err != nil {
				t.Fatalf("%s/%s: stdout is not a JSON envelope: %v\n%s", tc.name, command, err, got.stdout)
			}
			if envelope.Error == nil {
				continue
			}

			for _, action := range envelope.Error.NextAction {
				if len(absolutePathsIn(action)) == 0 {
					continue
				}
				seen++
				if classOf(action) == classUnknown {
					t.Errorf("%s: `%s` prints %q, which names a path and classifies as nothing; "+
						"the contradiction check cannot see this remedy at all", tc.name, command, action)
				}
			}
		}
	}

	if seen == 0 {
		t.Fatal("no path-naming remedy was harvested, so this proves nothing")
	}
	t.Logf("classified %d path-naming remedies", seen)
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
			// The containment boundary met one level below the row above: the
			// directory is where it belongs and one file inside it is a link out
			// of the worktree (finding BA-10). It is deliberately NOT the row
			// above's grade.
			//
			// A knowledge directory that escapes costs the repository its whole
			// store and is a usage error every command has to refuse; one record
			// that escapes costs the repository that record, exactly as an
			// unreadable one does (decision D-39), so all three commands exit 0
			// with no error object at all. That is what `broken: false` asserts
			// here, and the choice is the measured one rather than the plausible
			// one — `doctor --json`, `status --json` and `init --json` were run
			// against this disk and none of them produced an error payload.
			//
			// The condition still has to be visible, and it is: the knowledge
			// component and the knowledge check both read DEGRADED with code
			// PATH_ESCAPES_ROOT under its own D-51 branch. An exit-0 row here
			// cannot see a code, so that half is asserted by TestBrokenSetupMatrix's
			// "knowledge record that resolves outside the repository" row. The two
			// rows are one condition read at two altitudes: this one says no
			// command fails on it, that one says every command names it.
			name: "a single knowledge record that resolves outside the worktree",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "DEC-0001.json", supersededDecision)
				linkKnowledgeRecordOutside(t, repo, "DEC-0002.json")
				return repo
			},
		},
		{
			// The row above with one genuinely unreadable record beside it, which
			// is the boundary of decision D-51 rather than a second instance of
			// it: a set that is homogeneously escaping is published as
			// PATH_ESCAPES_ROOT, and a set that is not stays KNOWLEDGE_UNREADABLE,
			// because that code is wrong about none of it.
			//
			// It is here because the two rows differ in the published code and in
			// nothing a reader of this file would notice, so a branch that fired
			// on "any escaping record" instead of "every degraded record escapes"
			// would pass the row above and change this one. The code itself is
			// asserted in TestBrokenSetupMatrix; what this row adds is that the
			// mixture is still exit 0 from all three commands and still produces
			// no error object.
			name: "a knowledge record that escapes beside one that cannot be read",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "DEC-0001.json", supersededDecision)
				linkKnowledgeRecordOutside(t, repo, "DEC-0002.json")
				writeKnowledgeRecord(t, repo, "DEC-0003.json", "{not json")
				denyAccess(t, filepath.Join(knowledgeDir(repo), "decisions", "DEC-0003.json"))
				return repo
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
			// Carried out differently, deliberately: the report tells the reader
			// to upgrade the binary, which is the right instruction and one no
			// test can perform. clear deletes the record the newer schema wrote,
			// which is a different action that clears the same condition.
			// Finding F05 is that the matrix used to make this substitution
			// silently, for every row; the class above is asserted either way.
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
		{
			// MR-002's second fatal knowledge condition (decision D-39). Two
			// Decisions that supersede each other leave the lineage with no
			// newest record, so "which Decision is current" has no right answer
			// for either of them — which is a different cost from a malformed
			// record, and is why it is the one finding graded ERROR.
			name:   "two knowledge records that supersede each other",
			broken: true,
			remedy: classSupersedeCycle,
			// Carried out as printed, on one of the two files the report named.
			// The report says to drop the superseded id from DEC-0001 and from
			// DEC-0002, because either edge removed opens the loop; clear takes
			// the first of those two sentences and does exactly what it says.
			//
			// The bytes it leaves behind are the ones the row below starts from,
			// so what this proves is stronger than "all three then exit 0": the
			// cleared state is the legitimate supersede whose own row asserts
			// that nothing is wrong with it.
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "DEC-0001.json", cyclicSupersededDecision)
				writeKnowledgeRecord(t, repo, "DEC-0002.json", supersedingDecision)
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				rewriteKnowledgeRecordForRemedy(t, repo, "DEC-0001.json", supersededDecision)
			},
		},
		{
			// The over-fire guard for the row above, and the one this milestone
			// can least afford to be missing: spec §93's ordinary lineage, which
			// every repository that has ever revised a Decision carries. DEC-0002
			// supersedes DEC-0001 and DEC-0001 records that by carrying status
			// "superseded".
			//
			// It must read as neither of the two conditions that surround it: not
			// a cycle, because the walk from DEC-0002 reaches DEC-0001 and stops
			// there, and not a duplicate active lineage, because exactly one of
			// the pair is still active. A detection that fires here fires on
			// working repositories.
			name: "one knowledge record that legitimately supersedes another",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "DEC-0001.json", supersededDecision)
				writeKnowledgeRecord(t, repo, "DEC-0002.json", supersedingDecision)
				return repo
			},
		},
		{
			// Audit round 2's HIGH, pinned at the layer a user meets it.
			//
			// The two records are the healthy lineage of the row above, unchanged.
			// Beside them is a draft that step 5 rejects and that claims DEC-0001's
			// id. The pipeline's graph indexes every record it READ — a rejected
			// record's supersedes is what a round-1 fix restored, and restoring it
			// also started letting a rejected record's *id* speak for a node — so
			// the draft's "supersedes":["DEC-0002"] was unioned onto DEC-0001's
			// node and closed the lineage into a loop. Both correct files were
			// named in a fatal KNOWLEDGE_SUPERSEDE_CYCLE, at exit 1, with a remedy
			// neither of their bytes could carry out: the sentence told the reader
			// to drop a superseded id from files that were already right.
			//
			// Measured on the binary at the commit the audit graded: `doctor`,
			// `status` and `init` all exited 1 and published
			// KNOWLEDGE_SUPERSEDE_CYCLE. This row is `broken: false` because the
			// answer is that nothing here is fatal — a draft costs the repository
			// the draft (decision D-39), and the two records beside it are current
			// and answerable.
			//
			// It sits beside the cycle rows deliberately. A detection that fires
			// here fires on any working tree with a half-written record in it,
			// which is every working tree at some point in an afternoon.
			name: "a schema-invalid draft carrying a valid record's id",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "DEC-0001.json", supersededDecision)
				writeKnowledgeRecord(t, repo, "DEC-0002.json", supersedingDecision)
				writeKnowledgeRecord(t, repo, "DEC-0003.json", draftReusingAnActiveId)
				return repo
			},
		},
		{
			// The fail-open arm of the seam the three rows above guard, and the
			// gap audit round 3 found in this matrix: every knowledge row here
			// asked whether the pipeline refuses too much, and none asked whether
			// it still refuses at all. A guard checked in one direction only is
			// how the second remediation shipped an over-fire while closing a
			// fail-open.
			//
			// The store is audit round 3's own. DEC-0001.json and DEC-0002.json
			// supersede each other, so the lineage closes on itself exactly as in
			// the cycle row above; DEC-0001.json also carries a property no schema
			// defines, and DEC-0009.json is a schema-valid record carrying
			// DEC-0001's id that declares no supersede at all. Neither addition
			// changes which files a reader can trace the loop through, so neither
			// may change the verdict — measured at the commit the audit graded,
			// this disk reported exit 0 and DEGRADED.
			//
			// The remedy is the cycle row's, carried out on the file that declares
			// the closing edge. What is left behind is a store that is still
			// untidy — one misfiled namesake, one duplicate id — and no longer
			// fatal, which is the grade decision D-39 assigns each of those.
			name:   "a supersede cycle beside a namesake that claims one member's id",
			broken: true,
			remedy: classSupersedeCycle,
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "DEC-0001.json", halfWrittenCycleMember)
				writeKnowledgeRecord(t, repo, "DEC-0002.json", supersedingDecision)
				writeKnowledgeRecord(t, repo, "DEC-0009.json", namesakeOfACycleMember)
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				rewriteKnowledgeRecordForRemedy(t, repo, "DEC-0001.json", supersededDecision)
			},
		},
		{
			// A record this binary reads and rejects costs the repository that
			// record and no more (decision D-39), so it is graded the way the
			// unparseable record earlier in this group is graded rather than the
			// way the future-schema row is: exit 0 from every command, and no
			// error object anywhere. The two pipelines differ — the loader could
			// not read the unparseable one, and steps 5-11 read this one and
			// found it wrong (D-38) — and the grade is deliberately the same.
			name: "a knowledge record whose status is not one the schema defines",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				writeKnowledgeRecord(t, repo, "DEC-0001.json", invalidStatusDecision)
				return repo
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

// The records MR-002's rows are built from, spelled once for this package so the
// agreement matrix and the broken-setup matrix cannot end up describing two
// different repositories under one name.
//
// They are written out in full rather than assembled from a template because
// every field is load-bearing to some row, and two of them are the difference
// between a fatal condition and a healthy one: `status` decides whether the
// lineage has an end, `supersedes` decides whether it closes on itself, and
// supersededDecision and cyclicSupersededDecision differ in nothing else. A
// template would hide exactly the edit the cycle row's remedy performs.
const (
	// supersededDecision is the older half of an ordinary spec §93 lineage: it
	// has been replaced, it says so, and it points at nothing.
	supersededDecision = `{"schema_version":1,"kind":"decision","id":"DEC-0001",` +
		`"status":"superseded","created_at":"2026-01-01T00:00:00Z",` +
		`"title":"the first decision","decision":"Adopt the first approach."}`

	// cyclicSupersededDecision is supersededDecision with one array added: it now
	// claims to replace the record that replaced it, which closes the lineage on
	// itself. Dropping that array is the whole of the remedy the report prints.
	cyclicSupersededDecision = `{"schema_version":1,"kind":"decision","id":"DEC-0001",` +
		`"status":"superseded","created_at":"2026-01-01T00:00:00Z",` +
		`"title":"the first decision","decision":"Adopt the first approach.",` +
		`"supersedes":["DEC-0002"]}`

	// supersedingDecision is the newer half, and it is unchanged between the
	// cycle row and the healthy row beside it. Only its counterpart moves, which
	// is what makes the pair an over-fire guard rather than two unrelated
	// fixtures.
	supersedingDecision = `{"schema_version":1,"kind":"decision","id":"DEC-0002",` +
		`"status":"active","created_at":"2026-02-01T00:00:00Z",` +
		`"title":"the second decision","decision":"Adopt the second approach.",` +
		`"supersedes":["DEC-0001"]}`

	// invalidStatusDecision is inside the reader window, parses, is filed under
	// the id it carries, and still breaks decision.v1: "accepted" is not one of
	// the two values the status enum defines. It is the shape of a record written
	// by hand from memory, which is the shape steps 5-11 exist to catch.
	invalidStatusDecision = `{"schema_version":1,"kind":"decision","id":"DEC-0001",` +
		`"status":"accepted","created_at":"2026-01-01T00:00:00Z",` +
		`"title":"a decision with a status nobody defined","decision":"Adopt something."}`

	// draftReusingAnActiveId is the record that produced audit round 2's HIGH,
	// spelled here so the condition can be met at the layer a user meets it.
	//
	// It is filed at DEC-0003.json, it fails decision.v1 (status "draft" is not
	// one of the two the enum defines), it claims id DEC-0001 — which
	// supersededDecision already carries — and it claims to supersede DEC-0002.
	// Every one of those four is load-bearing: the file name keeps step 6 out of
	// the picture, the broken status is what makes it a record step 5 rejects,
	// the reused id is what let it be fused onto a valid record's node, and the
	// supersedes array is the edge that fusion turned into a loop.
	//
	// This is the shape of a half-written record left in a working tree, which is
	// why the over-fire matters: the two records beside it are correct, and a
	// pipeline that calls them a fatal supersede cycle blocks a repository over a
	// draft.
	draftReusingAnActiveId = `{"schema_version":1,"kind":"decision","id":"DEC-0001",` +
		`"status":"draft","created_at":"2026-03-01T00:00:00Z",` +
		`"title":"a draft reusing an id","decision":"Adopt a third approach.",` +
		`"supersedes":["DEC-0002"]}`

	// halfWrittenCycleMember is cyclicSupersededDecision with one property no
	// schema defines. It is still filed at the path its id names and still says
	// it supersedes DEC-0002, so the lineage it closes is exactly the same one —
	// what changed is only that step 5 now refuses the record.
	//
	// That difference is the whole of audit round 1's fail-open: a graph built
	// from step-5 survivors lost this record's edge, so one stray property turned
	// the milestone's only fatal condition off.
	halfWrittenCycleMember = `{"schema_version":1,"kind":"decision","id":"DEC-0001",` +
		`"status":"superseded","created_at":"2026-01-01T00:00:00Z",` +
		`"title":"the first decision","decision":"Adopt the first approach.",` +
		`"supersedes":["DEC-0002"],"note":"still being written"}`

	// namesakeOfACycleMember is filed at DEC-0009.json, satisfies decision.v1 in
	// full, carries DEC-0001's id and declares no "supersedes" at all.
	//
	// It is the record audit round 3's HIGH turned on. Under the rule that
	// preceded decision D-52 this file "vouched" for the id DEC-0001 — it was the
	// only claimant step 5 had accepted — which silenced the edge declared by the
	// file actually named DEC-0001.json and deleted a real cycle. Its own bytes
	// assert no supersede, so it could not have performed the remedy the report
	// would have printed either.
	namesakeOfACycleMember = `{"schema_version":1,"kind":"decision","id":"DEC-0001",` +
		`"status":"superseded","created_at":"2026-01-02T00:00:00Z",` +
		`"title":"a namesake filed under the wrong name","decision":"Adopt nothing in particular."}`
)

// linkKnowledgeRecordOutside puts a symbolic link where a record file belongs
// and points it at a perfectly valid record the repository does not contain.
//
// The target is valid on purpose. The condition under test is where the bytes
// live, not what they say: a record that is refused because it is outside the
// repository and a record that is refused because it is malformed are two
// different diagnoses with two different remedies (decision D-51), and a fixture
// whose target was also broken could not tell them apart.
func linkKnowledgeRecordOutside(t *testing.T, repo, name string) {
	t.Helper()

	outside := filepath.Join(t.TempDir(), name)
	writeFile(t, outside, []byte(supersedingDecision))

	dir := filepath.Join(knowledgeDir(repo), "decisions")
	mkdirForSetup(t, dir)
	if err := os.Symlink(outside, filepath.Join(dir, name)); err != nil {
		t.Fatalf("point %s out of the worktree: %v", name, err)
	}
}

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

// rewriteKnowledgeRecordForRemedy carries out an edit the report asked for on a
// record that is already in the store.
//
// It refuses a path that is not there rather than creating one. writeKnowledgeRecord
// creates the directory and the file, which is right for arranging a condition
// and wrong for carrying a remedy out: a row whose fixture name had drifted would
// otherwise "clear" the condition by dropping a brand-new file beside the broken
// one it was supposed to edit, and the run after it would still be broken while
// this half reported success.
func rewriteKnowledgeRecordForRemedy(t *testing.T, repo, name, content string) {
	t.Helper()

	path := filepath.Join(knowledgeDir(repo), "decisions", name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("carry out the remedy on %s: the record the report named is not there: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("carry out the remedy on %s: %v", path, err)
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
