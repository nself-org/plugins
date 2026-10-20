package agentapi

// Purpose: Push puts a lease-scoped digest or chunk on the node's control stream, in order, only for
// a node that holds a control stream, and never queues without bound behind a node that stops reading.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func pushFrame(kind, leaseID string, epoch uint64, body any) protocol.Message {
	return protocol.Message{Type: kind, LeaseID: leaseID, Epoch: &epoch, Body: body}
}

func TestPushReachesTheControlStreamInOrder(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	sha := strings.Repeat("ab", 32)
	if err := r.srv.Push(context.Background(), "n-1", pushFrame("digest", leaseID, epoch, protocol.Digest{Stream: "cache", SHA256: sha, Size: 4})); !errors.Is(err, ErrNoControlStream) {
		t.Fatalf("push to a node that is not connected: %v", err)
	}
	a.connect()
	if err := r.srv.Push(context.Background(), "n-1", pushFrame("digest", leaseID, epoch, protocol.Digest{Stream: "cache", SHA256: sha, Size: 4})); !errors.Is(err, ErrNoControlStream) {
		t.Fatalf("push without a control stream: %v", err)
	}
	resp, err := a.cl.Do(a.req(http.MethodGet, "/v1/agent/control", nil, nil))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("control: %v %v", resp, err)
	}
	defer resp.Body.Close()
	eventually(t, 2*time.Second, "the control stream to count as live", func() bool {
		return r.srv.Push(context.Background(), "n-1", pushFrame("digest", leaseID, epoch, protocol.Digest{Stream: "cache", SHA256: sha, Size: 4})) == nil
	})
	if err = r.srv.Push(context.Background(), "n-1", pushFrame("chunk", leaseID, epoch, protocol.Chunk{Stream: "cache", Offset: 0, Data: "AAAA", Digest: sha, Final: true})); err != nil {
		t.Fatal(err)
	}
	fr := newFrameReader(resp.Body)
	m, err := fr.next()
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := m.Body.(*protocol.Digest); !ok || d.SHA256 != sha || m.LeaseID != leaseID || m.Epoch == nil || *m.Epoch != epoch {
		t.Fatalf("first frame %s %+v", m.Type, m.Body)
	}
	if m, err = fr.next(); err != nil || m.Type != "chunk" {
		t.Fatalf("second frame %+v %v", m, err)
	}
}

func TestPushRefusesWhatIsNotALeaseScopedTransferFrame(t *testing.T) {
	r := newRig(t, nil)
	for _, m := range []protocol.Message{
		{Type: "cancel", LeaseID: "l", Epoch: new(uint64), Body: protocol.Cancel{LeaseID: "l"}},
		{Type: "digest", Body: protocol.Digest{Stream: "cache"}},
		{Type: "chunk", LeaseID: "l", Body: protocol.Chunk{Stream: "cache"}},
	} {
		if err := r.srv.Push(context.Background(), "n-1", m); err == nil || errors.Is(err, ErrNoControlStream) {
			t.Errorf("%s accepted or misreported: %v", m.Type, err)
		}
	}
}

func TestPushWaitsForRoomAndEndsWithItsContext(t *testing.T) {
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	leaseID, epoch := r.seedLease("1")
	a.connect()
	resp, err := a.cl.Do(a.req(http.MethodGet, "/v1/agent/control", nil, nil))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("control: %v %v", resp, err)
	}
	defer resp.Body.Close() // the node never reads: the socket fills, then the queue
	sha := strings.Repeat("ab", 32)
	data := strings.Repeat("A", 700<<10)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	sent := 0
	for ; sent < 400; sent++ {
		err = r.srv.Push(ctx, "n-1", pushFrame("chunk", leaseID, epoch, protocol.Chunk{Stream: "cache", Offset: int64(sent), Data: data, Digest: sha}))
		if err != nil {
			break
		}
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrNoControlStream) {
		t.Fatalf("push behind a node that does not read: %v after %d frames", err, sent)
	}
	if sent >= 400 {
		t.Fatal("the queue grew without bound")
	}
}
