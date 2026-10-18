package redact

// Purpose: every textual form a registered value can take in job output.
// Inputs:  one value (bytes). Outputs: exact-byte patterns (family A) and
// base64 patterns scanned with line breaks ignored (family B).
// Constraints: pure functions, no value is ever formatted into an error. Forms:
// raw, LF and CRLF line ends, URL (upper/lower hex, `+` or %20 for space, two
// reserved sets, all-bytes), JSON (HTML-escaped, plain, `\/`, ASCII-only,
// every rune as \uXXXX in both hex cases), hex and \xNN, base64 and base64url
// at all three alignments, padded and unpadded.

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// valueForms returns the family A and family B patterns for a registered value.
func valueForms(v []byte) (a, b [][]byte) {
	for _, t := range textForms(v) {
		if !bytes.ContainsAny(t, "\r\n") {
			b = append(b, t) // also scanned with line breaks ignored: folded output
		}
		a = append(a, t)
		a = append(a, urlForms(t)...)
		a = append(a, jsonForms(t)...)
		a = append(a, hexForms(t)...)
		a = append(a, shellForms(t)...)
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
			for _, f := range b64Forms(t, enc) {
				b = append(b, f)
				b = append(b, b64Carried(f)...)
			}
		}
	}
	return a, b
}

// textForms: the value as given, with LF ends and with CRLF ends.
func textForms(v []byte) [][]byte {
	lf := bytes.ReplaceAll(v, []byte("\r\n"), []byte("\n"))
	crlf := bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
	return [][]byte{v, lf, crlf}
}

// lineValues returns each line of a multi-line value that is long enough to
// register on its own. A single-line value yields nothing (the value is itself).
func lineValues(v []byte) [][]byte {
	norm := bytes.ReplaceAll(v, []byte("\r\n"), []byte("\n"))
	if !bytes.Contains(norm, []byte("\n")) {
		return nil
	}
	var out [][]byte
	for _, l := range bytes.Split(norm, []byte("\n")) {
		l = bytes.Trim(l, " \t\r")
		if len(l) >= MinValueLen {
			out = append(out, append([]byte(nil), l...))
		}
	}
	return out
}

// pemParts returns the pieces of a value outside its PEM blocks that are long
// enough to register on their own: the stream drops a PEM body unseen, so a
// value that holds one is matched by its head and tail instead.
func pemParts(v []byte) [][]byte {
	var out [][]byte
	rest := v
	parsed := false
	for {
		i := bytes.Index(rest, pemBeg)
		if i < 0 {
			break
		}
		he, ok, _ := pemHeader(rest, i, pemBeg)
		if !ok {
			break
		}
		if i >= MinValueLen {
			out = append(out, append([]byte(nil), rest[:i]...))
		}
		end, closed := pemClose(rest, he)
		if !closed {
			return out
		}
		parsed = true
		rest = rest[end:]
	}
	if parsed && len(rest) >= MinValueLen {
		out = append(out, append([]byte(nil), rest...))
	}
	return out
}

// isUnreserved reports whether an encoder leaves c raw (unreserved plus extra;
// tilde says whether this encoder keeps '~').
func isUnreserved(c byte, extra string, tilde bool) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '-' || c == '_' || c == '.':
		return true
	case c == '~':
		return tilde // PHP urlencode, Java URLEncoder and URLSearchParams escape it
	}
	return strings.IndexByte(extra, c) >= 0
}

// urlForms: percent-encodings of the kinds common encoders produce, each form
// once.
func urlForms(v []byte) [][]byte {
	var out [][]byte
	seen := map[string]bool{}
	add := func(f []byte) {
		if !seen[string(f)] {
			seen[string(f)] = true
			out = append(out, f)
		}
	}
	tildes := []bool{true}
	if bytes.IndexByte(v, '~') >= 0 {
		tildes = append(tildes, false)
	}
	for _, extra := range urlSetsFor(v) {
		for _, tilde := range tildes {
			for _, upper := range []bool{true, false} {
				for _, plus := range []bool{true, false} {
					add(percent(v, func(c byte) bool { return isUnreserved(c, extra, tilde) }, upper, plus))
				}
			}
		}
	}
	for _, upper := range []bool{true, false} {
		add(percent(v, func(byte) bool { return false }, upper, false))
	}
	return out
}

// percent encodes every byte keep rejects as %XX (or + for a space).
func percent(v []byte, keep func(byte) bool, upper, plus bool) []byte {
	digits := "0123456789abcdef"
	if upper {
		digits = "0123456789ABCDEF"
	}
	var b bytes.Buffer
	for _, c := range v {
		switch {
		case keep(c):
			b.WriteByte(c)
		case c == ' ' && plus:
			b.WriteByte('+')
		default:
			b.WriteByte('%')
			b.WriteByte(digits[c>>4])
			b.WriteByte(digits[c&15])
		}
	}
	return b.Bytes()
}

// jsonForms: the inside of a JSON string literal under the common encoders.
func jsonForms(v []byte) [][]byte {
	plain := jsonInner(v, false, false, false, true)
	out := [][]byte{jsonInner(v, true, false, false, true), plain,
		jsonInner(v, false, false, false, false), jsonInner(v, true, false, false, false)}
	for _, upper := range []bool{false, true} {
		out = append(out, jsonInner(v, false, true, upper, true), jsonInner(v, false, true, upper, false), jsonAll(v, upper))
	}
	out = append(out, jsonDotNet(v))
	for _, f := range out[:len(out):len(out)] { // PHP-style `\/` on top of any other escaping
		if bytes.Contains(f, []byte("/")) {
			out = append(out, bytes.ReplaceAll(f, []byte("/"), []byte(`\/`)))
		}
	}
	return out
}

// jsonInner escapes control characters, quotes and backslashes; with html it
// also escapes < > & and U+2028/9; with ascii it writes every non-ASCII rune as
// \uXXXX; short selects \n \r \t \b \f over \u000a style escapes.
func jsonInner(v []byte, html, ascii, upper, short bool) []byte {
	var b bytes.Buffer
	for _, r := range string(v) {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case short && strings.ContainsRune("\n\r\t\b\f", r):
			b.WriteString(map[rune]string{'\n': `\n`, '\r': `\r`, '\t': `\t`, '\b': `\b`, '\f': `\f`}[r])
		case r < 0x20 || (html && (r == '<' || r == '>' || r == '&' || r == 0x2028 || r == 0x2029)):
			b.WriteString(uEscape(r, upper))
		case r > 0x7f && ascii:
			b.WriteString(uEscape(r, upper))
		default:
			b.WriteRune(r)
		}
	}
	return b.Bytes()
}

// jsonDotNet is System.Text.Json's default: ASCII-safe, with + & < > ' ` written
// (and the quote) as uppercase \u00XX and everything else as plain JSON.
func jsonDotNet(v []byte) []byte {
	var b bytes.Buffer
	for _, r := range string(v) {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"' || strings.ContainsRune("+&<>'`", r) || r > 0x7f:
			b.WriteString(uEscape(r, true))
		case strings.ContainsRune("\n\r\t\b\f", r):
			b.WriteString(map[rune]string{'\n': `\n`, '\r': `\r`, '\t': `\t`, '\b': `\b`, '\f': `\f`}[r])
		case r < 0x20:
			b.WriteString(uEscape(r, true))
		default:
			b.WriteRune(r)
		}
	}
	return b.Bytes()
}

// jsonAll writes every rune as \uXXXX (surrogate pairs above the BMP).
func jsonAll(v []byte, upper bool) []byte {
	var b bytes.Buffer
	for _, r := range string(v) {
		b.WriteString(uEscape(r, upper))
	}
	return b.Bytes()
}

func uEscape(r rune, upper bool) string {
	f := "\\u%04x"
	if upper {
		f = "\\u%04X"
	}
	if r > 0xFFFF {
		r -= 0x10000
		return fmt.Sprintf(f, 0xD800+(r>>10)) + fmt.Sprintf(f, 0xDC00+(r&0x3FF))
	}
	return fmt.Sprintf(f, r)
}

// hexForms: lower and upper hex, and \xNN escapes.
func hexForms(v []byte) [][]byte {
	h := hex.EncodeToString(v)
	var x strings.Builder
	for _, c := range v {
		fmt.Fprintf(&x, `\x%02x`, c)
	}
	var xu strings.Builder
	for _, c := range v {
		fmt.Fprintf(&xu, `\x%02X`, c)
	}
	return [][]byte{[]byte(h), []byte(strings.ToUpper(h)), []byte(x.String()), []byte(xu.String())}
}

// b64Forms returns the base64 text a value produces when it starts at byte
// offset 0, 1 or 2 of a group, keeping only characters the value alone decides:
// unpadded interior, the partial final character, and the padded ending.
func b64Forms(v []byte, enc *base64.Encoding) [][]byte {
	var out [][]byte
	raw := enc.WithPadding(base64.NoPadding)
	for k := 0; k < 3; k++ {
		in := append(make([]byte, k), v...)
		c0 := (8*k + 5) / 6
		full := 8 * len(in) / 6
		nopad := raw.EncodeToString(in)
		out = append(out, []byte(nopad[c0:full]), []byte(nopad[c0:]), []byte(enc.EncodeToString(in)[c0:]))
	}
	return out
}
