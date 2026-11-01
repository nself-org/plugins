package authz

// Purpose: CleanCandidates now skips the four UTF-16 decodings when the scanner
// proves none can hold a value (D-1515). refCleanCandidates is the previous
// implementation, kept here as the reference: the verdict of the new one must be
// identical on every input, including values with non-ASCII, surrogate-pair and
// case-folding forms, encoded in all four readings with skipped units inserted.
// Run the fuzzer with: go test -fuzz FuzzCleanCandidatesMatchesReference ./internal/authz

import (
	"bytes"
	"encoding/base64"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// refCleanCandidates is CleanCandidates before D-1515: raw, NUL-stripped and
// four decoded readings, each passed to Scan.
func refCleanCandidates(s ValueScanner, b []byte) bool {
	if !Clean(s, b) {
		return false
	}
	var buf []byte
	if bytes.IndexByte(b, 0) >= 0 {
		buf = stripNUL(nil, b)
		if !Clean(s, buf) {
			return false
		}
	}
	for _, big := range []bool{false, true} {
		for offset := 0; offset < 2; offset++ {
			buf = appendUTF16(buf[:0], b, offset, big)
			if !Clean(s, buf) {
				return false
			}
		}
	}
	return true
}

var gateValues = [][]string{
	{"bench-secret-01-Zq7Lm4Xv9w", "second-value-ABCdef-123456"},
	{"my secret with spaces 99", "tab\tseparated-value-77"},
	{"雪-secret-值-123456"},
	{"emoji-😀-secret-value-42", "ünïcödé-secret-0987"},
	{"kelvin-K-İstanbul-value-1", "KEYKEY-upper-123456"},
	{"line1-secret-aaaa\nline2-secret-bbbb"},
	{"nul\x00inside-value-123456"},
	{"p+s/s!w'0$r&d,01:abc"},
}

func newGateRedactor(t testing.TB, vals []string) *redact.Redactor {
	r := redact.New()
	for _, v := range vals {
		if err := r.Register([]byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func utf16Bytes(s string, big bool) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(s)) {
		if big {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

// skippers are texts the scan ignores (or might): invisible controls, spaces,
// line breaks, NUL, colour escapes, a lone ESC, folding letters.
var skippers = []string{"\u200b", "\u00ad", " ", "\n", "\r\n", "\t", "\x00", "\x1b[31m", "\x1b[0m", "\x1b", "\u0085", "\u00a0", "\ufeff", "\u2028", "\u3000"}

// mutate inserts skippers between runes and flips case.
func mutate(rng *rand.Rand, s string) string {
	var sb strings.Builder
	for _, r := range s {
		if rng.Intn(4) == 0 {
			sb.WriteString(skippers[rng.Intn(len(skippers))])
		}
		switch rng.Intn(8) {
		case 0:
			sb.WriteString(strings.ToUpper(string(r)))
		case 2: // letters that fold to ASCII: KELVIN SIGN and LATIN CAPITAL I WITH DOT ABOVE
			switch r {
			case 'k', 'K':
				sb.WriteRune(0x212A)
			case 'i', 'I':
				sb.WriteRune(0x130)
			default:
				sb.WriteRune(r)
			}
		case 1:
			sb.WriteString(strings.ToLower(string(r)))
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// genChunk assembles a chunk from raw and UTF-16 pieces of the values and noise.
func genChunk(rng *rand.Rand, vals []string, pieces int) []byte {
	var b []byte
	if rng.Intn(2) == 0 {
		b = append(b, byte(rng.Intn(256))) // odd alignment
	}
	for k := 0; k < pieces; k++ {
		v := vals[rng.Intn(len(vals))]
		var s string
		switch rng.Intn(9) {
		case 0:
			s = v
		case 1:
			s = strings.ToUpper(v)
		case 2:
			s = reverse(v)
		case 3:
			s = base64.StdEncoding.EncodeToString([]byte(v))
		case 4:
			s = v[:rng.Intn(len(v))] // partial value
		case 5:
			s = strings.Repeat(skippers[rng.Intn(len(skippers))], rng.Intn(3000)) // long skipped padding
		case 6:
			s = string([]rune{rune(0x212A), rune(0x130), 0x1F600, 0xFFFD, 'a', 'Z'})
		case 7:
			s = strings.Repeat("x", rng.Intn(40))
		default:
			s = v
		}
		s = mutate(rng, s)
		switch rng.Intn(5) {
		case 0:
			b = append(b, s...)
		case 1:
			b = append(b, utf16Bytes(s, false)...)
		case 2:
			b = append(b, utf16Bytes(s, true)...)
		case 3: // lone surrogates and random units
			b = append(b, utf16Bytes(s, rng.Intn(2) == 0)...)
			b = append(b, byte(rng.Intn(256)), 0xD8, 0x00, 0xDC, byte(rng.Intn(256)), 0xDC)
		default:
			for i := 0; i < rng.Intn(20); i++ {
				b = append(b, byte(rng.Intn(256)))
			}
		}
	}
	return b
}

func TestCleanCandidatesMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1515))
	rejected, gated := 0, 0
	for _, vals := range gateValues {
		r := newGateRedactor(t, vals)
		for i := 0; i < 1500; i++ {
			b := genChunk(rng, vals, rng.Intn(6))
			got, want := CleanCandidates(r, b), refCleanCandidates(r, b)
			if got != want {
				t.Fatalf("values %q chunk %x: got clean=%v, reference clean=%v", vals, b, got, want)
			}
			if !want {
				rejected++
			}
			if !r.UTF16Candidate(b) {
				gated++
			}
		}
	}
	if rejected < 1000 || gated < 1000 {
		t.Fatalf("degenerate corpus: %d rejected, %d gated of %d", rejected, gated, len(gateValues)*1500)
	}
}

// TestCleanCandidatesTailPadding: a partial value followed by more skipped bytes
// than the conservative mask allows is reported by Scan; the gate must not hide it.
func TestCleanCandidatesTailPadding(t *testing.T) {
	vals := gateValues[0]
	r := newGateRedactor(t, vals)
	for _, pad := range []string{"\u200b", " ", "\x00", "\u00ad", "\n"} {
		for _, n := range []int{10, 500, 1500, 4000, 20000} {
			s := vals[0][:9] + strings.Repeat(pad, n)
			for _, big := range []bool{false, true} {
				for _, lead := range []int{0, 1} {
					b := append(make([]byte, lead), utf16Bytes(s, big)...)
					if got, want := CleanCandidates(r, b), refCleanCandidates(r, b); got != want {
						t.Fatalf("pad %q x%d big=%v lead=%d: got %v want %v", pad, n, big, lead, got, want)
					}
				}
			}
		}
	}
}

func FuzzCleanCandidatesMatchesReference(f *testing.F) {
	f.Add([]byte("noise"), "bench-secret-01-Zq7Lm4Xv9w", uint8(1))
	f.Add([]byte{0x1b, 0}, "bench-secret-01-Zq7Lm4Xv9w", uint8(2))
	f.Add([]byte("x"), "雪-secret-值-123456", uint8(3))
	f.Add([]byte{0x2a, 0x21, 0x30, 0x01}, "kelvin-K-İstanbul-value-1", uint8(6))
	f.Add([]byte{0, 0xd8, 0, 0xdc}, "emoji-😀-secret-value-42", uint8(5))
	f.Fuzz(func(t *testing.T, blob []byte, val string, mode uint8) {
		r := redact.New()
		if r.Register([]byte(val)) != nil || r.Register([]byte("anchor-ASCII-secret-9876")) != nil {
			return
		}
		rng := rand.New(rand.NewSource(int64(mode)))
		enc := mutate(rng, val)
		if mode&8 != 0 {
			enc = strings.ToUpper(enc)
		}
		var piece []byte
		switch mode % 4 {
		case 0:
			piece = []byte(enc)
		case 1:
			piece = utf16Bytes(enc, false)
		case 2:
			piece = utf16Bytes(enc, true)
		default:
			piece = utf16Bytes(enc[:len(enc)/2], mode&16 != 0)
		}
		k := int(mode) % (len(blob) + 1)
		for _, b := range [][]byte{blob, append(append(append([]byte{}, blob[:k]...), piece...), blob[k:]...)} {
			if got, want := CleanCandidates(r, b), refCleanCandidates(r, b); got != want {
				t.Fatalf("chunk %x value %q: got clean=%v, reference clean=%v", b, val, got, want)
			}
		}
	})
}
