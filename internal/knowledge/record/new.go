package record

import (
	"fmt"
	"regexp"
	"slices"
	"time"
)

// The id patterns, copied verbatim from the "pattern" keyword each kind's
// schema document sets on "id".
//
// Copying a rule that already exists in JSON is the drift decision D-36 warns
// about, and it is done here only because a constructor has to refuse a bad id
// before any schema is in reach — this package depends on nothing else under
// internal/knowledge and holds no compiled validator. The copy is made safe by
// TestIDPatternsAreTheOnesTheSchemaDocumentsDeclare, which reads the two
// patterns back out of the shipped documents and compares the strings, so the
// documents stay the authority and a divergence fails the suite rather than
// producing records one side accepts and the other rejects.
var (
	decisionIDPattern  = regexp.MustCompile(`^DEC-[0-9]{4,}$`)
	invariantIDPattern = regexp.MustCompile(`^INV-[0-9]{4,}$`)
)

// NewDecision builds an active Decision record (spec §53).
//
// The rejection order is fixed so that a caller fixing one complaint meets the
// next in the same sequence on every run: an option naming a property this
// kind's schema does not define is refused first, because passing the wrong
// constructor's option is a different mistake from passing a bad id; then the
// parameters in declaration order; then the option values.
//
// Everything the decision.v1 document requires is either a parameter or
// stamped: schema_version and kind are stamped, status defaults to active
// because a newly written Decision has not been superseded by anything yet, and
// the remaining four arrive as arguments. The optional properties arrive as
// options, and an option belonging to invariant.v1 is refused rather than
// dropped.
func NewDecision(id string, at time.Time, title, decision string, opts ...Option) (Decision, error) {
	settings, err := newBuilder(KindDecision, opts)
	if err != nil {
		return Decision{}, err
	}

	if !decisionIDPattern.MatchString(id) {
		return Decision{}, fmt.Errorf("%w: %q, want %s", ErrInvalidID, id, decisionIDPattern)
	}
	createdAt, err := stampedTime(at)
	if err != nil {
		return Decision{}, err
	}
	if title == "" {
		return Decision{}, &EmptyFieldError{Property: "title"}
	}
	if decision == "" {
		return Decision{}, &EmptyFieldError{Property: "decision"}
	}

	if err := settings.checkSupersedes(decisionIDPattern, id); err != nil {
		return Decision{}, err
	}
	// A Decision's scope is optional (decision.v1 does not list it in
	// "required"), so a nil one is the normal case and checkScope says so.
	if err := settings.checkScope(); err != nil {
		return Decision{}, err
	}
	if err := settings.checkTags(); err != nil {
		return Decision{}, err
	}

	return Decision{
		SchemaVersion: writeVersion,
		Kind:          KindDecision,
		ID:            id,
		Status:        StatusActive,
		CreatedAt:     createdAt,
		Title:         title,
		Context:       settings.context,
		Decision:      decision,
		Consequences:  settings.consequences,
		// Handed over rather than cloned again. WithSupersedes, WithTags and
		// WithScope already copy what the caller passed — that is the one
		// boundary external data crosses — and the builder dies with this call,
		// so nothing else holds a reference. A second clone here would be a
		// guard no mutation could falsify, and unfalsifiable guards are what
		// MR-001's second audit round was spent removing.
		Supersedes: settings.supersedes,
		Scope:      settings.scope,
		Tags:       settings.tags,
	}, nil
}

// NewInvariant builds an active Invariant record (spec §54).
//
// The third parameter is "statement", and there is no "title" parameter, which
// is a change to the signature AC-01.2 originally froze. The reason is that the
// frozen form contradicted the frozen document beside it. AC-01.1 says the
// Invariant mirrors invariant.v1.schema.json; that document declares no "title"
// property and sets additionalProperties:false, so a title has nowhere to go.
// The first implementation resolved the contradiction by accepting the
// parameter and then refusing every value it could hold except "", which left a
// published constructor carrying an argument that could never be anything —
// zero bits of information, and a compiler-enforced lie about what the caller
// may supply. Both halves of AC-01.2 could not stand, and the half derived from
// the schema document wins, because D-36 makes the document the contract.
// AC-01.2 in docs/engineering/mr-002-requirements.md was amended to match, and
// the amendment says so in its own words rather than silently.
//
// The rejection order matches NewDecision's: refused options first, then the
// parameters in declaration order, then the option values.
//
// Two properties invariant.v1 marks required have no parameter either: "scope"
// and "severity" arrive through WithScope and WithSeverity, and their absence
// is an error rather than a default. Defaulting a severity would have this
// constructor decide how hard validation pushes back on a change (spec §51),
// which is the author's judgement and not a writer's.
func NewInvariant(id string, at time.Time, statement string, opts ...Option) (Invariant, error) {
	settings, err := newBuilder(KindInvariant, opts)
	if err != nil {
		return Invariant{}, err
	}

	if !invariantIDPattern.MatchString(id) {
		return Invariant{}, fmt.Errorf("%w: %q, want %s", ErrInvalidID, id, invariantIDPattern)
	}
	createdAt, err := stampedTime(at)
	if err != nil {
		return Invariant{}, err
	}
	if statement == "" {
		return Invariant{}, &EmptyFieldError{Property: "statement"}
	}

	if !settings.severitySet {
		return Invariant{}, ErrSeverityMissing
	}
	if !slices.Contains(severities, settings.severity) {
		return Invariant{}, fmt.Errorf("%w: %q is not one of %v", ErrSeverityUnknown, settings.severity, severities)
	}
	if settings.scope == nil {
		return Invariant{}, ErrScopeMissing
	}

	if err := settings.checkSupersedes(invariantIDPattern, id); err != nil {
		return Invariant{}, err
	}
	if err := settings.checkScope(); err != nil {
		return Invariant{}, err
	}
	if err := settings.checkTags(); err != nil {
		return Invariant{}, err
	}

	return Invariant{
		SchemaVersion: writeVersion,
		Kind:          KindInvariant,
		ID:            id,
		Status:        StatusActive,
		CreatedAt:     createdAt,
		Statement:     statement,
		Rationale:     settings.rationale,
		Severity:      settings.severity,
		// See NewDecision: the builder's copies are already private.
		Supersedes: settings.supersedes,
		Scope:      *settings.scope,
		Tags:       settings.tags,
	}, nil
}

// stampedTime converts a caller's instant into the one spelling that may be
// persisted.
//
// tech-stack §42: all persisted timestamps are UTC. The conversion belongs here
// rather than at the encoder because time.Time carries its location and
// MarshalJSON writes whatever offset it finds — a record built from a
// 03:04:05+03:00 value would otherwise be committed carrying that offset, and
// every consumer that compares or sorts the serialized strings would order it
// against a UTC sibling wrongly.
func stampedTime(at time.Time) (time.Time, error) {
	if at.IsZero() {
		return time.Time{}, ErrZeroCreatedAt
	}
	return at.UTC(), nil
}
