package local

import (
	"context"
	"testing"
)

func TestProxyOnlyLeakTeardownUsesParentAttempt(t *testing.T) {
	prof := &fakeProfile{level: "container", av: up("container")}
	p := newProvider(t, NewContainerBackend(prof, nil, nil), nil)
	if err := p.Teardown(context.Background(), "burst-A1-px"); err != nil {
		t.Fatal(err)
	}
	if len(prof.tornDown) != 1 || prof.tornDown[0] != "burst-A1" {
		t.Fatalf("teardown: %v", prof.tornDown)
	}
}
