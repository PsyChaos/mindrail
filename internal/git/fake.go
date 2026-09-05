package git

import (
	"context"
	"slices"
	"strings"
	"sync"
)

// Invocation is one recorded call, kept so a test can assert on the exact
// argument vector rather than on the output it happened to produce.
type Invocation struct {
	Dir  string
	Args []string
}

// FakeResponse is what a scripted invocation returns.
type FakeResponse struct {
	Stdout string
	Stderr string
	Err    error
}

// FakeRunner is the shared test double for CommandRunner.
//
// It lives in the non-test build on purpose: the doctor, bootstrap and CLI
// units all need to drive the git adapter deterministically from their own
// packages, and a double hidden in this package's _test.go files would be
// invisible to them (design §4, Unit A).
//
// Responses is keyed by strings.Join(args, " "); an argv with no entry falls
// back to Default. The zero value is usable and answers everything with an
// empty successful response.
//
// Use it through a pointer — Run has a pointer receiver, so that is the only
// form that satisfies CommandRunner anyway.
type FakeRunner struct {
	Responses map[string]FakeResponse
	Default   FakeResponse

	// Calls records every invocation in order. Read it after the code under
	// test has finished.
	Calls []Invocation

	mu sync.Mutex
}

// Run records the invocation and replays the scripted response. It ignores ctx
// deliberately: a double that could fail for reasons the test did not set up
// would make failures ambiguous.
func (f *FakeRunner) Run(_ context.Context, dir string, args ...string) ([]byte, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// The caller owns args and may reuse the backing array; a recording that
	// changes after the fact is worse than no recording.
	f.Calls = append(f.Calls, Invocation{Dir: dir, Args: slices.Clone(args)})

	response, ok := f.Responses[strings.Join(args, " ")]
	if !ok {
		response = f.Default
	}
	return []byte(response.Stdout), []byte(response.Stderr), response.Err
}
