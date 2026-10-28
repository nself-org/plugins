//go:build security

package redteam

// Purpose: TM-29 attacks on forms of a registered value that a terminal or a reader still shows
// as the secret: render-invisible insertions (SGR, NUL, zero width) and plain re-orderings.
// Inputs:  fakeSecret, registered with the real redact.Redactor, printed in transformed forms.
// Outputs: pass or fail.
// Constraints: local fixtures only. These previously recorded residuals are now
//          unconditional regression tests.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

var sgrRe = regexp.MustCompile("\x1b\\[[0-9;:]*m")

// rendered is what a reader sees: colour codes, NUL and zero-width characters take no space.
func rendered(s string) string {
	s = sgrRe.ReplaceAllString(s, "")
	return strings.NewReplacer("\x00", "", "\u200b", "", "\u200d", "", "\u00ad", "", "\ufeff", "").Replace(s)
}

func interleave(v, sep string) string {
	parts := make([]string, 0, len(v))
	for i := 0; i < len(v); i++ {
		parts = append(parts, v[i:i+1])
	}
	return strings.Join(parts, sep)
}

func TestB_Redactor_RenderInvisibleInsertions(t *testing.T) {
	r := redact.New()
	mustRegister(t, r, fakeSecret)
	for name, in := range map[string]string{
		"sgr-between-chars": interleave(fakeSecret, "\x1b[0m"),
		"sgr-once":          fakeSecret[:8] + "\x1b[1m" + fakeSecret[8:],
		"nul-between-chars": interleave(fakeSecret, "\x00"),
		"zero-width-space":  fakeSecret[:8] + "\u200b" + fakeSecret[8:],
		"soft-hyphen":       fakeSecret[:8] + "\u00ad" + fakeSecret[8:],
	} {
		out := rendered(feedChunks(r, "k="+in+"\n"))
		if strings.Contains(out, fakeSecret) {
			t.Errorf("%s: the secret reads in clear once invisible bytes are dropped: %q", name, out)
		}
	}
}

func TestB_Redactor_ReorderedAndCaseFolded(t *testing.T) {
	r := redact.New()
	mustRegister(t, r, fakeSecret)
	rev := []byte(fakeSecret)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	half := len(fakeSecret) / 2
	for name, in := range map[string]string{
		"reversed":   string(rev),
		"upper":      strings.ToUpper(fakeSecret),
		"lower":      strings.ToLower(fakeSecret),
		"two-lines":  fakeSecret[:half] + "\n" + fakeSecret[half:],
		"space-fold": fakeSecret[:half] + " " + fakeSecret[half:],
	} {
		out := feedChunks(r, "k="+in+"\n")
		norm := strings.ToLower(strings.NewReplacer("\n", "", " ", "").Replace(out))
		rn := strings.ToLower(fakeSecret)
		rrev := string(rev)
		leaked := strings.Contains(norm, rn) || strings.Contains(strings.ToLower(out), strings.ToLower(rrev)) || strings.Contains(norm, strings.NewReplacer("a", "4", "e", "3").Replace(rn))
		if leaked {
			t.Errorf("%s: transformed secret released: %q", name, out)
		}
	}
}
