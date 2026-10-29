package run

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// D-1331: the coordinator refuses to deliver a lease whose declared scope the job's class may not have (the
// scheduler's admission rule is not the only gate), and every lease carries the class the job was placed at,
// with or without secrets, so the node can check the scope again.
func TestDeliverRefusesALANScopeForAnUntrustedJobAndCarriesTheClass(t *testing.T) {
	probe := &probeDriver{kind: fmt.Sprintf("netscopeprobe%d", time.Now().UnixNano())}
	if err := transport.Register(probe); err != nil {
		t.Fatal(err)
	}
	var n model.Capability
	n.Identity.ID, n.Identity.Transport = "x1", probe.kind
	job := model.Job{Kind: "test", Run: []string{"true"}}
	for _, c := range []struct {
		trust   model.TrustClass
		scope   model.NetworkScope
		refused bool
	}{
		{"untrusted", "lan", true},
		{"untrusted", "privileged", true},
		{"", "lan", true}, // no class reads as untrusted
		{"untrusted", "internet", false},
		{"untrusted", "", false},
		{"owner", "lan", false},
		{"collaborator", "privileged", false},
	} {
		t.Run(fmt.Sprintf("%s/%s", c.trust, c.scope), func(t *testing.T) {
			probe.mu.Lock()
			probe.lease.AttemptID = ""
			probe.mu.Unlock()
			d := deliveryFor(n, job)
			d.Trust, d.Network = c.trust, c.scope
			err := newTransportAdapter(nil, nil, "").Deliver(context.Background(), d)
			probe.mu.Lock()
			got := probe.lease
			probe.mu.Unlock()
			if c.refused {
				var ne *netscope.Error
				if !errors.As(err, &ne) || ne.Code != netscope.CodeScopeFailed {
					t.Fatalf("Deliver = %v, want E679", err)
				}
				if got.AttemptID != "" {
					t.Fatalf("a refused lease reached the node: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.NetworkScope != c.scope || got.Trust != c.trust {
				t.Fatalf("the lease carries scope=%q trust=%q, want %q %q", got.NetworkScope, got.Trust, c.scope, c.trust)
			}
		})
	}
}
