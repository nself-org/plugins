package redact

import (
	"strings"
	"testing"
)

func TestWorkflowCommandsNeutralised(t *testing.T) {
	r := New()
	cases := map[string]string{
		"::add-mask::abcdef\n":                    "[::]add-mask::abcdef\n",
		"  ::set-output name=x::y\n":              "  [::]set-output name=x::y\n",
		"::stop-commands::tok\n":                  "[::]stop-commands::tok\n",
		"a ::save-state name=k::v\n":              "a [::]save-state name=k::v\n",
		"::error file=a.go::boom\n":               "[::]error file=a.go::boom\n",
		"##vso[task.setvariable variable=a]b\n":   "[##]vso[task.setvariable variable=a]b\n",
		"##[group]title\n":                        "[##][group]title\n",
		"std::error::Error and std::fmt::Debug\n": "std::error::Error and std::fmt::Debug\n",
		"::unknown-thing::x\n":                    "::unknown-thing::x\n",
		"## heading and ### more\n":               "## heading and ### more\n",
	}
	for in, want := range cases {
		if got := run(r, in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
		if got := run(r, bytewise(in)...); got != want {
			t.Errorf("bytewise %q: got %q want %q", in, got, want)
		}
		for i := 1; i < len(in); i++ {
			if got := run(r, in[:i], in[i:]); got != want {
				t.Fatalf("%q split %d: got %q want %q", in, i, got, want)
			}
		}
	}
}

func TestEscapeSequencesNeutralised(t *testing.T) {
	r := New()
	keep := "\x1b[31mred\x1b[0m \x1b[1;38;5;12mx\x1b[m\n"
	if got := run(r, keep); got != keep {
		t.Fatalf("colour changed: %q", got)
	}
	cases := map[string]string{
		"\x1b]8;;http://example.test\x07link\x1b]8;;\x07": "^[]8;;http://example.test\x07link^[]8;;\x07",
		"\x1b]52;c;Zm9v\x07":                              "^[]52;c;Zm9v\x07",
		"\x1b]0;title\x1b\\":                              "^[]0;title^[\\",
		"\x1b[2Jclear\x1b[H":                              "^[[2Jclear^[[H",
		"\x1b[?25l":                                       "^[[?25l",
		"\x1bPdcs\x1b\\":                                  "^[Pdcs^[\\",
		"a\u009b2Jb":                                      "a^[2Jb",
		"price £ 5":                                       "price £ 5",
	}
	for in, want := range cases {
		if got := run(r, in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
		for i := 1; i < len(in); i++ {
			if got := run(r, in[:i], in[i:]); got != want {
				t.Fatalf("%q split %d: got %q want %q", in, i, got, want)
			}
		}
		if got := run(r, bytewise(in)...); got != want {
			t.Errorf("bytewise %q: got %q", in, got)
		}
	}
	if got := run(r, "tail\x1b"); got != "tail^[" {
		t.Errorf("lone ESC at end: %q", got)
	}
	if got := run(r, "x\x1b[1;"); got != "x^[[1;" {
		t.Errorf("incomplete CSI at end: %q", got)
	}
	if got := run(r, "x\x1b["+strings.Repeat("1;", 30)+"m"); !strings.HasPrefix(got, "x^[[") {
		t.Errorf("over-long SGR must be neutralised: %q", got)
	}
}

func TestMaskBeatsNeutralise(t *testing.T) {
	r := New()
	mustRegister(t, r, "::add-mask::secretvalue")
	if got := run(r, "x ::add-mask::secretvalue y\n"); got != "x [REDACTED] y\n" {
		t.Fatalf("got %q", got)
	}
}
