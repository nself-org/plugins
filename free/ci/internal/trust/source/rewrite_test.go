package source_test

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func TestClassifyRewriteNeedsPatchVerification(t *testing.T) {
	for _, subject := range []string{"commit (amend): work", "rebase (pick): work", "pull --rebase (pick): work", "cherry-pick: work"} {
		for _, verified := range []bool{false, true} {
			t.Run(subject+map[bool]string{false: "/foreign-or-unknown", true: "/unique"}[verified], func(t *testing.T) {
				g := source.GitFacts{Head: "a", DefaultRemoteRef: "refs/remotes/origin/main", OffDefault: []source.Commit{{SHA: "a"}}, ReflogCreated: map[string]string{"a": subject}, RewriteVerified: map[string]bool{"a": verified}}
				got := source.ClassifyLocal(g)
				if (got.Class == "owner") != verified {
					t.Fatalf("class = %s, verified = %v", got.Class, verified)
				}
			})
		}
	}
}
