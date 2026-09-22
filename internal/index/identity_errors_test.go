package index_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/index"
)

// TestIdentityCodesCarryDistinctRemedies is TASK-01 AC-01.3: the two codes
// enter allCodes with a remedy each, and no two share one.
func TestIdentityCodesCarryDistinctRemedies(t *testing.T) {
	errs := []error{
		index.AmbiguousIdentity("old-key", []string{"new-a", "new-b"}),
		index.OrphanedProtectedSymbol("INV-0001", "a.py:gone"),
	}
	seen := map[string]app.Code{}
	for _, err := range errs {
		payload, ok := app.PayloadOf(err)
		if !ok {
			t.Fatalf("error %v carries no payload", err)
		}
		if !app.IsRegistered(payload.Code) {
			t.Fatalf("code %q is not registered", payload.Code)
		}
		if len(payload.NextAction) == 0 {
			t.Fatalf("code %q carries no remedy", payload.Code)
		}
		for _, remedy := range payload.NextAction {
			if owner, ok := seen[remedy]; ok {
				t.Fatalf("remedy %q shared by %q and %q", remedy, owner, payload.Code)
			}
			seen[remedy] = payload.Code
		}
	}
}
