package authz_test

// Purpose: TestInvariant_I11, "untrusted never writes a namespace a trusted job
// reads" (Epic invariant catalogue), including the per-PR variant: PR A never
// writes a namespace PR B reads.
// The set of namespaces a trusted job reads is computed from CacheAccess, not
// listed by hand, so a widened read rule widens the set the writer is tested
// against. The writer is given the most permissive state (finalizing, passed,
// matching capability): only the namespace rules stand between it and the write.

import (
	"fmt"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// candidates are the namespaces of one project a store could be asked about.
func candidates(t *testing.T, project string) []authz.Namespace {
	t.Helper()
	var out []authz.Namespace
	for _, class := range append([]model.TrustClass{"untrusted"}, trustedClasses...) {
		for _, p := range []authz.Protection{authz.Protected, authz.Unprotected} {
			for _, pr := range [][2]string{{"fork/repo", "7"}, {"fork/repo", "8"}, {"other/repo", "7"}} {
				if ns, err := authz.CacheNamespace(project, class, p, pr[0], pr[1]); err == nil && !contains(out, ns) {
					out = append(out, ns)
				}
			}
		}
	}
	return out
}

func contains(list []authz.Namespace, ns authz.Namespace) bool {
	for _, x := range list {
		if x == ns {
			return true
		}
	}
	return false
}

func TestInvariant_I11(t *testing.T) {
	all := candidates(t, "proj-1")
	foreign := candidates(t, "proj-2")
	invariants.Check(t, func(w invariants.World) error {
		if w.Trust != "untrusted" {
			return nil
		}
		head, pr := "fork/repo", "7"
		if w.Ownership == "other" {
			head = "other/repo"
		}
		if w.Mode == "team" {
			pr = "8"
		}
		writerNS, err := authz.CacheNamespace("proj-1", w.Trust, authz.Protected, head, pr)
		if err != nil {
			return err
		}
		writer := finalJob(writerNS)
		for _, target := range append(append([]authz.Namespace{}, all...), foreign...) {
			// CheckWrite and CacheAccess are two layers; either one granting is a write.
			if authz.CheckWrite(writer, target, currentCap) != nil && authz.CacheAccess(writer, target) != authz.ModeRWOwn {
				continue
			}
			if !target.Equal(writerNS) {
				return fmt.Errorf("untrusted %s/%s wrote %+v, not its own namespace", head, pr, target)
			}
			// target is writable by the untrusted job: no trusted job may read it
			for _, class := range trustedClasses {
				for _, p := range []authz.Protection{authz.Protected, authz.Unprotected} {
					readerNS, rerr := authz.CacheNamespace("proj-1", class, p, "", "")
					if rerr != nil {
						return rerr
					}
					if authz.CacheAccess(finalJob(readerNS), target) != authz.ModeNone {
						return fmt.Errorf("trusted %s/%s reads %+v that untrusted %s/%s writes", class, p, target, head, pr)
					}
				}
			}
		}
		return nil
	})
}

// The per-PR variant spelled out: two PRs, every ordering, every state.
func TestCacheAccess_PerPRNamespacesDistinct(t *testing.T) {
	a := mustNS(t, "untrusted", authz.Unprotected, "fork/repo", "1")
	b := mustNS(t, "untrusted", authz.Unprotected, "fork/repo", "2")
	if err := authz.CheckWrite(finalJob(a), b, currentCap); err == nil {
		t.Fatal("PR A wrote PR B's namespace")
	}
	if err := authz.CheckWrite(finalJob(b), a, currentCap); err == nil {
		t.Fatal("PR B wrote PR A's namespace")
	}
	if err := authz.CheckWrite(finalJob(a), a, currentCap); err != nil {
		t.Fatalf("PR A cannot write its own namespace: %v", err)
	}
	if authz.CacheAccess(finalJob(b), a) != authz.ModeNone {
		t.Fatal("PR B reads PR A's namespace")
	}
}
