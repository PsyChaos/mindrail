package cli_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
)

// TestVersionStampVocabulary pins the release-stamping contract (TASK-03
// AC-03.1): build_date and dirty ride every version report, and
// unstamped local builds answer "unknown" rather than claiming a stamp
// they do not have. Linker-stamped values are proven by the release run
// itself (findings), not here — this test pins the shape and the honest
// defaults.
func TestVersionStampVocabulary(t *testing.T) {
	repo := newInitializedRepo(t)
	got := run(t, repo, "version", "--json")
	got.requireExit(t, app.ExitSuccess)
	var data struct {
		BuildDate string `json:"build_date"`
		Dirty     string `json:"dirty"`
	}
	decodeData(t, got.stdout, &data)
	if data.BuildDate == "" {
		t.Fatal("build_date missing from version report")
	}
	// Unstamped test binaries carry no linker flags: the only honest
	// answer is the explicit unknown, never a claimed state.
	if data.Dirty != "unknown" {
		t.Fatalf("dirty = %q, want unknown without linker stamps", data.Dirty)
	}
}
