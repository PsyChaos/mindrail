package loader_test

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/schemas"
)

// TestLoadAbsentKnowledgeTreeIsHealthyZero is decision D-06. A clean clone of
// a repository that simply has no records must not look broken: MR-002's
// acceptance criteria depend on an empty store being a legal, healthy state.
func TestLoadAbsentKnowledgeTreeIsHealthyZero(t *testing.T) {
	worktree := t.TempDir()

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if store.Present {
		t.Error("Present = true, want false")
	}
	if store.Count() != 0 {
		t.Errorf("Count() = %d, want 0", store.Count())
	}
	if len(store.Problems) != 0 {
		t.Errorf("Problems = %v, want none", store.Problems)
	}
	if store.HasFatalProblem() {
		t.Error("HasFatalProblem() = true, want false")
	}
	if store.Root != ".mindrail/knowledge" {
		t.Errorf("Root = %q, want %q", store.Root, ".mindrail/knowledge")
	}
	if store.WriteSchemaVersion != schema.WriteVersion {
		t.Errorf("WriteSchemaVersion = %d, want %d", store.WriteSchemaVersion, schema.WriteVersion)
	}
	if !slices.Equal(store.ReadableSchemaVersions, schema.ReadableVersions()) {
		t.Errorf("ReadableSchemaVersions = %v, want %v", store.ReadableSchemaVersions, schema.ReadableVersions())
	}
}

// TestLoadEmptyScaffoldedTreeIsPresentAndZero covers the state decision D-05
// creates: init writes the directories and a .gitkeep in each, and neither the
// directories nor the .gitkeep may be mistaken for a record.
func TestLoadEmptyScaffoldedTreeIsPresentAndZero(t *testing.T) {
	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	writeRecord(t, worktree, "decisions", ".gitkeep", "")
	writeRecord(t, worktree, "invariants", ".gitkeep", "")

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if !store.Present {
		t.Error("Present = false, want true once the tree exists")
	}
	if store.Count() != 0 {
		t.Errorf("Count() = %d, want 0", store.Count())
	}
	if len(store.Problems) != 0 {
		t.Errorf("Problems = %v, want none: .gitkeep is scaffolding, not a record", store.Problems)
	}
}

// TestLoadCountsDecisionsAndInvariants is the ordinary path: records are
// bucketed by the directory they live in and reported in a stable order.
func TestLoadCountsDecisionsAndInvariants(t *testing.T) {
	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)

	writeRecord(t, worktree, "decisions", "DEC-0002.json", decisionJSON("DEC-0002", 1))
	writeRecord(t, worktree, "decisions", "DEC-0001.json", decisionJSON("DEC-0001", 1))
	writeRecord(t, worktree, "invariants", "INV-0001.json", invariantJSON("INV-0001", 1))
	writeRecord(t, worktree, "decisions", "notes.md", "not a record\n")
	writeRecord(t, worktree, "invariants", "README.txt", "not a record\n")

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if !store.Present {
		t.Fatal("Present = false, want true")
	}
	if store.Count() != 3 {
		t.Fatalf("Count() = %d, want 3 (problems: %v)", store.Count(), store.Problems)
	}
	if len(store.Problems) != 0 {
		t.Errorf("Problems = %v, want none", store.Problems)
	}

	// slices.EqualFunc rather than slices.Equal: RecordRef stopped being
	// comparable when D-46 gave it a []byte body. sameRef compares every field,
	// so this assertion is the one it was before, not a narrowed version of it.
	wantDecisions := []loader.RecordRef{
		{Kind: loader.KindDecision, ID: "DEC-0001", Path: ".mindrail/knowledge/decisions/DEC-0001.json", SchemaVersion: 1, Body: []byte(decisionJSON("DEC-0001", 1))},
		{Kind: loader.KindDecision, ID: "DEC-0002", Path: ".mindrail/knowledge/decisions/DEC-0002.json", SchemaVersion: 1, Body: []byte(decisionJSON("DEC-0002", 1))},
	}
	if !slices.EqualFunc(store.Decisions, wantDecisions, sameRef) {
		t.Errorf("Decisions = %s, want %s (sorted by file name)", describeRefs(store.Decisions), describeRefs(wantDecisions))
	}

	wantInvariants := []loader.RecordRef{
		{Kind: loader.KindInvariant, ID: "INV-0001", Path: ".mindrail/knowledge/invariants/INV-0001.json", SchemaVersion: 1, Body: []byte(invariantJSON("INV-0001", 1))},
	}
	if !slices.EqualFunc(store.Invariants, wantInvariants, sameRef) {
		t.Errorf("Invariants = %s, want %s", describeRefs(store.Invariants), describeRefs(wantInvariants))
	}
}

// TestLoadRecordsCorruptJSONAsNonFatalProblem keeps one broken file from
// hiding every healthy record beside it: the load reports the file and carries
// on, and the store stays usable.
func TestLoadRecordsCorruptJSONAsNonFatalProblem(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{name: "truncated object", contents: `{"schema_version": 1, "id": "DEC-0002"`},
		{name: "not json at all", contents: "this is a note, not a record\n"},
		{name: "empty file", contents: ""},
		{name: "top-level array", contents: `[{"schema_version": 1}]`},
		{name: "missing schema_version", contents: `{"kind": "decision", "id": "DEC-0002"}`},
		{name: "schema_version is a string", contents: `{"schema_version": "1", "id": "DEC-0002"}`},
		{name: "schema_version is fractional", contents: `{"schema_version": 1.5, "id": "DEC-0002"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worktree := t.TempDir()
			makeKnowledgeDirs(t, worktree)
			writeRecord(t, worktree, "decisions", "DEC-0001.json", decisionJSON("DEC-0001", 1))
			writeRecord(t, worktree, "decisions", "DEC-0002.json", tt.contents)

			store, err := newLoader(t, worktree).Load(context.Background())
			if err != nil {
				t.Fatalf("Load() error = %v, want nil: a broken record is data, not a failure", err)
			}

			if store.Count() != 1 {
				t.Errorf("Count() = %d, want 1: the healthy record must still load", store.Count())
			}
			if len(store.Problems) != 1 {
				t.Fatalf("Problems = %v, want exactly one", store.Problems)
			}

			problem := store.Problems[0]
			if problem.Fatal {
				t.Error("Fatal = true, want false: an unreadable record does not block the workspace")
			}
			if problem.Code != app.CodeKnowledgeUnreadable {
				t.Errorf("Code = %q, want %q", problem.Code, app.CodeKnowledgeUnreadable)
			}
			if problem.Path != ".mindrail/knowledge/decisions/DEC-0002.json" {
				t.Errorf("Path = %q, want the repo-relative slash path of the broken file", problem.Path)
			}
			if problem.Message == "" {
				t.Error("Message is empty")
			}
			if store.HasFatalProblem() {
				t.Error("HasFatalProblem() = true, want false")
			}
		})
	}
}

// TestLoadFailsClosedOnUnknownNewerSchemaVersion is the kernel-scope §3
// branch that must never be retrofitted: a record this binary does not
// understand is fatal, because half-reading it would silently drop whatever
// the newer version added.
func TestLoadFailsClosedOnUnknownNewerSchemaVersion(t *testing.T) {
	tests := []struct {
		name    string
		version int
	}{
		{name: "next version", version: 2},
		{name: "far future version", version: 99},
		{name: "pre-history version", version: 0},
		{name: "negative version", version: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worktree := t.TempDir()
			makeKnowledgeDirs(t, worktree)
			writeRecord(t, worktree, "invariants", "INV-0007.json", invariantJSON("INV-0007", tt.version))

			store, err := newLoader(t, worktree).Load(context.Background())
			if err != nil {
				t.Fatalf("Load() error = %v, want nil: the condition is reported, not thrown", err)
			}

			if store.Count() != 0 {
				t.Errorf("Count() = %d, want 0: an unreadable-version record must not be counted", store.Count())
			}
			if len(store.Problems) != 1 {
				t.Fatalf("Problems = %v, want exactly one", store.Problems)
			}

			problem := store.Problems[0]
			if !problem.Fatal {
				t.Error("Fatal = false, want true: an unreadable schema version blocks the workspace")
			}
			if problem.Code != app.CodeKnowledgeSchemaUnsupported {
				t.Errorf("Code = %q, want %q", problem.Code, app.CodeKnowledgeSchemaUnsupported)
			}
			if !strings.Contains(problem.Message, strconv.Itoa(tt.version)) {
				t.Errorf("Message = %q, want it to name the unreadable version %d", problem.Message, tt.version)
			}
			if !store.HasFatalProblem() {
				t.Error("HasFatalProblem() = false, want true")
			}
		})
	}
}

// TestLoadPathsAreRepoRelativeSlash pins tech-stack §74: every path this
// package hands upward is repository-relative with forward slashes, on every
// host operating system.
func TestLoadPathsAreRepoRelativeSlash(t *testing.T) {
	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	writeRecord(t, worktree, "decisions", "DEC-0001.json", decisionJSON("DEC-0001", 1))
	writeRecord(t, worktree, "invariants", "INV-0001.json", invariantJSON("INV-0001", 1))
	writeRecord(t, worktree, "invariants", "INV-0002.json", "broken")

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	paths := []string{store.Root}
	for _, ref := range slices.Concat(store.Decisions, store.Invariants) {
		paths = append(paths, ref.Path)
	}
	for _, problem := range store.Problems {
		paths = append(paths, problem.Path)
	}
	// The store root, one decision, one invariant and one problem.
	if len(paths) != 4 {
		t.Fatalf("collected %d paths, want 4: %v", len(paths), paths)
	}

	for _, path := range paths {
		if strings.Contains(path, `\`) {
			t.Errorf("path %q contains a backslash", path)
		}
		if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
			t.Errorf("path %q is absolute, want repository-relative", path)
		}
		if !strings.HasPrefix(path, ".mindrail/knowledge") {
			t.Errorf("path %q is not under the knowledge root", path)
		}
		if strings.Contains(path, worktree) {
			t.Errorf("path %q leaks the machine-local worktree location", path)
		}
	}
}

// TestLoadDoesNotPerformMR002Validation is decision D-27's scope line, stated
// as a test. Steps 5-14 of spec §95 belong to MR-002; performing any of them
// here would make MR-002's diagnostics unreachable and would reject records
// this milestone has no authority to judge.
func TestLoadDoesNotPerformMR002Validation(t *testing.T) {
	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)

	// Step 6: the file name disagrees with the id inside it.
	writeRecord(t, worktree, "decisions", "DEC-0001.json", decisionJSON("DEC-9999", 1))
	// Step 7: a duplicate id.
	writeRecord(t, worktree, "decisions", "DEC-0002.json", decisionJSON("DEC-9999", 1))
	// Steps 5 and 8: fields the schema requires are missing, and the supersede
	// target does not exist.
	writeRecord(t, worktree, "invariants", "INV-0001.json",
		`{"schema_version": 1, "id": "INV-0001", "supersedes": ["INV-4242"]}`)

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(store.Problems) != 0 {
		t.Errorf("Problems = %v, want none: steps 5-14 belong to MR-002", store.Problems)
	}
	if store.Count() != 3 {
		t.Errorf("Count() = %d, want 3", store.Count())
	}
	if got := store.Decisions[0].ID; got != "DEC-9999" {
		t.Errorf("Decisions[0].ID = %q, want the id read verbatim from the document", got)
	}
}

// TestLoadKindComesFromTheDirectory keeps bucketing independent of the
// document's own "kind" field: reconciling the two is spec §95 step 6, which
// MR-002 owns.
func TestLoadKindComesFromTheDirectory(t *testing.T) {
	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	writeRecord(t, worktree, "invariants", "INV-0001.json",
		`{"schema_version": 1, "kind": "decision", "id": "INV-0001"}`)

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(store.Decisions) != 0 {
		t.Errorf("Decisions = %+v, want none", store.Decisions)
	}
	if len(store.Invariants) != 1 || store.Invariants[0].Kind != loader.KindInvariant {
		t.Fatalf("Invariants = %+v, want one record of kind %q", store.Invariants, loader.KindInvariant)
	}
}

// TestLoadWithOnlyOneSubdirectory covers the partially scaffolded repository:
// a project with decisions but no invariants yet is healthy.
func TestLoadWithOnlyOneSubdirectory(t *testing.T) {
	worktree := t.TempDir()
	mustMkdirAll(t, filepath.Join(worktree, ".mindrail", "knowledge", "decisions"))
	writeRecord(t, worktree, "decisions", "DEC-0001.json", decisionJSON("DEC-0001", 1))

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if !store.Present || store.Count() != 1 || len(store.Problems) != 0 {
		t.Fatalf("store = %+v, want one decision and no problems", store)
	}
}

// TestLoadRespectsContextCancellation keeps the loader inside the warm-path
// budget: a cancelled context stops the walk instead of reading a large store
// nobody is waiting for any more.
func TestLoadRespectsContextCancellation(t *testing.T) {
	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	for i := range 50 {
		id := fmt.Sprintf("DEC-%04d", i)
		writeRecord(t, worktree, "decisions", id+".json", decisionJSON(id, 1))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := newLoader(t, worktree).Load(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() error = %v, want context.Canceled", err)
	}
}

// TestLoadRejectsAFileWhereTheTreeShouldBe reports a misconfigured repository
// instead of quietly behaving as if it had no knowledge at all.
func TestLoadRejectsAFileWhereTheTreeShouldBe(t *testing.T) {
	worktree := t.TempDir()
	mustMkdirAll(t, filepath.Join(worktree, ".mindrail"))
	if err := os.WriteFile(filepath.Join(worktree, ".mindrail", "knowledge"), []byte("oops"), 0o644); err != nil {
		t.Fatalf("writing decoy: %v", err)
	}

	_, err := newLoader(t, worktree).Load(context.Background())
	if err == nil {
		t.Fatal("Load() error = nil, want a structured error")
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if payload.Code != app.CodeKnowledgeUnreadable {
		t.Errorf("code = %q, want %q", payload.Code, app.CodeKnowledgeUnreadable)
	}
	if len(payload.NextAction) == 0 {
		t.Error("next_action is empty; spec §84 promises a remedy")
	}
}

// TestLoadPerformsNoNetworkCalls backs acceptance criterion 1: init must work
// with no network. The recording resolver proves nothing dialed at run time,
// and the import scan proves nothing in this package could.
func TestLoadPerformsNoNetworkCalls(t *testing.T) {
	var dials atomic.Int64
	original := net.DefaultResolver
	t.Cleanup(func() { net.DefaultResolver = original })
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return nil, errors.New("network access is not allowed while loading knowledge")
		},
	}

	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	// A record whose $schema-ish fields point at a URL must not tempt the
	// loader into fetching anything (tech-stack §39).
	writeRecord(t, worktree, "decisions", "DEC-0001.json",
		`{"schema_version": 1, "id": "DEC-0001", "$schema": "https://example.invalid/decision.json"}`)

	if _, err := newLoader(t, worktree).Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if got := dials.Load(); got != 0 {
		t.Errorf("dials = %d, want 0", got)
	}

	for _, forbidden := range []string{"net", "net/http", "net/url"} {
		if importsPackage(t, ".", forbidden) {
			t.Errorf("package imports %q; the knowledge loader must be reachable offline", forbidden)
		}
	}
}

// importsPackage reports whether any non-test file in dir imports path. It
// reads the source rather than shelling out to the toolchain so the assertion
// holds in a sandbox with no module cache.
func importsPackage(t *testing.T, dir, path string) bool {
	t.Helper()

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parsing %s: %v", dir, err)
	}

	quoted := strconv.Quote(path)
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, spec := range file.Imports {
				if spec.Path.Value == quoted {
					return true
				}
			}
		}
	}
	return false
}

func newLoader(t *testing.T, worktreeRoot string) *loader.Loader {
	t.Helper()

	root, err := filesystem.NewRoot(worktreeRoot)
	if err != nil {
		t.Fatalf("NewRoot(%q): %v", worktreeRoot, err)
	}
	registry, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		t.Fatalf("NewRegistry(embedded): %v", err)
	}
	return loader.New(root, registry)
}

func makeKnowledgeDirs(t *testing.T, worktreeRoot string) {
	t.Helper()
	for _, sub := range []string{"decisions", "invariants"} {
		mustMkdirAll(t, filepath.Join(worktreeRoot, ".mindrail", "knowledge", sub))
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
}

func writeRecord(t *testing.T, worktreeRoot, subdir, name, contents string) {
	t.Helper()
	path := filepath.Join(worktreeRoot, ".mindrail", "knowledge", subdir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func decisionJSON(id string, version int) string {
	return fmt.Sprintf(`{
  "schema_version": %d,
  "kind": "decision",
  "id": %q,
  "status": "active",
  "created_at": "2026-01-01T00:00:00Z",
  "title": "Test decision",
  "decision": "Do the thing."
}`, version, id)
}

func invariantJSON(id string, version int) string {
	return fmt.Sprintf(`{
  "schema_version": %d,
  "kind": "invariant",
  "id": %q,
  "status": "active",
  "created_at": "2026-01-01T00:00:00Z",
  "statement": "The thing stays true.",
  "severity": "CRITICAL",
  "scope": {"level": "PROJECT"}
}`, version, id)
}

// TestUnreadableStoreKeepsGoErrorsOutOfWhy pins the boundary between the two
// audiences of an error. why is prose a person reads; the wrapped Go error,
// with its internal package names and its machine-local absolute paths, is for
// the cause and metadata channels. Leaking one into the other is how a
// permission problem ends up reading like a stack trace.
func TestUnreadableStoreKeepsGoErrorsOutOfWhy(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads directories regardless of their mode")
	}

	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	decisions := filepath.Join(worktree, ".mindrail", "knowledge", "decisions")
	if err := os.Chmod(decisions, 0o000); err != nil {
		t.Fatalf("make the decisions directory unreadable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(decisions, 0o700) })

	_, err := newLoader(t, worktree).Load(context.Background())
	if err == nil {
		t.Fatalf("Load() = nil error for an unreadable store")
	}
	payload, ok := app.PayloadOf(err)
	if !ok {
		t.Fatalf("error carries no domain payload: %v", err)
	}
	if payload.Code != app.CodeKnowledgeUnreadable {
		t.Fatalf("payload code = %q, want %q", payload.Code, app.CodeKnowledgeUnreadable)
	}

	for _, leak := range []string{"filesystem:", "lstat ", "canonicalize ", worktree} {
		if strings.Contains(payload.Why, leak) {
			t.Errorf("why = %q, which leaks %q; that belongs in the cause or metadata", payload.Why, leak)
		}
	}
	if !strings.Contains(payload.Why, loader.StoreRoot) {
		t.Errorf("why = %q, want it to name the store in its repo-relative spelling", payload.Why)
	}
	if payload.Metadata["detail"] == "" {
		t.Errorf("the underlying error was dropped instead of moved: metadata = %v", payload.Metadata)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("the cause chain lost the permission error: %v", err)
	}
}

// TestUnreadableRecordMessageStaysRepoRelative is the same rule one level down:
// a per-record problem already names the record in the only spelling Mindrail
// prints (tech-stack §74), so the Go error must not re-attach an absolute one.
func TestUnreadableRecordMessageStaysRepoRelative(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads files regardless of their mode")
	}

	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	record := filepath.Join(worktree, ".mindrail", "knowledge", "decisions", "DEC-0001.json")
	if err := os.WriteFile(record, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatalf("seed a record: %v", err)
	}
	if err := os.Chmod(record, 0o000); err != nil {
		t.Fatalf("make the record unreadable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(record, 0o600) })

	store, err := newLoader(t, worktree).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v; one unreadable record must not fail the whole store", err)
	}
	if len(store.Problems) != 1 {
		t.Fatalf("Problems = %+v, want exactly one", store.Problems)
	}

	problem := store.Problems[0]
	if strings.Contains(problem.Message, worktree) {
		t.Errorf("Message = %q, which repeats the machine-local absolute path", problem.Message)
	}
	if !strings.Contains(problem.Message, "permission denied") {
		t.Errorf("Message = %q, want it to keep the reason for the failure", problem.Message)
	}
	if problem.Fatal {
		t.Errorf("an unreadable file was marked fatal; only an unreadable schema_version is")
	}
}
