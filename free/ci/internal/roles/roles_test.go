package roles

// Purpose: tests of the role matrix, identities, tokens and the audit rows of decisions and mutations.
// Inputs:  a real SQLite store in t.TempDir(), a real pki CA for certificates, an injected clock.
// Outputs: pass or fail; no network.
// Constraints: the matrix test covers every catalogued action x role x operator; mutants under
//          testdata/mutants are killed by TestAuthorize* and TestActions*.

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var epoch = time.Unix(1_800_000_000, 0)

type rig struct {
	t    *testing.T
	st   *store.Store
	svc  *Service
	dir  string
	path string
	now  time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{t: t, dir: t.TempDir(), now: epoch}
	r.path = filepath.Join(r.dir, "state.db")
	st, err := store.Open(r.path, store.Options{Clock: func() time.Time { return r.now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r.st = st
	r.svc = New(st, &audit.Log{St: st})
	r.svc.Now = func() time.Time { return r.now }
	r.svc.Self = "alice"
	return r
}

func (r *rig) op() Actor { return r.svc.Operator("cli:alice") }

func (r *rig) auditRows() []store.AuditEntry {
	rows, err := r.st.AuditEntries(context.Background(), 0, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	return rows
}

func (r *rig) grant(principal string, role Role, projects ...string) {
	r.t.Helper()
	if err := r.svc.Assign(context.Background(), r.op(), Request{Principal: principal, Role: role, Projects: projects}); err != nil {
		r.t.Fatal(err)
	}
}

// user grants role to principal on every project and returns the actor a certificate login would give.
func (r *rig) user(principal string, role Role) Actor {
	r.t.Helper()
	r.grant(principal, role, AllProjects)
	return Actor{id: principal, kind: KindUser, role: role, projects: []string{AllProjects}, svc: r.svc}
}

func (r *rig) tokenActor(principal string, role Role, projects ...string) Actor {
	r.t.Helper()
	tok, _, err := r.svc.IssueToken(context.Background(), r.op(), Request{Principal: principal, Role: role, Projects: projects})
	if err != nil {
		r.t.Fatal(err)
	}
	a, err := r.svc.ActorFromToken(context.Background(), tok)
	if err != nil {
		r.t.Fatal(err)
	}
	return a
}

func TestAuthorizeRoleOrder(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for _, tc := range []struct {
		have, need Role
		ok         bool
	}{
		{Member, Member, true}, {Member, Release, false}, {Member, Admin, false},
		{Release, Member, true}, {Release, Release, true}, {Release, Admin, false},
		{Admin, Member, true}, {Admin, Release, true}, {Admin, Admin, true},
		{Role("root"), Member, false}, {Member, Role("root"), false},
	} {
		a := Actor{id: "u", kind: KindUser, role: tc.have, projects: []string{AllProjects}, svc: r.svc}
		err := a.Can(tc.need, "p1")
		if (err == nil) != tc.ok {
			t.Errorf("%s needs %s: err=%v want ok=%v", tc.have, tc.need, err, tc.ok)
		}
		if err != nil && ReasonOf(err) == "" {
			t.Errorf("a refusal is an E687 error with a reason: %v", err)
		}
	}
	if err := Authorize(ctx, Actor{}, Member, ""); ReasonOf(err) != ReasonNoActor {
		t.Fatalf("the zero actor is refused: %v", err)
	}
	if err := Authorize(ctx, r.user("ursula", Admin), Role("root"), ""); ReasonOf(err) != ReasonUnknownRole {
		t.Fatalf("an unknown role is refused: %v", err)
	}
	if rows := r.auditRows(); len(rows) == 0 || !strings.Contains(rows[len(rows)-1].Detail, "result=deny") {
		t.Fatalf("an unknown-role decision is audited: %+v", rows)
	}
}

func TestAuthorizeProjectScope(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	rel := r.tokenActor("bob", Release, "p1", "p2")
	if err := Authorize(ctx, rel, Release, "p1"); err != nil {
		t.Fatalf("allowlisted project: %v", err)
	}
	if err := Authorize(ctx, rel, Release, "p3"); ReasonOf(err) != ReasonProject {
		t.Fatalf("other project must be refused: %v", err)
	}
	if err := Authorize(ctx, rel, Release, ""); ReasonOf(err) != ReasonProject {
		t.Fatalf("a scoped actor cannot do a project-wide release action: %v", err)
	}
	if err := Authorize(ctx, rel, Member, ""); err != nil {
		t.Fatalf("a member-level project-less check passes for a scoped actor: %v", err)
	}
	all := r.tokenActor("carol", Admin, AllProjects)
	if err := Authorize(ctx, all, Admin, "anything"); err != nil {
		t.Fatalf("* covers every project: %v", err)
	}
}

// TestActionsMatrix checks every catalogued action for every role and the operator.
func TestActionsMatrix(t *testing.T) {
	r := newRig(t)
	ctx := WithAuthor(context.Background(), "someone-else")
	want := map[string]Role{
		ActPolicyTeamSet: Admin, ActRunnerRegister: Admin, ActRunnerRevoke: Admin, ActNodeTrustSet: Admin, ActProviderAuthorize: Admin,
		ActPolicySet: Admin, ActPrivacyZoneSet: Admin, ActSecretAuthorize: Admin, ActRunApprove: Release, ActSpendApprove: Admin,
		ActReleaseStart: Release, ActLogsView: Member, ActArtifactsView: Member, ActAuditView: Release, ActRolesManage: Admin, ActTokenManage: Admin,
	}
	if got := Actions(); len(got) != len(want) {
		t.Fatalf("the catalogue has %d actions, the table %d: %v", len(got), len(want), got)
	}
	users := map[Role]Actor{Member: r.user("u-member", Member), Release: r.user("u-release", Release), Admin: r.user("u-admin", Admin)}
	for action, min := range want {
		if got, ok := Needs(action); !ok || got != min {
			t.Errorf("%s needs %s, catalogue says %s", action, min, got)
		}
		for _, have := range []Role{Member, Release, Admin} {
			a := users[have]
			err := AuthorizeAction(ctx, a, action, "p1")
			if ok := have.Rank() >= min.Rank(); (err == nil) != ok {
				t.Errorf("%s by %s: err=%v want allowed=%v", action, have, err, ok)
			}
		}
		if err := AuthorizeAction(ctx, r.op(), action, "p1"); err != nil {
			t.Errorf("the operator may do %s: %v", action, err)
		}
	}
	member := users[Member]
	for _, a := range []string{ActRunnerRegister, ActRunnerRevoke, ActPolicySet, ActRunApprove, ActSpendApprove, ActReleaseStart} {
		if ReasonOf(AuthorizeAction(ctx, member, a, "p1")) != ReasonRole {
			t.Errorf("a member must not do %s", a)
		}
	}
	rel := users[Release]
	for _, a := range []string{ActNodeTrustSet, ActSpendApprove} {
		if AuthorizeAction(ctx, rel, a, "p1") == nil {
			t.Errorf("only admin does %s", a)
		}
	}
	if ReasonOf(AuthorizeAction(ctx, rel, "no.such.action", "p1")) != ReasonUnknownAct {
		t.Fatal("an unknown action is refused")
	}
}

func TestActionsReleaseStartAndApproveRules(t *testing.T) {
	r := newRig(t)
	rel := r.tokenActor("bob", Release, "p1")
	ctx := WithAuthor(context.Background(), "author-1")
	if err := AuthorizeAction(ctx, rel, ActReleaseStart, "p1"); err != nil {
		t.Fatalf("release starts a release job on an allowlisted project: %v", err)
	}
	if ReasonOf(AuthorizeAction(ctx, rel, ActReleaseStart, "p2")) != ReasonProject {
		t.Fatal("release.start on a project outside the allowlist is refused")
	}
	if ReasonOf(AuthorizeAction(ctx, rel, ActReleaseStart, "")) != ReasonProject {
		t.Fatal("release.start needs a project")
	}
	wide := r.tokenActor("wendy", Release, AllProjects)
	for _, a := range []string{ActReleaseStart, ActRunApprove, ActLogsView, ActArtifactsView} {
		if ReasonOf(AuthorizeAction(ctx, wide, a, "")) != ReasonProject {
			t.Fatalf("%s names a project even for an actor allowed on every project", a)
		}
	}
	if err := AuthorizeAction(ctx, rel, ActRunApprove, "p1"); err != nil {
		t.Fatalf("release approves a run it did not author: %v", err)
	}
	own := WithAuthor(context.Background(), "bob")
	if ReasonOf(AuthorizeAction(own, rel, ActRunApprove, "p1")) != ReasonSelfApproval {
		t.Fatal("no self-approval")
	}
	if ReasonOf(AuthorizeAction(context.Background(), rel, ActRunApprove, "p1")) != ReasonNoAuthor {
		t.Fatal("an unknown author fails closed")
	}
}

func TestAuthorizeAuditsDecisions(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	member := r.tokenActor("m", Member, "p1")
	base := len(r.auditRows())
	if err := AuthorizeAction(ctx, member, ActLogsView, "p1"); err != nil {
		t.Fatal(err)
	}
	if got := len(r.auditRows()); got != base {
		t.Fatalf("a member-level read writes no row, wrote %d", got-base)
	}
	if err := AuthorizeAction(ctx, member, ActPolicySet, "p1"); err == nil {
		t.Fatal("member may not set policy")
	}
	if err := Authorize(ctx, r.op(), Admin, ""); err != nil {
		t.Fatal(err)
	}
	rows := r.auditRows()[base:]
	if len(rows) != 2 {
		t.Fatalf("want exactly one row per audited decision (2), got %d", len(rows))
	}
	if !strings.Contains(rows[0].Detail, "result=deny") || !strings.Contains(rows[0].Detail, "reason=role") || !strings.HasPrefix(rows[0].Actor, "token:") {
		t.Errorf("deny row: %+v", rows[0])
	}
	if !strings.Contains(rows[1].Detail, "result=allow") || rows[1].Actor != "operator:cli:alice" {
		t.Errorf("the operator is allowed and still audited: %+v", rows[1])
	}
	if rep, _ := (&audit.Log{St: r.st}).Verify(ctx, 0); !rep.OK() || rep.Unsealed != 0 {
		t.Fatalf("decision rows are sealed: %+v", rep)
	}
}

func TestIdentityTokenStates(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	tok, row, err := r.svc.IssueToken(ctx, r.op(), Request{Principal: "bob", Role: Release, Projects: []string{"p1"}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	a, err := r.svc.ActorFromToken(ctx, tok)
	if err != nil || a.ID() != "bob" || a.Role() != Release || len(a.Projects()) != 1 || a.Kind() != KindToken {
		t.Fatalf("a valid token maps to its role and projects: %+v, %v", a, err)
	}
	if _, err = r.svc.ActorFromToken(ctx, tok+"x"); ReasonOf(err) != ReasonTokenUnknown {
		t.Fatalf("unknown token: %v", err)
	}
	r.now = r.now.Add(2 * time.Hour)
	if _, err = r.svc.ActorFromToken(ctx, tok); ReasonOf(err) != ReasonTokenExpired {
		t.Fatalf("expired token: %v", err)
	}
	r.now = epoch
	if found, err := r.svc.RevokeToken(ctx, r.op(), row.ID); err != nil || !found {
		t.Fatalf("revoke: %v %v", found, err)
	}
	if _, err = r.svc.ActorFromToken(ctx, tok); ReasonOf(err) != ReasonTokenRevoked {
		t.Fatalf("revoked token: %v", err)
	}
	if found, _ := r.svc.RevokeToken(ctx, r.op(), row.ID); found {
		t.Fatal("a second revoke finds nothing")
	}
	if _, _, err = r.svc.IssueToken(ctx, r.op(), Request{Principal: "bob", Role: Admin, Projects: []string{"p"}, TTL: 100 * 24 * time.Hour * 10}); err == nil {
		t.Fatal("a lifetime above the cap is refused")
	}
}

func TestTokenStorageHoldsOnlyTheDigest(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	tok, _, err := r.svc.IssueToken(ctx, r.op(), Request{Principal: "bob", Role: Admin, Projects: []string{AllProjects}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 43 || strings.ContainsAny(tok, "+/=") {
		t.Fatalf("a token is 32 random bytes in base64url (43 chars): %q", tok)
	}
	_ = r.st.Close() // flush the WAL
	for _, f := range []string{r.path, r.path + "-wal", r.path + "-shm"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(b), tok) {
			t.Fatalf("%s holds the clear token", filepath.Base(f))
		}
	}
	st, err := store.Open(r.path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if _, ok, _ := st.TokenByDigest(ctx, digestOf(tok)); !ok {
		t.Fatal("the sha256 digest finds the token")
	}
	rows, _ := st.AuditEntries(ctx, 0, 0)
	for _, e := range rows {
		if strings.Contains(e.Detail+e.Target+e.Actor, tok) {
			t.Fatal("an audit row holds the token")
		}
	}
}

func userCert(t *testing.T, r *rig, id string) (*x509.Certificate, *pki.CA) {
	t.Helper()
	ca, err := pki.New(context.Background(), pki.Options{Store: r.st, DataDir: r.dir, Now: func() time.Time { return r.now }})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ca.IssueUser(context.Background(), id, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	blk, _ := pem.Decode(b.CertPEM)
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert, ca
}

func TestIdentityPeerCertificates(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	cert, _ := userCert(t, r, "dave")
	if _, err := r.svc.ActorFromPeer(ctx, cert); ReasonOf(err) != ReasonNoGrant {
		t.Fatalf("a user certificate with no grant has no role: %v", err)
	}
	r.grant("dave", Release, "p1")
	a, err := r.svc.ActorFromPeer(ctx, cert)
	if err != nil || a.ID() != "dave" || a.Kind() != KindUser || a.Role() != Release || AuthorizeAction(WithAuthor(ctx, "x"), a, ActRunApprove, "p1") != nil {
		t.Fatalf("a user certificate maps to its actor: %+v, %v", a, err)
	}
	if found, err := r.svc.Revoke(ctx, r.op(), "dave"); err != nil || !found {
		t.Fatal(err)
	}
	if _, err = r.svc.ActorFromPeer(ctx, cert); ReasonOf(err) != ReasonNoGrant {
		t.Fatalf("a revoked grant takes effect at once: %v", err)
	}
	for _, kind := range []string{"node", "coordinator"} {
		n := &x509.Certificate{SerialNumber: big.NewInt(7), NotBefore: r.now.Add(-time.Hour), NotAfter: r.now.Add(time.Hour), URIs: []*url.URL{{Scheme: "nself-ci", Opaque: kind + "/n1"}}}
		if _, err = r.svc.ActorFromPeer(ctx, n); ReasonOf(err) != ReasonNodeKind || err.(*Error).ErrCode() != "E687" {
			t.Fatalf("a %s certificate is rejected with E687: %v", kind, err)
		}
	}
	if _, err = r.svc.ActorFromPeer(ctx, nil); ReasonOf(err) != ReasonBadCert {
		t.Fatalf("no certificate: %v", err)
	}
	r.now = r.now.Add(3 * time.Hour)
	if _, err = r.svc.ActorFromPeer(ctx, cert); ReasonOf(err) != ReasonBadCert {
		t.Fatalf("an expired certificate: %v", err)
	}
}

func TestPeerCertificateDeniedSerial(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	cert, _ := userCert(t, r, "erin")
	r.grant("erin", Admin, AllProjects)
	if _, err := r.svc.ActorFromPeer(ctx, cert); err != nil {
		t.Fatal(err)
	}
	if _, err := r.st.PKIDeny(ctx, "admin", "erin", "test", hexSerial(cert)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.ActorFromPeer(ctx, cert); ReasonOf(err) != ReasonBadCert {
		t.Fatalf("a denied serial is refused: %v", err)
	}
}

func hexSerial(c *x509.Certificate) string { return hex.EncodeToString(c.SerialNumber.Bytes()) }

func TestMutationsWriteExactlyOneRow(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	steps := []struct {
		name string
		do   func() error
	}{
		{"roles.grant", func() error {
			return r.svc.Assign(ctx, r.op(), Request{Principal: "bob", Role: Member, Projects: []string{"p1"}})
		}},
		{"roles.revoke", func() error { _, err := r.svc.Revoke(ctx, r.op(), "bob"); return err }},
		{"token.issue", func() error {
			_, _, err := r.svc.IssueToken(ctx, r.op(), Request{Principal: "bob", Role: Member, Projects: []string{"p1"}})
			return err
		}},
	}
	for _, s := range steps {
		before := len(r.auditRows())
		if err := s.do(); err != nil {
			t.Fatal(s.name, err)
		}
		rows := r.auditRows()[before:]
		if len(rows) != 1 || rows[0].Action != s.name || rows[0].Head == nil {
			t.Fatalf("%s must write exactly one sealed row: %+v", s.name, rows)
		}
	}
	before := len(r.auditRows())
	if found, _ := r.svc.Revoke(ctx, r.op(), "nobody"); found || len(r.auditRows()) != before {
		t.Fatal("revoking nothing writes nothing")
	}
	member := r.tokenActor("m", Member, AllProjects)
	if err := r.svc.Assign(ctx, member, Request{Principal: "x", Role: Admin, Projects: []string{AllProjects}}); ReasonOf(err) != ReasonRole {
		t.Fatalf("only admin grants roles: %v", err)
	}
	for _, bad := range []Request{
		{Principal: "Bad Name", Role: Member, Projects: []string{"p"}},
		{Principal: "bob", Role: Role("owner"), Projects: []string{"p"}},
		{Principal: "bob", Role: Member},
		{Principal: "bob", Role: Member, Projects: []string{"a b"}},
	} {
		if r.svc.Assign(ctx, r.op(), bad) == nil {
			t.Errorf("request %+v must be refused", bad)
		}
	}
}

func TestNonSelfPrincipals(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	Use(r.svc)
	count := func() int {
		n, err := NonSelfPrincipals(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count() != 0 {
		t.Fatal("an empty store has none")
	}
	if err := r.svc.Assign(ctx, r.op(), Request{Principal: "alice", Role: Admin, Projects: []string{AllProjects}, Self: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.svc.IssueToken(ctx, r.op(), Request{Principal: "alice", Role: Member, Projects: []string{AllProjects}, Self: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.svc.IssueToken(ctx, r.op(), Request{Principal: "alice", Role: Admin, Projects: []string{AllProjects}, Self: true}); err != nil {
		t.Fatal(err)
	}
	if count() != 0 {
		t.Fatalf("--self member and admin tokens and an admin grant count 0, got %d", count())
	}
	if r.svc.Assign(ctx, r.op(), Request{Principal: "bob", Role: Member, Projects: []string{"p"}, Self: true}) == nil {
		t.Fatal("--self is only for the operator's own identity")
	}
	r.grant("bob", Admin, AllProjects)
	if count() != 1 {
		t.Fatalf("an admin grant for another identity counts 1, got %d", count())
	}
	if _, err := r.svc.Revoke(ctx, r.op(), "bob"); err != nil || count() != 0 {
		t.Fatalf("revoking it counts 0, got %d (%v)", count(), err)
	}
	_, tok, _ := r.svc.IssueToken(ctx, r.op(), Request{Principal: "bob", Role: Member, Projects: []string{"p"}})
	if count() != 1 {
		t.Fatalf("a member token for another identity counts 1, got %d", count())
	}
	if _, err := r.svc.RevokeToken(ctx, r.op(), tok.ID); err != nil || count() != 0 {
		t.Fatalf("revoking the token counts 0, got %d", count())
	}
	cert, _ := userCert(t, r, "frank")
	if count() != 1 {
		t.Fatalf("a live user certificate not marked self counts 1, got %d", count())
	}
	if err := r.st.CertSubjectPut(ctx, hexSerial(cert), "frank", store.SubjectSelf); err != nil || count() != 0 {
		t.Fatalf("a self certificate counts 0, got %d (%v)", count(), err)
	}
	Use(nil)
	if _, err := NonSelfPrincipals(ctx); err == nil {
		t.Fatal("no installed service is an error, not a silent zero")
	}
}

func TestAuthorizeRefreshesRevocation(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	// A token actor resolved earlier loses access the moment its token is revoked.
	tok, row, _ := r.svc.IssueToken(ctx, r.op(), Request{Principal: "tina", Role: Admin, Projects: []string{AllProjects}})
	a, err := r.svc.ActorFromToken(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if err = Authorize(ctx, a, Admin, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = r.svc.RevokeToken(ctx, r.op(), row.ID); err != nil {
		t.Fatal(err)
	}
	if ReasonOf(Authorize(ctx, a, Admin, "")) != ReasonTokenRevoked {
		t.Fatal("a revoked token must stop working on an actor resolved before the revocation")
	}
	// Expiry applies to a resolved actor too.
	tok2, _, _ := r.svc.IssueToken(ctx, r.op(), Request{Principal: "tina", Role: Admin, Projects: []string{AllProjects}, TTL: time.Hour})
	a2, _ := r.svc.ActorFromToken(ctx, tok2)
	r.now = r.now.Add(2 * time.Hour)
	if ReasonOf(Authorize(ctx, a2, Admin, "")) != ReasonTokenExpired {
		t.Fatal("an expired token must stop working on a resolved actor")
	}
	r.now = epoch
	// A user actor follows its grant: lowered role applies at once, a revoked grant ends access, a denied serial too.
	cert, _ := userCert(t, r, "uma")
	r.grant("uma", Admin, AllProjects)
	u, err := r.svc.ActorFromPeer(ctx, cert)
	if err != nil {
		t.Fatal(err)
	}
	r.grant("uma", Member, AllProjects)
	if ReasonOf(Authorize(ctx, u, Admin, "")) != ReasonRole {
		t.Fatal("a lowered role applies to a resolved actor")
	}
	if err = Authorize(ctx, u, Member, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = r.st.PKIDeny(ctx, "admin", "uma", "test", hexSerial(cert)); err != nil {
		t.Fatal(err)
	}
	if ReasonOf(Authorize(ctx, u, Member, "")) != ReasonBadCert {
		t.Fatal("a denied serial ends a resolved actor")
	}
	cert2, _ := userCert(t, r, "uri")
	r.grant("uri", Admin, AllProjects)
	u2, _ := r.svc.ActorFromPeer(ctx, cert2)
	if _, err = r.svc.Revoke(ctx, r.op(), "uri"); err != nil {
		t.Fatal(err)
	}
	if ReasonOf(Authorize(ctx, u2, Member, "")) != ReasonNoGrant {
		t.Fatal("a revoked grant ends a resolved actor")
	}
}

func TestAuthorizeRevokeEndsTokensOfThePrincipal(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.grant("bob", Admin, AllProjects)
	tok, _, _ := r.svc.IssueToken(ctx, r.op(), Request{Principal: "bob", Role: Admin, Projects: []string{AllProjects}})
	other, _, _ := r.svc.IssueToken(ctx, r.op(), Request{Principal: "carol", Role: Admin, Projects: []string{AllProjects}})
	before := len(r.auditRows())
	if found, err := r.svc.Revoke(ctx, r.op(), "bob"); err != nil || !found {
		t.Fatal(found, err)
	}
	if len(r.auditRows()) != before+1 {
		t.Fatal("revoking a principal is one audit row")
	}
	if _, err := r.svc.ActorFromToken(ctx, tok); ReasonOf(err) != ReasonTokenRevoked {
		t.Fatalf("the revoked principal's token is dead: %v", err)
	}
	if _, err := r.svc.ActorFromToken(ctx, other); err != nil {
		t.Fatalf("another principal's token is untouched: %v", err)
	}
	// A principal with only a token (no grant) is also found by revoke.
	if found, err := r.svc.Revoke(ctx, r.op(), "carol"); err != nil || !found {
		t.Fatalf("a token-only principal is revoked: %v %v", found, err)
	}
}

func TestAuthorizeReservesSelfAsAName(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	Use(r.svc)
	for _, req := range []Request{
		{Principal: "self", Role: Admin, Projects: []string{AllProjects}},
		{Principal: "self", Role: Admin, Projects: []string{AllProjects}, Self: true}, // Service.Self is alice, not "self"
	} {
		if ReasonOf(r.svc.Assign(ctx, r.op(), req)) != ReasonBadRequest {
			t.Errorf("%+v must be refused", req)
		}
		if _, _, err := r.svc.IssueToken(ctx, r.op(), req); ReasonOf(err) != ReasonBadRequest {
			t.Errorf("token %+v must be refused", req)
		}
	}
	if err := r.st.CertSubjectPut(ctx, "01", "self", "self-lookalike"); err == nil {
		t.Fatal("the store refuses a subject that is neither self nor the principal")
	}
	if err := r.st.GrantPut(ctx, store.RoleGrant{Principal: "self", Role: "admin", Subject: "self", GrantedBy: "x"}, store.AuditInput{Actor: "x", Action: "a", Target: "t"}); err != nil {
		t.Fatalf("subject self is how the operator's own grant is stored: %v", err)
	}
	if err := r.st.GrantPut(ctx, store.RoleGrant{Principal: "self", Role: "admin", Subject: "bob", GrantedBy: "x"}, store.AuditInput{Actor: "x", Action: "a", Target: "t"}); err == nil {
		t.Fatal("a subject other than the principal's own name is refused")
	}
}

func TestActionsNonAuthorIsNormalised(t *testing.T) {
	r := newRig(t)
	rel := r.tokenActor("bob", Release, AllProjects)
	for _, author := range []string{"bob", "BOB", "user:bob", "cli:bob", "operator:cli:bob", "token:tok-abc:bob", " bob "} {
		ctx := WithAuthor(context.Background(), author)
		if ReasonOf(AuthorizeAction(ctx, rel, ActRunApprove, "p1")) != ReasonSelfApproval {
			t.Errorf("author %q is bob: self approval must be refused", author)
		}
	}
	for _, author := range []string{"bobby", "alice", "token:tok-abc:alice", "xbob"} {
		if err := AuthorizeAction(WithAuthor(context.Background(), author), rel, ActRunApprove, "p1"); err != nil {
			t.Errorf("author %q is someone else: %v", author, err)
		}
	}
	// The local operator is the only person of a PERSONAL coordinator: no author is needed and its own runs pass.
	for _, ctx := range []context.Context{context.Background(), WithAuthor(context.Background(), "alice"), WithAuthor(context.Background(), "cli:alice")} {
		if err := AuthorizeAction(ctx, r.op(), ActRunApprove, "p1"); err != nil {
			t.Errorf("the operator may approve: %v", err)
		}
	}
}

func TestAuthorizeNeedsAnAuditLog(t *testing.T) {
	r := newRig(t)
	svc := New(r.st, nil)
	op := svc.Operator("cli:alice")
	if err := Authorize(context.Background(), op, Admin, ""); err == nil {
		t.Fatal("an audited decision with no audit log fails closed")
	}
	if err := Authorize(context.Background(), op, Member, ""); err != nil {
		t.Fatalf("an unaudited decision needs no log: %v", err)
	}
	if got := op.Projects(); len(got) != 1 {
		t.Fatal(got)
	}
}

func TestAuthorizeProjectIsQuotedInTheRow(t *testing.T) {
	r := newRig(t)
	if err := Authorize(context.Background(), r.op(), Admin, "p result=deny"); err != nil {
		t.Fatal(err)
	}
	rows := r.auditRows()
	if d := rows[len(rows)-1].Detail; !strings.Contains(d, `project="p result=deny"`) {
		t.Fatalf("the project is quoted so it cannot forge a field: %s", d)
	}
}

func TestAuthorizeMutationsRefreshTheActor(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	tok, row, _ := r.svc.IssueToken(ctx, r.op(), Request{Principal: "tina", Role: Admin, Projects: []string{AllProjects}})
	a, err := r.svc.ActorFromToken(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.svc.Assign(ctx, a, Request{Principal: "x1", Role: Member, Projects: []string{"p"}}); err != nil {
		t.Fatalf("an admin token may grant: %v", err)
	}
	if _, err = r.svc.RevokeToken(ctx, r.op(), row.ID); err != nil {
		t.Fatal(err)
	}
	if ReasonOf(r.svc.Assign(ctx, a, Request{Principal: "x2", Role: Member, Projects: []string{"p"}})) != ReasonTokenRevoked {
		t.Fatal("a revoked token must not administer through a resolved actor")
	}
	if _, _, err = r.svc.IssueToken(ctx, a, Request{Principal: "x2", Role: Member, Projects: []string{"p"}}); ReasonOf(err) != ReasonTokenRevoked {
		t.Fatal("nor issue tokens")
	}
	if _, err = r.svc.List(ctx, a); ReasonOf(err) != ReasonTokenRevoked {
		t.Fatal("nor list")
	}
	if _, err = r.svc.Tokens(ctx, Actor{}); ReasonOf(err) != ReasonNoActor {
		t.Fatal("the zero actor lists nothing")
	}
}

func TestAuthorizeEndsWithTheCertificate(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	cert, _ := userCert(t, r, "cora")
	r.grant("cora", Admin, AllProjects)
	a, err := r.svc.ActorFromPeer(ctx, cert)
	if err != nil {
		t.Fatal(err)
	}
	if err = Authorize(ctx, a, Admin, ""); err != nil {
		t.Fatal(err)
	}
	r.now = cert.NotAfter.Add(time.Second)
	if ReasonOf(Authorize(ctx, a, Member, "")) != ReasonBadCert {
		t.Fatal("a resolved user actor must not outlive its certificate")
	}
}
