// Package mcp binds Mindrail's application services to the official MCP Go
// SDK as six tools (MR-014): bootstrap, status, search, context, decide
// and invariant. Handlers are typed In/Out structs — the structs ARE the
// contract (decision D-194) — and every tool calls the services the CLI
// already uses, never a reimplementation. Deferred values refuse loudly
// with NOT_IMPLEMENTED_IN_THIS_VERSION and a next action, never silently.
package mcp

import (
	"github.com/PsyChaos/mindrail/internal/app"
)

// Refusal is the deferred-value answer: the 0.1 surface does not implement
// what was asked, says so with the wire code, and names the next action.
// It rides successful tool output (not a transport error) so agents can
// branch on it like any other value.
type Refusal struct {
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	NextAction []string `json:"next_action"`
}

// NotImplemented refuses a present-but-deferred value. Absent optionals
// never reach here — only explicit asks refuse (decision D-188).
func NotImplemented(what, supported, action string) *Refusal {
	return &Refusal{
		Code:       string(app.CodeNotImplementedInThisVersion),
		Message:    what + " is not implemented in this version; supported: " + supported,
		NextAction: []string{action},
	}
}

// Invalid refuses malformed input as a Go error: the request itself is
// broken, not merely deferred. Diagnostics name parameters, never values
// (spec §19).
func Invalid(why string) error {
	return app.NewError(app.CodeCommandLineInvalid, app.KindUsage, why,
		"No repository facts were changed.", "Correct the tool arguments and retry.")
}
