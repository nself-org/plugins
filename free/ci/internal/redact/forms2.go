package redact

// Purpose: forms a value takes when a shell traces it or a base64 text is
// carried inside a URL or a JSON string.
// Inputs:  one value or one family B (base64) pattern. Outputs: extra patterns.
// Constraints: pure functions; only forms that differ from the plain value are
// returned, so a value with no shell metacharacter adds nothing.

import (
	"bytes"
	"fmt"
	"strings"
)

// shellForms: bash `set -x` and `printf %q` write a value with quotes and
// backslashes inserted: 'it'\”s' for a quote, a backslash before each
// metacharacter for %q, $'...' with named and octal escapes for control bytes
// (and for bytes >= 0x80 under the C locale). Go %q and Python repr write control
// bytes as \xNN inside a quoted string (goQuoted).
func shellForms(v []byte) [][]byte {
	var out [][]byte
	if bytes.IndexByte(v, '\'') >= 0 {
		out = append(out, bytes.ReplaceAll(v, []byte("'"), []byte(`'\''`)))
	}
	if bytes.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) || hasHighByte(v) { // bash $'...' tracing
		var a bytes.Buffer
		for _, c := range v {
			switch {
			case strings.IndexByte("\a\b\t\n\v\f\r\x1b", c) >= 0:
				a.WriteString(map[byte]string{7: `\a`, 8: `\b`, 9: `\t`, 10: `\n`, 11: `\v`, 12: `\f`, 13: `\r`, 27: `\E`}[c])
			case c < 0x20 || c >= 0x7f:
				fmt.Fprintf(&a, `\%03o`, c)
			case c == '\'' || c == '\\':
				a.WriteByte('\\')
				a.WriteByte(c)
			default:
				a.WriteByte(c)
			}
		}
		out = append(out, a.Bytes())
	}
	if bytes.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		out = append(out, goQuoted(v))
	}
	var q bytes.Buffer
	for _, c := range v {
		if strings.IndexByte(" !\"#$&'()*;<>?[\\]^`{|}~\t", c) >= 0 {
			q.WriteByte('\\')
		}
		q.WriteByte(c)
	}
	if !bytes.Equal(q.Bytes(), v) {
		out = append(out, q.Bytes())
	}
	return out
}

// b64Carried: a base64 pattern as it prints inside a URL query (%2B %2F %3D in
// both hex cases) or a PHP json_encode string (`\/`).
func b64Carried(f []byte) [][]byte {
	if !bytes.ContainsAny(f, "+/=") {
		return nil
	}
	s := string(f)
	out := [][]byte{
		[]byte(strings.NewReplacer("+", "%2B", "/", "%2F", "=", "%3D").Replace(s)),
		[]byte(strings.NewReplacer("+", "%2b", "/", "%2f", "=", "%3d").Replace(s)),
	}
	if strings.Contains(s, "/") {
		out = append(out, []byte(strings.ReplaceAll(s, "/", `\/`)))
	}
	return out
}

func hasHighByte(v []byte) bool {
	for _, c := range v {
		if c >= 0x80 {
			return true
		}
	}
	return false
}

// goQuoted is the inside of a Go %q / Python repr string: backslash escapes for
// quote, backslash and control bytes, other bytes (including UTF-8) as they are.
func goQuoted(v []byte) []byte {
	var b bytes.Buffer
	for _, c := range v {
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case strings.IndexByte("\a\b\t\n\v\f\r", c) >= 0:
			b.WriteString(map[byte]string{7: `\a`, 8: `\b`, 9: `\t`, 10: `\n`, 11: `\v`, 12: `\f`, 13: `\r`}[c])
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, c)
		default:
			b.WriteByte(c)
		}
	}
	return b.Bytes()
}
