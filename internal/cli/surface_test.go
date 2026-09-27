package cli_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/cli"
)

// TestCommandSurface pins the public CLI surface: no milestone may
// add a top-level command without deliberation. Subcommands evolve within
// their owner's scope; a new top-level verb is a product decision, and this
// test makes it a loud one. MR-017 deliberately adds verify, knowledge and
// hook; the 0.2 line adds update and client-neutral JEV setup. The list below
// is the deliberation record, not an accident.
func TestCommandSurface(t *testing.T) {
	want := []string{"agent", "checkpoint", "doctor", "hook", "init", "jev", "knowledge", "lease", "mcp", "session", "status", "task", "update", "verify", "version"}
	var got []string
	for _, cmd := range cli.NewRoot().Commands() {
		got = append(got, cmd.Name())
	}
	if len(got) != len(want) {
		t.Fatalf("top-level commands = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("top-level commands = %v, want %v", got, want)
		}
	}
}
