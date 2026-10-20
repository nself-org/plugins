package policy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/roles"
)

func TestMachineSpendWrites(t *testing.T) {
	for _, tc := range []struct {
		name, old, next, op string
		refused             bool
	}{
		{"loosen", `"off"`, `"on"`, OpSet, true},
		{"tighten", `"on"`, `"off"`, OpSet, false},
		{"unchanged", `"off"`, `"off"`, OpSet, false},
		{"unset", `"off"`, "", OpUnset, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, _, actor := testWriter(t)
			ctx := context.Background()
			change := Change{Op: OpSet, Scope: "user", Key: "economics.paid_enabled", Value: json.RawMessage(tc.old)}
			if _, err := w.Apply(ctx, actor, change); err != nil {
				t.Fatal(err)
			}
			w.Machine = true
			change.Op, change.Value = tc.op, json.RawMessage(tc.next)
			_, err := w.Apply(ctx, actor, change)
			if tc.refused && roles.ReasonOf(err) != ReasonMachineInvoked || !tc.refused && err != nil {
				t.Fatalf("refused=%v err=%v", tc.refused, err)
			}
		})
	}
}
