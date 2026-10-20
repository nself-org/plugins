package agent

// Purpose: the push of a restore frame waits out a control stream that is being reopened, and stops with its context.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestFramePushWaitsForTheControlStream(t *testing.T) {
	calls := 0
	u := frameUp{ctx: context.Background(), rt: &route{node: "n", leaseID: "l", epoch: 2}, push: func(_ context.Context, node string, m protocol.Message) error {
		calls++
		if calls < 3 {
			return agentapi.ErrNoControlStream
		}
		if node != "n" || m.LeaseID != "l" || m.Epoch == nil || *m.Epoch != 2 || m.Type != "digest" {
			t.Errorf("frame %+v for %s", m, node)
		}
		return nil
	}}
	if err := u.Digest(protocol.Digest{Stream: "cache", SHA256: "x", Size: 1}); err != nil || calls != 3 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestFramePushStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	u := frameUp{ctx: ctx, rt: &route{node: "n", leaseID: "l", epoch: 1}, push: func(context.Context, string, protocol.Message) error { return agentapi.ErrNoControlStream }}
	start := time.Now()
	if err := u.Chunk(protocol.Chunk{Stream: "cache"}); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatalf("err=%v after %s", err, time.Since(start))
	}
}
