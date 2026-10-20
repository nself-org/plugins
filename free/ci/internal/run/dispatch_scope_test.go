package run

// Purpose: the lease built for a remote node carries the attempt's trust class and declared scope, and an
// untrusted attempt that declares lan or privileged is refused before anything is delivered (D-1331).

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func TestAdapterLeaseCarriesTrustAndScopeAndRefusesLAN(t *testing.T) {
	probe := &probeDriver{kind: fmt.Sprintf("scopeprobe%d", time.Now().UnixNano())}
	if err := transport.Register(probe); err != nil {
		t.Fatal(err)
	}
	var n model.Capability
	n.Identity.ID, n.Identity.Transport = "x1", probe.kind
	job := model.Job{Kind: "static", Run: []string{"true"}}
	a := newTransportAdapter(nil, nil, "")

	d := deliveryFor(n, job)
	d.Trust, d.Network = "collaborator", "internet"
	if err := a.Deliver(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	probe.mu.Lock()
	got := probe.lease
	probe.lease = protocol.Lease{}
	probe.mu.Unlock()
	if got.Trust != "collaborator" || got.Network != "internet" {
		t.Fatalf("lease trust %q network %q, want collaborator/internet", got.Trust, got.Network)
	}

	for _, scope := range []model.NetworkScope{"lan", "privileged"} {
		d := deliveryFor(n, job)
		d.Trust, d.Network = "untrusted", scope
		err := a.Deliver(context.Background(), d)
		var ne *netscope.Error
		if !errors.As(err, &ne) || ne.Code != netscope.CodeScopeFailed {
			t.Fatalf("untrusted %s: err %v, want E679", scope, err)
		}
		probe.mu.Lock()
		delivered := probe.lease.LeaseID != ""
		probe.mu.Unlock()
		if delivered {
			t.Fatalf("untrusted %s was delivered", scope)
		}
	}
}
