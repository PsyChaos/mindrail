package continuity

import (
	"reflect"
	"testing"
	"time"
)

func TestDefaultPolicyThresholds(t *testing.T) {
	p := DefaultPolicy()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.WarnUsedBasisPoints != 5500 || p.HandoffUsedBasisPoints != 6000 || p.HardUsedBasisPoints != 7500 {
		t.Fatalf("unexpected defaults: %#v", p)
	}
}

func TestPolicyWarnsAndRequiresStableHandoffPressure(t *testing.T) {
	p := DefaultPolicy()
	intent := Intent{State: StateObserving, LastUsedBasisPoints: -1}
	base := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	intent, decision, err := p.Evaluate(intent, observation(1, 550_000, base))
	if err != nil || !decision.Warn || intent.State != StateWarned {
		t.Fatalf("warn: %#v %#v %v", intent, decision, err)
	}
	intent, decision, err = p.Evaluate(intent, observation(2, 600_000, base.Add(time.Second)))
	if err != nil || decision.RequestHandoff || intent.State != StateWarned {
		t.Fatalf("first: %#v %#v %v", intent, decision, err)
	}
	intent, decision, err = p.Evaluate(intent, observation(3, 600_000, base.Add(2*time.Second)))
	if err != nil || !decision.Checkpoint || !decision.RequestHandoff || intent.State != StateWarned {
		t.Fatalf("stable: %#v %#v %v", intent, decision, err)
	}
}

func TestPolicyIgnoresStaleAndUnknownContext(t *testing.T) {
	p := DefaultPolicy()
	base := time.Now().UTC()
	intent := Intent{State: StateWarned, LastObservationSequence: 4, LastUsedBasisPoints: 5700}
	next, decision, err := p.Evaluate(intent, observation(4, 900_000, base))
	if err != nil || !decision.Ignored || !reflect.DeepEqual(next, intent) {
		t.Fatalf("stale: %#v %#v %v", next, decision, err)
	}
	next, decision, err = p.Evaluate(intent, Observation{Sequence: 5, ContextUsed: -1, ObservedAt: base, Source: "test"})
	if err != nil || decision.RequestHandoff || next.State != StateWarned || next.LastUsedBasisPoints != -1 {
		t.Fatalf("unknown: %#v %#v %v", next, decision, err)
	}
}

func TestPolicyHardProtectionAtBoundary(t *testing.T) {
	p := DefaultPolicy()
	intent := Intent{State: StateWarned, LastObservationSequence: 1, ConsecutiveHandoffObservations: 1}
	next, decision, err := p.Evaluate(intent, observation(2, 750_000, time.Now().UTC()))
	if err != nil || !decision.HardProtection || next.State != StateWarned {
		t.Fatalf("hard: %#v %#v %v", next, decision, err)
	}
}

func TestPolicyUsesExactFloorBoundariesAndNeedsPositiveUsage(t *testing.T) {
	p := DefaultPolicy()
	base := time.Now().UTC()
	intent := Intent{State: StateObserving, LastUsedBasisPoints: -1}
	next, decision, err := p.Evaluate(intent, Observation{Sequence: 1, ContextUsed: 54_995, ContextLimit: 100_000, ObservedAt: base, Source: "test"})
	if err != nil || decision.Warn || next.State != StateObserving || next.LastUsedBasisPoints != 5499 {
		t.Fatalf("below boundary: next=%#v decision=%#v err=%v", next, decision, err)
	}
	next, decision, err = p.Evaluate(next, Observation{Sequence: 2, ContextUsed: 0, ContextLimit: 100_000, ObservedAt: base.Add(time.Second), Source: "test"})
	if err != nil || decision.Warn || next.LastUsedBasisPoints != -1 {
		t.Fatalf("zero usage: next=%#v decision=%#v err=%v", next, decision, err)
	}
}

func observation(sequence, used int64, at time.Time) Observation {
	return Observation{Sequence: sequence, ContextUsed: used, ContextLimit: 1_000_000, ObservedAt: at, Source: "test"}
}
