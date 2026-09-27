package identity

import "testing"

// TestABackwardsClockStillMintsAscendingIds is the branch NewID cannot reach
// from outside.
//
// A host clock can step backwards — NTP corrections, a virtual machine
// resuming from a snapshot, a laptop waking in another timezone with a bad
// hardware clock. The published format promises that creation order is
// lexicographic order, and the millisecond prefix is what carries it, so a
// backwards step would mint an id that sorts *before* one already written and
// silently reorder the rows using it as a tie breaker.
//
// nextEntropy clamps: a millisecond that is not strictly newer keeps the last
// one and increments the randomness instead. That is asserted here rather than
// through NewID because there is no way to make time.Now go backwards from a
// test, and a branch no test can reach is the shape three MR-002 audit rounds
// flagged.
func TestABackwardsClockStillMintsAscendingIds(t *testing.T) {
	entropy.mu.Lock()
	entropy.millis = 0
	entropy.last = [idRandomnessBytes]byte{}
	entropy.mu.Unlock()

	const start = 1_700_000_000_000

	first, firstRandom := nextEntropy(start)
	if first != start {
		t.Fatalf("nextEntropy(%d) stamped %d, want the millisecond it was given", start, first)
	}

	// Ten seconds backwards, which is a correction no plausible clock avoids.
	for step := range 4 {
		millis, randomness := nextEntropy(start - 10_000)
		if millis != start {
			t.Fatalf("step %d: a backwards clock stamped %d, want the clamp to %d", step, millis, start)
		}
		if compareRandomness(randomness, firstRandom) <= 0 {
			t.Fatalf("step %d: randomness %v does not sort after %v, so the id would sort backwards",
				step, randomness, firstRandom)
		}
		firstRandom = randomness
	}

	// The other arm: a clock that really did move forward is not clamped, or
	// every id after a single backwards step would carry a stale millisecond
	// forever.
	if millis, _ := nextEntropy(start + 1); millis != start+1 {
		t.Errorf("a forward clock stamped %d, want %d", millis, start+1)
	}
}

// TestTheEntropyCounterCarriesAcrossAByte is the increment's own edge.
//
// The counter walks from the least significant byte and carries on overflow, so
// the one input that exercises the carry is a randomness value ending in 0xff.
// Without the carry, 0x…ff would wrap to 0x…00 and the next id in the same
// millisecond would sort before its predecessor.
func TestTheEntropyCounterCarriesAcrossAByte(t *testing.T) {
	const millis = 1_700_000_000_000

	entropy.mu.Lock()
	entropy.millis = millis
	entropy.last = [idRandomnessBytes]byte{}
	entropy.last[idRandomnessBytes-1] = 0xff
	before := entropy.last
	entropy.mu.Unlock()

	_, after := nextEntropy(millis)
	if compareRandomness(after, before) <= 0 {
		t.Fatalf("randomness %v does not sort after %v across a byte boundary", after, before)
	}
	if after[idRandomnessBytes-1] != 0 || after[idRandomnessBytes-2] != 1 {
		t.Errorf("randomness = %v, want the low byte wrapped to 0 and the next one carried to 1", after)
	}
}

// compareRandomness orders two randomness values the way the rendered base32
// body orders them: most significant byte first.
func compareRandomness(a, b [idRandomnessBytes]byte) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}
