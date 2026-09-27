package identity_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/PsyChaos/mindrail/internal/identity"
)

// idShape is the whole published format: a caller-supplied prefix, a hyphen, and
// 26 Crockford base32 characters. The class is spelled out rather than written
// as [0-9A-Z] because the four letters Crockford drops — I, L, O and U — are the
// point of choosing that alphabet, and a pattern that accepted them would pass
// on an id nobody can read aloud.
var idShape = regexp.MustCompile(`^(PRJ|WS|SES|TSK|CKP)-[0-9A-HJKMNP-TV-Z]{26}$`)

// TestAnIdIsPrefixedUniqueAndSortsInMintOrder is decision D-26's format half,
// moved here from internal/workspace by D-57.
//
// Creation order must equal lexicographic order. The ids are used as tie
// breakers wherever a fixed or coarse clock gives several rows one timestamp, so
// an id that sorted by its random part would silently reorder those rows — and
// 500 consecutive mints is enough to land many of them in one millisecond, which
// is the case the entropy counter exists for.
func TestAnIdIsPrefixedUniqueAndSortsInMintOrder(t *testing.T) {
	const total = 500

	ids := make([]string, 0, total)
	seen := make(map[string]struct{}, total)
	for range total {
		id := identity.NewID("WS")
		if !idShape.MatchString(id) {
			t.Fatalf("NewID(%q) = %q, want prefix + '-' + 26 Crockford base32 characters", "WS", id)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("NewID produced %q twice", id)
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if !slices.IsSorted(ids) {
		for i := 1; i < len(ids); i++ {
			if ids[i] <= ids[i-1] {
				t.Fatalf("id %d (%q) does not sort after id %d (%q)", i, ids[i], i-1, ids[i-1])
			}
		}
	}
}

// TestEveryPrefixThisModuleMintsKeepsItsSpelling is the guard the extraction
// needs.
//
// The minter no longer lives beside the packages that name things, so nothing
// else would notice a change that dropped the hyphen or shortened the body.
// Every prefix in the module is minted here, including the three MR-003 adds, so
// a format change is a red test rather than a migration nobody planned.
func TestEveryPrefixThisModuleMintsKeepsItsSpelling(t *testing.T) {
	for _, prefix := range []string{"PRJ", "WS", "SES", "TSK", "CKP"} {
		id := identity.NewID(prefix)
		if !strings.HasPrefix(id, prefix+"-") {
			t.Errorf("NewID(%q) = %q, want the %s- prefix", prefix, id, prefix)
		}
		if !idShape.MatchString(id) {
			t.Errorf("NewID(%q) = %q, which is not the published shape", prefix, id)
		}
	}
}

// TestAnIdLeaksNeitherTheClockNorTheCaller is the opacity half of D-26 at the
// format layer.
//
// The timestamp is in there — it has to be, for the sort order — but as 48 bits
// of a base32 body, not as digits a reader could lift out. An id that spelled
// its millisecond in decimal would still sort correctly and would still pass
// every other test in this file.
func TestAnIdLeaksNeitherTheClockNorTheCaller(t *testing.T) {
	id := identity.NewID("TSK")
	body := strings.TrimPrefix(id, "TSK-")

	// Every character comes from the alphabet, so no separator, path fragment or
	// punctuation from the caller's world can have reached it.
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	for _, r := range body {
		if !strings.ContainsRune(alphabet, r) {
			t.Errorf("id %q contains %q, which is not in Crockford's alphabet", id, r)
		}
	}
}
