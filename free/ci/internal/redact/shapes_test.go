package redact

import (
	"encoding/base64"
	"strings"
	"testing"
)

// Fixtures are built at run time from fake parts so no token-shaped literal
// sits in the source.
func shapeCorpus() map[string]string {
	rep := func(s string, n int) string { return strings.Repeat(s, n) }
	jwt := func(h, p string) string {
		e := base64.RawURLEncoding.EncodeToString
		return e([]byte(h)) + "." + e([]byte(p)) + "." + e([]byte("not-a-real-signature-fake"))
	}
	return map[string]string{
		"ghp":     "ghp" + "_" + rep("Ab1", 12),
		"gho":     "gho" + "_" + rep("Ab1", 12),
		"ghs":     "ghs" + "_" + rep("Ab1", 12),
		"ghu":     "ghu" + "_" + rep("Ab1", 12),
		"ghr":     "ghr" + "_" + rep("Ab1", 12),
		"pat":     "github" + "_pat_" + rep("Xy9_", 15),
		"glpat":   "glpat" + "-" + rep("Zz_8", 6),
		"glrt":    "glrt" + "-" + rep("Zz_8", 6),
		"akia":    "AKIA" + "ABCDEFGH12345678",
		"asia":    "ASIA" + "ABCDEFGH12345678",
		"sklive":  "sk_" + "live_" + rep("a1B2", 8),
		"rklive":  "rk_" + "live_" + rep("a1B2", 8),
		"jwt":     jwt(`{"alg":"none","typ":"fake"}`, `{"sub":"nobody","fake":true}`),
		"slack":   "xox" + "b-" + rep("1234567890-", 3),
		"bearer":  "Bearer " + rep("abCD12.-", 4),
		"nselfpr": "nself" + "_pro_" + rep("q7Z", 8),
	}
}

func TestTokenShapesMasked(t *testing.T) {
	r := New()
	for name, tok := range shapeCorpus() {
		in := "before " + tok + " after\n"
		want := "before [REDACTED] after\n"
		if got := run(r, in); got != want {
			t.Errorf("%s: got %q", name, got)
		}
		for i := 1; i < len(in); i += 2 {
			if got := run(r, in[:i], in[i:]); got != want {
				t.Errorf("%s split %d: got %q", name, i, got)
			}
		}
		if got := strings.Join(bytewise(run(r, bytewise(in)...)), ""); got != want {
			t.Errorf("%s bytewise: got %q", name, got)
		}
	}
}

func pemOpen(label string) string { return "-----BEGIN " + label + "-----" }

func pemBlock(label, body string) string {
	return "-----BEGIN " + label + "-----\n" + body + "\n-----END " + label + "-----"
}

func TestPEMBlocks(t *testing.T) {
	r := New()
	body := strings.Repeat("TUlJRmFrZUtleU1hdGVyaWFsRm9yVGVzdHNPbmx5\n", 30)
	for _, label := range []string{"RSA PRIVATE KEY", "OPENSSH PRIVATE KEY", "CERTIFICATE", "EC PRIVATE KEY"} {
		in := "head\n" + pemBlock(label, body) + "\ntail\n"
		want := "head\n[REDACTED]\ntail\n"
		if got := run(r, in); got != want {
			t.Errorf("%s: got %q", label, got)
		}
		for _, n := range []int{1, 7, 64, 100, 1000} {
			var chunks []string
			for i := 0; i < len(in); i += n {
				chunks = append(chunks, in[i:min(i+n, len(in))])
			}
			if got := run(r, chunks...); got != want {
				t.Errorf("%s chunk %d: got %q", label, n, got)
			}
		}
	}
	// an unclosed block is masked to the end of the stream
	if got := run(r, "x\n"+pemOpen("PRIVATE KEY")+"\nabc\ndef\n"); got != "x\n[REDACTED]" {
		t.Errorf("unclosed: %q", got)
	}
	// PEM body is dropped as it streams: the carry-over stays small
	f := r.NewFilter()
	f.Redact([]byte(pemOpen("PRIVATE KEY") + "\n"))
	for i := 0; i < 1000; i++ {
		f.Redact([]byte(body))
		if len(f.buf) > 4*pemTail {
			t.Fatalf("PEM body buffered: %d", len(f.buf))
		}
	}
	if tail := string(f.Redact([]byte("-----END " + "PRIVATE KEY-----\nafter\n"))); tail != "\nafter\n" {
		t.Fatalf("after END: %q", tail)
	}
}

func TestShapeFalsePositives(t *testing.T) {
	r := New()
	for _, s := range []string{"eyJhbGciOiJub25lIn0 is just a header\n", "task-run-done: ok\n"} {
		if got := run(r, s); got != s {
			t.Errorf("changed %q -> %q", s, got)
		}
	}
}
