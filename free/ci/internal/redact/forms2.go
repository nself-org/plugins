package redact

// Purpose: forms a value takes when a shell traces it or a base64 text is
// carried inside a URL or a JSON string.
// Inputs:  one value or one family B (base64) pattern. Outputs: extra patterns.
// Constraints: pure functions; only forms that differ from the plain value are
// returned, so a value with no shell metacharacter adds nothing.

import (
	"bytes"
	"strings"
)

// shellForms: bash `set -x` and `printf %q` write a value with quotes and
// backslashes inserted: 'it'\”s' for a quote, a backslash before each
// metacharacter for %q.
func shellForms(v []byte) [][]byte {
	var out [][]byte
	if bytes.IndexByte(v, '\'') >= 0 {
		out = append(out, bytes.ReplaceAll(v, []byte("'"), []byte(`'\''`)))
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
