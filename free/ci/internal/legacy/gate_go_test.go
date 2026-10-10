// Regression tests for the Go gate on a repo with a checked-in vendor/ tree
// and one slow package (D-0286): gofmt listed 22 vendored files and go test
// panicked at the 120 s step timeout in cli cmd/commands, so the gate was red
// on a clean origin/main.
package legacy

import "testing"

func TestDropVendored(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"only vendor", "vendor/github.com/google/uuid/dce.go\nvendor/gopkg.in/yaml.v3/apic.go", ""},
		{"mixed", "vendor/a/b.go\ncmd/main.go\ninternal/x/vendor/c.go\ninternal/y.go", "cmd/main.go\ninternal/y.go"},
		{"dot prefix", "./vendor/a.go\n./cmd/b.go", "./cmd/b.go"},
		{"parse error in vendor", "vendor/a.go:1:1: expected 'package'", ""},
		{"first-party error mentioning vendor kept", "cmd/a.go:3:1: import \"x/vendor/y\" invalid", "cmd/a.go:3:1: import \"x/vendor/y\" invalid"},
		{"vendor-like name kept", "internal/vendored/a.go\nvendorlib/b.go", "internal/vendored/a.go\nvendorlib/b.go"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dropVendored(c.in); got != c.want {
				t.Fatalf("dropVendored(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestGoTestTimeout(t *testing.T) {
	cases := []struct{ step, want int }{
		{0, goTestMinTimeout},
		{5, goTestMinTimeout},
		{120, goTestMinTimeout},
		{600, 600},
		{1800, 1800},
	}
	for _, c := range cases {
		if got := goTestTimeout(c.step); got != c.want {
			t.Errorf("goTestTimeout(%d) = %d, want %d", c.step, got, c.want)
		}
	}
}
