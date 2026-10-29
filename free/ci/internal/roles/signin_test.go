package roles

// Purpose: tests of the audit rows of refused sign-ins and refused admin mutations (D-1276), and of the
// documented two-row pattern of a mutation that authorizes first.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
)

func auditEvent(a Actor, action, target string) audit.Event {
	return audit.Event{Actor: a.Label(), Action: action, Target: target}
}

func (r *rig) actions() []string {
	var out []string
	for _, e := range r.auditRows() {
		out = append(out, e.Action)
	}
	return out
}

func TestSignInFailuresWriteARateLimitedRow(t *testing.T) {
	r := newRig(t)
	ctx := WithPeer(context.Background(), "203.0.113.9")
	secret := "guess-0123456789"
	for i := 0; i < 50; i++ { // a token guessing run
		if _, err := r.svc.ActorFromToken(ctx, fmt.Sprintf("%s-%d", secret, i)); ReasonOf(err) != ReasonTokenUnknown {
			t.Fatalf("guess %d: %v", i, err)
		}
	}
	rows := r.auditRows()
	if len(rows) != 1 || rows[0].Action != "signin.fail" || rows[0].Actor != "token:unknown" || rows[0].Target != "token" {
		t.Fatalf("a guessing run leaves one row per window: %+v", rows)
	}
	if d := rows[0].Detail; !strings.Contains(d, "reason=token_unknown") || !strings.Contains(d, `peer="203.0.113.9"`) || strings.Contains(d, secret) {
		t.Fatalf("the row names the reason and peer, never the token: %q", d)
	}
	r.now = r.now.Add(signInWindow)
	if _, err := r.svc.ActorFromToken(ctx, "guess-again"); err == nil {
		t.Fatal("a guess was accepted")
	}
	rows = r.auditRows()
	if len(rows) != 2 || !strings.Contains(rows[1].Detail, "suppressed=49") {
		t.Fatalf("the next window's row counts the rows held back: %+v", rows)
	}
	if _, err := r.svc.ActorFromToken(WithPeer(context.Background(), "198.51.100.1"), "other-peer"); err == nil || len(r.auditRows()) != 3 {
		t.Fatalf("another peer has its own budget: %d rows", len(r.auditRows()))
	}
}

func TestSignInFailureBudgetBoundsManyPeers(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 5*signInBudget; i++ {
		ctx := WithPeer(context.Background(), fmt.Sprintf("10.0.%d.%d", i/200, i%200))
		_, _ = r.svc.ActorFromToken(ctx, "nope")
	}
	if n := len(r.auditRows()); n != signInBudget {
		t.Fatalf("%d rows, want the budget of %d", n, signInBudget)
	}
}

func TestSignInFailureRowsForKnownTokensAndCertificates(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	tok, issued, err := r.svc.IssueToken(ctx, r.op(), Request{Principal: "bob", Role: Member, Projects: []string{"p1"}})
	if err != nil {
		t.Fatal(err)
	}
	r.now = r.now.Add(DefaultTokenTTL + time.Hour)
	if _, err = r.svc.ActorFromToken(ctx, tok); ReasonOf(err) != ReasonTokenExpired {
		t.Fatal(err)
	}
	cert, _ := userCert(t, r, "dave")
	if _, err = r.svc.ActorFromPeer(ctx, cert); ReasonOf(err) != ReasonNoGrant {
		t.Fatal(err)
	}
	r.now = r.now.Add(signInWindow) // the certificate bucket is spent for this window
	if _, err = r.svc.ActorFromPeer(ctx, nil); ReasonOf(err) != ReasonBadCert {
		t.Fatal(err)
	}
	rows := r.auditRows()
	if len(rows) != 4 { // the token issue, then one row per failed kind
		t.Fatalf("rows: %v", r.actions())
	}
	want := []struct{ actor, target, reason string }{
		{"token:" + issued.ID + ":bob", "token", "token_expired"},
		{"user:dave", "certificate", "no_grant"},
		{"certificate:unknown", "certificate", "bad_certificate"},
	}
	for i, w := range want {
		e := rows[i+1]
		if e.Action != "signin.fail" || e.Actor != w.actor || e.Target != w.target || !strings.Contains(e.Detail, "reason="+w.reason) {
			t.Errorf("row %d: %+v, want %+v", i+1, e, w)
		}
	}
}

func TestSuccessfulSignInWritesNoRow(t *testing.T) {
	r := newRig(t)
	a := r.tokenActor("m", Member, "p1")
	before := len(r.auditRows())
	if _, err := r.svc.ActorFromToken(context.Background(), "x"); err == nil || a.ID() != "m" {
		t.Fatal("setup")
	}
	if len(r.auditRows()) != before+1 {
		t.Fatalf("only the failure writes a row: %v", r.actions())
	}
}

func TestRefusedAdminMutationsWriteARow(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	member := r.tokenActor("m", Member, AllProjects)
	before := len(r.auditRows())
	_ = r.svc.Grant(ctx, r.op(), Request{Principal: "Bad Name", Role: Member, Projects: []string{"p"}})
	_ = r.svc.Grant(ctx, member, Request{Principal: "x", Role: Admin, Projects: []string{AllProjects}})
	_, _, _ = r.svc.IssueToken(ctx, r.op(), Request{Principal: "bob", Role: Member, Projects: []string{"p"}, TTL: time.Second})
	_, _ = r.svc.Revoke(ctx, member, "bob")
	_, _ = r.svc.RevokeToken(ctx, member, "tok-1")
	rows := r.auditRows()[before:]
	want := []struct{ actor, action, reason string }{
		{r.op().Label(), "roles.grant.refused", "bad_request"},
		{member.Label(), "roles.grant.refused", "role"},
		{r.op().Label(), "token.issue.refused", "bad_request"},
		{member.Label(), "roles.revoke.refused", "role"},
		{member.Label(), "token.revoke.refused", "role"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows: %+v", rows)
	}
	for i, w := range want {
		if rows[i].Actor != w.actor || rows[i].Action != w.action || rows[i].Detail != "reason="+w.reason {
			t.Errorf("row %d: %+v, want %+v", i, rows[i], w)
		}
	}
	if rows[0].Target != `"Bad_Name"` {
		t.Errorf("the refused principal is quoted: %q", rows[0].Target)
	}
}

// A release-level remote mutation leaves the authorize row and its own row, in that order (the documented
// pattern): the decision is recorded whether or not the mutation follows.
func TestAuthorizeThenMutationLeavesTwoRows(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	a := r.tokenActor("rel", Release, "p1")
	before := len(r.auditRows())
	if err := Authorize(ctx, a, Release, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Log.Append(ctx, auditEvent(a, "run.submit", "p1")); err != nil {
		t.Fatal(err)
	}
	rows := r.auditRows()[before:]
	if len(rows) != 2 || rows[0].Action != "authorize" || rows[1].Action != "run.submit" || rows[0].Actor != rows[1].Actor {
		t.Fatalf("two rows, one actor: %+v", rows)
	}
}
