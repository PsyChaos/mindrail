package record_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/internal/knowledge/schema"
)

// This file is the adjacent healthy case for decision D-49, written from the
// writer's side rather than the schema's.
//
// D-49 narrowed a shipped v1 contract: both schema documents gained a `pattern`
// on created_at, so a spelling that validated yesterday may not today. That is a
// live behaviour change for existing repositories, and the case that matters is
// not the one the fix was aimed at — it is the one beside it. If any instant a
// Mindrail binary can stamp renders as a spelling the new pattern refuses, the
// fix has made this project unable to read its own output, which is worse by a
// wide margin than the four unloadable spellings it removed.
//
// FIX-D asserted this over the instants its own tests happened to use. These
// rows are chosen instead for the places a timestamp's *rendering* changes
// shape: whether a fractional part appears at all, how many digits it has after
// Go trims trailing zeros, and where the year, month and second run out of
// digits.

// TestEveryInstantTheWriterCanStampRendersAsASpellingTheContractAccepts walks
// the boundaries of Go's RFC 3339 rendering and asks the shipped documents about
// each result.
func TestEveryInstantTheWriterCanStampRendersAsASpellingTheContractAccepts(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		// rendersWith is a fragment the marshalled form must contain, so a row
		// that stopped exercising what it was written for fails loudly instead
		// of passing on a different spelling.
		rendersWith string
	}{
		{
			name:        "no fractional part at all",
			at:          time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
			rendersWith: "T03:04:05Z",
		},
		{
			name:        "one fractional digit, because Go trims the trailing zeros",
			at:          time.Date(2026, time.January, 2, 3, 4, 5, 100000000, time.UTC),
			rendersWith: ".1Z",
		},
		{
			name:        "three fractional digits",
			at:          time.Date(2026, time.January, 2, 3, 4, 5, 123000000, time.UTC),
			rendersWith: ".123Z",
		},
		{
			name:        "the full nine fractional digits, which is the pattern's ceiling",
			at:          time.Date(2026, time.January, 2, 3, 4, 5, 123456789, time.UTC),
			rendersWith: ".123456789Z",
		},
		{
			name:        "a single nanosecond, which renders as nine digits of mostly zero",
			at:          time.Date(2026, time.January, 2, 3, 4, 5, 1, time.UTC),
			rendersWith: ".000000001Z",
		},
		{
			name:        "the last nanosecond of a second",
			at:          time.Date(2026, time.January, 2, 3, 4, 5, 999999999, time.UTC),
			rendersWith: ".999999999Z",
		},
		{
			name:        "midnight, where every clock field is at its minimum",
			at:          time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			rendersWith: "2026-01-01T00:00:00Z",
		},
		{
			name:        "the last second of a year, where every clock field is at its maximum",
			at:          time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC),
			rendersWith: "2026-12-31T23:59:59Z",
		},
		{
			name:        "a leap day, which only the calendar-aware keyword can judge",
			at:          time.Date(2024, time.February, 29, 12, 0, 0, 0, time.UTC),
			rendersWith: "2024-02-29T",
		},
		{
			name:        "the smallest four-digit year",
			at:          time.Date(1000, time.January, 1, 0, 0, 0, 0, time.UTC),
			rendersWith: "1000-01-01T",
		},
		{
			name:        "a three-digit year, which Go pads to four",
			at:          time.Date(999, time.January, 1, 0, 0, 0, 0, time.UTC),
			rendersWith: "0999-01-01T",
		},
		{
			name:        "the largest four-digit year",
			at:          time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC),
			rendersWith: "9999-12-31T",
		},
		{
			// The constructors call .UTC(), so an offset instant must arrive as
			// Z rather than as the offset it was written with. This row is the
			// one that would fail if D-49's Z-anchored spelling and the writer's
			// normalisation ever stopped agreeing.
			name:        "an eastern offset, which the constructor normalises to Z",
			at:          time.Date(2026, time.January, 2, 6, 4, 5, 0, time.FixedZone("east", 3*3600)),
			rendersWith: "T03:04:05Z",
		},
		{
			name:        "a western offset, likewise",
			at:          time.Date(2026, time.January, 1, 22, 4, 5, 0, time.FixedZone("west", -5*3600)),
			rendersWith: "2026-01-02T03:04:05Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := record.NewDecision("DEC-0001", tt.at, "A title", "A decision.")
			if err != nil {
				t.Fatalf("NewDecision() error = %v, want nil", err)
			}
			invariant, err := record.NewInvariant("INV-0001", tt.at, "A statement.",
				record.WithSeverity(record.SeverityHigh),
				record.WithScope(record.Scope{Level: record.ScopeProject}))
			if err != nil {
				t.Fatalf("NewInvariant() error = %v, want nil", err)
			}

			// The rendering is asserted before the schema is asked, so a row
			// whose fixture stopped producing the shape it names cannot pass by
			// exercising a shape another row already covers.
			for _, built := range []any{decision, invariant} {
				encoded, err := json.Marshal(built)
				if err != nil {
					t.Fatalf("marshalling %T: %v", built, err)
				}
				if !strings.Contains(string(encoded), tt.rendersWith) {
					t.Fatalf("%T rendered created_at without %q: %s", built, tt.rendersWith, encoded)
				}
			}

			// Both documents, because D-49 changed both and a rule stated in
			// only one of them is the hole FIX-D's own mutation found.
			requireValid(t, schema.KindDecision, decision)
			requireValid(t, schema.KindInvariant, invariant)
		})
	}
}

// TestTheWriterStillRefusesTheInstantItAlwaysRefused is the other side of the
// row above: D-49 narrowed the schema, and narrowing a schema must not quietly
// widen the writer. The zero time is the one instant a caller can hand these
// constructors that the document would happily accept and they must not.
func TestTheWriterStillRefusesTheInstantItAlwaysRefused(t *testing.T) {
	if _, err := record.NewDecision("DEC-0001", time.Time{}, "A title", "A decision."); err == nil {
		t.Error("NewDecision(zero time) error = nil, want a refusal; a record dating itself to year one is a caller who forgot the clock")
	}
	if _, err := record.NewInvariant("INV-0001", time.Time{}, "A statement.",
		record.WithSeverity(record.SeverityHigh),
		record.WithScope(record.Scope{Level: record.ScopeProject})); err == nil {
		t.Error("NewInvariant(zero time) error = nil, want a refusal")
	}
}
