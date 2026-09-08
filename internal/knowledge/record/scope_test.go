package record_test

import (
	"errors"
	"testing"

	"github.com/PsyChaos/mindrail/internal/knowledge/record"
)

// TestTheDriveLetterBoundaryIsPinnedFromBothSides is finding B-08's regression
// test and its own over-fire guard in one table.
//
// The defect it holds shut: checkRepoRelative refused every target whose second
// byte was a colon and whose first was an ASCII letter, so "a:b/c.go" — a legal
// POSIX repository-relative path, one character away from the plainly legal
// "ab/c.go" — was reported as "an absolute machine path". A record carrying it
// took the store to DEGRADED / KNOWLEDGE_INVALID.
//
// Both directions are pinned deliberately. Widening the rule until "C:/x" is
// accepted would be the same defect written backwards, and this project has now
// produced that pair twice; narrowing it again until "a:b/c.go" is refused is
// where it started. Every row is written as a literal rather than computed from
// the rule under test, so the table cannot agree with a wrong implementation by
// sharing its arithmetic.
func TestTheDriveLetterBoundaryIsPinnedFromBothSides(t *testing.T) {
	t.Parallel()

	const (
		accept = true
		reject = false
	)

	tests := []struct {
		name   string
		target string
		want   bool
		why    string
	}{
		// The finding's control and scenario, one character apart.
		{"the control the finding measured", "ab/c.go", accept,
			"an ordinary repository-relative path"},
		{"the finding itself: one colon inserted", "a:b/c.go", accept,
			"a legal POSIX path; refusing it was B-08"},

		// The rule that must survive: a rooted drive letter is a machine path.
		{"the drive-rooted form the rule exists for", "C:/x", reject,
			"absolute on Windows, and names a machine root"},
		{"a lowercase drive is still a drive", "c:/x", reject, ""},
		{"the first letter of the range", "A:/x", reject, ""},
		{"the last letter of the range", "Z:/x", reject, ""},
		{"the last lowercase letter of the range", "z:/x", reject, ""},
		{"a rooted drive with a doubled slash", "C://x", reject, ""},
		{"a rooted drive naming the root itself", "C:/", reject, ""},

		// Drive-relative spellings. These are the same shape as "a:b/c.go" and
		// cannot be told apart from it by any byte, so they follow it.
		{"a bare volume name", "C:", accept,
			"drive-relative, not drive-rooted; the same shape as a:b/c.go"},
		{"a drive-relative name", "C:x", accept,
			"identical in shape to a:b/c.go with a different letter"},
		{"a colon-named file", "a:b", accept, ""},

		// Over-fire guards on the letter class. None of these was ever a drive.
		{"a digit before the colon", "1:b/c.go", accept, ""},
		{"a rooted digit is not a drive", "9:/x", accept, ""},
		{"two letters before the colon", "AB:/x", accept, ""},
		{"the byte just past 'Z' in ASCII", "[:/x", accept, ""},
		{"a colon in a later segment", "internal/testdata/fixture:1.json", accept, ""},

		// The other three refusals checkRepoRelative owns, so that a change to
		// the drive rule cannot quietly take one of them with it.
		{"backslashes are refused before the drive rule is asked", `C:\x`, reject,
			"caught by the backslash branch, not the drive branch"},
		{"a backslash path with no drive at all", `internal\app.go`, reject, ""},
		{"an absolute POSIX path", "/abs", reject, ""},
		{"a path leaving the repository", "../up", reject, ""},
		{"an unnormalized path", "./internal//app.go", reject, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := record.Scope{Level: record.ScopeFile, Target: tt.target}.Validate()

			if tt.want == accept && err != nil {
				t.Fatalf("Scope{FILE, %q}.Validate() = %v, want nil — %s", tt.target, err, tt.why)
			}
			if tt.want == reject {
				if err == nil {
					t.Fatalf("Scope{FILE, %q}.Validate() = nil, want a refusal — %s", tt.target, tt.why)
				}
				if !errors.Is(err, record.ErrScopeTargetNotRepoRelative) {
					t.Fatalf("Scope{FILE, %q}.Validate() = %v, want one matching ErrScopeTargetNotRepoRelative",
						tt.target, err)
				}
			}
		})
	}
}

// TestTheDriveRuleIsAskedAtEveryLevelThatCarriesATarget stops the B-08 fix from
// being correct only at FILE level.
//
// Scope.Validate short-circuits PROJECT before it reaches the target at all, so
// the four remaining levels are the ones the rule governs. A fix applied inside
// one level's branch would pass the table above and still leave the other three
// refusing "a:b/c.go".
func TestTheDriveRuleIsAskedAtEveryLevelThatCarriesATarget(t *testing.T) {
	t.Parallel()

	for _, level := range []record.ScopeLevel{
		record.ScopePackage, record.ScopeModule, record.ScopeFile, record.ScopeSymbol,
	} {
		t.Run(string(level), func(t *testing.T) {
			t.Parallel()

			if err := (record.Scope{Level: level, Target: "a:b/c.go"}).Validate(); err != nil {
				t.Errorf("Scope{%s, \"a:b/c.go\"}.Validate() = %v, want nil", level, err)
			}
			if err := (record.Scope{Level: level, Target: "C:/x"}).Validate(); err == nil {
				t.Errorf("Scope{%s, \"C:/x\"}.Validate() = nil, want a refusal", level)
			}
		})
	}
}

// TestAConstructorAgreesWithScopeValidateAboutTheDriveBoundary carries B-08's
// fix across the writer boundary.
//
// Scope.Validate is one of two readers of this rule: the constructors call
// checkScope, which calls it, and internal/knowledge/validate's step 11 calls it
// directly. A record whose scope Validate accepts but whose constructor refuses
// would be the same writer-validator split B-05 is about, in a second place.
func TestAConstructorAgreesWithScopeValidateAboutTheDriveBoundary(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"a:b/c.go", "C:", "C:x"} {
		if _, err := record.NewDecision("DEC-0001", createdAt, "t", "d",
			record.WithScope(record.Scope{Level: record.ScopeFile, Target: target})); err != nil {
			t.Errorf("NewDecision() with scope target %q: error = %v, want nil", target, err)
		}
	}
	if _, err := record.NewDecision("DEC-0001", createdAt, "t", "d",
		record.WithScope(record.Scope{Level: record.ScopeFile, Target: "C:/x"})); !errors.Is(err, record.ErrScopeTargetNotRepoRelative) {
		t.Errorf("NewDecision() with scope target \"C:/x\": error = %v, want ErrScopeTargetNotRepoRelative", err)
	}
}
