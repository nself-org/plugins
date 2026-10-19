package pki

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func (r *rig) verifyChain(cert *x509.Certificate, inter []byte, eku x509.ExtKeyUsage) error {
	pool := x509.NewCertPool()
	pool.AddCert(r.ca.Root())
	ip := x509.NewCertPool()
	if im, err := x509.ParseCertificate(inter); err == nil {
		ip.AddCert(im)
	}
	_, err := cert.Verify(x509.VerifyOptions{Roots: pool, Intermediates: ip, CurrentTime: r.clk.now(), KeyUsages: []x509.ExtKeyUsage{eku}})
	return err
}

func TestValidID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"a", true}, {"0abc", true}, {"node-1", true}, {"a.b_c-d", true}, {strings.Repeat("a", 63), true},
		{"", false}, {strings.Repeat("a", 64), false}, {"A", false}, {"aB", false}, {"-a", false}, {".a", false}, {"_a", false},
		{"a/b", false}, {"a b", false}, {"a:b", false}, {"a\n", false}, {"\na", false}, {"é", false}, {"a\x00", false},
	}
	for _, c := range tests {
		if got := ValidID(c.id); got != c.want {
			t.Errorf("ValidID(%q) = %v", c.id, got)
		}
	}
}

func TestURI(t *testing.T) {
	if got := URI(KindNode, "n1").String(); got != "nself-ci:node/n1" {
		t.Fatalf("uri %q", got)
	}
	if u := URI(KindCoordinator, "c"); u.Scheme != "nself-ci" || u.Opaque != "coordinator/c" {
		t.Fatalf("%+v", u)
	}
}

func TestCheckTTL(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want time.Duration
		bad  bool
	}{
		{0, 7 * day, false}, {30 * time.Minute, 30 * time.Minute, false}, {time.Minute, time.Minute, false},
		{59 * time.Second, 0, true}, {time.Minute - 1, 0, true}, {7 * day, 7 * day, false}, {7*day + 1, 0, true},
		{8 * day, 0, true}, {-time.Hour, 0, true}, {30 * time.Second, 0, true}, {1, 0, true},
	}
	for _, c := range tests {
		got, err := CheckTTL(c.in)
		if c.bad {
			wantReason(t, err, ReasonCertTTL)
			if got != 0 || !strings.HasPrefix(err.Error(), "E660: ") {
				t.Errorf("CheckTTL(%v) = %v %v", c.in, got, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("CheckTTL(%v) = %v %v, want %v", c.in, got, err, c.want)
		}
	}
	if DefaultTTL != 7*day || MinTTL != time.Minute || ClockSkew != 120*time.Second {
		t.Fatal("TTL constants changed")
	}
}

func TestIssueNodeCertificate(t *testing.T) {
	r := newRig(t)
	csr, key := newCSR(t)
	is, err := r.ca.IssueNode(context.Background(), NodeRequest{CSR: csr, NodeID: "node-1", OfferID: "offer-9"})
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(is.CertDER)
	if err != nil {
		t.Fatal(err)
	}
	now := r.clk.now()
	if !cert.NotBefore.Equal(now.Add(-120*time.Second)) || !cert.NotAfter.Equal(now.Add(7*day)) || !is.NotAfter.Equal(cert.NotAfter) {
		t.Fatalf("validity %v .. %v (%v)", cert.NotBefore, cert.NotAfter, is.NotAfter)
	}
	if cert.Subject.CommonName != "node-1" || len(cert.DNSNames) != 0 || len(cert.IPAddresses) != 0 || len(cert.EmailAddresses) != 0 {
		t.Fatalf("the CSR's identity leaked into the certificate: %+v %v", cert.Subject, cert.DNSNames)
	}
	if len(cert.URIs) != 1 || cert.URIs[0].String() != "nself-ci:node/node-1" {
		t.Fatalf("URIs %v", cert.URIs)
	}
	if len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth || cert.KeyUsage != x509.KeyUsageDigitalSignature || cert.IsCA || !cert.BasicConstraintsValid {
		t.Fatalf("usage: %v %v ca=%v", cert.ExtKeyUsage, cert.KeyUsage, cert.IsCA)
	}
	if !cert.PublicKey.(*ecdsa.PublicKey).Equal(&key.PublicKey) {
		t.Fatal("certificate does not carry the CSR key")
	}
	if cert.Issuer.CommonName != "nself-ci node CA" {
		t.Fatalf("issuer %q", cert.Issuer.CommonName)
	}
	if is.Serial != serialHex(cert) || len(is.Chain) != 2 || string(is.Chain[0]) != string(is.CertDER) || string(is.Chain[1]) != string(r.ca.inter[KindNode].cert.Raw) || string(is.RootDER) != string(r.ca.RootDER()) {
		t.Fatalf("issued: %+v", is)
	}
	if err = r.verifyChain(cert, is.Chain[1], x509.ExtKeyUsageClientAuth); err != nil {
		t.Fatalf("does not chain: %v", err)
	}
	if err = r.verifyChain(cert, is.Chain[1], x509.ExtKeyUsageServerAuth); err == nil {
		t.Fatal("a node certificate verifies for server auth")
	}
	row, err := r.st.PKICertGet(context.Background(), is.Serial)
	want := store.PKICert{Serial: is.Serial, Kind: "node", NodeID: "node-1", Subject: "nself-ci:node/node-1", OfferID: "offer-9",
		NotBefore: cert.NotBefore.UnixNano(), NotAfter: cert.NotAfter.UnixNano(), CapAfter: 0, IssuedAt: now.UnixNano()}
	if err != nil || row != want {
		t.Fatalf("row %+v %v\nwant %+v", row, err, want)
	}
}

func TestIssueNodeTTLAndCap(t *testing.T) {
	tests := []struct {
		ttl     time.Duration
		life    time.Duration
		capped  bool
		wantErr bool
	}{
		{0, 7 * day, false, false},
		{7 * day, 7 * day, false, false},
		{30 * time.Minute, 30 * time.Minute, true, false},
		{time.Minute, time.Minute, true, false},
		{7*day - time.Second, 7*day - time.Second, true, false},
		{59 * time.Second, 0, false, true},
		{8 * day, 0, false, true},
		{30 * time.Second, 0, false, true},
	}
	for _, c := range tests {
		t.Run(c.ttl.String(), func(t *testing.T) {
			r := newRig(t)
			csr, _ := newCSR(t)
			is, err := r.ca.IssueNode(context.Background(), NodeRequest{CSR: csr, NodeID: "n1", TTL: c.ttl})
			if c.wantErr {
				wantReason(t, err, ReasonCertTTL)
				if is != nil || r.count(`SELECT count(*) FROM pki_cert`) != 0 {
					t.Fatal("a certificate was issued or recorded")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !is.NotAfter.Equal(r.clk.now().Add(c.life)) {
				t.Fatalf("NotAfter %v", is.NotAfter)
			}
			row, _ := r.st.PKICertGet(context.Background(), is.Serial)
			wantCap := int64(0)
			if c.capped {
				wantCap = is.NotAfter.UnixNano()
			}
			if row.CapAfter != wantCap || row.NotAfter != is.NotAfter.UnixNano() {
				t.Fatalf("cap_after %d, want %d", row.CapAfter, wantCap)
			}
		})
	}
}

func TestIssueNodeRejections(t *testing.T) {
	r := newRig(t)
	good, _ := newCSR(t)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaCSR, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, rsaKey)
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p384CSR, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, p384)
	_, edKey, _ := ed25519.GenerateKey(rand.Reader)
	edCSR, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, edKey)
	tampered := append([]byte{}, good...)
	tampered[len(tampered)-1] ^= 0xff // the signature is the last field
	tests := []struct {
		name   string
		req    NodeRequest
		reason string
	}{
		{"garbage", NodeRequest{CSR: []byte("not a csr"), NodeID: "n1"}, ReasonBadCSR},
		{"empty", NodeRequest{NodeID: "n1"}, ReasonBadCSR},
		{"bad signature", NodeRequest{CSR: tampered, NodeID: "n1"}, ReasonBadCSR},
		{"RSA key", NodeRequest{CSR: rsaCSR, NodeID: "n1"}, ReasonBadCSR},
		{"P-384 key", NodeRequest{CSR: p384CSR, NodeID: "n1"}, ReasonBadCSR},
		{"Ed25519 key", NodeRequest{CSR: edCSR, NodeID: "n1"}, ReasonBadCSR},
		{"empty id", NodeRequest{CSR: good}, ReasonBadCSR},
		{"upper-case id", NodeRequest{CSR: good, NodeID: "Node"}, ReasonBadCSR},
		{"slash id", NodeRequest{CSR: good, NodeID: "a/b"}, ReasonBadCSR},
		{"long id", NodeRequest{CSR: good, NodeID: strings.Repeat("a", 64)}, ReasonBadCSR},
		{"ttl", NodeRequest{CSR: good, NodeID: "n1", TTL: 8 * day}, ReasonCertTTL},
		{"bad id wins over bad ttl", NodeRequest{CSR: good, NodeID: "BAD", TTL: 8 * day}, ReasonBadCSR},
		{"bad ttl wins over bad csr", NodeRequest{CSR: []byte("x"), NodeID: "n1", TTL: 8 * day}, ReasonCertTTL},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			is, err := r.ca.IssueNode(context.Background(), c.req)
			wantReason(t, err, c.reason)
			if is != nil {
				t.Fatal("issued despite the error")
			}
		})
	}
	if n := r.count(`SELECT count(*) FROM pki_cert`); n != 0 {
		t.Fatalf("%d certificates recorded for refused requests", n)
	}
	// every reason text names no key bytes
	_, err = r.ca.IssueNode(context.Background(), NodeRequest{CSR: good, NodeID: "BAD/ID"})
	if strings.Contains(err.Error(), string(good)) {
		t.Fatal("error carries the CSR")
	}
}

func TestCSRKeyFingerprint(t *testing.T) {
	csr, key := newCSR(t)
	got, err := CSRKeyFingerprint(csr)
	want, _ := KeyFingerprint(&key.PublicKey)
	if err != nil || got != want {
		t.Fatalf("%s %v", got, err)
	}
	if fp, err := CSRKeyFingerprint([]byte("junk")); fp != "" {
		t.Fatalf("junk: %q", fp)
	} else {
		wantReason(t, err, ReasonBadCSR)
	}
	csr2, _ := newCSR(t)
	if g2, _ := CSRKeyFingerprint(csr2); g2 == got {
		t.Fatal("two keys, one fingerprint")
	}
}

func TestIssueUserBundle(t *testing.T) {
	r := newRig(t)
	b, leaf, _ := r.issueUser("alice", 0)
	now := r.clk.now()
	if leaf.Subject.CommonName != "alice" || len(leaf.URIs) != 1 || leaf.URIs[0].String() != "nself-ci:user/alice" || leaf.Issuer.CommonName != "nself-ci user CA" {
		t.Fatalf("identity: %v %v", leaf.Subject, leaf.URIs)
	}
	if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth || leaf.IsCA {
		t.Fatalf("usage %v", leaf.ExtKeyUsage)
	}
	if !leaf.NotBefore.Equal(now.Add(-120*time.Second)) || !leaf.NotAfter.Equal(now.Add(7*day)) {
		t.Fatalf("validity %v .. %v", leaf.NotBefore, leaf.NotAfter)
	}
	// CertPEM: leaf then user intermediate; RootPEM: the root; KeyPEM: PKCS8 of the leaf key
	var certs []*x509.Certificate
	rest := b.CertPEM
	for {
		var blk *pem.Block
		blk, rest = pem.Decode(rest)
		if blk == nil {
			break
		}
		if blk.Type != "CERTIFICATE" {
			t.Fatalf("block %s", blk.Type)
		}
		c, err := x509.ParseCertificate(blk.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		certs = append(certs, c)
	}
	if len(certs) != 2 || string(certs[1].Raw) != string(r.ca.inter[KindUser].cert.Raw) {
		t.Fatalf("chain of %d", len(certs))
	}
	rb, _ := pem.Decode(b.RootPEM)
	if rb == nil || rb.Type != "CERTIFICATE" || string(rb.Bytes) != string(r.ca.RootDER()) {
		t.Fatal("RootPEM is not the root")
	}
	kb, _ := pem.Decode(b.KeyPEM)
	if kb == nil || kb.Type != "PRIVATE KEY" {
		t.Fatalf("KeyPEM block %v", kb)
	}
	k, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil || !k.(*ecdsa.PrivateKey).PublicKey.Equal(leaf.PublicKey) {
		t.Fatalf("key does not match: %v", err)
	}
	if err = r.verifyChain(certs[0], certs[1].Raw, x509.ExtKeyUsageClientAuth); err != nil {
		t.Fatal(err)
	}
	if err = r.verifyChain(certs[0], certs[1].Raw, x509.ExtKeyUsageServerAuth); err == nil {
		t.Fatal("user certificate verifies for server auth")
	}
	row, err := r.st.PKICertGet(context.Background(), serialHex(leaf))
	want := store.PKICert{Serial: serialHex(leaf), Kind: "user", NodeID: "alice", Subject: "nself-ci:user/alice", NotBefore: leaf.NotBefore.UnixNano(), NotAfter: leaf.NotAfter.UnixNano(), IssuedAt: now.UnixNano()}
	if err != nil || row != want {
		t.Fatalf("row %+v %v", row, err)
	}
	// a short user ttl does not set a renewal cap (users never renew) and keeps the lifetime
	_, short, _ := r.issueUser("bob", 30*time.Minute)
	if !short.NotAfter.Equal(now.Add(30 * time.Minute)) {
		t.Fatalf("short ttl %v", short.NotAfter)
	}
	row, _ = r.st.PKICertGet(context.Background(), serialHex(short))
	if row.CapAfter != 0 {
		t.Fatalf("user cap_after %d", row.CapAfter)
	}
	// rejections
	for _, id := range []string{"", "Bob", "a/b"} {
		_, err = r.ca.IssueUser(context.Background(), id, 0)
		wantReason(t, err, ReasonBadCSR)
	}
	_, err = r.ca.IssueUser(context.Background(), "carol", 8*day)
	wantReason(t, err, ReasonCertTTL)
	_, err = r.ca.IssueUser(context.Background(), "BAD", 8*day)
	wantReason(t, err, ReasonBadCSR)
	if n := r.count(`SELECT count(*) FROM pki_cert`); n != 2 {
		t.Fatalf("%d rows", n)
	}
}

func TestIssueServerLeaf(t *testing.T) {
	r := newRig(t)
	dns := []string{"ci.example.com", "ci2.example.com"}
	ips := []net.IP{net.ParseIP("10.1.2.3"), net.ParseIP("::1")}
	c, err := r.ca.IssueServer("coord-1", dns, ips, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	leaf := c.Leaf
	if len(c.Certificate) != 3 || string(c.Certificate[0]) != string(leaf.Raw) ||
		string(c.Certificate[1]) != string(r.ca.inter[KindCoordinator].cert.Raw) || string(c.Certificate[2]) != string(r.ca.RootDER()) {
		t.Fatal("chain order must be leaf, coordinator intermediate, root")
	}
	if strings.Join(leaf.DNSNames, ",") != "ci.example.com,ci2.example.com" || len(leaf.IPAddresses) != 2 || !leaf.IPAddresses[0].Equal(ips[0]) || !leaf.IPAddresses[1].Equal(ips[1]) {
		t.Fatalf("SANs %v %v", leaf.DNSNames, leaf.IPAddresses)
	}
	if len(leaf.URIs) != 1 || leaf.URIs[0].String() != "nself-ci:coordinator/coord-1" || leaf.Subject.CommonName != "coord-1" || leaf.Issuer.CommonName != "nself-ci coordinator CA" {
		t.Fatalf("identity %v", leaf.URIs)
	}
	if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth || leaf.KeyUsage != x509.KeyUsageDigitalSignature || leaf.IsCA {
		t.Fatalf("usage %v", leaf.ExtKeyUsage)
	}
	if !leaf.NotAfter.Equal(r.clk.now().Add(2*time.Hour)) || !leaf.NotBefore.Equal(r.clk.now().Add(-120*time.Second)) {
		t.Fatalf("validity %v .. %v", leaf.NotBefore, leaf.NotAfter)
	}
	if !c.PrivateKey.(*ecdsa.PrivateKey).PublicKey.Equal(leaf.PublicKey) {
		t.Fatal("private key does not match the leaf")
	}
	if err = r.verifyChain(leaf, c.Certificate[1], x509.ExtKeyUsageServerAuth); err != nil {
		t.Fatal(err)
	}
	if err = r.verifyChain(leaf, c.Certificate[1], x509.ExtKeyUsageClientAuth); err == nil {
		t.Fatal("server leaf verifies for client auth")
	}
	if err = leaf.VerifyHostname("10.1.2.3"); err != nil {
		t.Fatal(err)
	}
	if err = leaf.VerifyHostname("other.example.com"); err == nil {
		t.Fatal("hostname matched without a SAN")
	}
	if r.count(`SELECT count(*) FROM pki_cert`) != 0 {
		t.Fatal("the server leaf was recorded in the store")
	}
	// no SANs: none in the leaf; default ttl 7 days
	bare, err := r.ca.IssueServer("coord-1", nil, nil, 0)
	if err != nil || len(bare.Leaf.DNSNames) != 0 || len(bare.Leaf.IPAddresses) != 0 || !bare.Leaf.NotAfter.Equal(r.clk.now().Add(7*day)) {
		t.Fatalf("bare leaf: %v", err)
	}
	_, err = r.ca.IssueServer("Coord", nil, nil, 0)
	wantReason(t, err, ReasonBadCSR)
	_, err = r.ca.IssueServer("coord-1", nil, nil, 30*time.Second)
	wantReason(t, err, ReasonCertTTL)
	_, err = r.ca.IssueServer("coord-1", nil, nil, 8*day)
	wantReason(t, err, ReasonCertTTL)
}

func TestSANsFromURL(t *testing.T) {
	tests := []struct {
		in  string
		dns []string
		ips []string
		bad bool
	}{
		{"https://ci.example.com:8443/x?y=1", []string{"ci.example.com"}, nil, false},
		{"http://ci.example.com", []string{"ci.example.com"}, nil, false},
		{"https://127.0.0.1:8080", nil, []string{"127.0.0.1"}, false},
		{"https://[::1]:8080/", nil, []string{"::1"}, false},
		{"https://user:pw@host.example:1", []string{"host.example"}, nil, false},
		{"", nil, nil, true},
		{"https://", nil, nil, true},
		{"ci.example.com", nil, nil, true},
		{"/just/a/path", nil, nil, true},
		{"://bad", nil, nil, true},
		{"http://[bad", nil, nil, true},
	}
	for _, c := range tests {
		dns, ips, err := SANsFromURL(c.in)
		if c.bad {
			wantReason(t, err, ReasonBadChain)
			if dns != nil || ips != nil {
				t.Errorf("%q: values with an error", c.in)
			}
			continue
		}
		var gotIPs []string
		for _, ip := range ips {
			gotIPs = append(gotIPs, ip.String())
		}
		if err != nil || strings.Join(dns, ",") != strings.Join(c.dns, ",") || strings.Join(gotIPs, ",") != strings.Join(c.ips, ",") {
			t.Errorf("%q: %v %v %v", c.in, dns, gotIPs, err)
		}
		if (c.dns == nil) != (dns == nil) || (c.ips == nil) != (ips == nil) {
			t.Errorf("%q: nil-ness of dns/ips", c.in)
		}
	}
}

func TestIdentityOf(t *testing.T) {
	mk := func(uris ...string) *x509.Certificate {
		c := &x509.Certificate{}
		for _, s := range uris {
			u, err := url.Parse(s)
			if err != nil {
				t.Fatal(err)
			}
			c.URIs = append(c.URIs, u)
		}
		return c
	}
	tests := []struct {
		name string
		cert *x509.Certificate
		id   string
		kind Kind
	}{
		{"node", mk("nself-ci:node/n1"), "n1", KindNode},
		{"user", mk("nself-ci:user/u.1"), "u.1", KindUser},
		{"coordinator", mk("nself-ci:coordinator/c-1"), "c-1", KindCoordinator},
		{"one-letter id", mk("nself-ci:node/x"), "x", KindNode},
		{"no URIs", &x509.Certificate{}, "", ""},
		{"empty id", mk("nself-ci:node/"), "", ""},
		{"no slash", mk("nself-ci:node"), "", ""},
		{"other kind", mk("nself-ci:admin/x"), "", ""},
		{"prefix of a kind", mk("nself-ci:nodes/x"), "", ""},
		{"other scheme", mk("https://node/x"), "", ""},
		{"other scheme opaque", mk("mailto:node/x"), "", ""},
		{"second URI matches", mk("https://example.com/a", "nself-ci:user/u2"), "u2", KindUser},
		{"first match wins", mk("nself-ci:node/a", "nself-ci:user/b"), "a", KindNode},
		{"non-matching nself-ci then match", mk("nself-ci:bogus/a", "nself-ci:coordinator/b"), "b", KindCoordinator},
	}
	for _, c := range tests {
		id, kind := IdentityOf(c.cert)
		if id != c.id || kind != c.kind {
			t.Errorf("%s: (%q,%q), want (%q,%q)", c.name, id, kind, c.id, c.kind)
		}
	}
}

func TestRenewAtIsTwoThirdsOfLife(t *testing.T) {
	nb := time.Unix(1_000_000, 0)
	tests := []struct {
		life time.Duration
		want time.Duration
	}{
		{90 * time.Second, 60 * time.Second},
		{3 * time.Hour, 2 * time.Hour},
		{100 * time.Second, 66_666_666_666 * time.Nanosecond},
		{7*day + 120*time.Second, (7*day + 120*time.Second) * 2 / 3},
	}
	for _, c := range tests {
		got := RenewAt(&x509.Certificate{NotBefore: nb, NotAfter: nb.Add(c.life)})
		if !got.Equal(nb.Add(c.want)) {
			t.Errorf("life %v: %v, want +%v", c.life, got.Sub(nb), c.want)
		}
	}
	r := newRig(t)
	n := r.issueNode("n1", 0)
	if got := RenewAt(n.Cert); !got.Equal(n.Cert.NotBefore.Add((7*day + 120*time.Second) * 2 / 3)) {
		t.Fatalf("issued cert renews at %v", got)
	}
}

func TestRenew(t *testing.T) {
	ctx := context.Background()
	type env struct {
		r    *rig
		peer *x509.Certificate
	}
	fresh := func(t *testing.T) env {
		r := newRig(t)
		n := r.issueNode("n1", 0)
		return env{r, n.Cert}
	}
	t.Run("ok", func(t *testing.T) {
		e := fresh(t)
		e.r.clk.advance(5 * day)
		csr, key := newCSR(t)
		is, err := e.r.ca.Renew(ctx, e.peer, csr)
		if err != nil {
			t.Fatal(err)
		}
		cert, _ := x509.ParseCertificate(is.CertDER)
		id, kind := IdentityOf(cert)
		if id != "n1" || kind != KindNode || is.Serial == serialHex(e.peer) || !cert.PublicKey.(*ecdsa.PublicKey).Equal(&key.PublicKey) {
			t.Fatalf("renewed %s %s", id, kind)
		}
		if !is.NotAfter.Equal(e.r.clk.now().Add(7 * day)) {
			t.Fatalf("NotAfter %v", is.NotAfter)
		}
		row, _ := e.r.st.PKICertGet(ctx, is.Serial)
		if row.OfferID != "offer-n1" || row.CapAfter != 0 || row.NodeID != "n1" || row.Kind != "node" {
			t.Fatalf("row %+v", row)
		}
		if n := e.r.count(`SELECT count(*) FROM pki_cert WHERE node_id='n1'`); n != 2 {
			t.Fatalf("%d rows", n)
		}
	})
	t.Run("last second before expiry", func(t *testing.T) {
		e := fresh(t)
		e.r.clk.set(e.peer.NotAfter.Add(-time.Second))
		csr, _ := newCSR(t)
		if _, err := e.r.ca.Renew(ctx, e.peer, csr); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("expired", func(t *testing.T) {
		for _, d := range []time.Duration{0, time.Second, 30 * day} {
			e := fresh(t)
			e.r.clk.set(e.peer.NotAfter.Add(d))
			csr, _ := newCSR(t)
			_, err := e.r.ca.Renew(ctx, e.peer, csr)
			wantReason(t, err, ReasonExpired)
			if e.r.count(`SELECT count(*) FROM pki_cert`) != 1 {
				t.Fatal("expired renew issued")
			}
		}
	})
	t.Run("denied", func(t *testing.T) {
		e := fresh(t)
		if _, err := e.r.ca.Revoke(ctx, "cli:admin", "n1", "gone"); err != nil {
			t.Fatal(err)
		}
		csr, _ := newCSR(t)
		_, err := e.r.ca.Renew(ctx, e.peer, csr)
		wantReason(t, err, ReasonRevoked)
	})
	t.Run("short-lived lineage", func(t *testing.T) {
		r := newRig(t)
		n := r.issueNode("n1", 30*time.Minute)
		csr, _ := newCSR(t)
		_, err := r.ca.Renew(ctx, n.Cert, csr)
		wantReason(t, err, ReasonRenewCap)
		if r.count(`SELECT count(*) FROM pki_cert`) != 1 {
			t.Fatal("renew_cap issued")
		}
	})
	t.Run("user certificate", func(t *testing.T) {
		r := newRig(t)
		_, leaf, _ := r.issueUser("alice", 0)
		csr, _ := newCSR(t)
		_, err := r.ca.Renew(ctx, leaf, csr)
		wantReason(t, err, ReasonWrongKind)
	})
	t.Run("no identity", func(t *testing.T) {
		r := newRig(t)
		csr, _ := newCSR(t)
		_, err := r.ca.Renew(ctx, &x509.Certificate{}, csr)
		wantReason(t, err, ReasonWrongKind)
	})
	t.Run("wrong kind is checked before expiry", func(t *testing.T) {
		r := newRig(t)
		_, leaf, _ := r.issueUser("alice", 0)
		r.clk.advance(30 * day)
		csr, _ := newCSR(t)
		_, err := r.ca.Renew(ctx, leaf, csr)
		wantReason(t, err, ReasonWrongKind)
	})
	t.Run("unknown serial", func(t *testing.T) {
		r := newRig(t)
		other := newRig(t)
		n := other.issueNode("n1", 0)
		csr, _ := newCSR(t)
		_, err := r.ca.Renew(ctx, n.Cert, csr)
		wantReason(t, err, ReasonUnknownNode)
	})
	t.Run("bad csr", func(t *testing.T) {
		e := fresh(t)
		_, err := e.r.ca.Renew(ctx, e.peer, []byte("junk"))
		wantReason(t, err, ReasonBadCSR)
	})
	t.Run("store closed", func(t *testing.T) {
		e := fresh(t)
		_ = e.r.st.Close()
		csr, _ := newCSR(t)
		_, err := e.r.ca.Renew(ctx, e.peer, csr)
		wantReason(t, err, ReasonUnknownNode)
	})
}

func TestRevokeDeniesEveryCertificateOfTheNode(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	a, b, other := r.issueNode("n1", 0), r.issueNode("n1", time.Hour), r.issueNode("n2", 0)
	added, err := r.ca.Revoke(ctx, "cli:admin", "n1", "stolen")
	if err != nil || added != 2 {
		t.Fatalf("added %d %v", added, err)
	}
	for _, c := range []issued{a, b} {
		if d, _ := r.st.PKIIsDenied(ctx, c.Serial); !d {
			t.Fatalf("%s not denied", c.Serial)
		}
	}
	if d, _ := r.st.PKIIsDenied(ctx, other.Serial); d {
		t.Fatal("another node's certificate was denied")
	}
	if got := r.audit("pki.deny"); len(got) != 1 || got[0] != [3]string{"cli:admin", "n1", "serials=2 reason=stolen"} {
		t.Fatalf("audit %v", got)
	}
	if added, err = r.ca.Revoke(ctx, "cli:admin", "n1", "again"); err != nil || added != 0 {
		t.Fatalf("second revoke %d %v", added, err)
	}
	if added, err = r.ca.Revoke(ctx, "cli:admin", "ghost", "none"); err != nil || added != 0 {
		t.Fatalf("unknown node %d %v", added, err)
	}
	if _, err = r.ca.Revoke(ctx, "", "n1", "x"); err == nil {
		t.Fatal("revoke without an actor accepted")
	}
	_ = r.st.Close()
	if _, err = r.ca.Revoke(ctx, "cli:admin", "n1", "x"); err == nil {
		t.Fatal("revoke on a closed store succeeded")
	}
}

func TestRevokeHookDeniesInsideNodeRevoke(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.addNode("n1")
	r.addNode("n2")
	a, b, other := r.issueNode("n1", 0), r.issueNode("n1", 0), r.issueNode("n2", 0)
	hook := RevokeHook(r.clk.now)
	r.clk.advance(time.Hour)
	res, err := r.st.NodeRevoke(ctx, "n1", "cli:admin", []store.RevokeHook{hook})
	if err != nil || res.Removed {
		t.Fatalf("revoke: %+v %v", res, err)
	}
	for _, c := range []issued{a, b} {
		if d, _ := r.st.PKIIsDenied(ctx, c.Serial); !d {
			t.Fatalf("%s not denied by the hook", c.Serial)
		}
	}
	if d, _ := r.st.PKIIsDenied(ctx, other.Serial); d {
		t.Fatal("hook denied another node")
	}
	var reason, node string
	var at int64
	if err = r.ro().QueryRow(`SELECT reason,node_id,denied_at FROM pki_denied WHERE serial=?`, a.Serial).Scan(&reason, &node, &at); err != nil || reason != "node_revoked" || node != "n1" || at != r.clk.now().UnixNano() {
		t.Fatalf("deny row: %q %q %d %v", reason, node, at, err)
	}
	if r.count(`SELECT count(*) FROM pki_denied`) != 2 {
		t.Fatal("deny list size")
	}
	// idempotent
	if _, err = r.st.NodeRevoke(ctx, "n1", "cli:admin", []store.RevokeHook{hook}); err != nil {
		t.Fatal(err)
	}
	if r.count(`SELECT count(*) FROM pki_denied`) != 2 {
		t.Fatal("hook added duplicates")
	}
	// Check sees the node as revoked and the deny list as holding its certs
	g := NewGuard(r.st)
	wantReason(t, g.Check(ctx, a.Cert), ReasonRevoked)
	if err = g.Check(ctx, other.Cert); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedKeysAreFresh(t *testing.T) {
	r := newRig(t)
	_, l1, _ := r.issueUser("a", 0)
	_, l2, _ := r.issueUser("a", 0)
	if l1.PublicKey.(*ecdsa.PublicKey).Equal(l2.PublicKey) || serialHex(l1) == serialHex(l2) {
		t.Fatal("user keys or serials repeat")
	}
}

// A user certificate whose id equals a node id is not the node's certificate: revoking the node (through
// CA.Revoke or the lifecycle hook) must not deny the user.
func TestRevokeLeavesUserCertificateOfTheSameIDAlone(t *testing.T) {
	ctx := context.Background()
	t.Run("CA.Revoke", func(t *testing.T) {
		r := newRig(t)
		nodeCert := r.issueNode("shared", 0)
		_, user, _ := r.issueUser("shared", 0)
		added, err := r.ca.Revoke(ctx, "cli:admin", "shared", "x")
		if err != nil || added != 1 {
			t.Fatalf("added %d %v", added, err)
		}
		if d, _ := r.st.PKIIsDenied(ctx, nodeCert.Serial); !d {
			t.Fatal("node certificate not denied")
		}
		if d, _ := r.st.PKIIsDenied(ctx, serialHex(user)); d {
			t.Fatal("the user certificate was denied by a node revoke")
		}
	})
	t.Run("RevokeHook through NodeRevoke", func(t *testing.T) {
		r := newRig(t)
		r.addNode("shared")
		nodeCert := r.issueNode("shared", 0)
		_, user, _ := r.issueUser("shared", 0)
		if _, err := r.st.NodeRevoke(ctx, "shared", "cli:admin", []store.RevokeHook{RevokeHook(r.clk.now)}); err != nil {
			t.Fatal(err)
		}
		if d, _ := r.st.PKIIsDenied(ctx, nodeCert.Serial); !d {
			t.Fatal("node certificate not denied")
		}
		if d, _ := r.st.PKIIsDenied(ctx, serialHex(user)); d {
			t.Fatal("the user certificate was denied by the lifecycle hook")
		}
		if r.count(`SELECT count(*) FROM pki_denied`) != 1 {
			t.Fatal("deny list size")
		}
		// the user's own certificate still passes Check
		if err := NewGuard(r.st).Check(ctx, user); err != nil {
			t.Fatalf("user check: %v", err)
		}
	})
}
