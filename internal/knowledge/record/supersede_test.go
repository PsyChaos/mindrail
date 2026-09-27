package record_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// activeDecision builds a valid active Decision for the supersede fixtures.
func activeDecision(t *testing.T, id string, opts ...record.Option) record.Decision {
	t.Helper()
	built, err := record.NewDecision(id, createdAt, "A title", "A decision.", opts...)
	if err != nil {
		t.Fatalf("NewDecision(%q) error = %v, want nil", id, err)
	}
	return built
}

// TestSupersedeReturnsTheReplacementFirstAndMovesThePrior holds AC-01.4's
// positive half.
//
// The return order is asserted explicitly because both returns are Decision and
// a swapped call site compiles: the first return is identified by carrying the
// prior's id in "supersedes", the second by being the prior with its status
// moved. Naming them the other way round would fail here rather than at the
// point where two records overwrote each other's files.
func TestSupersedeReturnsTheReplacementFirstAndMovesThePrior(t *testing.T) {
	prior := activeDecision(t, "DEC-0001", record.WithTags("knowledge"))
	replacement := activeDecision(t, "DEC-0002")

	carrying, superseded, err := record.Supersede(prior, replacement)
	if err != nil {
		t.Fatalf("Supersede() error = %v, want nil", err)
	}

	if carrying.ID != replacement.ID {
		t.Fatalf("first return has id %q, want the replacement's %q", carrying.ID, replacement.ID)
	}
	if !slices.Contains(carrying.Supersedes, prior.ID) {
		t.Errorf("first return Supersedes = %v, want it to contain the prior's id %q", carrying.Supersedes, prior.ID)
	}
	if carrying.Status != record.StatusActive {
		t.Errorf("first return Status = %q, want %q; the replacement is what is current now", carrying.Status, record.StatusActive)
	}

	if superseded.ID != prior.ID {
		t.Fatalf("second return has id %q, want the prior's %q", superseded.ID, prior.ID)
	}
	if superseded.Status != record.StatusSuperseded {
		t.Errorf("second return Status = %q, want %q", superseded.Status, record.StatusSuperseded)
	}

	// The prior keeps everything else it had: superseding is a status change,
	// not a rewrite. A record that lost its tags on the way through would break
	// the lineage the spec keeps records for (spec §93).
	if !slices.Equal(superseded.Tags, prior.Tags) {
		t.Errorf("second return Tags = %v, want the prior's %v", superseded.Tags, prior.Tags)
	}
	if superseded.Title != prior.Title || superseded.Decision != prior.Decision {
		t.Errorf("second return prose changed: %+v, want the prior's", superseded)
	}

	// Both halves of the pair are records step 5 accepts, which is what makes
	// the transition writable rather than merely computed.
	requireValid(t, schema.KindDecision, carrying)
	requireValid(t, schema.KindDecision, superseded)
}

// TestSupersedeRejectsSelfAndAnAlreadySupersededPriorWithDistinctErrors holds
// the rest of AC-01.4.
//
// "Distinct" is asserted both ways round: each rejection matches its own
// sentinel and does *not* match the other's. Asserting only the positive
// direction would pass if both conditions returned one shared error, which is
// exactly the collapse the criterion forbids.
func TestSupersedeRejectsSelfAndAnAlreadySupersededPriorWithDistinctErrors(t *testing.T) {
	same := activeDecision(t, "DEC-0001")

	superseded := activeDecision(t, "DEC-0001")
	superseded.Status = record.StatusSuperseded

	tests := []struct {
		name       string
		prior      record.Decision
		replace    record.Decision
		wantErr    error
		notWantErr error
	}{
		{
			name:       "a decision superseding itself",
			prior:      same,
			replace:    same,
			wantErr:    record.ErrSupersedesSelf,
			notWantErr: record.ErrPriorAlreadySuperseded,
		},
		{
			name:       "a prior that was already superseded",
			prior:      superseded,
			replace:    activeDecision(t, "DEC-0002"),
			wantErr:    record.ErrPriorAlreadySuperseded,
			notWantErr: record.ErrSupersedesSelf,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			carrying, moved, err := record.Supersede(tt.prior, tt.replace)
			if err == nil {
				t.Fatalf("Supersede() error = nil, want one matching %v", tt.wantErr)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Supersede() error = %v, want one matching %v", err, tt.wantErr)
			}
			if errors.Is(err, tt.notWantErr) {
				t.Fatalf("Supersede() error = %v matches %v as well, so the two conditions are not distinct", err, tt.notWantErr)
			}
			if carrying.ID != "" || moved.ID != "" {
				t.Errorf("Supersede() returned %+v / %+v alongside its error, want two zero Decisions", carrying, moved)
			}
		})
	}
}

// TestSupersedeRefusesAPriorWithNoStatusItCanReasonAbout covers the third
// refusal: a zero Decision has neither status the schema declares, and calling
// it "already superseded" would name a transition that never happened.
func TestSupersedeRefusesAPriorWithNoStatusItCanReasonAbout(t *testing.T) {
	prior := activeDecision(t, "DEC-0001")
	prior.Status = ""

	_, _, err := record.Supersede(prior, activeDecision(t, "DEC-0002"))
	if !errors.Is(err, record.ErrPriorNotActive) {
		t.Fatalf("Supersede() error = %v, want one matching ErrPriorNotActive", err)
	}
	if errors.Is(err, record.ErrPriorAlreadySuperseded) {
		t.Fatal("a prior with no status was reported as already superseded, which names a transition that never happened")
	}
}

// TestSupersedeRefusesRecordsWithoutAWellFormedID exists so the self-supersede
// refusal cannot fire on two unnamed records. Two zero Decisions have equal ids
// and would otherwise be told they supersede themselves — a diagnosis of a
// mistake the caller did not make.
func TestSupersedeRefusesRecordsWithoutAWellFormedID(t *testing.T) {
	_, _, err := record.Supersede(record.Decision{}, record.Decision{})
	if !errors.Is(err, record.ErrInvalidID) {
		t.Fatalf("Supersede() over two zero Decisions: error = %v, want one matching ErrInvalidID", err)
	}
	if errors.Is(err, record.ErrSupersedesSelf) {
		t.Fatal("two unnamed decisions were reported as superseding themselves")
	}
}

// TestSupersedeDoesNotRepeatATargetItAlreadyCarries guards the schema's
// uniqueItems on "supersedes": a replacement that already names the prior — for
// instance because Supersede was called twice on the same pair — must not grow
// a second copy, which step 5 would reject.
func TestSupersedeDoesNotRepeatATargetItAlreadyCarries(t *testing.T) {
	prior := activeDecision(t, "DEC-0001")
	replacement := activeDecision(t, "DEC-0002", record.WithSupersedes("DEC-0001", "DEC-0003"))

	carrying, _, err := record.Supersede(prior, replacement)
	if err != nil {
		t.Fatalf("Supersede() error = %v, want nil", err)
	}

	want := []string{"DEC-0001", "DEC-0003"}
	if !slices.Equal(carrying.Supersedes, want) {
		t.Fatalf("Supersedes = %v, want %v unchanged", carrying.Supersedes, want)
	}
	requireValid(t, schema.KindDecision, carrying)

	// Twice through is the same as once through: the transition is idempotent
	// on the array it appends to.
	again, _, err := record.Supersede(prior, carrying)
	if err != nil {
		t.Fatalf("Supersede() a second time: error = %v, want nil", err)
	}
	if !slices.Equal(again.Supersedes, want) {
		t.Fatalf("Supersedes after a second Supersede = %v, want %v", again.Supersedes, want)
	}
}

// TestSupersedeSharesNoBackingArrayWithItsInputs is the immutability rule where
// it is load-bearing: append reuses a caller's backing array whenever it has
// spare capacity, so a replacement the caller still holds could grow a member
// it never added.
func TestSupersedeSharesNoBackingArrayWithItsInputs(t *testing.T) {
	// The prior carries its own lineage, because superseding a record that
	// already superseded something is the ordinary case in a chain — and
	// because it is the only way the moved copy's "supersedes" array is
	// non-empty and therefore aliasable at all.
	prior := activeDecision(t, "DEC-0001",
		record.WithTags("knowledge"),
		record.WithSupersedes("DEC-0009"),
		record.WithScope(record.Scope{Level: record.ScopeFile, Target: "internal/app/code.go"}))
	// Capacity beyond length is what makes the aliasing reachable; a slice
	// built by append alone would usually have none and the bug would hide.
	replacement := activeDecision(t, "DEC-0002", record.WithSupersedes("DEC-0003"))
	replacement.Supersedes = append(make([]string, 0, 8), replacement.Supersedes...)

	carrying, moved, err := record.Supersede(prior, replacement)
	if err != nil {
		t.Fatalf("Supersede() error = %v, want nil", err)
	}

	// Editing the outputs must not reach the inputs.
	carrying.Supersedes[0] = "DEC-9999"
	moved.Supersedes[0] = "DEC-8888"
	moved.Tags[0] = "rewritten"
	moved.Scope.Target = "somewhere/else.go"

	if replacement.Supersedes[0] != "DEC-0003" {
		t.Errorf("input replacement Supersedes[0] = %q after the output was edited, want %q", replacement.Supersedes[0], "DEC-0003")
	}
	if prior.Supersedes[0] != "DEC-0009" {
		t.Errorf("input prior Supersedes[0] = %q after the output was edited, want %q", prior.Supersedes[0], "DEC-0009")
	}
	if prior.Tags[0] != "knowledge" {
		t.Errorf("input prior Tags[0] = %q after the output was edited, want %q", prior.Tags[0], "knowledge")
	}
	if prior.Scope.Target != "internal/app/code.go" {
		t.Errorf("input prior Scope.Target = %q after the output was edited, want %q", prior.Scope.Target, "internal/app/code.go")
	}
	if prior.Status != record.StatusActive {
		t.Errorf("input prior Status = %q, want %q; Supersede returns a moved copy rather than moving its argument", prior.Status, record.StatusActive)
	}
	if len(replacement.Supersedes) != 1 {
		t.Errorf("input replacement Supersedes = %v, want the one entry it was built with", replacement.Supersedes)
	}
}
