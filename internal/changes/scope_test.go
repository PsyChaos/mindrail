package changes_test

import (
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/changes"
)

// TestFindingTypesCarryCodesRemediesAndBlocking is TASK-01 AC-01.3 at the
// finding level: the three codes enter allCodes with a remedy each, every
// finding blocks, and no two share a remedy.
func TestFindingTypesCarryCodesRemediesAndBlocking(t *testing.T) {
	findings := []changes.Finding{
		changes.ScopeDrift("TSK-1", "CHG-1", "/r/a.py", "modified", "outside scope", []string{"/r/b.py"}),
		changes.UnregisteredChange("TSK-1", "CHG-1", "k", "reconcile", nil),
		changes.AmbiguousAttribution("TSK-1", "CHG-1", "k", []string{"CHG-1", "CHG-2"}, nil),
	}
	seen := map[string]app.Code{}
	for _, finding := range findings {
		if !app.IsRegistered(finding.Code) {
			t.Fatalf("code %q is not registered", finding.Code)
		}
		if !finding.Blocking {
			t.Fatalf("code %q does not block", finding.Code)
		}
		if len(finding.NextAction) == 0 {
			t.Fatalf("code %q carries no remedy", finding.Code)
		}
		if finding.Detail == "" {
			t.Fatalf("code %q carries no detail", finding.Code)
		}
		for _, remedy := range finding.NextAction {
			if owner, ok := seen[remedy]; ok {
				t.Fatalf("remedy %q shared by %q and %q", remedy, owner, finding.Code)
			}
			seen[remedy] = finding.Code
		}
	}
}
