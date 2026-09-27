package validate_test

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// This file closes round 4's third coverage gap, which it stated as: "everything
// was measured on ext4 on Linux. isCanonical compares a file name against a JSON
// id byte for byte; on a case-insensitive or Unicode-normalising filesystem that
// comparison is *argued* unreachable, not measured."
//
// The argument being replaced is worth spelling out, because what can be
// measured here is narrower than the gap's wording and pretending otherwise
// would be the same overstatement round 4 spent eight findings on.
//
// A filesystem changes two things and only two. It decides which names can
// coexist in one directory, and it decides which bytes `os.ReadDir` hands back
// for a name that was written. It does not reach into decision D-52, which is a
// comparison between a directory entry and a JSON property, and the only place
// that comparison could be wrong is if some name a real filesystem can produce
// were treated as owning an id it does not spell.
//
// So the two tests below measure exactly that pair:
//
//   - Every spelling a folding or normalising filesystem can hand back for
//     "DEC-0001.json" — the case-folded ones, the decomposed one, the ones
//     Windows trims — owns nothing, and the record filed under it cannot close a
//     lineage. This is a pure-function test, and it is the whole of what D-52
//     does with a file name.
//   - The loader reports the name the directory actually holds, and the
//     ownership verdict follows that name rather than the id in the document.
//     That one runs against a real filesystem and asserts the same property on a
//     case-sensitive and a case-insensitive one, so it is meaningful when this
//     suite is finally run on macOS or Windows rather than skipped there.
//
// What is still not measured: an actual case-insensitive or normalising mount.
// Nothing in this repository can create one without root, and the project's
// advertised matrix is still exercised on Linux only.

// TestOnlyAByteIdenticalFileNameOwnsAnId is decision D-52 against every spelling
// of one id a real filesystem can produce.
//
// Each row files a record that declares DEC-0001 and supersedes DEC-0002, beside
// a correct DEC-0002.json that supersedes DEC-0001. The pair is one edge short of
// the milestone's only fatal finding, and the edge that is missing is the one the
// oddly-named file would supply if its name owned the id. The control row uses
// the exact name and reports the cycle, which is what makes the other rows a
// measurement rather than a store with nothing in it.
func TestOnlyAByteIdenticalFileNameOwnsAnId(t *testing.T) {
	validator := shippedValidator(t)

	for _, tt := range []struct {
		name string
		why  string
		owns bool
	}{
		{name: "DEC-0001.json", why: "the name the id spells", owns: true},
		{name: "dec-0001.json", why: "a case-folding filesystem's spelling", owns: false},
		{name: "Dec-0001.json", why: "one letter folded", owns: false},
		{name: "DEC-0001.JSON", why: "the extension folded", owns: false},
		{name: "DEC-0001́.json", why: "a combining accent, as a normalising filesystem decomposes", owns: false},
		{name: "DEC-0001 .json", why: "a trailing space, which Windows trims on creation", owns: false},
		{name: "DEC-0001.json.", why: "a trailing dot, which Windows also trims", owns: false},
	} {
		t.Run(tt.why, func(t *testing.T) {
			store := storeOf(t, []testRecord{
				decisionAt(tt.name, decisionDoc("DEC-0001", map[string]any{
					"status": "superseded", "supersedes": []any{"DEC-0002"},
				})),
				decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
					"status": "superseded", "supersedes": []any{"DEC-0001"},
				})),
			})

			findings := validate.Check(store, validator)
			claimant := ".mindrail/knowledge/decisions/" + tt.name
			cycle := pathsAt(findings, validate.StepSupersedeCycle)

			if tt.owns {
				want := []string{claimant, ".mindrail/knowledge/decisions/DEC-0002.json"}
				slices.Sort(want)
				if !slices.Equal(cycle, want) {
					t.Fatalf("step 9 reported %v, want the closed pair %v\n\t%s", cycle, want, describe(findings))
				}
				if got := stepsAt(findings, claimant); slices.Contains(got, validate.StepFilenameConsistency) {
					t.Errorf("%s reported step 6, but its name is the one its id spells", claimant)
				}
				return
			}

			if len(cycle) != 0 {
				t.Errorf("step 9 reported %v; %q does not spell DEC-0001, so it supplies no edge for it\n\t%s",
					cycle, tt.name, describe(findings))
			}
			if got := stepsAt(findings, claimant); !slices.Contains(got, validate.StepFilenameConsistency) {
				t.Errorf("%s reported steps %v; a file name that disagrees with the id is step 6's condition and the reader has to be told\n\t%s",
					claimant, got, describe(findings))
			}
		})
	}
}

// TestOwnershipFollowsTheNameTheDirectoryHolds runs the same rule against a real
// filesystem, and is written so that it says something on every filesystem
// rather than only on a case-sensitive one.
//
// It writes one record under a case-folded name and then asks the directory what
// it is called. On ext4 the answer is the folded name, the record owns nothing,
// and step 6 says so. On APFS or NTFS the answer may be a name this test did not
// write — that is the whole hazard the gap named — and the assertion is the same
// either way: the loader carries the listed name, and the ownership verdict is
// the one that name implies.
func TestOwnershipFollowsTheNameTheDirectoryHolds(t *testing.T) {
	worktree := t.TempDir()
	writeKnowledgeRecord(t, worktree, "decisions", "dec-0001.json",
		decisionDoc("DEC-0001", nil))

	dir := filepath.Join(worktree, ".mindrail", "knowledge", "decisions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	if len(entries) != 1 {
		t.Fatalf("the directory holds %d entries after one write, want 1", len(entries))
	}
	listed := entries[0].Name()
	t.Logf("the filesystem under %s lists the written name %q as %q", os.TempDir(), "dec-0001.json", listed)

	store := loadStore(t, worktree)
	if len(store.Decisions) != 1 {
		t.Fatalf("the loader read %d decisions, want 1", len(store.Decisions))
	}
	if got := path.Base(store.Decisions[0].Path); got != listed {
		t.Fatalf("the loader carries the record as %q, but the directory lists it as %q; "+
			"decision D-52 compares the id against whichever of the two this is", got, listed)
	}

	findings := validate.Check(store, shippedValidator(t))
	reported := slices.Contains(
		stepsAt(findings, ".mindrail/knowledge/decisions/"+listed),
		validate.StepFilenameConsistency)
	switch wantReported := listed != "DEC-0001.json"; {
	case wantReported && !reported:
		t.Errorf("the record is listed as %q and declares DEC-0001, and step 6 said nothing:\n\t%s",
			listed, describe(findings))
	case !wantReported && reported:
		t.Errorf("the filesystem listed the record as %q, which is the name its id spells, and step 6 reported it anyway:\n\t%s",
			listed, describe(findings))
	}
}

// TestAtMostOneRecordCanBeCanonicalForAnId is the half of the gap that is about
// which names can coexist rather than about which bytes come back.
//
// Decision D-52 rests on "file names are unique inside a directory, so at most
// one record in a store can be canonical for any (kind, id)". On a case-folding
// filesystem the two names below are one file and the second write overwrites
// the first; on a case-sensitive one they are two files and only one of them is
// canonical. The premise holds in both worlds and this is what measures it,
// rather than the sentence in newGraph's comment asserting it.
func TestAtMostOneRecordCanBeCanonicalForAnId(t *testing.T) {
	worktree := t.TempDir()
	for _, name := range []string{"DEC-0001.json", "dec-0001.json"} {
		writeKnowledgeRecord(t, worktree, "decisions", name, decisionDoc("DEC-0001", nil))
	}

	store := loadStore(t, worktree)
	canonical := make([]string, 0, len(store.Decisions))
	for _, ref := range store.Decisions {
		if path.Base(ref.Path) == ref.ID+".json" {
			canonical = append(canonical, ref.Path)
		}
	}

	if len(canonical) > 1 {
		t.Fatalf("%d records are canonical for DEC-0001: %v; D-52 assumes at most one", len(canonical), canonical)
	}
	t.Logf("the filesystem kept %d of the two names and %d of them is canonical for DEC-0001",
		len(store.Decisions), len(canonical))
}
