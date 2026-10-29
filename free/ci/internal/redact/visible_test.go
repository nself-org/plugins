package redact

import (
	"strings"
	"testing"
)

func TestInvisibleSecretAcrossEveryChunkBoundary(t *testing.T) {
	const secret = "synthetic-Visible-0123456789"
	r := New()
	if err := r.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	for _, sep := range []string{"\x1b[31m", "\x00", "\u200b", "\u200d", "\u00ad", "\ufeff"} {
		in := strings.Join(strings.Split(secret, ""), sep)
		for split := 0; split <= len(in); split++ {
			got := run(r, in[:split], in[split:])
			if !strings.Contains(got, Placeholder) || strings.Contains(got, "synthetic") {
				t.Fatalf("separator %q split %d was not masked: %q", sep, split, got)
			}
		}
		if got := run(r, bytewise(in)...); !strings.Contains(got, Placeholder) {
			t.Fatalf("bytewise separator %q: %q", sep, got)
		}
	}
	// Normal Unicode continuation bytes must not be mistaken for raw C1 controls.
	in := "日本語 é café 😀"
	if got := run(New(), bytewise(in)...); got != in {
		t.Fatalf("Unicode changed: %q", got)
	}
}
