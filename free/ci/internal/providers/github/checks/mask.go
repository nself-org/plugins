package checks

// Purpose: one place that turns text safe. A Redactor holds the values this adapter handles
// (the PEM, the JWT, the installation token) and, with no value registered, still masks token
// shapes. Everything that leaves the package as text (an error, the check-run text) goes through it.

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// registerValue adds a secret value; a value the redactor refuses (too short) is not worth hiding.
func registerValue(r *redact.Redactor, v []byte) { _ = r.Register(v) }

// maskString runs s through the redactor.
func maskString(r *redact.Redactor, s string) string {
	var b bytes.Buffer
	w := r.Wrap(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.String()
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return truncRunes(s, 200)
}

func truncRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// safeText masks first and cuts second: a secret that straddles the cut must not leave a prefix.
func safeText(r *redact.Redactor, s string) string { return firstLine(maskString(r, s)) }
