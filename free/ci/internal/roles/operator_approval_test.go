package roles

// Purpose: the local operator may approve its own run on a PERSONAL coordinator and may not on a TEAM
// coordinator (D-1436); an unknown author never locks the operator out.
// Constraints: the rig's temp store; the mode row is written the way `serve --team` writes it.

import (
	"context"
	"testing"
)

func TestOperatorSelfApprovalFollowsTheCoordinatorMode(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	own := WithAuthor(ctx, "operator:cli:alice")
	approve := func(c context.Context) string { return ReasonOf(AuthorizeAction(c, r.op(), ActRunApprove, "p1")) }

	if got := approve(own); got != "" { // no serve ever stored a mode
		t.Fatalf("no mode row: %q", got)
	}
	if _, err := r.st.CoordinatorModeEnsure(ctx, false, "test"); err != nil {
		t.Fatal(err)
	}
	if got := approve(own); got != "" {
		t.Fatalf("PERSONAL: the operator is the only person: %q", got)
	}
	if _, err := r.st.CoordinatorModeEnsure(ctx, true, "test"); err != nil {
		t.Fatal(err)
	}
	for _, author := range []string{"alice", "cli:alice", "operator:cli:alice", "user:ALICE"} {
		if got := approve(WithAuthor(ctx, author)); got != ReasonSelfApproval {
			t.Errorf("TEAM, author %q is the operator: %q, want %q", author, got, ReasonSelfApproval)
		}
	}
	if got := approve(WithAuthor(ctx, "bob")); got != "" {
		t.Errorf("TEAM, another author: %q", got)
	}
	if got := approve(ctx); got != "" {
		t.Errorf("TEAM, unknown author: %q", got)
	}
}
