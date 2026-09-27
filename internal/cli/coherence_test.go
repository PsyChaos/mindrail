package cli_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/doctor"
	"github.com/PsyChaos/mindrail/internal/status"
)

// initRemedy is the one command MR-001 tells a user to run. It is spelled once
// here so the invariant and the over-fire guards below cannot drift apart from
// each other or from doctor's own constant.
const initRemedy = "mindrail init"

// TestNoDocumentContradictsItsOwnErrorObject is the invariant that stops finding
// H11's class from coming back.
//
// One command produces one document. That document carries a top-level `error`
// object naming the condition that stopped the run and the remedy for it, and it
// carries a `next_action` on the report as a whole, on every component of a
// status report and on every check of a doctor report. All of them describe the
// same repository at the same instant, so they have to agree about what the
// reader should do next.
//
// They did not. On an initialised repository whose runtime root had become
// unwritable, the error object said to fix the permissions while the `runtime_db`
// component and the `sqlite` check — in the very same JSON document — said to run
// `mindrail init`, the one command that cannot succeed there. The reader who
// follows the component gets exit 4 and the same advice again.
//
// The error object is the authority, because it is the value `ok`, the exit code
// and the remedy are all derived from (finding F11). Two things it says are
// asserted, and neither is a spelling of one hard-coded string:
//
//   - Which Mindrail commands are worth running. If the error object does not
//     send the reader to `mindrail <sub>`, nothing else in the document may.
//     Written against `mindrail init` because that is the only command MR-001
//     has a remedy for, and stated over whatever commands the remedies actually
//     mention so that a later one is covered the day it is added.
//   - What to do about each path it names. "Move this aside", "change these
//     permissions", "free space here" and "remount this read-write" are four
//     mutually exclusive answers about one directory, and three of the four
//     cannot be carried out when the fourth is the true one. A document that
//     prints two of them about the same path has sent the reader to do something
//     that will not work.
//
// It deliberately says nothing about documents with no error object — a
// repository that merely has not been initialised has no authority to contradict,
// and its components are supposed to recommend init.
//
// It runs over the whole broken-setup matrix rather than a list of its own. The
// two lists were maintained separately for three audits and drifted, which is how
// finding E10's condition came to be absent from both; one matrix means a
// condition added for either invariant is checked by both.
func TestNoDocumentContradictsItsOwnErrorObject(t *testing.T) {
	for _, tc := range agreementConditions() {
		t.Run(tc.name, func(t *testing.T) {
			for _, command := range commandsUnderTest {
				t.Run(command, func(t *testing.T) {
					repo := tc.setup(t)
					assertDocumentIsCoherent(t, command, runWith(t, repo, tc.options, command, "--json").stdout)
				})
			}
		})
	}
}

// assertDocumentIsCoherent applies the invariant to one rendered envelope.
func assertDocumentIsCoherent(t *testing.T, command, stdout string) {
	t.Helper()

	var envelope struct {
		Error *app.ErrorPayload `json:"error"`
		Data  json.RawMessage   `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("%s: stdout is not a JSON envelope: %v\n%s", command, err, stdout)
	}

	if envelope.Error == nil {
		return
	}

	authority := remedy{where: "error.next_action", actions: envelope.Error.NextAction}
	for _, found := range remediesIn(t, command, envelope.Data) {
		for _, contradiction := range contradictionsBetween(authority, found) {
			t.Errorf("%s: %s\n  error object: %s => %v\n  %s: %v",
				command, contradiction,
				envelope.Error.Code, authority.actions,
				found.where, found.actions)
		}
	}
}

// contradictionsBetween is the invariant itself, as a function of two remedy
// lists so that it can be exercised against the documents that produced the
// findings it exists for rather than only against whatever the matrix happens
// to render today.
func contradictionsBetween(authority, found remedy) []string {
	return slices.Concat(
		unauthorisedCommands(authority, found),
		contradictoryPathRemedies(authority, found),
	)
}

// unauthorisedCommands is the first half: a component may not send the reader to
// a Mindrail command the error object does not.
func unauthorisedCommands(authority, found remedy) []string {
	authorised := mindrailCommandsIn(authority.actions)

	var contradictions []string
	for _, recommended := range mindrailCommandsIn(found.actions) {
		if slices.Contains(authorised, recommended) {
			continue
		}
		contradictions = append(contradictions, fmt.Sprintf(
			"%s recommends `%s`, which the error object in the same document contradicts.",
			found.where, recommended))
	}
	return contradictions
}

// contradictoryPathRemedies is the second half, and it is the one that
// generalises past `mindrail init`.
//
// Every remedy this binary prints about a path belongs to one of five classes,
// and the classes are exclusive by construction: a filesystem with no room left
// has correct mode bits, a read-only mount refuses the chmod with the same errno
// it refused the write with, and an entry standing in the way is cleared by
// neither. So two remedies naming one path in two classes cannot both be right,
// and the reader who picks the wrong one is sent to do something that fails.
func contradictoryPathRemedies(authority, found remedy) []string {
	settled := pathRemedyClasses(authority.actions)

	var contradictions []string
	for _, path := range slices.Sorted(maps.Keys(pathRemedyClasses(found.actions))) {
		class := pathRemedyClasses(found.actions)[path]
		want, named := settled[path]
		if !named || want == class {
			continue
		}
		contradictions = append(contradictions, fmt.Sprintf(
			"%s says to %s %q while the error object in the same document says to %s it.",
			found.where, class, path, want))
	}
	return contradictions
}

// TestTheCoherenceInvariantReportsTheDocumentsItWasWrittenFor is the invariant's
// own over-fire and under-fire guard.
//
// An invariant that passes because it recognises nothing is worse than no
// invariant: it reports a green suite over exactly the documents it was written
// to reject. So it is run against the two rendered documents that produced
// findings H11 and E2 — reconstructed from the reports themselves — and against
// the coherent shapes it must leave alone.
func TestTheCoherenceInvariantReportsTheDocumentsItWasWrittenFor(t *testing.T) {
	tests := []struct {
		name      string
		authority remedy
		found     remedy
		wantFlag  bool
	}{
		{
			// Finding H11 as it was rendered: the error object said to fix the
			// permissions and the component beside it said to run the one
			// command that cannot succeed there.
			name:      "a component offers an init the error object does not",
			authority: remedy{where: "error.next_action", actions: []string{`check the permissions on "/repo/.git/mindrail"`}},
			found:     remedy{where: "data.components.runtime_db.next_action", actions: []string{"mindrail init"}},
			wantFlag:  true,
		},
		{
			// Finding E2: a chmod prescribed for a filesystem with zero bytes
			// free, beside the sentence that names the real condition.
			name:      "a check prescribes a chmod for a path the error object says is full",
			authority: remedy{where: "error.next_action", actions: []string{`free space on the filesystem holding "/repo/.git"`}},
			found:     remedy{where: "data.checks[].next_action", actions: []string{`check the permissions on "/repo/.git"`}},
			wantFlag:  true,
		},
		{
			// Finding E4: a read-only mount answered with a chmod that fails
			// with the same EROFS that refused the write.
			name:      "a check prescribes a chmod for a read-only mount",
			authority: remedy{where: "error.next_action", actions: []string{`remount the filesystem holding "/repo/.git" read-write, or move this repository to a writable location`}},
			found:     remedy{where: "data.checks[].next_action", actions: []string{`Make /repo/.git writable.`}},
			wantFlag:  true,
		},
		{
			name:      "an obstruction answered with a chmod",
			authority: remedy{where: "error.next_action", actions: []string{`remove or move aside "/repo/.mindrail"`}},
			found:     remedy{where: "data.checks[].next_action", actions: []string{`check the permissions on "/repo/.mindrail"`}},
			wantFlag:  true,
		},
		{
			// The over-fire guards. A component that repeats the error object's
			// own remedy, one that names a different path, one that offers the
			// same init the error object already offers, and one whose sentence
			// this table does not recognise are all coherent.
			name:      "a component repeating the error object's remedy",
			authority: remedy{where: "error.next_action", actions: []string{`check the permissions on "/repo/.git"`}},
			found:     remedy{where: "data.checks[].next_action", actions: []string{`check the permissions on "/repo/.git"`}},
		},
		{
			name:      "a component naming a different path",
			authority: remedy{where: "error.next_action", actions: []string{`check the permissions on "/repo/.git"`}},
			found:     remedy{where: "data.checks[].next_action", actions: []string{`remove or move aside "/repo/.mindrail"`}},
		},
		{
			name:      "a component offering the init the error object also offers",
			authority: remedy{where: "error.next_action", actions: []string{`remove or move aside "/repo/.git/mindrail", then run ` + "`mindrail init`" + `.`}},
			found:     remedy{where: "data.components.runtime_db.next_action", actions: []string{"mindrail init"}},
		},
		{
			name:      "a sentence that prescribes nothing about a path",
			authority: remedy{where: "error.next_action", actions: []string{`check the permissions on "/repo/.git"`}},
			found:     remedy{where: "data.checks[].next_action", actions: []string{"Resolve the failure reported by the git check, then read this report again."}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := contradictionsBetween(tc.authority, tc.found)
			if tc.wantFlag && len(got) == 0 {
				t.Fatalf("the invariant reported nothing about a document it exists to reject:\n  %v\n  %v",
					tc.authority.actions, tc.found.actions)
			}
			if !tc.wantFlag && len(got) != 0 {
				t.Fatalf("the invariant reported %v about a coherent document:\n  %v\n  %v",
					got, tc.authority.actions, tc.found.actions)
			}
		})
	}
}

// remedyClass names what a remedy sentence tells the reader to do about a path.
// The strings are the phrasing a failure message uses, so a report of a
// contradiction reads as one sentence.
type remedyClass string

const (
	classUnknown     remedyClass = ""
	classObstruction remedyClass = "move aside what is in the way of"
	classRelink      remedyClass = "repoint the link at"
	classPermission  remedyClass = "change the permissions on"
	classSpace       remedyClass = "free space on"
	classReadOnly    remedyClass = "remount"

	// The five below were a second, private table owned by the agreement
	// matrix, which is how the invariant on this side came to be blind to them:
	// the matrix could tell a rebuild from an upgrade while
	// contradictoryPathRemedies could not tell either from nothing. A remedy
	// this table does not recognise is not read as agreeing — it asserts
	// nothing — so a whole vocabulary sitting outside it is a whole vocabulary
	// the path comparison cannot see.
	classRebuild    remedyClass = "move the runtime database aside and run mindrail init"
	classEditConfig remedyClass = "correct the entry the report named"
	classContain    remedyClass = "put the path back inside the repository"
	classUpgrade    remedyClass = "upgrade to a binary that reads these records"
	classEnterable  remedyClass = "make sure the path exists and can be entered"

	// MR-002's one fatal knowledge finding (decision D-39). It is declared here,
	// beside the ten above, rather than anywhere the knowledge rows could keep a
	// vocabulary of their own: the five classes in the group above spent three
	// audits in a private table owned by the agreement matrix, and the whole
	// point of merging them was that a second table is how a real remedy becomes
	// invisible to the invariant that reads them.
	classSupersedeCycle remedyClass = "break the supersede cycle recorded in"
)

// remedyPhrases maps the sentences this binary actually prints onto the class
// each one belongs to, most specific first.
//
// It is a table of the real phrasings rather than a parser: every one of them is
// written in this repository, by filesystem.unwritableSubjectError,
// storage.Presence, storage.WriteAccess or a doctor check, and a phrase nobody
// prints classifies as nothing and asserts nothing. That is the honest failure
// mode — a remedy this table does not recognise is not read as agreeing, it is
// read as unclassified — and it is why a new sentence has to be added here
// rather than silently passing.
var remedyPhrases = []struct {
	match func(lowered string) bool
	class remedyClass
}{
	{containing("repoint the link"), classRelink},
	{containing("remount the filesystem"), classReadOnly},
	{containing("free space on"), classSpace},
	// Before "move aside": "Move <db> aside and run `mindrail init` to rebuild
	// it" is a rebuild, not an obstruction, and the two have different remedies.
	// "Remove or move aside the directory at <db>, then run `mindrail init`."
	// keeps the obstruction class it declares, because removing the directory is
	// the action and the init is the consequence.
	{containing("to rebuild it"), classRebuild},
	{containing("move aside"), classObstruction},
	{containing("restore write permission on"), classPermission},
	{containing("check the permissions on"), classPermission},
	// "Make <path> writable.", "usable." and "readable." are the doctor checks'
	// own phrasing, and they are matched as that shape rather than as the bare
	// substring "make ".
	//
	// "make " on its own is a word, not a sentence. It classified "Delete the
	// whole repository to make it stop" as a permission remedy, which meant the
	// seven agreement-matrix rows declaring classPermission were satisfied by
	// any sentence at all that happened to contain it — the check those rows
	// exist to perform, defeated by an English verb.
	{makesSomethingUsable, classPermission},
	{containing("fix or remove the offending entry"), classEditConfig},
	{containing("upgrade mindrail"), classUpgrade},
	{containing("use a path inside the repository"), classContain},
	{containing(`for a ".." segment`), classContain},
	{containing("is a directory you can enter"), classEnterable},
	// doctor.findingRemedyByStep[validate.StepSupersedeCycle]. Its position is
	// free rather than chosen, and that was checked rather than assumed, in both
	// directions. Nothing above it reads the sentence it matches: the nearest
	// candidate, makesSomethingUsable, requires the sentence to open with
	// "make ", and this one opens with "break ". And it reads nothing above it
	// matches: the substring is the whole clause "break the supersede cycle", so
	// the two other remedies in the binary that mention superseding at all —
	// "Add the record %s supersedes, or drop that id from its supersedes list."
	// and `Set status "superseded" on %s ...` — are outside it. The whole package
	// was run with this entry moved to the front of the table as well as at the
	// end, green both times, which is what "does not shadow and is not shadowed"
	// means operationally.
	//
	// The other six sentences findingRemedyByStep prints are deliberately absent.
	// They belong to KNOWLEDGE_INVALID, which is DEGRADED at exit 0 and therefore
	// never reaches an error object, so no row in the matrix can hold the binary
	// to a class for them: a class added here for one of those would be a claim
	// nothing could falsify. TestTheRemedyClassifierRefusesASentenceThatMerelyContainsAVerb
	// states that absence as a fact instead of leaving it to be inferred.
	{containing("break the supersede cycle"), classSupersedeCycle},
}

func containing(phrase string) func(string) bool {
	return func(lowered string) bool { return strings.Contains(lowered, phrase) }
}

// makesSomethingUsable recognises the doctor checks' "Make <path> writable."
// and "Make <path> usable." — an instruction that opens with the verb and says
// what state the path has to end in.
func makesSomethingUsable(lowered string) bool {
	if !strings.HasPrefix(lowered, "make ") {
		return false
	}
	for _, state := range []string{" writable", " usable", " readable"} {
		if strings.Contains(lowered, state) {
			return true
		}
	}
	return false
}

// classOf reads one action sentence.
func classOf(action string) remedyClass {
	lowered := strings.ToLower(action)
	for _, candidate := range remedyPhrases {
		if candidate.match(lowered) {
			return candidate.class
		}
	}
	return classUnknown
}

// TestTheRemedyClassifierRefusesASentenceThatMerelyContainsAVerb is the
// classifier's own guard.
//
// Every assertion built on classOf — the coherence invariant's path comparison
// and the agreement matrix's per-row remedy class — is worth exactly what this
// function's precision is worth, and it was worth very little: seven matrix rows
// asserting classPermission were satisfied by any sentence containing "make ".
// The rows here are the real phrasings, and the ones beside them are what a
// wrong remedy looks like.
func TestTheRemedyClassifierRefusesASentenceThatMerelyContainsAVerb(t *testing.T) {
	tests := []struct {
		action string
		want   remedyClass
	}{
		{`Make "/repo/.git/mindrail" writable.`, classPermission},
		{`Make "/repo/.mindrail/knowledge" usable.`, classPermission},
		{`check the permissions on "/repo/.mindrail"`, classPermission},
		{`restore write permission on /repo/.git/mindrail/mindrail.db`, classPermission},
		{`free space on the filesystem holding "/repo/.git"`, classSpace},
		{`remove or move aside "/repo/.mindrail"`, classObstruction},
		{`remove or repoint the link at "/repo/.mindrail"`, classRelink},
		{`remount the filesystem holding /repo read-write`, classReadOnly},
		{`Break the supersede cycle by dropping the superseded id from .mindrail/knowledge/decisions/DEC-0001.json.`, classSupersedeCycle},

		// The six remedies MR-002 prints for a KNOWLEDGE_INVALID record classify
		// as nothing, and that is the intended reading rather than an oversight.
		// A DEGRADED knowledge check produces no error object, so the agreement
		// matrix cannot assert a class for any of them and nothing would notice a
		// wrong one. This row is where that decision is recorded: a later wave
		// that gives one of these sentences a class has to change this line, and
		// changing it is the moment to say what now holds the class honest.
		{`Correct .mindrail/knowledge/decisions/DEC-0001.json so it satisfies the knowledge schema document for its kind.`, classUnknown},

		// Decision D-51's two remedies, for the same reason and with the same
		// consequence if that reason stops holding. A declined record is reported
		// by a DEGRADED knowledge reading at exit 0, so no error object carries
		// these sentences and no agreement-matrix row can hold the binary to a
		// class for them.
		//
		// They are spelled out here rather than left unmentioned because they are
		// the two remedies in this binary that talk about a link without being
		// classRelink: "repoint the link at <path>" is a dangling link to point
		// somewhere real, and these are a link that resolves perfectly to
		// something this repository does not own. A future entry matching a bare
		// "the link" would collapse the two conditions into one class, and the row
		// that notices is this one.
		{`Replace the link at .mindrail/knowledge/decisions/DEC-0002.json with the record itself, or remove it.`, classUnknown},
		{`Replace or remove the remaining 3 links under .mindrail/knowledge that resolve outside the repository root.`, classUnknown},

		// None of these tells the reader to change a permission, and none of
		// them may be read as if it did.
		{"Delete the whole repository to make it stop", classUnknown},
		{"Reinstall Mindrail from your package manager", classUnknown},
		{"Ask your administrator to make a decision", classUnknown},
		{"Run `mindrail init` to make progress", classUnknown},
		{"", classUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			if got := classOf(tc.action); got != tc.want {
				t.Errorf("classOf(%q) = %q, want %q", tc.action, got, tc.want)
			}
		})
	}
}

// pathRemedyClasses maps every absolute path a remedy names onto what that
// remedy says to do about it.
//
// A sentence that names two paths files the same class under both, which is
// correct: "remount the filesystem holding X read-write, or move this repository
// to a writable location" is one instruction about one filesystem however many
// paths it spells.
func pathRemedyClasses(actions []string) map[string]remedyClass {
	classes := make(map[string]remedyClass)
	for _, action := range actions {
		class := classOf(action)
		if class == classUnknown {
			continue
		}
		for _, path := range absolutePathsIn(action) {
			// The first class named for a path wins, so a document is reported
			// against the remedy its error object printed first rather than
			// against whichever happened to be scanned last.
			if _, seen := classes[path]; !seen {
				classes[path] = class
			}
		}
	}
	return classes
}

// absolutePathsIn pulls the absolute paths out of one remedy sentence.
//
// Paths are printed quoted by the filesystem layer and bare by the storage and
// doctor layers, so both spellings are read, and the trailing punctuation a
// sentence ends with is trimmed off: "/x/y." and "/x/y" are one path.
func absolutePathsIn(action string) []string {
	found := make([]string, 0, 2)
	for _, field := range strings.Fields(action) {
		trimmed := strings.Trim(field, `"'`+"`.,;:")
		if strings.HasPrefix(trimmed, "/") && len(trimmed) > 1 {
			found = append(found, filepath.Clean(trimmed))
		}
	}
	return found
}

// mindrailCommandsIn lists the `mindrail <sub>` commands a remedy sends the
// reader to, deduplicated and sorted so a failure message is stable.
func mindrailCommandsIn(actions []string) []string {
	found := make([]string, 0, 2)
	for _, action := range actions {
		rest := action
		for {
			index := strings.Index(rest, mindrailBinary+" ")
			if index < 0 {
				break
			}
			rest = rest[index+len(mindrailBinary)+1:]
			sub := strings.Trim(strings.Fields(rest + " ")[0], "`.,;:'\"")
			if sub == "" {
				continue
			}
			command := mindrailBinary + " " + sub
			if !slices.Contains(found, command) {
				found = append(found, command)
			}
		}
	}
	slices.Sort(found)
	return found
}

// mindrailBinary is the name a remedy spells when it sends the reader back to
// this program.
const mindrailBinary = "mindrail"

// remedy is one next_action list found in a rendered document, with the JSON
// path it was found at so a failure names the field rather than the value.
type remedy struct {
	where   string
	actions []string
}

// remediesIn collects every next_action in a command payload.
//
// It walks the decoded JSON rather than the Go types on purpose. The three
// payloads have three shapes, and a field added by a later change is covered the
// moment it exists: an invariant that had to be taught about each new carrier
// would go stale exactly when a new carrier introduced a new contradiction.
func remediesIn(t *testing.T, command string, data json.RawMessage) []remedy {
	t.Helper()

	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("%s: data is not JSON: %v", command, err)
	}

	var found []remedy
	var walk func(path string, node any)
	walk = func(path string, node any) {
		switch value := node.(type) {
		case map[string]any:
			for _, key := range slices.Sorted(maps.Keys(value)) {
				child := join(path, key)
				if key == "next_action" {
					if actions, ok := stringsOf(value[key]); ok {
						found = append(found, remedy{where: child, actions: actions})
						continue
					}
				}
				walk(child, value[key])
			}
		case []any:
			for _, element := range value {
				walk(path+"[]", element)
			}
		}
	}
	walk("data", document)

	sort.Slice(found, func(i, j int) bool { return found[i].where < found[j].where })
	return found
}

// TestEveryUninitialisedSpellingExitsZero is finding H14.
//
// An interrupted first `mindrail init` can leave a repository in three shapes:
// nothing under the runtime root, a mindrail.db that was created and never
// written to, and a database that was created and not yet migrated. They are
// one state — the repository is not initialised — with one remedy, and decision
// D-03 puts that state at exit 0 with BLOCKED readiness.
//
// Two of the three were reported that way. The middle one was opened, found to
// be in rollback-journal mode because nothing had ever converted it, and
// reported as a fatal RUNTIME_DB_UNAVAILABLE at exit 4 with ok:false — a driver
// fact about a file nobody had finished creating, promoted to a failure. The
// rows are run together rather than separately because the defect is the
// disagreement between them, not any one row's value.
func TestEveryUninitialisedSpellingExitsZero(t *testing.T) {
	setups := map[string]func(t *testing.T) string{
		"nothing under the runtime root": newRepo,
		"a database file that was created and never written": func(t *testing.T) string {
			repo := newRepo(t)
			runtimeRoot := filepath.Join(repo, ".git", "mindrail")
			if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
				t.Fatalf("create %s: %v", runtimeRoot, err)
			}
			writeFile(t, runtimeDBPath(t, repo), nil)
			return repo
		},
		"a database created and not yet migrated": func(t *testing.T) string {
			repo := newRepo(t)
			createUnmigratedDatabase(t, repo)
			return repo
		},
	}

	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			repo := setup(t)

			got := run(t, repo, "status", "--json")
			got.requireExit(t, app.ExitSuccess)
			assertNoErrorEnvelope(t, "status", got.stdout)

			var report status.Report
			decodeData(t, got.stdout, &report)

			if report.Readiness != status.ReadinessBlocked {
				t.Errorf("readiness = %q, want %q", report.Readiness, status.ReadinessBlocked)
			}
			component := report.Components[status.ComponentRuntimeDB]
			if component.Code != app.CodeWorkspaceNotInitialized {
				t.Errorf("runtime_db code = %q, want %q: this is one state with one spelling",
					component.Code, app.CodeWorkspaceNotInitialized)
			}
			if !slices.Contains(component.NextAction, initRemedy) {
				t.Errorf("runtime_db does not offer `%s`: %v", initRemedy, component.NextAction)
			}

			// The remedy the report prints is the one that has to work.
			if initOut := run(t, repo, "init"); initOut.code != app.ExitSuccess {
				t.Fatalf("`%s` exited %d: %v\n%s", initRemedy, initOut.code, initOut.err, initOut.stdout)
			}
			run(t, repo, "status", "--json").requireExit(t, app.ExitSuccess)
		})
	}
}

// TestAPopulatedNonWALDatabaseIsNotCalledUninitialised is the over-fire guard
// for finding H14, and the adjacent condition the new answer must not swallow.
//
// A mindrail.db that holds data but was never converted to WAL is not an
// unfinished first run: something else wrote it, Mindrail will not read it as
// it stands, and reporting it as "not initialised, exit 0" would hide a real
// problem behind a state a consumer is told to ignore. Only the zero-length
// file — the one that holds nothing at all — is the state.
func TestAPopulatedNonWALDatabaseIsNotCalledUninitialised(t *testing.T) {
	repo := newRepo(t)
	runtimeRoot := filepath.Join(repo, ".git", "mindrail")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatalf("create %s: %v", runtimeRoot, err)
	}

	path := runtimeDBPath(t, repo)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open %s with database/sql: %v", path, err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE seeded (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed a rollback-journal database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close the seed database: %v", err)
	}

	got := run(t, repo, "status", "--json")
	if got.code == app.ExitSuccess {
		t.Fatalf("status exited 0 on a database Mindrail refuses to read:\n%s", got.stdout)
	}

	var report status.Report
	decodeData(t, got.stdout, &report)

	component := report.Components[status.ComponentRuntimeDB]
	if component.Code == app.CodeWorkspaceNotInitialized {
		t.Errorf("runtime_db code = %q, which calls somebody else's database an unfinished first run",
			component.Code)
	}
	if component.State != doctor.StateError {
		t.Errorf("runtime_db state = %q, want %q", component.State, doctor.StateError)
	}
}

// TestHealthyRepositoryIsUntouchedByRemedyCoherence is the first over-fire guard
// for finding H11's fix.
//
// The fix reaches into every reading of the runtime store and can replace its
// remedy. On a repository where nothing is wrong there is nothing to replace,
// and a report that started naming an obstruction here would be describing a
// failure that does not exist.
func TestHealthyRepositoryIsUntouchedByRemedyCoherence(t *testing.T) {
	repo := newInitializedRepo(t)

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)
	assertNoErrorEnvelope(t, "status", got.stdout)

	var report status.Report
	decodeData(t, got.stdout, &report)

	if report.Readiness != status.ReadinessPartialReady {
		t.Fatalf("readiness = %q, want %q while a healthy repository's syntax is at INVENTORY", report.Readiness, status.ReadinessPartialReady)
	}
	for name, component := range report.Components {
		if component.State == doctor.StateError || component.State == doctor.StateUnavailable {
			t.Errorf("component %q is %s on a healthy repository: %+v", name, component.State, component)
		}
		if len(component.NextAction) != 0 {
			t.Errorf("component %q offers a remedy on a healthy repository: %v", name, component.NextAction)
		}
	}

	doctorOut := run(t, repo, "doctor", "--json")
	doctorOut.requireExit(t, app.ExitSuccess)

	var health doctor.Report
	decodeData(t, doctorOut.stdout, &health)
	for _, check := range health.Checks {
		if check.State != doctor.StateOK {
			t.Errorf("check %q is %s on a healthy repository: %+v", check.Name, check.State, check)
		}
	}
}

// TestAnOccupiedCacheDirectoryIsNotAnObstruction is the second over-fire guard,
// and it is the adjacent condition the fix must not swallow.
//
// A runtime root nothing can write makes `mindrail init` impossible, so a
// reading that recommends init there is replaced by the obstruction. A cache
// directory nothing can write makes nothing impossible: MR-001 stores nothing in
// it, init still succeeds, and the repository is still usable. Grading the two
// alike is precisely how a fully working repository was once driven to BLOCKED
// and exit 4 (finding F13), so this asserts the opposite in both directions —
// the remedy is still `mindrail init`, and running it still works.
func TestAnOccupiedCacheDirectoryIsNotAnObstruction(t *testing.T) {
	repo := newRepo(t)
	runtimeRoot := filepath.Join(repo, ".git", "mindrail")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatalf("create %s: %v", runtimeRoot, err)
	}
	writeFile(t, filepath.Join(runtimeRoot, "cache"), []byte("not a directory"))

	got := run(t, repo, "status", "--json")
	got.requireExit(t, app.ExitSuccess)
	assertNoErrorEnvelope(t, "status", got.stdout)

	var report status.Report
	decodeData(t, got.stdout, &report)

	component, present := report.Components[status.ComponentRuntimeDB]
	if !present {
		t.Fatalf("status report has no runtime_db component: %v", report.Components)
	}
	if component.Code != app.CodeWorkspaceNotInitialized {
		t.Errorf("runtime_db code = %q, want %q: an occupied cache directory is not an obstruction",
			component.Code, app.CodeWorkspaceNotInitialized)
	}
	if !slices.Contains(component.NextAction, initRemedy) {
		t.Fatalf("runtime_db no longer offers `%s` on a repository init would fix: %v",
			initRemedy, component.NextAction)
	}

	if initOut := run(t, repo, "init"); initOut.code != app.ExitSuccess {
		t.Fatalf("the remedy the report printed exited %d: %v\n%s",
			initOut.code, initOut.err, initOut.stdout)
	}
}

// stringsOf converts a decoded JSON array of strings. A next_action that is not
// one is a shape defect the JSON contract tests catch; here it is simply not a
// remedy this invariant can read.
func stringsOf(node any) ([]string, bool) {
	elements, ok := node.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(elements))
	for _, element := range elements {
		text, ok := element.(string)
		if !ok {
			return nil, false
		}
		out = append(out, text)
	}
	return out, true
}
