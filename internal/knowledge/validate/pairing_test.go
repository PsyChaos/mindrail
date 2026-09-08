package validate_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
	"github.com/PsyChaos/mindrail/schemas"
)

// The three records TestLoadDoesNotPerformMR002Validation writes, copied
// verbatim from internal/knowledge/loader/loader_test.go:283-291 and its
// decisionJSON helper at :490.
//
// Copied rather than shared because the loader's fixtures live in package
// loader_test and no other package can import them. That is exactly why the
// pairing test below re-runs the loader over them and re-asserts the loader's
// own half of the claim: if the fixtures on that side change, the loader
// assertions here fail, and if this side's expectations drift, the step
// assertions fail. Neither side can move alone.
const (
	// Step 6: the file name disagrees with the id inside it.
	pairedDecisionOne = `{
  "schema_version": 1,
  "kind": "decision",
  "id": "DEC-9999",
  "status": "active",
  "created_at": "2026-01-01T00:00:00Z",
  "title": "Test decision",
  "decision": "Do the thing."
}`
	// Step 7: a duplicate id. Identical bytes, a different file name.
	pairedDecisionTwo = pairedDecisionOne
	// Steps 5 and 8: fields the schema requires are missing, and the supersede
	// target does not exist.
	pairedInvariant = `{"schema_version": 1, "id": "INV-0001", "supersedes": ["INV-4242"]}`
)

// TestTheRecordsTheLoaderIgnoresAreTheOnesTheValidatorFinds is AC-04.4.
//
// MR-001 stopped at step 4 and proved it by loading three broken records without
// a single problem. This is the other half of that pair: the same three records,
// through the real loader, and then through the pipeline that owns steps 5-11.
// Either side drifting turns this red.
func TestTheRecordsTheLoaderIgnoresAreTheOnesTheValidatorFinds(t *testing.T) {
	worktree := t.TempDir()
	writeKnowledgeRecord(t, worktree, "decisions", "DEC-0001.json", pairedDecisionOne)
	writeKnowledgeRecord(t, worktree, "decisions", "DEC-0002.json", pairedDecisionTwo)
	writeKnowledgeRecord(t, worktree, "invariants", "INV-0001.json", pairedInvariant)

	store := loadStore(t, worktree)

	// The loader's half, restated here so this test fails if MR-001's scope line
	// moves. Steps 5-14 are not the loader's, so all three records load clean.
	if len(store.Problems) != 0 {
		t.Fatalf("the loader recorded problems for records it is meant to ignore: %+v", store.Problems)
	}
	if store.Count() != 3 {
		t.Fatalf("the loader read %d records, want 3", store.Count())
	}

	findings := validate.Check(store, shippedValidator(t))

	const (
		decOne    = ".mindrail/knowledge/decisions/DEC-0001.json"
		decTwo    = ".mindrail/knowledge/decisions/DEC-0002.json"
		invariant = ".mindrail/knowledge/invariants/INV-0001.json"
	)

	// Both decisions carry DEC-9999, so neither file name agrees with its id and
	// both are duplicates of each other.
	for _, tt := range []struct {
		step validate.Step
		want []string
	}{
		{validate.StepFilenameConsistency, []string{decOne, decTwo}},
		{validate.StepUniqueID, []string{decOne, decTwo}},
		{validate.StepSchema, []string{invariant}},
	} {
		if got := pathsAt(findings, tt.step); !slices.Equal(got, tt.want) {
			t.Errorf("step %d reported %v, want %v\n\t%s", tt.step, got, tt.want, describe(findings))
		}
	}

	// The invariant's dangling supersede target is *not* reported, and that is
	// decision D-40 rather than an omission: the record failed step 5, so steps
	// 6-11 are not asked about fields step 5 has just said are wrong. The loader
	// test names this record for steps 5 and 8; the pipeline answers step 5, and
	// step 8 becomes reachable for it only once its required fields are there.
	if got := stepsAt(findings, invariant); !slices.Equal(got, []validate.Step{validate.StepSchema}) {
		t.Errorf("the invariant reported steps %v, want [5] only (D-40)\n\t%s", got, describe(findings))
	}

	// And nothing beyond those. Two files carrying one id is a naming mistake,
	// not a forked lineage: a step 10 finding here would be a second diagnosis of
	// a condition step 7 has already named.
	wantSteps := []validate.Step{validate.StepFilenameConsistency, validate.StepUniqueID}
	for _, recordPath := range []string{decOne, decTwo} {
		if got := stepsAt(findings, recordPath); !slices.Equal(got, wantSteps) {
			t.Errorf("%s reported steps %v, want %v\n\t%s", recordPath, got, wantSteps, describe(findings))
		}
	}
}

// TestTheFixtureStoreIsTheOneTheLoaderProduces is what lets every other test in
// this package build its store in memory.
//
// Check opens nothing (decision D-41), so a filesystem in each test would be
// ceremony — but a hand-built Store is only evidence if it is the Store a real
// load produces. This builds the same three records both ways and compares every
// field, including the body bytes decision D-46 carries.
func TestTheFixtureStoreIsTheOneTheLoaderProduces(t *testing.T) {
	records := []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"supersedes": []any{"DEC-0000"},
			"scope":      map[string]any{"level": "FILE", "target": "internal/app/code.go"},
		})),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", nil)),
		invariantAt("INV-0001.json", invariantDoc("INV-0001", nil)),
	}

	worktree := t.TempDir()
	for _, rec := range records {
		dir := "decisions"
		if rec.kind == loader.KindInvariant {
			dir = "invariants"
		}
		writeKnowledgeRecord(t, worktree, dir, rec.name, rec.body)
	}

	loaded := loadStore(t, worktree)
	built := storeOf(t, records)

	assertSameRefs(t, "decisions", loaded.Decisions, built.Decisions)
	assertSameRefs(t, "invariants", loaded.Invariants, built.Invariants)

	if loaded.Present != built.Present || loaded.Root != built.Root {
		t.Errorf("loaded {Present:%v Root:%q}, built {Present:%v Root:%q}",
			loaded.Present, loaded.Root, built.Present, built.Root)
	}
	if len(loaded.Problems) != 0 {
		t.Errorf("the loader refused a fixture record: %+v", loaded.Problems)
	}
}

func assertSameRefs(t *testing.T, bucket string, loaded, built []loader.RecordRef) {
	t.Helper()

	if len(loaded) != len(built) {
		t.Fatalf("%s: the loader produced %d refs, the fixture %d", bucket, len(loaded), len(built))
	}
	for i := range loaded {
		got, want := loaded[i], built[i]
		if got.Kind != want.Kind || got.ID != want.ID || got.Path != want.Path || got.SchemaVersion != want.SchemaVersion {
			t.Errorf("%s[%d]: loader %v, fixture %v",
				bucket, i,
				[]any{got.Kind, got.ID, got.Path, got.SchemaVersion},
				[]any{want.Kind, want.ID, want.Path, want.SchemaVersion})
		}
		if string(got.Body) != string(want.Body) {
			t.Errorf("%s[%d] %s: the loader carried %q, the fixture %q",
				bucket, i, got.Path, got.Body, want.Body)
		}
	}
}

// TestEveryRecordTheConstructorsProduceSurvivesTheWholePipeline is AC-01.5 and
// AC-11.5, and it is the pairing design §5 exists for: a writer whose own
// validator rejects its output is a defect neither side can see alone.
//
// The four records are a small consistent store rather than four isolated
// values, because three of the seven steps are about a set: a record built with
// WithSupersedes fails step 8 unless the record it names is written too, and a
// lineage whose older member is still active fails step 10.
func TestEveryRecordTheConstructorsProduceSurvivesTheWholePipeline(t *testing.T) {
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	bare, err := record.NewDecision("DEC-0001", createdAt, "The first decision", "Do the first thing.")
	if err != nil {
		t.Fatalf("NewDecision(bare): %v", err)
	}
	full, err := record.NewDecision("DEC-0002", createdAt, "The second decision", "Do the second thing.",
		record.WithContext("The situation that forced the choice."),
		record.WithConsequences("What the choice costs."),
		record.WithScope(record.Scope{Level: record.ScopeFile, Target: "internal/app/code.go"}),
		record.WithTags("alpha", "beta"),
	)
	if err != nil {
		t.Fatalf("NewDecision(full): %v", err)
	}
	// Bound by name: Supersede returns (replacement, prior), the reverse of its
	// parameters, and both are Decision, so a swapped call site compiles.
	carrying, superseded, err := record.Supersede(bare, full)
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}

	bareInvariant, err := record.NewInvariant("INV-0001", createdAt, "The first thing stays true.",
		record.WithSeverity(record.SeverityHigh),
		record.WithScope(record.Scope{Level: record.ScopeProject}),
	)
	if err != nil {
		t.Fatalf("NewInvariant(bare): %v", err)
	}
	// The package ships no SupersedeInvariant helper — record.Supersede covers
	// Decisions only, matching the frozen signature — so the older Invariant's
	// status is moved here. Leaving it active would be a real step 10 finding
	// rather than a defect in the constructors, and this test is about the
	// constructors.
	bareInvariant.Status = record.StatusSuperseded

	fullInvariant, err := record.NewInvariant("INV-0002", createdAt, "The second thing stays true.",
		record.WithSeverity(record.SeverityCritical),
		record.WithScope(record.Scope{Level: record.ScopeModule, Target: "internal/knowledge"}),
		record.WithRationale("Why violating the statement is harmful."),
		record.WithSupersedes("INV-0001"),
		record.WithTags("gamma"),
	)
	if err != nil {
		t.Fatalf("NewInvariant(full): %v", err)
	}

	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", encodeRecord(t, superseded)),
		decisionAt("DEC-0002.json", encodeRecord(t, carrying)),
		invariantAt("INV-0001.json", encodeRecord(t, bareInvariant)),
		invariantAt("INV-0002.json", encodeRecord(t, fullInvariant)),
	})

	if findings := validate.Check(store, shippedValidator(t)); len(findings) != 0 {
		t.Errorf("the constructors produced records their own pipeline rejects:\n\t%s", describe(findings))
	}

	// Without this the test would pass on four records carrying nothing but the
	// required properties, and "one with every optional field set" would be a
	// comment rather than a fact. The expectation is read out of the schema
	// documents, so a property added to either kind has to appear in a fixture
	// here before this test goes green again.
	registry, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded): %v", err)
	}
	for _, tt := range []struct {
		name     string
		document string
		value    any
		want     string
	}{
		{"the full decision", schema.DecisionSchemaName, carrying, "properties"},
		{"the bare decision", schema.DecisionSchemaName, superseded, "required"},
		{"the full invariant", schema.InvariantSchemaName, fullInvariant, "properties"},
		{"the bare invariant", schema.InvariantSchemaName, bareInvariant, "required"},
	} {
		want := schemaKeys(t, registry, tt.document, tt.want)
		got := documentKeys(t, encodeRecord(t, tt.value))
		if !slices.Equal(got, want) {
			t.Errorf("%s emits %v, want %s's %q set %v", tt.name, got, tt.document, tt.want, want)
		}
	}
}

// encodeRecord renders a record the way a writer would.
//
// SetEscapeHTML(false) is what MR-014's writer will need: encoding/json escapes
// '&', '<' and '>' by default, and repository content is diffed and reviewed by
// people. It is set here so that this test's records are the bytes a writer
// would commit rather than a variant only this test produces.
func encodeRecord(t *testing.T, value any) string {
	t.Helper()

	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		t.Fatalf("encoding %T: %v", value, err)
	}
	// Encode appends a newline; whether a record file ends in one is the writer's
	// business and not something the schema or this pipeline judges.
	return strings.TrimSuffix(buffer.String(), "\n")
}

// documentKeys returns the JSON member names a record marshalled to, sorted.
func documentKeys(t *testing.T, document string) []string {
	t.Helper()

	var members map[string]json.RawMessage
	if err := json.Unmarshal([]byte(document), &members); err != nil {
		t.Fatalf("decoding %s: %v", document, err)
	}
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// schemaKeys reads a keyword's member names out of a shipped schema document:
// "properties" for every property the kind defines, "required" for the ones it
// insists on.
func schemaKeys(t *testing.T, registry *schema.Registry, document, keyword string) []string {
	t.Helper()

	raw, ok := registry.Document(document)
	if !ok {
		t.Fatalf("the registry does not ship %s", document)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decoding %s: %v", document, err)
	}

	var names []string
	switch keyword {
	case "properties":
		var properties map[string]json.RawMessage
		if err := json.Unmarshal(decoded[keyword], &properties); err != nil {
			t.Fatalf("decoding %s %s: %v", document, keyword, err)
		}
		for name := range properties {
			names = append(names, name)
		}
	case "required":
		if err := json.Unmarshal(decoded[keyword], &names); err != nil {
			t.Fatalf("decoding %s %s: %v", document, keyword, err)
		}
	default:
		t.Fatalf("no rule for reading %q out of a schema document", keyword)
	}

	if len(names) == 0 {
		t.Fatalf("%s declares no %s, so the comparison would be vacuous", document, keyword)
	}
	slices.Sort(names)
	return names
}

// loadStore runs the real loader over a worktree.
func loadStore(t *testing.T, worktree string) loader.Store {
	t.Helper()

	root, err := filesystem.NewRoot(worktree)
	if err != nil {
		t.Fatalf("NewRoot(%q): %v", worktree, err)
	}
	registry, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded): %v", err)
	}

	store, err := loader.New(root, registry).Load(context.Background())
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	return store
}

// writeKnowledgeRecord puts one record file where the loader walks for it.
func writeKnowledgeRecord(t *testing.T, worktree, subdir, name, contents string) {
	t.Helper()

	dir := filepath.Join(worktree, ".mindrail", "knowledge", subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", filepath.Join(dir, name), err)
	}
}
