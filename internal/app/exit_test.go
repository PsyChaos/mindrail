package app

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	base := errors.New("boom")

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil error is success", nil, ExitSuccess},
		{"unclassified error is a generic failure", base, ExitFailed},
		{"failed", Failed(base), ExitFailed},
		{"usage", Usage(base), ExitUsage},
		{"denied", Denied(base), ExitDenied},
		{"unavailable", Unavailable(base), ExitUnavailable},
		{"wrapped classification survives", fmt.Errorf("context: %w", Denied(base)), ExitDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestErrorUnwrapsToCause(t *testing.T) {
	cause := errors.New("cause")

	if err := Denied(cause); !errors.Is(err, cause) {
		t.Errorf("errors.Is(Denied(cause), cause) = false, want true")
	}
}

func TestErrorMessageUsesCause(t *testing.T) {
	if got := Failed(errors.New("disk full")).Error(); got != "disk full" {
		t.Errorf("Error() = %q, want %q", got, "disk full")
	}
}
