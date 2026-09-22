package parser

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/PsyChaos/mindrail/internal/index/parser/internal/nativealloc"
	ts "github.com/tree-sitter/go-tree-sitter"
)

// SetAllocator is process-global: no test in this package may call t.Parallel.
// Actual libc allocations are tracked, including resources owned by queries.
func TestNativeResourcesReturnToBaselineAfter1000CyclesPerLanguage(t *testing.T) {
	tracker := nativealloc.Install()
	defer tracker.Restore()
	r := registryForTest(t)
	defer r.Close() // close BEFORE restoring the allocator, including on failure
	for _, a := range r.adapters {
		t.Run(a.info.Language, func(t *testing.T) {
			cycle := func() {
				s, err := a.Parse(context.Background(), SourceFile{Content: []byte(packageFixtures[a.info.Language])})
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				if _, err := a.Symbols(s); err != nil {
					t.Fatal(err)
				}
				if _, err := a.References(s); err != nil {
					t.Fatal(err)
				}
				if _, err := a.Imports(s); err != nil {
					t.Fatal(err)
				}
			}
			cycle() // warmup; retained registry queries are included in baseline
			beforeCount, beforeBytes := tracker.Live()
			for range 1000 {
				cycle()
			}
			afterCount, afterBytes := tracker.Live()
			t.Logf("1000 cycles: native blocks %d -> %d; bytes %d -> %d", beforeCount, afterCount, beforeBytes, afterBytes)
			if afterCount != beforeCount || afterBytes != beforeBytes {
				t.Fatal("native allocations grew")
			}
		})
	}
	r.Close()
	if count, size := tracker.Live(); count != 0 || size != 0 {
		t.Fatalf("registry disposal leaked %d blocks / %d bytes", count, size)
	}
}

func TestEveryEmbeddedQueryCompilesAndGrammarABIIsCompatible(t *testing.T) {
	r := registryForTest(t)
	for _, a := range r.adapters {
		abi := a.language.AbiVersion()
		if abi < ts.MIN_COMPATIBLE_LANGUAGE_VERSION || abi > ts.LANGUAGE_VERSION {
			t.Fatalf("%s incompatible ABI %d", a.info.Language, abi)
		}
		for _, kind := range []string{"symbols", "references", "imports"} {
			path := "queries/" + a.info.Language + "/" + kind + ".scm"
			content, err := queries.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			query, queryErr := ts.NewQuery(a.language, string(content))
			if queryErr != nil {
				t.Fatalf("%s: %v", path, queryErr)
			}
			query.Close()
		}
	}
}

func TestRegistryConstructionFailureClosesEarlierQueries(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid", true: "missing"}[missing], func(t *testing.T) {
			files := fstest.MapFS{}
			if err := fs.WalkDir(queries, ".", func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				content, err := queries.ReadFile(path)
				files[path] = &fstest.MapFile{Data: content}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			path := "queries/tsx/references.scm"
			if missing {
				delete(files, path)
			} else {
				files[path].Data = []byte("(not_a_real_node) @name")
			}
			tracker := nativealloc.Install()
			defer tracker.Restore()
			r, err := newRegistry(files)
			if r != nil {
				r.Close()
				t.Fatal("invalid registry escaped")
			}
			if err == nil {
				t.Fatal("query failure swallowed")
			}
			if count, size := tracker.Live(); count != 0 || size != 0 {
				t.Fatalf("failure leaked %d blocks / %d bytes", count, size)
			}
		})
	}
}

type canceledAfterEntry struct {
	context.Context
	calls int
}

func (c *canceledAfterEntry) Err() error {
	c.calls++
	if c.calls > 1 {
		return context.Canceled
	}
	return nil
}

func TestCanceledAfterParseClosesTree(t *testing.T) {
	tracker := nativealloc.Install()
	defer tracker.Restore()
	r := registryForTest(t)
	defer r.Close()
	beforeCount, beforeBytes := tracker.Live()
	ctx := &canceledAfterEntry{Context: context.Background()}
	s, err := r.adapters[0].Parse(ctx, SourceFile{Content: []byte(fixtures["python"])})
	if s != nil || !errors.Is(err, context.Canceled) {
		if s != nil {
			s.Close()
		}
		t.Fatalf("post-parse cancellation: %v", err)
	}
	if count, size := tracker.Live(); count != beforeCount || size != beforeBytes {
		t.Fatalf("canceled parse leaked %d blocks / %d bytes", count-beforeCount, size-beforeBytes)
	}
}
