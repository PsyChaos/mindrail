package record_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// The two golden documents, one per kind (AC-01.6).
//
// They are hand-written from the schema documents and there is deliberately no
// -update flag anywhere in this package. A regenerated golden would be the
// struct describing itself, and every assertion below would then pass through
// any tag change at all — which is the one failure the round-trip exists to
// catch. When a golden has to change, edit it by hand and read the diff.
const (
	decisionGolden  = "decision.v1.golden.json"
	invariantGolden = "invariant.v1.golden.json"
)

// TestGoldenRecordsRoundTripByteForByte holds AC-01.6.
//
// Decode the golden into the struct, re-encode it, compare bytes. A field
// dropped by a misspelled tag decodes to its zero value and re-encodes as
// absent or empty; a renamed tag writes a member the golden does not have.
// Either way the bytes differ.
//
// The two goldens are shaped to make the failure reachable rather than
// theoretical: the decision's scope is PROJECT with no target, so deleting the
// ",omitempty" from Scope.Target adds a `"target": ""` member and turns this
// red, while the invariant's scope carries a target so the field is exercised
// in both states across the pair.
func TestGoldenRecordsRoundTripByteForByte(t *testing.T) {
	t.Run("decision", func(t *testing.T) {
		golden := readGolden(t, decisionGolden)

		var decoded record.Decision
		if err := json.Unmarshal(golden, &decoded); err != nil {
			t.Fatalf("decoding %s: %v", decisionGolden, err)
		}
		if got := encode(t, decoded); !bytes.Equal(got, golden) {
			t.Fatalf("re-encoded %s differs from the golden\n got:\n%s\nwant:\n%s", decisionGolden, got, golden)
		}
	})

	t.Run("invariant", func(t *testing.T) {
		golden := readGolden(t, invariantGolden)

		var decoded record.Invariant
		if err := json.Unmarshal(golden, &decoded); err != nil {
			t.Fatalf("decoding %s: %v", invariantGolden, err)
		}
		if got := encode(t, decoded); !bytes.Equal(got, golden) {
			t.Fatalf("re-encoded %s differs from the golden\n got:\n%s\nwant:\n%s", invariantGolden, got, golden)
		}
	})
}

// TestGoldensCarryEveryPropertyTheirSchemaDefines is what stops the round-trip
// above from being vacuous.
//
// A golden that omitted an optional property would round-trip perfectly while
// saying nothing about that property's tag. Requiring the goldens to be
// complete — every member the document defines, present — is what makes "the
// bytes match" mean "no tag is wrong".
func TestGoldensCarryEveryPropertyTheirSchemaDefines(t *testing.T) {
	tests := []struct {
		name   string
		golden string
		// properties are the members decision.v1 and invariant.v1 define. They
		// are listed rather than derived, because a list derived from the
		// struct would go blank at the same moment the struct lost a field.
		properties []string
	}{
		{
			name:   "decision",
			golden: decisionGolden,
			properties: []string{
				"schema_version", "kind", "id", "status", "created_at",
				"title", "context", "decision", "consequences", "supersedes", "scope", "tags",
			},
		},
		{
			name:   "invariant",
			golden: invariantGolden,
			properties: []string{
				"schema_version", "kind", "id", "status", "created_at",
				"statement", "rationale", "severity", "supersedes", "scope", "tags",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var members map[string]json.RawMessage
			if err := json.Unmarshal(readGolden(t, tt.golden), &members); err != nil {
				t.Fatalf("decoding %s: %v", tt.golden, err)
			}
			for _, property := range tt.properties {
				if _, ok := members[property]; !ok {
					t.Errorf("%s has no %q member, so the round-trip says nothing about that tag", tt.golden, property)
				}
			}
			if len(members) != len(tt.properties) {
				t.Errorf("%s has %d members, want exactly the %d its schema defines", tt.golden, len(members), len(tt.properties))
			}
		})
	}
}

// TestGoldenRecordsSatisfyTheirShippedSchemaDocument ties the goldens to the
// contract rather than to this test's encoder.
//
// Without it the round-trip would only prove the struct and the golden agree
// with each other; two files can agree on something the schema rejects. Running
// step 5 over the golden bytes makes the fixture a real record.
func TestGoldenRecordsSatisfyTheirShippedSchemaDocument(t *testing.T) {
	tests := []struct {
		name   string
		golden string
		kind   schema.RecordKind
	}{
		{name: "decision", golden: decisionGolden, kind: schema.KindDecision},
		{name: "invariant", golden: invariantGolden, kind: schema.KindInvariant},
	}

	validator := shippedValidator(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := validator.Validate(tt.kind, schema.WriteVersion, readGolden(t, tt.golden))
			if len(findings) != 0 {
				t.Fatalf("step 5 over %s = %v, want no findings", tt.golden, describe(findings))
			}
		})
	}
}

// TestGoldensAreRecordsTheConstructorsWouldAlsoProduce closes the last gap
// between the fixtures and the writer.
//
// A golden that only the schema accepts, and that NewDecision would refuse,
// would let this package ship a fixture nothing in it can create. Rebuilding
// each golden through the constructors and comparing the encoded bytes proves
// the two paths land on the same record — with the one documented exception
// that a golden may carry a status the constructor never stamps, since
// "superseded" is Supersede's to set.
func TestGoldensAreRecordsTheConstructorsWouldAlsoProduce(t *testing.T) {
	t.Run("decision", func(t *testing.T) {
		golden := readGolden(t, decisionGolden)

		var want record.Decision
		if err := json.Unmarshal(golden, &want); err != nil {
			t.Fatalf("decoding %s: %v", decisionGolden, err)
		}

		built, err := record.NewDecision(want.ID, want.CreatedAt, want.Title, want.Decision,
			record.WithContext(want.Context),
			record.WithConsequences(want.Consequences),
			record.WithSupersedes(want.Supersedes...),
			record.WithScope(*want.Scope),
			record.WithTags(want.Tags...))
		if err != nil {
			t.Fatalf("NewDecision() from %s: error = %v, want nil", decisionGolden, err)
		}
		if got := encode(t, built); !bytes.Equal(got, golden) {
			t.Fatalf("NewDecision() rebuild of %s differs\n got:\n%s\nwant:\n%s", decisionGolden, got, golden)
		}
	})

	t.Run("invariant", func(t *testing.T) {
		golden := readGolden(t, invariantGolden)

		var want record.Invariant
		if err := json.Unmarshal(golden, &want); err != nil {
			t.Fatalf("decoding %s: %v", invariantGolden, err)
		}

		built, err := record.NewInvariant(want.ID, want.CreatedAt, "", want.Statement,
			record.WithRationale(want.Rationale),
			record.WithSeverity(want.Severity),
			record.WithSupersedes(want.Supersedes...),
			record.WithScope(want.Scope),
			record.WithTags(want.Tags...))
		if err != nil {
			t.Fatalf("NewInvariant() from %s: error = %v, want nil", invariantGolden, err)
		}
		// The golden is a superseded record and the constructor always stamps
		// active, which is the transition Supersede owns. Everything else must
		// match byte for byte.
		if built.Status != record.StatusActive {
			t.Fatalf("NewInvariant() Status = %q, want %q", built.Status, record.StatusActive)
		}
		built.Status = want.Status
		if got := encode(t, built); !bytes.Equal(got, golden) {
			t.Fatalf("NewInvariant() rebuild of %s differs\n got:\n%s\nwant:\n%s", invariantGolden, got, golden)
		}
	})
}
