package cli_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/cli"
)

// TestCommandSurfaceUnchangedIn01 pins the 0.1 CLI surface: no milestone may
// add a top-level command without deliberation. Subcommands evolve within
// their owner's scope; a new top-level verb is a product decision, and this
// test makes it a loud one.
func TestCommandSurfaceUnchangedIn01(t *testing.T) {
	want := []string{"checkpoint", "doctor", "init", "lease", "session", "status", "task", "version"}
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
