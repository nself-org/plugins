package agentapi

// Purpose: a control stream that sits idle longer than the write timeout stays open and still carries the
//          next frame (a write deadline left armed makes an HTTP/2 stream reset itself with INTERNAL_ERROR).
// Inputs:  the rig with a 150 ms write timeout. Outputs: pass or fail.
// Constraints: the idle wait is a few times the timeout; the frame must arrive on the same stream.

import (
	"net/http"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestIdleControlStreamOutlivesWriteTimeout(t *testing.T) {
	old := writeTimeout
	writeTimeout = 150 * time.Millisecond
	t.Cleanup(func() { writeTimeout = old })
	r := newRig(t, nil)
	a := r.newAgent("n-1")
	leaseID, _ := r.seedLease("1")
	a.connect()
	resp, err := a.cl.Do(a.req(http.MethodGet, "/v1/agent/control", nil, nil))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("control: %v %v", resp, err)
	}
	defer resp.Body.Close()
	eventually(t, 2*time.Second, "the control stream to count as live", func() bool {
		return r.srv.Drain("n-1", time.Unix(1_900_000_000, 0)) == nil
	})
	fr := newFrameReader(resp.Body)
	if m, err := fr.next(); err != nil || m.Type != "drain" {
		t.Fatalf("first control frame = %+v %v, want drain", m, err)
	}
	time.Sleep(4 * writeTimeout) // idle: no frame is being written, the old deadline would have fired
	if err = r.srv.Cancel("n-1", leaseID, "user cancel"); err != nil {
		t.Fatalf("Cancel after the idle wait: %v (the control stream was reset)", err)
	}
	m, err := fr.next()
	if err != nil {
		t.Fatalf("control stream after an idle wait: %v", err)
	}
	if c, ok := m.Body.(*protocol.Cancel); !ok || c.LeaseID != leaseID {
		t.Fatalf("control frame after the idle wait = %+v, want cancel for %s", m, leaseID)
	}
}
