package record

import (
	"fmt"
	"regexp"
	"slices"
)

// Option sets one optional field of a record under construction.
//
// A single Option type serves both kinds because the frozen signatures
// (design §3) declare one, and because the two records share most of their
// optional properties. The cost is that an option can be handed to the kind
// whose schema has no such property; that is refused with an *OptionKindError
// rather than ignored. additionalProperties:false means the value could never
// have been written, so ignoring it would leave the caller believing a field
// they set is in the record.
//
// Options are applied left to right and a later option setting the same
// property replaces the earlier one. WithSupersedes and WithTags therefore
// *replace* their arrays rather than appending to them; passing the same option
// twice keeps only the last.
type Option func(*builder) error

// builder accumulates what the options set, so the constructors validate the
// whole record once rather than each option validating in isolation. Several
// rules — a supersedes entry equal to the record's own id, an invariant with no
// scope — are only decidable when the required parameters and the options are
// both in hand.
type builder struct {
	// kind is the record being built, and is what lets an option refuse a kind
	// whose schema does not define its property.
	kind         string
	context      string
	consequences string
	rationale    string
	severity     Severity
	// severitySet separates "no WithSeverity was passed" from "WithSeverity was
	// passed an empty value". They are different mistakes and get different
	// errors; a bare `severity == ""` test would report the second as the first
	// and send the caller looking for a call they already made.
	severitySet bool
	supersedes  []string
	scope       *Scope
	tags        []string
}

// newBuilder applies every option in order.
func newBuilder(kind string, opts []Option) (*builder, error) {
	b := &builder{kind: kind}
	for index, opt := range opts {
		if opt == nil {
			// A nil Option would panic on call. Naming its position costs one
			// line and turns a stack trace into a fixable message.
			return nil, fmt.Errorf("knowledge record: option %d is nil", index)
		}
		if err := opt(b); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// forKind builds an option that only one record kind accepts.
func forKind(kind, property string, apply func(*builder)) Option {
	return func(b *builder) error {
		if b.kind != kind {
			return &OptionKindError{Property: property, Kind: b.kind}
		}
		apply(b)
		return nil
	}
}

// WithContext sets a Decision's "context": the situation that forced a choice.
// An Invariant's schema has no such property and refuses this option.
func WithContext(context string) Option {
	return forKind(KindDecision, "context", func(b *builder) { b.context = context })
}

// WithConsequences sets a Decision's "consequences": what the choice costs or
// forecloses. An Invariant's schema has no such property and refuses this
// option.
func WithConsequences(consequences string) Option {
	return forKind(KindDecision, "consequences", func(b *builder) { b.consequences = consequences })
}

// WithRationale sets an Invariant's "rationale": why violating the statement is
// harmful. A Decision's schema has no such property and refuses this option.
func WithRationale(rationale string) Option {
	return forKind(KindInvariant, "rationale", func(b *builder) { b.rationale = rationale })
}

// WithSeverity sets an Invariant's "severity". It is not optional in the
// schema's sense — invariant.v1 requires the property — but it arrives as an
// option because NewInvariant's signature has no parameter for it, and
// NewInvariant fails when it is absent rather than inventing a default. A
// Decision's schema has no such property and refuses this option.
func WithSeverity(severity Severity) Option {
	return forKind(KindInvariant, "severity", func(b *builder) {
		b.severity = severity
		b.severitySet = true
	})
}

// WithScope sets where the record applies. It is optional for a Decision and
// required for an Invariant, exactly as the two schema documents differ.
//
// The copy inside the closure is load-bearing, and its absence is subtle
// enough to be worth spelling out: the closure captures the parameter, so
// `b.scope = &scope` would hand every builder the address of one shared
// variable. One Option value reused across two constructions — which is
// ordinary, since an Option is a value a caller keeps — would then give both
// records the same *Scope, and editing either record's scope would edit the
// other's. Copying per application is what keeps a reused Option safe.
func WithScope(scope Scope) Option {
	return func(b *builder) error {
		copied := scope
		b.scope = &copied
		return nil
	}
}

// WithSupersedes sets the ids this record replaces (spec §93, §94). It replaces
// any previously set list rather than appending to it.
func WithSupersedes(ids ...string) Option {
	return func(b *builder) error {
		b.supersedes = slices.Clone(ids)
		return nil
	}
}

// WithTags sets the record's tags. It replaces any previously set list rather
// than appending to it.
func WithTags(tags ...string) Option {
	return func(b *builder) error {
		b.tags = slices.Clone(tags)
		return nil
	}
}

// checkSupersedes holds the "supersedes" array to what its schema declares —
// every entry matching the id pattern of the record's own kind, no entry
// repeated — and to the one rule the schema cannot express: a record that names
// itself is a closed walk of length one, which spec §95 step 9 calls a cycle.
func (b *builder) checkSupersedes(pattern *regexp.Regexp, ownID string) error {
	seen := make(map[string]struct{}, len(b.supersedes))
	for _, id := range b.supersedes {
		if id == ownID {
			return fmt.Errorf("%w: %s", ErrSupersedesSelf, id)
		}
		if !pattern.MatchString(id) {
			return fmt.Errorf("%w: %q in supersedes, want %s", ErrInvalidID, id, pattern)
		}
		if _, duplicate := seen[id]; duplicate {
			return &DuplicateItemError{Property: "supersedes", Value: id}
		}
		seen[id] = struct{}{}
	}
	return nil
}

// checkTags holds the "tags" array to its schema: each entry has a minimum
// length of one, and the array is uniqueItems.
func (b *builder) checkTags() error {
	seen := make(map[string]struct{}, len(b.tags))
	for index, tag := range b.tags {
		if tag == "" {
			return fmt.Errorf("%w: tags[%d]", &EmptyFieldError{Property: "tags"}, index)
		}
		if _, duplicate := seen[tag]; duplicate {
			return &DuplicateItemError{Property: "tags", Value: tag}
		}
		seen[tag] = struct{}{}
	}
	return nil
}

// checkScope validates an optional scope, and adds the one writer-side rule
// Scope.Validate deliberately leaves out.
//
// Scope.Validate answers spec §95 step 11, whose conditions AC-03.4 fixes, and
// a PROJECT scope carrying a target is not among them — so a record already on
// disk with one is not a finding. Authoring that contradiction is a different
// question, and the schema's own description of "target" answers it: "Absent
// for PROJECT scope".
func (b *builder) checkScope() error {
	if b.scope == nil {
		return nil
	}
	if err := b.scope.Validate(); err != nil {
		return err
	}
	if b.scope.Level == ScopeProject && b.scope.Target != "" {
		return fmt.Errorf("%w: %q", ErrScopeTargetOnProject, b.scope.Target)
	}
	return nil
}
