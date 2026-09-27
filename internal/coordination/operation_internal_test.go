package coordination

import "testing"

// TestTheRequestHashIsStableAndSensitive is AC-06.6: the same command and
// parameters hash the same wherever they are computed — the hash is what a
// second process's delivery is matched on — and changing any one parameter
// changes it, so a "retry" that differs in one field is a different request.
func TestTheRequestHashIsStableAndSensitive(t *testing.T) {
	type params struct {
		Task   string `json:"task"`
		By     string `json:"by"`
		To     State  `json:"to"`
		Reason string `json:"reason"`
		Expect int64  `json:"expect_revision"`
	}
	base := params{"TSK-1", "named:SES-1", StateClaimed, "", 0}

	first, err := requestHash("task state", base)
	if err != nil {
		t.Fatalf("requestHash = %v", err)
	}
	second, err := requestHash("task state", params{"TSK-1", "named:SES-1", StateClaimed, "", 0})
	if err != nil {
		t.Fatalf("requestHash = %v", err)
	}
	if first != second || len(first) != 64 {
		t.Errorf("the same request hashed to %q and %q; want one 64-character hex value", first, second)
	}

	variants := map[string]params{
		"task":     {"TSK-2", "named:SES-1", StateClaimed, "", 0},
		"session":  {"TSK-1", "named:SES-2", StateClaimed, "", 0},
		"minted":   {"TSK-1", "mint:WSP-1", StateClaimed, "", 0},
		"to":       {"TSK-1", "named:SES-1", StateInProgress, "", 0},
		"reason":   {"TSK-1", "named:SES-1", StateClaimed, "why", 0},
		"expected": {"TSK-1", "named:SES-1", StateClaimed, "", 1},
	}
	seen := map[string]string{first: "base"}
	for name, variant := range variants {
		hash, err := requestHash("task state", variant)
		if err != nil {
			t.Fatalf("requestHash(%s) = %v", name, err)
		}
		if other, clash := seen[hash]; clash {
			t.Errorf("changing %s produced the same hash as %s", name, other)
		}
		seen[hash] = name
	}
	if other, err := requestHash("task open", base); err != nil || other == first {
		t.Errorf("the command name is not part of the hash: %q vs %q (%v)", other, first, err)
	}

	// The attribution key tells a named session from a minted one and one
	// workspace from another.
	if attributionKey(NamedSession("SES-1")) == attributionKey(MintFor("SES-1")) {
		t.Error("a named session and a workspace with the same id key the same")
	}
	if attributionKey(MintFor("WSP-1")) == attributionKey(MintFor("WSP-2")) {
		t.Error("two workspaces key the same")
	}
}
