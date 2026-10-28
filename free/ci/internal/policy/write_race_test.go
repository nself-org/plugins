package policy

// Purpose: the policy write checks read the stored rows inside the write
// transaction (D-1283). A writer whose checks ran before the transaction could
// land a loosening over an operator tightening that arrived in between.
// Inputs: a real SQLite store and the git fixtures of helpers_test.go.
// Outputs: pass or fail; no network. Constraints: tests only.

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
)

// The machine-invoked write below passes against the stored rows it first sees
// (none: "vm" tightens the default). The operator tightens trust.isolation_min to
// "ephemeral-vm" while the writer reads the protected branch; the machine write must be
// judged against that row and refused, not land "vm" over it.
func TestMachineWriteSeesConcurrentTightening(t *testing.T) {
	f := newFixture(t)
	w, st, actor := testWriter(t)
	operator := &Writer{Store: st, Log: w.Log, Keys: w.Keys, Run: testRunner}
	ctx := context.Background()
	var tightened atomic.Bool
	w.Machine = true
	w.Run = func(c context.Context, argv ...string) (string, int, error) {
		if tightened.CompareAndSwap(false, true) { // once, on the writer's first git call
			if _, err := operator.Apply(ctx, actor, Change{Op: OpSet, Scope: "user", Key: "trust.isolation_min", Value: json.RawMessage(`"ephemeral-vm"`)}); err != nil {
				t.Errorf("operator write: %v", err)
			}
		}
		return testRunner(c, argv...)
	}
	_, err := w.Apply(ctx, actor, Change{Op: OpSet, Scope: "user", Root: f.work, Key: "trust.isolation_min", Value: json.RawMessage(`"vm"`)})
	if !tightened.Load() {
		t.Fatal("the writer never asked git, so the race window did not open")
	}
	if err == nil {
		t.Fatal("the machine write loosened over a concurrent operator tightening")
	}
	rows, rerr := st.PolicyRows(ctx, "user", LocalSubject)
	if rerr != nil || len(rows) != 1 || string(rows[0].Value) != `"ephemeral-vm"` {
		t.Fatalf("stored rows %v (%v), want the operator's ephemeral-vm", rows, rerr)
	}
}
