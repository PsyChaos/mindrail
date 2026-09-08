package record

import (
	"fmt"
	"strings"
)

// ScopeLevel names how wide a record's claim is (spec §54).
type ScopeLevel string

const (
	ScopeProject ScopeLevel = "PROJECT"
	ScopePackage ScopeLevel = "PACKAGE"
	ScopeModule  ScopeLevel = "MODULE"
	ScopeFile    ScopeLevel = "FILE"
	ScopeSymbol  ScopeLevel = "SYMBOL"
)

// scopeLevels is every level the schema documents admit, in the order their
// enum lists them so that an error naming the alternatives reads the same way
// on every run.
var scopeLevels = []ScopeLevel{ScopeProject, ScopePackage, ScopeModule, ScopeFile, ScopeSymbol}

// Scope is where a record applies.
//
// Target is repository-relative with forward slashes (tech-stack §74), or a
// symbol reference at SYMBOL level. It is absent for PROJECT scope, which is
// what the ",omitempty" carries: the schema's $defs/scope requires only
// "level", so a Go struct that always emitted "target" would write a document
// the schema calls valid but the description calls wrong.
type Scope struct {
	Level  ScopeLevel `json:"level"`
	Target string     `json:"target,omitempty"`
}

// Validate reports the scope's syntax, which is exactly the question spec §95
// step 11 asks: is the level one of the five, and is the target present and
// spelled as a repository-relative forward-slash path.
//
// It answers syntax only. Whether the target names something that exists needs
// MR-005's structural index and is step 12 (design §6), so a target naming a
// file that is not there is not an error here.
//
// It is exported so that internal/knowledge/validate's step 11 can ask this
// package rather than write a second implementation of the same rule — two
// implementations of one rule is the drift decision D-36 exists to prevent, and
// a writer and a validator disagreeing about what a scope is would be the
// defect design §5's pairing looks for. The three failures are distinct
// sentinels so a caller can tell them apart without reading prose.
//
// Deliberately absent: a PROJECT scope carrying a target is *not* refused here,
// because AC-03.4 does not list it among step 11's conditions and a reader must
// not invent a finding the frozen pipeline does not own. The constructors are
// stricter than this on their own, writer-side account — see checkScope.
func (s Scope) Validate() error {
	if !isScopeLevel(s.Level) {
		return fmt.Errorf("%w: %q is not one of %v", ErrScopeLevelUnknown, s.Level, scopeLevels)
	}
	if s.Level == ScopeProject {
		// A project-wide claim needs no target to point at, so an absent one is
		// the normal case rather than a missing value.
		return nil
	}
	if s.Target == "" {
		return fmt.Errorf("%w: %s scope names nothing", ErrScopeTargetMissing, s.Level)
	}
	if err := checkRepoRelative(s.Target); err != nil {
		return err
	}
	return nil
}

func isScopeLevel(level ScopeLevel) bool {
	for _, candidate := range scopeLevels {
		if level == candidate {
			return true
		}
	}
	return false
}

// checkRepoRelative holds a target to the one spelling Mindrail stores or
// prints (tech-stack §74): repository-relative, normalized, forward slashes.
//
// The rule is stated over segments rather than through path.Clean because
// Clean leaves a leading ".." intact — "../secrets" survives it unchanged and
// would pass a Clean-based check while naming something outside the repository
// entirely. Scanning segments refuses that, an absolute "/x", an empty segment
// from "a//b" or a trailing slash, and a "." or ".." anywhere, in one pass.
func checkRepoRelative(target string) error {
	if strings.Contains(target, `\`) {
		return fmt.Errorf("%w: %q is spelled with backslashes; repository paths use '/'", ErrScopeTargetNotRepoRelative, target)
	}
	if isWindowsDriveRooted(target) {
		return fmt.Errorf("%w: %q is an absolute machine path, not a repository-relative one", ErrScopeTargetNotRepoRelative, target)
	}
	for _, segment := range strings.Split(target, "/") {
		switch segment {
		case "":
			return fmt.Errorf("%w: %q has an empty path segment, so it is absolute or unnormalized", ErrScopeTargetNotRepoRelative, target)
		case ".", "..":
			return fmt.Errorf("%w: %q contains a %q segment, so it is unnormalized or leaves the repository", ErrScopeTargetNotRepoRelative, target, segment)
		}
	}
	return nil
}

// isWindowsDriveRooted recognises "C:/x", the one absolute spelling that
// survives the backslash refusal above. "C:\x" is already caught, but a caller
// that helpfully converted the separators would otherwise smuggle a
// machine-local root into repository content.
func isWindowsDriveRooted(target string) bool {
	if len(target) < 2 || target[1] != ':' {
		return false
	}
	drive := target[0]
	return (drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')
}

// clone copies an optional scope so that a record and the caller that supplied
// its scope never share one. Returning the same pointer would let a later edit
// to the caller's Scope rewrite a record that was already built, which is the
// mutation this codebase's immutability rule exists to prevent.
func (s *Scope) clone() *Scope {
	if s == nil {
		return nil
	}
	copied := *s
	return &copied
}
