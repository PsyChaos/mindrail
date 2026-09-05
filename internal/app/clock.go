package app

import (
	"fmt"
	"time"
)

// Clock is the time seam of tech-stack §42. Anything that persists or renders a
// timestamp takes one, so tests can assert on an exact string instead of a
// tolerance window.
type Clock interface{ Now() time.Time }

// SystemClock is the production clock. It returns UTC at the source rather than
// converting later, so no caller can persist a local-zone timestamp by
// forgetting a .UTC().
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

// FixedClock is the deterministic clock used by tests and by golden fixtures.
type FixedClock struct{ Instant time.Time }

func (c FixedClock) Now() time.Time { return c.Instant.UTC() }

// FormatTime renders the one timestamp format Mindrail persists: RFC 3339 with
// nanosecond precision, forced to UTC so the Z offset is unconditional.
func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// ParseTime reads a timestamp written by FormatTime and returns it in UTC.
// It is strict: a value that is not RFC 3339 is a corrupt record, not a value
// to guess at.
func ParseTime(s string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", s, err)
	}
	return parsed.UTC(), nil
}

// ShutdownTimeout bounds the §88 shutdown sequence. It matches the SQLite
// busy_timeout of 5000 ms (decision D-08): a shutdown that gave up sooner than
// the contention window could abandon a write that was about to succeed.
const ShutdownTimeout = 5 * time.Second
