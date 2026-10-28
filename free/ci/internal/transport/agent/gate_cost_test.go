package agent

// Purpose: reports and bounds what one log frame costs the gate (D-1376).
// Constraints: the bound is loose (an Ed25519 verify alone is far above it) so a slow machine does not fail it.

import (
	"context"
	"testing"
)

// TestGateLogFrameCost reports what one log frame costs the gate (run with -v): the case that repeats thousands of
// times a second per node. It asserts only that the cached path stays far under the cost of a signature check.
func TestGateLogFrameCost(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := r.gate()
	c := call(lease, tok, ep)
	if err := g.Check(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	res := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := g.Check(context.Background(), c); err != nil {
				b.Fatal(err)
			}
		}
	})
	t.Logf("log frame through the gate: %s", res.MemString()+" "+res.String())
	if ns := res.NsPerOp(); ns > 20_000 { // an Ed25519 verify alone is ~50us; a cached frame is a hash and two map reads
		t.Errorf("a cached log frame costs %d ns, want under 20000", ns)
	}
}
