package run

// Purpose: the run policy resolves at the class the run is admitted at and at the repository
// visibility the host knows (D-1436), instead of a fixed owner / unknown.
// Constraints: a temp repository and store (runScopeRepo); RepoVisibility is restored.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

func TestPolicyHookResolvesAtTheRunClass(t *testing.T) {
	ctx := context.Background()
	root, _, db := runScopeRepo(t)
	owner, err := PolicyHook{}.Resolve(ctx, PolicyRequest{Root: root, Store: db, Class: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if blank, err := (PolicyHook{}).Resolve(ctx, PolicyRequest{Root: root, Store: db}); err != nil || blank.Digest != owner.Digest {
		t.Fatalf("no class resolves as the operator's own checkout: %v %q vs %q", err, blank.Digest, owner.Digest)
	}
	untrusted, err := PolicyHook{}.Resolve(ctx, PolicyRequest{Root: root, Store: db, Class: "untrusted"})
	if err != nil {
		t.Fatal(err)
	}
	if !untrusted.HasSecretClasses || len(untrusted.SecretClasses) != 0 {
		t.Fatalf("an untrusted run holds no secret class: %v %v", untrusted.HasSecretClasses, untrusted.SecretClasses)
	}
	if untrusted.Digest == owner.Digest {
		t.Fatal("the untrusted default scope is not part of the run policy")
	}
}

func TestPolicyVisibilityReportsPrivateOnly(t *testing.T) {
	t.Cleanup(func() { RepoVisibility = nil })
	if got := policyVisibility("acme/app"); got != "unknown" {
		t.Fatalf("no host seam: %q", got)
	}
	for vis, want := range map[attest.Visibility]string{attest.Private: "private", attest.Public: "unknown", "": "unknown"} {
		RepoVisibility = func(owner, repo string) attest.Visibility {
			if owner != "acme" || repo != "app" {
				t.Errorf("asked about %s/%s", owner, repo)
			}
			return vis
		}
		if got := policyVisibility("acme/app"); got != want {
			t.Errorf("seam says %q: %q, want %q", vis, got, want)
		}
	}
	if got := policyVisibility(""); got != "unknown" {
		t.Fatalf("no repository: %q", got)
	}
}
