package store

// Purpose: tests of the role rows: one live grant per principal, token digests, subject counting.
// Inputs:  testStore. Outputs: pass or fail.
// Constraints: audit input is empty here (the roles package tests the rows); no clear token is stored.

import (
	"context"
	"testing"
	"time"
)

func TestGrantReplaceRevoke(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	put := func(role string) {
		t.Helper()
		in := AuditInput{Actor: "a", Action: "roles.grant", Target: "bob", Detail: role}
		if err := s.RoleAssignmentPut(ctx, RoleAssignment{Principal: "bob", Role: role, Projects: []string{"p"}, Subject: "bob", GrantedBy: "a"}, in); err != nil {
			t.Fatal(err)
		}
	}
	put("member")
	put("admin")
	gs, _ := s.RoleAssignmentList(ctx)
	if len(gs) != 1 || gs[0].Role != "admin" {
		t.Fatalf("a new grant replaces the live one: %+v", gs)
	}
	if err := s.RoleAssignmentPut(ctx, RoleAssignment{Principal: "x", Role: "owner", Subject: "x", GrantedBy: "a"}, AuditInput{Actor: "a", Action: "g", Target: "x"}); err == nil {
		t.Fatal("a fourth role is refused by the table")
	}
	if found, err := s.GrantRevoke(ctx, "bob", AuditInput{Actor: "a", Action: "roles.revoke", Target: "bob"}); err != nil || !found {
		t.Fatal(found, err)
	}
	if _, ok, _ := s.RoleAssignmentGet(ctx, "bob"); ok {
		t.Fatal("revoked grant is not live")
	}
	if auditCount(t, s) != 3 {
		t.Fatalf("grant, replace and revoke write one row each, got %d", auditCount(t, s))
	}
}

func TestTokenDigestUniqueAndNonSelf(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tok := Token{ID: "tok-1", Digest: "d1", Principal: "bob", Role: "member", Projects: []string{"p"}, Subject: "bob", IssuedBy: "a", ExpiresAt: time.Now().Add(time.Hour).UnixNano()}
	in := AuditInput{Actor: "a", Action: "token.issue", Target: "bob"}
	if err := s.TokenPut(ctx, tok, in); err != nil {
		t.Fatal(err)
	}
	dup := tok
	dup.ID = "tok-2"
	if err := s.TokenPut(ctx, dup, in); err == nil {
		t.Fatal("a digest is unique")
	}
	if n, err := s.NonSelfPrincipals(ctx); err != nil || n != 1 {
		t.Fatalf("one non-self token: %d %v", n, err)
	}
	self := Token{ID: "tok-3", Digest: "d3", Principal: "alice", Role: "admin", Projects: []string{"*"}, Subject: SubjectSelf, IssuedBy: "a", ExpiresAt: time.Now().Add(time.Hour).UnixNano()}
	if err := s.TokenPut(ctx, self, in); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.NonSelfPrincipals(ctx); n != 1 {
		t.Fatalf("a self token adds nothing: %d", n)
	}
	expired := Token{ID: "tok-4", Digest: "d4", Principal: "bob", Role: "member", Projects: []string{"p"}, Subject: "bob", IssuedBy: "a", ExpiresAt: time.Now().Add(-time.Hour).UnixNano()}
	if err := s.TokenPut(ctx, expired, in); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.NonSelfPrincipals(ctx); n != 1 {
		t.Fatalf("an expired token is not live: %d", n)
	}
}

func TestSubjectRules(t *testing.T) {
	for _, tc := range []struct {
		principal, subject string
		ok                 bool
	}{{"bob", "bob", true}, {"bob", SubjectSelf, true}, {"bob", "alice", false}, {SubjectSelf, SubjectSelf, true}, {SubjectSelf, "bob", false}, {"bob", "", false}} {
		if got := subjectOK(tc.principal, tc.subject); got != tc.ok {
			t.Errorf("subjectOK(%q,%q) = %v", tc.principal, tc.subject, got)
		}
	}
}

func TestGrantRevokeAlsoEndsTokens(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	exp := time.Now().Add(time.Hour).UnixNano()
	in := AuditInput{Actor: "a", Action: "x", Target: "bob"}
	_ = s.TokenPut(ctx, Token{ID: "t1", Digest: "d1", Principal: "bob", Role: "member", Projects: []string{"p"}, Subject: "bob", IssuedBy: "a", ExpiresAt: exp}, in)
	_ = s.TokenPut(ctx, Token{ID: "t2", Digest: "d2", Principal: "carol", Role: "member", Projects: []string{"p"}, Subject: "carol", IssuedBy: "a", ExpiresAt: exp}, in)
	if found, err := s.GrantRevoke(ctx, "bob", in); err != nil || !found {
		t.Fatalf("a token-only principal counts as found: %v %v", found, err)
	}
	if tk, _, _ := s.TokenByID(ctx, "t1"); tk.RevokedAt == 0 {
		t.Fatal("bob's token is revoked")
	}
	if tk, _, _ := s.TokenByID(ctx, "t2"); tk.RevokedAt != 0 {
		t.Fatal("carol's token is untouched")
	}
	if found, _ := s.GrantRevoke(ctx, "bob", in); found {
		t.Fatal("nothing left to revoke")
	}
}

func TestRevokeProceedsOnADamagedChain(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := s.AuditAppend(ctx, AuditInput{Actor: "a", Action: "x", Target: "t", Detail: "d", Seal: standIn}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.writer.Exec("UPDATE audit SET head=NULL WHERE seq=2"); err != nil {
		t.Fatal(err)
	}
	in := AuditInput{Actor: "a", Action: "roles.revoke", Target: "bob", Detail: "d", Seal: standIn}
	if err := s.RoleAssignmentPut(ctx, RoleAssignment{Principal: "bob", Role: "admin", Projects: []string{"*"}, Subject: "bob", GrantedBy: "a"}, in); err != nil {
		t.Fatalf("a grant still records its row beside a damaged chain: %v", err)
	}
	if found, err := s.GrantRevoke(ctx, "bob", in); err != nil || !found {
		t.Fatalf("a revocation must never be blocked by audit damage: %v %v", found, err)
	}
	if _, ok, _ := s.RoleAssignmentGet(ctx, "bob"); ok {
		t.Fatal("the grant is revoked")
	}
	if n, err := s.AuditSeal(ctx, standIn); !codeIs(err, "E609") || n != 0 {
		t.Fatalf("an explicit seal reports the damage as E609: %d %v", n, err)
	}
}
