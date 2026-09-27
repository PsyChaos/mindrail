package validation

import (
	"os"
	"regexp"
	"strings"
)

// Redacted is the replacement for every secret value and pattern match.
// One constant, so a stored row can never leak through a varied mask.
const Redacted = "[REDACTED]"

// Redactor scrubs process output before storage (spec §§77–79): exact
// known-secret values first, then default patterns. Values are read from
// the environment at redaction time and never stored — not in rows, not in
// errors, not in logs. The original output is kept nowhere: redacted text
// is canonical, even on false positives.
type Redactor struct {
	names []string
}

// NewRedactor builds a Redactor over config-named secret env vars. Names
// are validated the way config validation does; values are read per call,
// never retained.
func NewRedactor(names []string) (*Redactor, error) {
	for _, name := range names {
		if !validSecretName(name) {
			return nil, invalidInput("redactor needs valid secret env names")
		}
	}
	return &Redactor{names: names}, nil
}

func validSecretName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

// Redact scrubs one text: exact values, then patterns. Empty values match
// nothing — an empty secret must not redact the world.
func (r *Redactor) Redact(text string) string {
	for _, name := range r.names {
		if value := os.Getenv(name); value != "" {
			text = strings.ReplaceAll(text, value, Redacted)
		}
	}
	for _, pattern := range defaultPatterns {
		text = pattern.ReplaceAllString(text, Redacted)
	}
	return text
}

// defaultPatterns is the fixed 0.1 set (spec §79 categories, decision
// D-161): bearer tokens, private key blocks, common API token formats,
// credentialed database URLs, password-like key/value output. Configurable
// regex is deferred; every category below is pinned by a test.
var defaultPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/=]+`),
	regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9\-]+`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`://[^/\s:]+:[^/\s@]+@`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*[^\s]+`),
}
