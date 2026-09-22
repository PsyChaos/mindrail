package impact_test

import (
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/impact"
)

// TestAnalyzeNameMatchFallback is TASK-02 AC-02.1. A cross-file name user
// with no resolved edge arrives as STRUCTURAL_NAME_MATCH at confidence
// ≤ 0.5 with the reason named — never promoted to a direct edge.
func TestAnalyzeNameMatchFallback(t *testing.T) {
	fx := newImpactFixture(t)
	seedSymbol(t, fx, "SYM-N-A", "a", "fa", "/r/a.py")
	seedSymbol(t, fx, "SYM-N-W", "w", "fa", "/r/c.py")

	result, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols: []impact.Input{{UID: "SYM-N-A"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var match *impact.Entry
	for i, entry := range result.Entries {
		if entry.Via.Kind == impact.NameMatchEdge {
			match = &result.Entries[i]
		}
		if entry.Via.Kind == impact.DirectEdge {
			t.Fatalf("no direct edge exists, found %+v", entry)
		}
	}
	if match == nil {
		t.Fatalf("no name-match entry in %+v", result.Entries)
	}
	if match.Confidence > 0.5 {
		t.Fatalf("confidence = %v, want ≤ 0.5", match.Confidence)
	}
	if match.FallbackReason == "" {
		t.Fatal("name-match entry names no reason")
	}
	if match.Via.ReferrerKey != "w" || match.Via.ReferrerPath != "/r/c.py" {
		t.Fatalf("via = %+v", match.Via)
	}
	if match.Depth != 1 {
		t.Fatalf("depth = %d", match.Depth)
	}
	if result.StructuralBreadth != impact.ModuleBreadth || result.Breadth != impact.ModuleBreadth {
		t.Fatalf("breadth = %q / %q, want MODULE", result.StructuralBreadth, result.Breadth)
	}
}

// TestAnalyzeSameNameAmbiguityNeverCollapses is TASK-02 AC-02.3. Two
// declarations sharing one name produce one entry naming every candidate
// uid: no choice, no merge, still fully explained.
func TestAnalyzeSameNameAmbiguityNeverCollapses(t *testing.T) {
	fx := newImpactFixture(t)
	seedSymbol(t, fx, "SYM-M-A", "a", "fa", "/r/a.py")
	seedSymbol(t, fx, "SYM-M-W1", "w1", "fa", "/r/c.py")
	seedSymbol(t, fx, "SYM-M-W2", "w2", "fa", "/r/d.py")

	result, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols: []impact.Input{{UID: "SYM-M-A"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var ambiguous *impact.Entry
	for i, entry := range result.Entries {
		if entry.Via.Kind == impact.NameMatchEdge {
			t.Fatalf("collision collapsed to name-match: %+v", entry)
		}
		if entry.Via.Kind == impact.AmbiguousEdge {
			if ambiguous != nil {
				t.Fatalf("two ambiguous entries: %+v", result.Entries)
			}
			ambiguous = &result.Entries[i]
		}
	}
	if ambiguous == nil {
		t.Fatalf("no ambiguous entry in %+v", result.Entries)
	}
	for _, uid := range []string{"SYM-M-W1", "SYM-M-W2"} {
		if !strings.Contains(ambiguous.FallbackReason, uid) {
			t.Fatalf("reason names no %q: %q", uid, ambiguous.FallbackReason)
		}
	}
	if ambiguous.Confidence > 0.5 || ambiguous.FallbackReason == "" {
		t.Fatalf("ambiguous = %+v", ambiguous)
	}
}

// TestAnalyzeFileFallbackReportsFloor is TASK-02 AC-02.2. The changed file
// (and its unit) is reported with its reason at MODULE breadth, and the
// fallback invents no symbol edges: the file entry's target is the analyzed
// symbol itself.
func TestAnalyzeFileFallbackReportsFloor(t *testing.T) {
	fx := newImpactFixture(t)
	seedSymbol(t, fx, "SYM-F-A", "a", "fa", "/r/a.py")

	result, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols: []impact.Input{{UID: "SYM-F-A"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var floor *impact.Entry
	for i, entry := range result.Entries {
		if entry.Via.Kind == impact.FileEdge {
			floor = &result.Entries[i]
		}
	}
	if floor == nil {
		t.Fatalf("no file entry in %+v", result.Entries)
	}
	if floor.TargetUID != "SYM-F-A" || floor.TargetPath != "/r/a.py" {
		t.Fatalf("floor = %+v", floor)
	}
	if floor.FallbackReason == "" {
		t.Fatal("file entry names no reason")
	}
	if result.Breadth != impact.ModuleBreadth {
		t.Fatalf("breadth = %q, want MODULE", result.Breadth)
	}
}

// TestAnalyzeListsBoundInvariants is TASK-02 AC-02.4. Bound invariants are
// listed beside their symbols from the bindings table with explicit scope
// text; non-bound statuses never appear.
func TestAnalyzeListsBoundInvariants(t *testing.T) {
	fx := newImpactFixture(t)
	a := seedSymbol(t, fx, "SYM-V-A", "a", "fa", "/r/a.py")
	seedSymbol(t, fx, "SYM-V-B", "b", "fb", "/r/a.py")
	seedReference(t, fx, "b", "fa", a)
	bind := func(invariant, uid, status string) {
		t.Helper()
		if _, err := fx.db.ExecContext(t.Context(), `INSERT INTO invariant_symbol_bindings
			(invariant_id, symbol_uid, status, updated_at)
			VALUES (?, ?, ?, '2026-09-23T10:00:00Z')`, invariant, uid, status); err != nil {
			t.Fatal(err)
		}
	}
	bind("INV-1", "SYM-V-A", "bound")
	bind("INV-9", "SYM-V-A", "orphaned")

	result, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols: []impact.Input{{UID: "SYM-V-A"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) == 0 {
		t.Fatal("no entries")
	}
	for _, entry := range result.Entries {
		if !containsString(entry.Invariants, "INV-1") {
			t.Fatalf("entry misses INV-1: %+v", entry)
		}
		if containsString(entry.Invariants, "INV-9") {
			t.Fatalf("orphaned binding listed: %+v", entry)
		}
		if !strings.Contains(entry.ScopeJustification, "INV-1") {
			t.Fatalf("no scope text: %+v", entry)
		}
	}
}

// TestAnalyzeDirectOnlySkipsFallbacks pins the AC-01.4 mode: with DirectOnly
// the file floor, name users and invariants-by-fallback stay out and breadth
// holds TARGETED.
func TestAnalyzeDirectOnlySkipsFallbacks(t *testing.T) {
	fx := newImpactFixture(t)
	seedSymbol(t, fx, "SYM-O-A", "a", "fa", "/r/a.py")
	seedSymbol(t, fx, "SYM-O-W", "w", "fa", "/r/c.py")

	result, err := fx.service.Analyze(t.Context(), impact.Request{
		Symbols:    []impact.Input{{UID: "SYM-O-A"}},
		DirectOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("entries = %+v, want none without direct edges", result.Entries)
	}
	if result.Breadth != impact.TargetedBreadth {
		t.Fatalf("breadth = %q", result.Breadth)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
