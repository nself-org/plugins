package jsonout

// jsonout_test.go: hasJSONArg decides whether a run is in JSON mode before any
// command ran, so it must read --json the way pflag does.
//
// Inputs: argument vectors. Outputs: test results. No network, no disk.

import "testing"

func TestHasJSONArg(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"none", []string{"monitors", "list"}, false},
		{"bare", []string{"monitors", "list", "--json"}, true},
		{"true value", []string{"monitors", "list", "--json=true"}, true},
		{"false value", []string{"monitors", "list", "--json=false", "--bogus"}, false},
		{"false then bare", []string{"--json=false", "--json"}, true},
		{"bare then false", []string{"--json", "--json=false"}, false},
		{"after double dash", []string{"monitors", "list", "--bogus", "--", "--json"}, false},
		{"before double dash", []string{"--json", "--", "x"}, true},
		{"unparsable value keeps prior", []string{"--json", "--json=maybe"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasJSONArg(c.args); got != c.want {
				t.Fatalf("hasJSONArg(%q) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}
