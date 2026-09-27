package validate_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// This file closes the coverage gap round 4 named first among the things it did
// not look at: "no store mixing decisions and invariants was enumerated by
// anyone", and "no test in the repository asserts step 9 over an invariant at
// all".
//
// The second half is now false twice over — the acceptance sweep runs both kinds
// (see ownership_sweep_test.go) and the fatal message is read below over an
// invariant group. This file is about the first half: what happens when both
// kinds are in one store.
//
// The property under test is that nothing happens. Step 9's answer for a mixed
// store is exactly the two answers its halves would get alone, because a
// lineage node is a (kind, id) pair and no edge crosses kinds. That is stated in
// three places — node's doc comment, newGraph's `node{kind: key.kind, ...}`, and
// both shipped documents pinning "supersedes" items to their own kind's pattern
// — and until now it was measured in one small fixture.

// TestStepNineOverAMixedStoreIsWhatEachKindWouldGetAlone pairs every store in
// the decision space with an unrelated store from the invariant space.
//
// The pairing is a permutation rather than a nested loop: 4,864 x 4,864 is 23
// million stores and would not finish, while pairing store i with store
// (i*1237 + 911) mod 4864 visits every decision store once and every invariant
// store once. The stride is prime and coprime with 4,864, so the map is a
// bijection; 911 keeps it off the diagonal, because pairing a store with its own
// mirror image is the one pairing that would hide a defect which fused the two
// kinds — two halves of identical shape produce the same finding set fused or
// not.
func TestStepNineOverAMixedStoreIsWhatEachKindWouldGetAlone(t *testing.T) {
	validator := shippedValidator(t)

	decisions := storesOfOneKind(sweptDecisions)
	invariants := storesOfOneKind(sweptInvariants)
	if len(decisions) != len(invariants) {
		t.Fatalf("the two spaces hold %d and %d stores; the pairing below assumes one size",
			len(decisions), len(invariants))
	}
	size := len(decisions)

	// Which of the four combinations each pairing produced, so a sweep that only
	// ever mixed two clean halves cannot pass in silence.
	combinations := map[string]int{}

	for i, left := range decisions {
		right := invariants[(i*1237+911)%size]

		wantLeft := oracleCycleMembers(left)
		wantRight := oracleCycleMembers(right)
		want := slices.Concat(wantLeft, wantRight)
		slices.Sort(want)
		combinations[fmt.Sprintf("decisions=%v invariants=%v", len(wantLeft) > 0, len(wantRight) > 0)]++

		mixed := slices.Concat(left, right)
		findings := validate.Check(storeOf(t, recordsFor(mixed)), validator)
		if got := pathsAt(findings, validate.StepSupersedeCycle); !slices.Equal(got, want) {
			t.Fatalf("mixed store %s\nstep 9 reported %v\nits two halves alone say %v\n\t%s",
				renderStore(mixed), got, want, describe(findings))
		}
	}

	// All four combinations have to occur, or the sweep graded fewer answers than
	// it looks like it did. The two mixed ones are the interesting rows: a store
	// where one kind is broken and the other is clean is where a fused graph
	// would drag the clean half into the finding.
	for _, left := range []bool{false, true} {
		for _, right := range []bool{false, true} {
			key := fmt.Sprintf("decisions=%v invariants=%v", left, right)
			if combinations[key] == 0 {
				t.Errorf("no pairing produced %s, so that combination was never graded", key)
			}
		}
	}
	t.Logf("paired %d mixed stores: %v", size, combinations)
}

// TestAnInvariantCycleIsFatalAndNamesOnlyInvariants reads the milestone's one
// fatal finding over an invariant group, in a store that also holds a decision
// group.
//
// Two records of each kind close a loop, so the report must carry four findings
// and each of them must name its own two ids and neither of the other kind's.
// The ids are numbered alike on purpose — DEC-0001 beside INV-0001 — because a
// graph keyed on the bare id string would fuse them, and a fixture that gave the
// two kinds different numbers could not tell the difference.
func TestAnInvariantCycleIsFatalAndNamesOnlyInvariants(t *testing.T) {
	closed := func(target string) map[string]any {
		return map[string]any{"status": "superseded", "supersedes": []any{target}}
	}
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", closed("DEC-0002"))),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", closed("DEC-0001"))),
		invariantAt("INV-0001.json", invariantDoc("INV-0001", closed("INV-0002"))),
		invariantAt("INV-0002.json", invariantDoc("INV-0002", closed("INV-0001"))),
	})

	findings := validate.Check(store, shippedValidator(t))

	want := []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0002.json",
		".mindrail/knowledge/invariants/INV-0001.json",
		".mindrail/knowledge/invariants/INV-0002.json",
	}
	if got := pathsAt(findings, validate.StepSupersedeCycle); !slices.Equal(got, want) {
		t.Fatalf("step 9 reported %v, want both closed groups %v\n\t%s", got, want, describe(findings))
	}

	for _, finding := range findingsAt(findings, validate.StepSupersedeCycle) {
		if !finding.Fatal {
			t.Errorf("the step 9 finding on %s is not fatal (decision D-39)", finding.Path)
		}
		if finding.Code != app.CodeKnowledgeSupersedeCycle {
			t.Errorf("the step 9 finding on %s carries code %q, want %q",
				finding.Path, finding.Code, app.CodeKnowledgeSupersedeCycle)
		}

		// A message that named the other kind's ids would send the reader to a
		// file whose bytes have nothing to do with the group they are in.
		own, other := "INV-000", "DEC-000"
		if strings.Contains(finding.Path, "/decisions/") {
			own, other = "DEC-000", "INV-000"
		}
		for _, n := range []string{"1", "2"} {
			if !strings.Contains(finding.Message, own+n) {
				t.Errorf("the finding on %s does not name its group member %s: %q",
					finding.Path, own+n, finding.Message)
			}
			if strings.Contains(finding.Message, other+n) {
				t.Errorf("the finding on %s names %s, which is in the other kind's group: %q",
					finding.Path, other+n, finding.Message)
			}
		}
		if want := "group of 2 records:"; !strings.Contains(finding.Message, want) {
			t.Errorf("the finding on %s counts its group as something other than %q:\n\t%q",
				finding.Path, want, finding.Message)
		}
	}
}

// TestTheDecisionsAreStillSilentWhenOnlyTheInvariantsClose is the other arm of
// the test above, and the one that would catch a fused graph.
//
// Exactly one edge moves: INV-0002 no longer names INV-0001, so the invariant
// lineage has an end and the decision pair is the only closed group left. A
// graph that keyed nodes on the bare id would still have INV-0001 and DEC-0001
// as one node, and the invariants would be dragged into the decisions' finding.
func TestTheDecisionsAreStillSilentWhenOnlyTheInvariantsClose(t *testing.T) {
	validator := shippedValidator(t)

	// Arm one: the decisions close, the invariants do not.
	decisionsOnly := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"status": "superseded", "supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"status": "superseded", "supersedes": []any{"DEC-0001"},
		})),
		invariantAt("INV-0001.json", invariantDoc("INV-0001", map[string]any{
			"status": "superseded", "supersedes": []any{"INV-0002"},
		})),
		invariantAt("INV-0002.json", invariantDoc("INV-0002", nil)),
	})
	want := []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0002.json",
	}
	findings := validate.Check(decisionsOnly, validator)
	if got := pathsAt(findings, validate.StepSupersedeCycle); !slices.Equal(got, want) {
		t.Errorf("step 9 reported %v, want only the closed decision pair %v\n\t%s",
			got, want, describe(findings))
	}

	// Arm two: the same store with the two kinds' roles exchanged.
	invariantsOnly := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"status": "superseded", "supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", nil)),
		invariantAt("INV-0001.json", invariantDoc("INV-0001", map[string]any{
			"status": "superseded", "supersedes": []any{"INV-0002"},
		})),
		invariantAt("INV-0002.json", invariantDoc("INV-0002", map[string]any{
			"status": "superseded", "supersedes": []any{"INV-0001"},
		})),
	})
	want = []string{
		".mindrail/knowledge/invariants/INV-0001.json",
		".mindrail/knowledge/invariants/INV-0002.json",
	}
	findings = validate.Check(invariantsOnly, validator)
	if got := pathsAt(findings, validate.StepSupersedeCycle); !slices.Equal(got, want) {
		t.Errorf("step 9 reported %v, want only the closed invariant pair %v\n\t%s",
			got, want, describe(findings))
	}
}

// TestAnInvariantThatSupersedesItselfIsCountedAsOneRecord is the group-of-one
// message over the kind that had never reached it.
//
// Round 4's R4-M16 found the fatal message hard-coding the plural noun, and the
// helper that fixed it is shared by both kinds — but shared code with one kind's
// test behind it is exactly the arrangement that made "no test asserts step 9
// over an invariant" survive four rounds.
func TestAnInvariantThatSupersedesItselfIsCountedAsOneRecord(t *testing.T) {
	findings := validate.Check(storeOf(t, []testRecord{
		invariantAt("INV-0001.json", invariantDoc("INV-0001", map[string]any{
			"status":     "superseded",
			"supersedes": []any{"INV-0001"},
		})),
	}), shippedValidator(t))

	got := findingsAt(findings, validate.StepSupersedeCycle)
	if len(got) != 1 {
		t.Fatalf("a self-superseding invariant produced %d step-9 findings, want 1:\n\t%s",
			len(got), describe(findings))
	}
	if want := "group of 1 record:"; !strings.Contains(got[0].Message, want) {
		t.Errorf("step 9 counts the group as %q; the message reads:\n\t%q", want, got[0].Message)
	}
	if !got[0].Fatal {
		t.Error("a self-superseding invariant is a lineage with no end, so its finding is fatal")
	}
}

// TestASupersedeTargetInTheOtherKindsDirectoryIsNotAnEdge is the seam the mixed
// sweep cannot reach: the enumeration only ever writes same-kind targets,
// because both shipped documents pin the array's items to their own kind's id
// pattern and a cross-kind target is therefore a record step 5 refuses.
//
// Refusing it is not the same as ignoring it. A rejected record keeps its edges
// — that is decision D-52 and the whole of round 1's fail-open — so these two
// files really do declare a loop, and the only thing standing between them and a
// fatal finding is that a lineage node is a (kind, id) pair. Both records are
// still reported, by step 5, which is where a reader is told what to fix.
func TestASupersedeTargetInTheOtherKindsDirectoryIsNotAnEdge(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"INV-0001"},
		})),
		invariantAt("INV-0001.json", invariantDoc("INV-0001", map[string]any{
			"supersedes": []any{"DEC-0001"},
		})),
	})

	findings := validate.Check(store, shippedValidator(t))

	if got := pathsAt(findings, validate.StepSupersedeCycle); len(got) != 0 {
		t.Errorf("step 9 reported %v; a decision and an invariant naming each other are two nodes, not a loop\n\t%s",
			got, describe(findings))
	}
	for _, recordPath := range []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/invariants/INV-0001.json",
	} {
		if got := stepsAt(findings, recordPath); !slices.Contains(got, validate.StepSchema) {
			t.Errorf("%s reported steps %v; a cross-kind supersede target breaks the shipped pattern and step 5 has to say so\n\t%s",
				recordPath, got, describe(findings))
		}
	}
}

// TestAFileInTheInvariantsDirectoryCannotCloseALineageOfDecisions is the test
// that has teeth against a graph which forgot the kind, and it took a mutation
// to find out that the one above does not.
//
// The reason is worth stating, because it is not obvious and it is the whole
// argument for this fixture. Both shipped documents pin their id with a
// `pattern`, so no record step 5 accepts can carry the other kind's id — and a
// store built only from accepted records therefore behaves identically whether
// the graph keys on (kind, id) or on the bare id string. The separation is only
// observable through a record step 5 *refused*, and a refused record receives no
// finding of its own (D-40). So the only way to see the kind do any work is to
// have a refused record of one kind change the verdict on an accepted record of
// the other.
//
// That is this store. `invariants/DEC-0002.json` is a file in the invariants
// directory named after a decision: it is canonical for its own name, it is
// refused by step 5 twice over — the id and the supersede target both break
// invariant.v1's pattern — and it declares the one edge that would close the two
// decisions into a loop. Keyed on (kind, id) it owns node (invariant, DEC-0002),
// whose supersede target (invariant, DEC-0001) is not in the store, so it
// contributes nothing and the decisions stay silent. Keyed on the bare id it
// becomes a second owner of DEC-0002 and hands the decisions a fatal finding
// neither of their documents asks for — round 2's defect, arriving through the
// kind door.
func TestAFileInTheInvariantsDirectoryCannotCloseALineageOfDecisions(t *testing.T) {
	validator := shippedValidator(t)

	bridged := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"status": "superseded", "supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", nil)),
		invariantAt("DEC-0002.json", invariantDoc("DEC-0002", map[string]any{
			"supersedes": []any{"DEC-0001"},
		})),
	})

	findings := validate.Check(bridged, validator)
	if got := pathsAt(findings, validate.StepSupersedeCycle); len(got) != 0 {
		t.Errorf("step 9 reported %v; the edge that closes the loop is declared in the invariants directory, where it names no decision\n\t%s",
			got, describe(findings))
	}
	for _, finding := range findings {
		if strings.Contains(finding.Path, "/decisions/") {
			t.Errorf("the two decisions are correct, and %s reported step %d: %q",
				finding.Path, finding.Step, finding.Message)
		}
	}

	// The other arm: the same edge, declared where it does belong. Without it the
	// assertion above would hold for a store that simply has no loop in it, and
	// this fixture would prove nothing about the kind.
	closed := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"status": "superseded", "supersedes": []any{"DEC-0002"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"status": "superseded", "supersedes": []any{"DEC-0001"},
		})),
	})
	want := []string{
		".mindrail/knowledge/decisions/DEC-0001.json",
		".mindrail/knowledge/decisions/DEC-0002.json",
	}
	if got := pathsAt(validate.Check(closed, validator), validate.StepSupersedeCycle); !slices.Equal(got, want) {
		t.Errorf("step 9 reported %v for the same pair with the edge in the decisions directory, want %v", got, want)
	}
}
