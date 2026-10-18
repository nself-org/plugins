package redact

import (
	"fmt"
	"strings"
	"testing"
)

// BenchmarkWrap: throughput with 64 registered values over log-like text that
// holds some secrets, an escape and a command. The ticket floor is 10 MB/s.
func BenchmarkWrap(b *testing.B) { benchWrap(b, "bench-secret-%02d-%s") }

// BenchmarkWrapPunct registers punctuation-heavy values, which expand into many more forms.
func BenchmarkWrapPunct(b *testing.B) { benchWrap(b, "p+s/s!w'0$r&d,%02d:%s?@") }

func benchWrap(b *testing.B, format string) {
	r := New()
	for i := 0; i < 64; i++ {
		_ = r.Register([]byte(fmt.Sprintf(format, i, strings.Repeat("x7Q", 6))))
	}
	var sb strings.Builder
	for sb.Len() < 32<<10 {
		n := sb.Len()
		fmt.Fprintf(&sb, "2026-10-18T12:00:%02d INFO step %d finished in 1.%03ds status=ok path=/var/lib/ci/work/abc/def\n", n%60, n, n%1000)
		if n%7 == 0 {
			sb.WriteString("retrying with bench-secret-07-x7Qx7Qx7Qx7Qx7Qx7Q against https://example.test/api\n")
		}
		if n%11 == 0 {
			sb.WriteString("\x1b[32mgreen\x1b[0m ::notice::done std::fmt::Display\n")
		}
	}
	chunk := []byte(sb.String())
	var sink discard
	w := r.Wrap(&sink)
	b.SetBytes(int64(len(chunk)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = w.Write(chunk)
	}
	_ = w.Close()
}

type discard struct{ n int }

func (d *discard) Write(p []byte) (int, error) { d.n += len(p); return len(p), nil }
