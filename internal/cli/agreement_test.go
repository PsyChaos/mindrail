package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
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
	// broken says whether every command should report a failure. The over-fire
	// rows set it false and then assert that none of them reports one.
	broken bool
	// clear carries out the remedy on disk. Nil where there is nothing to undo.
	clear func(t *testing.T, repo string)
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
func TestOneConditionIsNamedTheSameWayByEveryCommand(t *testing.T) {
	for _, tc := range agreementConditions() {
		t.Run(tc.name, func(t *testing.T) {
			answers := make(map[string]answer, len(commandsUnderTest))
			for _, command := range commandsUnderTest {
				repo := tc.setup(t)
				answers[command] = answerOf(t, repo, command)
			}

			first := commandsUnderTest[0]
			for _, command := range commandsUnderTest[1:] {
				assertSameAnswer(t, tc.name, first, answers[first], command, answers[command])
			}

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

			// The sentence all three printed, carried out.
			repo := tc.setup(t)
			tc.clear(t, repo)
			for _, command := range commandsUnderTest {
				if got := run(t, repo, command); got.code != app.ExitSuccess {
					t.Fatalf("%s: after carrying out the remedy %v, `%s` exited %d: %v\n%s",
						tc.name, answers[first].nextAction, command, got.code, got.err, got.stdout)
				}
			}
		})
	}
}

// agreementConditions is the matrix.
//
// The first three are conditions in the working tree, which is where the two
// disagreeing answers came from: the config loader reads that directory and
// `init` writes it, and until pass 6 they described it differently. The last
// three are the over-fire guards — a healthy repository, one that has simply not
// been initialised, and the adjacent condition that must not be read as an
// obstruction: a repository config directory that refuses writes but has nothing
// left to receive, where `mindrail init` exits 0.
func agreementConditions() []condition {
	repoConfigDir := func(repo string) string { return filepath.Join(repo, ".mindrail") }

	return []condition{
		{
			name:   "a regular file where the repository config directory belongs",
			broken: true,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				writeFile(t, repoConfigDir(repo), []byte("not a directory"))
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				if err := os.Remove(repoConfigDir(repo)); err != nil {
					t.Fatalf("carry out the remedy: %v", err)
				}
			},
		},
		{
			name:   "a worktree that will not accept the repository config directory",
			broken: true,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				denyWrites(t, repo)
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				if err := os.Chmod(repo, 0o700); err != nil {
					t.Fatalf("carry out the remedy: %v", err)
				}
			},
		},
		{
			name:   "a dangling symlink where the repository config directory belongs",
			broken: true,
			setup: func(t *testing.T) string {
				repo := newRepo(t)
				if err := os.Symlink(filepath.Join(repo, "gone"), repoConfigDir(repo)); err != nil {
					t.Fatalf("create a dangling symlink: %v", err)
				}
				return repo
			},
			clear: func(t *testing.T, repo string) {
				t.Helper()
				if err := os.Remove(repoConfigDir(repo)); err != nil {
					t.Fatalf("carry out the remedy: %v", err)
				}
			},
		},
		{name: "a healthy initialised repository", setup: newInitializedRepo},
		{name: "a repository that has never been initialised", setup: newRepo},
		{
			name: "a repository config directory with nothing left to receive",
			setup: func(t *testing.T) string {
				repo := newInitializedRepo(t)
				denyWrites(t, repoConfigDir(repo))
				return repo
			},
		},
	}
}

// answerOf runs one command in JSON mode and reduces its envelope.
func answerOf(t *testing.T, repo, command string) answer {
	t.Helper()

	got := run(t, repo, command, "--json")

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
