package authz

// Purpose: the pooled UTF-16 and NUL-strip builders must produce the exact
// bytes of the reference conversions CleanCandidates used before (D-1515).

import (
	"bytes"
	"math/rand"
	"testing"
	"unicode/utf16"
)

func refUTF16(b []byte, offset int, big bool) []byte {
	units := make([]uint16, 0, len(b)/2)
	for i := offset; i+1 < len(b); i += 2 {
		a, c := b[i], b[i+1]
		if big {
			a, c = c, a
		}
		units = append(units, uint16(a)|uint16(c)<<8)
	}
	return []byte(string(utf16.Decode(units)))
}

func TestAppendUTF16MatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	pieces := [][]byte{{0x00, 0xD8}, {0x00, 0xDC}, {0x3D, 0xD8}, {0x00, 0xDE}, {0xFF, 0xFF}, {0x41, 0x00}, {0xE9, 0x00}, {0x00, 0x4E}, {0xFD, 0xFF}}
	for n := 0; n < 4000; n++ {
		var b []byte
		for k, m := 0, rng.Intn(24); k < m; k++ {
			if rng.Intn(3) == 0 {
				b = append(b, byte(rng.Intn(256)))
			} else {
				b = append(b, pieces[rng.Intn(len(pieces))]...)
			}
		}
		for _, big := range []bool{false, true} {
			for off := 0; off < 2; off++ {
				got := appendUTF16([]byte("junk"), b, off, big)[4:]
				if want := refUTF16(b, off, big); !bytes.Equal(got, want) {
					t.Fatalf("input %x off %d big %v: got %x want %x", b, off, big, got, want)
				}
			}
		}
		if got, want := stripNUL(nil, b), bytes.ReplaceAll(b, []byte{0}, nil); !bytes.Equal(got, want) {
			t.Fatalf("stripNUL %x: got %x want %x", b, got, want)
		}
	}
}
