package validation_test

import (
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/validation"
)

// TestRedactExactValues is TASK-02 AC-02.1: config-named env values become
// [REDACTED] wherever they appear; empty values match nothing; names
// validate like config.
func TestRedactExactValues(t *testing.T) {
	t.Setenv("MR010_RED_TEST", "s3cr3t-exact-value")
	redactor, err := validation.NewRedactor([]string{"MR010_RED_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	got := redactor.Redact("token=s3cr3t-exact-value done")
	if got != "token=[REDACTED] done" {
		t.Fatalf("redacted = %q", got)
	}
	unset, err := validation.NewRedactor([]string{"MR010_RED_DEFINITELY_UNSET"})
	if err != nil {
		t.Fatal(err)
	}
	if got := unset.Redact("abc"); got != "abc" {
		t.Fatalf("unset secret redacted the world: %q", got)
	}
	if _, err := validation.NewRedactor([]string{"HAS SPACE"}); err == nil {
		t.Fatal("invalid secret name accepted")
	}
	empty, err := validation.NewRedactor(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := empty.Redact("untouched"); got != "untouched" {
		t.Fatalf("empty redactor changed text: %q", got)
	}
}

// TestRedactDefaultPatterns is TASK-02 AC-02.2: each fixed category scrubs
// without the original surviving anywhere in the output.
func TestRedactDefaultPatterns(t *testing.T) {
	redactor, err := validation.NewRedactor(nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"authorization: Bearer abcDEF123._-~":                                 "authorization: [REDACTED]",
		"key ghp_abcdef123456":                                                "key [REDACTED]",
		"aws AKIAIOSFODNN7EXAMPLE here":                                       "aws [REDACTED] here",
		"db postgres://u:p4ss@host/x ok":                                      "db postgres[REDACTED]host/x ok",
		"password: hunter2 done":                                              "[REDACTED] done",
		"-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----": "[REDACTED]",
	}
	for input, want := range cases {
		if got := redactor.Redact(input); got != want {
			t.Errorf("Redact(%q) = %q, want %q", input, got, want)
		}
	}
	if got := redactor.Redact("nothing secret here"); got != "nothing secret here" {
		t.Errorf("clean text changed: %q", got)
	}
	if !strings.Contains(validation.Redacted, "REDACTED") {
		t.Fatal("redaction marker lost its shape")
	}
}
