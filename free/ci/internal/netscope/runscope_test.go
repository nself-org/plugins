package netscope

import (
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// RunScope (D-1331): an undeclared scope runs as none for every class (the class default never widens a job that
// declared nothing), and a declared scope gets the same refusal Effective makes.
func TestRunScope(t *testing.T) {
	for _, class := range []model.TrustClass{"owner", "internal", "collaborator", "untrusted", "", "bogus"} {
		if got, err := RunScope(class, ""); err != nil || got != "none" {
			t.Fatalf("RunScope(%q, \"\") = %q, %v; want none", class, got, err)
		}
	}
	for _, c := range []struct {
		class   model.TrustClass
		scope   model.NetworkScope
		refused bool
	}{
		{"untrusted", "lan", true}, {"untrusted", "privileged", true}, {"", "lan", true}, {"bogus", "privileged", true},
		{"owner", "nowhere", true},
		{"untrusted", "restricted", false}, {"untrusted", "internet", false}, {"untrusted", "none", false},
		{"owner", "lan", false}, {"internal", "privileged", false}, {"collaborator", "lan", false},
	} {
		got, err := RunScope(c.class, c.scope)
		var ne *Error
		if c.refused {
			if !errors.As(err, &ne) || ne.Code != CodeScopeFailed || got != "" {
				t.Fatalf("RunScope(%q, %q) = %q, %v; want E679", c.class, c.scope, got, err)
			}
			continue
		}
		if err != nil || got != c.scope {
			t.Fatalf("RunScope(%q, %q) = %q, %v", c.class, c.scope, got, err)
		}
	}
}
