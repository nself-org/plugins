package registry

import (
	"context"
	"testing"
)

func TestUpdateCannotClaimProviderAuthority(t *testing.T) {
	for _, kind := range []string{"hosted", "lan"} {
		t.Run(kind, func(t *testing.T) {
			r, c := setup(t)
			old := c.Location.Kind
			c.Location.Kind.Value = &kind
			c.Identity.Labels = []string{"user.fast", "nself.provider=hosted"}
			got, _, err := r.UpdateCapability(context.Background(), c.Identity.ID, c, "agent")
			if err != nil {
				t.Fatal(err)
			}
			if kind == "hosted" && *got.Capability.Location.Kind.Value != *old.Value {
				t.Fatal("node claimed hosted location")
			}
			for _, label := range got.Capability.Identity.Labels {
				if label == "nself.provider=hosted" {
					t.Fatal("node claimed provider label")
				}
			}
		})
	}
}
