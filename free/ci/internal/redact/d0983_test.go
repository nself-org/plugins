package redact

// Purpose: regression tests for D-0983. Scan must not keep one span per
// matching position of a periodic value, and a mask that covers the body of a
// colour sequence must not leave a raw ESC in front of the placeholder.

import (
	"bytes"
	"runtime"
	"testing"
)

func TestScanPeriodicBlobStaysSmall(t *testing.T) {
	r := New()
	mustRegister(t, r, "abababab")
	blob := bytes.Repeat([]byte("ab"), 1<<19) // 1 MiB
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	got := r.Scan(blob)
	runtime.ReadMemStats(&after)
	if len(got) != 1 || got[0].Offset != 0 || got[0].Len != len(blob) {
		t.Fatalf("matches: %v", got)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 1<<20 {
		t.Fatalf("Scan allocated %d bytes for a 1 MiB blob", alloc)
	}
}

func TestMaskedSGRBodyRewritesEsc(t *testing.T) {
	r := New()
	mustRegister(t, r, "[31mred", "[1;32mSECRET")
	cases := map[string]string{
		"\x1b[31mred\x1b[0m":     "^[[REDACTED]\x1b[0m",
		"\x1b[1;32mSECRET tail":  "^[[REDACTED] tail",
		"\x1b[0m ok \x1b[1;32mx": "\x1b[0m ok \x1b[1;32mx",
	}
	for in, want := range cases {
		if got := run(r, in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
		if got := run(r, bytewise(in)...); got != want {
			t.Errorf("bytewise %q: got %q want %q", in, got, want)
		}
		for i := 1; i < len(in); i++ {
			if got := run(r, in[:i], in[i:]); got != want {
				t.Fatalf("%q split %d: got %q want %q", in, i, got, want)
			}
		}
	}
}
