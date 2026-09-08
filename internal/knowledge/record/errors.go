package record

import (
	"errors"
	"fmt"
)

// The construction and transition failures, one sentinel per condition.
//
// They are sentinels rather than messages because control flow must never turn
// on an error string (tech-stack §72), and because a test that tells two
// rejections apart by their prose is asserting the prose: the right and the
// wrong answer both mention the field. Every one of them stays reachable with
// errors.Is through the fmt.Errorf %w wrapping below.
var (
	// ErrInvalidID reports an id that does not match the pattern its kind's
	// schema document declares.
	ErrInvalidID = errors.New("knowledge record: id does not match the pattern its schema declares")

	// ErrZeroCreatedAt reports a record built from the zero time.
	//
	// The schema would accept it — "0001-01-01T00:00:00Z" is a well-formed
	// date-time — which is exactly why the constructor refuses it. A record
	// that dates itself to year one is a caller who forgot the clock, and
	// writing it would put a lie in repository content that no later validation
	// can detect.
	ErrZeroCreatedAt = errors.New("knowledge record: created_at is the zero time, so the record claims no creation instant")

	// ErrEmptyField reports a property whose schema gives it "minLength": 1 and
	// which the caller left empty. Use errors.As with *EmptyFieldError to learn
	// which property it was.
	ErrEmptyField = errors.New("knowledge record: a property its schema gives a minimum length of one was left empty")

	// ErrOptionKindMismatch reports an Option naming a property the record kind
	// it was passed to does not have. Use errors.As with *OptionKindError.
	//
	// Refusing beats ignoring. Both schema documents set
	// additionalProperties:false, so the value could not have been written; a
	// silently dropped option would leave the caller believing a field they set
	// is in the record.
	ErrOptionKindMismatch = errors.New("knowledge record: option names a property this record kind's schema does not define")

	// ErrDuplicateItem reports a repeated entry in an array the schema marks
	// "uniqueItems": true. Use errors.As with *DuplicateItemError.
	ErrDuplicateItem = errors.New("knowledge record: an array its schema marks uniqueItems repeats a value")

	// ErrScopeLevelUnknown reports a scope level outside the five spec §54
	// defines.
	ErrScopeLevelUnknown = errors.New("knowledge record: scope level is not one of the five the schema declares")

	// ErrScopeTargetMissing reports a non-PROJECT scope that names nothing.
	ErrScopeTargetMissing = errors.New("knowledge record: scope target is required for every level but PROJECT")

	// ErrScopeTargetOnProject reports a PROJECT scope carrying a target.
	//
	// This is a writer-side rule, stricter than spec §95 step 11: AC-03.4 does
	// not list it among the pipeline's conditions, so Scope.Validate does not
	// refuse it and a record already on disk that carries one is not a finding.
	// A constructor may still refuse to author the contradiction — the schema's
	// own description of target says "Absent for PROJECT scope".
	ErrScopeTargetOnProject = errors.New("knowledge record: a PROJECT scope applies everywhere, so it cannot also name a target")

	// ErrScopeTargetNotRepoRelative reports a target that is not the
	// repository-relative forward-slash spelling tech-stack §74 mandates.
	ErrScopeTargetNotRepoRelative = errors.New("knowledge record: scope target is not a repository-relative forward-slash path")

	// ErrScopeMissing reports an Invariant built without a scope. Its schema
	// requires one, and a record whose own writer produced something the schema
	// rejects is the defect design §5's writer/validator pairing exists to
	// catch.
	ErrScopeMissing = errors.New("knowledge record: an invariant's schema requires a scope, so WithScope is not optional here")

	// ErrSeverityMissing reports an Invariant built without a severity, which
	// its schema also requires.
	ErrSeverityMissing = errors.New("knowledge record: an invariant's schema requires a severity, so WithSeverity is not optional here")

	// ErrSeverityUnknown reports a severity outside the four the schema's enum
	// declares.
	ErrSeverityUnknown = errors.New("knowledge record: severity is not one of the four the schema declares")

	// ErrInvariantHasNoTitle reports a non-empty title handed to NewInvariant.
	//
	// invariant.v1 declares no "title" property and sets
	// additionalProperties:false, so there is nowhere to put the value: writing
	// it would make every such record invalid, and dropping it would lose the
	// caller's data without saying so. The frozen signature (design §3,
	// AC-01.2) keeps the parameter, so the constructor refuses instead. What
	// the caller almost always means is the statement, which is the fourth
	// parameter.
	ErrInvariantHasNoTitle = errors.New("knowledge record: invariant.v1 declares no title property, so NewInvariant has nowhere to put one")

	// ErrSupersedesSelf reports a record that supersedes itself, whether
	// through Supersede or through WithSupersedes.
	//
	// The schema permits it — "supersedes" is only a unique array of well-formed
	// ids — but a self-reference is a closed walk of length one, which spec §95
	// step 9 calls a supersede cycle and decision D-39 makes fatal. Authoring
	// one would hand the repository a condition that blocks it.
	ErrSupersedesSelf = errors.New("knowledge record: a record cannot supersede itself, which would be a supersede cycle of length one")

	// ErrPriorAlreadySuperseded reports a Supersede whose prior record has
	// already been superseded once. Lineage is a chain: superseding the same
	// record twice forks it, and then "which decision is current" has two
	// answers.
	ErrPriorAlreadySuperseded = errors.New("knowledge record: the prior decision is already superseded, so superseding it again would fork its lineage")

	// ErrPriorNotActive reports a Supersede whose prior record carries neither
	// status the schema's enum declares — a zero Decision, or one built by
	// something other than these constructors.
	ErrPriorNotActive = errors.New("knowledge record: the prior decision does not carry a status the schema declares, so its lineage cannot be reasoned about")
)

// EmptyFieldError names the property that was left empty.
//
// It is a type rather than one sentinel per property so that a test can assert
// *which* property was refused without matching on a message — the message
// names the property whether or not the code checked the right one — and so
// that a schema growing a fourth minLength property does not grow a fourth
// sentinel.
type EmptyFieldError struct {
	// Property is the JSON property name as the schema document spells it.
	Property string
}

func (e *EmptyFieldError) Error() string {
	return fmt.Sprintf("knowledge record: %q must not be empty; its schema gives it a minimum length of one", e.Property)
}

// Unwrap makes errors.Is(err, ErrEmptyField) true for every property, so a
// caller that only cares about the class does not have to enumerate them.
func (e *EmptyFieldError) Unwrap() error { return ErrEmptyField }

// OptionKindError names the option and the record kind that refused it.
type OptionKindError struct {
	// Property is the JSON property name the option would have set.
	Property string
	// Kind is the record kind the option was applied to.
	Kind string
}

func (e *OptionKindError) Error() string {
	return fmt.Sprintf("knowledge record: %q is not a property of a %s record's schema", e.Property, e.Kind)
}

func (e *OptionKindError) Unwrap() error { return ErrOptionKindMismatch }

// DuplicateItemError names the array and the value that repeated in it.
type DuplicateItemError struct {
	// Property is the JSON property name of the array.
	Property string
	// Value is the entry that appeared more than once.
	Value string
}

func (e *DuplicateItemError) Error() string {
	return fmt.Sprintf("knowledge record: %q repeats %q, and its schema marks the array uniqueItems", e.Property, e.Value)
}

func (e *DuplicateItemError) Unwrap() error { return ErrDuplicateItem }
