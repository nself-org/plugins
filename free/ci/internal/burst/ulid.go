package burst

// Purpose: the intent token, a 26-character ULID (48-bit millisecond time, 80
// bits of entropy, Crockford base32): short enough for a 63-character label,
// sortable by birth, and unguessable.
// Inputs:  the time of the intent. Outputs: the token.
// Constraints: entropy comes from crypto/rand; a short read is an error, never a weaker token.

import (
	"crypto/rand"
	"fmt"
	"io"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newULID(now time.Time) (string, error) { return ulidFrom(now, rand.Reader) }

func ulidFrom(now time.Time, entropy io.Reader) (string, error) {
	var e [10]byte
	if _, err := io.ReadFull(entropy, e[:]); err != nil {
		return "", fmt.Errorf("burst: entropy: %w", err)
	}
	ms := uint64(now.UnixMilli())
	var out [26]byte
	for i := 9; i >= 0; i-- {
		out[i] = crockford[ms&31]
		ms >>= 5
	}
	var acc uint32
	bits, n := 0, 10
	for _, b := range e {
		acc = acc<<8 | uint32(b)
		bits += 8
		for bits >= 5 {
			out[n] = crockford[(acc>>(bits-5))&31]
			n++
			bits -= 5
		}
	}
	return string(out[:]), nil
}
