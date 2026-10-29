package provision

import (
	"bytes"
	"strings"
	"testing"
)

// HelpOnly answers like core `nself runner provision|verify --help`: the captured text on a help request,
// the first parse error when a flag before the help token is bad, and help (never a run) when the help
// token is taken as a flag's value.
func TestHelpOnlyMatchesCore(t *testing.T) {
	for _, name := range []string{"provision", "verify"} {
		want, ok := Help(name)
		if !ok || want == "" {
			t.Fatalf("no captured help for %s", name)
		}
		for _, tc := range []struct {
			args    []string
			code    int
			out, er string
		}{
			{[]string{"--help"}, 0, want, ""},
			{[]string{"-h"}, 0, want, ""},
			{[]string{"--bogus", "--help"}, 1, "", "Error: unknown flag: --bogus\n"},
			{[]string{"--ssh-key", "--help"}, 0, want, ""},
		} {
			var out, er bytes.Buffer
			code := HelpOnly(name, tc.args, Deps{Out: &out, Err: &er})
			if code != tc.code || out.String() != tc.out || er.String() != tc.er {
				t.Errorf("%s %s: exit %d stdout %q stderr %q; want %d %q %q", name, strings.Join(tc.args, " "),
					code, out.String(), er.String(), tc.code, tc.out, tc.er)
			}
		}
	}
}
