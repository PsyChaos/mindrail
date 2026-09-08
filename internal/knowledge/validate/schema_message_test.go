package validate_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/validate"
	"github.com/PsyChaos/mindrail/schemas"
)

// This file is about one property of step 5's text: a message that quotes a rule
// has to quote the rule that was applied.
//
// Step 5's wording is the schema library's, deliberately — decision D-36 makes
// the shipped document the contract and a second Go statement of any keyword
// would drift from it. The library renders a "pattern" violation by printing the
// regular expression through Go's %q, which doubles every backslash in it. For
// the two id patterns that is harmless, because they contain none. For
// created_at it publishes a different rule from the one being enforced.

// schemaPattern reads a property's "pattern" out of a shipped document.
//
// Read rather than spelled: this file must not carry a copy of any expression it
// is testing. The point of the fix is that the document is the only statement of
// the rule, and a test that restated it would be the drift it is guarding.
func schemaPattern(t *testing.T, document string, property string) string {
	t.Helper()

	raw, err := schemas.KnowledgeFS.ReadFile("knowledge/" + document)
	if err != nil {
		t.Fatalf("reading the shipped %s: %v", document, err)
	}
	var doc struct {
		Properties map[string]struct {
			Pattern     string `json:"pattern"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decoding the shipped %s: %v", document, err)
	}
	return doc.Properties[property].Pattern
}

// TestThePatternMessageForATimestampDoesNotMisquoteTheRule is the regression for
// R-04.
//
// Measured before the fix: a record whose created_at carries a non-UTC offset
// produced "does not match pattern '^…(\\.[0-9]{1,9})?(Z|[+-]00:00)$'". The
// doubled backslash makes that expression require a literal backslash before the
// fractional second — so "…T00:00:00.1Z", which the validator accepts, is
// refused by the rule the message prints. A reader who pastes it into a tester
// is debugging a rule that does not exist.
//
// The assertions are about what the message must NOT contain, because the harm
// is a specific false statement rather than a missing one. The pattern is read
// out of the shipped document so that changing the expression — another wave is
// doing exactly that — cannot make this test pass by accident or fail by drift.
func TestThePatternMessageForATimestampDoesNotMisquoteTheRule(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"created_at": "2026-01-01T00:00:00+01:00",
		})),
	})

	findings := findingsAt(validate.Check(store, shippedValidator(t)), validate.StepSchema)
	if len(findings) != 1 {
		t.Fatalf("step 5 reported %d findings, want the one pattern violation:\n\t%s",
			len(findings), describe(findings))
	}
	message := findings[0].Message

	// The row is only about "pattern"; if the document stops rejecting this
	// spelling with that keyword the test is measuring something else.
	if !strings.Contains(message, `"pattern"`) {
		t.Fatalf("this store no longer produces a pattern violation, so the test guards nothing: %q", message)
	}

	pattern := schemaPattern(t, "decision.v1.schema.json", "created_at")
	if pattern == "" {
		t.Fatal("the shipped decision document has no created_at pattern, so there is nothing to misquote")
	}
	if !strings.Contains(pattern, `\`) {
		t.Skip("the created_at pattern no longer contains a backslash, so Go quoting cannot misquote it")
	}

	// A message carrying an escaped copy of the expression is the defect. The
	// check is on the escape rather than on the expression, because the escaped
	// form is by construction not equal to the pattern this test just read.
	if strings.Contains(message, `\`) {
		t.Errorf("step 5's message reproduces an escaped regular expression: %q", message)
	}
	if strings.Contains(message, "does not match pattern") {
		t.Errorf("step 5's message still quotes the library's rendering of the rule: %q", message)
	}
	// And it has to leave the reader somewhere. The pointer names the field they
	// edit; the document and the description name the rule they edit it to.
	for _, fact := range []string{"/created_at", "decision.v1.schema.json", "description"} {
		if !strings.Contains(message, fact) {
			t.Errorf("step 5's message does not carry %q: %q", fact, message)
		}
	}
}

// TestTheCreatedAtRuleTheMessageSendsTheReaderToIsThere is the pairing that
// keeps the message's pointer from dangling.
//
// The fix above replaces a quoted expression with a reference: "the created_at
// property's own description in <document>". That is only a remedy while the
// description exists and says something. Both shipped documents are checked,
// because a message is produced for whichever kind the record is.
func TestTheCreatedAtRuleTheMessageSendsTheReaderToIsThere(t *testing.T) {
	for _, document := range []string{"decision.v1.schema.json", "invariant.v1.schema.json"} {
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
		if strings.TrimSpace(doc.Properties["created_at"].Description) == "" {
			t.Errorf("%s sends the reader to the created_at property's description and it is empty",
				document)
		}
	}
}

// TestAPatternMessageThatQuotesItsRuleFaithfullyIsStillPublished is the
// over-fire guard, and it is why the fix is one property rather than one
// keyword.
//
// Both documents pin the id and the supersedes items with expressions that
// contain no character %q escapes, so the library prints them exactly. That
// sentence is the most useful thing a reader with a malformed id can be given,
// and suppressing every "pattern" message to remove one misquote would trade a
// false statement for a whole class of missing ones.
//
// The expected text is read out of the shipped document, so this asserts the
// message and the contract agree rather than that the message contains a string
// this file happens to spell.
func TestAPatternMessageThatQuotesItsRuleFaithfullyIsStillPublished(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-1.json", decisionDoc("DEC-1", nil)),
	})

	findings := findingsAt(validate.Check(store, shippedValidator(t)), validate.StepSchema)
	var message string
	for _, finding := range findings {
		if strings.Contains(finding.Message, "/id") {
			message = finding.Message
		}
	}
	if message == "" {
		t.Fatalf("step 5 said nothing about a malformed id:\n\t%s", describe(findings))
	}

	pattern := schemaPattern(t, "decision.v1.schema.json", "id")
	if pattern == "" {
		t.Fatal("the shipped decision document has no id pattern, so this guards nothing")
	}
	if strings.Contains(pattern, `\`) {
		t.Skipf("the id pattern now contains a backslash (%s), so it is no longer a faithfully quotable rule", pattern)
	}
	if !strings.Contains(message, pattern) {
		t.Errorf("step 5's message no longer quotes the id rule it enforced\n\tmessage: %q\n\tpattern: %q",
			message, pattern)
	}
}

// TestTheOtherKeywordsOnTheSameFieldKeepTheLibrarysWording is the second
// over-fire arm: the special case is one keyword on one pointer, not everything
// said about created_at.
//
// "yesterday" fails the "format" keyword, which owns calendar validity — a rule
// a regular expression cannot decide, and one the library states well. A fix
// that keyed on the pointer alone would replace this diagnosis with a sentence
// about spelling, sending the reader to look for a stray offset in a value that
// is not a date at all.
func TestTheOtherKeywordsOnTheSameFieldKeepTheLibrarysWording(t *testing.T) {
	store := storeOf(t, []testRecord{
		decisionAt("DEC-0001.json", decisionDoc("DEC-0001", map[string]any{
			"created_at": "yesterday",
		})),
	})

	findings := findingsAt(validate.Check(store, shippedValidator(t)), validate.StepSchema)
	if len(findings) != 1 {
		t.Fatalf("step 5 reported %d findings, want the one format violation:\n\t%s",
			len(findings), describe(findings))
	}
	message := findings[0].Message
	if !strings.Contains(message, `"format"`) {
		t.Fatalf("this store no longer produces a format violation, so the test guards nothing: %q", message)
	}
	if !strings.Contains(message, "date-time") {
		t.Errorf("step 5 replaced the format diagnosis with something else: %q", message)
	}
	if !strings.Contains(message, "/created_at") {
		t.Errorf("step 5's message does not name the offending pointer: %q", message)
	}
}
