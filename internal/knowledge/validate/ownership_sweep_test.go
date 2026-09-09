package validate_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// This file is decision D-52's own acceptance condition, which the decision
// states as a requirement rather than a suggestion: "the implementation must be
// checked against every store the three audit rounds constructed, not against a
// new fixture set", and "if any case in that enumeration comes out wrong under
// this rule, the rule is wrong".
//
// The enumeration is audit round 3's, reproduced exactly: every decision store of
// two or three files, file names drawn from DEC-0001.json, DEC-0002.json and
// DEC-0003.json, each file declaring id DEC-0001 or DEC-0002, superseding any
// subset of those two ids, and either satisfying its schema or carrying one
// property no schema defines. That is 3 x 16^2 + 16^3 = 4,864 stores, and it is
// the space in which all three of MR-002's identity defects live: DEC-0003.json
// can never be filed under an id it might declare, so every store containing it
// holds a claimant that owns nothing.
//
// The oracle below is written independently of the implementation and stays that
// way on purpose. It does not build a graph, run Tarjan or share a line with
// lineage.go: with two possible ids a closed walk is either one file superseding
// itself or two files superseding each other, so the whole question is four
// comparisons. An oracle that reused the code it grades would agree with it by
// construction, which is what makes most exhaustive sweeps worth nothing.

// storeFile is one file in a swept store: what it is called, what it says it is,
// what it says it replaces, and whether step 5 takes it.
type storeFile struct {
	name       string
	id         string
	supersedes []string
	rejected   bool
}

// owns reports whether this file is the canonical record for the id it declares
// — decision D-52's test, spelled here as the file name rather than as a call
// into the package under test.
func (f storeFile) owns() bool { return f.name == f.id+".json" }

// TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore is the sweep.
//
// The assertion is on the exact set of files step 9 reports, not on whether it
// reported anything. Every one of MR-002's three identity defects published the
// right *number* of findings for some store while naming the wrong files, and
// round 2's HIGH was a fatal finding attached to two files that were correct.
func TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore(t *testing.T) {
	validator := shippedValidator(t)

	names := []string{"DEC-0001.json", "DEC-0002.json", "DEC-0003.json"}
	ids := []string{"DEC-0001", "DEC-0002"}
	supersedeSets := [][]string{nil, {"DEC-0001"}, {"DEC-0002"}, {"DEC-0001", "DEC-0002"}}

	// Every file a store may contain: 2 ids x 4 supersede sets x 2 schema
	// verdicts, per file name.
	variants := func(name string) []storeFile {
		out := make([]storeFile, 0, len(ids)*len(supersedeSets)*2)
		for _, id := range ids {
			for _, targets := range supersedeSets {
				for _, rejected := range []bool{false, true} {
					out = append(out, storeFile{name: name, id: id, supersedes: targets, rejected: rejected})
				}
			}
		}
		return out
	}

	// The name sets: every 2- and 3-subset of the three file names, in order, so
	// a failure names a store a reader can rebuild.
	nameSets := [][]string{
		{names[0], names[1]},
		{names[0], names[2]},
		{names[1], names[2]},
		{names[0], names[1], names[2]},
	}

	swept, withCycle := 0, 0
	for _, set := range nameSets {
		perFile := make([][]storeFile, 0, len(set))
		for _, name := range set {
			perFile = append(perFile, variants(name))
		}

		for _, store := range combine(perFile) {
			swept++
			want := oracleCycleMembers(store)
			if len(want) > 0 {
				withCycle++
			}

			records := make([]testRecord, 0, len(store))
			for _, file := range store {
				extra := map[string]any{}
				if len(file.supersedes) > 0 {
					targets := make([]any, 0, len(file.supersedes))
					for _, target := range file.supersedes {
						targets = append(targets, target)
					}
					extra["supersedes"] = targets
				}
				if file.rejected {
					extra["note"] = "junk"
				}
				records = append(records, decisionAt(file.name, decisionDoc(file.id, extra)))
			}

			findings := validate.Check(storeOf(t, records), validator)
			got := pathsAt(findings, validate.StepSupersedeCycle)
			if slices.Equal(got, want) {
				continue
			}
			t.Fatalf("store %s\nstep 9 reported %v\nthe oracle says   %v\n\t%s",
				renderStore(store), got, want, describe(findings))
		}
	}

	// The sweep's own guard. A bug in the enumeration that produced ten stores,
	// or an oracle that found a cycle in none of them, would otherwise pass in
	// silence — and this test's whole value is that it looked at all of them.
	if swept != 4864 {
		t.Errorf("the sweep covered %d stores, and round 3's enumeration is 4864", swept)
	}
	if withCycle == 0 || withCycle == swept {
		t.Errorf("%d of %d stores hold a reportable cycle, so the sweep grades only one answer", withCycle, swept)
	}
	t.Logf("swept %d stores, %d of them holding a reportable supersede cycle", swept, withCycle)
}

// oracleCycleMembers returns the record paths step 9 must report for a store,
// derived from decision D-52's four rules and from nothing else.
//
// Rules 1 and 2: only a file filed under the id it declares speaks for that id,
// whatever step 5 made of the document. Rule 3 falls out of the same line — an
// id no file is named after owns nothing, so it supplies no edge and cannot be
// on a walk. Rule 4 and decision D-40 decide who is named: the canonical record,
// and only when step 5 accepted it.
func oracleCycleMembers(store []storeFile) []string {
	// The owner of each id, if the store has one. At most one file can be
	// canonical for an id, because file names are unique in a directory.
	owner := make(map[string]storeFile, 2)
	for _, file := range store {
		if file.owns() {
			owner[file.id] = file
		}
	}

	// With two ids in play, a closed walk is one of two shapes.
	onCycle := make(map[string]bool, 2)
	for id, file := range owner {
		if slices.Contains(file.supersedes, id) {
			onCycle[id] = true
		}
		for _, target := range file.supersedes {
			other, exists := owner[target]
			if !exists || target == id {
				continue
			}
			if slices.Contains(other.supersedes, id) {
				onCycle[id], onCycle[target] = true, true
			}
		}
	}

	// D-40 decides who is named, and it decides it per record: a record step 5
	// refused was never asked step 9, so it receives no finding and the reader is
	// told to correct it by step 5 instead. The skip below is the whole of that
	// rule, and it needs to know nothing about where the components are — a group
	// whose every member was refused contributes nothing because each of its
	// members is skipped, not because anything counted the group.
	//
	// That is why there is no "is this group reportable at all" guard here. An
	// earlier draft had one; it could not change any of the 4,864 answers, because
	// the loop below already returns an empty slice in exactly the cases the guard
	// returned one for. It also justified itself with "every closed walk in this
	// space is a single component", which is false: two ids that each supersede
	// themselves are two disjoint components, and 204 of the swept stores hold
	// that shape. A per-record rule is right for a reason that has nothing to do
	// with how many components there are, which is what makes the guard both dead
	// and unnecessary.
	paths := make([]string, 0, len(onCycle))
	for id := range onCycle {
		if owner[id].rejected {
			continue
		}
		paths = append(paths, ".mindrail/knowledge/decisions/"+owner[id].name)
	}
	slices.Sort(paths)
	return paths
}

// combine returns every store that takes one variant of each file.
func combine(perFile [][]storeFile) [][]storeFile {
	stores := [][]storeFile{{}}
	for _, variants := range perFile {
		next := make([][]storeFile, 0, len(stores)*len(variants))
		for _, prefix := range stores {
			for _, variant := range variants {
				store := make([]storeFile, len(prefix), len(prefix)+1)
				copy(store, prefix)
				next = append(next, append(store, variant))
			}
		}
		stores = next
	}
	return stores
}

// renderStore spells a failing store out in one line, so a failure is a fixture
// rather than an index into an enumeration nobody can see.
func renderStore(store []storeFile) string {
	parts := make([]string, 0, len(store))
	for _, file := range store {
		verdict := "valid"
		if file.rejected {
			verdict = "rejected"
		}
		parts = append(parts, fmt.Sprintf("%s{id=%s supersedes=%v %s}",
			file.name, file.id, file.supersedes, verdict))
	}
	return strings.Join(parts, " ")
}
