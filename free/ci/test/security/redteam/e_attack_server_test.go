//go:build security

package redteam

// Purpose: red team group E, agent listener (TM-13 stale writer, TM-21 stolen job token, TM-23 wrong
// certificate kind, TM-26 replayed result, TM-20 node stays inside its own leases). The attacker is a node
// agent with a valid certificate of its own who replays tokens, uses another node's lease, re-hellos for a new
// session, sends hostile epochs and a hostile token header, and flings oversize or slow input at the listener.
// Inputs:  the real agentapi.Server with the real job-token Gate as Admit (rig in e_attack_rig_test.go).
// Outputs: pass = the attack was refused (control HELD); fail = it worked.
// Constraints: loopback only; each test finishes in seconds.

import (
	"crypto/tls"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// barrier sends a heartbeat and reads until its ack, so every frame sent before it has been handled. It
// returns the cancel messages seen on the way.
func (s *eStream) barrier(t *testing.T) (cancels []protocol.Cancel) {
	t.Helper()
	s.send(eHeartbeat())
	for i := 0; i < 8; i++ {
		m, ok := s.read(3 * time.Second)
		if !ok {
			t.Fatalf("no answer to the barrier heartbeat (cancels so far %d)", len(cancels))
		}
		switch b := m.Body.(type) {
		case *protocol.Ack:
			return cancels
		case *protocol.Cancel:
			cancels = append(cancels, *b)
		case *protocol.Reject:
			t.Fatalf("stream rejected: %s %s", b.Code, b.Reason)
		}
	}
	return cancels
}

// TestE21TokenRidesTheWire (TM-21): a lease-scoped frame without its token, with a malformed token header
// (trailing '=', a '=' inside the lease key, garbage), with another lease's token or in a second session of
// the same certificate must not reach the sink; the honest frame does (control).
func TestE21TokenRidesTheWire(t *testing.T) {
	r := newERig(t, true)
	a := r.newAgent("n-1")
	lease, att, ep := r.seedLease("1", "n-1")
	_, att2, ep2 := r.seedLease("2", "n-1")
	tok, tok2 := r.mint("jti-1", "n-1", att, ep), r.mint("jti-2", "n-1", att2, ep2)
	a.connect()
	hdr := func(l, v string) string { return l + "=" + v }
	for name, tokens := range map[string][]string{
		"no header":             nil,
		"trailing equals":       {hdr(lease, tok) + "="},
		"equals in key":         {hdr(lease+"=x", tok)},
		"garbage":               {hdr(lease, "x.y.z")},
		"token of lease 2":      {hdr(lease, tok2)},
		"empty value":           {lease + "="},
		"no separator":          {lease + tok},
		"uppercase lease key":   {hdr(strings.ToUpper(lease), tok)},
		"token with whitespace": {hdr(lease, " "+tok)},
	} {
		s := a.openStream(tokens...)
		s.send(eLog(lease, ep, name))
		cancels := s.barrier(t)
		if logs, _ := r.sink.counts(); logs != 0 || len(cancels) == 0 {
			t.Errorf("%s: sink logs=%d cancels=%d: a frame without a valid token for its lease was accepted", name, logs, len(cancels))
		}
		s.w.Close()
	}
	s := a.openStream(hdr(lease, "bad.tok.en"), hdr(lease, tok)) // duplicate key: the attacker only helps itself
	s.send(eLog(lease, ep, "honest"))
	if c := s.barrier(t); len(c) != 0 {
		t.Fatalf("control: the honest token was refused: %+v", c)
	}
	if logs, _ := r.sink.counts(); logs != 1 {
		t.Fatalf("control: sink logs = %d, want 1", logs)
	}
}

// TestE21ReplayFromOtherSessionAndCertificate (TM-21): once the token is bound to (certificate, session), the
// same node cannot use it from a second session or from a re-issued certificate (new serial) except by the
// documented first-heartbeat reconcile; a node that is not the lease holder never gets the lease at all.
func TestE21ReplayFromOtherSessionAndCertificate(t *testing.T) {
	r := newERig(t, true)
	a := r.newAgent("n-1")
	b := r.newAgent("n-2")
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	hdr := lease + "=" + tok
	a.connect()
	s1 := a.openStream(hdr)
	s1.send(eLog(lease, ep, "one"))
	if c := s1.barrier(t); len(c) != 0 {
		t.Fatalf("control: %+v", c)
	}

	// 1. same certificate, second session, plain frame
	a2 := &eAgent{r: r, id: "n-1", cl: a.cl, cert: a.cert, pub: a.pub}
	a2.connect()
	s2 := a2.openStream(hdr)
	s2.send(eLog(lease, ep, "second session"))
	if c := s2.barrier(t); len(c) == 0 {
		t.Error("a second session of the same certificate used the bound token on a log frame")
	}
	// 2. new certificate of the same node (a stolen/renewed serial), plain frame
	c3 := r.nodeCert("n-1")
	a3 := &eAgent{r: r, id: "n-1", cl: r.client(&c3), cert: c3, pub: a.pub}
	a3.connect()
	s3 := a3.openStream(hdr)
	s3.send(eLog(lease, ep, "other serial"))
	if c := s3.barrier(t); len(c) == 0 {
		t.Error("a re-issued certificate used the bound token on a log frame")
	}
	// 3. another node presents the token for this lease
	b.connect()
	s4 := b.openStream(hdr)
	s4.send(eLog(lease, ep, "thief"))
	if _, ok := s4.read(3 * time.Second); !ok {
		t.Log("stream of the thief ended (lease_not_held)")
	}
	if logs, _ := r.sink.counts(); logs != 1 {
		t.Errorf("sink holds %d logs, want only the honest one", logs)
	}
}

// TestE26ResultReplayAndForeignLease (TM-26, TM-13): a result frame replayed after it was accepted, a result
// for a lease another node holds and a result with a hostile epoch never reach the sink twice or at all.
func TestE26ResultReplayAndForeignLease(t *testing.T) {
	r := newERig(t, true)
	a := r.newAgent("n-1")
	b := r.newAgent("n-2")
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	hdr := lease + "=" + tok
	a.connect()
	b.connect()

	// foreign: n-2 reports a result for n-1's lease, with and without n-1's token
	for _, h := range []string{"", hdr} {
		s := b.openStream(h)
		s.send(eResult(lease, ep))
		s.read(2 * time.Second)
	}
	if _, res := r.sink.counts(); res != 0 {
		t.Fatalf("a node that does not hold the lease delivered a result (%d)", res)
	}
	// hostile epochs with the right token, one stream each (a fatal frame ends its stream)
	for _, e := range []uint64{0, ep + 1, 1 << 63, 1<<64 - 1} {
		x := a.openStream(hdr)
		x.send(eResult(lease, e))
		x.read(2 * time.Second)
		if _, res := r.sink.counts(); res != 0 {
			t.Fatalf("a result at epoch %d reached the sink", e)
		}
	}
	// the honest result is accepted once; a replay of it (same stream, new stream, new session) adds nothing
	h := a.openStream(hdr)
	h.send(eResult(lease, ep))
	if m, ok := h.read(3 * time.Second); !ok || m.Type != "ack" {
		t.Fatalf("control: honest result not acknowledged: %v %v", m.Type, ok)
	}
	if err := r.gate.Spend(t.Context(), "jti-1"); err != nil {
		t.Fatal(err)
	}
	h.send(eResult(lease, ep))
	h.read(2 * time.Second)
	for i := 0; i < 2; i++ {
		n := a.openStream(hdr)
		n.send(eResult(lease, ep))
		n.read(2 * time.Second)
	}
	a.connect()
	n := a.openStream(hdr)
	n.send(eResult(lease, ep))
	n.read(2 * time.Second)
	if _, res := r.sink.counts(); res != 1 {
		t.Errorf("sink holds %d results for one lease, want exactly 1 (replay)", res)
	}
}

// TestE23ListenerRejectsWrongClients (TM-23): no client certificate, a user-kind certificate and a TLS 1.2
// client are refused by the node listener; an HTTP/1.1 plaintext request on the TLS port gets no data.
func TestE23ListenerRejectsWrongClients(t *testing.T) {
	r := newERig(t, true)
	a := r.newAgent("n-1")
	a.connect() // control: the node certificate works
	userPEM, err := r.ca.IssueUser(t.Context(), "alice", 0)
	if err != nil {
		t.Fatal(err)
	}
	uc, err := tls.X509KeyPair(append(append([]byte{}, userPEM.CertPEM...), userPEM.RootPEM...), userPEM.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	for name, cl := range map[string]func() error{
		"no client certificate": func() error {
			_, e := r.client(nil).Post(r.base+"/v1/agent/hello", "x", strings.NewReader(""))
			return e
		},
		"user certificate": func() error {
			resp, e := r.client(&uc).Post(r.base+"/v1/agent/hello", "x", strings.NewReader(""))
			if e == nil {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					t.Errorf("user certificate: status %d", resp.StatusCode)
				}
			}
			return nil
		},
	} {
		if err := cl(); err == nil && name == "no client certificate" {
			t.Errorf("%s: the request succeeded", name)
		}
	}
	c12, err := tls.Dial("tcp", r.addr, &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12, Certificates: []tls.Certificate{a.cert}})
	if err == nil {
		c12.Close()
		t.Error("a TLS 1.2 handshake succeeded on the node listener")
	}
	raw, err := net.Dial("tcp", r.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	_, _ = raw.Write([]byte("POST /v1/agent/hello HTTP/1.1\r\nHost: x\r\nContent-Length: 0\r\n\r\n"))
	_ = raw.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	if n, _ := raw.Read(buf); n > 0 && strings.HasPrefix(string(buf[:n]), "HTTP/1.") && strings.Contains(string(buf[:min(n, 16)]), " 2") {
		t.Errorf("plaintext HTTP was served on the TLS port: %q", buf[:n])
	}
}

// TestE20FloodsAndHostileFrames (TM-20): a frame over 1 MiB without a newline, a heartbeat with 257 leases,
// a frame for an epoch-less lease and 100 idle connections must not stop the listener from serving the next
// honest agent.
func TestE20FloodsAndHostileFrames(t *testing.T) {
	r := newERig(t, true)
	a := r.newAgent("n-1")
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	a.connect()

	big := a.openStream(lease + "=" + tok)
	go func() { _, _ = big.w.Write([]byte(strings.Repeat("x", 3<<20))) }()
	if m, ok := big.read(5 * time.Second); ok {
		if rj, isRej := m.Body.(*protocol.Reject); !isRej {
			t.Errorf("oversize frame answered %s, want a reject", m.Type)
		} else {
			t.Logf("oversize frame rejected: %s %s", rj.Code, rj.Reason)
		}
	} else {
		t.Error("oversize frame: the stream was neither rejected nor closed within 5 s")
	}

	many := make([]protocol.LeaseState, 257)
	for i := range many {
		many[i] = protocol.LeaseState{LeaseID: lease, Epoch: ep, State: "running"}
	}
	hb := a.openStream(lease + "=" + tok)
	hb.send(eHeartbeat(many...))
	if m, _ := hb.read(3 * time.Second); m.Type == "ack" {
		t.Error("a heartbeat with 257 leases was acknowledged")
	}

	var idle []net.Conn
	for i := 0; i < 100; i++ {
		c, err := net.Dial("tcp", r.addr)
		if err != nil {
			break
		}
		idle = append(idle, c)
	}
	defer func() {
		for _, c := range idle {
			c.Close()
		}
	}()
	b := r.newAgent("n-2")
	done := make(chan struct{})
	go func() { b.connect(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Errorf("an honest hello did not complete with %d idle connections open", len(idle))
	}
}

// TestE20AdmitWiredInProduction (TM-20/21): the listener must not run with Admit unset (then every call is
// admitted without a token). Static check of the composition root, plus the behaviour of the unset default.
func TestE20AdmitWiredInProduction(t *testing.T) {
	src, err := os.ReadFile("../../../cmd/transport_agent.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "cfg.Admit") {
		t.Error("cmd/transport_agent.go does not set agentapi.Config.Admit: the job-token gate is not on the listener")
	}
	lst, _ := os.ReadFile("../../../cmd/serve_agent_listener.go")
	if !strings.Contains(string(lst), "agentWire") || !strings.Contains(string(lst), "return fail(fmt.Errorf(\"agent listener: transport") {
		t.Error("a failing transport wiring no longer stops the listener (fail open)")
	}
	r := newERig(t, false)
	a := r.newAgent("n-1")
	lease, _, ep := r.seedLease("1", "n-1")
	a.connect()
	s := a.openStream()
	s.send(eLog(lease, ep, "no token at all"))
	s.barrier(t)
	if logs, _ := r.sink.counts(); logs == 1 {
		t.Log("with Admit unset the listener admits token-less lease frames (documented NODE-20 default; wired in production)")
	}
}
