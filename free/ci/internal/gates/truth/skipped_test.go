package truth

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func utf16le(s string) string {
	u := utf16.Encode([]rune(s))
	b := []byte{0xFF, 0xFE}
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return string(b)
}

// TestDeadHostInSkippedFiles: a binary, UTF-16 or oversize file is still read
// for dead hosts (D-1239); other kinds stay skipped for such files.
func TestDeadHostInSkippedFiles(t *testing.T) {
	big := strings.Repeat("x\n", maxFileBytes/2+10) + "ssh " + plantedHost() + "\n"
	res, _ := scanFixture(t, map[string]string{
		"bin.dat":     "\x00\x01 connect " + plantedHost() + " now\x00 SEKRET_MARKER",
		"nobom.dat":   utf16le("ssh " + plantedName() + "\n")[2:],
		"wide.txt":    utf16le("ssh " + plantedName() + "\nSEKRET_MARKER\n"),
		"huge.log":    big,
		"clean.dat":   "\x00 nothing here \x00",
		"ZeroW/a.txt": "ssh 203​.0.113.7 now\n",
	}, fixtureRules())
	got := map[string]int{}
	for _, f := range res.Findings {
		got[f.Path] = f.Line
		if f.Kind != KindDeadHost || f.Match != "" {
			t.Errorf("only dead-host findings, with no matched text: %+v", f)
		}
	}
	if len(got) != 5 || got["bin.dat"] == 0 || got["wide.txt"] != 1 || got["nobom.dat"] != 1 || got["huge.log"] != maxFileBytes/2+11 || got["ZeroW/a.txt"] != 1 {
		t.Fatalf("findings = %+v", res.Findings)
	}
	if res.Skipped != 5 {
		t.Errorf("skipped = %d, want 5 (still counted)", res.Skipped)
	}
}

// TestGlobIsCaseInsensitive: README.MD and A.Md are in scope of a *.md rule.
func TestGlobIsCaseInsensitive(t *testing.T) {
	for _, c := range []struct{ pat, rel string }{{"**/*.md", "docs/A.MD"}, {"*.md", "README.Md"}, {"docs/**", "DOCS/x.md"}} {
		if !matchGlob(c.pat, c.rel) {
			t.Errorf("matchGlob(%q, %q) = false", c.pat, c.rel)
		}
	}
}
