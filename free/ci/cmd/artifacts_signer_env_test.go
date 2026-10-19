package main

// Purpose: the signer's cli children get NSELF_V15=1 (D-1236). The released cli 1.4.x has
// `config secrets resolve` only with it, and the exec funnel strips the caller's environment.
// Inputs:  the fake cli of artifacts_key_test.go behind a wrapper that gates resolve like cli 1.4.
// Outputs: pass or fail.
// Constraints: unix only (shell wrapper).

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/signing"
)

// gateResolveOnV15 puts a wrapper in front of the fake cli: `config secrets resolve` answers exit 2
// (unknown command) unless the child has NSELF_V15=1, as a released cli 1.4.x does.
func gateResolveOnV15(t *testing.T) {
	t.Helper()
	real, err := exec.LookPath("nself")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(real, real+".real"); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nif [ \"$1 $2 $3\" = \"config secrets resolve\" ] && [ \"$NSELF_V15\" != 1 ]; then exit 2; fi\nexec '" + real + ".real' \"$@\"\n"
	if err = os.WriteFile(real, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestSignerResolverChildSeesV15(t *testing.T) {
	for _, tc := range []struct {
		name  string
		gated bool
		env   string // the caller's own NSELF_V15 (the funnel must not depend on it)
	}{
		{"cli 1.4 gates resolve, caller env stripped", true, ""},
		{"cli 1.4 gates resolve, caller env set", true, "1"},
		{"cli without the gate", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fakeNself(t)
			if tc.gated {
				gateResolveOnV15(t)
			}
			t.Setenv("NSELF_V15", tc.env)
			t.Setenv("NSELF_CI_HOME", t.TempDir())
			if err := os.WriteFile(filepath.Join(dir, "RELEASE_KEY"), []byte("k"), 0o600); err != nil {
				t.Fatal(err)
			}
			res := signerResolver(t.TempDir())
			ctx := context.Background()
			if err := res.Probe(ctx); err != nil {
				t.Fatalf("probe: %v", err)
			}
			ref, err := secrets.Parse("release/RELEASE_KEY")
			if err != nil {
				t.Fatal(err)
			}
			vals, err := res.Resolve(ctx, "key", []secrets.Ref{ref})
			if err != nil || len(vals) != 1 || string(vals[0].Bytes) != "k" {
				t.Fatalf("resolve: %v %v", vals, err)
			}
		})
	}
}

// The key store the signer writes with passes the same environment to its cli children.
func TestSignerStoreChildSeesV15(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st, err := signerStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cs, ok := st.(*signing.CLIStore)
	if !ok || !slices.Contains(cs.Env, "NSELF_V15=1") {
		t.Fatalf("store %T env %v", st, cs)
	}
}
