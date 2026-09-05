// Package schema owns the knowledge JSON Schema documents and the reader
// compatibility window that decides which records this binary may read.
//
// MR-001 ships the documents and enforces the window; it deliberately does not
// compile or evaluate the schemas (decision D-28). Compiling them is MR-002's
// job, together with the acceptance criteria that justify the dependency, so a
// packaging mistake still fails here at construction while record-level
// enforcement stays with the milestone that owns it.
package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
)

// Dir is the sub-directory the documents live in, mirroring tech-stack §7's
// schemas/knowledge/ layout. NewRegistry reads from it so that the embedded
// schemas.KnowledgeFS can be passed straight through.
const Dir = "knowledge"

// The documents a 0.1 binary cannot start without. Knowledge records come in
// exactly these two kinds (spec §53, §54).
const (
	DecisionSchemaName  = "decision.v1.schema.json"
	InvariantSchemaName = "invariant.v1.schema.json"
)

// WriteVersion is the schema version this binary stamps on records it writes
// (kernel-scope §3).
const WriteVersion = 1

// readableVersions is the reader window. Every record carries schema_version
// from day one and an unknown version fails closed, because retrofitting the
// field later would mean rewriting every repository's knowledge store
// (kernel-scope §3).
var readableVersions = []int{1}

// ErrUnsupportedVersion reports a record this binary must refuse to interpret.
// Callers wrap it with %w so errors.Is keeps working across package
// boundaries; matching on the message is never control flow (tech-stack §72).
var ErrUnsupportedVersion = errors.New("knowledge schema_version not readable by this binary")

// requiredNames is the minimum set NewRegistry insists on.
var requiredNames = []string{DecisionSchemaName, InvariantSchemaName}

// ReadableVersions returns the schema versions this binary can read, sorted
// ascending. The result is a copy so the window cannot be edited in place.
func ReadableVersions() []int {
	return slices.Clone(readableVersions)
}

// Registry is an immutable view over the embedded schema documents.
type Registry struct {
	docs map[string][]byte
	// names is precomputed and sorted so callers get a stable order without
	// re-sorting the map on every call.
	names []string
}

// NewRegistry reads every <Dir>/*.json document out of fsys.
//
// It fails when a required document is missing or is not valid JSON: a binary
// that shipped without its schemas cannot validate anything, and finding that
// out at startup is far cheaper than finding it out per record. Documents
// beyond the required set are kept, so a future schema version can ship
// alongside v1 without breaking a v1 binary.
func NewRegistry(fsys fs.FS) (*Registry, error) {
	if fsys == nil {
		return nil, errors.New("knowledge schema registry: nil filesystem")
	}

	entries, err := fs.Glob(fsys, path.Join(Dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("knowledge schema registry: listing %s: %w", Dir, err)
	}

	docs := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		data, readErr := fs.ReadFile(fsys, entry)
		if readErr != nil {
			return nil, fmt.Errorf("knowledge schema registry: reading %s: %w", entry, readErr)
		}
		if !json.Valid(data) {
			return nil, fmt.Errorf("knowledge schema registry: %s is not valid JSON", entry)
		}
		docs[path.Base(entry)] = data
	}

	for _, name := range requiredNames {
		if _, ok := docs[name]; !ok {
			return nil, fmt.Errorf("knowledge schema registry: required document %s is missing", name)
		}
	}

	names := slices.Sorted(maps.Keys(docs))
	return &Registry{docs: docs, names: names}, nil
}

// Names returns every document name held, sorted. The result is a copy.
func (r *Registry) Names() []string {
	return slices.Clone(r.names)
}

// Document returns the raw JSON Schema document for name, which is a base file
// name such as "decision.v1.schema.json". The returned bytes are a copy: the
// registry backs every later validation in the process and must survive a
// caller that decodes into it or rewrites it.
func (r *Registry) Document(name string) ([]byte, bool) {
	doc, ok := r.docs[name]
	if !ok {
		return nil, false
	}
	return slices.Clone(doc), true
}

// WriteVersion returns the schema version records are written with.
func (r *Registry) WriteVersion() int { return WriteVersion }

// ReadableVersions returns the reader window, sorted ascending, as a copy.
func (r *Registry) ReadableVersions() []int { return ReadableVersions() }

// Supports reports whether a record stamped with version can be read. It fails
// closed: a version outside the window is unreadable rather than read on a
// best-effort basis, because a partially understood record is worse than an
// admitted one (kernel-scope §3).
func (r *Registry) Supports(version int) bool {
	return slices.Contains(readableVersions, version)
}
