package report

import "testing"

func TestStatusContextOutboxKey(t *testing.T) {
	res := Status{Owner: "o", Repo: "r"}
	claim := Status{Owner: "o", Repo: "r", StatusContext: ClaimContext}
	watch := Status{Owner: "o", Repo: "r", StatusContext: WatchContext}
	if keyFor(res) != key("o", "r") {
		t.Fatalf("result key changed: %s", keyFor(res))
	}
	seen := map[string]bool{}
	for _, s := range []Status{res, claim, watch} {
		if seen[keyFor(s)] {
			t.Fatalf("collapsed key %s", keyFor(s))
		}
		seen[keyFor(s)] = true
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	bad := Status{Owner: "o", Repo: "r", Revision: sha, State: Pending, StatusContext: "other/ctx"}
	if bad.Validate() == nil {
		t.Fatal("a foreign context must be refused")
	}
	if got := Args(Status{Owner: "o", Repo: "r", Revision: sha, State: Pending, StatusContext: ClaimContext}); !contains(got, "context="+ClaimContext) {
		t.Fatalf("argv: %v", got)
	}
}

func contains(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}
