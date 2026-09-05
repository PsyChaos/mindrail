package workspace

import (
	"crypto/rand"
	"sync"
	"time"
)

// Identifier prefixes. They are part of the persisted value, so an id read out
// of a log or an MCP payload says what kind of thing it names without a lookup.
const (
	projectIDPrefix   = "PRJ"
	workspaceIDPrefix = "WS"
)

// crockfordAlphabet is Crockford's base32: the digits plus the uppercase
// letters, minus I, L, O and U. Dropping those four is what makes an id safe
// to read aloud or retype -- 1/I and 0/O cannot be confused -- and dropping U
// keeps the alphabet from spelling words.
const crockfordAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// The id body is a 128-bit value in the ULID layout: 48 bits of millisecond
// timestamp followed by 80 bits of randomness, rendered as 26 base32
// characters. The timestamp leads so that lexicographic order is creation
// order, which is what lets a query sort by id instead of by a timestamp
// column whose resolution may tie.
const (
	idBodyLength      = 26
	idTimestampBytes  = 6
	idRandomnessBytes = 10
)

// entropy carries the monotonic-within-a-millisecond state. Two ids minted in
// the same millisecond would otherwise sort by their random parts, i.e.
// arbitrarily -- and MR-001 mints project and workspace ids back to back.
var entropy struct {
	mu     sync.Mutex
	millis int64
	last   [idRandomnessBytes]byte
}

// NewID returns an opaque, time-sortable identifier: prefix + "-" + 26
// characters.
//
// Opaque is the point (decision D-26). The id names a workspace without
// encoding where that workspace lives, so a clone that moves keeps its
// identity and nothing downstream can be tempted to parse a path back out.
func NewID(prefix string) string {
	millis, randomness := nextEntropy(time.Now().UTC().UnixMilli())

	var body [idTimestampBytes + idRandomnessBytes]byte
	for i := idTimestampBytes - 1; i >= 0; i-- {
		body[i] = byte(millis)
		millis >>= 8
	}
	copy(body[idTimestampBytes:], randomness[:])

	return prefix + "-" + encodeCrockford(body)
}

// nextEntropy returns the millisecond to stamp and the randomness to pair with
// it. Within one millisecond it increments the previous randomness rather than
// drawing new bytes, which keeps consecutive ids ordered without needing a
// finer clock; a clock that stepped backwards is clamped forward for the same
// reason.
func nextEntropy(millis int64) (int64, [idRandomnessBytes]byte) {
	entropy.mu.Lock()
	defer entropy.mu.Unlock()

	if millis <= entropy.millis {
		for i := idRandomnessBytes - 1; i >= 0; i-- {
			entropy.last[i]++
			if entropy.last[i] != 0 {
				break
			}
		}
		return entropy.millis, entropy.last
	}

	var fresh [idRandomnessBytes]byte
	// crypto/rand.Read is documented never to return an error; it terminates
	// the process rather than hand back predictable bytes.
	_, _ = rand.Read(fresh[:])
	// Clearing the top bit leaves 2^79 increments of headroom before the
	// counter above could wrap and break the ordering guarantee -- more ids in
	// one millisecond than any process will ever mint.
	fresh[0] &= 0x7f

	entropy.millis = millis
	entropy.last = fresh
	return millis, fresh
}

// encodeCrockford renders 128 bits as 26 base32 characters, most significant
// first. 26 characters hold 130 bits, so the value is padded with two leading
// zero bits; this is the same layout ULID uses.
func encodeCrockford(value [idTimestampBytes + idRandomnessBytes]byte) string {
	out := make([]byte, 0, idBodyLength)

	var (
		buffer    uint32
		bufferLen = 2 // the two padding bits
	)
	for _, b := range value {
		buffer = buffer<<8 | uint32(b)
		bufferLen += 8
		for bufferLen >= 5 {
			bufferLen -= 5
			out = append(out, crockfordAlphabet[(buffer>>bufferLen)&0x1f])
		}
	}

	return string(out)
}
