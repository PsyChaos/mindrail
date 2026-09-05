package schema_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/schemas"
)

// TestEmbeddedSchemasPresent pins the tech-stack §39 promise that a released
// binary carries its schemas: the registry must build from the embedded FS
// alone, with no filesystem or network access.
func TestEmbeddedSchemasPresent(t *testing.T) {
	registry, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded) error = %v, want nil", err)
	}

	wantNames := []string{schema.DecisionSchemaName, schema.InvariantSchemaName}
	if got := registry.Names(); !slices.Equal(got, wantNames) {
		t.Fatalf("Names() = %v, want %v", got, wantNames)
	}

	for _, name := range wantNames {
		doc, ok := registry.Document(name)
		if !ok {
			t.Fatalf("Document(%q) not found", name)
		}
		if len(doc) == 0 {
			t.Fatalf("Document(%q) is empty", name)
		}

		var decoded map[string]any
		if err := json.Unmarshal(doc, &decoded); err != nil {
			t.Fatalf("Document(%q) is not valid JSON: %v", name, err)
		}
		if got := decoded["$schema"]; got != "https://json-schema.org/draft/2020-12/schema" {
			t.Errorf("Document(%q) $schema = %v, want the Draft 2020-12 dialect", name, got)
		}
	}

	if _, ok := registry.Document("no-such.schema.json"); ok {
		t.Error("Document(unknown) reported found")
	}
}

// TestEmbeddedSchemaDocumentIsACopy guards the registry against a caller that
// edits the slice it was handed: the embedded documents back every later
// validation in the process.
func TestEmbeddedSchemaDocumentIsACopy(t *testing.T) {
	registry, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded) error = %v", err)
	}

	first, _ := registry.Document(schema.DecisionSchemaName)
	original := first[0]
	first[0] = 'X'

	second, _ := registry.Document(schema.DecisionSchemaName)
	if second[0] != original {
		t.Fatalf("Document returned shared storage: byte 0 = %q after caller mutation, want %q", second[0], original)
	}
}

// TestVersionWindowIsWriteOneReadableOne pins kernel-scope §3: 0.1 declares
// write_schema_version = 1 and readable_schema_versions = [1].
func TestVersionWindowIsWriteOneReadableOne(t *testing.T) {
	if schema.WriteVersion != 1 {
		t.Errorf("WriteVersion = %d, want 1", schema.WriteVersion)
	}
	if got := schema.ReadableVersions(); !slices.Equal(got, []int{1}) {
		t.Errorf("ReadableVersions() = %v, want [1]", got)
	}

	registry, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded) error = %v", err)
	}
	if got := registry.WriteVersion(); got != schema.WriteVersion {
		t.Errorf("Registry.WriteVersion() = %d, want %d", got, schema.WriteVersion)
	}
	if got := registry.ReadableVersions(); !slices.Equal(got, []int{1}) {
		t.Errorf("Registry.ReadableVersions() = %v, want [1]", got)
	}

	// The package-level window must not be reachable through the accessor.
	window := registry.ReadableVersions()
	window[0] = 99
	if got := registry.ReadableVersions(); !slices.Equal(got, []int{1}) {
		t.Errorf("ReadableVersions() = %v after caller mutation, want [1]", got)
	}
}

// TestSupportsRejectsNewerVersion is the kernel-scope §3 fail-closed branch:
// anything outside the reader window is unreadable, not best-effort readable.
func TestSupportsRejectsNewerVersion(t *testing.T) {
	registry, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded) error = %v", err)
	}

	tests := []struct {
		name    string
		version int
		want    bool
	}{
		{name: "current write version", version: 1, want: true},
		{name: "next major version", version: 2, want: false},
		{name: "far future version", version: 99, want: false},
		{name: "pre-history version", version: 0, want: false},
		{name: "negative version", version: -1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registry.Supports(tt.version); got != tt.want {
				t.Errorf("Supports(%d) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}

	if schema.ErrUnsupportedVersion == nil {
		t.Fatal("ErrUnsupportedVersion sentinel is nil")
	}
	wrapped := errors.New("outer: " + schema.ErrUnsupportedVersion.Error())
	if errors.Is(wrapped, schema.ErrUnsupportedVersion) {
		t.Fatal("a string-formatted error must not satisfy errors.Is; the sentinel has to be wrapped with %w")
	}
}

// TestNewRegistryRejectsIncompleteOrCorruptFS proves the constructor is the
// place a packaging mistake surfaces: a binary missing a schema must fail at
// construction, not at the first record it cannot explain.
func TestNewRegistryRejectsIncompleteOrCorruptFS(t *testing.T) {
	validDecision := mustReadEmbedded(t, schema.DecisionSchemaName)
	validInvariant := mustReadEmbedded(t, schema.InvariantSchemaName)

	tests := []struct {
		name string
		fsys fstest.MapFS
	}{
		{
			name: "empty filesystem",
			fsys: fstest.MapFS{},
		},
		{
			name: "invariant document missing",
			fsys: fstest.MapFS{
				"knowledge/" + schema.DecisionSchemaName: {Data: validDecision},
			},
		},
		{
			name: "decision document missing",
			fsys: fstest.MapFS{
				"knowledge/" + schema.InvariantSchemaName: {Data: validInvariant},
			},
		},
		{
			name: "document is not JSON",
			fsys: fstest.MapFS{
				"knowledge/" + schema.DecisionSchemaName:  {Data: []byte("{ not json")},
				"knowledge/" + schema.InvariantSchemaName: {Data: validInvariant},
			},
		},
		{
			name: "document is empty",
			fsys: fstest.MapFS{
				"knowledge/" + schema.DecisionSchemaName:  {Data: []byte{}},
				"knowledge/" + schema.InvariantSchemaName: {Data: validInvariant},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := schema.NewRegistry(tt.fsys); err == nil {
				t.Fatal("NewRegistry() error = nil, want an error")
			}
		})
	}
}

// TestNewRegistryAcceptsExtraDocuments keeps the registry additive: a future
// schema version shipped alongside v1 must not break a v1 binary's startup.
func TestNewRegistryAcceptsExtraDocuments(t *testing.T) {
	fsys := fstest.MapFS{
		"knowledge/" + schema.DecisionSchemaName:  {Data: mustReadEmbedded(t, schema.DecisionSchemaName)},
		"knowledge/" + schema.InvariantSchemaName: {Data: mustReadEmbedded(t, schema.InvariantSchemaName)},
		"knowledge/decision.v2.schema.json":       {Data: []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema"}`)},
	}

	registry, err := schema.NewRegistry(fsys)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v, want nil", err)
	}

	want := []string{schema.DecisionSchemaName, "decision.v2.schema.json", schema.InvariantSchemaName}
	if got := registry.Names(); !slices.Equal(got, want) {
		t.Fatalf("Names() = %v, want %v (sorted)", got, want)
	}
}

func mustReadEmbedded(t *testing.T, name string) []byte {
	t.Helper()
	data, err := schemas.KnowledgeFS.ReadFile("knowledge/" + name)
	if err != nil {
		t.Fatalf("reading embedded %q: %v", name, err)
	}
	return data
}
