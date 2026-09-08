package loader_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
	"github.com/PsyChaos/mindrail/schemas"
)

// maxAllocationsPerRecord bounds what one more record in the store may cost the
// loader, and it is finding B-A3 written down as a number.
//
// The budget is on allocations rather than on elapsed time because elapsed time
// is not a property of the code: a test that fails on a busy machine gets a
// retry, then a skip, then a deletion, and the guard is gone before the
// regression comes back. Allocation count is deterministic, and it moves for
// exactly the reason B-A3 moved — canonicalizing a path allocates a string per
// component and per link followed.
//
// Measured on the three commits this fix sits between, per record:
//
//	8e5c2e5 (before containment)  27  — join the name onto the resolved bucket
//	3bd453c (containment added)   54  — Root.Resolve of the whole path, per record
//	this commit                   29  — one os.Lstat of the entry
//
// Forty sits between the last two with room on both sides: it does not fail
// because a record grew a field, and it cannot pass if the whole path is
// canonicalized once per record again.
const maxAllocationsPerRecord = 40

// TestLoadDoesNotRecanonicalizeTheStorePathPerRecord is the regression test for
// finding B-A3.
//
// Closing the symbolic-link gap in a record's path (finding BA-10) put every
// record through Root.Resolve, which canonicalizes every component of what it is
// given. Four of those components — the worktree root, .mindrail, knowledge and
// the bucket — are the same for every record in the bucket and had already been
// resolved once to walk it, so the work grew with the number of records while
// answering nothing new. It cost about 3.9 µs a record, 45% of the loader's
// per-record time at scale, and moved the 150 ms warm-path budget for
// `status --json` from roughly 8,700 records to roughly 5,700.
//
// The cost is measured at the margin — the difference between a store of n
// records and a store of 2n — so that the fixed cost of opening a store, of the
// schema registry and of the two directory walks cancels out and what is left is
// the price of one record.
func TestLoadDoesNotRecanonicalizeTheStorePathPerRecord(t *testing.T) {
	const small, large = 200, 400

	perRecord := (allocationsForStore(t, large) - allocationsForStore(t, small)) / float64(large-small)
	if perRecord > maxAllocationsPerRecord {
		t.Errorf("one record costs %.1f allocations, want at most %d;"+
			" a per-record canonicalization of the whole store path costs about 54 (B-A3)",
			perRecord, maxAllocationsPerRecord)
	}

	// The other direction. A budget nothing can exceed is not a budget, and a
	// measurement that came out at zero would mean the harness stopped
	// measuring rather than that the loader stopped allocating.
	if perRecord <= 0 {
		t.Errorf("one record costs %.1f allocations, which cannot be right:"+
			" the measurement is not observing the load", perRecord)
	}
}

// allocationsForStore reports the allocations one Load of a store of n valid
// decision records performs.
func allocationsForStore(t *testing.T, n int) float64 {
	t.Helper()

	worktree := t.TempDir()
	makeKnowledgeDirs(t, worktree)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("DEC-%04d", i)
		writeRecord(t, worktree, "decisions", id+".json", decisionJSON(id, schema.WriteVersion))
	}

	l := newLoader(t, worktree)
	ctx := context.Background()
	if store, err := l.Load(ctx); err != nil || store.Count() != n {
		t.Fatalf("Load() = %d records, %v; want %d records and no error", store.Count(), err, n)
	}

	return testing.AllocsPerRun(3, func() {
		if _, err := l.Load(ctx); err != nil {
			t.Errorf("Load() error = %v", err)
		}
	})
}

// BenchmarkLoad is the instrument the numbers in maxAllocationsPerRecord came
// from, kept so the next reader can reproduce them rather than trust them. Run
// it with -benchmem; the per-record figures are the reported ones divided by the
// store size.
func BenchmarkLoad(b *testing.B) {
	for _, n := range []int{100, 1000, 2000, 5000, 10000} {
		b.Run(fmt.Sprintf("records=%d", n), func(b *testing.B) {
			l := benchmarkLoader(b, n)
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := l.Load(ctx); err != nil {
					b.Fatalf("Load() error = %v", err)
				}
			}
		})
	}
}

func benchmarkLoader(b *testing.B, n int) *loader.Loader {
	b.Helper()

	worktree := b.TempDir()
	decisions := filepath.Join(worktree, ".mindrail", "knowledge", "decisions")
	if err := os.MkdirAll(decisions, 0o755); err != nil {
		b.Fatalf("creating %s: %v", decisions, err)
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("DEC-%05d", i)
		body := decisionJSON(id, schema.WriteVersion)
		if err := os.WriteFile(filepath.Join(decisions, id+".json"), []byte(body), 0o644); err != nil {
			b.Fatalf("writing %s: %v", id, err)
		}
	}

	root, err := filesystem.NewRoot(worktree)
	if err != nil {
		b.Fatalf("NewRoot(%q): %v", worktree, err)
	}
	registry, err := schema.NewRegistry(schemas.KnowledgeFS)
	if err != nil {
		b.Fatalf("NewRegistry(embedded): %v", err)
	}

	l := loader.New(root, registry)
	if store, err := l.Load(context.Background()); err != nil || store.Count() != n {
		b.Fatalf("Load() = %d records, %v; want %d records and no error", store.Count(), err, n)
	}
	return l
}
