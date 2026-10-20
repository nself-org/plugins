package main

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestNetscopeProbePrintsTheProvenScopesAsOneJSONLine(t *testing.T) {
	old := proveScopes
	t.Cleanup(func() { proveScopes = old })
	for _, c := range []struct {
		proved []model.NetworkScope
		want   string
	}{{[]model.NetworkScope{"none", "restricted"}, `{"scopes":["none","restricted"]}`}, {nil, `{"scopes":[]}`}} {
		proveScopes = func(context.Context) []model.NetworkScope { return c.proved }
		code, out, _ := runCmd(t, "netscope-probe")
		if code != 0 || strings.TrimSpace(out) != c.want {
			t.Fatalf("code %d out %q, want %s", code, out, c.want)
		}
	}
	if code, _, _ := runCmd(t, "netscope-probe", "extra"); code != 2 {
		t.Fatalf("extra argument accepted: %d", code)
	}
}
