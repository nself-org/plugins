package jit

// Purpose: runner names carry their own birth time, because GitHub's runner list has no creation
// date and the orphan sweep needs an age. A name is "nself-jit-" plus a 26-character ULID
// (48-bit millisecond time, 80 bits of entropy, Crockford base32).
// Constraints: the entropy source is injected so tests are deterministic; a short read is an error,
//          never a weaker name.

import (
	"crypto/rand"
	"fmt"
	"io"
	"strings"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newULID encodes now and 10 bytes from entropy (crypto/rand when nil).
func newULID(now time.Time, entropy io.Reader) (string, error) {
	if entropy == nil {
		entropy = rand.Reader
	}
	var e [10]byte
	if _, err := io.ReadFull(entropy, e[:]); err != nil {
		return "", fmt.Errorf("jit: entropy: %w", err)
	}
	ms := uint64(now.UnixMilli())
	var out [26]byte
	for i := 9; i >= 0; i-- {
		out[i] = crockford[ms&31]
		ms >>= 5
	}
	// 80 bits = 16 characters of 5 bits.
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

// ulidTime reads the time out of a ULID; ok is false for anything that is not one.
func ulidTime(id string) (time.Time, bool) {
	if len(id) != 26 {
		return time.Time{}, false
	}
	var ms uint64
	for _, c := range strings.ToUpper(id[:10]) {
		i := strings.IndexRune(crockford, c)
		if i < 0 {
			return time.Time{}, false
		}
		ms = ms<<5 | uint64(i)
	}
	if strings.Trim(strings.ToUpper(id[10:]), crockford) != "" || ms > 1<<48-1 {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(ms)).UTC(), true
}

// runnerBirth is the registration time encoded in a runner name made by this package.
func runnerBirth(name string) (time.Time, bool) {
	id, ok := strings.CutPrefix(name, RunnerPrefix)
	if !ok {
		return time.Time{}, false
	}
	return ulidTime(id)
}
