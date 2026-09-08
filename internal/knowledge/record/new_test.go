package record_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// propertyOf extracts the JSON property name a typed construction error names.
//
// Reading it structurally is the whole point. Every one of these errors also
// mentions the property in its message, so an assertion on the message would
// pass for a constructor that named the wrong field for the right reason —
// which is the substring trap MR-001 paid for.
func propertyOf(t *testing.T, err error) string {
	t.Helper()

	var empty *record.EmptyFieldError
	if errors.As(err, &empty) {
		return empty.Property
	}
	var mismatch *record.OptionKindError
	if errors.As(err, &mismatch) {
		return mismatch.Property
	}
	var duplicate *record.DuplicateItemError
	if errors.As(err, &duplicate) {
		return duplicate.Property
	}

	t.Fatalf("error %v carries no property name; want one of *EmptyFieldError, *OptionKindError, *DuplicateItemError", err)
	return ""
}

// TestNewDecisionRefusesWhatItsSchemaWouldReject walks every way a Decision can
// be malformed at construction.
//
// Each row asserts a sentinel with errors.Is and, where the error is typed, the
// property name it carries. Neither is a substring of a message: a constructor
// that refused the right record for the wrong reason fails here.
func TestNewDecisionRefusesWhatItsSchemaWouldReject(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		at           time.Time
		title        string
		decision     string
		opts         []record.Option
		wantErr      error
		wantProperty string
	}{
		{
			name: "id with too few digits", id: "DEC-1", at: createdAt, title: "t", decision: "d",
			wantErr: record.ErrInvalidID,
		},
		{
			name: "id in the wrong case", id: "dec-0001", at: createdAt, title: "t", decision: "d",
			wantErr: record.ErrInvalidID,
		},
		{
			name: "id carrying an invariant's prefix", id: "INV-0001", at: createdAt, title: "t", decision: "d",
			wantErr: record.ErrInvalidID,
		},
		{
			name: "empty id", id: "", at: createdAt, title: "t", decision: "d",
			wantErr: record.ErrInvalidID,
		},
		{
			name: "zero created_at", id: "DEC-0001", at: time.Time{}, title: "t", decision: "d",
			wantErr: record.ErrZeroCreatedAt,
		},
		{
			name: "empty title", id: "DEC-0001", at: createdAt, title: "", decision: "d",
			wantErr: record.ErrEmptyField, wantProperty: "title",
		},
		{
			name: "empty decision", id: "DEC-0001", at: createdAt, title: "t", decision: "",
			wantErr: record.ErrEmptyField, wantProperty: "decision",
		},
		{
			name: "superseding itself", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithSupersedes("DEC-0001")},
			wantErr: record.ErrSupersedesSelf,
		},
		{
			name: "supersede target with a malformed id", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithSupersedes("DEC-2")},
			wantErr: record.ErrInvalidID,
		},
		{
			name: "supersede target repeated", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithSupersedes("DEC-0002", "DEC-0002")},
			wantErr: record.ErrDuplicateItem, wantProperty: "supersedes",
		},
		{
			name: "empty tag", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithTags("knowledge", "")},
			wantErr: record.ErrEmptyField, wantProperty: "tags",
		},
		{
			name: "tag repeated", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithTags("knowledge", "knowledge")},
			wantErr: record.ErrDuplicateItem, wantProperty: "tags",
		},
		{
			name: "scope level outside the five", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithScope(record.Scope{Level: "GALAXY", Target: "internal"})},
			wantErr: record.ErrScopeLevelUnknown,
		},
		{
			name: "scope naming nothing below PROJECT", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithScope(record.Scope{Level: record.ScopeFile})},
			wantErr: record.ErrScopeTargetMissing,
		},
		{
			name: "PROJECT scope carrying a target", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithScope(record.Scope{Level: record.ScopeProject, Target: "internal"})},
			wantErr: record.ErrScopeTargetOnProject,
		},
		{
			name: "scope target spelled with backslashes", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithScope(record.Scope{Level: record.ScopeFile, Target: `internal\app\code.go`})},
			wantErr: record.ErrScopeTargetNotRepoRelative,
		},
		{
			name: "absolute scope target", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithScope(record.Scope{Level: record.ScopeFile, Target: "/etc/passwd"})},
			wantErr: record.ErrScopeTargetNotRepoRelative,
		},
		{
			name: "scope target leaving the repository", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithScope(record.Scope{Level: record.ScopeFile, Target: "../secrets/key.pem"})},
			wantErr: record.ErrScopeTargetNotRepoRelative,
		},
		{
			name: "unnormalized scope target", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithScope(record.Scope{Level: record.ScopeFile, Target: "./internal//app.go"})},
			wantErr: record.ErrScopeTargetNotRepoRelative,
		},
		{
			name: "drive-rooted scope target", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithScope(record.Scope{Level: record.ScopeFile, Target: "C:/repo/internal/app.go"})},
			wantErr: record.ErrScopeTargetNotRepoRelative,
		},
		{
			name: "an invariant's severity", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithSeverity(record.SeverityHigh)},
			wantErr: record.ErrOptionKindMismatch, wantProperty: "severity",
		},
		{
			name: "an invariant's rationale", id: "DEC-0001", at: createdAt, title: "t", decision: "d",
			opts:    []record.Option{record.WithRationale("because")},
			wantErr: record.ErrOptionKindMismatch, wantProperty: "rationale",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			built, err := record.NewDecision(tt.id, tt.at, tt.title, tt.decision, tt.opts...)
			if err == nil {
				t.Fatalf("NewDecision() error = nil, want %v (built %+v)", tt.wantErr, built)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewDecision() error = %v, want one matching %v", err, tt.wantErr)
			}
			// A constructor that returned a half-built record alongside its
			// error invites a caller who checked neither to write it.
			if !reflect.DeepEqual(built, record.Decision{}) {
				t.Errorf("NewDecision() returned %+v alongside its error, want the zero Decision", built)
			}
			if tt.wantProperty != "" {
				if got := propertyOf(t, err); got != tt.wantProperty {
					t.Errorf("NewDecision() error names property %q, want %q", got, tt.wantProperty)
				}
			}
		})
	}
}

// TestNewInvariantRefusesWhatItsSchemaWouldReject is NewInvariant's half, and
// carries the three refusals that exist only because invariant.v1 differs from
// decision.v1: a required severity, a required scope, and a title with nowhere
// to go.
func TestNewInvariantRefusesWhatItsSchemaWouldReject(t *testing.T) {
	fileScope := record.WithScope(record.Scope{Level: record.ScopeFile, Target: "internal/app/code.go"})
	high := record.WithSeverity(record.SeverityHigh)

	tests := []struct {
		name         string
		id           string
		at           time.Time
		title        string
		statement    string
		opts         []record.Option
		wantErr      error
		wantProperty string
	}{
		{
			name: "id carrying a decision's prefix", id: "DEC-0001", at: createdAt, statement: "s",
			opts: []record.Option{high, fileScope}, wantErr: record.ErrInvalidID,
		},
		{
			name: "zero created_at", id: "INV-0001", at: time.Time{}, statement: "s",
			opts: []record.Option{high, fileScope}, wantErr: record.ErrZeroCreatedAt,
		},
		{
			name: "a title the schema has nowhere to put", id: "INV-0001", at: createdAt, title: "A title", statement: "s",
			opts: []record.Option{high, fileScope}, wantErr: record.ErrInvariantHasNoTitle,
		},
		{
			name: "empty statement", id: "INV-0001", at: createdAt, statement: "",
			opts: []record.Option{high, fileScope}, wantErr: record.ErrEmptyField, wantProperty: "statement",
		},
		{
			name: "no severity", id: "INV-0001", at: createdAt, statement: "s",
			opts: []record.Option{fileScope}, wantErr: record.ErrSeverityMissing,
		},
		{
			name: "an empty severity, which is not the same as none", id: "INV-0001", at: createdAt, statement: "s",
			opts: []record.Option{record.WithSeverity(""), fileScope}, wantErr: record.ErrSeverityUnknown,
		},
		{
			name: "a severity outside the enum", id: "INV-0001", at: createdAt, statement: "s",
			opts: []record.Option{record.WithSeverity("URGENT"), fileScope}, wantErr: record.ErrSeverityUnknown,
		},
		{
			name: "no scope", id: "INV-0001", at: createdAt, statement: "s",
			opts: []record.Option{high}, wantErr: record.ErrScopeMissing,
		},
		{
			name: "scope naming nothing below PROJECT", id: "INV-0001", at: createdAt, statement: "s",
			opts:    []record.Option{high, record.WithScope(record.Scope{Level: record.ScopeSymbol})},
			wantErr: record.ErrScopeTargetMissing,
		},
		{
			name: "superseding itself", id: "INV-0001", at: createdAt, statement: "s",
			opts:    []record.Option{high, fileScope, record.WithSupersedes("INV-0001")},
			wantErr: record.ErrSupersedesSelf,
		},
		{
			name: "supersede target carrying a decision's prefix", id: "INV-0001", at: createdAt, statement: "s",
			opts:    []record.Option{high, fileScope, record.WithSupersedes("DEC-0002")},
			wantErr: record.ErrInvalidID,
		},
		{
			name: "a decision's context", id: "INV-0001", at: createdAt, statement: "s",
			opts:    []record.Option{high, fileScope, record.WithContext("the situation")},
			wantErr: record.ErrOptionKindMismatch, wantProperty: "context",
		},
		{
			name: "a decision's consequences", id: "INV-0001", at: createdAt, statement: "s",
			opts:    []record.Option{high, fileScope, record.WithConsequences("the cost")},
			wantErr: record.ErrOptionKindMismatch, wantProperty: "consequences",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			built, err := record.NewInvariant(tt.id, tt.at, tt.title, tt.statement, tt.opts...)
			if err == nil {
				t.Fatalf("NewInvariant() error = nil, want %v (built %+v)", tt.wantErr, built)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewInvariant() error = %v, want one matching %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(built, record.Invariant{}) {
				t.Errorf("NewInvariant() returned %+v alongside its error, want the zero Invariant", built)
			}
			if tt.wantProperty != "" {
				if got := propertyOf(t, err); got != tt.wantProperty {
					t.Errorf("NewInvariant() error names property %q, want %q", got, tt.wantProperty)
				}
			}
		})
	}
}

// TestConstructorsAcceptEveryShapeTheirSchemasAllow is the over-fire guard for
// both tables above: each row is the healthy neighbour of a refusal, and every
// accepted record is then put through step 5, so "accepted" means the shipped
// document agrees rather than only that this package did.
func TestConstructorsAcceptEveryShapeTheirSchemasAllow(t *testing.T) {
	t.Run("decision", func(t *testing.T) {
		tests := []struct {
			name string
			id   string
			opts []record.Option
		}{
			{name: "no options at all", id: "DEC-0001"},
			{name: "an id longer than four digits", id: "DEC-000042"},
			{name: "two distinct supersede targets", id: "DEC-0001",
				opts: []record.Option{record.WithSupersedes("DEC-0002", "DEC-0003")}},
			{name: "two distinct tags", id: "DEC-0001",
				opts: []record.Option{record.WithTags("knowledge", "validation")}},
			{name: "a PROJECT scope with no target", id: "DEC-0001",
				opts: []record.Option{record.WithScope(record.Scope{Level: record.ScopeProject})}},
			{name: "a FILE scope with a repo-relative target", id: "DEC-0001",
				opts: []record.Option{record.WithScope(record.Scope{Level: record.ScopeFile, Target: "internal/app/code.go"})}},
			{name: "a SYMBOL scope naming a symbol", id: "DEC-0001",
				opts: []record.Option{record.WithScope(record.Scope{Level: record.ScopeSymbol, Target: "internal/app.Code"})}},
			{name: "a PACKAGE scope", id: "DEC-0001",
				opts: []record.Option{record.WithScope(record.Scope{Level: record.ScopePackage, Target: "internal/knowledge"})}},
			{name: "its own optional prose", id: "DEC-0001",
				opts: []record.Option{record.WithContext("the situation"), record.WithConsequences("the cost")}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				built, err := record.NewDecision(tt.id, createdAt, "A title", "A decision.", tt.opts...)
				if err != nil {
					t.Fatalf("NewDecision() error = %v, want nil", err)
				}
				requireValid(t, schema.KindDecision, built)
			})
		}
	})

	t.Run("invariant", func(t *testing.T) {
		projectScope := record.WithScope(record.Scope{Level: record.ScopeProject})

		tests := []struct {
			name string
			id   string
			opts []record.Option
		}{
			{name: "only what its schema requires", id: "INV-0001",
				opts: []record.Option{record.WithSeverity(record.SeverityLow), projectScope}},
			{name: "an id longer than four digits", id: "INV-000042",
				opts: []record.Option{record.WithSeverity(record.SeverityLow), projectScope}},
			{name: "its own optional prose", id: "INV-0001",
				opts: []record.Option{record.WithSeverity(record.SeverityMedium), projectScope, record.WithRationale("why it matters")}},
			{name: "two distinct supersede targets", id: "INV-0001",
				opts: []record.Option{record.WithSeverity(record.SeverityHigh), projectScope, record.WithSupersedes("INV-0002", "INV-0003")}},
			{name: "the most severe level", id: "INV-0001",
				opts: []record.Option{record.WithSeverity(record.SeverityCritical), projectScope, record.WithTags("knowledge")}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				built, err := record.NewInvariant(tt.id, createdAt, "", "A statement.", tt.opts...)
				if err != nil {
					t.Fatalf("NewInvariant() error = %v, want nil", err)
				}
				requireValid(t, schema.KindInvariant, built)
			})
		}
	})
}

// TestNewInvariantRefusesATitleItHasNowhereToPut is the resolution of the one
// place the frozen signature and the frozen schema disagree, written as a test
// so the resolution is visible rather than buried in a doc comment.
//
// invariant.v1 declares no "title" and sets additionalProperties:false. Storing
// the argument would make every record this constructor produced fail step 5;
// dropping it would lose the caller's data in silence. The constructor refuses,
// and the fourth parameter — the statement — is where the prose belongs.
func TestNewInvariantRefusesATitleItHasNowhereToPut(t *testing.T) {
	severity := record.WithSeverity(record.SeverityHigh)
	scope := record.WithScope(record.Scope{Level: record.ScopeProject})

	if _, err := record.NewInvariant("INV-0001", createdAt, "A title", "A statement.", severity, scope); !errors.Is(err, record.ErrInvariantHasNoTitle) {
		t.Fatalf("NewInvariant() with a title: error = %v, want one matching ErrInvariantHasNoTitle", err)
	}

	// The over-fire guard: an empty title is the normal call, and the fourth
	// parameter reaches "statement" rather than being swallowed with the third.
	built, err := record.NewInvariant("INV-0001", createdAt, "", "A statement.", severity, scope)
	if err != nil {
		t.Fatalf("NewInvariant() without a title: error = %v, want nil", err)
	}
	if built.Statement != "A statement." {
		t.Errorf("Statement = %q, want the fourth argument", built.Statement)
	}
	requireValid(t, schema.KindInvariant, built)
}

// TestOptionsAreRefusedByTheKindWhoseSchemaLacksTheProperty pairs each refusal
// with the acceptance that proves the refusal is about the kind and not about
// the option being broken.
//
// Refusing rather than ignoring matters because both documents set
// additionalProperties:false: the value could never have been written, so a
// silently dropped option would leave the caller believing a field they set is
// in the record.
func TestOptionsAreRefusedByTheKindWhoseSchemaLacksTheProperty(t *testing.T) {
	tests := []struct {
		name     string
		option   record.Option
		property string
		// acceptedBy names the kind whose schema does define the property; the
		// other kind must refuse it.
		acceptedByDecision bool
	}{
		{name: "context", option: record.WithContext("c"), property: "context", acceptedByDecision: true},
		{name: "consequences", option: record.WithConsequences("c"), property: "consequences", acceptedByDecision: true},
		{name: "rationale", option: record.WithRationale("r"), property: "rationale"},
		{name: "severity", option: record.WithSeverity(record.SeverityLow), property: "severity"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decisionErr := func() error {
				_, err := record.NewDecision("DEC-0001", createdAt, "t", "d", tt.option)
				return err
			}()
			invariantErr := func() error {
				_, err := record.NewInvariant("INV-0001", createdAt, "", "s", tt.option,
					record.WithSeverity(record.SeverityLow), record.WithScope(record.Scope{Level: record.ScopeProject}))
				return err
			}()

			refused, accepted := invariantErr, decisionErr
			refusedBy := record.KindInvariant
			if !tt.acceptedByDecision {
				refused, accepted = decisionErr, invariantErr
				refusedBy = record.KindDecision
			}

			if !errors.Is(refused, record.ErrOptionKindMismatch) {
				t.Fatalf("%s applied to a %s: error = %v, want one matching ErrOptionKindMismatch", tt.name, refusedBy, refused)
			}
			if got := propertyOf(t, refused); got != tt.property {
				t.Errorf("%s refusal names property %q, want %q", tt.name, got, tt.property)
			}
			var mismatch *record.OptionKindError
			if errors.As(refused, &mismatch) && mismatch.Kind != refusedBy {
				t.Errorf("%s refusal names kind %q, want %q", tt.name, mismatch.Kind, refusedBy)
			}
			if accepted != nil {
				t.Errorf("%s applied to the kind whose schema defines it: error = %v, want nil", tt.name, accepted)
			}
		})
	}
}

// TestConstructorsCopyEverythingTheCallerStillHolds is the immutability rule
// applied where it is also correctness: a record must not share a slice or a
// scope with the caller who supplied it, or a later edit on either side rewrites
// a record that was already built and committed to.
func TestConstructorsCopyEverythingTheCallerStillHolds(t *testing.T) {
	supersedes := []string{"DEC-0002"}
	tags := []string{"knowledge"}
	scope := record.Scope{Level: record.ScopeFile, Target: "internal/app/code.go"}

	built, err := record.NewDecision("DEC-0001", createdAt, "t", "d",
		record.WithSupersedes(supersedes...), record.WithTags(tags...), record.WithScope(scope))
	if err != nil {
		t.Fatalf("NewDecision() error = %v, want nil", err)
	}

	supersedes[0] = "DEC-9999"
	tags[0] = "rewritten"
	scope.Target = "somewhere/else.go"

	if built.Supersedes[0] != "DEC-0002" {
		t.Errorf("Supersedes[0] = %q after the caller's slice was edited, want %q", built.Supersedes[0], "DEC-0002")
	}
	if built.Tags[0] != "knowledge" {
		t.Errorf("Tags[0] = %q after the caller's slice was edited, want %q", built.Tags[0], "knowledge")
	}
	if built.Scope == nil || built.Scope.Target != "internal/app/code.go" {
		t.Errorf("Scope = %+v after the caller's Scope was edited, want target %q", built.Scope, "internal/app/code.go")
	}

	// The second half is about reusing one Option, which is the case that
	// actually aliases. An Option is a value a caller keeps and passes to
	// several constructions; each of them closes over the same captured
	// parameters, so an option that handed out the address of its capture — or
	// the caller's slice — would give every record it built one shared scope
	// and one shared array. Building two records from *the same three Option
	// values* is what makes that reachable; two separate WithScope calls would
	// pass whatever the implementation did.
	shared := []record.Option{
		record.WithScope(record.Scope{Level: record.ScopeFile, Target: "internal/app/code.go"}),
		record.WithSupersedes("DEC-0007"),
		record.WithTags("knowledge"),
	}
	first, err := record.NewDecision("DEC-0001", createdAt, "t", "d", shared...)
	if err != nil {
		t.Fatalf("NewDecision() error = %v, want nil", err)
	}
	second, err := record.NewDecision("DEC-0002", createdAt, "t", "d", shared...)
	if err != nil {
		t.Fatalf("NewDecision() error = %v, want nil", err)
	}

	first.Scope.Target = "rewritten/by/the/first.go"
	first.Supersedes[0] = "DEC-9999"
	first.Tags[0] = "rewritten"

	if second.Scope.Target != "internal/app/code.go" {
		t.Errorf("second record Scope.Target = %q after the first was edited, want %q", second.Scope.Target, "internal/app/code.go")
	}
	if second.Supersedes[0] != "DEC-0007" {
		t.Errorf("second record Supersedes[0] = %q after the first was edited, want %q", second.Supersedes[0], "DEC-0007")
	}
	if second.Tags[0] != "knowledge" {
		t.Errorf("second record Tags[0] = %q after the first was edited, want %q", second.Tags[0], "knowledge")
	}
}
