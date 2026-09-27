package record

import (
	"encoding/json"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/schemas"
)

// This file holds every assertion that compares something in Go to the same
// rule as the shipped schema document states it. Decision D-36 makes the
// documents the contract; this package restates four of their rules in Go
// because a constructor has to refuse bad input before any schema is in reach,
// and each restatement is only safe while a test reads the original back out.
//
// It is an internal test because three of the four restatements —
// decisionIDPattern, severities, scopeLevels — are unexported, and exporting
// them so an external test could reach them would widen the package's API to
// suit its tests.

const (
	decisionSchemaName  = "decision.v1.schema.json"
	invariantSchemaName = "invariant.v1.schema.json"
)

// shippedDocument decodes one embedded schema document into a generic map, so
// an assertion can read its expectation out of the contract rather than
// restating it a third time.
func shippedDocument(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := schemas.KnowledgeFS.ReadFile("knowledge/" + name)
	if err != nil {
		t.Fatalf("reading embedded %s: %v", name, err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decoding embedded %s: %v", name, err)
	}
	return document
}

// valueAt walks a decoded document by member name.
//
// It fails rather than returning a zero value on a missing member. A schema
// restructured out from under one of these paths would otherwise turn the
// assertion below into a comparison against nothing, which passes — the
// "nobody looked, published as clean" failure this project keeps paying for.
func valueAt(t *testing.T, document map[string]any, path ...string) any {
	t.Helper()
	var current any = document
	for index, member := range path {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("schema path %v: %q is not an object", path, strings.Join(path[:index], "."))
		}
		current, ok = object[member]
		if !ok {
			t.Fatalf("schema path %v: no member %q", path, member)
		}
	}
	return current
}

func stringAt(t *testing.T, document map[string]any, path ...string) string {
	t.Helper()
	value, ok := valueAt(t, document, path...).(string)
	if !ok {
		t.Fatalf("schema path %v is not a string", path)
	}
	return value
}

func stringsAt(t *testing.T, document map[string]any, path ...string) []string {
	t.Helper()
	raw, ok := valueAt(t, document, path...).([]any)
	if !ok {
		t.Fatalf("schema path %v is not an array", path)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("schema path %v holds the non-string entry %v", path, item)
		}
		out = append(out, text)
	}
	return out
}

// TestIDPatternsAreTheOnesTheSchemaDocumentsDeclare is what makes the two
// regexps in new.go safe to keep. They exist because a constructor must refuse
// a malformed id without a compiled schema in hand; the moment either differs
// from the document's own "pattern", a record this package writes and a record
// step 5 accepts stop being the same set.
func TestIDPatternsAreTheOnesTheSchemaDocumentsDeclare(t *testing.T) {
	tests := []struct {
		name     string
		document string
		pattern  *regexp.Regexp
	}{
		{name: "decision", document: decisionSchemaName, pattern: decisionIDPattern},
		{name: "invariant", document: invariantSchemaName, pattern: invariantIDPattern},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := shippedDocument(t, tt.document)

			declared := stringAt(t, document, "properties", "id", "pattern")
			if tt.pattern.String() != declared {
				t.Errorf("Go id pattern = %q, %s declares %q", tt.pattern.String(), tt.document, declared)
			}

			// The same pattern governs every entry of "supersedes", and the
			// constructors check supersede targets with the same regexp. A
			// document that tightened one and not the other would leave this
			// package writing lineage ids step 5 rejects.
			onItems := stringAt(t, document, "properties", "supersedes", "items", "pattern")
			if tt.pattern.String() != onItems {
				t.Errorf("Go id pattern = %q, %s declares %q on supersedes items", tt.pattern.String(), tt.document, onItems)
			}
		})
	}
}

// TestStructsMirrorTheirSchemaDocuments holds AC-01.1 exactly: the structs carry
// the fields and JSON tags their documents define, no more and no fewer.
//
// The second comparison is the one with teeth. A schema's "required" list and a
// struct's set of fields written without ",omitempty" are the same statement
// made in two languages, so the only way they can differ is that one of them is
// wrong — an omitempty on a required property writes a document missing a field
// the schema demands, and a missing omitempty on an optional one writes an
// empty string where the document said absent.
func TestStructsMirrorTheirSchemaDocuments(t *testing.T) {
	tests := []struct {
		name     string
		document string
		subPath  []string
		value    any
	}{
		{name: "decision", document: decisionSchemaName, value: Decision{}},
		{name: "invariant", document: invariantSchemaName, value: Invariant{}},
		{name: "scope as decision.v1 defines it", document: decisionSchemaName, subPath: []string{"$defs", "scope"}, value: Scope{}},
		{name: "scope as invariant.v1 defines it", document: invariantSchemaName, subPath: []string{"$defs", "scope"}, value: Scope{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := shippedDocument(t, tt.document)
			if len(tt.subPath) > 0 {
				sub, ok := valueAt(t, node, tt.subPath...).(map[string]any)
				if !ok {
					t.Fatalf("%s %v is not an object", tt.document, tt.subPath)
				}
				node = sub
			}

			properties, ok := valueAt(t, node, "properties").(map[string]any)
			if !ok {
				t.Fatalf("%s %v properties is not an object", tt.document, tt.subPath)
			}
			declared := slices.Sorted(maps.Keys(properties))
			required := stringsAt(t, node, "required")
			slices.Sort(required)

			emitted, mandatory := jsonTagsOf(t, tt.value)
			if !slices.Equal(emitted, declared) {
				t.Errorf("%T emits %v, %s declares %v", tt.value, emitted, tt.document, declared)
			}
			if !slices.Equal(mandatory, required) {
				t.Errorf("%T always emits %v, %s requires %v", tt.value, mandatory, tt.document, required)
			}
		})
	}
}

// jsonTagsOf returns every property name the struct emits, and the subset it
// emits unconditionally.
func jsonTagsOf(t *testing.T, value any) (emitted, mandatory []string) {
	t.Helper()
	structType := reflect.TypeOf(value)
	for index := range structType.NumField() {
		field := structType.Field(index)
		tag, ok := field.Tag.Lookup("json")
		if !ok {
			t.Fatalf("%s.%s has no json tag, so its property name would be the Go field name by accident", structType.Name(), field.Name)
		}
		parts := strings.Split(tag, ",")
		if parts[0] == "" || parts[0] == "-" {
			t.Fatalf("%s.%s has json tag %q, so the field never reaches a record", structType.Name(), field.Name, tag)
		}
		emitted = append(emitted, parts[0])
		if !slices.Contains(parts[1:], "omitempty") {
			mandatory = append(mandatory, parts[0])
		}
	}
	slices.Sort(emitted)
	slices.Sort(mandatory)
	return emitted, mandatory
}

// TestStatusValuesAreTheOnesTheSchemaDocumentsDeclare guards the two Status
// constants against the "status" enum in both documents, which must agree with
// each other as well as with Go: Supersede moves a Decision into a value that
// step 5 has to accept.
func TestStatusValuesAreTheOnesTheSchemaDocumentsDeclare(t *testing.T) {
	inGo := []string{string(StatusActive), string(StatusSuperseded)}

	for _, name := range []string{decisionSchemaName, invariantSchemaName} {
		declared := stringsAt(t, shippedDocument(t, name), "properties", "status", "enum")
		if !slices.Equal(inGo, declared) {
			t.Errorf("Go statuses = %v, %s declares %v", inGo, name, declared)
		}
	}
}

// TestSeverityValuesAreTheOnesTheInvariantSchemaDeclares guards the four
// Severity constants, in order: NewInvariant refuses anything outside the
// slice, and the slice is what its error message enumerates.
func TestSeverityValuesAreTheOnesTheInvariantSchemaDeclares(t *testing.T) {
	inGo := make([]string, 0, len(severities))
	for _, severity := range severities {
		inGo = append(inGo, string(severity))
	}

	declared := stringsAt(t, shippedDocument(t, invariantSchemaName), "properties", "severity", "enum")
	if !slices.Equal(inGo, declared) {
		t.Fatalf("Go severities = %v, %s declares %v", inGo, invariantSchemaName, declared)
	}
}

// TestScopeLevelsAreTheOnesTheSchemaDocumentsDeclare guards the five levels
// Scope.Validate checks against, in both documents. Scope.Validate is the rule
// internal/knowledge/validate's step 11 is meant to ask rather than reimplement,
// so a level Go accepts and the schema rejects would put the writer and the
// pipeline into disagreement about what a scope is.
func TestScopeLevelsAreTheOnesTheSchemaDocumentsDeclare(t *testing.T) {
	inGo := make([]string, 0, len(scopeLevels))
	for _, level := range scopeLevels {
		inGo = append(inGo, string(level))
	}

	for _, name := range []string{decisionSchemaName, invariantSchemaName} {
		declared := stringsAt(t, shippedDocument(t, name), "$defs", "scope", "properties", "level", "enum")
		if !slices.Equal(inGo, declared) {
			t.Errorf("Go scope levels = %v, %s declares %v", inGo, name, declared)
		}
	}
}

// TestKindConstantsAgreeWithTheDocumentsTheLoaderAndTheSchemaPackage holds the
// four spellings of the same two words together.
//
// "decision" and "invariant" now exist as this package's constants, as
// internal/knowledge/loader.RecordKind, as internal/knowledge/schema.RecordKind
// and as each document's "kind" const. The three Go copies exist because the
// packages may not import each other in the directions that would remove them;
// nothing but this test stops one of them from drifting, and a drifted kind
// would route a record to the wrong schema in silence.
func TestKindConstantsAgreeWithTheDocumentsTheLoaderAndTheSchemaPackage(t *testing.T) {
	tests := []struct {
		name     string
		document string
		inRecord string
		inLoader loader.RecordKind
		inSchema schema.RecordKind
	}{
		{
			name:     "decision",
			document: decisionSchemaName,
			inRecord: KindDecision,
			inLoader: loader.KindDecision,
			inSchema: schema.KindDecision,
		},
		{
			name:     "invariant",
			document: invariantSchemaName,
			inRecord: KindInvariant,
			inLoader: loader.KindInvariant,
			inSchema: schema.KindInvariant,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			declared := stringAt(t, shippedDocument(t, tt.document), "properties", "kind", "const")
			if tt.inRecord != declared {
				t.Errorf("record kind constant = %q, %s pins %q", tt.inRecord, tt.document, declared)
			}
			if string(tt.inLoader) != declared {
				t.Errorf("loader kind constant = %q, %s pins %q", tt.inLoader, tt.document, declared)
			}
			if string(tt.inSchema) != declared {
				t.Errorf("schema kind constant = %q, %s pins %q", tt.inSchema, tt.document, declared)
			}
			// SchemaNameFor derives the document name from the kind, so a kind
			// that drifted would also ask the validator for a document that
			// does not exist. Naming it here says which of the two failures a
			// reader is looking at.
			if got := schema.SchemaNameFor(tt.inSchema, writeVersion); got != tt.document {
				t.Errorf("SchemaNameFor(%q, %d) = %q, want %q", tt.inSchema, writeVersion, got, tt.document)
			}
		})
	}
}

// TestConstructorsStampTheVersionTheDocumentsAndTheRegistryAgreeOn keeps the
// local writeVersion equal to both the "const" the documents pin on
// schema_version and the version the registry says this binary writes. The
// constant is local because this package depends on nothing else under
// internal/knowledge (design §2); this is the price of that.
func TestConstructorsStampTheVersionTheDocumentsAndTheRegistryAgreeOn(t *testing.T) {
	for _, name := range []string{decisionSchemaName, invariantSchemaName} {
		declared, ok := valueAt(t, shippedDocument(t, name), "properties", "schema_version", "const").(float64)
		if !ok {
			t.Fatalf("%s schema_version const is not a number", name)
		}
		if declared != float64(writeVersion) {
			t.Errorf("writeVersion = %d, %s pins %v", writeVersion, name, declared)
		}
	}
	if schema.WriteVersion != writeVersion {
		t.Errorf("writeVersion = %d, schema.WriteVersion = %d", writeVersion, schema.WriteVersion)
	}
	if !slices.Contains(schema.ReadableVersions(), writeVersion) {
		t.Errorf("writeVersion = %d is outside the reader window %v, so this package writes records the loader refuses",
			writeVersion, schema.ReadableVersions())
	}
}
