package loader_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
)

// awkwardDecision is a document written the way a person writes one and not the
// way a machine re-encodes one: the keys are out of order, the whitespace is
// irregular, and "tags" repeats a value. Steps 5-11 judge the file the
// repository committed, so the fixture is chosen to be a document a decode and
// re-encode would visibly change — see the guard in
// TestRecordRefBodyIsTheExactBytesOnDisk, which refuses to run against a fixture
// that round-trips unchanged.
const awkwardDecision = `{
    "kind"   : "decision",
  "schema_version":1,
      "id" : "DEC-0007",
  "status":"active",

  "created_at": "2026-01-01T00:00:00Z",
  "title": "The body is handed over verbatim"   ,
  "decision": "Keep the bytes.",
  "tags": ["shape",  "shape"]
}
`

// TestRecordRefBodyIsTheExactBytesOnDisk is AC-04.1: RecordRef.Body holds the
// exact bytes step 1 read.
//
// Byte equality against the file, not a "contains" or a decoded comparison. A
// loader that decoded the document and re-encoded it from its own map would
// satisfy every looser assertion while quietly handing step 5 a document the
// repository never wrote — reordered keys, normalised whitespace, and one of
// the two duplicate tags gone, which is the difference between a uniqueItems
// finding and no finding at all.
func TestRecordRefBodyIsTheExactBytesOnDisk(t *testing.T) {
	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	writeRecord(t, worktree, "decisions", "DEC-0007.json", awkwardDecision)
	writeRecord(t, worktree, "invariants", "INV-0001.json", invariantJSON("INV-0001", 1))

	// The fixture is only an assertion if a re-encode would change it. Without
	// this line a later edit could reduce awkwardDecision to something
	// json.Marshal reproduces byte for byte, and the test would keep passing
	// against the very implementation it exists to reject.
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(awkwardDecision), &decoded); err != nil {
		t.Fatalf("the fixture is not a JSON object: %v", err)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-encoding the fixture: %v", err)
	}
	if string(reencoded) == awkwardDecision {
		t.Fatal("the fixture survives a re-encode unchanged, so it cannot tell verbatim bytes from re-encoded ones")
	}

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(store.Decisions) != 1 || len(store.Invariants) != 1 {
		t.Fatalf("store = %s / %s, want one of each (problems: %v)",
			describeRefs(store.Decisions), describeRefs(store.Invariants), store.Problems)
	}

	if got := string(store.Decisions[0].Body); got != awkwardDecision {
		t.Errorf("Decisions[0].Body =\n%q\nwant the file's bytes\n%q", got, awkwardDecision)
	}
	if got, want := string(store.Invariants[0].Body), invariantJSON("INV-0001", 1); got != want {
		t.Errorf("Invariants[0].Body =\n%q\nwant the file's bytes\n%q", got, want)
	}
}

// TestBodyIsCarriedForEveryRecordTheLoaderAccepted closes the gap the test above
// leaves: one populated body proves the field is reachable, not that the loader
// fills it on every branch it returns a reference from. A record read through a
// different bucket, or a second record in the same bucket, must carry its own
// bytes rather than a nil left behind by a loop that filled only the first.
func TestBodyIsCarriedForEveryRecordTheLoaderAccepted(t *testing.T) {
	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)

	want := map[string]string{
		".mindrail/knowledge/decisions/DEC-0001.json":  decisionJSON("DEC-0001", 1),
		".mindrail/knowledge/decisions/DEC-0002.json":  decisionJSON("DEC-0002", 1),
		".mindrail/knowledge/invariants/INV-0001.json": invariantJSON("INV-0001", 1),
		".mindrail/knowledge/invariants/INV-0002.json": invariantJSON("INV-0002", 1),
	}
	writeRecord(t, worktree, "decisions", "DEC-0001.json", want[".mindrail/knowledge/decisions/DEC-0001.json"])
	writeRecord(t, worktree, "decisions", "DEC-0002.json", want[".mindrail/knowledge/decisions/DEC-0002.json"])
	writeRecord(t, worktree, "invariants", "INV-0001.json", want[".mindrail/knowledge/invariants/INV-0001.json"])
	writeRecord(t, worktree, "invariants", "INV-0002.json", want[".mindrail/knowledge/invariants/INV-0002.json"])

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	refs := slices.Concat(store.Decisions, store.Invariants)
	if len(refs) != len(want) {
		t.Fatalf("loaded %d records, want %d (problems: %v)", len(refs), len(want), store.Problems)
	}
	for _, ref := range refs {
		expected, known := want[ref.Path]
		if !known {
			t.Errorf("loaded an unexpected record at %q", ref.Path)
			continue
		}
		if got := string(ref.Body); got != expected {
			t.Errorf("%s Body = %q, want %q", ref.Path, got, expected)
		}
	}
}

// TestEachRecordBodyIsIndependentlyOwned is the aliasing question asked as a
// test rather than answered by reading os.ReadFile once.
//
// os.ReadFile allocates a fresh slice per call today, which is why readRecord
// stores its result without copying. That is a property of an implementation
// this package does not own, and "read every record into one reused buffer" is
// the obvious allocation win somebody will eventually propose. If the bytes were
// ever shared, every RecordRef would end up describing the last file read, and
// step 5 would report the same findings against every record in the store.
func TestEachRecordBodyIsIndependentlyOwned(t *testing.T) {
	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	writeRecord(t, worktree, "decisions", "DEC-0001.json", decisionJSON("DEC-0001", 1))
	writeRecord(t, worktree, "decisions", "DEC-0002.json", decisionJSON("DEC-0002", 1))
	writeRecord(t, worktree, "invariants", "INV-0001.json", invariantJSON("INV-0001", 1))

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(store.Decisions) != 2 || len(store.Invariants) != 1 {
		t.Fatalf("store = %s / %s, want two decisions and one invariant",
			describeRefs(store.Decisions), describeRefs(store.Invariants))
	}

	untouched := map[string]string{
		store.Decisions[1].Path:  string(store.Decisions[1].Body),
		store.Invariants[0].Path: string(store.Invariants[0].Body),
	}

	// Scribble over the whole of one body, including any capacity beyond it: a
	// shared buffer would show up as either an overwritten neighbour or an
	// altered file.
	scribbled := store.Decisions[0].Body[:cap(store.Decisions[0].Body)]
	for i := range scribbled {
		scribbled[i] = 'X'
	}

	for path, before := range untouched {
		var after string
		switch path {
		case store.Decisions[1].Path:
			after = string(store.Decisions[1].Body)
		default:
			after = string(store.Invariants[0].Body)
		}
		if after != before {
			t.Errorf("%s Body changed when another record's body was written to:\ngot  %q\nwant %q", path, after, before)
		}
	}

	onDisk, err := os.ReadFile(filepath.Join(worktree, ".mindrail", "knowledge", "decisions", "DEC-0001.json"))
	if err != nil {
		t.Fatalf("re-reading the scribbled record: %v", err)
	}
	if got, want := string(onDisk), decisionJSON("DEC-0001", 1); got != want {
		t.Errorf("writing to Body reached the file: %q, want %q", got, want)
	}
}

// populatedStore is the fixture the wire-shape assertions marshal. It is
// populated on every branch that has a shape — a decision, an invariant and a
// problem — because a key set asserted over an empty slice proves nothing about
// the element type inside it.
func populatedStore() loader.Store {
	return loader.Store{
		Present:   true,
		Root:      loader.StoreRoot,
		Decisions: []loader.RecordRef{decisionRef()},
		Invariants: []loader.RecordRef{{
			Kind:          loader.KindInvariant,
			ID:            "INV-0001",
			Path:          ".mindrail/knowledge/invariants/INV-0001.json",
			SchemaVersion: 1,
		}},
		Problems: []loader.Problem{{
			Path:    ".mindrail/knowledge/decisions/DEC-0002.json",
			Code:    app.CodeKnowledgeUnreadable,
			Message: ".mindrail/knowledge/decisions/DEC-0002.json is not a JSON object",
			Fatal:   false,
		}},
		WriteSchemaVersion:     1,
		ReadableSchemaVersions: []int{1},
	}
}

// TestStoreMarshalsToThePreMR002Shape is AC-04.2 stated as a byte comparison.
//
// The document below was captured from the tree before RecordRef gained Body
// (decision D-46) and is frozen here by hand rather than regenerated: a golden
// a later change can refresh would accept the very drift this test exists to
// refuse. The store is repository-owned content and this is its published
// shape, so anything that widens it — a field without a tag, a `json:"-"` that
// silently becomes `json:"body"` — has to be a deliberate edit to this literal.
func TestStoreMarshalsToThePreMR002Shape(t *testing.T) {
	encoded, err := json.Marshal(populatedStore())
	if err != nil {
		t.Fatalf("Marshal(store): %v", err)
	}

	want := `{"present":true,"root":".mindrail/knowledge",` +
		`"decisions":[{"kind":"decision","id":"DEC-0001",` +
		`"path":".mindrail/knowledge/decisions/DEC-0001.json","schema_version":1}],` +
		`"invariants":[{"kind":"invariant","id":"INV-0001",` +
		`"path":".mindrail/knowledge/invariants/INV-0001.json","schema_version":1}],` +
		`"problems":[{"path":".mindrail/knowledge/decisions/DEC-0002.json",` +
		// Spelled out rather than interpolated from app.CodeKnowledgeUnreadable.
		// Interpolating it would compare the constant with itself and pass
		// whatever it were changed to, and AC-12.4 says no existing code value
		// changes — a document frozen against a moving constant is not frozen.
		`"code":"KNOWLEDGE_UNREADABLE",` +
		`"message":".mindrail/knowledge/decisions/DEC-0002.json is not a JSON object",` +
		`"fatal":false}],` +
		`"write_schema_version":1,"readable_schema_versions":[1]}`

	if string(encoded) != want {
		t.Errorf("Marshal(store) =\n\t%s\nwant\n\t%s", encoded, want)
	}
}

// TestStoreJSONKeySetIsUnchangedByTheBody is the same criterion asserted as a
// set rather than as a document, so a failure names the offending key instead
// of leaving a reader to diff two long lines.
//
// Every level is checked against a written-out list, not against a prefix or a
// substring: `json:"-"` turning into `json:"body"` adds a key that no substring
// assertion over the encoded document would notice, and that is exactly the
// regression D-46 is one edit away from.
func TestStoreJSONKeySetIsUnchangedByTheBody(t *testing.T) {
	encoded, err := json.Marshal(populatedStore())
	if err != nil {
		t.Fatalf("Marshal(store): %v", err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &top); err != nil {
		t.Fatalf("Unmarshal(store JSON): %v", err)
	}

	wantTop := []string{
		"decisions", "invariants", "present", "problems",
		"readable_schema_versions", "root", "write_schema_version",
	}
	if got := sortedKeys(top); !slices.Equal(got, wantTop) {
		t.Errorf("store keys = %v, want %v", got, wantTop)
	}

	// The record shape is what D-46 puts at risk, so both buckets are read:
	// decisions and invariants are the same Go type, and a test that inspected
	// only one would still pass if a later change gave them different ones.
	wantRecord := []string{"id", "kind", "path", "schema_version"}
	for _, bucket := range []string{"decisions", "invariants"} {
		for i, object := range decodeObjects(t, top[bucket]) {
			if got := sortedKeys(object); !slices.Equal(got, wantRecord) {
				t.Errorf("%s[%d] keys = %v, want %v", bucket, i, got, wantRecord)
			}
		}
	}

	wantProblem := []string{"code", "fatal", "message", "path"}
	for i, object := range decodeObjects(t, top["problems"]) {
		if got := sortedKeys(object); !slices.Equal(got, wantProblem) {
			t.Errorf("problems[%d] keys = %v, want %v", i, got, wantProblem)
		}
	}
}

// TestNoStoreJSONKeyIsSpeltBody sweeps the whole encoded document rather than
// the three levels above, because the body must not reach any wire — not under
// the name D-46 gives it and not smuggled through some nested object a later
// milestone adds. The comparison is case-folded so an untagged `Body` field,
// which encoding/json publishes under its Go name, is caught by the same line.
func TestNoStoreJSONKeyIsSpeltBody(t *testing.T) {
	encoded, err := json.Marshal(populatedStore())
	if err != nil {
		t.Fatalf("Marshal(store): %v", err)
	}

	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("Unmarshal(store JSON): %v", err)
	}

	keys := everyKey(document)
	if len(keys) == 0 {
		t.Fatal("collected no keys at all; the sweep is not reading the document")
	}
	for _, key := range keys {
		if strings.EqualFold(key, "body") {
			t.Errorf("the encoded store carries a %q key; RecordRef.Body must stay off the wire (D-46)", key)
		}
	}
}

// everyKey returns every object key in a decoded JSON document, at every depth.
func everyKey(node any) []string {
	switch typed := node.(type) {
	case map[string]any:
		keys := []string{}
		for key, value := range typed {
			keys = append(keys, key)
			keys = append(keys, everyKey(value)...)
		}
		return keys
	case []any:
		keys := []string{}
		for _, value := range typed {
			keys = append(keys, everyKey(value)...)
		}
		return keys
	default:
		return nil
	}
}

func sortedKeys(object map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func decodeObjects(t *testing.T, raw json.RawMessage) []map[string]json.RawMessage {
	t.Helper()

	var objects []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &objects); err != nil {
		t.Fatalf("Unmarshal(array of objects): %v", err)
	}
	if len(objects) == 0 {
		t.Fatal("the fixture bucket is empty, so its element shape was never asserted")
	}
	return objects
}

// TestRecordRefFieldsAreExactlyTheDeclaredContract pins AC-04.1 structurally.
//
// The table is the contract written out: the field that carries the bytes is
// named Body, is []byte, and is excluded from JSON. Reflection rather than a
// value assertion, because the tag is the whole of D-46's "the wire shape does
// not change" clause and a tag is invisible to every other kind of test. It
// also catches a field added by a later milestone without a matching line in
// sameRef, which would otherwise silently narrow every reference comparison in
// this package.
func TestRecordRefFieldsAreExactlyTheDeclaredContract(t *testing.T) {
	want := []struct {
		name string
		kind string
		tag  string
	}{
		{name: "Kind", kind: "loader.RecordKind", tag: `json:"kind"`},
		{name: "ID", kind: "string", tag: `json:"id"`},
		{name: "Path", kind: "string", tag: `json:"path"`},
		{name: "SchemaVersion", kind: "int", tag: `json:"schema_version"`},
		{name: "Body", kind: "[]uint8", tag: `json:"-"`},
	}

	refType := reflect.TypeOf(loader.RecordRef{})
	if got := refType.NumField(); got != len(want) {
		t.Fatalf("RecordRef has %d fields, want %d: %v", got, len(want), fieldNames(refType))
	}

	for i, expected := range want {
		field := refType.Field(i)
		if field.Name != expected.name {
			t.Errorf("field %d is named %q, want %q", i, field.Name, expected.name)
			continue
		}
		if got := field.Type.String(); got != expected.kind {
			t.Errorf("field %s has type %s, want %s", field.Name, got, expected.kind)
		}
		if got := string(field.Tag); got != expected.tag {
			t.Errorf("field %s has tag `%s`, want `%s`", field.Name, got, expected.tag)
		}
	}
}

func fieldNames(structType reflect.Type) []string {
	names := make([]string, 0, structType.NumField())
	for i := range structType.NumField() {
		names = append(names, structType.Field(i).Name)
	}
	return names
}

// decisionRef is the decision populatedStore carries. Its Body is populated on
// purpose: a nil body under a `json:"body"` tag encodes as null, which is still
// a key and would still fail the shape assertions, but only for a reason a
// reader would have to reconstruct. A non-empty body makes the mutation visible
// in the failure message itself.
func decisionRef() loader.RecordRef {
	return loader.RecordRef{
		Kind:          loader.KindDecision,
		ID:            "DEC-0001",
		Path:          ".mindrail/knowledge/decisions/DEC-0001.json",
		SchemaVersion: 1,
		Body:          []byte(`{"schema_version":1,"kind":"decision","id":"DEC-0001"}`),
	}
}

// sameRef and describeRefs exist because D-46 made RecordRef non-comparable:
// []byte has no ==, so slices.Equal no longer accepts []RecordRef and %+v
// prints a body as a list of decimal byte values. Comparing every field by hand
// keeps the assertions that used slices.Equal exactly as exhaustive as the ==
// they lost, and TestRecordRefFieldsAreExactlyTheDeclaredContract is what fails
// if a later field is added without a line here.
func sameRef(a, b loader.RecordRef) bool {
	return a.Kind == b.Kind &&
		a.ID == b.ID &&
		a.Path == b.Path &&
		a.SchemaVersion == b.SchemaVersion &&
		string(a.Body) == string(b.Body)
}

func describeRefs(refs []loader.RecordRef) string {
	described := make([]string, 0, len(refs))
	for _, ref := range refs {
		described = append(described, fmt.Sprintf("{Kind:%s ID:%s Path:%s SchemaVersion:%d Body:%q}",
			ref.Kind, ref.ID, ref.Path, ref.SchemaVersion, ref.Body))
	}
	return "[" + strings.Join(described, " ") + "]"
}
