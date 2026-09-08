package record_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/schemas"
)

// The three signatures design §3 freezes and AC-01.2 grades, asserted by
// assignment to an explicit function type.
//
// A test that merely called these would keep passing after a parameter was
// reordered or a type widened, because the call site would have been updated in
// the same edit. A declared function type cannot be updated in passing: the
// package stops building, which is the loudest failure available.
var (
	_ func(string, time.Time, string, string, ...record.Option) (record.Decision, error)  = record.NewDecision
	_ func(string, time.Time, string, string, ...record.Option) (record.Invariant, error) = record.NewInvariant
	_ func(record.Decision, record.Decision) (record.Decision, record.Decision, error)    = record.Supersede
)

// createdAt is the instant every fixture in this package is stamped with. It is
// fixed rather than time.Now so that a golden comparison is a statement about
// the encoder rather than about the clock.
var createdAt = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

// shippedValidator compiles the embedded schema documents — the same ones spec
// §95 step 5 evaluates records against.
//
// Tests here validate through it rather than by re-reading the JSON, because
// the question they ask is whether a record this package writes is one the
// pipeline accepts. Answering it any other way would let the writer and the
// validator agree only because a test made them.
func shippedValidator(t *testing.T) *schema.Validator {
	t.Helper()
	registry, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded) error = %v, want nil", err)
	}
	validator, err := schema.NewValidator(registry)
	if err != nil {
		t.Fatalf("NewValidator(embedded) error = %v, want nil", err)
	}
	return validator
}

// findingsFor marshals a record and runs step 5 over the bytes.
func findingsFor(t *testing.T, kind schema.RecordKind, value any) []schema.Finding {
	t.Helper()
	document, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshalling %T: %v", value, err)
	}
	return shippedValidator(t).Validate(kind, schema.WriteVersion, document)
}

// describe reduces findings to the instance location and keyword pair, which is
// the fact. Messages are prose and both a right and a wrong answer contain the
// field name, so an assertion on them would not distinguish the two.
func describe(findings []schema.Finding) []string {
	out := make([]string, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding.InstanceLocation+" "+finding.Keyword)
	}
	return out
}

// requireValid fails with the located findings rather than with a boolean, so a
// broken constructor says which property it broke.
func requireValid(t *testing.T, kind schema.RecordKind, value any) {
	t.Helper()
	if findings := findingsFor(t, kind, value); len(findings) != 0 {
		t.Fatalf("step 5 over %T = %v, want no findings", value, describe(findings))
	}
}

// encode is the round-trip encoder AC-01.6 compares against.
//
// SetEscapeHTML(false) is not decoration. A knowledge record is repository
// content that people read and diff (design §1), and the default escaping would
// rewrite an "&" a human typed as "&" every time the record was
// re-serialized. The invariant golden carries both "&" and "<" so that deleting
// this line turns the round-trip red instead of quietly corrupting prose.
func encode(t *testing.T, value any) []byte {
	t.Helper()
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		t.Fatalf("encoding %T: %v", value, err)
	}
	return buffer.Bytes()
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(path.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading golden %s: %v", name, err)
	}
	return data
}

// createdAtOf pulls the serialized timestamp back out of an encoded record.
// AC-01.3 is about the string that reaches the file, and comparing two
// time.Time values would pass on an encoder that wrote the wrong offset.
func createdAtOf(t *testing.T, value any) string {
	t.Helper()
	document, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshalling %T: %v", value, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(document, &fields); err != nil {
		t.Fatalf("decoding %T: %v", value, err)
	}
	raw, ok := fields["created_at"]
	if !ok {
		t.Fatalf("%T marshalled without a created_at member: %s", value, document)
	}
	var stamped string
	if err := json.Unmarshal(raw, &stamped); err != nil {
		t.Fatalf("%T created_at is not a JSON string: %s", value, raw)
	}
	return stamped
}

// TestNewDecisionStampsWhatTheSchemaRequires covers AC-01.2 for a Decision: the
// three fields the caller never supplies are supplied here, and the four that
// arrive as parameters land where the schema names them.
func TestNewDecisionStampsWhatTheSchemaRequires(t *testing.T) {
	decision, err := record.NewDecision("DEC-0001", createdAt, "Compile the shipped schemas", "Validate every record against the document it names.")
	if err != nil {
		t.Fatalf("NewDecision() error = %v, want nil", err)
	}

	if decision.SchemaVersion != schema.WriteVersion {
		t.Errorf("SchemaVersion = %d, want %d", decision.SchemaVersion, schema.WriteVersion)
	}
	if decision.Kind != record.KindDecision {
		t.Errorf("Kind = %q, want %q", decision.Kind, record.KindDecision)
	}
	if decision.Status != record.StatusActive {
		t.Errorf("Status = %q, want %q", decision.Status, record.StatusActive)
	}
	if decision.ID != "DEC-0001" {
		t.Errorf("ID = %q, want %q", decision.ID, "DEC-0001")
	}
	if decision.Title != "Compile the shipped schemas" {
		t.Errorf("Title = %q, want the third argument", decision.Title)
	}
	if decision.Decision != "Validate every record against the document it names." {
		t.Errorf("Decision = %q, want the fourth argument", decision.Decision)
	}
	// A Decision's scope is optional, so the absence of WithScope must leave it
	// absent rather than produce an empty object the schema would reject on
	// $defs/scope's required "level".
	if decision.Scope != nil {
		t.Errorf("Scope = %+v, want nil when no WithScope was passed", decision.Scope)
	}
	requireValid(t, schema.KindDecision, decision)
}

// TestNewInvariantStampsWhatTheSchemaRequires covers AC-01.2 for an Invariant,
// including the two required properties that have no parameter and arrive as
// options.
func TestNewInvariantStampsWhatTheSchemaRequires(t *testing.T) {
	scope := record.Scope{Level: record.ScopeFile, Target: "internal/knowledge/schema/validator.go"}
	invariant, err := record.NewInvariant("INV-0001", createdAt, "", "Every record validates against its schema.",
		record.WithSeverity(record.SeverityHigh), record.WithScope(scope))
	if err != nil {
		t.Fatalf("NewInvariant() error = %v, want nil", err)
	}

	if invariant.SchemaVersion != schema.WriteVersion {
		t.Errorf("SchemaVersion = %d, want %d", invariant.SchemaVersion, schema.WriteVersion)
	}
	if invariant.Kind != record.KindInvariant {
		t.Errorf("Kind = %q, want %q", invariant.Kind, record.KindInvariant)
	}
	if invariant.Status != record.StatusActive {
		t.Errorf("Status = %q, want %q", invariant.Status, record.StatusActive)
	}
	if invariant.Statement != "Every record validates against its schema." {
		t.Errorf("Statement = %q, want the fourth argument", invariant.Statement)
	}
	if invariant.Severity != record.SeverityHigh {
		t.Errorf("Severity = %q, want %q", invariant.Severity, record.SeverityHigh)
	}
	if invariant.Scope != scope {
		t.Errorf("Scope = %+v, want %+v", invariant.Scope, scope)
	}
	requireValid(t, schema.KindInvariant, invariant)
}

// TestConstructorsSerializeCreatedAtAsUTCRFC3339 holds AC-01.3.
//
// It asserts the serialized string, not the time.Time: two time.Time values
// denoting the same instant in different locations compare equal under
// Time.Equal and unequal under ==, and neither tells you what reached the file.
// The fixture is deliberately three hours east of UTC, and the guard below
// fails the test if that ever stops being true — a fixture that was already UTC
// would leave this asserting nothing at all.
func TestConstructorsSerializeCreatedAtAsUTCRFC3339(t *testing.T) {
	east := time.FixedZone("UTC+3", 3*60*60)
	at := time.Date(2026, time.January, 2, 6, 4, 5, 0, east)
	const want = "2026-01-02T03:04:05Z"

	if at.Format(time.RFC3339) == want {
		t.Fatal("the fixture is already UTC, so this test would assert nothing")
	}

	decision, err := record.NewDecision("DEC-0001", at, "t", "d")
	if err != nil {
		t.Fatalf("NewDecision() error = %v, want nil", err)
	}
	invariant, err := record.NewInvariant("INV-0001", at, "", "s",
		record.WithSeverity(record.SeverityLow), record.WithScope(record.Scope{Level: record.ScopeProject}))
	if err != nil {
		t.Fatalf("NewInvariant() error = %v, want nil", err)
	}

	tests := []struct {
		name  string
		kind  schema.RecordKind
		value any
	}{
		{name: "decision", kind: schema.KindDecision, value: decision},
		{name: "invariant", kind: schema.KindInvariant, value: invariant},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createdAtOf(t, tt.value)
			if got != want {
				t.Fatalf("serialized created_at = %q, want %q", got, want)
			}
			// Parsing it back proves the string is RFC 3339 rather than merely
			// equal to a literal this test also wrote.
			parsed, err := time.Parse(time.RFC3339, got)
			if err != nil {
				t.Fatalf("serialized created_at %q does not parse as RFC 3339: %v", got, err)
			}
			if !parsed.Equal(at) {
				t.Errorf("serialized created_at %q denotes %v, want the same instant as %v", got, parsed, at)
			}
			// The document's "format": "date-time" is asserted rather than
			// annotated (decision D-48), so step 5 is a second, independent
			// reader of the same string.
			requireValid(t, tt.kind, tt.value)
		})
	}
}
