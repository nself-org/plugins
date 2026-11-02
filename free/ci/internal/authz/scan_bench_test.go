package authz_test

// Purpose: benchmark CleanCandidates on a 1 MiB chunk (D-1515). The chunk mixes
// log text, source-like text and binary noise, none of it holding a registered
// value, so every scan form runs to completion.

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func benchScanner(b testing.TB) *redact.Redactor {
	r := redact.New()
	for i := 0; i < 16; i++ {
		if err := r.Register([]byte(fmt.Sprintf("bench-secret-%02d-Zq7Lm4Xv9w", i))); err != nil {
			b.Fatal(err)
		}
	}
	return r
}

func benchChunk(n int) []byte {
	rng := rand.New(rand.NewSource(1))
	var sb strings.Builder
	for sb.Len() < n/2 {
		fmt.Fprintf(&sb, "2026-10-18T12:00:%02d INFO step %d finished in 1.%03ds path=/var/lib/ci/work/abc/def\n", sb.Len()%60, sb.Len(), sb.Len()%1000)
		fmt.Fprintf(&sb, "func (w *scanWriter) Write(p []byte) (int, error) { return len(p), nil } // %d\n", sb.Len())
	}
	out := []byte(sb.String())
	for len(out) < n {
		out = append(out, byte(rng.Intn(256)))
	}
	return out[:n]
}

func BenchmarkCleanCandidates1MiB(b *testing.B) {
	r, chunk := benchScanner(b), benchChunk(1<<20)
	b.SetBytes(int64(len(chunk)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !authz.CleanCandidates(r, chunk) {
			b.Fatal("unexpected match")
		}
	}
}

// BenchmarkCleanCandidatesNoValues1MiB is the same chunk with nothing registered,
// as in the cache bench: only the cost of reading the chunk in its other forms.
func BenchmarkCleanCandidatesNoValues1MiB(b *testing.B) {
	r, chunk := redact.New(), benchChunk(1<<20)
	b.SetBytes(int64(len(chunk)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !authz.CleanCandidates(r, chunk) {
			b.Fatal("unexpected match")
		}
	}
}
