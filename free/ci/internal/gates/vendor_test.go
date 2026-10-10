package gates

import "testing"

// TestDropVendored pins the go.fmt vendor skip (D-0286): vendored modules
// must not fail the gate, first-party files must still be reported.
func TestDropVendored(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"only vendor", "vendor/github.com/google/uuid/dce.go\n", ""},
		{"mixed", "vendor/a/b.go\ncmd/main.go\nsub/vendor/c.go\ninternal/y.go", "cmd/main.go\ninternal/y.go"},
		{"vendored parse error", "vendor/a.go:1:1: expected 'package'", ""},
		{"first-party error mentioning vendor kept", "cmd/a.go:3:1: see /vendor/x", "cmd/a.go:3:1: see /vendor/x"},
		{"vendor-like name kept", "internal/vendored/a.go", "internal/vendored/a.go"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dropVendored(c.in); got != c.want {
				t.Fatalf("dropVendored(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
