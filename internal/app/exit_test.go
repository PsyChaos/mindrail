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

// TestExitCodeFromDomainError covers decision D-03: a DomainError classifies
// itself, and it does so even when a process-level carrier is wrapped around
// it, because the domain knows more about the failure than the caller does.
func TestExitCodeFromDomainError(t *testing.T) {
	domain := func(code Code, kind Kind) *DomainError {
		return NewError(code, kind, "why", "impact", "next")
	}

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not a git repository is a usage error", domain(CodeNotAGitRepository, KindUsage), ExitUsage},
		{"bare repository is a usage error", domain(CodeBareRepository, KindUsage), ExitUsage},
		{"invalid config is a usage error", domain(CodeConfigInvalid, KindUsage), ExitUsage},
		{"unwritable runtime path is unavailable", domain(CodeRuntimePathUnwritable, KindUnavailable), ExitUnavailable},
		{"unopenable database is unavailable", domain(CodeRuntimeDBUnavailable, KindUnavailable), ExitUnavailable},
		{"corrupt database is unavailable", domain(CodeRuntimeDBCorrupt, KindUnavailable), ExitUnavailable},
		{"git timeout is unavailable", domain(CodeGitTimeout, KindUnavailable), ExitUnavailable},
		{"migration failure is an operation failure", domain(CodeMigrationFailed, KindFailed), ExitFailed},
		{"schema ahead is an operation failure", domain(CodeRuntimeDBSchemaTooNew, KindFailed), ExitFailed},
		{"denied classification is honoured", domain(CodeKnowledgeSchemaUnsupported, KindDenied), ExitDenied},
		{
			name: "domain classification survives wrapping",
			err:  fmt.Errorf("bootstrap: %w", fmt.Errorf("git: %w", error(domain(CodeBareRepository, KindUsage)))),
			want: ExitUsage,
		},
		{
			name: "domain classification wins over the process carrier",
			err:  Failed(domain(CodeNotAGitRepository, KindUsage)),
			want: ExitUsage,
		},
		{
			name: "a domain error with an unset kind is a generic failure",
			err:  &DomainError{Code: CodeWorkspaceRegistrationFailed},
			want: ExitFailed,
		},
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
