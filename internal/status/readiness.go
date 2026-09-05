// Package status owns the readiness model of spec §107: the six component
// health slots, the overall readiness verdict derived from them, and the shapes
// `mindrail status` and `mindrail init` report.
//
// It is the one package name MR-001 adds to tech-stack §7 (decision D-17). The
// model is shared by status and init, it is not a doctor check, and it cannot
// live under internal/index because MR-005 owns that tree and MR-001 builds no
// index.
//
// Nothing here mutates anything. Build is a pure function of the already
// resolved doctor.Subject, which is what lets `status` report a missing setup
// instead of quietly creating one (decision D-01).
package status

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/doctor"
)

// Readiness is the spec §107 overall verdict. It answers "can an agent start
// work here?", which is a different question from any single component's health
// (doctor.State) and from init's terminal verdict (TerminalState); decision
// D-16 keeps all three separate.
type Readiness string

const (
	// ReadinessPartialReady means some components are still coming up while the
	// rest are usable. MR-001 never reports it: with no index there is nothing
	// that can be partway ready (decision D-02). MR-005 gives it meaning.
	ReadinessPartialReady Readiness = "PARTIAL_READY"
	ReadinessReady        Readiness = "READY"
	ReadinessDegraded     Readiness = "DEGRADED"
	ReadinessBlocked      Readiness = "BLOCKED"
)

var allReadiness = []Readiness{
	ReadinessPartialReady,
	ReadinessReady,
	ReadinessDegraded,
	ReadinessBlocked,
}

// Readinesses returns the four values in spec §107 order, as a copy.
func Readinesses() []Readiness {
	return slices.Clone(allReadiness)
}

// Valid reports whether r is one of the four.
func (r Readiness) Valid() bool {
	return slices.Contains(allReadiness, r)
}

// UnmarshalJSON fails closed on anything outside Readinesses(). Agents gate
// their own behaviour on this field, so a value this binary does not understand
// must not be passed along as if it did.
func (r *Readiness) UnmarshalJSON(b []byte) error {
	var raw string
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("status: readiness must be a JSON string: %w", err)
	}

	candidate := Readiness(raw)
	if !candidate.Valid() {
		return fmt.Errorf("status: unknown readiness %q, want one of %v", raw, allReadiness)
	}

	*r = candidate
	return nil
}

// ComponentName is one of the six health slots of spec §107.
type ComponentName string

const (
	ComponentKnowledge   ComponentName = "knowledge"
	ComponentInventory   ComponentName = "inventory"
	ComponentSyntax      ComponentName = "syntax"
	ComponentSemantic    ComponentName = "semantic"
	ComponentCoverageMap ComponentName = "coverage_map"
	ComponentRuntimeDB   ComponentName = "runtime_db"
)

// componentOrder is spec §107's list, in its order. Reporting order is also
// blocking order: the first component that blocks is the one named as the
// blocker, so a knowledge problem is reported ahead of a runtime one.
var componentOrder = []ComponentName{
	ComponentKnowledge,
	ComponentInventory,
	ComponentSyntax,
	ComponentSemantic,
	ComponentCoverageMap,
	ComponentRuntimeDB,
}

// Components returns exactly the six of spec §107, in that order, as a copy.
func Components() []ComponentName {
	return slices.Clone(componentOrder)
}

// Component is one slot's health.
//
// It carries doctor.State rather than a vocabulary of its own because §84's
// five values already describe exactly this: a component that is fine, reduced,
// broken, unreachable, or not used by this project.
type Component struct {
	State      doctor.State `json:"state"`
	Summary    string       `json:"summary"`
	Code       app.Code     `json:"code,omitempty"`
	NextAction []string     `json:"next_action,omitempty"`
}

// blocks reports whether this component's state stops work.
//
// DEGRADED does not block: the installation still works with less than it
// wants. ERROR and UNAVAILABLE do, and NOT_APPLICABLE never can — §84 is
// explicit that an unused capability is not a failure, and treating it as one
// is exactly what would pin a healthy install at BLOCKED forever.
func (c Component) blocks() bool {
	return c.State == doctor.StateError || c.State == doctor.StateUnavailable
}

// inspected reports whether the check behind this component actually ran.
//
// STARTUP_INCOMPLETE is doctor's code for a reading whose inputs the startup
// sequence never populated. Such a component still blocks — work cannot start
// on an installation this report cannot vouch for — but it is a consequence of
// some other failure, never the cause of one.
func (c Component) inspected() bool {
	return c.Code != app.CodeStartupIncomplete
}
