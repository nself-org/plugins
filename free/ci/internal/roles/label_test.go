package roles

// Purpose: tests of ActorFromLabel (D-1337): the dispatch of a queued run re-authorizes the submitter
// from the audit label the user API stored, so a revoked or demoted submitter loses release.start.
// Inputs:  the rig of roles_test.go. Outputs: pass or fail. Constraints: no network.

import (
	"context"
	"testing"
)

func TestActorFromLabelReauthorizesSubmitter(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	member := r.user("bob", Member)
	rel := r.user("carol", Release)
	tok := r.tokenActor("dave", Release, "o/r")
	cases := []struct {
		name, label string
		allow       bool
	}{
		{"member user", member.Label(), false},
		{"release user", rel.Label(), true},
		{"release token on its project", tok.Label(), true},
		{"operator", r.op().Label(), true},
		{"unknown user", "user:nobody", false},
		{"unknown token", "token:tok-none:x", false},
		{"empty label", "", false},
		{"garbage", "owner", false},
	}
	for _, c := range cases {
		a, err := r.svc.ActorFromLabel(ctx, c.label)
		if err == nil {
			err = AuthorizeAction(ctx, a, ActReleaseStart, "o/r")
		}
		if (err == nil) != c.allow {
			t.Errorf("%s: allow=%v err=%v", c.name, c.allow, err)
		}
	}
	if _, err := r.svc.Revoke(ctx, r.op(), "carol"); err != nil {
		t.Fatal(err)
	}
	a, err := r.svc.ActorFromLabel(ctx, rel.Label())
	if err == nil {
		err = AuthorizeAction(ctx, a, ActReleaseStart, "o/r")
	}
	if err == nil {
		t.Error("a revoked submitter still passed release.start")
	}
}
