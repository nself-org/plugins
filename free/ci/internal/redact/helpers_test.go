package redact

// Purpose: shared test helpers. Forms are built with the standard library, not
// with encodings.go, so the golden tests check the generator independently.

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// run feeds chunks through one Filter and returns everything it released.
func run(r *Redactor, chunks ...string) string {
	f := r.NewFilter()
	var out []byte
	for _, c := range chunks {
		out = append(out, f.Redact([]byte(c))...)
	}
	return string(append(out, f.Flush()...))
}

func bytewise(s string) []string {
	out := make([]string, len(s))
	for i := 0; i < len(s); i++ {
		out[i] = s[i : i+1]
	}
	return out
}

var multiMask = regexp.MustCompile(`(\[REDACTED\])+`)

// collapse joins adjacent masks: chunking may print two touching masks as one.
func collapse(s string) string { return multiMask.ReplaceAllString(s, Placeholder) }

func mustRegister(t testing.TB, r *Redactor, vals ...string) {
	t.Helper()
	for _, v := range vals {
		if err := r.Register([]byte(v)); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
}

var lowerPct = regexp.MustCompile(`%[0-9A-F]{2}`)

func lowerHexPct(s string) string {
	return lowerPct.ReplaceAllStringFunc(s, strings.ToLower)
}

func jsonInnerStd(v string, html bool) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(html)
	_ = e.Encode(v)
	s := strings.TrimSpace(b.String())
	return s[1 : len(s)-1]
}

func uAll(v string, upper bool) string {
	var b strings.Builder
	for _, r := range v {
		if upper {
			fmt.Fprintf(&b, `\u%04X`, r)
		} else {
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	return b.String()
}

// plainForms are the non-base64 encodings of v as an encoder would print them.
func plainForms(v string) map[string]string {
	hx := hex.EncodeToString([]byte(v))
	var xs strings.Builder
	for i := 0; i < len(v); i++ {
		fmt.Fprintf(&xs, `\x%02x`, v[i])
	}
	return map[string]string{
		"raw":        v,
		"url":        url.QueryEscape(v),
		"url-lower":  lowerHexPct(url.QueryEscape(v)),
		"url-path":   strings.ReplaceAll(url.QueryEscape(v), "+", "%20"),
		"json":       jsonInnerStd(v, true),
		"json-plain": jsonInnerStd(v, false),
		"u-lower":    uAll(v, false),
		"u-upper":    uAll(v, true),
		"hex":        hx,
		"hex-upper":  strings.ToUpper(hx),
		"xesc":       xs.String(),
	}
}

// wrapAt breaks s every n characters with nl.
func wrapAt(s string, n int, nl string) string {
	var b strings.Builder
	for len(s) > n {
		b.WriteString(s[:n] + nl)
		s = s[n:]
	}
	return b.String() + s
}

// b64Forms embeds v between prefix and suffix and encodes the whole.
func b64Embedded(prefix, v, suffix string) map[string]string {
	in := []byte(prefix + v + suffix)
	std := base64.StdEncoding.EncodeToString(in)
	return map[string]string{
		"b64":        std,
		"b64-raw":    base64.RawStdEncoding.EncodeToString(in),
		"b64url":     base64.URLEncoding.EncodeToString(in),
		"b64url-raw": base64.RawURLEncoding.EncodeToString(in),
		"b64-64":     wrapAt(std, 64, "\n"),
		"b64-76":     wrapAt(std, 76, "\r\n"),
	}
}

// leak is what survives of an encoded form between the fixed margins.
func leak(out, before, after string) (string, bool) {
	if !strings.HasPrefix(out, before) || !strings.HasSuffix(out, after) || len(out) < len(before)+len(after) {
		return "", false
	}
	mid := out[len(before) : len(out)-len(after)]
	return strings.ReplaceAll(mid, Placeholder, ""), strings.Contains(mid, Placeholder)
}
