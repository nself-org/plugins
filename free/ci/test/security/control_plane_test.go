//go:build security

package security

// Purpose: authorized control-plane operations (TM-24, TM-25, TM-34): a member cannot change policy,
// trust, runners, secrets, roles, tokens or spend; a release role approves a run but never its own
// and never changes policy; a revoked token ends access at the next decision; an unknown action is
// refused; every decision, allow or deny, leaves a hash-chained audit row without the token, and an
// edited row is named by the verifier.
// Inputs:  a real store, role service and audit log over a temporary database.
// Outputs: pass or fail.
// Constraints: the actions and roles are the production catalogue; tokens are generated per test.

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type controlPlane struct {
	t    *testing.T
	st   *store.Store
	svc  *roles.Service
	log  *audit.Log
	path string
}

func newControlPlane(t *testing.T) *controlPlane {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	st := openStoreAt(t, path)
	log := &audit.Log{St: st}
	svc := roles.New(st, log)
	svc.Self = "alice"
	return &controlPlane{t: t, st: st, svc: svc, log: log, path: path}
}

func (c *controlPlane) actor(role roles.Role, principal string, projects ...string) roles.Actor {
	c.t.Helper()
	tok, _, err := c.svc.IssueToken(context.Background(), c.svc.Operator("cli:alice"), roles.Request{Principal: principal, Role: role, Projects: projects, TTL: time.Hour})
	if err != nil {
		c.t.Fatal(err)
	}
	a, err := c.svc.ActorFromToken(context.Background(), tok)
	if err != nil {
		c.t.Fatal(err)
	}
	return a
}

// TestMemberCannotRunControlPlaneActions: every privileged action in the catalogue is refused to a
// member, with the role reason, and an unknown action is refused to everyone.
func TestMemberCannotRunControlPlaneActions(t *testing.T) {
	c := newControlPlane(t)
	ctx := context.Background()
	member := c.actor(roles.Member, "mallory", "acme/app")
	privileged := []string{roles.ActPolicyTeamSet, roles.ActRunnerRegister, roles.ActRunnerRevoke, roles.ActNodeTrustSet, roles.ActProviderAuthorize,
		roles.ActPolicySet, roles.ActPrivacyZoneSet, roles.ActSecretAuthorize, roles.ActRunApprove, roles.ActSpendApprove, roles.ActReleaseStart,
		roles.ActAuditView, roles.ActRolesManage, roles.ActTokenManage}
	for _, act := range privileged {
		err := roles.AuthorizeAction(roles.WithAuthor(ctx, "someone-else"), member, act, "acme/app")
		if got := roles.ReasonOf(err); got != roles.ReasonRole {
			t.Errorf("member doing %s: reason %q (%v), want %q", act, got, err, roles.ReasonRole)
		}
	}
	// A member sees the logs of their own project and not another's.
	if err := roles.AuthorizeAction(ctx, member, roles.ActLogsView, "acme/app"); err != nil {
		t.Errorf("control: a member was refused the logs of their project: %v", err)
	}
	if got := roles.ReasonOf(roles.AuthorizeAction(ctx, member, roles.ActLogsView, "acme/other")); got != roles.ReasonProject {
		t.Errorf("a member read another project's logs: reason %q", got)
	}
	admin := c.actor(roles.Admin, "root", "*")
	if got := roles.ReasonOf(roles.AuthorizeAction(ctx, admin, "policy.delete_everything", "acme/app")); got != roles.ReasonUnknownAct {
		t.Errorf("an unknown action was not refused as such: %q", got)
	}
}

// TestRunApprovalNeedsAnotherPerson (TM-34): a release role approves a run it did not author, never
// its own, and never changes policy.
func TestRunApprovalNeedsAnotherPerson(t *testing.T) {
	c := newControlPlane(t)
	ctx := context.Background()
	rel := c.actor(roles.Release, "bob", "acme/app")
	if err := roles.AuthorizeAction(roles.WithAuthor(ctx, "mallory"), rel, roles.ActRunApprove, "acme/app"); err != nil {
		t.Fatalf("control: a release role was refused the approval of another person's run: %v", err)
	}
	for _, author := range []string{"bob", "user:bob", "token:tok-1:bob", "BOB"} {
		if got := roles.ReasonOf(roles.AuthorizeAction(roles.WithAuthor(ctx, author), rel, roles.ActRunApprove, "acme/app")); got != roles.ReasonSelfApproval {
			t.Errorf("author %q approving their own run: reason %q, want %q", author, got, roles.ReasonSelfApproval)
		}
	}
	if got := roles.ReasonOf(roles.AuthorizeAction(ctx, rel, roles.ActRunApprove, "acme/app")); got != roles.ReasonNoAuthor {
		t.Errorf("approval with an unknown author: reason %q", got)
	}
	if got := roles.ReasonOf(roles.AuthorizeAction(ctx, rel, roles.ActPolicySet, "acme/app")); got != roles.ReasonRole {
		t.Errorf("a release role changed policy: reason %q", got)
	}
	if got := roles.ReasonOf(roles.AuthorizeAction(roles.WithAuthor(ctx, "mallory"), rel, roles.ActRunApprove, "acme/other")); got != roles.ReasonProject {
		t.Errorf("a release role scoped to acme/app approved a run of acme/other: reason %q", got)
	}
}

// TestRevokedTokenEndsAccess: a token revoked after the actor was resolved is refused at the next
// decision, not at the next login.
func TestRevokedTokenEndsAccess(t *testing.T) {
	c := newControlPlane(t)
	ctx := context.Background()
	tok, tk, err := c.svc.IssueToken(ctx, c.svc.Operator("cli:alice"), roles.Request{Principal: "carol", Role: roles.Admin, Projects: []string{"acme/app"}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.svc.ActorFromToken(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if err = roles.AuthorizeAction(ctx, a, roles.ActPolicySet, "acme/app"); err != nil {
		t.Fatalf("control: the admin token was refused before the revoke: %v", err)
	}
	if _, err = c.svc.RevokeToken(ctx, c.svc.Operator("cli:alice"), tk.ID); err != nil {
		t.Fatal(err)
	}
	if got := roles.ReasonOf(roles.AuthorizeAction(ctx, a, roles.ActPolicySet, "acme/app")); got != roles.ReasonTokenRevoked {
		t.Fatalf("a revoked token still authorized policy.set: reason %q", got)
	}
	if _, err = c.svc.ActorFromToken(ctx, tok); err == nil {
		t.Fatal("a revoked token resolved to an actor")
	}
}

// TestDecisionsAreAuditedAndTamperEvident (TM-25): allow and deny both leave a row, no row holds the
// token, and an edited row is named by the verifier.
func TestDecisionsAreAuditedAndTamperEvident(t *testing.T) {
	c := newControlPlane(t)
	ctx := context.Background()
	tok, _, err := c.svc.IssueToken(ctx, c.svc.Operator("cli:alice"), roles.Request{Principal: "dave", Role: roles.Member, Projects: []string{"acme/app"}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	member, err := c.svc.ActorFromToken(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	_ = roles.AuthorizeAction(ctx, member, roles.ActPolicySet, "acme/app")                                   // deny
	_ = roles.AuthorizeAction(ctx, c.actor(roles.Admin, "erin", "acme/app"), roles.ActPolicySet, "acme/app") // allow
	if _, err = c.log.Seal(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := c.st.AuditEntries(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var allow, deny int
	for _, r := range rows {
		if r.Action != "authorize" {
			continue
		}
		switch {
		case strings.Contains(r.Detail, "result=allow"):
			allow++
		case strings.Contains(r.Detail, "result=deny") && strings.Contains(r.Detail, "reason=role"):
			deny++
		}
		if strings.Contains(r.Actor+r.Target+r.Detail, tok) {
			t.Fatalf("audit row %d holds the token", r.Seq)
		}
	}
	if allow < 1 || deny < 1 {
		t.Fatalf("audit rows: %d allow, %d deny; both decisions must be recorded (%d rows)", allow, deny, len(rows))
	}
	if rep, err := c.log.Verify(ctx, 0); err != nil || !rep.OK() {
		t.Fatalf("control: the untouched log did not verify: %+v %v", rep, err)
	}
	db, err := sql.Open("sqlite", c.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE audit SET detail='result=allow forged' WHERE seq=?", rows[len(rows)-1].Seq); err != nil {
		t.Fatal(err)
	}
	if rep, err := c.log.Verify(ctx, 0); err != nil || rep.OK() || rep.BadSeq != rows[len(rows)-1].Seq {
		t.Fatalf("an edited audit row was not named: %+v %v", rep, err)
	}
}
