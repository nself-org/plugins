package registry

import (
	"context"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"reflect"
	"testing"
)

func TestIntakeKeepsOperatorLocationAndPoolLabels(t *testing.T) {
	for _, phase := range []string{"register", "update"} {
		t.Run(phase, func(t *testing.T) {
			r, c := setup(t)
			ctx := context.Background()
			hosted := "hosted"
			c.Location.Kind = model.Fact[string]{Value: &hosted, Source: "assigned", Confidence: "known"}
			c.Identity.Labels = append(c.Identity.Labels, "pool.team", "pool.record")
			var n Node
			var err error
			if phase == "register" {
				c.Identity.ID += "-forged"
				n, err = r.Register(ctx, c, "node")
			} else {
				n, _, err = r.UpdateCapability(ctx, c.Identity.ID, c, "node")
			}
			if err != nil {
				t.Fatal(err)
			}
			if n.Capability.Location.Kind.Value != nil && *n.Capability.Location.Kind.Value == "hosted" {
				t.Fatal("self-reported hosted location accepted")
			}
			for _, label := range n.Capability.Identity.Labels {
				if label == "pool.team" || label == "pool.record" {
					t.Fatal("self-reported pool label accepted")
				}
			}
			if phase == "update" {
				old, err := r.Get(ctx, c.Identity.ID)
				if err != nil {
					t.Fatal(err)
				}
				c.Location.Region.Value = new(string)
				got, _, err := r.UpdateCapability(ctx, c.Identity.ID, c, "node")
				if err != nil || !reflect.DeepEqual(got.Capability.Location, old.Capability.Location) {
					t.Fatalf("operator location changed: %v", err)
				}
			}
		})
	}
}
