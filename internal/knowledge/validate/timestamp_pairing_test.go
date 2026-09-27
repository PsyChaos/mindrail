package validate_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
)

// This file is finding B-05's regression test: the pairing AC-01.5 asserts in
// one direction, asserted in the other.
//
// AC-01.5 says every record the constructors write passes the full pipeline.
// The dual — every record the pipeline passes, the package's own types can load
// — was never stated and was false. `format: "date-time"` is RFC 3339, and
// RFC 3339 admits spellings Go's time.Time cannot parse: a lowercase "t" or "z"
// (RFC 3339 §5.6 makes the literals case-insensitive), and a leap second
// (§5.7). It also admits any offset, while the property's own description and
// tech-stack §42 both say UTC. So the pipeline called records clean that
// record.Decision could not load, and clean records that were not UTC.
//
// Decision D-49 closed it in the schema documents rather than in Go — see the
// requirements document for the argument. What this file holds is the property,
// not the mechanism: it drives every spelling through both readers from one
// table, so the day either side moves without the other, a row goes red and
// names which reader disagreed.

// timestampReading is one reader's verdict on one spelling. Only three states
// exist, and "loaded a different instant" is deliberately one of them: a reader
// that silently truncates is not agreeing with the schema, it is disagreeing
// quietly, which is worse.
type timestampReading struct {
	accepted bool
	instant  time.Time
}

// readBySchema runs spec §95 step 5 — the shipped documents, which are the
// contract (D-36) — over this created_at, in BOTH kinds.
//
// Both, because both documents declare the property and a fix applied to one
// document only is invisible to a decision-shaped table. That was not
// hypothetical: removing the pattern from invariant.v1 alone left this file
// green until this function was made to ask twice. The two kinds are required
// to agree rather than merely both being asked, so a record's timestamp cannot
// become valid by changing which directory it lives in.
func readBySchema(t *testing.T, stamp string) bool {
	t.Helper()

	validator := shippedValidator(t)

	decisionStore := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{"created_at": stamp})),
	})
	byDecision := len(findingsAt(validate.Check(decisionStore, validator), validate.StepSchema)) == 0

	invariantStore := storeOf(t, []testRecord{
		invariantAt("INV-0001.json", invariantDoc("INV-0001", map[string]any{"created_at": stamp})),
	})
	byInvariant := len(findingsAt(validate.Check(invariantStore, validator), validate.StepSchema)) == 0

	if byDecision != byInvariant {
		t.Fatalf("created_at %q is accepted by one kind and refused by the other "+
			"(decision accepted=%v, invariant accepted=%v); both documents declare the same rule, "+
			"so a timestamp must not become valid by moving the record to another directory",
			stamp, byDecision, byInvariant)
	}
	return byDecision
}

// readByRecord decodes the same document into record.Decision, which is what a
// consumer of this package holds a knowledge record in.
func readByRecord(t *testing.T, stamp string) timestampReading {
	t.Helper()
	body := decisionDoc("DEC-0001", map[string]any{"created_at": stamp})
	var decoded record.Decision
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		return timestampReading{}
	}
	return timestampReading{accepted: true, instant: decoded.CreatedAt}
}

// readByInvariantRecord is the second kind. Both documents carry the same
// created_at rule, so a fix applied to one and not the other must fail.
func readByInvariantRecord(t *testing.T, stamp string) timestampReading {
	t.Helper()
	body := invariantDoc("INV-0001", map[string]any{"created_at": stamp})
	var decoded record.Invariant
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		return timestampReading{}
	}
	return timestampReading{accepted: true, instant: decoded.CreatedAt}
}

// TestNoTimestampIsAcceptedByTheSchemaAndRejectedByTheTypesThatLoadIt is the
// invariant, and it is one-directional on purpose.
//
// "The schema rejects what Go would have accepted" is harmless: nothing reaches
// a Go reader without passing step 5 first, so a stricter contract only means
// the repository is told about a record sooner. "The schema accepts what Go
// cannot load" is the defect — it publishes a record as clean and then fails
// somewhere with no finding to explain it. Only the second direction is
// asserted, so that tightening the contract further never fails this test for
// the wrong reason.
func TestNoTimestampIsAcceptedByTheSchemaAndRejectedByTheTypesThatLoadIt(t *testing.T) {
	for _, stamp := range everyTimestampSpelling() {
		t.Run(stamp.name, func(t *testing.T) {
			if !readBySchema(t, stamp.value) {
				return // The contract refused it; no Go reader is ever handed it.
			}

			for _, reader := range []struct {
				name string
				read func(*testing.T, string) timestampReading
			}{
				{"record.Decision", readByRecord},
				{"record.Invariant", readByInvariantRecord},
			} {
				got := reader.read(t, stamp.value)
				if !got.accepted {
					t.Errorf("step 5 accepts created_at %q but %s cannot decode it; "+
						"the pipeline would call this record clean and no consumer could load it",
						stamp.value, reader.name)
					continue
				}
				if !got.instant.Equal(stamp.denotes) {
					t.Errorf("step 5 accepts created_at %q and %s loads it as %s, want %s; "+
						"a reader that loads a different instant disagrees with the contract quietly",
						stamp.value, reader.name, got.instant.UTC().Format(time.RFC3339Nano),
						stamp.denotes.UTC().Format(time.RFC3339Nano))
				}
			}

			// app.ParseTime is the project's declared reader of persisted
			// timestamps (tech-stack §42). It is not on the knowledge path today,
			// which is exactly why it is asserted here: the next reader to be
			// pointed at a record must not reopen this finding.
			if _, err := app.ParseTime(stamp.value); err != nil {
				t.Errorf("step 5 accepts created_at %q but app.ParseTime rejects it: %v", stamp.value, err)
			}
		})
	}
}

// TestTheTimestampSpellingsTheContractAcceptsAreExactlyTheOnesItShould is the
// over-fire guard, and it is the half that matters most here.
//
// Narrowing a shipped schema document is the move that takes a working
// repository down: every row below marked accepted is a record somebody may
// already have committed, and D-49 is only defensible if none of them stopped
// validating. The two UTC offset spellings are the sharp edge — "+00:00" is
// what most ISO-8601 libraries emit and it denotes exactly the same instant as
// "Z", so a pattern anchored on "Z$" would have been a fresh over-fire dressed
// up as a fix.
func TestTheTimestampSpellingsTheContractAcceptsAreExactlyTheOnesItShould(t *testing.T) {
	for _, stamp := range everyTimestampSpelling() {
		t.Run(stamp.name, func(t *testing.T) {
			if got := readBySchema(t, stamp.value); got != stamp.valid {
				verb := map[bool]string{true: "accepts", false: "rejects"}
				t.Fatalf("step 5 %s created_at %q, want it to %s it — %s",
					verb[got], stamp.value, map[bool]string{true: "accept", false: "reject"}[stamp.valid], stamp.why)
			}
		})
	}
}

// TestEveryRecordTheConstructorsWriteCarriesATimestampTheContractAccepts keeps
// D-49 from breaking the writer.
//
// A narrowed created_at rule is only safe if the one thing that writes records
// still satisfies it. The three instants below are the shapes time.Time can
// marshal: whole seconds, a short fraction, and full nanosecond precision — the
// last is the one a bound of nine fractional digits could have clipped.
func TestEveryRecordTheConstructorsWriteCarriesATimestampTheContractAccepts(t *testing.T) {
	instants := map[string]time.Time{
		"whole seconds":         time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		"a short fraction":      time.Date(2026, time.January, 2, 3, 4, 5, 500000000, time.UTC),
		"full nanosecond":       time.Date(2026, time.January, 2, 3, 4, 5, 123456789, time.UTC),
		"a non-UTC input":       time.Date(2026, time.January, 2, 3, 4, 5, 0, time.FixedZone("MSK", 3*60*60)),
		"a non-UTC input, nano": time.Date(2026, time.January, 2, 3, 4, 5, 987654321, time.FixedZone("EST", -5*60*60)),
	}

	for name, at := range instants {
		t.Run(name, func(t *testing.T) {
			decision, err := record.NewDecision("DEC-0001", at, "A title", "A decision.")
			if err != nil {
				t.Fatalf("NewDecision() error = %v, want nil", err)
			}
			body, err := json.Marshal(decision)
			if err != nil {
				t.Fatalf("marshalling the decision: %v", err)
			}

			store := storeOf(t, []testRecord{decisionAt("DEC-0001.json", string(body))})
			if findings := validate.Check(store, shippedValidator(t)); len(findings) != 0 {
				t.Fatalf("a record NewDecision wrote fails its own pipeline:\n\t%s\n\tdocument: %s",
					describe(findings), body)
			}
		})
	}
}

// timestampSpelling is one row of the single table both directions read.
//
// valid and denotes are written as literals rather than computed, so the table
// cannot agree with a wrong implementation by sharing its arithmetic — the trap
// MR-001's second audit round was spent on.
type timestampSpelling struct {
	name    string
	value   string
	valid   bool
	denotes time.Time
	why     string
}

func everyTimestampSpelling() []timestampSpelling {
	utc := func(h, m, s, ns int) time.Time {
		return time.Date(2026, time.January, 1, h, m, s, ns, time.UTC)
	}

	return []timestampSpelling{
		// The control the finding measured, and the spelling every record this
		// binary has ever written carries.
		{name: "the canonical spelling", value: "2026-01-01T00:00:00Z", valid: true, denotes: utc(0, 0, 0, 0),
			why: "the form time.Time marshals and every committed record carries"},
		{name: "the golden fixtures' own stamp", value: "2026-01-02T03:04:05Z", valid: true,
			denotes: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
			why:     "both testdata goldens carry exactly this; rejecting it breaks AC-01.6"},

		// Fractional seconds. time.Time marshals up to nine digits and no more,
		// so nine is the boundary and it must be inside.
		{name: "a one-digit fraction", value: "2026-01-01T00:00:00.5Z", valid: true, denotes: utc(0, 0, 0, 500000000),
			why: "RFC3339Nano emits this for a half-second instant"},
		{name: "nine fractional digits", value: "2026-01-01T00:00:00.123456789Z", valid: true,
			denotes: utc(0, 0, 0, 123456789), why: "full nanosecond precision, the most time.Time can hold"},

		// The offsets that denote UTC. These are the over-fire trap: both are
		// currently valid, both parse, and both mean the same instant as Z.
		{name: "a plus-zero offset", value: "2026-01-01T00:00:00+00:00", valid: true, denotes: utc(0, 0, 0, 0),
			why: "what most ISO-8601 libraries emit for UTC; it denotes the same instant as Z"},
		{name: "a minus-zero offset", value: "2026-01-01T00:00:00-00:00", valid: true, denotes: utc(0, 0, 0, 0),
			why: "RFC 3339's 'offset unknown', still the same instant, still parsed by Go"},

		// The four spellings finding B-05 measured as split.
		{name: "a lowercase zone designator", value: "2026-01-01T00:00:00z", valid: false, denotes: time.Time{},
			why: "RFC 3339 §5.6 permits it and Go cannot parse it — half of the split B-05 found"},
		{name: "a lowercase date-time separator", value: "2026-01-01t00:00:00Z", valid: false, denotes: time.Time{},
			why: "RFC 3339 §5.6 permits it and Go cannot parse it"},
		{name: "a leap second", value: "2026-12-31T23:59:60Z", valid: false, denotes: time.Time{},
			why: "RFC 3339 §5.7 permits it and time.Time has no representation for it at all"},
		{name: "a western offset", value: "2026-01-01T00:00:00-05:00", valid: false, denotes: time.Time{},
			why: "parses, but the property's own description and tech-stack §42 say UTC"},
		{name: "an eastern offset", value: "2026-01-01T00:00:00+03:00", valid: false, denotes: time.Time{},
			why: "same: a valid RFC 3339 instant that is not the UTC spelling this project persists"},
		{name: "more precision than time.Time holds", value: "2026-01-01T00:00:00.1234567890123Z", valid: false,
			denotes: time.Time{},
			why:     "Go accepts it and silently truncates to nine digits, which loads a different instant"},

		// Spellings both readers already refused. They are here so that a change
		// to the pattern cannot quietly stop one of them being caught.
		{name: "a space instead of the separator", value: "2026-01-01 00:00:00Z", valid: false, denotes: time.Time{},
			why: "the control the finding measured as agreed-rejected"},
		{name: "a day February does not have", value: "2026-02-30T00:00:00Z", valid: false, denotes: time.Time{},
			why: "only \"format\" can decide this; no regular expression knows how long February is"},
		{name: "an hour outside the day", value: "2026-01-01T24:00:00Z", valid: false, denotes: time.Time{},
			why: "\"format\" owns it"},
		{name: "a month outside the year", value: "2026-13-01T00:00:00Z", valid: false, denotes: time.Time{},
			why: "\"format\" owns it"},
		{name: "an hour and minute out of range", value: "2026-01-01T99:99:59Z", valid: false, denotes: time.Time{},
			why: "\"pattern\" deliberately does not bound these; \"format\" does"},
		{name: "prose instead of a timestamp", value: "yesterday", valid: false, denotes: time.Time{},
			why: "D-48's witness"},
		{name: "no zone at all", value: "2026-01-01T00:00:00", valid: false, denotes: time.Time{},
			why: "RFC 3339 requires an offset"},
		{name: "a date with no time", value: "2026-01-01", valid: false, denotes: time.Time{},
			why: "a date is not a date-time"},
		{name: "an offset without its colon", value: "2026-01-01T00:00:00+0000", valid: false, denotes: time.Time{},
			why: "RFC 3339 requires the colon; \"format\" already refused it"},
	}
}
