// Package coordination carries the sequential agent handover: who was working
// (Session), what on (Task), and what they said on the way out (Checkpoint).
//
// The milestone it serves is MR-003, and its scenario is one sentence: two
// different AgentSessions using the same workspace one after the other continue
// the same Task. Everything in this package follows from that.
//
// Three properties are load-bearing rather than incidental.
//
// A session is minted, never resumed (decision D-56). The second agent of a
// handover gets a new session id and continues the same task; nothing looks a
// session up to reattach to it and no "current session" is stored anywhere.
// Continuity lives in the Task and the Checkpoint, which are the two entities
// that survive a process exit — a design that resumed sessions would have put it
// in the least durable of the three.
//
// The state table below is the only place a task's lifecycle is written down
// (decision D-55). Every writer consults it; a second copy — in the CLI's flag
// validation, say — is how the two would come to disagree about what BLOCKED may
// become.
//
// CLAIMED is a state, not a lease (decision D-58). This package records which
// session claimed a task and nothing else: no expiry, no renewal, no refusal on
// the grounds of time. MR-004 adds the lease on top of that column.
package coordination

import (
	"fmt"
	"slices"
	"strings"
)

// State is where a task is in spec §58's lifecycle.
type State string

const (
	StateOpen            State = "OPEN"
	StateClaimed         State = "CLAIMED"
	StateInProgress      State = "IN_PROGRESS"
	StateBlocked         State = "BLOCKED"
	StateReadyToComplete State = "READY_TO_COMPLETE"
	StateCompleted       State = "COMPLETED"
	StateAbandoned       State = "ABANDONED"
)

// states is every state, in lifecycle order rather than alphabetically: a help
// text and an error message that list them in the order work moves through them
// tell a reader something an alphabetical list does not.
var states = []State{
	StateOpen,
	StateClaimed,
	StateInProgress,
	StateBlocked,
	StateReadyToComplete,
	StateCompleted,
	StateAbandoned,
}

// States returns every state a task can be in, in lifecycle order. The result is
// a copy: it reaches help text and error messages, and a caller that sorted what
// it was given would reorder the vocabulary for everyone.
func States() []State { return slices.Clone(states) }

// transitions is decision D-55's table, and the only statement of it.
//
// Two entries are worth naming because neither is a mechanical consequence of
// the lifecycle running forwards.
//
// CLAIMED -> OPEN is the release path. An agent that claimed a task and cannot
// do it puts it back rather than abandoning work someone else could pick up, and
// without this edge "I claimed the wrong thing" would cost the task.
//
// READY_TO_COMPLETE -> IN_PROGRESS is the reopen path. It is what keeps the
// completion gate MR-013 will add from being a one-way door: a task whose
// evidence turns out to be stale has to be able to go back to being worked on.
//
// COMPLETED and ABANDONED have no entry at all. They are terminal, and a
// terminal state with an escape hatch is not one.
var transitions = map[State][]State{
	StateOpen:            {StateClaimed, StateAbandoned},
	StateClaimed:         {StateInProgress, StateOpen, StateAbandoned},
	StateInProgress:      {StateBlocked, StateReadyToComplete, StateAbandoned},
	StateBlocked:         {StateInProgress, StateAbandoned},
	StateReadyToComplete: {StateCompleted, StateInProgress, StateAbandoned},
	StateCompleted:       nil,
	StateAbandoned:       nil,
}

// CanTransition reports whether a task in `from` may move to `to`.
//
// A move to the state the task is already in is refused like any other absent
// edge, and that is deliberate rather than an oversight. Treating it as a
// success would be a second, weaker idempotency rule — MR-004 owns idempotency
// and does it with an operation_id — and a command that reported success while
// changing nothing is the failure mode MR-001's audit spent three findings on.
func CanTransition(from, to State) bool {
	return slices.Contains(transitions[from], to)
}

// TransitionsFrom returns the states a task in `from` may move to, in the order
// the table declares them. It exists so a refusal can tell the reader what *is*
// available instead of only what is not.
func TransitionsFrom(from State) []State { return slices.Clone(transitions[from]) }

// Terminal reports whether a task in this state can still move. It is derived
// from the table rather than from a list of its own, so a state that gained an
// outgoing edge cannot stay terminal by omission.
func (s State) Terminal() bool { return len(transitions[s]) == 0 }

// Valid reports whether s is one of the seven. A State read out of the database
// or off a command line is a string until this says otherwise.
func (s State) Valid() bool { return slices.Contains(states, s) }

// String makes State printable without a conversion at every call site.
func (s State) String() string { return string(s) }

// ParseState turns a caller's spelling into a State.
//
// It is case-insensitive on the way in because the wire spelling is uppercase
// and a person typing `--to completed` at a shell means the same thing. It is
// not permissive about anything else: no abbreviations, no hyphens for
// underscores. The published vocabulary is seven words and a caller who typed an
// eighth needs to be told which seven exist, which is what the error does.
//
// The error is a plain one, deliberately. Whether a bad `--to` value is a usage
// mistake or a domain refusal is a question about the caller, not about the
// value, and this package has no opinion on command lines — the CLI wraps this
// into app.KindUsage, and a future MCP tool will wrap it into its own shape.
func ParseState(raw string) (State, error) {
	candidate := State(strings.ToUpper(strings.TrimSpace(raw)))
	if candidate.Valid() {
		return candidate, nil
	}

	names := make([]string, 0, len(states))
	for _, state := range states {
		names = append(names, string(state))
	}
	return "", fmt.Errorf("%q is not a task state; the states are %s", raw, strings.Join(names, ", "))
}
