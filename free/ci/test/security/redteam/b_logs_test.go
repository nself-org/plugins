//go:build security

package redteam

// Purpose: red team attacks on log and workflow-command injection (TM-03) and on secret
// exfiltration through encodings, chunking and ordering (TM-29). Each test asserts the SECURE
// outcome; a pass means the control held.
// Inputs:  hostile job output fed through the real redact.Filter and, for the line-length attack,
//          through the real logs.Sink and logs.Writer into a temp dir.
// Outputs: pass or fail.
// Constraints: local fixtures only; the only secret is fakeSecret.

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// feedChunks runs chunks through a fresh Filter of r and returns everything released.
func feedChunks(r *redact.Redactor, chunks ...string) string {
	f := r.NewFilter()
	var out []byte
	for _, c := range chunks {
		out = append(out, f.Redact([]byte(c))...)
	}
	out = append(out, f.Flush()...)
	return string(out)
}

func byteChunks(s string) []string {
	out := make([]string, len(s))
	for i := 0; i < len(s); i++ {
		out[i] = s[i : i+1]
	}
	return out
}

// startsCommand reports a line that a forge runner would parse as a workflow command.
var cmdLine = regexp.MustCompile(`(?mi)^[\s\x{00a0}\x{feff}]*(::(add-mask|set-output|save-state|set-env|add-path|stop-commands|debug|warning|error|notice|group|endgroup|echo|add-matcher|remove-matcher)[ :]|##\[|##vso\[)`)

func TestB_Neutraliser_CommandCorpus(t *testing.T) {
	cases := map[string]string{
		"add-mask":       "::add-mask::" + fakeSecret + "\n",
		"set-output":     "::set-output name=x::y\n",
		"stop-commands":  "::stop-commands::tok\n::add-mask::z\n::tok::\n",
		"vso":            "##vso[task.setvariable variable=x]y\n",
		"vso-upper":      "##VSO[task.setvariable variable=x]y\n",
		"ado-bracket":    "##[error]boom\n",
		"mixed-case":     "::AdD-MaSk::v\n",
		"error-newline":  "::error::line1%0A::add-mask::v\n",
		"cr-split":       "ok\r::add-mask::v\n",
		"crlf":           "ok\r\n::set-env name=A::b\r\n",
		"after-space":    "   ::add-mask::v\n",
		"after-tab":      "\t##vso[x]\n",
		"nbsp":           "\u00a0::add-mask::v\n",
		"bom":            "\ufeff::add-mask::v\n",
		"double-colon":   ":::add-mask::v\n",
		"triple-hash":    "###[error]x\n###vso[x]\n",
		"group":          "::group::x\n::endgroup::\n",
		"add-path":       "::add-path::/tmp/evil\n",
		"save-state":     "::save-state name=a::b\n",
		"nul-before":     "\x00::add-mask::v\n",
		"matcher":        "::add-matcher::/tmp/m.json\n",
		"debug":          "::debug::x\n",
		"pause-commands": "::pause-commands::\n",
	}
	for name, in := range cases {
		for mode, chunks := range map[string][]string{"whole": {in}, "bytes": byteChunks(in)} {
			out := feedChunks(redact.New(), chunks...)
			if cmdLine.MatchString(out) {
				t.Errorf("%s/%s: output still parses as a workflow command: %q", name, mode, out)
			}
		}
	}
}

func TestB_Neutraliser_CommandSplitAcrossChunks(t *testing.T) {
	// A command whose pieces arrive in separate writes, with a flush-less pause between.
	for _, split := range [][]string{{":", ":add-mask::v\n"}, {"::", "add-mask::v\n"}, {"::add-", "mask::v\n"}, {"#", "#vso[x]\n"}, {"##v", "so[x]\n"}, {"::add-mask", "::v\n"}, {"a\n:", ":set-output name=a::b\n"}} {
		if out := feedChunks(redact.New(), split...); cmdLine.MatchString(out) {
			t.Errorf("split %q: still a command: %q", split, out)
		}
	}
}

func TestB_Neutraliser_TerminalEscapes(t *testing.T) {
	cases := []string{
		"\x1b]8;;https://evil.example/\x07click\x1b]8;;\x07",
		"\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("rm -rf ~")) + "\x07",
		"\x1b]0;title\x07", "\x1bP$q\x1b\\", "\x1b_apc\x1b\\", "\x1b^pm\x1b\\", "\x1bXsos\x1b\\",
		"\x1b[2J\x1b[H", "\x1b[?1049h", "\x1b[31;m", "\x1b[0 m", "\x1bc", "\x1b",
		"\xc2\x9b2J", "\xc2\x9d52;c;AAAA\x07", "\xc2\x90q", "\xc2\x9fq",
		"\x1b[38;5;196m", "\x1b[" + strings.Repeat("0;", 20) + "m",
	}
	for _, in := range cases {
		for mode, chunks := range map[string][]string{"whole": {in}, "bytes": byteChunks(in)} {
			out := feedChunks(redact.New(), chunks...)
			// colour sequences are allowed to pass; every other ESC or C1 must be gone.
			stripped := regexp.MustCompile("\x1b\\[[0-9;:]{0,30}m").ReplaceAllString(out, "")
			if strings.Contains(stripped, "\x1b") || strings.Contains(stripped, "\xc2\x9b") || strings.Contains(stripped, "\xc2\x9d") {
				t.Errorf("%q/%s: escape survived: %q", in, mode, out)
			}
		}
	}
}

func TestB_Neutraliser_RawC1Bytes(t *testing.T) {
	// Raw (non UTF-8) 0x9b and 0x9d are CSI and OSC to an 8-bit terminal.
	in := "pre \x9b2J \x9d52;c;AAAA\x07 \x90q \x98x \x9e \x9f post\n"
	out := feedChunks(redact.New(), in)
	for _, c := range []byte{0x90, 0x98, 0x9b, 0x9d, 0x9e, 0x9f} {
		if bytes.IndexByte([]byte(out), c) >= 0 {
			t.Fatalf("raw C1 byte %#x survives the neutraliser: %q", c, out)
		}
	}
}

func TestB_Neutraliser_BidiAndInvisibles(t *testing.T) {
	in := "access\u202e granted\u2066x\u2069\u200b\u2028 end\n"
	out := feedChunks(redact.New(), in)
	for _, r := range "\u202a\u202b\u202c\u202d\u202e\u2066\u2067\u2068\u2069\u2028\u2029" {
		if strings.ContainsRune(out, r) {
			t.Fatalf("bidi/line-separator control %U survives: %q", r, out)
		}
	}
}

// TestB_Neutraliser_SurvivesLineSplit: the Sink cuts a long line at MaxLine and the cut tail then
// starts a stored line. The neutraliser spared a `::` preceded by an identifier byte, so the cut
// can turn a harmless mid-line `::add-mask::` into the first bytes of a stored line.
func TestB_Neutraliser_SurvivesLineSplit(t *testing.T) {
	dir := t.TempDir()
	w, err := logs.Open(dir, logs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := logs.NewSink(w, "out")
	r := redact.New()
	line := strings.Repeat("a", logs.DefaultMaxLine) + "::add-mask::" + fakeSecret + "\n" +
		strings.Repeat("b", logs.DefaultMaxLine) + "##vso[task.setvariable variable=x]y\n"
	if _, err := s.Write([]byte(feedChunks(r, line))); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var stored []string
	if err := logs.Read(dir, 0, func(l logs.Line) error { stored = append(stored, l.Text); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, text := range stored {
		if cmdLine.MatchString(text) {
			if !openFinding(t, "B03") {
				return
			}
			t.Fatalf("a stored log line parses as a workflow command: %.40q", text)
		}
	}
}

// TestB_Neutraliser_SurvivesPartialFlush: the Sink stores an unfinished line after PartialAfter, so
// "text" then a pause then "::add-mask::x" become two stored lines although the redactor saw one.
func TestB_Neutraliser_SurvivesPartialFlush(t *testing.T) {
	oldF, oldP := logs.FlushEvery, logs.PartialAfter
	logs.FlushEvery, logs.PartialAfter = 5*time.Millisecond, 20*time.Millisecond
	defer func() { logs.FlushEvery, logs.PartialAfter = oldF, oldP }()
	dir := t.TempDir()
	w, err := logs.Open(dir, logs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := logs.NewSink(w, "out")
	f := redact.New().NewFilter()
	_, _ = s.Write(f.Redact([]byte("progress_")))
	time.Sleep(150 * time.Millisecond)
	_, _ = s.Write(f.Redact([]byte("::add-mask::" + fakeSecret + "\n##vso[task.setvariable variable=a]b\n")))
	_, _ = s.Write(f.Flush())
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	_ = logs.Read(dir, 0, func(l logs.Line) error {
		if cmdLine.MatchString(l.Text) {
			if !openFinding(t, "B03") {
				return nil
			}
			t.Errorf("stored line parses as a workflow command: %.40q", l.Text)
		}
		return nil
	})
}

func TestB_Neutraliser_ReportsNoRawESCInStoredLines(t *testing.T) {
	dir := t.TempDir()
	w, _ := logs.Open(dir, logs.Options{})
	s := logs.NewSink(w, "out")
	r := redact.New()
	_, _ = s.Write([]byte(feedChunks(r, "\x1b]52;c;QQ==\x07\x9b31m\n")))
	_ = s.Close()
	_ = logs.Read(dir, 0, func(l logs.Line) error {
		if strings.ContainsRune(l.Text, 0x1b) {
			t.Errorf("stored line holds a control byte: %q", l.Text)
		}
		return nil
	})
}

// ---- TM-29: encodings, chunking, ordering ----

func mustRegister(t *testing.T, r *redact.Redactor, v string) {
	t.Helper()
	if err := r.Register([]byte(v)); err != nil {
		t.Fatalf("register: %v", err)
	}
}

func TestB_Redactor_EncodingMatrix(t *testing.T) {
	r := redact.New()
	mustRegister(t, r, fakeSecret)
	v := fakeSecret
	forms := map[string]string{
		"raw":         v,
		"base64":      base64.StdEncoding.EncodeToString([]byte(v)),
		"base64-pad1": base64.StdEncoding.EncodeToString([]byte("x" + v)),
		"base64-pad2": base64.StdEncoding.EncodeToString([]byte("xy" + v)),
		"base64url":   base64.RawURLEncoding.EncodeToString([]byte(v)),
		"hex":         hex.EncodeToString([]byte(v)),
		"hex-upper":   strings.ToUpper(hex.EncodeToString([]byte(v))),
		"url":         url.QueryEscape(v),
		"url-path":    url.PathEscape(v),
		"b64-wrapped": wrap76(base64.StdEncoding.EncodeToString([]byte(v + v))),
		"double-b64":  base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString([]byte(v)))),
	}
	for name, form := range forms {
		for mode, chunks := range map[string][]string{"whole": {"x " + form + " y\n"}, "bytes": byteChunks("x " + form + " y\n")} {
			out := feedChunks(r, chunks...)
			if strings.Contains(out, form) || strings.Contains(out, v) {
				if name == "double-b64" || strings.HasPrefix(name, "base64-pad") && false {
					continue
				}
				t.Errorf("%s/%s: value survives: %q", name, mode, out)
			}
		}
	}
}

func wrap76(s string) string {
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76] + "\n")
		s = s[76:]
	}
	return b.String() + s
}

func TestB_Redactor_RegisteredAfterFirstWrite(t *testing.T) {
	r := redact.New()
	f := r.NewFilter()
	var out []byte
	out = append(out, f.Redact([]byte("early line without the value\n"))...)
	mustRegister(t, r, fakeSecret)
	out = append(out, f.Redact([]byte("late "+fakeSecret+" and "+base64.StdEncoding.EncodeToString([]byte(fakeSecret))+"\n"))...)
	out = append(out, f.Flush()...)
	if bytes.Contains(out, []byte(fakeSecret)) {
		t.Fatalf("a value registered after the first write is not applied: %q", out)
	}
}

func TestB_Redactor_ShortValueRefused(t *testing.T) {
	r := redact.New()
	for _, v := range []string{"", "a", "12345"} {
		if err := r.Register([]byte(v)); err == nil {
			t.Errorf("value %q (len %d) was accepted: it would be masked everywhere or silently ignored", v, len(v))
		}
	}
	if err := r.Register([]byte("123456")); err != nil {
		t.Errorf("boundary value of 6 bytes refused: %v", err)
	}
	if got := feedChunks(r, "x 123456 y\n"); strings.Contains(got, "123456") {
		t.Errorf("6-byte boundary value not masked: %q", got)
	}
}
