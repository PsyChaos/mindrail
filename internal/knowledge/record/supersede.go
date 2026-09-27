package record

import (
	"fmt"
	"slices"
)

// Supersede returns the pair the caller must write: the replacement carrying
// prior.ID in its "supersedes", and the prior with its status moved to
// superseded (spec §93). Records are immutable, so the older Decision stays in
// the repository and the lineage survives rather than being overwritten.
//
// The return order is (replacement, prior) — the reverse of the parameter
// order. That is the order the frozen contract enumerates them in (design §3's
// comment, and AC-01.4), and it is worth naming as a trap: both returns are
// Decision, so a call site that swaps them still compiles and would write each
// record over the other's file. Bind them by name.
//
// It writes nothing and opens nothing. Two files have to land together for the
// lineage to stay consistent, and what to do about a half-written pair is a
// transaction question this package has no filesystem to answer. The claim is
// checkable rather than merely stated: the package imports nothing that could
// reach a filesystem, and a test asserts the whole import set.
func Supersede(prior Decision, replacement Decision) (Decision, Decision, error) {
	// The ids are checked before they are compared so that two unnamed records
	// are reported as unnamed. Without this, a pair of zero Decisions would
	// trip the self-supersede refusal below and tell the caller their record
	// supersedes itself, which is a diagnosis of a mistake they did not make.
	if !decisionIDPattern.MatchString(prior.ID) {
		return Decision{}, Decision{}, fmt.Errorf("%w: prior %q, want %s", ErrInvalidID, prior.ID, decisionIDPattern)
	}
	if !decisionIDPattern.MatchString(replacement.ID) {
		return Decision{}, Decision{}, fmt.Errorf("%w: replacement %q, want %s", ErrInvalidID, replacement.ID, decisionIDPattern)
	}
	if prior.ID == replacement.ID {
		return Decision{}, Decision{}, fmt.Errorf("%w: %s", ErrSupersedesSelf, prior.ID)
	}

	// Order matters: an already-superseded prior is the specific condition and
	// has to be recognised before the general "not active" one, or the more
	// useful diagnosis would never be reached.
	if prior.Status == StatusSuperseded {
		return Decision{}, Decision{}, fmt.Errorf("%w: %s", ErrPriorAlreadySuperseded, prior.ID)
	}
	if prior.Status != StatusActive {
		return Decision{}, Decision{}, fmt.Errorf("%w: %s carries status %q", ErrPriorNotActive, prior.ID, prior.Status)
	}

	// Cloned before the append so the caller's slice is never the one written
	// into: append would otherwise reuse the caller's backing array whenever it
	// had spare capacity, and the replacement they still hold would grow a
	// member they did not add.
	supersedes := slices.Clone(replacement.Supersedes)
	seen := make(map[string]struct{}, len(supersedes)+1)
	for _, id := range supersedes {
		if _, duplicate := seen[id]; duplicate {
			// This function owns the array it appends to, so it refuses to hand
			// back one that breaks the schema's uniqueItems — a record built by
			// NewDecision cannot carry a duplicate, and one that does was
			// assembled by hand.
			return Decision{}, Decision{}, &DuplicateItemError{Property: "supersedes", Value: id}
		}
		seen[id] = struct{}{}
	}
	if _, already := seen[prior.ID]; !already {
		supersedes = append(supersedes, prior.ID)
	}

	supersededPrior := prior.clone()
	supersededPrior.Status = StatusSuperseded

	carrying := replacement.clone()
	carrying.Supersedes = supersedes

	return carrying, supersededPrior, nil
}

// clone deep-copies the parts of a Decision that are shared by reference.
//
// A Decision is a value, so its scalar fields are already copied; its two
// slices and its scope pointer are not. Returning records that alias their
// inputs would let a later edit to either rewrite the other, which is the
// mutation the immutability rule exists to prevent — and here it is also
// correctness, because Supersede's whole purpose is to hand back two records
// the caller writes as they are.
func (d Decision) clone() Decision {
	d.Supersedes = slices.Clone(d.Supersedes)
	d.Scope = d.Scope.clone()
	d.Tags = slices.Clone(d.Tags)
	return d
}
