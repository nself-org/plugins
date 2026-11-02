package roles

import (
	"context"
	"testing"
)

func TestSubmittedActorReleaseAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, kind, project string
		role                Role
		revoke, mismatch    bool
		want                bool
	}{
		{"member user", "user", "p1", Member, false, false, false},
		{"release user", "user", "p1", Release, false, false, true},
		{"revoked user", "user", "p1", Release, true, false, false},
		{"member token", "token", "p1", Member, false, false, false},
		{"release token", "token", "p1", Release, false, false, true},
		{"revoked token", "token", "p1", Release, true, false, false},
		{"wrong token principal", "token", "p1", Release, false, true, false},
		{"project limited token", "token", "p2", Release, false, false, false},
		{"operator label", "operator", "p1", Release, false, false, false},
		{"missing actor", "", "p1", Release, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			ctx := context.Background()
			r.grant("bob", tc.role, "p1")
			label := tc.kind + ":bob"
			if tc.kind == "token" {
				a := r.tokenActor("bob", tc.role, "p1")
				label = a.Label()
				if tc.mismatch {
					label = "token:" + a.tokenID + ":eve"
				}
				if tc.revoke {
					if _, err := r.svc.RevokeToken(ctx, r.op(), a.tokenID); err != nil {
						t.Fatal(err)
					}
				}
			} else if tc.revoke {
				if _, err := r.svc.Revoke(ctx, r.op(), "bob"); err != nil {
					t.Fatal(err)
				}
			}
			a, err := r.svc.ActorFromSubmitter(ctx, label)
			if err == nil {
				err = AuthorizeAction(ctx, a, ActReleaseStart, tc.project)
			}
			if (err == nil) != tc.want {
				t.Fatalf("release.start allowed = %v, want %v: %v", err == nil, tc.want, err)
			}
		})
	}
}
