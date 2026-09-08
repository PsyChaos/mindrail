package validate

import (
	"context"
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
