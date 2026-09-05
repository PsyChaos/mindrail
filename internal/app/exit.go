// Package app owns process-level concerns shared by every entry point:
// exit code mapping, root context lifecycle and application wiring.
package app

import (
	"errors"
	"fmt"
)

// Exit codes follow the small convention in tech-stack §13. Fine-grained
// error information belongs in structured output, not in new exit codes.
const (
	ExitSuccess     = 0 // success
	ExitFailed      = 1 // operation failed
	ExitUsage       = 2 // invalid usage or configuration
	ExitDenied      = 3 // verification denied
	ExitUnavailable = 4 // temporary or runtime unavailable
)

// Kind classifies an error into one of the exit code categories.
type Kind int

const (
	KindFailed Kind = iota
	KindUsage
	KindDenied
	KindUnavailable
)

// Error is the process-level error carrier. Commands wrap their failures in it
// so that main can map them to an exit code without inspecting domain types.
type Error struct {
	Kind Kind
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("app error (kind %d)", e.Kind)
	}
	return e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// Failed reports an operation that ran but did not succeed.
func Failed(err error) error { return &Error{Kind: KindFailed, Err: err} }

// Usage reports invalid usage or invalid configuration.
func Usage(err error) error { return &Error{Kind: KindUsage, Err: err} }

// Denied reports a verification decision that refused to allow completion.
func Denied(err error) error { return &Error{Kind: KindDenied, Err: err} }

// Unavailable reports a temporary or runtime-unavailable condition.
func Unavailable(err error) error { return &Error{Kind: KindUnavailable, Err: err} }

// ExitCode maps an error to a process exit code. A nil error is success; an
// error carrying no explicit kind is a generic operation failure.
//
// A *DomainError is consulted before the *Error carrier: the layer that
// detected the failure knows its class, while a caller that merely wrapped it
// in Failed() was guessing (decision D-03).
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}

	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		return exitCodeForKind(domainErr.Kind)
	}

	var appErr *Error
	if errors.As(err, &appErr) {
		return exitCodeForKind(appErr.Kind)
	}

	return ExitFailed
}

func exitCodeForKind(kind Kind) int {
	switch kind {
	case KindUsage:
		return ExitUsage
	case KindDenied:
		return ExitDenied
	case KindUnavailable:
		return ExitUnavailable
	default:
		return ExitFailed
	}
}
