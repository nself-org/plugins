package redact

// Regressions from the round 1 reviews (Codex and Sonnet CR) of P7-TRUST-19.

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRegressJSONShortEscapesAndURLPathForms(t *testing.T) {
	r := New()
	mustRegister(t, r, "abc\x08def\x0cghi", "a+b/c?d", "Zq8/xK+pL2mN9vB4=ab cd")
	for _, in := range []string{
		`abc\bdef\fghi`, `abc\u0008def\u000cghi`,
		"a+b%2Fc%3Fd", "a%2Bb/c%3Fd", "a+b%2fc%3fd",
		"Zq8/xK+pL2mN9vB4=ab%20cd",       // encodeURI
		"Zq8/xK%2BpL2mN9vB4%3Dab%20cd",   // python quote(safe='/')
		"Zq8%2FxK%2BpL2mN9vB4%3Dab%20cd", // encodeURIComponent-like
		"Zq8%2FxK%2BpL2mN9vB4%3Dab+cd",   // QueryEscape
		"Zq8%2fxK%2bpL2mN9vB4%3dab+cd",   // lower hex
		"Zq8%2FxK+pL2mN9vB4=ab%20cd",     // mixed
	} {
		got := run(r, "x "+in+" y")
		if got != "x [REDACTED] y" && !strings.HasPrefix(got, "x [REDACTED]") {
			t.Errorf("%q survived: %q", in, got)
		}
		if bw := strings.Join(bytewise(run(r, bytewise("x "+in+" y")...)), ""); collapse(bw) != collapse(got) {
			t.Errorf("%q chunking differs: %q vs %q", in, bw, got)
		}
	}
}

func TestRegressPEMCloseAndReplay(t *testing.T) {
	r := New()
	pb, pe := "-----BEGIN "+"KEY-----", "-----END "+"KEY-----"
	cases := []struct{ name, in string }{
		{"close then command", pb + "x" + pe + "::add-mask::foo"},
		{"replay", pe + "hello" + pb},
		{"token after close", pb + "x" + pe + "sk-" + strings.Repeat("a1B2", 8)},
	}
	for _, c := range cases {
		whole := run(r, c.in)
		if bw := run(r, bytewise(c.in)...); collapse(bw) != collapse(whole) {
			t.Errorf("%s: bytewise %q vs whole %q", c.name, bw, whole)
		}
		for i := 1; i < len(c.in); i++ {
			if got := run(r, c.in[:i], c.in[i:]); collapse(got) != collapse(whole) {
				t.Fatalf("%s: split %d %q vs %q", c.name, i, got, whole)
			}
		}
	}
	if got := run(r, pb+"x"+pe+"::add-mask::foo"); !strings.Contains(got, "[::]add-mask") {
		t.Errorf("command after END not neutralised: %q", got)
	}
	if got := run(r, pe+"hello"+pb); strings.Count(got, "hello") != 1 {
		t.Errorf("text replayed: %q", got)
	}
	if got := run(r, pb+"x"+pe+"sk-"+strings.Repeat("a1B2", 8)); strings.Contains(got, "sk-a1B2") {
		t.Errorf("token after END leaked: %q", got)
	}
}

func TestRegressBase64MatchCostIsLinear(t *testing.T) {
	r := New()
	n := 8000
	mustRegister(t, r, strings.Repeat("a", n))
	enc := b64Embedded("", strings.Repeat("a", 3*n), "")["b64-76"]
	start := time.Now()
	got := run(r, enc)
	if !strings.Contains(got, Placeholder) || time.Since(start) > 5*time.Second {
		t.Fatalf("took %v, out has mask=%v", time.Since(start), strings.Contains(got, Placeholder))
	}
}

func TestRegressBufferCapacityShrinks(t *testing.T) {
	r := New()
	mustRegister(t, r, strings.Repeat("q", 3000))
	f := r.NewFilter()
	f.Redact([]byte(strings.Repeat("!", 8<<20) + strings.Repeat("q", 2000)))
	if cap(f.buf) > 1<<20 {
		t.Fatalf("retained capacity %d after one large write", cap(f.buf))
	}
}

type failWriter struct{ err error }

func (w failWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRegressSinkErrorsDoNotEchoValues(t *testing.T) {
	r := New()
	mustRegister(t, r, "abcdef-secret")
	sink := errors.New("open /logs/abcdef-secret: disk full")
	for name, call := range map[string]func(io.WriteCloser) error{
		"write": func(w io.WriteCloser) error { _, err := w.Write([]byte("hello\n")); return err },
		"close": func(w io.WriteCloser) error { _, _ = w.Write([]byte("tail without newline")); return w.Close() },
	} {
		err := call(r.Wrap(failWriter{sink}))
		if err == nil || strings.Contains(err.Error(), "abcdef-secret") || !errors.Is(err, sink) {
			t.Errorf("%s: err=%v", name, err)
		}
	}
}

func TestRegressTokenGluedToPEMAndRepeatedPrefixes(t *testing.T) {
	r := New()
	pb, pe := "-----BEGIN "+"PRIVATE KEY-----", "-----END "+"PRIVATE KEY-----"
	in := "glpat" + "-" + strings.Repeat("Zz_8", 6) + pb + "\nSECRETBODYSECRETBODY\n" + pe + "\n"
	if got := run(r, in); strings.Contains(got, "SECRETBODY") {
		t.Errorf("PEM body leaked: %q", got)
	}
	start := time.Now()
	got := run(r, strings.Repeat("eyJ", 1<<18))
	if len(got) != 3<<18 || time.Since(start) > 3*time.Second {
		t.Errorf("repeated eyJ: %d bytes in %v", len(got), time.Since(start))
	}
}

func TestRegressCommandCaseAndTripleHash(t *testing.T) {
	r := New()
	for in, want := range map[string]string{
		"::ADD-MASK::x\n":  "[::]ADD-MASK::x\n",
		"::Set-Output a\n": "[::]Set-Output a\n",
		"###vso[a]b\n":     "#[##]vso[a]b\n",
	} {
		if got := run(r, in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
		if got := run(r, bytewise(in)...); got != want {
			t.Errorf("bytewise %q: got %q", in, got)
		}
	}
}

func TestRegressFoldedRawValue(t *testing.T) {
	r := New()
	v := "Zq8xKpL2mN9vB4ab-cd-ef-gh-ij-kl-mn"
	mustRegister(t, r, v)
	for _, nl := range []string{"\n", "\r\n"} {
		in := v[:12] + nl + v[12:24] + nl + v[24:]
		if got := run(r, "p "+in+" q"); strings.Contains(strings.ReplaceAll(strings.ReplaceAll(got, "\r", ""), "\n", ""), v[:12]) {
			t.Errorf("folded value survived: %q", got)
		}
	}
	var _ = bytes.MinRead
}

func TestRegressPEMHeaderDashesAreNotAnEndMarker(t *testing.T) {
	r := New()
	in := "-----BEGIN 0-----END " + strings.Repeat("0", 18) + "-----" + strings.Repeat("0", 33)
	whole := run(r, in)
	if bw := run(r, bytewise(in)...); collapse(bw) != collapse(whole) {
		t.Fatalf("bytewise %q vs whole %q", bw, whole)
	}
}

// Round 2 (Codex) regressions.

func TestRegressRound2Forms(t *testing.T) {
	r := New()
	mustRegister(t, r, "Abcd+!", "ab/é12")
	for _, in := range []string{"Abcd+%21", "Abcd%2B%21", `ab\/é12`, `ab\/é12`, `ab/é12`, `ab\/é12`} {
		if got := run(r, "x "+in+" y"); got != "x [REDACTED] y" {
			t.Errorf("%q survived: %q", in, got)
		}
		if got := run(r, bytewise("x "+in+" y")...); got != "x [REDACTED] y" {
			t.Errorf("%q bytewise: %q", in, got)
		}
	}
}

func TestRegressShorterFoldedSuffixAfterCapRejection(t *testing.T) {
	r := New()
	mustRegister(t, r, "zzzzzzabcdefQ", "abcdefghi")
	chunks := []string{"z\nz\nz\nz\nz\nzabc\ndef", "ghi"}
	whole := run(r, strings.Join(chunks, ""))
	split := run(r, chunks...)
	if strings.Contains(whole, "ghi") || collapse(whole) != collapse(split) {
		t.Fatalf("whole %q split %q", whole, split)
	}
}

func TestRegressPartialPEMOpenerAfterMaxToken(t *testing.T) {
	r := New()
	pb, pe := "-----BEGIN "+"PRIVATE KEY-----", "-----END "+"PRIVATE KEY-----"
	for _, n := range []int{240, 249, 250, 251, 255, 300} {
		tok := "glpat" + "-" + strings.Repeat("a", n)
		first, second := tok+"-----BEGIN", " PRIVATE KEY-----\nSYNTHETIC_PRIVATE_BODY\n"+pe+"\n"
		whole := run(r, first+second)
		if split := run(r, first, second); collapse(split) != collapse(whole) || strings.Contains(split, "SYNTHETIC") {
			t.Errorf("n=%d whole %q split %q", n, whole, split)
		}
	}
	_ = pb
}

func TestRegressJWTAfterPunctuationAndLinearity(t *testing.T) {
	r := New()
	e := func(s string) string {
		return strings.TrimRight(b64Embedded("", s, "")["b64url-raw"], "=")
	}
	jwt := e(`{"alg":"none","typ":"fake"}`) + "." + e(`{"sub":"nobody","fake":true}`) + "." + strings.Repeat("a", 43)
	for _, pre := range []string{"-", "_", ".x-", "token="} {
		if got := run(r, pre+jwt+" z"); !strings.Contains(got, Placeholder) || strings.Contains(got, jwt[10:]) {
			t.Errorf("pre %q: %q", pre, got)
		}
	}
	start := time.Now()
	for _, unit := range []string{"eyJ-", "eyJ.", "eyJ_x"} {
		_ = run(r, strings.Repeat(unit, 1<<18))
	}
	if d := time.Since(start); d > 8*time.Second {
		t.Fatalf("repeated JWT prefixes took %v", d)
	}
}

// Round 3 (Codex + CR) regressions.

func TestRegressSkipDoesNotHideStraddlingPrefix(t *testing.T) {
	r := New()
	body := strings.Repeat("aB3x", 6)
	for _, pre := range []string{"ghp" + "_", "whsec" + "_", "Bearer ", "sk" + "_live_"} {
		in := "x " + pre + pre + body + " z"
		whole := run(r, in)
		if strings.Contains(whole, body) {
			t.Errorf("%q survived: %q", pre, whole)
		}
		if bw := run(r, bytewise(in)...); collapse(bw) != collapse(whole) || strings.Contains(bw, body) {
			t.Errorf("%q bytewise %q vs whole %q", pre, bw, whole)
		}
	}
}

func TestRegressJWTGluedToPEMOpener(t *testing.T) {
	r := New()
	e := func(s string) string {
		return strings.TrimRight(b64Embedded("", s, "")["b64url-raw"], "=")
	}
	jwt := e(`{"alg":"none","typ":"fake"}`) + "." + e(`{"sub":"nobody","fake":true}`) + "." + strings.Repeat("a", 43)
	in := jwt + "-----BEGIN " + "PRIVATE KEY-----\nSYNTHETIC_PRIVATE_BODY\n-----END " + "PRIVATE KEY-----\n"
	whole := run(r, in)
	if strings.Contains(whole, "SYNTHETIC") || strings.Contains(whole, "PRIVATE KEY") {
		t.Fatalf("whole %q", whole)
	}
	if bw := run(r, bytewise(in)...); collapse(bw) != collapse(whole) {
		t.Fatalf("bytewise %q vs whole %q", bw, whole)
	}
}

func TestRegressVSOCommandCaseAndSinkErrorUnwrap(t *testing.T) {
	r := New()
	for _, in := range []string{"##VSO[task.setvariable variable=x]y\n", "##Vso[task.setvariable variable=x]y\n"} {
		if got := run(r, in); strings.Contains(got, "##V") {
			t.Errorf("%q passed through: %q", in, got)
		}
		if got := run(r, bytewise(in)...); strings.Contains(got, "##V") {
			t.Errorf("%q bytewise: %q", in, got)
		}
	}
	mustRegister(t, r, "abcdef-secret")
	sink := errors.New("open /logs/abcdef-secret: disk full")
	_, err := r.Wrap(failWriter{sink}).Write([]byte("hello\n"))
	if !errors.Is(err, sink) {
		t.Fatal("errors.Is lost the sentinel")
	}
	for e := error(err); e != nil; e = errors.Unwrap(e) {
		if strings.Contains(e.Error(), "abcdef-secret") {
			t.Fatalf("chain exposes the value: %q", e.Error())
		}
	}
}

func TestRegressURLFormsBoundedForPunctuationHeavyValues(t *testing.T) {
	v := []byte("SYNTH042!$&,/?#*" + strings.Repeat("a", 600))
	if n := len(urlForms(v)); n > 60 {
		t.Fatalf("%d url forms for one value", n)
	}
	r := New()
	mustRegister(t, r, string(v), "Zq8/xK+pL2mN9vB4=ab cd")
	for _, in := range []string{"Zq8/xK%2BpL2mN9vB4%3Dab%20cd", "Zq8%2FxK%2BpL2mN9vB4%3Dab+cd", "Zq8/xK+pL2mN9vB4=ab%20cd"} {
		if got := run(r, "x="+in+"&"); got != "x=[REDACTED]&" {
			t.Errorf("%q survived: %q", in, got)
		}
	}
}

// Round 4: encoders the first unions missed.
func TestRegressURLEncoderCoverage(t *testing.T) {
	r := New()
	mustRegister(t, r, "abcdef!$/#", "abcdef!$,/", "Zq8~xK2mN9vB4!", "a!b+c,d/e?f#mn", "ab*cd~ef!gh")
	for _, in := range []string{"abcdef!$/%23", "abcdef%21$,%2F", "Zq8%7ExK2mN9vB4%21", "Zq8%7ExK2mN9vB4!", "a!b+c,d/e?f%23mn", "ab*cd%7Eef%21gh"} {
		if got := run(r, "x="+in+"&"); got != "x=[REDACTED]&" {
			t.Errorf("%q survived: %q", in, got)
		}
		if got := run(r, bytewise("x="+in+"&")...); got != "x=[REDACTED]&" {
			t.Errorf("%q bytewise: %q", in, got)
		}
	}
}

func TestRegressCappedJWTDoesNotSwallowPEMOpener(t *testing.T) {
	r := New()
	pe := "-----END " + "PRIVATE KEY-----"
	for _, n := range []int{8170, 8175, 8178, 8180, 8182, 8184, 8186} {
		in := "eyJ" + strings.Repeat("a", n) + ".b.c-----BEGIN " + "PRIVATE KEY-----\nSYNTHETIC_PRIVATE_BODY\n" + pe + "\n"
		whole := run(r, in)
		if strings.Contains(whole, "SYNTHETIC") {
			t.Errorf("n=%d whole leaked: %.80q", n, whole)
		}
		if bw := run(r, bytewise(in)...); strings.Contains(bw, "SYNTHETIC") || collapse(bw) != collapse(whole) {
			t.Errorf("n=%d bytewise %.80q vs whole %.80q", n, bw, whole)
		}
	}
}

// Round 5: per-character safe sets, checked against the real Go encoders.
func TestRegressGoStdlibEncoders(t *testing.T) {
	for _, v := range []string{"abcdef!'$/#", "abcdef!$,:/", "abcdef!$,@/", "abcdef!$/#", "abcdef!$,/", "q8 !'()*$&+=:@,;/?#~z"} {
		r := New()
		mustRegister(t, r, v)
		encoded := map[string]string{
			"fragment": (&url.URL{Fragment: v}).EscapedFragment(),
			"userinfo": url.User(v).String(),
			"path":     (&url.URL{Path: v}).EscapedPath(),
			"pathseg":  url.PathEscape(v),
			"query":    url.QueryEscape(v),
		}
		for name, e := range encoded {
			if got := run(r, "x "+e+" y"); got != "x [REDACTED] y" {
				t.Errorf("%s of %q (%q) survived: %q", name, v, e, got)
			}
			if got := run(r, bytewise("x "+e+" y")...); got != "x [REDACTED] y" {
				t.Errorf("%s of %q bytewise: %q", name, v, got)
			}
		}
	}
}

func TestRegressURLFormsBudgetForManyValues(t *testing.T) {
	r := New()
	for i := 0; i < 64; i++ {
		prefix := fmt.Sprintf("CO5%03d!*$,/?#~ ", i)
		mustRegister(t, r, prefix+strings.Repeat("x", 497))
	}
	if n := len(urlForms([]byte("CO5000!*$,/?#~ " + strings.Repeat("x", 497)))); n > 120 {
		t.Fatalf("%d url forms for one 512-byte value", n)
	}
}

func TestRegressJWTEndingInDashesAtEOF(t *testing.T) {
	r := New()
	jwt := "eyJhbGciOiJIUzI1NiJ9.e30K.aa--"
	if got := run(r, jwt); got != Placeholder {
		t.Fatalf("EOF: %q", got)
	}
	if got := run(r, jwt+"!"); got != Placeholder+"!" {
		t.Fatalf("delimited: %q", got)
	}
	if got := run(r, bytewise(jwt)...); got != Placeholder {
		t.Fatalf("bytewise: %q", got)
	}
}

// Round 5 fuzz: a held partial must survive as long as a longer pattern could
// still accept its line breaks.
func TestRegressFoldedHoldUsesLongestPatternCap(t *testing.T) {
	r := New()
	mustRegister(t, r, "fuzz-secret-value-1")
	in := "ZnV6ei1zZWNyZX\r\r\r\r\rQtdmFsdWUtM"
	whole := run(r, in)
	if whole != Placeholder {
		t.Fatalf("whole %q", whole)
	}
	if bw := run(r, bytewise(in)...); bw != whole {
		t.Fatalf("bytewise %q vs whole %q", bw, whole)
	}
	for cut := 0; cut <= len(in); cut++ {
		if got := run(r, in[:cut], in[cut:]); got != whole {
			t.Fatalf("cut %d: %q", cut, got)
		}
	}
}

// Round 6 (Opus adversarial, Codex): values that hold a PEM block, more encoders.
func TestRegressValueContainingPEMBlockSplitInsideBody(t *testing.T) {
	pb, pe := "-----BEGIN "+"PRIVATE KEY-----", "-----END "+"PRIVATE KEY-----"
	json := `{"type":"service_account","private_key_id":"0f1e2d3c4b5a","private_key":"` + pb + `\nSYNTHETICBODYSYNTHETICBODY\n` + pe + `\n","client_email":"svc@demo.example.test"}`
	cases := []string{json, "k" + "-----BEGIN Z-----q", "-----END " + "A-----SECRETTAIL123"}
	for _, v := range cases {
		r := New()
		mustRegister(t, r, v)
		in := "creds=" + v + " after"
		whole := run(r, in)
		for cut := 0; cut <= len(in); cut++ {
			got := run(r, in[:cut], in[cut:])
			if strings.Contains(got, "SYNTHETICBODY") || strings.Contains(got, "svc@demo") || strings.Contains(got, "SECRETTAIL") || collapse(got) != collapse(whole) {
				t.Fatalf("value %.30q cut %d: %q vs whole %q", v, cut, got, whole)
			}
		}
	}
}

func TestRegressFilterReusableAfterFlushInsidePEM(t *testing.T) {
	f := New().NewFilter()
	_ = f.Redact([]byte("-----BEGIN " + "A-----\nabc"))
	_ = f.Flush()
	if got := string(f.Redact([]byte("hello world after flush\n"))) + string(f.Flush()); got != "hello world after flush\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRegressMoreEncoderForms(t *testing.T) {
	r := New()
	mustRegister(t, r, "pa+ss&wo<rd'99Z", "Jabcdef9zK", "it's-a-long-secret-99", "secret?>>?>value")
	b64 := base64.StdEncoding.EncodeToString([]byte("secret?>>?>value"))
	for _, in := range []string{
		`pa+ss&wo<rd'99Z`,
		`\x4A\x61\x62\x63\x64\x65\x66\x39\x7A\x4B`,
		`'it'\''s-a-long-secret-99'`, `it\'s-a-long-secret-99`,
		"q=" + strings.NewReplacer("+", "%2B", "/", "%2F", "=", "%3D").Replace(b64),
		"j=" + strings.ReplaceAll(b64, "/", `\/`),
	} {
		if got := run(r, "x "+in+" y"); strings.Contains(got, "x "+in) || !strings.Contains(got, Placeholder) || strings.Contains(got, "secret99") {
			t.Errorf("%q survived: %q", in, got)
		}
	}
}

func TestRegressNamedURLSetsSurviveTheBudget(t *testing.T) {
	v := "abcdef!*$/,:" + strings.Repeat("x", 2329)
	r := New()
	mustRegister(t, r, v)
	if got := run(r, url.User(v).String()); got != Placeholder {
		t.Fatalf("userinfo form survived: %.60q", got)
	}
}

// Round 7.
func TestRegressDotNetQuoteAndANSICAndBarePEMTail(t *testing.T) {
	r := New()
	mustRegister(t, r, `ab"cd+ef`, "ab\x01cd+ef", "-----BEGIN "+"A-----body-----END "+"A-----SECRETTAIL123")
	for _, in := range []string{`ab"cd+ef`, `$'ab\001cd+ef'`} {
		if got := run(r, "x "+in+" y"); strings.Contains(got, "cd") || !strings.Contains(got, Placeholder) {
			t.Errorf("%q survived: %q", in, got)
		}
	}
	chunks := []string{"-----BEGIN " + "A-----", "body-----END " + "A-----SECRETTAIL123"}
	whole := run(r, chunks[0]+chunks[1])
	if split := run(r, chunks...); strings.Contains(split, "SECRETTAIL") || collapse(split) != collapse(whole) {
		t.Errorf("split %q vs whole %q", split, whole)
	}
}

// Round 8: bash $'...' named control escapes, checked against the real bash.
func TestRegressBashQuotingMatchesBash(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("bash is required: %v", err)
	}
	for _, c := range []byte{1, 7, 8, 9, 10, 11, 12, 13, 27, 127} {
		v := "ab" + string([]byte{c}) + "cd+ef'g\\h"
		out, err := exec.Command(bash, "-c", `printf %q "$1"`, "x", v).Output()
		if err != nil {
			t.Fatal(err)
		}
		r := New()
		mustRegister(t, r, v)
		if got := run(r, "x "+string(out)+" y"); strings.Contains(got, "cd") || !strings.Contains(got, Placeholder) {
			t.Errorf("control %d: %q printed as %q survived: %q", c, v, out, got)
		}
	}
}

// Round 9: Go %q / Python repr with control bytes, bash under the C locale with UTF-8.
func TestRegressQuotedControlAndCLocale(t *testing.T) {
	r := New()
	mustRegister(t, r, "ab\x01cd+ef", "abé'cd+ef")
	for _, in := range []string{`"ab\x01cd+ef"`, `$'ab\303\251\'cd+ef'`} {
		if got := run(r, "x "+in+" y"); strings.Contains(got, "cd") || !strings.Contains(got, Placeholder) {
			t.Errorf("%q survived: %q", in, got)
		}
	}
	if got := run(r, "x "+fmt.Sprintf("%q", "ab\x01cd+ef")+" y"); strings.Contains(got, "cd") {
		t.Errorf("Go %%q survived: %q", got)
	}
}
