// Package doctor answers one question about an installation — "is this set up
// correctly, and if not, what exactly is wrong?" — as a list of independent,
// structured readings.
//
// Two properties are load-bearing. Doctor mutates nothing (spec §83): every
// check is a pure function of an already-resolved Subject, so running it can
// never turn a reportable misconfiguration into a silently repaired one. And
// every reading that is not OK carries a diagnostic, an impact and at least one
// next action (spec §84), because a health report that says "broken" without
// saying what it costs or what to do is indistinguishable from noise.
package doctor

import (
	"encoding/json"
	"fmt"
	"slices"
)

// State is the spec §84 health vocabulary. It describes one reading, not the
// installation: overall readiness is status.Readiness and the terminal init
// verdict is status.TerminalState, which are deliberately separate enums
// (decision D-16) because collapsing them would lose information the spec
// asserts about three different objects.
type State string

// The five states, in reporting order from healthiest to most severe cause for
// concern. NOT_APPLICABLE is not a failure: it means the capability is not used
// by this project (§84), never that this binary cannot provide it.
const (
	StateOK            State = "OK"
	StateDegraded      State = "DEGRADED"
	StateError         State = "ERROR"
	StateUnavailable   State = "UNAVAILABLE"
	StateNotApplicable State = "NOT_APPLICABLE"
)

// allStates is the registry. It is declared once so States, Valid and the
// decoder cannot drift apart.
var allStates = []State{
	StateOK,
	StateDegraded,
	StateError,
	StateUnavailable,
	StateNotApplicable,
}

// States returns the five states in declaration order. The result is a copy:
// the registry backs Valid and must survive a caller that sorts or truncates
// what it was given.
func States() []State {
	return slices.Clone(allStates)
}

// Valid reports whether s is one of the five. It exists so a state arriving
// from a persisted report or another process can be rejected rather than
// carried further as a plausible-looking unknown.
func (s State) Valid() bool {
	return slices.Contains(allStates, s)
}

// Marker returns the human glyph of decision D-21. Non-OK results print the
// state word next to it, so nothing about the report's meaning depends on the
// glyph rendering correctly.
func (s State) Marker() string {
	switch s {
	case StateOK:
		return "✓"
	case StateDegraded:
		return "△"
	case StateError:
		return "✗"
	case StateUnavailable:
		return "–"
	case StateNotApplicable:
		return "·"
	default:
		return "?"
	}
}

// UnmarshalJSON fails closed on anything outside States(). A report is read
// back by tooling that branches on the state, and a state this binary does not
// understand is a value it must not pretend to have understood.
func (s *State) UnmarshalJSON(b []byte) error {
	var raw string
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("doctor: state must be a JSON string: %w", err)
	}

	candidate := State(raw)
	if !candidate.Valid() {
		return fmt.Errorf("doctor: unknown state %q, want one of %v", raw, allStates)
	}

	*s = candidate
	return nil
}

// severity orders the states for worst-state reporting. UNAVAILABLE outranks
// DEGRADED because a subsystem that cannot be reached at all is worse than one
// running with less than it wants, and NOT_APPLICABLE ranks below OK because
// "not used here" is not a claim about health at all.
func severity(s State) int {
	switch s {
	case StateError:
		return 4
	case StateUnavailable:
		return 3
	case StateDegraded:
		return 2
	case StateOK:
		return 1
	case StateNotApplicable:
		return 0
	default:
		// An unrecognised state has to outrank everything: it is the one value
		// nobody can reason about.
		return 5
	}
}
