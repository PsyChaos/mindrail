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
// The enumeration is audit round 3's, reproduced exactly: every store of two or
// three files, file names drawn from <PREFIX>-0001.json, <PREFIX>-0002.json and
// <PREFIX>-0003.json, each file declaring id <PREFIX>-0001 or <PREFIX>-0002,
// superseding any subset of those two ids, and either satisfying its schema or
// carrying one property no schema defines. That is 3 x 16^2 + 16^3 = 4,864
// stores per kind, and it is the space in which all three of MR-002's identity
// defects live: <PREFIX>-0003.json can never be filed under an id it might
// declare, so every store containing it holds a claimant that owns nothing.
//
// # Why the enumeration is run twice
//
// Round 3 built it out of decisions, and until round 4 said so out loud nobody
// noticed that every fixture in the repository had. Decision D-52 is not a rule
// about decisions: it says "the record filed at the path its id names speaks for
// that id", and spec §95 step 6 imposes that on both kinds. A rule stated over
// two kinds and measured over one has been measured over half of what it claims,
// so the enumeration below runs once per kind and the oracle is written in terms
// of a kind rather than in terms of the decisions directory.
//
// The oracle is written independently of the implementation and stays that way
// on purpose. It does not build a graph, run Tarjan or share a line with
// lineage.go: with two possible ids a closed walk is either one file superseding
// itself or two files superseding each other, so the whole question is four
// comparisons. An oracle that reused the code it grades would agree with it by
// construction, which is what makes most exhaustive sweeps worth nothing.

// sweepKind is the vocabulary one kind's enumeration needs: the directory its
// records are filed in, the prefix both shipped documents pin its ids with, and
// the two fixture builders that put a document of that kind at a file name.
//
// It exists so the enumeration is written once. The alternative — a second copy
// of the sweep with "DEC" replaced by "INV" — is how the two would drift into
// grading different rules under one name.
type sweepKind struct {
	dir    string
	prefix string
	at     func(name, body string) testRecord
	doc    func(id string, extra map[string]any, drop ...string) string
}

var (
	sweptDecisions  = sweepKind{dir: "decisions", prefix: "DEC", at: decisionAt, doc: decisionDoc}
	sweptInvariants = sweepKind{dir: "invariants", prefix: "INV", at: invariantAt, doc: invariantDoc}
)

// id is this kind's spelling of the nth id in the enumeration; file is the name
// the record carrying it has to be filed under to own it.
func (k sweepKind) id(n int) string   { return fmt.Sprintf("%s-%04d", k.prefix, n) }
func (k sweepKind) file(n int) string { return k.id(n) + ".json" }

// storeFile is one file in a swept store: which kind's directory it sits in,
// what it is called, what it says it is, what it says it replaces, and whether
// step 5 takes it.
type storeFile struct {
	kind       sweepKind
	name       string
	id         string
	supersedes []string
	rejected   bool
}

// owns reports whether this file is the canonical record for the id it declares
// — decision D-52's test, spelled here as the file name rather than as a call
// into the package under test.
func (f storeFile) owns() bool { return f.name == f.id+".json" }

// path is where a finding about this file says it is. It is assembled from
// literals for the same reason the oracle builds no graph: a path spelled by the
// code under test would agree with that code whatever either of them said.
func (f storeFile) path() string { return ".mindrail/knowledge/" + f.kind.dir + "/" + f.name }

// TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore is the sweep,
// run over each kind's own 4,864 stores.
//
// The assertion is on the exact set of files step 9 reports, not on whether it
// reported anything. Every one of MR-002's three identity defects published the
// right *number* of findings for some store while naming the wrong files, and
// round 2's HIGH was a fatal finding attached to two files that were correct.
func TestTheOwnershipRuleAgreesWithAnIndependentOracleOverEveryStore(t *testing.T) {
	for _, kind := range []sweepKind{sweptDecisions, sweptInvariants} {
		t.Run(kind.dir, func(t *testing.T) {
			validator := shippedValidator(t)

			swept, withCycle, twoSelfLoops := 0, 0, 0
			for _, store := range storesOfOneKind(kind) {
				swept++
				want := oracleCycleMembers(store)
				if len(want) > 0 {
					withCycle++
				}
				if holdsTwoReportableSelfLoops(store) {
					twoSelfLoops++
				}

				findings := validate.Check(storeOf(t, recordsFor(store)), validator)
				got := pathsAt(findings, validate.StepSupersedeCycle)
				if slices.Equal(got, want) {
					continue
				}
				t.Fatalf("store %s\nstep 9 reported %v\nthe oracle says   %v\n\t%s",
					renderStore(store), got, want, describe(findings))
			}

			// The sweep's own guard. A bug in the enumeration that produced ten
			// stores, or an oracle that found a cycle in none of them, would
			// otherwise pass in silence — and this test's whole value is that it
			// looked at all of them.
			if swept != 4864 {
				t.Errorf("the sweep covered %d stores, and round 3's enumeration is 4864", swept)
			}
			if withCycle == 0 || withCycle == swept {
				t.Errorf("%d of %d stores hold a reportable cycle, so the sweep grades only one answer", withCycle, swept)
			}
			// The measurement behind oracleCycleMembers's comment about why a
			// per-record rule is right for a reason unrelated to component counts.
			// Asserted rather than asserted-in-prose: round 4's R4-M17 removed a
			// dead guard that justified itself with a false claim about this shape,
			// and the claim that replaced it is only worth as much as its number.
			if twoSelfLoops != 204 {
				t.Errorf("%d stores hold two ids that each supersede themselves; the measured figure is 204", twoSelfLoops)
			}
			t.Logf("swept %d stores, %d of them holding a reportable supersede cycle", swept, withCycle)
		})
	}
}

// storesOfOneKind enumerates round 3's space in one kind.
func storesOfOneKind(k sweepKind) [][]storeFile {
	names := []string{k.file(1), k.file(2), k.file(3)}
	ids := []string{k.id(1), k.id(2)}
	supersedeSets := [][]string{nil, {ids[0]}, {ids[1]}, {ids[0], ids[1]}}

	// Every file a store may contain: 2 ids x 4 supersede sets x 2 schema
	// verdicts, per file name.
	variants := func(name string) []storeFile {
		out := make([]storeFile, 0, len(ids)*len(supersedeSets)*2)
		for _, id := range ids {
			for _, targets := range supersedeSets {
				for _, rejected := range []bool{false, true} {
					out = append(out, storeFile{kind: k, name: name, id: id, supersedes: targets, rejected: rejected})
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

	stores := make([][]storeFile, 0, 4864)
	for _, set := range nameSets {
		perFile := make([][]storeFile, 0, len(set))
		for _, name := range set {
			perFile = append(perFile, variants(name))
		}
		stores = append(stores, combine(perFile)...)
	}
	return stores
}

// recordsFor renders a swept store as the fixture files it describes.
func recordsFor(store []storeFile) []testRecord {
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
		records = append(records, file.kind.at(file.name, file.kind.doc(file.id, extra)))
	}
	return records
}

// oracleNode is one identity in a swept store: a directory and an id. The
// directory is half the key because an id is only unique inside its kind, and
// keying on the id alone would make "a decision and an invariant are never one
// record" true in the oracle by assumption rather than by measurement.
type oracleNode struct{ dir, id string }

// oracleCycleMembers returns the record paths step 9 must report for a store,
// derived from decision D-52's four rules and from nothing else.
//
// Rules 1 and 2: only a file filed under the id it declares speaks for that id,
// whatever step 5 made of the document. Rule 3 falls out of the same line — an
// id no file is named after owns nothing, so it supplies no edge and cannot be
// on a walk. Rule 4 and decision D-40 decide who is named: the canonical record,
// and only when step 5 accepted it.
//
// "supersedes" is read as same-kind, which is not this oracle's invention: both
// shipped documents pin the array's items to their own kind's id pattern, so a
// decision naming an invariant's id is a record step 5 refuses, not an edge.
func oracleCycleMembers(store []storeFile) []string {
	// The owner of each id, if the store has one. At most one file can be
	// canonical for an id, because file names are unique in a directory.
	owner := make(map[oracleNode]storeFile, len(store))
	for _, file := range store {
		if file.owns() {
			owner[oracleNode{dir: file.kind.dir, id: file.id}] = file
		}
	}

	// With two ids in play per kind, a closed walk is one of two shapes.
	onCycle := make(map[oracleNode]bool, len(owner))
	for key, file := range owner {
		if slices.Contains(file.supersedes, key.id) {
			onCycle[key] = true
		}
		for _, target := range file.supersedes {
			to := oracleNode{dir: key.dir, id: target}
			other, exists := owner[to]
			if !exists || to == key {
				continue
			}
			if slices.Contains(other.supersedes, key.id) {
				onCycle[key], onCycle[to] = true, true
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
	// themselves are two disjoint components, and 204 of each kind's swept stores
	// hold that shape — the figure the sweep above now counts rather than claims.
	// A per-record rule is right for a reason that has nothing to do with how many
	// components there are, which is what makes the guard both dead and
	// unnecessary.
	paths := make([]string, 0, len(onCycle))
	for key := range onCycle {
		file := owner[key]
		if file.rejected {
			continue
		}
		paths = append(paths, file.path())
	}
	slices.Sort(paths)
	return paths
}

// holdsTwoReportableSelfLoops reports the two-disjoint-components shape the
// comment above describes: both ids owned, each owner naming itself, and at
// least one of the two accepted by step 5.
//
// The last clause is what makes the number a statement about the dead guard
// rather than about the enumeration. 272 stores per kind hold two
// self-superseding owners; in 68 of them step 5 refused both, so the store
// reports nothing and a guard that mistook the partition for one component
// could not have been caught there. The remaining 204 are the stores where the
// guard's false premise met a finding.
func holdsTwoReportableSelfLoops(store []storeFile) bool {
	selfLooping, reportable := 0, 0
	for _, file := range store {
		if !file.owns() || !slices.Contains(file.supersedes, file.id) {
			continue
		}
		selfLooping++
		if !file.rejected {
			reportable++
		}
	}
	return selfLooping == 2 && reportable > 0
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
		parts = append(parts, fmt.Sprintf("%s/%s{id=%s supersedes=%v %s}",
			file.kind.dir, file.name, file.id, file.supersedes, verdict))
	}
	return strings.Join(parts, " ")
}
