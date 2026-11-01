package redact

// Purpose: Scan is checked against the reference it replaced: values() followed
// by mergeSpans, which builds the offset tables for every pass (D-1515). The
// fast path may only skip work, never change the matches.

import (
	"encoding/base64"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func refScan(r *Redactor, blob []byte) []Match {
	spans, _ := r.snapshot().values(blob)
	var out []Match
	for _, s := range mergeSpans(spans) {
		out = append(out, Match{Offset: s.s, Len: s.e - s.s})
	}
	return out
}

func upperASCII(s string) string { return strings.ToUpper(s) }

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// scanInputs builds a blob from pieces that exercise the skip rules: the value
// and its forms, partial prefixes at the end, whitespace, controls, escapes,
// upper case, non-ASCII, invalid UTF-8 and line breaks.
func scanInputs(rng *rand.Rand, vals []string, n int) []byte {
	pieces := []string{"\n", "\r\n", " ", "\t", "\x00", "\x1b[31m", "\x1b[", "\x1b", "\u200b", "\u00ad", "\xc2\x85", "\xc2\xa0", "\xff", "\xe2\x80", "K", "İ", "é", "雪", "abc", "ABC", "Zq7", "+/=", "%2B"}
	for _, v := range vals {
		pieces = append(pieces, v, v[:len(v)/2], v[:len(v)-1], upperASCII(v), reverseString(v), b64(v))
	}
	var b []byte
	for i := 0; i < n; i++ {
		switch rng.Intn(6) {
		case 0:
			b = append(b, byte(rng.Intn(256)))
		default:
			b = append(b, pieces[rng.Intn(len(pieces))]...)
		}
	}
	return b
}

func TestScanMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(15))
	vals := []string{"bench-secret-01-Zq7Lm4Xv9w", "p+s/s!w'0$r&d,01:abc", "雪-secret-值-123456", "line1-secret-aaaa\nline2-secret-bbbb"}
	r := New()
	for _, v := range vals {
		if err := r.Register([]byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	hits := 0
	for i := 0; i < 6000; i++ {
		b := scanInputs(rng, vals, rng.Intn(40))
		got, want := r.Scan(b), refScan(r, b)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("input %q: got %v want %v", b, got, want)
		}
		if len(want) > 0 {
			hits++
		}
	}
	if hits == 0 || hits == 6000 {
		t.Fatalf("degenerate corpus: %d hits", hits)
	}
}

func FuzzScanMatchesReference(f *testing.F) {
	f.Add([]byte("xx bench-secret-01-Zq7Lm4Xv9w yy"), "bench-secret-01-Zq7Lm4Xv9w")
	f.Add([]byte("BENCH \n SECRET-01-zq7lm4xv9w\x1b[0m"), "bench-secret-01-Zq7Lm4Xv9w")
	f.Add([]byte("bench-secr\u200b\u200b"), "bench-secret-01-Zq7Lm4Xv9w")
	f.Fuzz(func(t *testing.T, blob []byte, val string) {
		r := New()
		if r.Register([]byte(val)) != nil {
			return
		}
		if got, want := r.Scan(blob), refScan(r, blob); !reflect.DeepEqual(got, want) {
			t.Fatalf("blob %q value %q: got %v want %v", blob, val, got, want)
		}
	})
}
