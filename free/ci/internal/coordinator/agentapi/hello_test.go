package agentapi

// Purpose: TestHello: version negotiation (E652), node-key pinning and key change (E660 node_key_changed),
//          strict frame decoding (E653) and the session the welcome opens.
// Inputs:  the rig. Outputs: pass or fail.
// Constraints: the real listener and CA; a rejected hello opens no session.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func rejectOf(t *testing.T, resp *http.Response) protocol.Reject {
	t.Helper()
	defer resp.Body.Close()
	m := readFrame(t, resp.Body)
	rej, ok := m.Body.(*protocol.Reject)
	if !ok {
		t.Fatalf("answer is %s, want reject", m.Type)
	}
	return *rej
}

func TestHello(t *testing.T) {
	t.Run("welcome carries session, heartbeat and agent ttl", func(t *testing.T) {
		r := newRig(t, nil)
		a := r.newAgent("n-1")
		resp := a.hello(nil)
		defer resp.Body.Close()
		w, ok := readFrame(t, resp.Body).Body.(*protocol.Welcome)
		if !ok || resp.StatusCode != 200 {
			t.Fatalf("status %d, body %T", resp.StatusCode, w)
		}
		if w.Version != 1 || w.SessionID == "" || w.HeartbeatMS != 10000 || w.AgentTTLMS != 25000 {
			t.Fatalf("welcome = %+v, want version 1, heartbeat 10000 ms, agent ttl 25000 ms", *w)
		}
	})
	t.Run("version negotiation", func(t *testing.T) {
		cases := []struct {
			name   string
			mut    func(*protocol.Hello)
			reason string
		}{
			{"no common version", func(h *protocol.Hello) { h.Versions = []int{2} }, "no_common_version"},
			{"agent older than the minimum", func(h *protocol.Hello) { h.AgentVersion = "1.4.0" }, "agent_version_too_old"},
			{"agent version malformed", func(h *protocol.Hello) { h.AgentVersion = "banana" }, "invalid_agent_version"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				r := newRig(t, func(cfg *Config) { cfg.MinAgentVersion = "1.5.0" })
				a := r.newAgent("n-1")
				resp := a.hello(c.mut)
				if resp.StatusCode != http.StatusUpgradeRequired {
					t.Fatalf("status %d, want 426", resp.StatusCode)
				}
				rej := rejectOf(t, resp)
				if rej.Code != protocol.CodeVersion || rej.Reason != c.reason || rej.MinAgentVersion != "1.5.0" {
					t.Fatalf("reject = %+v, want E652 %s naming min_agent_version 1.5.0", rej, c.reason)
				}
				if got := r.srv.NodesConnected(); len(got) != 0 {
					t.Fatalf("a rejected hello opened a session: %v", got)
				}
			})
		}
	})
	t.Run("node key is pinned and a changed key is rejected", func(t *testing.T) {
		r := newRig(t, nil)
		a := r.newAgent("n-1")
		a.connect() // pins on the first hello
		a.connect() // same key: accepted again
		first := a.session
		other, _, _ := ed25519.GenerateKey(rand.Reader)
		resp := a.hello(func(h *protocol.Hello) {
			h.NodeKey = protocol.NodeKey{Alg: "ed25519", KeyID: pki.NodeKeyID(other), Public: base64.StdEncoding.EncodeToString(other)}
		})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status %d, want 403", resp.StatusCode)
		}
		if rej := rejectOf(t, resp); rej.Code != pki.CodeInvalid || rej.Reason != pki.ReasonNodeKeyChanged {
			t.Fatalf("reject = %+v, want E660 node_key_changed", rej)
		}
		a.session = first // the earlier session is still the node's
		rq := a.req(http.MethodGet, "/v1/agent/leases", nil, map[string]string{HeaderWait: "0"})
		if got, err := a.cl.Do(rq); err != nil || got.StatusCode != http.StatusNoContent {
			t.Fatalf("session after a rejected hello: %v %v", got, err)
		}
	})
	t.Run("malformed node key", func(t *testing.T) {
		r := newRig(t, nil)
		a := r.newAgent("n-1")
		resp := a.hello(func(h *protocol.Hello) { h.NodeKey.KeyID = "0000000000000000" })
		if rej := rejectOf(t, resp); rej.Code != pki.CodeInvalid {
			t.Fatalf("reject = %+v, want E660", rej)
		}
	})
	t.Run("strict decoding", func(t *testing.T) {
		r := newRig(t, nil)
		a := r.newAgent("n-1")
		bad := map[string][]byte{
			"unknown field":  []byte(`{"v":1,"type":"hello","seq":1,"body":{"versions":[1],"extra":1}}` + "\n"),
			"wrong type":     frame(t, protocol.Message{Type: "ack", Body: protocol.Ack{AckSeq: 1}}),
			"empty body":     nil,
			"unterminated":   []byte(`{"v":1,"type":"hello","seq":1,"body":{}}`),
			"seq not one":    frame(t, protocol.Message{Type: "ack", Seq: 2, Body: protocol.Ack{AckSeq: 1}}),
			"oversize frame": append(bytes.Repeat([]byte("x"), protocol.MaxFrame+10), '\n'),
		}
		for name, b := range bad {
			resp, err := a.cl.Post(r.base+"/v1/agent/hello", "application/x-ndjson", bytes.NewReader(b))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusRequestEntityTooLarge {
				t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
			}
		}
		if got := r.srv.NodesConnected(); len(got) != 0 {
			t.Fatalf("a refused hello opened a session: %v", got)
		}
	})
	t.Run("requests after hello need the session", func(t *testing.T) {
		r := newRig(t, nil)
		a := r.newAgent("n-1")
		a.connect()
		for _, path := range []string{"/v1/agent/leases", "/v1/agent/control"} {
			rq := a.req(http.MethodGet, path, nil, nil)
			rq.Header.Set(HeaderSession, "not-the-session")
			resp, err := a.cl.Do(rq)
			if err != nil || resp.StatusCode != http.StatusConflict {
				t.Fatalf("%s with a wrong session: %v %v", path, resp, err)
			}
			resp.Body.Close()
		}
	})
}
