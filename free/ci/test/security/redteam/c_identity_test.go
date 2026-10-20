//go:build security

package redteam

// Purpose: red team group C, identity and authority (TM-21 stolen credentials, TM-23 node
// certificate on the admin API and a user certificate on the node listener, TM-24 unauthorised
// control-plane action, TM-39 key pinning). The attacker uses wrong-kind certificates over a real
// TLS handshake, token spellings, a scoped admin that tries a coordinator-wide action, author
// spellings against the non-author rule and a job token with edited parts.
// Inputs:  a real SQLite store in t.TempDir, a real pki CA, a real roles.Service, throwaway keys.
// Outputs: pass (the control held) or fail (the attack worked).
// Constraints: loopback and net.Pipe only; clocks are injected; no network.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type cRig struct {
	t   *testing.T
	st  *store.Store
	ca  *pki.CA
	svc *roles.Service
	now time.Time
}

func cNewRig(t *testing.T) *cRig {
	t.Helper()
	r := &cRig{t: t, now: time.Unix(1_800_000_000, 0)}
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{Clock: func() time.Time { return r.now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r.st = st
	r.ca, err = pki.New(context.Background(), pki.Options{Store: st, DataDir: dir, Now: func() time.Time { return r.now }})
	if err != nil {
		t.Fatal(err)
	}
	r.svc = roles.New(st, &audit.Log{St: st})
	r.svc.Now = func() time.Time { return r.now }
	r.svc.Self = "alice"
	return r
}

func (r *cRig) op() roles.Actor { return r.svc.Operator("cli:alice") }

func (r *cRig) nodeCert(id string) tls.Certificate {
	r.t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		r.t.Fatal(err)
	}
	is, err := r.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id})
	if err != nil {
		r.t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(is.CertDER)
	return tls.Certificate{Certificate: append(append([][]byte{}, is.Chain...), is.RootDER), PrivateKey: key, Leaf: leaf}
}

func (r *cRig) userCert(id string) tls.Certificate {
	r.t.Helper()
	b, err := r.ca.IssueUser(context.Background(), id, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(append(append([]byte{}, b.CertPEM...), b.RootPEM...), b.KeyPEM)
	if err != nil {
		r.t.Fatal(err)
	}
	pair.Leaf, _ = x509.ParseCertificate(pair.Certificate[0])
	return pair
}

func (r *cRig) grant(p string, role roles.Role, projects ...string) {
	r.t.Helper()
	if err := r.svc.Grant(context.Background(), r.op(), roles.Request{Principal: p, Role: role, Projects: projects}); err != nil {
		r.t.Fatal(err)
	}
}

func (r *cRig) token(p string, role roles.Role, projects ...string) (string, roles.Actor) {
	r.t.Helper()
	tok, _, err := r.svc.IssueToken(context.Background(), r.op(), roles.Request{Principal: p, Role: role, Projects: projects})
	if err != nil {
		r.t.Fatal(err)
	}
	a, err := r.svc.ActorFromToken(context.Background(), tok)
	if err != nil {
		r.t.Fatal(err)
	}
	return tok, a
}

// cHandshake runs a TLS handshake over net.Pipe and returns the server-side error.
func cHandshake(srv, cli *tls.Config) (serr error) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	_ = a.SetDeadline(time.Now().Add(5 * time.Second))
	_ = b.SetDeadline(time.Now().Add(5 * time.Second))
	done := make(chan error, 1)
	go func() {
		c := tls.Client(b, cli)
		err := c.Handshake()
		if err == nil {
			buf := make([]byte, 1)
			_, err = c.Read(buf)
		}
		done <- err
	}()
	s := tls.Server(a, srv)
	serr = s.Handshake()
	_ = a.Close()
	<-done
	return serr
}

// TestC23WrongKindCertificates (TM-23): a node certificate on the user listener, a user certificate and
// a coordinator leaf on the node listener, and every kind handed to ActorFromPeer.
func TestC23WrongKindCertificates(t *testing.T) {
	r := cNewRig(t)
	r.grant("mallory", roles.Admin, roles.AllProjects) // the node id below collides with a granted user id
	node, user := r.nodeCert("mallory"), r.userCert("mallory")
	leaf, err := r.ca.IssueServer("coord-1", nil, []net.IP{net.ParseIP("127.0.0.1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	g := pki.NewGuard(r.st)
	_ = g.Refresh(context.Background())
	nodeSrv, _ := r.ca.TLSServerConfig(pki.KindNode, leaf, g)
	userSrv, _ := r.ca.TLSServerConfig(pki.KindUser, leaf, g)
	cli := func(c *tls.Certificate) *tls.Config {
		cfg, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: r.ca.RootDER(), Cert: c, ServerName: "127.0.0.1", Now: func() time.Time { return r.now }})
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	if err := cHandshake(userSrv, cli(&user)); err != nil {
		t.Fatalf("control: the user certificate was refused by the user listener: %v", err)
	}
	for name, c := range map[string]struct {
		srv  *tls.Config
		cert tls.Certificate
	}{
		"node cert on the user listener":        {userSrv, node},
		"user cert on the node listener":        {nodeSrv, user},
		"coordinator leaf on the node listener": {nodeSrv, leaf},
		"coordinator leaf on the user listener": {userSrv, leaf},
	} {
		c := c
		if err := cHandshake(c.srv, cli(&c.cert)); err == nil {
			t.Errorf("%s: the handshake completed", name)
		}
	}
	// Defence in depth: past the TLS layer the role service refuses non-user kinds too, even for an id that holds a grant.
	for name, cert := range map[string]tls.Certificate{"node": node, "coordinator": leaf} {
		if _, err := r.svc.ActorFromPeer(context.Background(), cert.Leaf); roles.ReasonOf(err) != roles.ReasonNodeKind {
			t.Errorf("%s certificate at ActorFromPeer: %v", name, err)
		}
	}
	if _, err := r.ca.Renew(context.Background(), user.Leaf, nil); err == nil {
		t.Error("a user certificate renewed as a node")
	}
	for _, id := range []string{"", "A", "a/b", "a b", "a\n", ".a", "-a", strings.Repeat("a", 64), "аdmin", "node/x", "a@b", "../x"} {
		if pki.ValidID(id) {
			t.Errorf("identifier %q is valid", id)
		}
	}
}

// TestC21TokenSpellings (TM-21): the scoped user token is found only by its exact text; the stored
// digest is not a credential; an expired or revoked token is dead at the next decision, at the exact
// expiry second included.
func TestC21TokenSpellings(t *testing.T) {
	r := cNewRig(t)
	ctx := context.Background()
	tok, actor := r.token("bob", roles.Release, "p1")
	toks, err := r.svc.Tokens(ctx, r.op())
	if err != nil || len(toks) != 1 {
		t.Fatalf("tokens: %v %v", toks, err)
	}
	for name, v := range map[string]string{"stored digest": toks[0].Digest, "token id": toks[0].ID, "upper": strings.ToUpper(tok), "trailing space": tok + " ", "leading space": " " + tok,
		"newline": tok + "\n", "prefix": tok[:len(tok)-1], "Bearer": "Bearer " + tok, "nul": tok + "\x00", "empty": "", "doubled": tok + tok} {
		if _, err := r.svc.ActorFromToken(ctx, v); err == nil {
			t.Errorf("%s: accepted as a token", name)
		}
	}
	if err := roles.AuthorizeAction(ctx, actor, roles.ActAuditView, ""); err == nil {
		t.Error("a project-scoped token read the coordinator-wide audit log")
	}
	r.now = time.Unix(0, toks[0].ExpiresAt) // the exact expiry instant
	if _, err := r.svc.ActorFromToken(ctx, tok); roles.ReasonOf(err) != roles.ReasonTokenExpired {
		t.Errorf("at the expiry instant: %v", err)
	}
	if err := roles.AuthorizeAction(ctx, actor, roles.ActLogsView, "p1"); err == nil {
		t.Error("an Actor resolved before expiry still authorizes at the expiry instant")
	}
	r.now = time.Unix(1_800_000_000, 0)
	if _, err := r.svc.RevokeToken(ctx, r.op(), toks[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := roles.AuthorizeAction(ctx, actor, roles.ActLogsView, "p1"); err == nil {
		t.Error("an Actor resolved before the revocation still authorizes")
	}
	for _, ttl := range []time.Duration{-time.Hour, time.Second, 1 << 62, 400 * 24 * time.Hour} {
		if _, _, err := r.svc.IssueToken(ctx, r.op(), roles.Request{Principal: "carol", Role: roles.Member, Projects: []string{"p1"}, TTL: ttl}); err == nil {
			t.Errorf("a token with ttl %s was issued", ttl)
		}
	}
}

// TestC24ActionsByRole (TM-24): the actions the threat model puts behind admin or release refuse every
// lower role, an unknown or respelled action refuses, and a project-scoped admin cannot act coordinator-wide.
func TestC24ActionsByRole(t *testing.T) {
	r := cNewRig(t)
	ctx := context.Background()
	_, member := r.token("m", roles.Member, roles.AllProjects)
	_, release := r.token("r", roles.Release, roles.AllProjects)
	_, scopedAdmin := r.token("sa", roles.Admin, "p1")
	admin := map[string]bool{}
	for _, a := range []string{roles.ActPolicyTeamSet, roles.ActRunnerRegister, roles.ActRunnerRevoke, roles.ActNodeTrustSet, roles.ActProviderAuthorize, roles.ActPolicySet,
		roles.ActPrivacyZoneSet, roles.ActSecretAuthorize, roles.ActSpendApprove, roles.ActRolesManage, roles.ActTokenManage} {
		admin[a] = true
		for name, a2 := range map[string]roles.Actor{"member": member, "release": release} {
			if err := roles.AuthorizeAction(ctx, a2, a, ""); err == nil {
				t.Errorf("%s ran %s", name, a)
			}
		}
		if err := roles.AuthorizeAction(ctx, scopedAdmin, a, ""); err == nil {
			t.Errorf("a project-scoped admin ran %s coordinator-wide", a)
		}
	}
	for _, a := range []string{roles.ActRunApprove, roles.ActReleaseStart, roles.ActAuditView} {
		if err := roles.AuthorizeAction(ctx, member, a, "p1"); err == nil {
			t.Errorf("member ran %s", a)
		}
	}
	for _, a := range roles.Actions() {
		if !admin[a] && a != roles.ActRunApprove && a != roles.ActReleaseStart && a != roles.ActAuditView && a != roles.ActLogsView && a != roles.ActArtifactsView {
			t.Errorf("action %s has no expectation in this test: the catalogue grew", a)
		}
	}
	for _, a := range []string{"", " ", "Policy.Set", "policy.set ", "policy.set\x00", "policy.*", "*", "policy.set;", "POLICY.SET", "policy..set"} {
		if err := roles.AuthorizeAction(ctx, r.op(), a, ""); err == nil {
			t.Errorf("unknown action %q authorized even for the operator", a)
		}
	}
	if err := roles.AuthorizeAction(ctx, roles.Actor{}, roles.ActLogsView, "p1"); err == nil {
		t.Error("the zero Actor authorized")
	}
	if err := r.svc.Grant(ctx, scopedAdmin, roles.Request{Principal: "x", Role: roles.Admin, Projects: []string{roles.AllProjects}}); err == nil {
		t.Error("a project-scoped admin granted a coordinator-wide admin")
	}
	if _, _, err := r.svc.IssueToken(ctx, scopedAdmin, roles.Request{Principal: "x", Role: roles.Admin, Projects: []string{roles.AllProjects}}); err == nil {
		t.Error("a project-scoped admin minted a coordinator-wide admin token")
	}
}

// TestC34AuthorSpellings (TM-34): the author of a run cannot approve it under any spelling of the same
// principal, a project it is not scoped to refuses, and an unknown author refuses.
func TestC34AuthorSpellings(t *testing.T) {
	r := cNewRig(t)
	ctx := context.Background()
	_, bob := r.token("bob", roles.Release, "p1")
	for _, author := range []string{"bob", "BOB", " Bob ", "user:bob", "user:BOB", "token:tok-abc:bob", "token:tok-abc:Bob", "operator:cli:bob", "cli:bob", "operator:user:bob", "\tbob\n"} {
		err := roles.AuthorizeAction(roles.WithAuthor(ctx, author), bob, roles.ActRunApprove, "p1")
		if roles.ReasonOf(err) != roles.ReasonSelfApproval {
			t.Errorf("author %q: %v", author, err)
		}
	}
	if err := roles.AuthorizeAction(roles.WithAuthor(ctx, "mallory"), bob, roles.ActRunApprove, "p1"); err != nil {
		t.Fatalf("control: a non-author release approver was refused: %v", err)
	}
	for _, c := range []struct{ author, project string }{{"", "p1"}, {"mallory", "p2"}, {"mallory", "P1"}, {"mallory", "p1 "}, {"mallory", ""}, {"mallory", "*"}} {
		if err := roles.AuthorizeAction(roles.WithAuthor(ctx, c.author), bob, roles.ActRunApprove, c.project); err == nil {
			t.Errorf("author %q project %q: approved", c.author, c.project)
		}
	}
}
