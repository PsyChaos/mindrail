package validate_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
	"github.com/PsyChaos/mindrail/schemas"
)

// The frozen signature (design §3, AC-03.1), asserted by assignment to an
// explicit function type.
//
// A test that merely called Check would keep passing after a parameter was
// reordered or a return type widened, because the call site would have been
// updated in the same edit. A declared function type cannot be updated in
// passing: the package stops building, which is the loudest failure available.
var _ func(loader.Store, *schema.Validator) []validate.Finding = validate.Check

// timestamp is the created_at every fixture in this package carries. It is fixed
// rather than time.Now so that a finding is a statement about the record rather
// than about the clock.
const timestamp = "2026-01-01T00:00:00Z"

// testRecord is one file in a fixture store: which directory the loader would
// have walked it in, what it is called, and the bytes it holds.
type testRecord struct {
	kind loader.RecordKind
	name string
	body string
}

func decisionAt(name, body string) testRecord {
	return testRecord{kind: loader.KindDecision, name: name, body: body}
}

func invariantAt(name, body string) testRecord {
	return testRecord{kind: loader.KindInvariant, name: name, body: body}
}

// decisionDoc renders a decision document that satisfies decision.v1, with extra
// members merged in and named members dropped.
//
// Built from a map and marshalled rather than assembled with Sprintf so that a
// fixture which has to *omit* a required property says so — `drop("title")` —
// instead of being a second, hand-edited copy of the whole document that could
// differ from its siblings in ways the test did not intend.
func decisionDoc(id string, extra map[string]any, drop ...string) string {
	return renderDoc(map[string]any{
		"schema_version": 1,
		"kind":           "decision",
		"id":             id,
		"status":         "active",
		"created_at":     timestamp,
		"title":          "Test decision",
		"decision":       "Do the thing.",
	}, extra, drop)
}

// invariantDoc renders an invariant document that satisfies invariant.v1.
func invariantDoc(id string, extra map[string]any, drop ...string) string {
	return renderDoc(map[string]any{
		"schema_version": 1,
		"kind":           "invariant",
		"id":             id,
		"status":         "active",
		"created_at":     timestamp,
		"statement":      "The thing stays true.",
		"severity":       "CRITICAL",
		"scope":          map[string]any{"level": "PROJECT"},
	}, extra, drop)
}

func renderDoc(base, extra map[string]any, drop []string) string {
	maps.Copy(base, extra)
	for _, key := range drop {
		delete(base, key)
	}
	// encoding/json sorts a map's keys, so two fixtures differing in one member
	// differ in one line and a golden-free comparison stays readable.
	raw, err := json.Marshal(base)
	if err != nil {
		panic(fmt.Sprintf("rendering a fixture document: %v", err))
	}
	return string(raw)
}

// storeOf builds the Store the loader would have produced for these files.
//
// Check opens nothing (decision D-41), so every fixture here is in memory and no
// test in this package needs a filesystem. The one risk of that is a fixture
// shaped like nothing the loader emits;
// TestTheFixtureStoreIsTheOneTheLoaderProduces is what closes it, by building
// the same store both ways and comparing.
func storeOf(t *testing.T, records []testRecord, problems ...loader.Problem) loader.Store {
	t.Helper()

	store := loader.Store{
		Present:                true,
		Root:                   loader.StoreRoot,
		Decisions:              []loader.RecordRef{},
		Invariants:             []loader.RecordRef{},
		Problems:               []loader.Problem{},
		WriteSchemaVersion:     schema.WriteVersion,
		ReadableSchemaVersions: schema.ReadableVersions(),
	}

	for _, rec := range records {
		ref := refFor(t, rec)
		if rec.kind == loader.KindDecision {
			store.Decisions = append(store.Decisions, ref)
			continue
		}
		store.Invariants = append(store.Invariants, ref)
	}

	// os.ReadDir sorts by name, so the loader's buckets are sorted by file name
	// and a fixture written in another order would not be reproducing a real
	// load.
	byPath := func(a, b loader.RecordRef) int { return strings.Compare(a.Path, b.Path) }
	slices.SortFunc(store.Decisions, byPath)
	slices.SortFunc(store.Invariants, byPath)

	store.Problems = append(store.Problems, problems...)
	return store
}

// refFor performs the loader's steps 1-3 over a fixture's bytes: the id is read
// verbatim, the schema_version is parsed, and the body is carried unchanged
// (decision D-46).
func refFor(t *testing.T, rec testRecord) loader.RecordRef {
	t.Helper()

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rec.body), &fields); err != nil {
		t.Fatalf("fixture %s is not a JSON object, so the loader would have refused it: %v", rec.name, err)
	}

	var version int
	if raw, ok := fields["schema_version"]; ok {
		if err := json.Unmarshal(raw, &version); err != nil {
			t.Fatalf("fixture %s has a schema_version the loader could not parse: %v", rec.name, err)
		}
	}

	var id string
	if raw, ok := fields["id"]; ok {
		// Ignored deliberately: the loader's stringField leaves a non-string id
		// empty and lets step 5 judge its type.
		_ = json.Unmarshal(raw, &id)
	}

	return loader.RecordRef{
		Kind:          rec.kind,
		ID:            id,
		Path:          fixturePath(t, rec.kind, rec.name),
		SchemaVersion: version,
		Body:          []byte(rec.body),
	}
}

func fixturePath(t *testing.T, kind loader.RecordKind, name string) string {
	t.Helper()
	switch kind {
	case loader.KindDecision:
		return path.Join(loader.StoreRoot, "decisions", name)
	case loader.KindInvariant:
		return path.Join(loader.StoreRoot, "invariants", name)
	}
	t.Fatalf("fixture kind %q has no directory", kind)
	return ""
}

// problemAt builds the loader Problem that decision D-45's suppression reads:
// a file this binary could not parse, named repo-relative.
func problemAt(kind loader.RecordKind, name string) loader.Problem {
	dir := "decisions"
	if kind == loader.KindInvariant {
		dir = "invariants"
	}
	rel := path.Join(loader.StoreRoot, dir, name)
	return loader.Problem{
		Path:    rel,
		Code:    app.CodeKnowledgeUnreadable,
		Message: rel + " is not a JSON object: unexpected end of JSON input",
		Fatal:   false,
	}
}

// shippedValidator compiles the embedded schema documents — the same ones spec
// §95 step 5 evaluates records against.
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

// findingsAt returns the findings one step produced, in the order Check returned
// them.
func findingsAt(findings []validate.Finding, step validate.Step) []validate.Finding {
	matched := make([]validate.Finding, 0, len(findings))
	for _, finding := range findings {
		if finding.Step == step {
			matched = append(matched, finding)
		}
	}
	return matched
}

// pathsAt returns the record paths one step reported, sorted.
func pathsAt(findings []validate.Finding, step validate.Step) []string {
	paths := make([]string, 0, len(findings))
	for _, finding := range findingsAt(findings, step) {
		paths = append(paths, finding.Path)
	}
	slices.Sort(paths)
	return paths
}

// stepsAt returns the steps that reported one record path, sorted.
func stepsAt(findings []validate.Finding, recordPath string) []validate.Step {
	steps := make([]validate.Step, 0, len(findings))
	for _, finding := range findings {
		if finding.Path == recordPath {
			steps = append(steps, finding.Step)
		}
	}
	slices.Sort(steps)
	return steps
}

// describe renders findings as "step/path" pairs. %+v would print every message
// in full and bury the two fields a failure is actually about.
func describe(findings []validate.Finding) string {
	if len(findings) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		lines = append(lines, fmt.Sprintf("step %d %s [%s fatal=%v]",
			finding.Step, finding.Path, finding.Code, finding.Fatal))
	}
	return strings.Join(lines, "\n\t")
}

// TestStepNumbersAreTheSpecificationsOwn is AC-03.2.
//
// The numbers are asserted against literals rather than against each other:
// `StepScopeSyntax == StepSchema+6` would hold for any starting value, including
// zero, and a report that said "step 0" would name a step spec §95 does not
// have. The point of iota+5 is that a finding's step number is the
// specification's step number.
func TestStepNumbersAreTheSpecificationsOwn(t *testing.T) {
	for _, tt := range []struct {
		step validate.Step
		want int
	}{
		{validate.StepSchema, 5},
		{validate.StepFilenameConsistency, 6},
		{validate.StepUniqueID, 7},
		{validate.StepSupersedeTarget, 8},
		{validate.StepSupersedeCycle, 9},
		{validate.StepDuplicateActiveLineage, 10},
		{validate.StepScopeSyntax, 11},
	} {
		if int(tt.step) != tt.want {
			t.Errorf("step = %d, want %d", int(tt.step), tt.want)
		}
	}
}

// TestFindingCarriesTheFrozenWireShape is AC-03.2's second half. The field names
// and JSON tags are design §3's, and a consumer branching on `step` or `code`
// breaks if either is renamed.
func TestFindingCarriesTheFrozenWireShape(t *testing.T) {
	want := []string{
		"Path path",
		"ID id,omitempty",
		"Step step",
		"Code code",
		"Message message",
		"Fatal fatal",
	}

	structType := reflect.TypeOf(validate.Finding{})
	got := make([]string, 0, structType.NumField())
	for i := range structType.NumField() {
		field := structType.Field(i)
		got = append(got, field.Name+" "+field.Tag.Get("json"))
	}

	if !slices.Equal(got, want) {
		t.Errorf("Finding fields = %v, want %v", got, want)
	}
}

// TestCheckOverAValidStoreReturnsAnEmptyNonNilSlice is AC-03.9.
//
// Non-nil matters on the wire: a nil slice marshals as null, and a report whose
// findings key is null reads as "unknown" where it means "none".
func TestCheckOverAValidStoreReturnsAnEmptyNonNilSlice(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil)),
		invariantAt("INV-0001.json", invariantDoc("INV-0001", nil)),
	})

	findings := validate.Check(store, shippedValidator(t))
	if findings == nil {
		t.Fatal("Check returned a nil slice, which marshals as null")
	}
	if len(findings) != 0 {
		t.Errorf("Check over a valid store returned findings:\n\t%s", describe(findings))
	}

	raw, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("json.Marshal(findings): %v", err)
	}
	if string(raw) != "[]" {
		t.Errorf("marshalled findings = %s, want []", raw)
	}
}

// TestCheckOverAnAbsentStoreFindsNothing is AC-03.10.
//
// A clean clone of a repository that carries no knowledge records is healthy,
// not invalid (decision D-06). This is the guard against a later step that wants
// to say "the knowledge store is missing": that is the loader's reading to
// publish, and it publishes it as Present=false rather than as a finding.
func TestCheckOverAnAbsentStoreFindsNothing(t *testing.T) {
	absent := loader.Store{
		Present:                false,
		Root:                   loader.StoreRoot,
		Decisions:              []loader.RecordRef{},
		Invariants:             []loader.RecordRef{},
		Problems:               []loader.Problem{},
		WriteSchemaVersion:     schema.WriteVersion,
		ReadableSchemaVersions: schema.ReadableVersions(),
	}

	findings := validate.Check(absent, shippedValidator(t))
	if findings == nil {
		t.Fatal("Check returned a nil slice, which marshals as null")
	}
	if len(findings) != 0 {
		t.Errorf("Check over an absent store returned findings:\n\t%s", describe(findings))
	}
}

// TestCheckIsDeterministicAcrossRepeatedCalls is AC-03.8 and decision D-47.
//
// jsonschema/v6 returns a ValidationError's Causes in Go map-iteration order,
// which was observed to differ between five consecutive Validate calls on one
// schema and one instance in one process. The store below is deliberately noisy
// — several violations in one record, several records, several steps — because a
// single finding cannot be reordered and would make this pass regardless.
func TestCheckIsDeterministicAcrossRepeatedCalls(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0009", map[string]any{
			"created_at": "yesterday",
			"tags":       []any{"a", "a"},
			"surplus":    "not a property",
			"scope":      map[string]any{"level": "FILE", "target": `internal\app`},
		}, "title")),
		decisionAt("DEC-0002.json", decisionDoc("DEC-0002", map[string]any{
			"supersedes": []any{"DEC-0003"},
		})),
		decisionAt("DEC-0003.json", decisionDoc("DEC-0003", map[string]any{
			"supersedes": []any{"DEC-0002"},
		})),
		invariantAt("INV-0001.json", invariantDoc("INV-0001", map[string]any{
			"scope": map[string]any{"level": "MODULE"},
		})),
	})
	validator := shippedValidator(t)

	baseline := validate.Check(store, validator)
	if len(baseline) < 5 {
		t.Fatalf("the fixture produced %d findings; determinism over one finding is not a test",
			len(baseline))
	}
	first, err := json.Marshal(baseline)
	if err != nil {
		t.Fatalf("json.Marshal(findings): %v", err)
	}

	for run := range 24 {
		again, marshalErr := json.Marshal(validate.Check(store, validator))
		if marshalErr != nil {
			t.Fatalf("json.Marshal(findings): %v", marshalErr)
		}
		if string(again) != string(first) {
			t.Fatalf("run %d returned different bytes:\n first: %s\nsecond: %s", run, first, again)
		}
	}
}

// TestOnlyTheSupersedeCycleIsFatal is AC-03.6 and decision D-39.
//
// The store carries a condition for every step, so the assertion is a statement
// about all seven rather than about the one the author remembered. A finding
// that costs the repository more than the records it names is the over-fire this
// project has already produced twice.
func TestOnlyTheSupersedeCycleIsFatal(t *testing.T) {
	findings := validate.Check(everyConditionStore(t), shippedValidator(t))

	seen := make(map[validate.Step]bool, 7)
	for _, finding := range findings {
		seen[finding.Step] = true

		wantFatal := finding.Step == validate.StepSupersedeCycle
		if finding.Fatal != wantFatal {
			t.Errorf("step %d finding on %s has Fatal = %v, want %v",
				finding.Step, finding.Path, finding.Fatal, wantFatal)
		}

		wantCode := app.CodeKnowledgeInvalid
		if finding.Step == validate.StepSupersedeCycle {
			wantCode = app.CodeKnowledgeSupersedeCycle
		}
		if finding.Code != wantCode {
			t.Errorf("step %d finding on %s carries code %q, want %q",
				finding.Step, finding.Path, finding.Code, wantCode)
		}
	}

	for step := validate.StepSchema; step <= validate.StepScopeSyntax; step++ {
		if !seen[step] {
			t.Errorf("step %d produced no finding, so its fatality was never asserted:\n\t%s",
				step, describe(findings))
		}
	}
}

// TestEveryFindingCarriesARegisteredCodeAndNamesItsRecord is the house rule
// spec §84 and MR-001 finding R6 leave behind: status.componentFrom drops
// Diagnostic and Impact, so a finding whose message does not name the offending
// file is unactionable exactly where a reader meets it.
//
// The id half is asserted against the store rather than against a literal. A
// Finding's ID is the document's own spelling, which for step 6 is the value
// under dispute, and the only source of truth for it is the RecordRef the
// loader produced — comparing it to a constant written beside the fixture would
// pass whatever either said. Until this was added, blanking every `ID:` in
// steps.go left the whole suite green.
func TestEveryFindingCarriesARegisteredCodeAndNamesItsRecord(t *testing.T) {
	store := everyConditionStore(t)
	findings := validate.Check(store, shippedValidator(t))
	if len(findings) == 0 {
		t.Fatal("the fixture produced no findings, so this asserts nothing")
	}

	declaredID := make(map[string]string, store.Count())
	for _, ref := range slices.Concat(store.Decisions, store.Invariants) {
		declaredID[ref.Path] = ref.ID
	}
	named := 0

	for _, finding := range findings {
		if !app.IsRegistered(finding.Code) {
			t.Errorf("step %d finding on %s carries the unregistered code %q",
				finding.Step, finding.Path, finding.Code)
		}
		if finding.Path == "" {
			t.Errorf("step %d finding carries no path: %q", finding.Step, finding.Message)
		}
		switch want, known := declaredID[finding.Path]; {
		case !known:
			t.Errorf("step %d finding names %s, which is not a record this store carries",
				finding.Step, finding.Path)
		case finding.ID != want:
			t.Errorf("step %d finding on %s carries id %q, but the record there declares %q",
				finding.Step, finding.Path, finding.ID, want)
		case want != "":
			named++
		}
		if !strings.Contains(finding.Message, finding.Path) {
			t.Errorf("step %d message does not name %s: %q", finding.Step, finding.Path, finding.Message)
		}
		if strings.Contains(finding.Path, `\`) {
			t.Errorf("step %d finding path %q is not forward-slash spelled", finding.Step, finding.Path)
		}
		if strings.HasPrefix(finding.Path, "/") {
			t.Errorf("step %d finding path %q is absolute, want repository-relative", finding.Step, finding.Path)
		}
	}

	// Every id in the fixture could be empty and the loop above would then be
	// satisfied by a Finding that never carried one. This is the marker that
	// says the comparison had something to compare.
	if named == 0 {
		t.Error("no finding was matched against a non-empty declared id, so the id assertion proved nothing")
	}
}

// TestCheckRefusesANilValidator pins the one precondition Check has.
//
// Returning no findings instead would publish "nobody looked" as "we looked and
// it is clean", and there is no registered code for "this binary could not build
// its own validator" — AC-08.2 makes that a defect in the binary that bootstrap
// reports as such, never a repository condition. Failing in the binary's own
// voice is the honest remaining option, and this test is what makes it a
// decision rather than an accident.
func TestCheckRefusesANilValidator(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Error("Check(store, nil) returned instead of panicking, so an unvalidated store would read as clean")
		}
	}()

	store := storeOf(t, []testRecord{decisionAt("DEC-0001.json", decisionDoc("DEC-0001", nil))})
	_ = validate.Check(store, nil)
}

// everyConditionStore carries one record for each of steps 5-11 so that a test
// asserting a property of "every finding" sees all seven kinds.
func everyConditionStore(t *testing.T) loader.Store {
	t.Helper()
	return storeOf(t, []testRecord{
		// Step 5: a required property is missing.
		decisionAt("DEC-0100.json", decisionDoc("DEC-0100", nil, "title")),
		// Step 6: the file name and the id disagree.
		decisionAt("DEC-0200.json", decisionDoc("DEC-0299", nil)),
		// Step 7: two files carry one id (and, unavoidably, one of them also
		// disagrees with its file name — an id can only be duplicated by a file
		// that is not named after it).
		decisionAt("DEC-0300.json", decisionDoc("DEC-0300", nil)),
		decisionAt("DEC-0301.json", decisionDoc("DEC-0300", nil)),
		// Step 8: the supersede target is nowhere in the store.
		decisionAt("DEC-0400.json", decisionDoc("DEC-0400", map[string]any{
			"supersedes": []any{"DEC-0499"},
		})),
		// Step 9: two records supersede each other.
		decisionAt("DEC-0500.json", decisionDoc("DEC-0500", map[string]any{
			"supersedes": []any{"DEC-0501"},
		})),
		decisionAt("DEC-0501.json", decisionDoc("DEC-0501", map[string]any{
			"supersedes": []any{"DEC-0500"},
		})),
		// Step 10: both members of one lineage are still active.
		decisionAt("DEC-0600.json", decisionDoc("DEC-0600", nil)),
		decisionAt("DEC-0601.json", decisionDoc("DEC-0601", map[string]any{
			"supersedes": []any{"DEC-0600"},
		})),
		// Step 11: the scope target is not repository-relative.
		decisionAt("DEC-0700.json", decisionDoc("DEC-0700", map[string]any{
			"scope": map[string]any{"level": "FILE", "target": "../outside/thing.go"},
		})),
	})
}
