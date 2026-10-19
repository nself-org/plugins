package session

import (
	"strings"
	"testing"
)

func collect(masks ...string) (*lineWriter, *[]string) {
	var out []string
	var ms [][]byte
	for _, m := range masks {
		for _, x := range secretMasks(m) {
			ms = append(ms, []byte(x))
		}
	}
	return newLineWriter(func(s string) { out = append(out, s) }, func() [][]byte { return ms }), &out
}

// TestMultiLineSecretIsMaskedLineByLine proves a multi-line value (a service
// account JSON, a certificate) is hidden even when the job prints it line by line.
func TestMultiLineSecretIsMaskedLineByLine(t *testing.T) {
	w, out := collect("line-one-SECRETA\nline-two-SECRETB\n}")
	w.Write([]byte("x line-one-SECRETA\ny line-two-SECRETB\n}\n"))
	for _, l := range *out {
		if strings.Contains(l, "SECRET") {
			t.Fatalf("leaked: %q", *out)
		}
	}
	if (*out)[2] != "}" {
		t.Fatalf("a short line was masked: %q", *out)
	}
}

// TestSecretSplitAcrossChunksAndCutIsMasked proves a value split across Write
// calls, or lying across the long-line cut, is masked whole.
func TestSecretSplitAcrossChunksAndCutIsMasked(t *testing.T) {
	secret := strings.Repeat("S3cr3t-", 600) // 4200 bytes
	for _, pad := range []int{maxLogLine - 100, maxLogLine + 3, maxLogLine + 200, 3 * maxLogLine} {
		w, out := collect(secret)
		text := strings.Repeat("a", pad) + secret + strings.Repeat("b", 50)
		for i := 0; i < len(text); i += 1000 {
			w.Write([]byte(text[i:min(i+1000, len(text))]))
		}
		w.Flush()
		if all := strings.Join(*out, "\n"); strings.Contains(all, "S3cr3t") || !strings.Contains(all, maskText) {
			t.Fatalf("pad %d: secret leaked or not masked (%d lines)", pad, len(*out))
		}
	}
}
