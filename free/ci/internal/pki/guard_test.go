package pki

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestGuardRefreshFollowsTheStore(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	g := NewGuard(r.st)
	var cancelledA, cancelledB, cancelledOther atomic.Int32
	stopA := g.watch("aa", func() { cancelledA.Add(1) })
	g.watch("aa", func() { cancelledB.Add(1) })
	g.watch("bb", func() { cancelledOther.Add(1) })
	if g.next != 3 || len(g.watchers["aa"]) != 2 {
		t.Fatalf("watcher ids must count up from 1: next=%d", g.next)
	}
	if err := g.Refresh(ctx); err != nil || g.Denied("aa") {
		t.Fatalf("empty refresh: %v", err)
	}
	if _, err := r.st.PKIDeny(ctx, "cli:admin", "n1", "x", "aa"); err != nil {
		t.Fatal(err)
	}
	if g.Denied("aa") {
		t.Fatal("denied before a refresh")
	}
	if err := g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if !g.Denied("aa") || g.Denied("bb") || cancelledA.Load() != 1 || cancelledB.Load() != 1 || cancelledOther.Load() != 0 {
		t.Fatalf("after deny: aa=%v a=%d b=%d other=%d", g.Denied("aa"), cancelledA.Load(), cancelledB.Load(), cancelledOther.Load())
	}
	// rows already seen are not replayed
	if err := g.Refresh(ctx); err != nil || cancelledA.Load() != 1 {
		t.Fatalf("replayed: %d %v", cancelledA.Load(), err)
	}
	// a later deny: only new rows cancel
	if _, err := r.st.PKIDeny(ctx, "cli:admin", "n2", "x", "bb", "cc"); err != nil {
		t.Fatal(err)
	}
	if err := g.Refresh(ctx); err != nil || !g.Denied("bb") || !g.Denied("cc") || cancelledOther.Load() != 1 || cancelledA.Load() != 1 {
		t.Fatalf("second deny: %v", err)
	}
	// stop removes only its own watcher, then the entry
	stopA()
	if len(g.watchers["aa"]) != 1 {
		t.Fatalf("watchers of aa: %d", len(g.watchers["aa"]))
	}
	// a denied serial that is stopped no longer has a map entry once empty
	g.watch("zz", func() {})()
	if _, ok := g.watchers["zz"]; ok {
		t.Fatal("empty watcher set kept")
	}
	// Refresh surfaces a store error
	_ = r.st.Close()
	if err := g.Refresh(ctx); err == nil {
		t.Fatal("refresh on a closed store succeeded")
	}
}

func TestGuardRunRefreshesUntilCancelled(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	r := newRig(t)
	g := NewGuard(r.st)
	if _, err := r.st.PKIDeny(ctx, "cli:admin", "n1", "x", "pre"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { g.Run(ctx, 20*time.Millisecond); close(done) }()
	waitFor(t, func() bool { return g.Denied("pre") }, "initial refresh")
	if _, err := r.st.PKIDeny(ctx, "cli:admin", "n1", "x", "later"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return g.Denied("later") }, "periodic refresh")
	stop()
	select {
	case <-done:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("Run did not stop")
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestGuardCheck(t *testing.T) {
	ctx := context.Background()
	t.Run("registered node", func(t *testing.T) {
		r := newRig(t)
		r.addNode("n1")
		n := r.issueNode("n1", 0)
		if err := NewGuard(r.st).Check(ctx, n.Cert); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("denied certificate", func(t *testing.T) {
		r := newRig(t)
		r.addNode("n1")
		n := r.issueNode("n1", 0)
		_, _ = r.st.PKIDeny(ctx, "cli:admin", "n1", "x", n.Serial)
		err := NewGuard(r.st).Check(ctx, n.Cert)
		wantReason(t, err, ReasonRevoked)
	})
	t.Run("denied certificate of an unregistered node is revoked, not unknown", func(t *testing.T) {
		r := newRig(t)
		n := r.issueNode("n1", 0)
		_, _ = r.st.PKIDeny(ctx, "cli:admin", "n1", "x", n.Serial)
		wantReason(t, NewGuard(r.st).Check(ctx, n.Cert), ReasonRevoked)
	})
	t.Run("unregistered node", func(t *testing.T) {
		r := newRig(t)
		n := r.issueNode("n1", 0)
		wantReason(t, NewGuard(r.st).Check(ctx, n.Cert), ReasonUnknownNode)
	})
	t.Run("revoked node state without a denied serial", func(t *testing.T) {
		r := newRig(t)
		r.addNode("n1")
		n := r.issueNode("n1", 0)
		if _, err := r.st.NodeRevoke(ctx, "n1", "cli:admin", nil); err != nil {
			t.Fatal(err)
		}
		err := NewGuard(r.st).Check(ctx, n.Cert)
		wantReason(t, err, ReasonRevoked)
	})
	t.Run("user and coordinator certificates skip the node lookup", func(t *testing.T) {
		r := newRig(t)
		_, u, _ := r.issueUser("alice", 0)
		g := NewGuard(r.st)
		if err := g.Check(ctx, u); err != nil {
			t.Fatal(err)
		}
		_, _ = r.st.PKIDeny(ctx, "cli:admin", "alice", "x", serialHex(u))
		wantReason(t, g.Check(ctx, u), ReasonRevoked)
		if err := g.Check(ctx, r.serverCert(nil, nil, 0).Leaf); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("store error counts as revoked", func(t *testing.T) {
		r := newRig(t)
		n := r.issueNode("n1", 0)
		_ = r.st.Close()
		wantReason(t, NewGuard(r.st).Check(ctx, n.Cert), ReasonRevoked)
	})
	t.Run("Check reads the store, not the cache", func(t *testing.T) {
		r := newRig(t)
		r.addNode("n1")
		n := r.issueNode("n1", 0)
		g := NewGuard(r.st)
		_ = g.Refresh(ctx)
		_, _ = r.st.PKIDeny(ctx, "cli:admin", "n1", "x", n.Serial)
		if g.Denied(n.Serial) {
			t.Fatal("cache moved without refresh")
		}
		wantReason(t, g.Check(ctx, n.Cert), ReasonRevoked)
	})
}

func TestGuardMiddlewareUnits(t *testing.T) {
	r := newRig(t)
	n := r.issueNode("n1", 0)
	_, u, _ := r.issueUser("alice", 0)
	g := NewGuard(r.st)
	var served atomic.Int32
	var ctxSeen context.Context
	h := g.Middleware(KindNode)(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
		served.Add(1)
		ctxSeen = rq.Context()
		_, _ = io.WriteString(w, "ok")
	}))
	do := func(state *tls.ConnectionState) *httptest.ResponseRecorder {
		rq := httptest.NewRequest("GET", "/x", nil)
		rq.TLS = state
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, rq)
		return rec
	}
	tests := []struct {
		name   string
		state  *tls.ConnectionState
		code   int
		body   string
		served int32
	}{
		{"no TLS", nil, 401, "E660: no client certificate\n", 0},
		{"no peer certificate", &tls.ConnectionState{}, 401, "E660: no client certificate\n", 0},
		{"wrong kind", &tls.ConnectionState{PeerCertificates: []*x509.Certificate{u}}, 403, "E660: wrong certificate kind\n", 0},
		{"no identity", &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}}, 403, "E660: wrong certificate kind\n", 0},
		{"ok", &tls.ConnectionState{PeerCertificates: []*x509.Certificate{n.Cert}}, 200, "ok", 1},
	}
	for _, c := range tests {
		before := served.Load()
		rec := do(c.state)
		if rec.Code != c.code || rec.Body.String() != c.body || served.Load()-before != c.served {
			t.Errorf("%s: %d %q served=%d", c.name, rec.Code, rec.Body.String(), served.Load()-before)
		}
	}
	if len(g.watchers) != 0 {
		t.Fatalf("watchers leaked: %d", len(g.watchers))
	}
	if ctxSeen.Err() == nil {
		t.Fatal("the request context stays live after the handler returned")
	}
	// denied in memory: refused before the handler
	_, _ = r.st.PKIDeny(context.Background(), "cli:admin", "n1", "x", n.Serial)
	_ = g.Refresh(context.Background())
	before := served.Load()
	rec := do(&tls.ConnectionState{PeerCertificates: []*x509.Certificate{n.Cert}})
	if rec.Code != 403 || rec.Body.String() != "E660: certificate revoked\n" || served.Load() != before {
		t.Fatalf("denied: %d %q", rec.Code, rec.Body.String())
	}
	if len(g.watchers) != 0 {
		t.Fatalf("watchers leaked after a refusal: %d", len(g.watchers))
	}
	// the user guard accepts the user certificate
	uh := g.Middleware(KindUser)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	rq := httptest.NewRequest("GET", "/", nil)
	rq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{u}}
	rec = httptest.NewRecorder()
	uh.ServeHTTP(rec, rq)
	if rec.Code != 204 {
		t.Fatalf("user guard: %d", rec.Code)
	}
}

// TestRequestGuardCancelsLiveRequestWithinOneSecond is the ticket's live-session check: a request
// already running on an open mTLS connection is cancelled within 1 s of the revoke, and the next
// request on that same connection is refused.
func TestRequestGuardCancelsLiveRequestWithinOneSecond(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	r := newRig(t)
	r.addNode("n1")
	n := r.issueNode("n1", 0)
	g := NewGuard(r.st)
	go g.Run(ctx, 100*time.Millisecond)

	started := make(chan struct{}, 4)
	cancelled := make(chan time.Time, 4)
	mux := http.NewServeMux()
	mux.HandleFunc("/hold", func(w http.ResponseWriter, rq *http.Request) {
		started <- struct{}{}
		<-rq.Context().Done()
		cancelled <- time.Now()
	})
	mux.HandleFunc("/ping", func(w http.ResponseWriter, rq *http.Request) { _, _ = io.WriteString(w, "pong") })
	srvCfg, err := r.ca.TLSServerConfig(KindNode, r.serverCert(nil, loopback, 0), g)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(g.Middleware(KindNode)(mux))
	srv.TLS = srvCfg
	srv.StartTLS()
	defer srv.Close()

	cliCfg := r.clientCfg(&n.TLS)
	tr := &http.Transport{TLSClientConfig: cliCfg, MaxConnsPerHost: 1, DisableKeepAlives: false}
	defer tr.CloseIdleConnections()
	cl := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	resp, err := cl.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "pong" || resp.StatusCode != 200 {
		t.Fatalf("ping: %d %q", resp.StatusCode, body)
	}
	holdDone := make(chan error, 1)
	go func() {
		resp, err := cl.Get(srv.URL + "/hold")
		if err == nil {
			resp.Body.Close()
		}
		holdDone <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("hold request never started")
	}
	revokedAt := time.Now()
	if _, err = r.ca.Revoke(context.Background(), "cli:admin", "n1", "stolen"); err != nil {
		t.Fatal(err)
	}
	select {
	case at := <-cancelled:
		if d := at.Sub(revokedAt); d > time.Second {
			t.Fatalf("live request cancelled after %v", d)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("live request not cancelled within 1 s of the revoke")
	}
	<-holdDone
	// the next request, on a fresh or existing connection, is refused
	resp, err = cl.Get(srv.URL + "/ping")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("next request status %d", resp.StatusCode)
		}
	} // a rejected fresh handshake is also a refusal
}

// TestRequestGuardRefusesNextRequestOnExistingConnection pins the request-level check separately from the
// handshake: the connection stays open across the revoke.
func TestRequestGuardRefusesNextRequestOnExistingConnection(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	r := newRig(t)
	r.addNode("n1")
	n := r.issueNode("n1", 0)
	g := NewGuard(r.st)
	go g.Run(ctx, 50*time.Millisecond)
	srvCfg, _ := r.ca.TLSServerConfig(KindNode, r.serverCert(nil, loopback, 0), g)
	srv := httptest.NewUnstartedServer(g.Middleware(KindNode)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "pong") })))
	srv.TLS = srvCfg
	srv.StartTLS()
	defer srv.Close()
	var handshakes atomic.Int32
	cliCfg := r.clientCfg(&n.TLS)
	cliCfg.VerifyConnection = wrapCount(cliCfg.VerifyConnection, &handshakes)
	tr := &http.Transport{TLSClientConfig: cliCfg, MaxConnsPerHost: 1}
	defer tr.CloseIdleConnections()
	cl := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	get := func() (int, error) {
		resp, err := cl.Get(srv.URL)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	}
	if code, err := get(); err != nil || code != 200 {
		t.Fatalf("first: %d %v", code, err)
	}
	// deny straight in the store (not through lifecycle) so no handshake check can fire first
	if _, err := r.st.PKIDeny(context.Background(), "cli:admin", "n1", "x", n.Serial); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return g.Denied(n.Serial) }, "guard refresh")
	code, err := get()
	if err != nil || code != 403 {
		t.Fatalf("next request on the open connection: %d %v", code, err)
	}
	if handshakes.Load() != 1 {
		t.Fatalf("the connection was not reused: %d handshakes", handshakes.Load())
	}
}

func wrapCount(f func(tls.ConnectionState) error, n *atomic.Int32) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error { n.Add(1); return f(cs) }
}
