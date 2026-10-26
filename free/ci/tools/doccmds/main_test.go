package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunFixturePages(t *testing.T) {
	cases := []struct {
		page string
		code int
		out  string
	}{
		{"pass.md", 0, ""},
		{"fail.md", 1, "testdata/fail.md: block 2: exit 3\ntestdata/fail.md: block 3: exit 7\n"},
		{"expect-fail.md", 1, "testdata/expect-fail.md: block 2: exit 0 (expected non-zero: marked expect-fail)\ntestdata/expect-fail.md: block 3: exit 5\n"},
		{"no-blocks.md", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.page, func(t *testing.T) {
			r := &fakeRunner{codes: map[string]int{
				"exit 1": 1, "exit 3": 3, "exit 5": 5, "exit 7": 7,
			}}
			fx := &fakeFixture{}
			var out, errOut bytes.Buffer
			code := runWithOptions([]string{"testdata/" + tc.page}, &out, &errOut,
				Options{Runner: r, Fixture: fx.fixture})
			if code != tc.code || out.String() != tc.out || errOut.Len() != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q; want code=%d stdout=%q stderr empty",
					code, out.String(), errOut.String(), tc.code, tc.out)
			}
			if fx.made != len(r.calls) || fx.cleaned != fx.made {
				t.Errorf("calls=%d fixtures=%d cleanups=%d", len(r.calls), fx.made, fx.cleaned)
			}
		})
	}
}

func TestPagePath(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "caller")
	abs := filepath.Join(base, "page.md")
	for _, tc := range []struct{ base, page, want string }{
		{base, "page.md", abs},
		{base, abs, abs},
		{"", "page.md", "page.md"},
	} {
		if got := pagePath(tc.base, tc.page); got != tc.want {
			t.Errorf("pagePath(%q, %q) = %q; want %q", tc.base, tc.page, got, tc.want)
		}
	}
}

func TestWrapperExecutable(t *testing.T) {
	info, err := os.Stat("../../../../.github/scripts/check-doc-commands.sh")
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0755 {
		t.Errorf("wrapper mode = %04o; want 0755", got)
	}
}
