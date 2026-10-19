package pki

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestServerConfigShapes(t *testing.T) {
	r := newRig(t)
	leaf := r.serverCert(nil, loopback, 0)
	g := NewGuard(r.st)
	for _, k := range []Kind{KindNode, KindUser} {
		cfg, err := r.ca.TLSServerConfig(k, leaf, g)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MinVersion != tls.VersionTLS13 || !cfg.SessionTicketsDisabled || cfg.ClientAuth != tls.RequireAndVerifyClientCert || cfg.VerifyConnection == nil || len(cfg.Certificates) != 1 {
			t.Fatalf("%s config: %+v", k, cfg)
		}
		if !cfg.Time().Equal(r.clk.now()) {
			t.Fatal("server config does not use the CA clock")
		}
		subs := cfg.ClientCAs.Subjects() //nolint:staticcheck // the pool holds exactly one authority
		if len(subs) != 1 {
			t.Fatalf("%d client CAs", len(subs))
		}
		if string(subs[0]) != string(r.ca.inter[k].cert.RawSubject) {
			t.Fatalf("%s client CA is not the %s intermediate", k, k)
		}
	}
	for _, k := range []Kind{KindCoordinator, "", "admin"} {
		cfg, err := r.ca.TLSServerConfig(k, leaf, g)
		wantReason(t, err, ReasonWrongKind)
		if cfg != nil {
			t.Fatalf("config for %q", k)
		}
	}
	enroll := r.ca.EnrollTLSConfig(leaf)
	if enroll.MinVersion != tls.VersionTLS13 || !enroll.SessionTicketsDisabled || enroll.ClientAuth != tls.NoClientCert || enroll.VerifyConnection != nil || len(enroll.Certificates) != 1 || !enroll.Time().Equal(r.clk.now()) {
		t.Fatalf("enroll config: %+v", enroll)
	}
}

func TestClientConfig(t *testing.T) {
	r := newRig(t)
	n := r.issueNode("n1", 0)
	cfg, err := TLSClientConfig(ClientOptions{RootDER: r.ca.RootDER(), ServerName: "ci.example", Now: r.clk.now})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinVersion != tls.VersionTLS13 || cfg.ServerName != "ci.example" || len(cfg.Certificates) != 0 || cfg.VerifyConnection == nil || cfg.InsecureSkipVerify {
		t.Fatalf("config %+v", cfg)
	}
	if !cfg.Time().Equal(r.clk.now()) {
		t.Fatal("client clock")
	}
	if subs := cfg.RootCAs.Subjects(); len(subs) != 1 || string(subs[0]) != string(r.ca.Root().RawSubject) { //nolint:staticcheck
		t.Fatal("client trusts something other than the pinned root")
	}
	cfg, err = TLSClientConfig(ClientOptions{RootDER: r.ca.RootDER(), Cert: &n.TLS})
	if err != nil || len(cfg.Certificates) != 1 || cfg.Time != nil {
		t.Fatalf("with cert: %v", err)
	}
	for _, der := range [][]byte{nil, []byte("junk"), r.ca.RootDER()[:50]} {
		c, err := TLSClientConfig(ClientOptions{RootDER: der})
		wantReason(t, err, ReasonBadChain)
		if c != nil {
			t.Fatal("config with a bad root")
		}
	}
}

func TestServerVerifyConnectionBranches(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.addNode("n1")
	n := r.issueNode("n1", 0)
	_, u, _ := r.issueUser("alice", 0)
	g := NewGuard(r.st)
	cfg, err := r.ca.TLSServerConfig(KindNode, r.serverCert(nil, loopback, 0), g)
	if err != nil {
		t.Fatal(err)
	}
	nodeChain := []*x509.Certificate{n.Cert, r.ca.inter[KindNode].cert}
	userChain := []*x509.Certificate{u, r.ca.inter[KindUser].cert}
	tests := []struct {
		name   string
		cs     tls.ConnectionState
		reason string
	}{
		{"no certificate", tls.ConnectionState{}, ReasonNoCert},
		{"user certificate at a node server", tls.ConnectionState{PeerCertificates: []*x509.Certificate{u}, VerifiedChains: [][]*x509.Certificate{userChain}}, ReasonWrongKind},
		{"no identity", tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}}, ReasonWrongKind},
		{"node cert without verified chains", tls.ConnectionState{PeerCertificates: []*x509.Certificate{n.Cert}}, ReasonBadChain},
		{"node cert chained to the wrong intermediate", tls.ConnectionState{PeerCertificates: []*x509.Certificate{n.Cert}, VerifiedChains: [][]*x509.Certificate{{n.Cert, r.ca.inter[KindUser].cert}}}, ReasonBadChain},
		{"node cert chain ending at the root", tls.ConnectionState{PeerCertificates: []*x509.Certificate{n.Cert}, VerifiedChains: [][]*x509.Certificate{{n.Cert, r.ca.inter[KindNode].cert, r.ca.Root()}}}, ReasonBadChain},
		{"good", tls.ConnectionState{PeerCertificates: []*x509.Certificate{n.Cert}, VerifiedChains: [][]*x509.Certificate{nodeChain}}, ""},
	}
	for _, c := range tests {
		err := cfg.VerifyConnection(c.cs)
		if c.reason == "" {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if ReasonOf(err) != c.reason {
			t.Errorf("%s: %v, want %s", c.name, err, c.reason)
		}
	}
	// revoked certificate: the deny list is read on every call
	_, _ = r.st.PKIDeny(ctx, "cli:admin", "n1", "x", n.Serial)
	wantReason(t, cfg.VerifyConnection(tests[len(tests)-1].cs), ReasonRevoked)
	// user server accepts the user chain
	ucfg, _ := r.ca.TLSServerConfig(KindUser, r.serverCert(nil, loopback, 0), g)
	if err = ucfg.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{u}, VerifiedChains: [][]*x509.Certificate{userChain}}); err != nil {
		t.Fatal(err)
	}
	wantReason(t, ucfg.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{n.Cert}, VerifiedChains: [][]*x509.Certificate{nodeChain}}), ReasonWrongKind)
}

func TestClientVerifyConnectionBranches(t *testing.T) {
	r := newRig(t)
	n := r.issueNode("n1", 0)
	coord := r.serverCert(nil, loopback, 0)
	cfg := r.clientCfg(nil)
	root, nodeInter := r.ca.Root(), r.ca.inter[KindNode].cert
	coordInter := r.ca.inter[KindCoordinator].cert
	good := tls.ConnectionState{PeerCertificates: []*x509.Certificate{coord.Leaf}, VerifiedChains: [][]*x509.Certificate{{coord.Leaf, coordInter, root}}}
	tests := []struct {
		name string
		cs   tls.ConnectionState
		ok   bool
		why  string
	}{
		{"good", good, true, ""},
		{"no peer certificate", tls.ConnectionState{VerifiedChains: good.VerifiedChains}, false, ReasonNoCert},
		{"no verified chain", tls.ConnectionState{PeerCertificates: good.PeerCertificates}, false, ReasonNoCert},
		{"node leaf as server", tls.ConnectionState{PeerCertificates: []*x509.Certificate{n.Cert}, VerifiedChains: [][]*x509.Certificate{{n.Cert, nodeInter, root}}}, false, ReasonWrongKind},
		{"coordinator leaf, chain of two", tls.ConnectionState{PeerCertificates: good.PeerCertificates, VerifiedChains: [][]*x509.Certificate{{coord.Leaf, root}}}, false, ReasonWrongKind},
		{"coordinator leaf under another intermediate", tls.ConnectionState{PeerCertificates: good.PeerCertificates, VerifiedChains: [][]*x509.Certificate{{coord.Leaf, nodeInter, root}}}, false, ReasonWrongKind},
		{"no identity", tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}, VerifiedChains: good.VerifiedChains}, false, ReasonWrongKind},
	}
	for _, c := range tests {
		err := cfg.VerifyConnection(c.cs)
		if c.ok != (err == nil) || (!c.ok && ReasonOf(err) != c.why) {
			t.Errorf("%s: %v, want ok=%v %s", c.name, err, c.ok, c.why)
		}
	}
}

func TestEnrollHandshake(t *testing.T) {
	r := newRig(t)
	leaf := r.serverCert([]string{"ci.example.com"}, loopback, 0)
	srv := r.ca.EnrollTLSConfig(leaf)

	t.Run("pinned root, matching server name", func(t *testing.T) {
		cerr, serr, cs := handshake(t, srv, r.clientCfg(nil))
		if cerr != nil || serr != nil {
			t.Fatalf("client=%v server=%v", cerr, serr)
		}
		if cs.Version != tls.VersionTLS13 || cs.DidResume || len(cs.PeerCertificates) != 1 && len(cs.PeerCertificates) != 3 {
			t.Fatalf("state: v=%x resumed=%v", cs.Version, cs.DidResume)
		}
		if !Equal(CertFingerprint(cs.VerifiedChains[0][len(cs.VerifiedChains[0])-1].Raw), r.ca.RootFingerprint()) {
			t.Fatal("served chain does not end at the root fingerprint")
		}
	})
	t.Run("server name must match a SAN", func(t *testing.T) {
		cfg := r.clientCfg(nil)
		cfg.ServerName = "other.example.com"
		if cerr, _, _ := handshake(t, srv, cfg); cerr == nil {
			t.Fatal("handshake with a mismatched name succeeded")
		}
		cfg.ServerName = "ci.example.com"
		if cerr, serr, _ := handshake(t, srv, cfg); cerr != nil || serr != nil {
			t.Fatalf("DNS SAN: %v %v", cerr, serr)
		}
	})
	t.Run("a proxy with its own root", func(t *testing.T) {
		mitm := newRig(t)
		msrv := mitm.ca.EnrollTLSConfig(mitm.serverCert(nil, loopback, 0))
		if cerr, _, _ := handshake(t, msrv, r.clientCfg(nil)); cerr == nil {
			t.Fatal("client accepted a server chained to another root")
		}
	})
	t.Run("a proxy with the real root cannot sign for it", func(t *testing.T) {
		// same CA name, fresh keys: the chain does not verify against the pinned root
		mitm := newRig(t)
		leaf := mitm.serverCert(nil, loopback, 0)
		leaf.Certificate = append(leaf.Certificate[:2:2], r.ca.RootDER())
		if cerr, _, _ := handshake(t, mitm.ca.EnrollTLSConfig(leaf), r.clientCfg(nil)); cerr == nil {
			t.Fatal("forged chain accepted")
		}
	})
	t.Run("expired server leaf", func(t *testing.T) {
		short := r.serverCert(nil, loopback, time.Minute)
		cfg := r.clientCfg(nil)
		cfg.Time = func() time.Time { return r.clk.now().Add(2 * time.Minute) }
		if cerr, _, _ := handshake(t, r.ca.EnrollTLSConfig(short), cfg); cerr == nil {
			t.Fatal("expired leaf accepted")
		}
	})
}

// TestServerKindTable: an agent client accepts a coordinator leaf only; a node-kind or user-kind
// leaf chained to the same root is refused as a server (ticket acceptance, table test).
func TestServerKindTable(t *testing.T) {
	r := newRig(t)
	coord := r.serverCert(nil, loopback, 0)
	n := r.issueNode("n1", 0)
	_, uLeaf, uPair := r.issueUser("alice", 0)
	_ = uLeaf
	// serve the node and user leaves as servers: their SANs are empty, so give them the IP through a
	// leaf that would otherwise verify; the refusal must come from EKU and kind, not only the name
	tests := []struct {
		name string
		cert tls.Certificate
		ok   bool
	}{
		{"coordinator leaf", coord, true},
		{"node-kind leaf", n.TLS, false},
		{"user-kind leaf", uPair, false},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			cerr, _, _ := handshake(t, r.ca.EnrollTLSConfig(c.cert), r.clientCfg(nil))
			if c.ok != (cerr == nil) {
				t.Fatalf("client error: %v", cerr)
			}
			// with name verification and EKU removed the kind check alone still refuses
			cfg := r.clientCfg(nil)
			base := cfg.VerifyConnection
			cfg.InsecureSkipVerify = true
			cfg.VerifyConnection = func(cs tls.ConnectionState) error {
				chains, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{Roots: cfg.RootCAs, Intermediates: pool(cs.PeerCertificates[1:]), CurrentTime: r.clk.now(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
				if err != nil {
					return err
				}
				cs.VerifiedChains = chains
				return base(cs)
			}
			cerr, _, _ = handshake(t, r.ca.EnrollTLSConfig(c.cert), cfg)
			if c.ok != (cerr == nil) {
				t.Fatalf("kind check alone: %v", cerr)
			}
			if !c.ok && ReasonOf(errors.Unwrap(cerr)) != ReasonWrongKind && ReasonOf(cerr) != ReasonWrongKind {
				t.Fatalf("refusal reason: %v", cerr)
			}
		})
	}
}

func pool(cs []*x509.Certificate) *x509.CertPool {
	p := x509.NewCertPool()
	for _, c := range cs {
		p.AddCert(c)
	}
	return p
}

func TestMutualTLSHandshakes(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.addNode("n1")
	g := NewGuard(r.st)
	leaf := r.serverCert(nil, loopback, 0)
	nodeSrv, _ := r.ca.TLSServerConfig(KindNode, leaf, g)
	userSrv, _ := r.ca.TLSServerConfig(KindUser, leaf, g)
	n := r.issueNode("n1", 0)
	_, _, uPair := r.issueUser("alice", 0)

	t.Run("node certificate at the node server", func(t *testing.T) {
		cerr, serr, cs := handshake(t, nodeSrv, r.clientCfg(&n.TLS))
		if cerr != nil || serr != nil || len(cs.PeerCertificates) == 0 {
			t.Fatalf("%v %v", cerr, serr)
		}
	})
	t.Run("user certificate at the user server", func(t *testing.T) {
		if cerr, serr, _ := handshake(t, userSrv, r.clientCfg(&uPair)); cerr != nil || serr != nil {
			t.Fatalf("%v %v", cerr, serr)
		}
	})
	t.Run("the other kind is refused", func(t *testing.T) {
		if _, serr, _ := handshake(t, nodeSrv, r.clientCfg(&uPair)); serr == nil {
			t.Fatal("user certificate accepted at a node server")
		}
		if _, serr, _ := handshake(t, userSrv, r.clientCfg(&n.TLS)); serr == nil {
			t.Fatal("node certificate accepted at a user server")
		}
	})
	t.Run("no client certificate", func(t *testing.T) {
		if _, serr, _ := handshake(t, nodeSrv, r.clientCfg(nil)); serr == nil {
			t.Fatal("accepted without a client certificate")
		}
	})
	t.Run("a coordinator certificate is not a client certificate", func(t *testing.T) {
		coordPair := r.serverCert(nil, loopback, 0)
		if _, serr, _ := handshake(t, nodeSrv, r.clientCfg(&coordPair)); serr == nil {
			t.Fatal("coordinator leaf accepted as a client")
		}
	})
	t.Run("a certificate of another CA", func(t *testing.T) {
		other := newRig(t)
		foreign := other.issueNode("n1", 0)
		if _, serr, _ := handshake(t, nodeSrv, r.clientCfg(&foreign.TLS)); serr == nil {
			t.Fatal("foreign certificate accepted")
		}
	})
	t.Run("an unregistered node", func(t *testing.T) {
		ghost := r.issueNode("ghost", 0)
		_, serr, _ := handshake(t, nodeSrv, r.clientCfg(&ghost.TLS))
		wantReason(t, serr, ReasonUnknownNode)
	})
	t.Run("expired", func(t *testing.T) {
		r2 := newRig(t)
		r2.addNode("n1")
		srv, _ := r2.ca.TLSServerConfig(KindNode, r2.serverCert(nil, loopback, 0), NewGuard(r2.st))
		c := r2.issueNode("n1", time.Hour)
		r2.clk.advance(30 * time.Minute)
		if cerr, serr, _ := handshake(t, srv, r2.clientCfg(&c.TLS)); cerr != nil || serr != nil {
			t.Fatalf("still valid: %v %v", cerr, serr)
		}
		r2.clk.advance(time.Hour)
		cfg := r2.clientCfg(&c.TLS)
		cfg.Time = func() time.Time { return c.Cert.NotAfter.Add(-time.Second) } // the client does not mind; the server does
		if _, serr, _ := handshake(t, srv, cfg); serr == nil {
			t.Fatal("server accepted an expired client certificate")
		}
	})
	t.Run("revoked certificate fails a fresh handshake", func(t *testing.T) {
		r2 := newRig(t)
		r2.addNode("n1")
		srv, _ := r2.ca.TLSServerConfig(KindNode, r2.serverCert(nil, loopback, 0), NewGuard(r2.st))
		c := r2.issueNode("n1", 0)
		if cerr, serr, _ := handshake(t, srv, r2.clientCfg(&c.TLS)); cerr != nil || serr != nil {
			t.Fatalf("before revoke: %v %v", cerr, serr)
		}
		if _, err := r2.ca.Revoke(ctx, "cli:admin", "n1", "x"); err != nil {
			t.Fatal(err)
		}
		cerr, serr, _ := handshake(t, srv, r2.clientCfg(&c.TLS))
		wantReason(t, serr, ReasonRevoked)
		if cerr == nil {
			t.Fatal("client did not see the rejection")
		}
	})
	t.Run("revoked node fails a fresh handshake", func(t *testing.T) {
		r2 := newRig(t)
		r2.addNode("n1")
		srv, _ := r2.ca.TLSServerConfig(KindNode, r2.serverCert(nil, loopback, 0), NewGuard(r2.st))
		c := r2.issueNode("n1", 0)
		if _, err := r2.st.NodeRevoke(ctx, "n1", "cli:admin", nil); err != nil { // no deny rows: state only
			t.Fatal(err)
		}
		_, serr, _ := handshake(t, srv, r2.clientCfg(&c.TLS))
		wantReason(t, serr, ReasonRevoked)
	})
}

// countingCache is a ClientSessionCache that records every ticket the server offers.
type countingCache struct {
	puts, gets atomic.Int32
	inner      tls.ClientSessionCache
}

func (c *countingCache) Get(k string) (*tls.ClientSessionState, bool) {
	c.gets.Add(1)
	return c.inner.Get(k)
}

func (c *countingCache) Put(k string, s *tls.ClientSessionState) {
	if s != nil {
		c.puts.Add(1)
	}
	c.inner.Put(k, s)
}

// TestRevokedCertificateFailsResumedAttempt: session tickets are off, so a client that tries to resume
// is told nothing to resume and runs a full handshake, whose VerifyConnection sees the deny list.
func TestRevokedCertificateFailsResumedAttempt(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.addNode("n1")
	srv, _ := r.ca.TLSServerConfig(KindNode, r.serverCert(nil, loopback, 0), NewGuard(r.st))
	n := r.issueNode("n1", 0)
	cache := &countingCache{inner: tls.NewLRUClientSessionCache(8)}
	cfg := r.clientCfg(&n.TLS)
	cfg.ClientSessionCache = cache
	var verifies atomic.Int32
	cfg.VerifyConnection = wrapCount(cfg.VerifyConnection, &verifies)

	cerr, serr, cs := handshake(t, srv, cfg)
	if cerr != nil || serr != nil || cs.DidResume {
		t.Fatalf("first: %v %v resumed=%v", cerr, serr, cs.DidResume)
	}
	if cache.puts.Load() != 0 {
		t.Fatalf("the server offered %d session tickets", cache.puts.Load())
	}
	cerr, serr, cs = handshake(t, srv, cfg)
	if cerr != nil || serr != nil || cs.DidResume {
		t.Fatalf("second: %v %v resumed=%v", cerr, serr, cs.DidResume)
	}
	if _, err := r.ca.Revoke(ctx, "cli:admin", "n1", "x"); err != nil {
		t.Fatal(err)
	}
	cerr, serr, cs = handshake(t, srv, cfg)
	wantReason(t, serr, ReasonRevoked)
	if cerr == nil || cs.DidResume {
		t.Fatalf("resumed attempt after revoke: client=%v resumed=%v", cerr, cs.DidResume)
	}
	if cache.gets.Load() < 3 {
		t.Fatalf("client never tried to resume (%d lookups)", cache.gets.Load())
	}
}
