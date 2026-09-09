package validate_test

import (
	"os"
	"slices"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// wideEnumerationEnv turns the enumeration below on. It is off by default
// because it costs about twelve seconds without the race detector and several
// minutes with it, and `make check` is a gate meant to run on every save
// (tech-stack §136).
//
// The name deliberately does not start with MINDRAIL_. Every fixture in
// internal/cli refuses to run against a shell that exports one, so a developer
// who left this set would silently skip the whole CLI suite instead of running
// this one.
const wideEnumerationEnv = "MR002_WIDE_ENUMERATION"

// TestTheOwnershipRuleHoldsOverTheWiderSpace is the enumeration round 4 reached
// by sampling.
//
// The shipped acceptance sweep (ownership_sweep_test.go) is round 3's space: two
// ids, so a closed walk is either a self-edge or a two-record loop. Round 4's
// auditors went wider with fuzzers — 60,000 random stores, 16,000 cycle-biased —
// and one of them enumerated three files over three ids, 110,592 stores, which
// was the round's strongest single result and was decisions-only. Everything
// with three or more records on a closed walk lives in that space and in no
// shipped test.
//
// This is that enumeration, for both kinds: three files, three ids, every subset
// of the three as "supersedes", each file either satisfying its schema or
// carrying one property no schema defines. 3 x 8 x 2 = 48 variants per file and
// 48^3 = 110,592 stores per kind.
//
// The oracle is a transitive closure — Warshall over a three-node adjacency
// matrix — and it is a different algorithm from the implementation's Tarjan on
// purpose. A node is on a closed walk exactly when it can reach itself in one or
// more steps, which is the definition step 9 is written against, arrived at
// without sharing a line with the code that answers it.
//
// Run it with:
//
//	MR002_WIDE_ENUMERATION=1 go test ./internal/knowledge/validate/ \
//	  -run TestTheOwnershipRuleHoldsOverTheWiderSpace -timeout 600s -v
func TestTheOwnershipRuleHoldsOverTheWiderSpace(t *testing.T) {
	if os.Getenv(wideEnumerationEnv) == "" {
		t.Skipf("set %s=1 to run the 110,592-store enumeration per kind", wideEnumerationEnv)
	}

	for _, kind := range []sweepKind{sweptDecisions, sweptInvariants} {
		t.Run(kind.dir, func(t *testing.T) {
			validator := shippedValidator(t)
			ids := []string{kind.id(1), kind.id(2), kind.id(3)}

			subsets := make([][]string, 0, 8)
			for mask := range 8 {
				set := make([]string, 0, len(ids))
				for i, id := range ids {
					if mask&(1<<i) != 0 {
						set = append(set, id)
					}
				}
				subsets = append(subsets, set)
			}

			variants := func(name string) []storeFile {
				out := make([]storeFile, 0, len(ids)*len(subsets)*2)
				for _, id := range ids {
					for _, set := range subsets {
						for _, rejected := range []bool{false, true} {
							out = append(out, storeFile{
								kind: kind, name: name, id: id, supersedes: set, rejected: rejected,
							})
						}
					}
				}
				return out
			}

			perFile := [][]storeFile{
				variants(kind.file(1)), variants(kind.file(2)), variants(kind.file(3)),
			}

			swept, withCycle, groupsOfThree := 0, 0, 0
			for _, store := range combine(perFile) {
				swept++
				want := reachabilityOracle(store, ids)
				if len(want) > 0 {
					withCycle++
				}
				if len(want) == 3 {
					groupsOfThree++
				}

				findings := validate.Check(storeOf(t, recordsFor(store)), validator)
				if got := pathsAt(findings, validate.StepSupersedeCycle); !slices.Equal(got, want) {
					t.Fatalf("store %s\nstep 9 reported %v\nthe oracle says   %v\n\t%s",
						renderStore(store), got, want, describe(findings))
				}
			}

			if swept != 110592 {
				t.Errorf("the enumeration covered %d stores, and 48^3 is 110592", swept)
			}
			// The rows the shipped sweep cannot hold. Without them this would be
			// the two-id space again, run more slowly.
			if groupsOfThree == 0 {
				t.Error("no store produced a three-record group, so the wider space was not reached")
			}
			t.Logf("swept %d stores, %d holding a reportable cycle, %d of them a group of three",
				swept, withCycle, groupsOfThree)
		})
	}
}

// reachabilityOracle returns the record paths step 9 must report, computed by
// transitive closure rather than by strong-component decomposition.
//
// Edges come only from a node's canonical record (decision D-52) and only to
// ids some file in the store declares — an id nothing claims cannot be walked
// to. Who is *named* is decision D-40: the canonical record, and only when step
// 5 accepted it.
func reachabilityOracle(store []storeFile, ids []string) []string {
	owner := make(map[string]storeFile, len(ids))
	declared := make(map[string]bool, len(ids))
	for _, file := range store {
		declared[file.id] = true
		if file.owns() {
			owner[file.id] = file
		}
	}

	index := make(map[string]int, len(ids))
	for i, id := range ids {
		index[id] = i
	}
	reach := make([][]bool, len(ids))
	for i := range reach {
		reach[i] = make([]bool, len(ids))
	}
	for id, file := range owner {
		for _, target := range file.supersedes {
			if declared[target] {
				reach[index[id]][index[target]] = true
			}
		}
	}
	for k := range ids {
		for i := range ids {
			for j := range ids {
				if reach[i][k] && reach[k][j] {
					reach[i][j] = true
				}
			}
		}
	}

	paths := make([]string, 0, len(ids))
	for i, id := range ids {
		file, owned := owner[id]
		if !reach[i][i] || !owned || file.rejected {
			continue
		}
		paths = append(paths, file.path())
	}
	slices.Sort(paths)
	return paths
}
