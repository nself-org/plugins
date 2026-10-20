package pki

import (
	"context"
	"testing"
)

func TestNodeKeyCannotBeShared(t *testing.T) {
	for _, method := range []string{"pin", "hello", "pending hello"} {
		t.Run(method, func(t *testing.T) {
			r := newRig(t)
			ctx := context.Background()
			pub := edPub(t)
			if err := r.ca.PinNodeKey(ctx, "victim", pub); err != nil {
				t.Fatal(err)
			}
			var err error
			switch method {
			case "pin":
				err = r.ca.PinNodeKey(ctx, "attacker", pub)
			case "hello":
				err = r.ca.CheckNodeKey(ctx, "attacker", pub)
			case "pending hello":
				if err = r.ca.RotateNodeKey(ctx, "operator", "attacker", NodeKeyID(pub)); err != nil {
					t.Fatal(err)
				}
				err = r.ca.CheckNodeKey(ctx, "attacker", pub)
			}
			if err == nil {
				t.Fatal("accepted another node's key")
			}
			if err = r.ca.CheckNodeKey(ctx, "victim", pub); err != nil {
				t.Fatal(err)
			}
		})
	}
}
