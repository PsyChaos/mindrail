package schema_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/schemas"
)

// Fixtures are spelled out rather than built with record.NewDecision because
// this package must not depend on the writer it exists to police: a writer and
// a validator that agree only because one produced the other's input would
// hide exactly the disagreement REQ-01's AC-01.5 pairing looks for.
const (
	validDecision = `{"schema_version":1,"kind":"decision","id":"DEC-0001","status":"active",` +
		`"created_at":"2026-01-02T03:04:05Z","title":"Compile the shipped schemas",` +
		`"decision":"Validate every record against the document it names."}`

	validInvariant = `{"schema_version":1,"kind":"invariant","id":"INV-0001","status":"active",` +
		`"created_at":"2026-01-02T03:04:05Z","statement":"Every record validates against its schema.",` +
		`"severity":"HIGH","scope":{"level":"PROJECT"}}`

	// sixViolations breaks exactly the six constraints AC-02.6 names, one each:
	// pattern on id, format on created_at, uniqueItems on supersedes,
	// minLength on title, type reached through the $ref to $defs/scope, and
	// additionalProperties for "surprise".
	sixViolations = `{"schema_version":1,"kind":"decision","id":"DEC-1","status":"active",` +
		`"created_at":"yesterday","title":"","decision":"d",` +
		`"supersedes":["DEC-0002","DEC-0002"],"scope":{"level":123},"surprise":true}`
)

func embeddedValidator(t *testing.T) *schema.Validator {
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

// registryFrom builds a registry over exactly the documents given, so a test
// can ship a deliberately broken one without touching the embedded FS.
func registryFrom(t *testing.T, docs map[string][]byte) (*schema.Registry, error) {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, data := range docs {
		fsys["knowledge/"+name] = &fstest.MapFile{Data: data}
	}
	return schema.NewRegistry(fsys)
}

// located indexes findings by instance location and keyword, which is what the
// assertions below compare. Messages are prose; the pair is the fact.
func located(findings []schema.Finding) []string {
	out := make([]string, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding.InstanceLocation+" "+finding.Keyword)
	}
	return out
}

// TestValidatorAcceptsARecordThatSatisfiesItsSchema is the over-fire guard for
// every detection in this file: a healthy record must produce nothing at all,
// and the empty result must not be nil, so a caller can append to it and a
// marshalled report never says null.
func TestValidatorAcceptsARecordThatSatisfiesItsSchema(t *testing.T) {
	validator := embeddedValidator(t)

	tests := []struct {
		name     string
		kind     schema.RecordKind
		document string
	}{
		{name: "decision", kind: schema.KindDecision, document: validDecision},
		{name: "invariant", kind: schema.KindInvariant, document: validInvariant},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := validator.Validate(tt.kind, 1, []byte(tt.document))
			if findings == nil {
				t.Fatal("Validate() returned a nil slice; want an empty non-nil one")
			}
			if len(findings) != 0 {
				t.Fatalf("Validate() = %v, want no findings", located(findings))
			}
		})
	}
}

// TestValidatorAssertsTheDateTimeFormat holds decision D-48. Under Draft
// 2020-12 this library treats "format" as an annotation unless the metaschema
// declares the format-assertion vocabulary, and the shipped documents do not:
// deleting the AssertFormat() call in NewValidator makes created_at accept
// literally any string, and this test is the only thing that notices.
//
// Both rows matter, and the second was added when decision D-49 put a "pattern"
// beside the "format" on created_at. "yesterday" now breaks both keywords, and
// this library reports one finding per instance location, so the row below is
// only still a D-48 witness because it asserts the keyword rather than the
// existence of a finding — delete AssertFormat() and it reads "/created_at
// pattern" instead. That is a guard standing on a tie-break, and a tie-break is
// not a contract. The second row is a date only "format" can refuse: the
// spelling is canonical, so "pattern" is satisfied, and February's length is
// something no regular expression knows. It fails if and only if format
// assertion is on, whatever the reporting order does.
func TestValidatorAssertsTheDateTimeFormat(t *testing.T) {
	tests := map[string]struct {
		stamp string
		why   string
	}{
		"a string that is no kind of timestamp": {
			stamp: "yesterday",
			why:   "D-48's original witness; it breaks pattern too, so it asserts the keyword",
		},
		"a date the calendar does not have": {
			stamp: "2026-02-30T00:00:00Z",
			why:   "canonically spelled, so only format can refuse it — the tie-break-free witness",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			validator := embeddedValidator(t)

			document := strings.Replace(validDecision,
				`"created_at":"2026-01-02T03:04:05Z"`, `"created_at":"`+tt.stamp+`"`, 1)
			if document == validDecision {
				t.Fatal("fixture rewrite did not fire; the test would have asserted nothing")
			}

			findings := validator.Validate(schema.KindDecision, 1, []byte(document))
			if got := located(findings); !slices.Equal(got, []string{"/created_at format"}) {
				t.Fatalf("Validate() = %v, want exactly the format finding on /created_at — %s", got, tt.why)
			}
			if !strings.Contains(findings[0].Message, "created_at") {
				t.Errorf("Validate() message = %q, want it to name the offending field", findings[0].Message)
			}
		})
	}
}

// TestBothDocumentsStateTheSameCreatedAtRule is decision D-49's drift guard,
// and it exists because a mutation proved it was needed: deleting the pattern
// from invariant.v1 alone broke nothing, because every other test in this area
// is written over a decision.
//
// It reads the rule out of the shipped documents rather than restating it, for
// the same reason TestIDPatternsAreTheOnesTheSchemaDocumentsDeclare does — the
// documents are the contract (D-36), so a test that spelled the pattern out
// would be a second copy of the thing it is guarding. It asserts only that both
// documents carry the same non-empty rule, so tightening or loosening it is a
// one-place edit that this test never obstructs and never sleeps through.
func TestBothDocumentsStateTheSameCreatedAtRule(t *testing.T) {
	rules := make(map[string]map[string]string, 2)
	for _, name := range []string{schema.DecisionSchemaName, schema.InvariantSchemaName} {
		raw, err := schemas.KnowledgeFS.ReadFile(path.Join(schema.Dir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		var document struct {
			Properties struct {
				CreatedAt map[string]string `json:"created_at"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatalf("decoding %s: %v", name, err)
		}
		rules[name] = document.Properties.CreatedAt
	}

	for name, rule := range rules {
		if rule["format"] != "date-time" {
			t.Errorf("%s declares created_at format = %q, want \"date-time\" (D-48 asserts it)", name, rule["format"])
		}
		if rule["pattern"] == "" {
			t.Errorf("%s declares no created_at pattern; D-49 put one there and finding B-05 is reopened without it", name)
		}
	}

	decision := rules[schema.DecisionSchemaName]["pattern"]
	invariant := rules[schema.InvariantSchemaName]["pattern"]
	if decision != invariant {
		t.Fatalf("the two documents declare different created_at patterns:\n\t%s: %q\n\t%s: %q",
			schema.DecisionSchemaName, decision, schema.InvariantSchemaName, invariant)
	}
}

// TestValidatorAssertsTheCreatedAtPattern is the other half of the pair above,
// and it is what makes decision D-49 falsifiable.
//
// The pattern is the only thing refusing a timestamp that is a perfectly valid
// RFC 3339 instant but not the UTC spelling this project persists — "format"
// accepts every one of these. Removing the pattern from a shipped document
// would otherwise be silent here and would reopen finding B-05.
func TestValidatorAssertsTheCreatedAtPattern(t *testing.T) {
	tests := map[string]string{
		"a lowercase zone designator":     "2026-01-01T00:00:00z",
		"a lowercase date-time separator": "2026-01-01t00:00:00Z",
		"a leap second":                   "2026-12-31T23:59:60Z",
		"a western offset":                "2026-01-01T00:00:00-05:00",
		"more precision than Go holds":    "2026-01-01T00:00:00.1234567890123Z",
	}

	for name, stamp := range tests {
		t.Run(name, func(t *testing.T) {
			validator := embeddedValidator(t)

			document := strings.Replace(validDecision,
				`"created_at":"2026-01-02T03:04:05Z"`, `"created_at":"`+stamp+`"`, 1)
			if document == validDecision {
				t.Fatal("fixture rewrite did not fire; the test would have asserted nothing")
			}

			if got := located(validator.Validate(schema.KindDecision, 1, []byte(document))); !slices.Equal(got, []string{"/created_at pattern"}) {
				t.Fatalf("Validate() with created_at %q = %v, want exactly the pattern finding on /created_at",
					stamp, got)
			}
		})
	}

	// The over-fire guard, in the same test so it cannot be deleted separately:
	// the two offsets that denote UTC are not the spelling the writer emits, but
	// they are the same instant, and a pattern that refused them would take down
	// every repository whose records came from an ISO-8601 library.
	for name, stamp := range map[string]string{
		"the canonical spelling": "2026-01-02T03:04:05Z",
		"a plus-zero offset":     "2026-01-02T03:04:05+00:00",
		"a minus-zero offset":    "2026-01-02T03:04:05-00:00",
		"nine fractional digits": "2026-01-02T03:04:05.123456789Z",
	} {
		t.Run("accepted: "+name, func(t *testing.T) {
			validator := embeddedValidator(t)

			document := strings.Replace(validDecision,
				`"created_at":"2026-01-02T03:04:05Z"`, `"created_at":"`+stamp+`"`, 1)
			if findings := validator.Validate(schema.KindDecision, 1, []byte(document)); len(findings) != 0 {
				t.Fatalf("Validate() with created_at %q = %v, want no findings", stamp, located(findings))
			}
		})
	}
}

// TestValidatorReportsEveryViolationInOneRecord holds AC-02.6: the Causes tree
// is walked to its leaves, so a reader fixing a record sees every rule it
// breaks in one pass rather than one per pass.
func TestValidatorReportsEveryViolationInOneRecord(t *testing.T) {
	validator := embeddedValidator(t)

	findings := validator.Validate(schema.KindDecision, 1, []byte(sixViolations))
	want := []string{
		"/created_at format",
		"/id pattern",
		"/scope/level type",
		"/supersedes uniqueItems",
		"/surprise additionalProperties",
		"/title minLength",
	}
	if got := located(findings); !slices.Equal(got, want) {
		t.Fatalf("Validate() = %v, want %v", got, want)
	}
}

// TestValidatorReportsTheRefTargetNotTheRefWrapper holds the second half of
// AC-02.6. A "$ref" produces a kind.Reference node whose only child carries the
// keyword that actually rejected the value; reporting the wrapper would name
// the mechanism ("$ref failed") instead of the mistake ("level is a number").
func TestValidatorReportsTheRefTargetNotTheRefWrapper(t *testing.T) {
	validator := embeddedValidator(t)

	findings := validator.Validate(schema.KindDecision, 1, []byte(sixViolations))

	var scopeFinding *schema.Finding
	for i, finding := range findings {
		if finding.Keyword == "$ref" {
			t.Errorf("Validate() reported the $ref wrapper at %q; want the child that carries the keyword", finding.InstanceLocation)
		}
		if finding.InstanceLocation == "/scope/level" {
			scopeFinding = &findings[i]
		}
	}

	if scopeFinding == nil {
		t.Fatalf("Validate() = %v, want a finding located on /scope/level", located(findings))
	}
	if scopeFinding.Keyword != "type" {
		t.Errorf("finding at /scope/level has Keyword %q, want %q", scopeFinding.Keyword, "type")
	}
	// The wrapper's location is /scope; only the descended-to leaf can name the
	// member inside it, so this is what distinguishes the two.
	if scopeFinding.InstanceLocation == "/scope" {
		t.Error("finding stopped at the $ref wrapper's location")
	}
}

// TestValidatorNamesTheOffendingAdditionalProperty holds AC-02.7.
// kind.AdditionalProperties reports the *containing object* as its instance
// location and names the offending members only in .Properties, so a finding
// copied straight off it points at the whole record and leaves the reader to
// guess which key to delete.
func TestValidatorNamesTheOffendingAdditionalProperty(t *testing.T) {
	validator := embeddedValidator(t)

	t.Run("one extra property is located on itself", func(t *testing.T) {
		document := strings.Replace(validDecision, `"decision":`, `"surprise":true,"decision":`, 1)
		if document == validDecision {
			t.Fatal("fixture rewrite did not fire; the test would have asserted nothing")
		}

		findings := validator.Validate(schema.KindDecision, 1, []byte(document))
		if got := located(findings); !slices.Equal(got, []string{"/surprise additionalProperties"}) {
			t.Fatalf("Validate() = %v, want the finding located on /surprise, not on the record", got)
		}
	})

	t.Run("two extra properties are reported separately", func(t *testing.T) {
		document := strings.Replace(validDecision, `"decision":`, `"alpha":1,"beta":2,"decision":`, 1)
		if document == validDecision {
			t.Fatal("fixture rewrite did not fire; the test would have asserted nothing")
		}

		findings := validator.Validate(schema.KindDecision, 1, []byte(document))
		want := []string{"/alpha additionalProperties", "/beta additionalProperties"}
		if got := located(findings); !slices.Equal(got, want) {
			t.Fatalf("Validate() = %v, want %v", got, want)
		}

		// One message per offending key. The library's own rendering names both
		// keys in a single sentence, so a finding about "alpha" that also says
		// "beta" is the un-split wrapper wearing a split finding's location.
		for _, finding := range findings {
			other := map[string]string{"/alpha": "beta", "/beta": "alpha"}[finding.InstanceLocation]
			if !strings.Contains(finding.Message, strings.TrimPrefix(finding.InstanceLocation, "/")) {
				t.Errorf("finding at %s has message %q, want it to name that property", finding.InstanceLocation, finding.Message)
			}
			if strings.Contains(finding.Message, other) {
				t.Errorf("finding at %s has message %q, want it to name only its own property", finding.InstanceLocation, finding.Message)
			}
		}
	})
}

// TestValidatorFindingOrderIsStableAcrossRepeatedCalls holds decision D-47.
// ValidationError.Causes comes back in Go map-iteration order: five consecutive
// Validate calls on the same compiled schema and the same instance in one
// process were observed to produce five different leaf orders. Comparing every
// run against a fixed expected order rather than only against each other is
// what makes deleting the sort fail deterministically instead of one run in
// several hundred.
func TestValidatorFindingOrderIsStableAcrossRepeatedCalls(t *testing.T) {
	validator := embeddedValidator(t)

	want := []string{
		"/created_at format",
		"/id pattern",
		"/scope/level type",
		"/supersedes uniqueItems",
		"/surprise additionalProperties",
		"/title minLength",
	}

	first := validator.Validate(schema.KindDecision, 1, []byte(sixViolations))
	for run := range 20 {
		findings := validator.Validate(schema.KindDecision, 1, []byte(sixViolations))
		if got := located(findings); !slices.Equal(got, want) {
			t.Fatalf("run %d: Validate() = %v, want %v", run, got, want)
		}
		if !slices.Equal(findings, first) {
			t.Fatalf("run %d: Validate() = %+v, want the byte-identical result of run 0: %+v", run, findings, first)
		}
	}
}

// TestValidatorRejectsARegistryWhoseSchemaIDWasRenamed holds AC-02.2. The
// compile target is pinned in Go, so a document that renames its own "$id" —
// breaking every "$ref" that names it — is refused at construction instead of
// being compiled under whatever name it now claims. Compiling the document's
// own "$id" instead would make this pass silently.
func TestValidatorRejectsARegistryWhoseSchemaIDWasRenamed(t *testing.T) {
	renamed := bytes.Replace(
		mustReadEmbedded(t, schema.DecisionSchemaName),
		[]byte(`"https://mindrail.dev/schemas/knowledge/decision.v1.schema.json"`),
		[]byte(`"https://mindrail.dev/schemas/knowledge/decision.v1.renamed.json"`),
		1)
	if bytes.Equal(renamed, mustReadEmbedded(t, schema.DecisionSchemaName)) {
		t.Fatal("fixture rewrite did not fire; the test would have asserted nothing")
	}

	registry, err := registryFrom(t, map[string][]byte{
		schema.DecisionSchemaName:  renamed,
		schema.InvariantSchemaName: mustReadEmbedded(t, schema.InvariantSchemaName),
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v, want nil; the renamed document is still valid JSON", err)
	}

	validator, err := schema.NewValidator(registry)
	if err == nil {
		t.Fatalf("NewValidator() error = nil, want a failure; validator = %v", validator)
	}
	// errors.Is, not a substring: the refusal has to be recognisable without
	// control flow turning on an error message (tech-stack §72), and the
	// library's own loader failure spells the same condition "invalid file url".
	if !errors.Is(err, schema.ErrSchemaNotShipped) {
		t.Fatalf("NewValidator() error = %v, want one satisfying errors.Is(err, ErrSchemaNotShipped)", err)
	}
}

// TestValidatorRejectsAMalformedShippedSchemaAtConstruction holds AC-02.5. A
// document that is valid JSON but not a valid schema is a defect in the binary;
// finding it once, at construction, is what keeps it from being reported as a
// defect in every record the repository owns.
func TestValidatorRejectsAMalformedShippedSchemaAtConstruction(t *testing.T) {
	malformed := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema",` +
		`"$id":"https://mindrail.dev/schemas/knowledge/decision.v1.schema.json","type":"nonsense"}`)

	registry, err := registryFrom(t, map[string][]byte{
		schema.DecisionSchemaName:  malformed,
		schema.InvariantSchemaName: mustReadEmbedded(t, schema.InvariantSchemaName),
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v, want nil; the document is valid JSON", err)
	}

	if _, err := schema.NewValidator(registry); err == nil {
		t.Fatal("NewValidator() error = nil, want a failure for a document that is not a valid schema")
	}
}

// TestValidatorRejectsAShippedDocumentThatDeclaresNoID keeps the compiler's
// view and the file's own claim about its identity together. A document with no
// "$id" cannot be referenced by anything, and registering it under an invented
// URL would create a resource no "$ref" could name.
func TestValidatorRejectsAShippedDocumentThatDeclaresNoID(t *testing.T) {
	registry, err := registryFrom(t, map[string][]byte{
		schema.DecisionSchemaName:  mustReadEmbedded(t, schema.DecisionSchemaName),
		schema.InvariantSchemaName: mustReadEmbedded(t, schema.InvariantSchemaName),
		"decision.v2.schema.json":  []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema"}`),
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v, want nil", err)
	}

	if _, err := schema.NewValidator(registry); err == nil {
		t.Fatal("NewValidator() error = nil, want a failure for a document with no $id")
	}
}

// TestValidatorAcceptsAnExtraSchemaDocumentAlongsideV1 is the over-fire guard
// for the test above: the registry is additive on purpose, so a future version
// shipped beside v1 must not break a v1 binary's startup.
func TestValidatorAcceptsAnExtraSchemaDocumentAlongsideV1(t *testing.T) {
	registry, err := registryFrom(t, map[string][]byte{
		schema.DecisionSchemaName:  mustReadEmbedded(t, schema.DecisionSchemaName),
		schema.InvariantSchemaName: mustReadEmbedded(t, schema.InvariantSchemaName),
		"decision.v2.schema.json": []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema",` +
			`"$id":"https://mindrail.dev/schemas/knowledge/decision.v2.schema.json","type":"object"}`),
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v, want nil", err)
	}

	validator, err := schema.NewValidator(registry)
	if err != nil {
		t.Fatalf("NewValidator() error = %v, want nil; an extra document must not break a v1 binary", err)
	}
	if findings := validator.Validate(schema.KindDecision, 1, []byte(validDecision)); len(findings) != 0 {
		t.Fatalf("Validate() = %v, want no findings", located(findings))
	}
}

// TestValidatorReportsAKindOrVersionItCompiledNoSchemaFor refuses the
// fabricated pass. Returning nothing for a pair no document governs would
// publish "nobody looked" as "we looked and it is clean" — the reading this
// project keeps having to separate.
func TestValidatorReportsAKindOrVersionItCompiledNoSchemaFor(t *testing.T) {
	validator := embeddedValidator(t)

	tests := []struct {
		name    string
		kind    schema.RecordKind
		version int
	}{
		{name: "unknown kind", kind: schema.RecordKind("charter"), version: 1},
		{name: "version above the reader window", kind: schema.KindDecision, version: 2},
		{name: "version below the reader window", kind: schema.KindDecision, version: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := validator.Validate(tt.kind, tt.version, []byte(validDecision))
			if len(findings) != 1 {
				t.Fatalf("Validate() = %v, want exactly one finding", located(findings))
			}
			if findings[0].Keyword != schema.KeywordNoSchema {
				t.Errorf("Validate() Keyword = %q, want %q", findings[0].Keyword, schema.KeywordNoSchema)
			}
			// The keyword says "no document governed this"; the message is the
			// only place the reader learns *which* pair was refused, and both
			// halves matter — the same keyword is also returned for an
			// unexpected library failure. Finding BA-09 caught this: rewriting
			// the message to a constant left the assertions above green.
			if !strings.Contains(findings[0].Message, string(tt.kind)) {
				t.Errorf("Validate() Message = %q, want it to name the kind %q", findings[0].Message, tt.kind)
			}
			if !strings.Contains(findings[0].Message, fmt.Sprintf("schema_version %d", tt.version)) {
				t.Errorf("Validate() Message = %q, want it to name schema_version %d", findings[0].Message, tt.version)
			}
			if !strings.Contains(findings[0].Message, "was not validated") {
				t.Errorf("Validate() Message = %q, want it to say the record was not validated; "+
					"a finding that does not say so reads as a verdict about the record", findings[0].Message)
			}
		})
	}
}

// TestValidatorRefusesTwoDocumentsClaimingOneIdentity reaches the AddResource
// refusal, which is the third way a shipped document can be wrong: not absent
// (the registry catches that), not malformed (the compiler catches that), but
// claiming an identity another document already claimed.
//
// Finding BA-09 found this guard unkilled. Deleting it is silent in the happy
// case, because the compiler keeps whichever document registered first and
// compiles it perfectly well — so the binary would ship two documents, validate
// every record against one of them, and say nothing about the other. Which one
// wins is registration order, which is Names() order, which is alphabetical:
// a v2 document that copied v1's $id would be ignored in favour of v1, and every
// v2 record would be judged by the v1 rules.
func TestValidatorRefusesTwoDocumentsClaimingOneIdentity(t *testing.T) {
	impostor := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema",` +
		`"$id":"https://mindrail.dev/schemas/knowledge/decision.v1.schema.json","type":"object"}`)

	registry, err := registryFrom(t, map[string][]byte{
		schema.DecisionSchemaName:  mustReadEmbedded(t, schema.DecisionSchemaName),
		schema.InvariantSchemaName: mustReadEmbedded(t, schema.InvariantSchemaName),
		"decision.v2.schema.json":  impostor,
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v, want nil; the impostor is valid JSON", err)
	}

	validator, err := schema.NewValidator(registry)
	if err == nil {
		t.Fatalf("NewValidator() error = nil, want a failure; validator = %v", validator)
	}
	if !strings.Contains(err.Error(), "decision.v2.schema.json") {
		t.Fatalf("NewValidator() error = %q, want it to name the document that could not be registered", err)
	}
}

// TestValidatorRefusesADocumentClaimingTheMetaschemasIdentity is the second
// input that reaches only the AddResource refusal, and it reaches it down the
// other branch: the library refuses a document that names a draft metaschema
// rather than one that repeats a sibling's $id.
func TestValidatorRefusesADocumentClaimingTheMetaschemasIdentity(t *testing.T) {
	impostor := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema",` +
		`"$id":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)

	registry, err := registryFrom(t, map[string][]byte{
		schema.DecisionSchemaName:  mustReadEmbedded(t, schema.DecisionSchemaName),
		schema.InvariantSchemaName: mustReadEmbedded(t, schema.InvariantSchemaName),
		"decision.v2.schema.json":  impostor,
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v, want nil", err)
	}

	if _, err := schema.NewValidator(registry); err == nil {
		t.Fatal("NewValidator() error = nil, want a failure for a document claiming the metaschema's identity")
	}
}

// TestValidatorReportsADocumentThatIsNotASingleJSONValue keeps step 5 honest
// about bytes the loader would never have handed it. Silence would again be a
// fabricated pass.
func TestValidatorReportsADocumentThatIsNotASingleJSONValue(t *testing.T) {
	validator := embeddedValidator(t)

	for _, document := range []string{"", "{ not json", validDecision + " trailing"} {
		findings := validator.Validate(schema.KindDecision, 1, []byte(document))
		if len(findings) != 1 {
			t.Fatalf("Validate(%q) = %v, want exactly one finding", document, located(findings))
		}
		if findings[0].Keyword != schema.KeywordDocumentSyntax {
			t.Errorf("Validate(%q) Keyword = %q, want %q", document, findings[0].Keyword, schema.KeywordDocumentSyntax)
		}
	}
}

// TestValidatorDecodesNumbersWithoutLosingPrecision holds the second half of
// the jsonschema.UnmarshalJSON rule. encoding/json decodes a JSON number into
// float64, and 1.0000000000000001 rounds to exactly 1 on the way in — which
// would satisfy both "type": "integer" and "const": 1 and let a record through
// that the document rejects. The library's decoder keeps it as json.Number.
func TestValidatorDecodesNumbersWithoutLosingPrecision(t *testing.T) {
	validator := embeddedValidator(t)

	document := strings.Replace(validDecision, `"schema_version":1,`, `"schema_version":1.0000000000000001,`, 1)
	if document == validDecision {
		t.Fatal("fixture rewrite did not fire; the test would have asserted nothing")
	}

	findings := validator.Validate(schema.KindDecision, 1, []byte(document))
	if got := located(findings); !slices.Equal(got, []string{"/schema_version type"}) {
		t.Fatalf("Validate() = %v, want the type finding on /schema_version; a float64 round-trip would have made this record valid", got)
	}
}

// TestValidatorReportsTheInvariantSchemasOwnRules proves both compiled
// documents are wired to their own kind. Feeding the decision fixture's shape
// to the invariant schema would otherwise pass unnoticed if both keys pointed
// at the same compiled schema.
func TestValidatorReportsTheInvariantSchemasOwnRules(t *testing.T) {
	validator := embeddedValidator(t)

	document := strings.Replace(validInvariant, `"severity":"HIGH"`, `"severity":"URGENT"`, 1)
	if document == validInvariant {
		t.Fatal("fixture rewrite did not fire; the test would have asserted nothing")
	}

	findings := validator.Validate(schema.KindInvariant, 1, []byte(document))
	if got := located(findings); !slices.Equal(got, []string{"/severity enum"}) {
		t.Fatalf("Validate() = %v, want the enum finding on /severity", got)
	}

	// severity is not a member of the decision document at all, so the decision
	// schema rejects the same bytes for an entirely different reason. Identical
	// findings here would mean one compiled schema is answering for both kinds.
	if got := located(validator.Validate(schema.KindDecision, 1, []byte(document))); slices.Equal(got, []string{"/severity enum"}) {
		t.Fatal("the decision schema produced the invariant schema's finding; the two kinds share a compiled schema")
	}
}

// TestSchemaKindsAreSpeltTheSameWayAsTheLoaders is the guard on this package's
// resolution of the import cycle the frozen signature could not have.
// internal/knowledge/loader already imports this package, so Validate cannot
// take a loader.RecordKind; callers convert instead, and a conversion between
// two string types is silent when the strings disagree. This is what makes it
// loud.
func TestSchemaKindsAreSpeltTheSameWayAsTheLoaders(t *testing.T) {
	tests := []struct {
		fromLoader loader.RecordKind
		fromSchema schema.RecordKind
	}{
		{fromLoader: loader.KindDecision, fromSchema: schema.KindDecision},
		{fromLoader: loader.KindInvariant, fromSchema: schema.KindInvariant},
	}

	for _, tt := range tests {
		if schema.RecordKind(tt.fromLoader) != tt.fromSchema {
			t.Errorf("schema.RecordKind(loader.%q) = %q, want %q", tt.fromLoader, schema.RecordKind(tt.fromLoader), tt.fromSchema)
		}
	}

	// A validator keyed on the loader's spelling must find the compiled schema,
	// which is the only thing the conversion is for.
	validator := embeddedValidator(t)
	if findings := validator.Validate(schema.RecordKind(loader.KindInvariant), 1, []byte(validInvariant)); len(findings) != 0 {
		t.Fatalf("Validate(converted loader kind) = %v, want no findings", located(findings))
	}
}

// TestSchemaDocumentNamingConventionAgreesWithTheRegistryConstants keeps the
// derived document name and the two exported constants from drifting. The
// derivation is what lets a widened reader window fail loudly instead of
// leaving a version silently unvalidated, so it has to stay true of v1.
func TestSchemaDocumentNamingConventionAgreesWithTheRegistryConstants(t *testing.T) {
	tests := []struct {
		kind schema.RecordKind
		want string
	}{
		{kind: schema.KindDecision, want: schema.DecisionSchemaName},
		{kind: schema.KindInvariant, want: schema.InvariantSchemaName},
	}

	for _, tt := range tests {
		if got := schema.SchemaNameFor(tt.kind, schema.WriteVersion); got != tt.want {
			t.Errorf("SchemaNameFor(%q, %d) = %q, want %q", tt.kind, schema.WriteVersion, got, tt.want)
		}
	}
}

// TestNewValidatorRejectsANilRegistry keeps the constructor from producing a
// Validator that would panic on first use.
func TestNewValidatorRejectsANilRegistry(t *testing.T) {
	if _, err := schema.NewValidator(nil); err == nil {
		t.Fatal("NewValidator(nil) error = nil, want an error")
	}
}
