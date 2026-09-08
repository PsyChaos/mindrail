package schema

import (
	"bytes"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"

	"github.com/PsyChaos/mindrail/schemas"
)

// These tests are internal because the three properties they hold are not
// reachable from outside the package: NewRegistry refuses to build a registry
// that is missing a required document, so AC-02.4's "removing a required
// document" cannot be staged through the exported constructor; whether
// compilation happened at construction is a fact about a private field; and the
// leaf walk is where the "$ref" wrapper rule actually lives.

// registryWith builds a Registry directly, bypassing NewRegistry's insistence
// on the required documents. That insistence is exactly what has to be stepped
// around to ask what NewValidator does when one is missing.
func registryWith(t *testing.T, names ...string) *Registry {
	t.Helper()
	docs := make(map[string][]byte, len(names))
	for _, name := range names {
		data, err := schemas.KnowledgeFS.ReadFile("knowledge/" + name)
		if err != nil {
			t.Fatalf("reading embedded %q: %v", name, err)
		}
		docs[name] = data
	}
	return &Registry{docs: docs, names: slices.Sorted(maps.Keys(docs))}
}

// TestNewValidatorFailsWhenARequiredDocumentIsAbsent holds AC-02.4 and decision
// D-37. A validator that quietly skipped the document it could not find would
// report every invariant in the repository as clean while nothing had looked at
// one, and "nobody looked" published as "we looked and it is clean" is the
// reading this project keeps paying to separate.
// Each row also asserts which document was named and that the refusal is not a
// fetch attempt. Finding BA-09 is why: without the "ships no" check the compile
// below still fails, because nothing registered that $id and the denied loader
// turns it into ErrSchemaNotShipped — so a row asking only "err != nil" left the
// check deletable. The two conditions are different and get different answers:
// "this binary was never given the file" is a build problem, while "a $ref went
// looking for a document nothing registered" is a renamed $id (D-37), and
// TestValidatorRejectsARegistryWhoseSchemaIDWasRenamed owns that one.
func TestNewValidatorFailsWhenARequiredDocumentIsAbsent(t *testing.T) {
	tests := []struct {
		name    string
		present []string
		// named is the document the diagnosis has to point at: the loop runs
		// validatedKinds in order, so the first absent one is the one reported.
		named string
	}{
		{name: "invariant document absent", present: []string{DecisionSchemaName}, named: InvariantSchemaName},
		{name: "decision document absent", present: []string{InvariantSchemaName}, named: DecisionSchemaName},
		{name: "both absent", present: nil, named: DecisionSchemaName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator, err := NewValidator(registryWith(t, tt.present...))
			if err == nil {
				t.Fatalf("NewValidator() error = nil, want a failure; validator holds %d schemas", len(validator.schemas))
			}
			if !strings.Contains(err.Error(), "ships no "+tt.named) {
				t.Fatalf("NewValidator() error = %q, want it to say the binary ships no %s", err, tt.named)
			}
			if errors.Is(err, ErrSchemaNotShipped) {
				t.Fatalf("NewValidator() error = %q, want a missing-document refusal rather than a fetch attempt; "+
					"a document this binary never shipped must be reported before the compiler turns it into a $ref nobody registered", err)
			}
		})
	}
}

// TestNewValidatorAcceptsARegistryCarryingEveryDocumentItReads is the over-fire
// guard for the table above. "Ships no document for this pair" must fire only
// when the pair really has none: a registry holding both required documents
// constructs, and the resulting validator is one that actually validates rather
// than one that holds an empty map.
func TestNewValidatorAcceptsARegistryCarryingEveryDocumentItReads(t *testing.T) {
	validator, err := NewValidator(registryWith(t, DecisionSchemaName, InvariantSchemaName))
	if err != nil {
		t.Fatalf("NewValidator() error = %v, want nil", err)
	}
	want := len(validatedKinds) * len(ReadableVersions())
	if got := len(validator.schemas); got != want {
		t.Fatalf("NewValidator() compiled %d schemas, want %d", got, want)
	}
}

// TestNewValidatorCompilesEverySchemaAtConstruction holds AC-02.5 as a fact
// rather than as a hope. Compilation deferred to the first Validate call would
// leave this map empty here and would report a malformed shipped document once
// per record, blaming the repository for a defect in the binary.
func TestNewValidatorCompilesEverySchemaAtConstruction(t *testing.T) {
	registry, err := NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded) error = %v, want nil", err)
	}

	validator, err := NewValidator(registry)
	if err != nil {
		t.Fatalf("NewValidator(embedded) error = %v, want nil", err)
	}

	want := len(validatedKinds) * len(registry.ReadableVersions())
	if got := len(validator.schemas); got != want {
		t.Fatalf("NewValidator() compiled %d schemas, want %d (one per kind per readable version)", got, want)
	}
	for _, recordKind := range validatedKinds {
		for _, version := range registry.ReadableVersions() {
			if compiled := validator.schemas[schemaKey{kind: recordKind, version: version}]; compiled == nil {
				t.Errorf("no schema compiled for kind %q at schema_version %d", recordKind, version)
			}
		}
	}
}

// TestNewValidatorResolvesEverySchemaWithoutAFetch holds the no-network half of
// AC-02.4 without depending on the machine being offline. Every document is
// handed to the compiler through AddResource and the Draft 2020-12 metaschemas
// are embedded in the library, so a successful construction must have consulted
// the loader zero times. An error-free compile cannot prove that on its own: a
// loader never consulted and a loader consulted successfully both leave it
// green, which is why the attempts are recorded rather than counted at the end.
func TestNewValidatorResolvesEverySchemaWithoutAFetch(t *testing.T) {
	registry, err := NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded) error = %v, want nil", err)
	}

	validator, err := NewValidator(registry)
	if err != nil {
		t.Fatalf("NewValidator(embedded) error = %v, want nil", err)
	}

	if attempted := validator.loader.denied; len(attempted) != 0 {
		t.Fatalf("compiling the shipped schemas went looking for %v, want no fetch on any path", attempted)
	}

	// Validating a record must not reach for anything either: the compiled
	// schema is closed over its resources, and a "$ref" resolved lazily at
	// validation time would show up here.
	validator.Validate(KindDecision, 1, []byte(`{"schema_version":1,"kind":"decision","id":"DEC-0001",`+
		`"status":"active","created_at":"2026-01-02T03:04:05Z","title":"t","decision":"d","scope":{"level":"PROJECT"}}`))
	if attempted := validator.loader.denied; len(attempted) != 0 {
		t.Fatalf("validating a record went looking for %v, want no fetch on any path", attempted)
	}
}

// TestShippedSchemaDocumentsDeclareThePinnedID pins the identity half of
// decision D-37 against the constant the compiler is actually given. A document
// that renames its "$id" already fails construction; asserting it here as well
// is what makes the failure name the document instead of arriving as a compile
// error two layers down.
func TestShippedSchemaDocumentsDeclareThePinnedID(t *testing.T) {
	registry, err := NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded) error = %v, want nil", err)
	}

	for _, name := range []string{DecisionSchemaName, InvariantSchemaName} {
		raw, ok := registry.Document(name)
		if !ok {
			t.Fatalf("Document(%q) not found", name)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("decoding %s: %v", name, err)
		}
		id, err := documentID(doc)
		if err != nil {
			t.Fatalf("%s %v", name, err)
		}
		if want := schemaIDBase + name; id != want {
			t.Errorf("%s declares $id %q, want %q — the compile target is pinned to the latter", name, id, want)
		}
	}
}

// TestDocumentIDRefusesADocumentThatNamesItselfNothing keeps the constructor
// from registering a document under an invented URL, which would create a
// resource no "$ref" could name.
//
// Each row asserts *which* refusal it got, not merely that one arrived, and that
// is the whole point of the table. Finding BA-09 showed why: deleting the
// "is not a JSON object" type assertion and keeping `object, _ := ...` leaves a
// nil map, the `object["$id"]` lookup below then misses, and the function still
// returns an error — so a row that only asked "err != nil" published a deleted
// guard as a live one. A JSON array is not a document with no $id; it is not a
// schema document at all, and a reader sent looking for a missing "$id" member
// in `["not","an","object"]` has been told the wrong thing.
func TestDocumentIDRefusesADocumentThatNamesItselfNothing(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		// want is a distinguishing fragment of the diagnosis: no two rows may
		// share one, or the table stops separating the conditions.
		want string
	}{
		{name: "no $id member", doc: `{"$schema":"https://json-schema.org/draft/2020-12/schema"}`, want: "declares no $id"},
		{name: "empty $id", doc: `{"$id":""}`, want: "not a non-empty string"},
		{name: "$id is not a string", doc: `{"$id":42}`, want: "not a non-empty string"},
		{name: "document is not an object", doc: `["not","an","object"]`, want: "is not a JSON object"},
		{name: "document is a bare true", doc: `true`, want: "is not a JSON object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := jsonschema.UnmarshalJSON(strings.NewReader(tt.doc))
			if err != nil {
				t.Fatalf("decoding fixture: %v", err)
			}
			id, err := documentID(doc)
			if err == nil {
				t.Fatalf("documentID() = %q, nil; want an error", id)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("documentID() error = %q, want it to say %q — a refusal that names the wrong condition sends the reader to the wrong fix", err, tt.want)
			}
		})
	}
}

// TestDocumentIDAcceptsAnOrdinaryDocument is the over-fire guard for the table
// above. The rows there are all refusals, so a documentID that refused
// everything would satisfy every one of them; the shipped documents are the
// adjacent healthy case and must still be readable.
func TestDocumentIDAcceptsAnOrdinaryDocument(t *testing.T) {
	for _, name := range []string{DecisionSchemaName, InvariantSchemaName} {
		raw, err := schemas.KnowledgeFS.ReadFile("knowledge/" + name)
		if err != nil {
			t.Fatalf("reading embedded %q: %v", name, err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("decoding %s: %v", name, err)
		}
		id, err := documentID(doc)
		if err != nil {
			t.Fatalf("documentID(%s) error = %v, want nil", name, err)
		}
		if id == "" {
			t.Fatalf("documentID(%s) = %q, want a non-empty $id", name, id)
		}
	}
}

// TestNewValidatorNamesADocumentItWasListedButNotGiven reaches the one guard in
// NewValidator that nothing outside this package can reach: the read-back of a
// name Names() just produced.
//
// A Registry built by NewRegistry always agrees with itself — names is derived
// from docs and neither is mutable afterwards — so the only way to a disagreeing
// pair is a struct literal, which is exactly what this package's own helpers
// build. That makes the guard an invariant check on the type rather than a
// concurrency guard, and this test is what stops it from being deleted as dead
// code: without it the loop reads a nil document, hands it to
// jsonschema.UnmarshalJSON, and the constructor blames "unexpected end of JSON
// input" on a file that is merely absent from one half of the registry.
func TestNewValidatorNamesADocumentItWasListedButNotGiven(t *testing.T) {
	registry := registryWith(t, DecisionSchemaName, InvariantSchemaName)
	// The listing grows a name the document map does not carry. Nothing but a
	// literal can produce this, and a literal is how the tests above build one.
	registry.names = append(registry.names, "decision.v9.schema.json")

	_, err := NewValidator(registry)
	if err == nil {
		t.Fatal("NewValidator() error = nil, want a failure for a name the registry lists but cannot produce")
	}
	if !strings.Contains(err.Error(), "decision.v9.schema.json") {
		t.Fatalf("NewValidator() error = %q, want it to name decision.v9.schema.json", err)
	}
	// The wording is asserted because it is the only observable there is: with
	// the check gone the constructor still fails, on nil bytes, at the decoder —
	// same name, wrong diagnosis, and a reader sent to look at the contents of a
	// file the registry never held. Asserting "an error happened" is what let
	// this check be deleted silently in the first place.
	if !strings.Contains(err.Error(), "holds no document") {
		t.Fatalf("NewValidator() error = %q, want the registry's own diagnosis; "+
			"a decode failure on nil bytes blames the file's contents for its absence", err)
	}
	if strings.Contains(err.Error(), "decoding") {
		t.Fatalf("NewValidator() error = %q, want the absence reported before anything tried to decode it", err)
	}
}

// TestNewValidatorAcceptsARegistryWhoseTwoHalvesAgree is the over-fire guard for
// the test above: the check must fire on a disagreeing pair and on nothing else,
// so the same helper without the extra name has to construct.
func TestNewValidatorAcceptsARegistryWhoseTwoHalvesAgree(t *testing.T) {
	if _, err := NewValidator(registryWith(t, DecisionSchemaName, InvariantSchemaName)); err != nil {
		t.Fatalf("NewValidator() error = %v, want nil for a registry whose names and documents agree", err)
	}
}

// TestCollectFindingsDescendsToLeaves exercises the walk directly, on a tree
// built by hand rather than by a schema.
//
// Doing it this way is deliberate: which tree the library produces for a given
// document is the library's business and could change, while the rule this
// package owns — report the node that carries the keyword, never the wrapper
// above it — is ours and must hold for any shape.
func TestCollectFindingsDescendsToLeaves(t *testing.T) {
	leaf := &jsonschema.ValidationError{
		SchemaURL:        schemaIDBase + DecisionSchemaName + "#/$defs/scope/properties/level",
		InstanceLocation: []string{"scope", "level"},
		ErrorKind:        &kind.Type{Got: "number", Want: []string{"string"}},
	}
	wrapper := &jsonschema.ValidationError{
		SchemaURL:        schemaIDBase + DecisionSchemaName + "#/properties/scope",
		InstanceLocation: []string{"scope"},
		ErrorKind:        &kind.Reference{Keyword: "$ref", URL: schemaIDBase + DecisionSchemaName + "#/$defs/scope"},
		Causes:           []*jsonschema.ValidationError{leaf},
	}
	root := &jsonschema.ValidationError{
		SchemaURL: schemaIDBase + DecisionSchemaName + "#",
		ErrorKind: &kind.Schema{Location: schemaIDBase + DecisionSchemaName + "#"},
		Causes:    []*jsonschema.ValidationError{wrapper},
	}

	findings := collectFindings(root, nil)
	if len(findings) != 1 {
		t.Fatalf("collectFindings() returned %d findings, want 1", len(findings))
	}
	if findings[0].Keyword != "type" {
		t.Errorf("Keyword = %q, want %q; the $ref wrapper and the root are not findings", findings[0].Keyword, "type")
	}
	if findings[0].InstanceLocation != "/scope/level" {
		t.Errorf("InstanceLocation = %q, want %q", findings[0].InstanceLocation, "/scope/level")
	}
}

// TestCollectFindingsNeverTurnsAFailureIntoSilence covers the degenerate tree.
// A rejected record that produced no finding would be published as clean, so a
// node with no causes is reported whatever kind it carries.
func TestCollectFindingsNeverTurnsAFailureIntoSilence(t *testing.T) {
	root := &jsonschema.ValidationError{
		SchemaURL: schemaIDBase + DecisionSchemaName + "#",
		ErrorKind: &kind.Schema{Location: schemaIDBase + DecisionSchemaName + "#"},
	}

	if findings := collectFindings(root, nil); len(findings) != 1 {
		t.Fatalf("collectFindings() returned %d findings for a rejected record, want 1", len(findings))
	}
}

// TestPointerOfEscapesRFC6901Tokens keeps a member actually named "a/b" from
// being reported as a nested one. The library hands back raw tokens, so the
// escaping is this package's job.
func TestPointerOfEscapesRFC6901Tokens(t *testing.T) {
	tests := []struct {
		name   string
		tokens []string
		want   string
	}{
		{name: "record itself", tokens: nil, want: ""},
		{name: "plain member", tokens: []string{"created_at"}, want: "/created_at"},
		{name: "nested member", tokens: []string{"scope", "level"}, want: "/scope/level"},
		{name: "member containing a slash", tokens: []string{"a/b"}, want: "/a~1b"},
		{name: "member containing a tilde", tokens: []string{"a~b"}, want: "/a~0b"},
		{name: "tilde introduced by escaping is not re-escaped", tokens: []string{"a/~b"}, want: "/a~1~0b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pointerOf(tt.tokens); got != tt.want {
				t.Errorf("pointerOf(%q) = %q, want %q", tt.tokens, got, tt.want)
			}
		})
	}
}
