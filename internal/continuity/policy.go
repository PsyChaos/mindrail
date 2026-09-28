package continuity

import (
	"errors"
	"fmt"
)

const (
	DefaultWarnUsedPercent    = 55
	DefaultHandoffUsedPercent = 60
	DefaultHardUsedPercent    = 75
	DefaultConsecutiveReports = 2
)

type Policy struct {
	WarnUsedBasisPoints    int
	HandoffUsedBasisPoints int
	HardUsedBasisPoints    int
	ConsecutiveReports     int
}

func DefaultPolicy() Policy {
	return Policy{5500, 6000, 7500, DefaultConsecutiveReports}
}

func (p Policy) Validate() error {
	if p.WarnUsedBasisPoints < 1 || p.WarnUsedBasisPoints >= p.HandoffUsedBasisPoints ||
		p.HandoffUsedBasisPoints >= p.HardUsedBasisPoints || p.HardUsedBasisPoints > 10_000 {
		return errors.New("continuity thresholds must satisfy 0 < warn < handoff < hard <= 10000 basis points")
	}
	if p.ConsecutiveReports < 1 || p.ConsecutiveReports > 20 {
		return errors.New("continuity consecutive reports must be between 1 and 20")
	}
	return nil
}

// Evaluate is pure: callers persist its returned state using compare-and-swap.
// Lower or duplicate sequence numbers are ignored and no observation reverses a
// previously reached state.
func (p Policy) Evaluate(current Intent, observation Observation) (Intent, Decision, error) {
	if err := p.Validate(); err != nil {
		return current, Decision{}, err
	}
	if observation.Sequence < 1 || observation.ObservedAt.IsZero() || observation.Source == "" {
		return current, Decision{}, errors.New("continuity observation needs sequence, time and source")
	}
	if observation.Sequence <= current.LastObservationSequence {
		return current, Decision{State: current.State, UsedBasisPoints: current.LastUsedBasisPoints, Ignored: true}, nil
	}

	next := current
	next.LastObservationSequence = observation.Sequence
	decision := Decision{State: current.State, UsedBasisPoints: -1}
	if !observation.HasContextPressure() {
		next.LastUsedBasisPoints = -1
		next.ConsecutiveHandoffObservations = 0
		return next, decision, nil
	}

	used := observation.UsedBasisPoints()
	next.LastUsedBasisPoints = used
	decision.UsedBasisPoints = used
	if used >= p.HandoffUsedBasisPoints {
		next.ConsecutiveHandoffObservations++
	} else {
		next.ConsecutiveHandoffObservations = 0
	}

	switch current.State {
	case "", StateObserving:
		if used >= p.WarnUsedBasisPoints {
			next.State = StateWarned
			decision.Warn = true
		}
	case StateWarned:
	case StateCheckpointed, StateSpawnRequested, StateSpawnReady, StateHandedOff,
		StateClaimed, StateResumed, StateCompleted, StateManualRequired, StateCancelled, StateExpired:
		decision.State = current.State
		decision.HardProtection = used >= p.HardUsedBasisPoints && !terminal(current.State)
		return next, decision, nil
	default:
		return current, Decision{}, fmt.Errorf("unknown continuity state %q", current.State)
	}

	if next.State == StateWarned && used >= p.HandoffUsedBasisPoints &&
		next.ConsecutiveHandoffObservations >= p.ConsecutiveReports {
		decision.Checkpoint = true
		decision.RequestHandoff = true
	}
	decision.State = next.State
	decision.HardProtection = used >= p.HardUsedBasisPoints
	return next, decision, nil
}

func terminal(state State) bool {
	return state == StateCompleted || state == StateCancelled || state == StateExpired || state == StateManualRequired
}
