package app

import (
	"strings"
	"testing"
	"time"
)

// TestFormatTimeIsUTCRFC3339 pins tech-stack §42: every persisted timestamp is
// UTC RFC 3339, whatever zone the machine that produced it happens to be in.
func TestFormatTimeIsUTCRFC3339(t *testing.T) {
	kathmandu := time.FixedZone("UTC+5:45", 5*3600+45*60)

	tests := []struct {
		name    string
		instant time.Time
		want    string
	}{
		{
			name:    "non-UTC zone is converted",
			instant: time.Date(2026, 9, 5, 18, 45, 0, 0, kathmandu),
			want:    "2026-09-05T13:00:00Z",
		},
		{
			name:    "already UTC is unchanged",
			instant: time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC),
			want:    "2026-09-05T13:00:00Z",
		},
		{
			name:    "nanoseconds survive",
			instant: time.Date(2026, 9, 5, 13, 0, 0, 123456789, time.UTC),
			want:    "2026-09-05T13:00:00.123456789Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := FixedClock{Instant: tt.instant}

			now := clock.Now()
			if now.Location() != time.UTC {
				t.Errorf("FixedClock.Now() is in %v, want UTC", now.Location())
			}

			got := FormatTime(now)
			if got != tt.want {
				t.Errorf("FormatTime = %q, want %q", got, tt.want)
			}
			if !strings.HasSuffix(got, "Z") {
				t.Errorf("FormatTime = %q, want a Z offset", got)
			}

			parsed, err := ParseTime(got)
			if err != nil {
				t.Fatalf("ParseTime(%q): %v", got, err)
			}
			if !parsed.Equal(tt.instant) {
				t.Errorf("ParseTime round-trip = %v, want %v", parsed, tt.instant)
			}
			if parsed.Location() != time.UTC {
				t.Errorf("ParseTime returned %v, want UTC", parsed.Location())
			}
			if again := FormatTime(parsed); again != got {
				t.Errorf("second FormatTime = %q, want %q", again, got)
			}
		})
	}
}

func TestParseTimeRejectsMalformedInput(t *testing.T) {
	for _, in := range []string{"", "not a time", "2026-09-05", "2026-09-05 13:00:00"} {
		t.Run(in, func(t *testing.T) {
			if _, err := ParseTime(in); err == nil {
				t.Errorf("ParseTime(%q) = nil error, want a failure", in)
			}
		})
	}
}

// TestSystemClockIsUTC keeps the production clock honest; determinism is the
// FixedClock's job, so only the zone is asserted here.
func TestSystemClockIsUTC(t *testing.T) {
	var clock Clock = SystemClock{}

	now := clock.Now()
	if now.Location() != time.UTC {
		t.Errorf("SystemClock.Now() is in %v, want UTC", now.Location())
	}
	if now.IsZero() {
		t.Errorf("SystemClock.Now() = zero time")
	}
}

// TestShutdownTimeoutMatchesBusyTimeout: decision D-08 ties the bounded
// shutdown deadline to the SQLite busy_timeout of 5000 ms.
func TestShutdownTimeoutMatchesBusyTimeout(t *testing.T) {
	if ShutdownTimeout != 5*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 5s", ShutdownTimeout)
	}
}

// TestFixedClockSatisfiesClock is a compile-time guarantee that both clocks are
// substitutable wherever a seam takes a Clock.
func TestFixedClockSatisfiesClock(t *testing.T) {
	var clocks = []Clock{SystemClock{}, FixedClock{Instant: time.Unix(0, 0)}}
	for _, c := range clocks {
		if c.Now().Location() != time.UTC {
			t.Errorf("%T.Now() is not UTC", c)
		}
	}
}
