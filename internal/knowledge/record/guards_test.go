package record_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// The two refusals in this file are the ones finding BA-09 found unkilled: both
// are reachable, neither had an input pointed at it, and deleting either was
// silent across the whole suite. Each is paired here with the adjacent healthy
// case, because a refusal that fires on its neighbour is worse than one that
// never fires at all — the neighbour is the case callers actually have.

// TestAConstructorNamesThePositionOfANilOption reaches the nil-Option check in
// newBuilder, which is the only thing standing between a caller and a panic
// inside the variadic loop.
//
// A nil Option is not exotic: `record.NewDecision(id, at, title, decision,
// optionalScope())` where the helper returns a nil Option on some branch is the
// ordinary way to produce one, and Go has no way to refuse it at compile time
// because a nil func value satisfies the Option type. Without the check the call
// dereferences it, and the caller gets a stack trace inside a package they did
// not write instead of a sentence naming the argument they passed.
//
// The position is asserted, not merely the failure: with three options in hand,
// "one of them is nil" is not actionable and "option 1 is nil" is.
func TestAConstructorNamesThePositionOfANilOption(t *testing.T) {
	scope := record.WithScope(record.Scope{Level: record.ScopeProject})

	tests := []struct {
		name string
		opts []record.Option
		want string
	}{
		{name: "the only option", opts: []record.Option{nil}, want: "option 0 is nil"},
		{name: "first of three", opts: []record.Option{nil, scope, record.WithTags("a")}, want: "option 0 is nil"},
		{name: "second of three", opts: []record.Option{scope, nil, record.WithTags("a")}, want: "option 1 is nil"},
		{name: "last of three", opts: []record.Option{scope, record.WithTags("a"), nil}, want: "option 2 is nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both constructors share newBuilder, and both are asked, so a check
			// moved into one of them cannot pass by covering the other.
			decision, err := record.NewDecision("DEC-0001", createdAt, "A title", "A decision.", tt.opts...)
			if err == nil {
				t.Fatalf("NewDecision() error = nil, want a refusal; got %+v", decision)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewDecision() error = %q, want it to say %q", err, tt.want)
			}

			invariant, err := record.NewInvariant("INV-0001", createdAt, "A statement.", tt.opts...)
			if err == nil {
				t.Fatalf("NewInvariant() error = nil, want a refusal; got %+v", invariant)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewInvariant() error = %q, want it to say %q", err, tt.want)
			}
		})
	}
}

// TestOptionsThatAreNotNilAreStillApplied is the over-fire guard for the check
// above. "This option is nil" must be decided per option and not per call, so a
// call carrying several real options — including one whose effect is to set an
// empty array, which is the value nearest to nothing an Option can carry — has
// to construct and has to carry every value it was given.
func TestOptionsThatAreNotNilAreStillApplied(t *testing.T) {
	built, err := record.NewDecision("DEC-0001", createdAt, "A title", "A decision.",
		record.WithScope(record.Scope{Level: record.ScopeProject}),
		record.WithTags("knowledge", "lifecycle"),
		record.WithSupersedes(),
		record.WithContext("Because."))
	if err != nil {
		t.Fatalf("NewDecision() error = %v, want nil", err)
	}
	if !slices.Equal(built.Tags, []string{"knowledge", "lifecycle"}) {
		t.Errorf("Tags = %v, want the two that were set", built.Tags)
	}
	if built.Context != "Because." {
		t.Errorf("Context = %q, want %q", built.Context, "Because.")
	}
	if built.Scope == nil || built.Scope.Level != record.ScopeProject {
		t.Errorf("Scope = %+v, want the PROJECT scope that was set", built.Scope)
	}
	requireValid(t, schema.KindDecision, built)

	// No options at all is the other neighbour: an empty variadic list must not
	// be read as a list containing nothing usable.
	bare, err := record.NewDecision("DEC-0002", createdAt, "A title", "A decision.")
	if err != nil {
		t.Fatalf("NewDecision() with no options: error = %v, want nil", err)
	}
	requireValid(t, schema.KindDecision, bare)
}

// TestSupersedeRefusesAReplacementWhoseSupersedesAlreadyRepeatsItself reaches
// the uniqueItems check inside Supersede.
//
// The input has to be assembled by hand, and that is the point rather than an
// awkwardness: NewDecision's own checkSupersedes refuses a duplicate before the
// record exists, so the only Decision that can carry one is a struct literal or
// a field written after construction — which is precisely what a caller
// decoding a record off disk and then superseding it produces.
//
// Deleting the check is not merely untidy. Supersede appends to that array and
// hands it back as a record the caller is told to write; a duplicate that
// survives is written to disk, and step 5 then reports a uniqueItems violation
// against a file this package produced. The writer/validator pairing exists to
// stop exactly that.
func TestSupersedeRefusesAReplacementWhoseSupersedesAlreadyRepeatsItself(t *testing.T) {
	prior := activeDecision(t, "DEC-0001")

	tests := []struct {
		name       string
		supersedes []string
		value      string
	}{
		{name: "adjacent duplicates", supersedes: []string{"DEC-0003", "DEC-0003"}, value: "DEC-0003"},
		{name: "separated duplicates", supersedes: []string{"DEC-0003", "DEC-0004", "DEC-0003"}, value: "DEC-0003"},
		{name: "duplicate of the prior itself", supersedes: []string{"DEC-0001", "DEC-0001"}, value: "DEC-0001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replacement := activeDecision(t, "DEC-0002")
			replacement.Supersedes = tt.supersedes

			carrying, moved, err := record.Supersede(prior, replacement)
			if err == nil {
				t.Fatalf("Supersede() error = nil, want a refusal; it returned %v and %v, "+
					"and writing the first would put a uniqueItems violation on disk", carrying.Supersedes, moved.ID)
			}

			var duplicate *record.DuplicateItemError
			if !errors.As(err, &duplicate) {
				t.Fatalf("Supersede() error = %v, want a *DuplicateItemError", err)
			}
			if duplicate.Property != "supersedes" {
				t.Errorf("DuplicateItemError.Property = %q, want %q", duplicate.Property, "supersedes")
			}
			if duplicate.Value != tt.value {
				t.Errorf("DuplicateItemError.Value = %q, want %q", duplicate.Value, tt.value)
			}
			// Nothing is handed back beside the error: a caller that ignored it
			// must not be able to write a half-built pair.
			if carrying.ID != "" || moved.ID != "" {
				t.Errorf("Supersede() returned %q and %q beside its error, want two zero Decisions", carrying.ID, moved.ID)
			}
		})
	}
}

// TestSupersedeAcceptsAReplacementWhoseSupersedesAreMerelyMany is the over-fire
// guard for the test above, and it is the case that matters: a long, legitimate
// lineage is the normal shape of a record that has superseded several others,
// and a uniqueItems check that fired on it would refuse the ordinary call.
//
// The two neighbours are covered together — distinct entries, and an entry equal
// to the prior's id, which is a repeat of a value already present but not a
// repeat *within* the array. TestSupersedeDoesNotRepeatATargetItAlreadyCarries
// owns the second one's idempotence; what is asserted here is that neither is
// mistaken for the condition above.
func TestSupersedeAcceptsAReplacementWhoseSupersedesAreMerelyMany(t *testing.T) {
	prior := activeDecision(t, "DEC-0001")
	replacement := activeDecision(t, "DEC-0002")
	replacement.Supersedes = []string{"DEC-0003", "DEC-0004", "DEC-0005"}

	carrying, _, err := record.Supersede(prior, replacement)
	if err != nil {
		t.Fatalf("Supersede() error = %v, want nil for three distinct entries", err)
	}
	want := []string{"DEC-0003", "DEC-0004", "DEC-0005", "DEC-0001"}
	if !slices.Equal(carrying.Supersedes, want) {
		t.Fatalf("Supersedes = %v, want %v", carrying.Supersedes, want)
	}
	requireValid(t, schema.KindDecision, carrying)

	// The prior's id already present is not a duplicate: the array still holds
	// each value once, and Supersede has to notice that rather than appending a
	// second copy and then refusing its own output.
	alreadyThere := activeDecision(t, "DEC-0002")
	alreadyThere.Supersedes = []string{"DEC-0003", "DEC-0001"}
	again, _, err := record.Supersede(prior, alreadyThere)
	if err != nil {
		t.Fatalf("Supersede() error = %v, want nil when the prior's id is already carried once", err)
	}
	if !slices.Equal(again.Supersedes, []string{"DEC-0003", "DEC-0001"}) {
		t.Fatalf("Supersedes = %v, want it unchanged", again.Supersedes)
	}
	requireValid(t, schema.KindDecision, again)
}
