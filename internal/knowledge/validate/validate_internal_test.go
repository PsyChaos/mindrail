package validate

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/schemas"
)

// TestTheExpectedPathIsTheOneTheLoaderProduces is what makes decision D-45's
// suppression correct rather than plausible.
//
// The suppression asks whether a loader Problem names the file a referenced id
// would occupy, and it computes that file from two constants this package
// spells for itself — the record suffix and the two directory names — because
// the loader's own copies are unexported. Two spellings of one layout is
// precisely the drift that would make the suppression silently stop matching:
// every reference would look verifiable, and step 8 would report "does not
// exist" for records the loader had already named.
//
// So the loader is asked. A real record of each kind is written, loaded, and its
// Path compared against what expectedPath builds from the id and kind the loader
// itself reported.
func TestTheExpectedPathIsTheOneTheLoaderProduces(t *testing.T) {
	worktree := t.TempDir()
	write(t, worktree, "decisions", "DEC-0001.json", `{
  "schema_version": 1, "kind": "decision", "id": "DEC-0001", "status": "active",
  "created_at": "2026-01-01T00:00:00Z", "title": "t", "decision": "d"
}`)
	write(t, worktree, "invariants", "INV-0001.json", `{
  "schema_version": 1, "kind": "invariant", "id": "INV-0001", "status": "active",
  "created_at": "2026-01-01T00:00:00Z", "statement": "s", "severity": "LOW",
  "scope": {"level": "PROJECT"}
}`)

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

	refs := slices.Concat(store.Decisions, store.Invariants)
	if len(refs) != 2 {
		t.Fatalf("the loader read %d records, want one of each kind: %+v", len(refs), store.Problems)
	}

	seen := make(map[loader.RecordKind]int, 2)
	for _, ref := range refs {
		seen[ref.Kind]++

		got, ok := expectedPath(ref.Kind, ref.ID)
		if !ok {
			t.Errorf("expectedPath has no directory for kind %q, which the loader just produced", ref.Kind)
			continue
		}
		if got != ref.Path {
			t.Errorf("expectedPath(%q, %q) = %q, the loader produced %q", ref.Kind, ref.ID, got, ref.Path)
		}
		if back := idFromPath(ref.Path); back != ref.ID {
			t.Errorf("idFromPath(%q) = %q, want %q", ref.Path, back, ref.ID)
		}
	}

	// Both kinds have to have been exercised, or one of the two directory names
	// would be unchecked and the assertion above would be about half the layout.
	for _, kind := range []loader.RecordKind{loader.KindDecision, loader.KindInvariant} {
		if seen[kind] != 1 {
			t.Errorf("the loader produced %d records of kind %q, want 1", seen[kind], kind)
		}
	}
	if len(kindDirs) != len(seen) {
		t.Errorf("kindDirs holds %d kinds, the loader produces %d", len(kindDirs), len(seen))
	}
}

// TestEveryOneOfTheFourSortKeysDecidesAnOrder is decision D-47 asserted key by
// key.
//
// D-47 sorts by (Path, Step, InstanceLocation, Message), and two of those four
// could be deleted with the whole suite staying green. The end-to-end
// determinism test compares runs only against each other, so it is satisfied by
// any order that is stable within one process — including one that ignores three
// of the keys entirely.
//
// Each row differs in exactly one key and is fed in the wrong order, so deleting
// that key from sorted() turns exactly that row red and names it. The rows are
// built here rather than produced by Check because the instance location is
// carried beside the finding and never published: from outside the package it
// cannot be set, and a key nobody can construct a case for is a key nobody can
// test.
func TestEveryOneOfTheFourSortKeysDecidesAnOrder(t *testing.T) {
	at := func(path string, step Step, instance, message string) ordered {
		return ordered{
			instance: instance,
			finding:  Finding{Path: path, Step: step, Message: message},
		}
	}

	for _, tt := range []struct {
		key   string
		first ordered
		later ordered
	}{
		{
			key:   "Path",
			first: at("a.json", StepScopeSyntax, "/z", "z"),
			later: at("b.json", StepSchema, "/a", "a"),
		},
		{
			key:   "Step",
			first: at("a.json", StepSchema, "/z", "z"),
			later: at("a.json", StepScopeSyntax, "/a", "a"),
		},
		{
			key:   "InstanceLocation",
			first: at("a.json", StepSchema, "/a", "z"),
			later: at("a.json", StepSchema, "/z", "a"),
		},
		{
			key:   "Message",
			first: at("a.json", StepSchema, "/a", "a"),
			later: at("a.json", StepSchema, "/a", "z"),
		},
	} {
		t.Run(tt.key, func(t *testing.T) {
			// Every row's lower-priority keys are set so that they would order the
			// pair the other way round. A sort that had lost this key would
			// therefore not merely fail to reorder the pair, it would reverse it —
			// which is what stops the assertion from passing on a stable sort that
			// happens to leave the input alone.
			got := sorted([]ordered{tt.later, tt.first})
			if len(got) != 2 {
				t.Fatalf("sorted returned %d findings, want 2", len(got))
			}
			if got[0] != tt.first.finding {
				t.Errorf("the %s key did not decide the order: got %+v first, want %+v",
					tt.key, got[0], tt.first.finding)
			}
		})
	}
}

// TestSortedIsStableWhenEveryKeyAgrees is the over-fire guard for the test
// above: the sort must not reorder findings it has no key to distinguish.
//
// D-47 chose a stable sort so that two findings agreeing on all four keys keep
// the order they were produced in, which is itself deterministic. A sort that
// reached for some fifth tie-break — a pointer, a map address, a struct
// comparison — would satisfy every row above and still publish two different
// reports for one store.
func TestSortedIsStableWhenEveryKeyAgrees(t *testing.T) {
	same := func(id string) ordered {
		return ordered{
			instance: "/a",
			finding:  Finding{Path: "a.json", ID: id, Step: StepSchema, Message: "identical"},
		}
	}

	got := sorted([]ordered{same("second"), same("first")})
	if len(got) != 2 {
		t.Fatalf("sorted returned %d findings, want 2", len(got))
	}
	if got[0].ID != "second" || got[1].ID != "first" {
		t.Errorf("sorted reordered two findings its keys cannot tell apart: %q then %q",
			got[0].ID, got[1].ID)
	}
}

// TestSortedIsStableAtASizeWhereAnUnstableSortWouldShow is the test above at a
// size where the claim can actually fail.
//
// Two elements cannot fail it, and nor can forty identical ones. Go's
// slices.SortFunc is pdqsort: below thirteen elements it runs insertion sort,
// which is stable, and on an all-equal input it detects the run and leaves the
// order alone — so replacing SortStableFunc with SortFunc was measured to move
// zero elements at n = 2, 8, 13, 20, 40, 100 and 300 when every key agreed.
// Reordering appears only with enough elements AND more than one key group: at
// twenty elements in two groups it moved eighteen of them.
//
// So the shape below is the shape that can fail: two paths, ten findings each,
// every finding within a path agreeing on all four keys and distinguishable only
// by an ID the sort never looks at. This is the sort's contract as D-47 states
// it — findings the keys cannot separate keep the order they were produced in —
// and the previous form of this test could not have caught its loss.
func TestSortedIsStableAtASizeWhereAnUnstableSortWouldShow(t *testing.T) {
	const perPath = 10

	// Interleaved on the way in, so the sort has real work to do and the
	// within-path order is not already the input order.
	input := make([]ordered, 0, perPath*2)
	for i := range perPath {
		for _, path := range []string{"b.json", "a.json"} {
			input = append(input, ordered{
				instance: "/created_at",
				finding: Finding{
					Path:    path,
					ID:      fmt.Sprintf("%s#%d", path, i),
					Step:    StepSchema,
					Message: "identical for every finding on this path",
				},
			})
		}
	}

	got := sorted(input)
	if len(got) != perPath*2 {
		t.Fatalf("sorted returned %d findings, want %d", len(got), perPath*2)
	}

	// The one key that differs still decides: a.json before b.json.
	wantIDs := make([]string, 0, perPath*2)
	for _, path := range []string{"a.json", "b.json"} {
		for i := range perPath {
			wantIDs = append(wantIDs, fmt.Sprintf("%s#%d", path, i))
		}
	}
	gotIDs := make([]string, 0, len(got))
	for _, finding := range got {
		gotIDs = append(gotIDs, finding.ID)
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("sorted() = %v,\nwant %v — findings agreeing on all four keys must keep the order they were produced in", gotIDs, wantIDs)
	}
}

func write(t *testing.T, worktree, subdir, name, contents string) {
	t.Helper()

	dir := filepath.Join(worktree, ".mindrail", "knowledge", subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// TestValidateImportsNothingThatCanReachTheFilesystem turns AC-03.1's "opens
// nothing" into a property of the package rather than a claim about one function
// body.
//
// Decision D-41 says Check performs no I/O so that status, doctor and a future
// CI entry point reach identical verdicts from identical bytes. A test that
// exercised Check without a filesystem would keep passing the day someone added
// an os.Stat behind a condition it did not reach; an import set cannot be
// widened by accident.
func TestValidateImportsNothingThatCanReachTheFilesystem(t *testing.T) {
	want := []string{
		"cmp",
		"encoding/json",
		"fmt",
		"github.com/PsyChaos/mindrail/internal/app",
		"github.com/PsyChaos/mindrail/internal/knowledge/loader",
		"github.com/PsyChaos/mindrail/internal/knowledge/record",
		"github.com/PsyChaos/mindrail/internal/knowledge/schema",
		"path",
		"slices",
		"strings",
	}

	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parsing the package: %v", err)
	}

	seen := make(map[string]struct{}, len(want))
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, spec := range file.Imports {
				path, unquoteErr := strconv.Unquote(spec.Path.Value)
				if unquoteErr != nil {
					t.Fatalf("unquoting %s: %v", spec.Path.Value, unquoteErr)
				}
				seen[path] = struct{}{}
			}
		}
	}

	got := make([]string, 0, len(seen))
	for path := range seen {
		got = append(got, path)
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("the package imports\n\t%v\nwant\n\t%v", got, want)
	}
}
