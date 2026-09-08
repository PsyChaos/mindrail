package validate_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/knowledge/record"
	"github.com/PsyChaos/mindrail/schemas"
)

// This file is finding R-02's regression, and it is about a reason rather than a
// behaviour.
//
// D-49 narrowed created_at in both shipped documents and gave one reason for all
// six newly-refused spellings: that each was already a record this binary could
// not load, or could not load faithfully. Measured, that is true of three of them
// and false of the other three. A non-zero offset such as "-05:00" loads through
// every reader in this project and names exactly the instant it spells; twelve
// fractional digits whose tail is zero loads the same instant "Z" would. They are
// refused, correctly, because tech-stack §42 fixes how a persisted timestamp is
// spelled — a different ground, and one D-49 did not record.
//
// The behaviour did not change and must not: the pattern in both documents is
// byte-for-byte what D-49 shipped. What changed is the recorded reason, in D-49
// and in both properties' own "description". That description is not decoration
// any more — after finding R-04, step 5's message for a created_at "pattern"
// violation stops reproducing the expression and sends the reader to the
// description instead, so a false sentence there is the remedy a user is handed.
//
// Two halves, and both are needed. Measuring the grounds alone would stay green
// over the old, false prose; asserting the prose alone would stay green if the
// pattern later moved underneath it. Every claim below is therefore a pair: the
// sentence the shipped documents must carry, and the measurement that sentence
// has to survive.

// refusalGround is why a spelling is refused. The three values are ordered by how
// much harm accepting the spelling would do, and keeping them apart is the whole
// of R-02: "no reader can load this" and "every reader loads this correctly, but
// it is not how we spell it" are both good reasons to refuse, and recording the
// first when the second is true makes the contract lie about the repository.
type refusalGround int

const (
	// groundUnloadable — no reader in this project can decode the spelling.
	groundUnloadable refusalGround = iota
	// groundLossy — the readers decode it, as some other instant than it names.
	groundLossy
	// groundSpelling — every reader decodes it as exactly the instant it names.
	// Only tech-stack §42's rule about how a persisted timestamp is written
	// refuses it.
	groundSpelling
)

func (g refusalGround) String() string {
	switch g {
	case groundUnloadable:
		return "no reader in this project can load it"
	case groundLossy:
		return "the readers load it as a different instant"
	case groundSpelling:
		return "every reader loads exactly the instant it names; only §42's spelling rule refuses it"
	default:
		return "unclassified"
	}
}

// refusedSpelling is one row: a created_at both shipped documents refuse, the
// instant it names, and the ground D-49 now records for refusing it.
//
// denotes is a literal rather than a value parsed from value, for the reason the
// B-05 table gives: a row that computed its expectation with the code under test
// would agree with a wrong implementation.
type refusedSpelling struct {
	name    string
	value   string
	ground  refusalGround
	denotes time.Time
	why     string
}

func everyRefusedSpelling() []refusedSpelling {
	return []refusedSpelling{
		// The three D-49 described correctly. Go's time.RFC3339 layout does not
		// accept a case-folded literal, and time.Time cannot represent a leap
		// second at all.
		{name: "a lowercase zone designator", value: "2026-01-01T00:00:00z", ground: groundUnloadable,
			why: "RFC 3339 §5.6 permits it; no reader here parses it"},
		{name: "a lowercase date-time separator", value: "2026-01-01t00:00:00Z", ground: groundUnloadable,
			why: "RFC 3339 §5.6 permits it; no reader here parses it"},
		{name: "a leap second", value: "2026-12-31T23:59:60Z", ground: groundUnloadable,
			why: "RFC 3339 §5.7 permits it; time.Time has no representation for it"},

		// The row D-49 described correctly only by accident: this particular
		// value loses digits that are not zero, so it does load a different
		// instant. It is here to keep the fix from over-correcting — the lossy
		// ground is real and must not be argued away with the offsets.
		{name: "twelve fractional digits with a non-zero tail", value: "2026-01-01T00:00:00.123456789012Z",
			ground: groundLossy, denotes: time.Date(2026, time.January, 1, 0, 0, 0, 123456789012, time.UTC),
			why: "Go truncates to nine digits, so the record loads as an instant it does not name"},

		// R-02(b). Same shape, zero tail, nothing lost — and D-49 claimed the
		// whole class always loads a different instant.
		{name: "twelve fractional digits with a zero tail", value: "2026-01-01T00:00:00.000000000000Z",
			ground: groundSpelling, denotes: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			why: "nothing is lost; it is refused because §42 fixes the spelling, not because it misreads"},

		// R-02(a). Three offsets, west, east and half-hour, each loading the
		// instant it names.
		{name: "a western offset", value: "2026-01-01T00:00:00-05:00", ground: groundSpelling,
			denotes: time.Date(2026, time.January, 1, 5, 0, 0, 0, time.UTC),
			why:     "loads as 05:00Z, exactly what it says; refused on §42's spelling rule alone"},
		{name: "an eastern offset", value: "2026-01-01T00:00:00+03:00", ground: groundSpelling,
			denotes: time.Date(2025, time.December, 31, 21, 0, 0, 0, time.UTC),
			why:     "loads as the previous day 21:00Z, exactly what it says"},
		{name: "a half-hour offset", value: "2026-01-01T00:00:00+05:30", ground: groundSpelling,
			denotes: time.Date(2025, time.December, 31, 18, 30, 0, 0, time.UTC),
			why:     "the shape a whole-hour-only reading of the rule would have missed"},
	}
}

// measuredGround asks all three readers what they do with a spelling and derives
// the ground from the answers rather than from anything written down.
//
// The three are required to agree about loadability. A spelling one reader takes
// and another refuses is not a ground, it is a second B-05, and this function
// fails rather than picking a winner.
func measuredGround(t *testing.T, value string, denotes time.Time) refusalGround {
	t.Helper()

	byDecision := readByRecord(t, value)
	byInvariant := readByInvariantRecord(t, value)
	parsed, parseErr := app.ParseTime(value)
	loadedByAll := byDecision.accepted && byInvariant.accepted && parseErr == nil
	loadedByNone := !byDecision.accepted && !byInvariant.accepted && parseErr != nil

	if loadedByNone {
		return groundUnloadable
	}
	if !loadedByAll {
		t.Fatalf("created_at %q is loaded by some readers and not others "+
			"(record.Decision=%v, record.Invariant=%v, app.ParseTime err=%v); "+
			"that is a split between readers, not a ground for refusing a spelling",
			value, byDecision.accepted, byInvariant.accepted, parseErr)
	}
	if !byDecision.instant.Equal(denotes) || !byInvariant.instant.Equal(denotes) || !parsed.Equal(denotes) {
		return groundLossy
	}
	return groundSpelling
}

// TestEveryTimestampTheContractRefusesIsRefusedOnTheGroundTheDocumentsRecord is
// the under-fire arm and the classification in one.
//
// Under-fire first: every row must still be refused. R-02 is a finding about a
// false reason, and the repair that would be worst is one that made the reason
// true by widening the rule — the auditor's option 1, which would hand the offset
// question back to "format", and "format" accepts every offset RFC 3339 allows.
// Measured at 8e5c2e5, where there was no pattern: "…T00:00:00-05:00" produced
// READY and zero findings.
//
// Then the ground. If a row's measured ground stops matching the one recorded,
// either the readers changed or the documents are describing a repository that no
// longer exists; both are worth a red line.
func TestEveryTimestampTheContractRefusesIsRefusedOnTheGroundTheDocumentsRecord(t *testing.T) {
	for _, row := range everyRefusedSpelling() {
		t.Run(row.name, func(t *testing.T) {
			if readBySchema(t, row.value) {
				t.Fatalf("step 5 accepts created_at %q; both shipped documents refuse it — %s", row.value, row.why)
			}
			if got := measuredGround(t, row.value, row.denotes); got != row.ground {
				t.Fatalf("created_at %q is refused on the ground %q, and the documents record %q — %s",
					row.value, got, row.ground, row.why)
			}
		})
	}
}

// TestTheCreatedAtDescriptionIsTheSameSentenceInBothDocuments extends the drift
// guard to the half that became load-bearing.
//
// TestBothDocumentsStateTheSameCreatedAtRule already holds the two "pattern"
// values together, and that was the whole rule when it was written. It is not any
// more: step 5's message for a created_at "pattern" violation names the
// property's own "description" as the rule, so a description that drifted between
// the two documents would hand a decision's author and an invariant's author
// different contracts for the same field. The mutation that motivated the older
// test — deleting the pattern from invariant.v1 alone — has an exact twin here.
func TestTheCreatedAtDescriptionIsTheSameSentenceInBothDocuments(t *testing.T) {
	decision := createdAtDescription(t, "decision.v1.schema.json")
	invariant := createdAtDescription(t, "invariant.v1.schema.json")

	if strings.TrimSpace(decision) == "" {
		t.Fatal("decision.v1.schema.json states no created_at description, and step 5's message sends the reader to it")
	}
	if decision != invariant {
		t.Fatalf("the two documents describe created_at differently:\n\tdecision.v1:  %q\n\tinvariant.v1: %q",
			decision, invariant)
	}
}

// descriptionClaim is one sentence the shipped description has to carry, paired
// with the measurement that makes it true.
//
// This pairing is the point of the file. "A decision whose stated reason is false
// is a defect even when its behaviour is right" is not a property a test can read
// off the code, so it is split: says is asserted against the shipped document, and
// check is asserted against the readers. Reverting either half turns a row red.
type descriptionClaim struct {
	name  string
	says  string
	check func(*testing.T)
}

func everyDescriptionClaim() []descriptionClaim {
	return []descriptionClaim{
		{
			name: "it tells the reader which zone spellings to write",
			says: `a zero offset spelled "Z", "+00:00" or "-00:00"`,
			check: func(t *testing.T) {
				for _, zone := range []string{"Z", "+00:00", "-00:00"} {
					if !readBySchema(t, "2026-01-01T00:00:00"+zone) {
						t.Errorf("the description offers the zone %q and step 5 refuses it", zone)
					}
				}
				// And nothing else: a fourth acceptable zone would make the
				// sentence an incomplete answer to the reader it is given to.
				for _, zone := range []string{"+00:01", "-00:01", "+01:00"} {
					if readBySchema(t, "2026-01-01T00:00:00"+zone) {
						t.Errorf("step 5 also accepts the zone %q, which the description does not offer", zone)
					}
				}
			},
		},
		{
			name: "it states the precision bound",
			says: "at most nine fractional-second digits",
			check: func(t *testing.T) {
				if !readBySchema(t, "2026-01-01T00:00:00.123456789Z") {
					t.Error("the description allows nine fractional digits and step 5 refuses them")
				}
				if readBySchema(t, "2026-01-01T00:00:00.1234567890Z") {
					t.Error("step 5 accepts ten fractional digits, which the description says it does not")
				}
			},
		},
		{
			name: "it says the spelling it names is the one this project writes",
			says: "the only one this project writes",
			check: func(t *testing.T) {
				// The constructor is the only writer of records, and it is handed
				// a deliberately hostile instant: a half-hour eastern offset, the
				// shape the pattern refuses.
				at := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.FixedZone("IST", 5*60*60+30*60))
				decision, err := record.NewDecision("DEC-0001", at, "A title", "A decision.")
				if err != nil {
					t.Fatalf("NewDecision() error = %v, want nil", err)
				}
				written, err := json.Marshal(decision.CreatedAt)
				if err != nil {
					t.Fatalf("marshalling the written timestamp: %v", err)
				}
				stamp := strings.Trim(string(written), `"`)
				if !readBySchema(t, stamp) {
					t.Errorf("the constructor wrote created_at %q and step 5 refuses it", stamp)
				}
			},
		},
		{
			name: "it names unloadability as the ground for three spellings",
			says: "no reader in this project can load them at all",
			check: func(t *testing.T) {
				for _, value := range []string{"2026-01-01T00:00:00z", "2026-01-01t00:00:00Z", "2026-12-31T23:59:60Z"} {
					if got := measuredGround(t, value, time.Time{}); got != groundUnloadable {
						t.Errorf("the description says %q cannot be loaded at all, and the measured ground is %q",
							value, got)
					}
				}
			},
		},
		{
			name: "it does not claim a refused offset is misread",
			says: "the offset names exactly the instant it says",
			check: func(t *testing.T) {
				// The sentence R-02 exists for. Written as an instant literal so
				// the assertion cannot be satisfied by the same arithmetic that
				// produced the value.
				denotes := time.Date(2026, time.January, 1, 5, 0, 0, 0, time.UTC)
				if got := measuredGround(t, "2026-01-01T00:00:00-05:00", denotes); got != groundSpelling {
					t.Errorf("the description says the offset names exactly the instant it spells, "+
						"and the measured ground is %q", got)
				}
			},
		},
		{
			name: "it records the spelling rule as the ground for what still loads",
			says: "refused on the spelling rule alone",
			check: func(t *testing.T) {
				for _, row := range everyRefusedSpelling() {
					if row.ground != groundSpelling {
						continue
					}
					if readBySchema(t, row.value) {
						t.Errorf("the description says %q is refused, and step 5 accepts it", row.value)
					}
				}
			},
		},
		{
			name: "it qualifies the precision loss instead of generalising it",
			says: "loses precision whenever the extra digits are not zero",
			check: func(t *testing.T) {
				// Both halves of the qualifier. Asserting only the lossy row would
				// leave the old, unqualified claim green, which is exactly how
				// R-02(b) survived a full remediation round.
				lossy := time.Date(2026, time.January, 1, 0, 0, 0, 123456789012, time.UTC)
				if got := measuredGround(t, "2026-01-01T00:00:00.123456789012Z", lossy); got != groundLossy {
					t.Errorf("a non-zero twelve-digit tail measures as %q, and the description calls it lossy", got)
				}
				exact := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
				if got := measuredGround(t, "2026-01-01T00:00:00.000000000000Z", exact); got != groundSpelling {
					t.Errorf("a zero twelve-digit tail measures as %q, and the description says nothing is lost "+
						"when the extra digits are zero", got)
				}
			},
		},
	}
}

// TestTheCreatedAtDescriptionSaysOnlyThingsThatAreTrueOfThisRepository is R-02's
// regression proper.
//
// Before the amendment both documents said that of the spellings D-49 refuses,
// "none of those is a timestamp this project writes or can read back as the same
// instant". Measured, "-05:00" reads back as exactly the same instant, through
// record.Decision, record.Invariant and app.ParseTime alike. The sentence was
// false, it was the sentence step 5's own remedy points at, and a reader holding
// a record with an offset was being told to look for a bug that is not there.
func TestTheCreatedAtDescriptionSaysOnlyThingsThatAreTrueOfThisRepository(t *testing.T) {
	description := createdAtDescription(t, "decision.v1.schema.json")

	for _, claim := range everyDescriptionClaim() {
		t.Run(claim.name, func(t *testing.T) {
			if !strings.Contains(description, claim.says) {
				t.Fatalf("the shipped created_at description no longer carries %q, "+
					"so the measurement below guards nothing:\n\t%s", claim.says, description)
			}
			claim.check(t)
		})
	}
}

// TestTheAmendmentAcceptsEveryTimestampTheContractAcceptedBeforeIt is the
// over-fire arm, and for this finding it is the arm that decides the whole
// remedy.
//
// R-02 offered two repairs: widen the rule so the recorded reason becomes true,
// or correct the reason and leave the rule alone. The second was taken, so the
// accepted set must be bit-for-bit the set 3bd453c shipped — anything else is the
// behaviour change that was declined, arriving anyway.
//
// The last two rows are computed rather than spelled: whatever time.Time marshals
// is by definition what a record carries, so this cannot drift away from the
// writer even if the writer changes.
func TestTheAmendmentAcceptsEveryTimestampTheContractAcceptedBeforeIt(t *testing.T) {
	accepted := []struct{ name, value string }{
		{"the canonical spelling", "2026-01-01T00:00:00Z"},
		{"the golden fixtures' stamp", "2026-01-02T03:04:05Z"},
		{"a plus-zero offset", "2026-01-01T00:00:00+00:00"},
		{"a minus-zero offset", "2026-01-01T00:00:00-00:00"},
		{"one fractional digit", "2026-01-01T00:00:00.1Z"},
		{"three fractional digits", "2026-01-01T00:00:00.123Z"},
		{"six fractional digits", "2026-01-01T00:00:00.123456Z"},
		{"nine fractional digits", "2026-01-01T00:00:00.123456789Z"},
		{"the first year time.Time can name", "0001-01-01T00:00:00Z"},
		{"the last year four digits can name", "9999-12-31T23:59:59.999999999Z"},
		{"a plus-zero offset carrying a fraction", "2026-01-01T00:00:00.5+00:00"},
	}

	for _, row := range accepted {
		t.Run(row.name, func(t *testing.T) {
			if !readBySchema(t, row.value) {
				t.Fatalf("step 5 refuses created_at %q, which it accepted at 8e5c2e5 and at 3bd453c; "+
					"R-02's repair was to the recorded reason, not to the rule", row.value)
			}
		})
	}

	for _, at := range []time.Time{
		time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		time.Date(2026, time.January, 2, 3, 4, 5, 500000000, time.UTC),
		time.Date(2026, time.January, 2, 3, 4, 5, 123456789, time.UTC),
		time.Date(9999, time.December, 31, 23, 59, 59, 999999999, time.UTC),
	} {
		t.Run("what time.Time marshals for "+at.Format(time.RFC3339Nano), func(t *testing.T) {
			raw, err := json.Marshal(at)
			if err != nil {
				t.Fatalf("marshalling %s: %v", at, err)
			}
			stamp := strings.Trim(string(raw), `"`)
			if !readBySchema(t, stamp) {
				t.Fatalf("time.Time marshals %q and step 5 refuses it", stamp)
			}
		})
	}
}

// createdAtDescription reads a shipped document's created_at description.
//
// Read, never spelled, for the reason schemaPattern gives: the document is the
// contract, and a test carrying its own copy of the contract is guarding a copy.
func createdAtDescription(t *testing.T, document string) string {
	t.Helper()

	raw, err := schemas.KnowledgeFS.ReadFile("knowledge/" + document)
	if err != nil {
		t.Fatalf("reading the shipped %s: %v", document, err)
	}
	var doc struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decoding the shipped %s: %v", document, err)
	}
	return doc.Properties["created_at"].Description
}
