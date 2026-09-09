package coordination_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/coordination"
)

// legalMoves is spec §58's lifecycle, written out here as a flat list of pairs
// and read from nothing.
//
// It is deliberately not the shape the implementation uses. `transitions` there
// is a map from a state to its targets; this is a set of ordered pairs, built by
// hand from the specification and from decision D-55's table. An expectation
// derived from the map — "every pair the map contains is legal" — would agree
// with any map at all, which is exactly the unfalsifiable guard MR-001's second
// audit round was spent deleting.
var legalMoves = map[string]bool{
	"OPEN>CLAIMED":                  true,
	"OPEN>ABANDONED":                true,
	"CLAIMED>IN_PROGRESS":           true,
	"CLAIMED>OPEN":                  true,
	"CLAIMED>ABANDONED":             true,
	"IN_PROGRESS>BLOCKED":           true,
	"IN_PROGRESS>READY_TO_COMPLETE": true,
	"IN_PROGRESS>ABANDONED":         true,
	"BLOCKED>IN_PROGRESS":           true,
	"BLOCKED>ABANDONED":             true,
	"READY_TO_COMPLETE>COMPLETED":   true,
	"READY_TO_COMPLETE>IN_PROGRESS": true,
	"READY_TO_COMPLETE>ABANDONED":   true,
}

// TestEveryOrderedPairOfStatesGetsTheAnswerTheSpecificationGives is AC-03.3 and
// AC-03.4: all 49 pairs, both arms, enumerated rather than sampled.
//
// Seven states is small enough that looking at fewer than all of them would be a
// choice, and the pairs that matter most are the ones nobody thinks to write a
// case for — a task moving to the state it is already in, a terminal state
// moving anywhere at all.
func TestEveryOrderedPairOfStatesGetsTheAnswerTheSpecificationGives(t *testing.T) {
	all := coordination.States()
	if len(all) != 7 {
		t.Fatalf("States() returned %d states, and spec §58 defines 7", len(all))
	}

	pairs, legal := 0, 0
	for _, from := range all {
		for _, to := range all {
			pairs++
			want := legalMoves[string(from)+">"+string(to)]
			if want {
				legal++
			}
			if got := coordination.CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}

	if pairs != 49 {
		t.Errorf("the sweep covered %d ordered pairs, and 7 states make 49", pairs)
	}
	// The guard on the guard. A table that answered `false` everywhere would
	// satisfy every assertion above if this expectation were also empty.
	if legal != 13 {
		t.Errorf("the expectation holds %d legal moves; decision D-55's table has 13", legal)
	}
}

// TestATaskNeverMovesToTheStateItIsAlreadyIn is the row above worth naming, and
// the one a hand-written case list reliably omits.
//
// It is refused rather than treated as a success. MR-004 owns idempotency and
// does it with an operation_id; a self-transition quietly reporting "done" now
// would be a second, weaker rule that MR-004 would have to contradict — and a
// command that reports success while changing nothing is the failure mode
// MR-001's audit spent three findings on.
func TestATaskNeverMovesToTheStateItIsAlreadyIn(t *testing.T) {
	for _, state := range coordination.States() {
		if coordination.CanTransition(state, state) {
			t.Errorf("CanTransition(%s, %s) = true; a task does not move to where it is", state, state)
		}
	}
}

// TestTheTerminalStatesAreTerminalAndNothingElseIs pins both arms of Terminal.
//
// Terminal is derived from the transition table rather than from a list of its
// own, so a state that gained an outgoing edge cannot stay terminal by
// omission — and a state that lost its last one cannot stay non-terminal.
func TestTheTerminalStatesAreTerminalAndNothingElseIs(t *testing.T) {
	terminal := map[coordination.State]bool{
		coordination.StateCompleted: true,
		coordination.StateAbandoned: true,
	}

	for _, state := range coordination.States() {
		if got := state.Terminal(); got != terminal[state] {
			t.Errorf("%s.Terminal() = %v, want %v", state, got, terminal[state])
		}
		if got := len(coordination.TransitionsFrom(state)); (got == 0) != terminal[state] {
			t.Errorf("%s has %d transitions and Terminal() says %v", state, got, terminal[state])
		}
	}
}

// TestARefusalCanSayWhatIsAvailableInstead is what makes TASK_STATE_INVALID
// actionable rather than merely correct.
//
// A message that says "you cannot do that" and stops leaves the reader to guess.
// Every non-terminal state has to be able to name its own alternatives, and the
// list has to be the table's rather than a second copy.
func TestARefusalCanSayWhatIsAvailableInstead(t *testing.T) {
	for _, state := range coordination.States() {
		available := coordination.TransitionsFrom(state)
		for _, target := range available {
			if !coordination.CanTransition(state, target) {
				t.Errorf("TransitionsFrom(%s) offers %s, which CanTransition refuses", state, target)
			}
		}
		if state.Terminal() {
			continue
		}
		if len(available) == 0 {
			t.Errorf("%s is not terminal and offers nothing", state)
		}
	}
}

// TestTransitionsFromHandsOutACopy is the guard on the two accessors that return
// slices out of package state.
//
// Both are read by help text and by error messages, and a caller that sorted or
// truncated what it was given would reorder the lifecycle for every later
// caller in the process. The bug is invisible until a second reader appears,
// which in a CLI is the second command in one test binary.
func TestTransitionsFromHandsOutACopy(t *testing.T) {
	first := coordination.TransitionsFrom(coordination.StateInProgress)
	if len(first) < 2 {
		t.Fatalf("IN_PROGRESS offers %v; this test needs at least two to reorder", first)
	}
	slices.Reverse(first)

	second := coordination.TransitionsFrom(coordination.StateInProgress)
	if slices.Equal(first, second) {
		t.Error("reversing the returned slice reordered the package's own table")
	}

	states := coordination.States()
	states[0] = coordination.State("MANGLED")
	if coordination.States()[0] != coordination.StateOpen {
		t.Error("writing to the returned slice rewrote the published state vocabulary")
	}
}

func TestParseStateAcceptsTheSevenAndRefusesEverythingElse(t *testing.T) {
	for _, state := range coordination.States() {
		for _, spelling := range []string{string(state), strings.ToLower(string(state)), " " + string(state) + " "} {
			got, err := coordination.ParseState(spelling)
			if err != nil {
				t.Errorf("ParseState(%q) = %v, want %s", spelling, err, state)
				continue
			}
			if got != state {
				t.Errorf("ParseState(%q) = %s, want %s", spelling, got, state)
			}
		}
	}

	for _, refused := range []string{
		"", "done", "OPENED", "IN-PROGRESS", "in progress", "READY", "COMPLETE",
	} {
		got, err := coordination.ParseState(refused)
		if err == nil {
			t.Errorf("ParseState(%q) = %s, want a refusal", refused, got)
			continue
		}
		// The reader has to learn the vocabulary from the refusal, or the only
		// way to find out what is accepted is to read the source.
		for _, state := range coordination.States() {
			if !strings.Contains(err.Error(), string(state)) {
				t.Errorf("ParseState(%q) refused without naming %s: %v", refused, state, err)
			}
		}
	}
}

// TestNoOtherPackageStatesTheLifecycle is decision D-55's "one place" clause,
// enforced rather than asked for.
//
// The seven state literals appearing together outside this package would mean a
// second statement of the vocabulary, and the copy is what drifts. The test
// looks for the literals rather than for the constants because a copy would be
// written as strings — that is what makes it a copy.
func TestNoOtherPackageStatesTheLifecycle(t *testing.T) {
	offenders := lifecycleLiteralsOutsideThisPackage(t)
	for _, file := range offenders {
		t.Errorf("%s spells the task lifecycle out; decision D-55 keeps it in internal/coordination", file)
	}
}

func init() {
	// Keeps the pair key format honest: a typo in legalMoves would silently make
	// a legal move look illegal, and the sweep would then be grading a typo.
	for pair := range legalMoves {
		from, to, found := strings.Cut(pair, ">")
		if !found || from == "" || to == "" {
			panic(fmt.Sprintf("legalMoves key %q is not a from>to pair", pair))
		}
	}
}
