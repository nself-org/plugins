package agentapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestHelloPinsAgeAndQuarantinesChangedRecipient(t *testing.T) {
	r := newRig(t, func(cfg *Config) {
		reg := registry.New(cfg.Store)
		cfg.PinnedAge = reg.AgeRecipient
		cfg.CheckAge = func(ctx context.Context, node, recipient string) error {
			return reg.CheckAgeRecipient(ctx, node, recipient, "agent")
		}
	})
	a := r.newAgent("n-1")
	first := "age1first"
	for i := 0; i < 2; i++ {
		resp := a.hello(func(h *protocol.Hello) { h.AgeRecipient = &first })
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("first/same hello: %d", resp.StatusCode)
		}
		resp.Body.Close()
	}
	changed := "age1changed"
	resp := a.hello(func(h *protocol.Hello) { h.AgeRecipient = &changed })
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("changed hello: %d", resp.StatusCode)
	}
	if rej := rejectOf(t, resp); rej.Code != "E660" || rej.Reason != registry.QuarantineReason {
		t.Fatalf("reject: %+v", rej)
	}
	reg := registry.New(r.st)
	pin, err := reg.AgeRecipient(context.Background(), "n-1")
	if err != nil || pin == nil || *pin != first {
		t.Fatalf("pin changed: %v", err)
	}
	n, err := reg.Get(context.Background(), "n-1")
	if err != nil || n.Record.State != "maintenance" || n.Record.Reason != registry.QuarantineReason {
		t.Fatalf("quarantine: %+v %v", n.Record, err)
	}
}
