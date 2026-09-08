package schema

import (
	"bytes"
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
func TestNewValidatorFailsWhenARequiredDocumentIsAbsent(t *testing.T) {
	tests := []struct {
		name    string
		present []string
	}{
		{name: "invariant document absent", present: []string{DecisionSchemaName}},
		{name: "decision document absent", present: []string{InvariantSchemaName}},
		{name: "both absent", present: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator, err := NewValidator(registryWith(t, tt.present...))
			if err == nil {
				t.Fatalf("NewValidator() error = nil, want a failure; validator holds %d schemas", len(validator.schemas))
			}
		})
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
func TestDocumentIDRefusesADocumentThatNamesItselfNothing(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{name: "no $id member", doc: `{"$schema":"https://json-schema.org/draft/2020-12/schema"}`},
		{name: "empty $id", doc: `{"$id":""}`},
		{name: "$id is not a string", doc: `{"$id":42}`},
		{name: "document is not an object", doc: `["not","an","object"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := jsonschema.UnmarshalJSON(strings.NewReader(tt.doc))
			if err != nil {
				t.Fatalf("decoding fixture: %v", err)
			}
			if id, err := documentID(doc); err == nil {
				t.Fatalf("documentID() = %q, nil; want an error", id)
			}
		})
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
