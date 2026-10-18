package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func auditCount(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.readers.QueryRow("SELECT count(*) FROM audit").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestApplyWriteRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	set := PolicyWrite{Op: PolicySet, Scope: "project", Subject: "proj", Key: "trust.network", Value: []byte(`"none"`), Actor: "ali"}
	boom := errors.New("audit failed")
	if err := s.ApplyWrite(ctx, set, func(*Tx) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("callback error lost: %v", err)
	}
	if rows, err := s.PolicyRows(ctx, "project", "proj"); err != nil || len(rows) != 0 {
		t.Fatalf("the row survived a failed callback: %v %v", rows, err)
	}
	before := auditCount(t, s)
	if err := s.ApplyWrite(ctx, set, func(tx *Tx) error { return tx.AppendAudit("ali", "policy.set", "trust.network", "none") }); err != nil {
		t.Fatal(err)
	}
	rows, err := s.PolicyRows(ctx, "project", "proj")
	if err != nil || len(rows) != 1 || string(rows[0].Value) != `"none"` || rows[0].UpdatedBy != "ali" {
		t.Fatalf("rows %+v %v", rows, err)
	}
	if auditCount(t, s) != before+1 {
		t.Fatal("audit row not committed with the write")
	}
	// A bad audit row (empty actor) rolls the policy row back too.
	upd := set
	upd.Value = []byte(`"restricted"`)
	if err := s.ApplyWrite(ctx, upd, func(tx *Tx) error { return tx.AppendAudit("", "policy.set", "x", "") }); err == nil {
		t.Fatal("invalid audit accepted")
	}
	rows, _ = s.PolicyRows(ctx, "project", "proj")
	if len(rows) != 1 || string(rows[0].Value) != `"none"` {
		t.Fatalf("update survived a failed audit: %+v", rows)
	}
}

func TestPolicyWriteShapeAndPins(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for name, w := range map[string]PolicyWrite{
		"no actor":   {Op: PolicySet, Scope: "user", Subject: "u", Key: "k", Value: []byte(`1`)},
		"bad scope":  {Op: PolicySet, Scope: "pipeline", Subject: "u", Key: "k", Value: []byte(`1`), Actor: "a"},
		"bad json":   {Op: PolicySet, Scope: "user", Subject: "u", Key: "k", Value: []byte(`{`), Actor: "a"},
		"null value": {Op: PolicySet, Scope: "user", Subject: "u", Key: "k", Value: []byte(` null `), Actor: "a"},
		"huge value": {Op: PolicySet, Scope: "user", Subject: "u", Key: "k", Value: []byte(`"` + strings.Repeat("a", 70000) + `"`), Actor: "a"},
		"bad pin":    {Op: PolicyPinOp, Subject: "u", SHA: "main", Actor: "a"},
		"unknown op": {Op: "drop", Subject: "u", Actor: "a"},
	} {
		if err := s.ApplyWrite(ctx, w, nil); !codeIs(err, "E607") {
			t.Errorf("%s: %v", name, err)
		}
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	if err := s.ApplyWrite(ctx, PolicyWrite{Op: PolicyPinOp, Subject: "u", SHA: sha, Actor: "a"}, nil); err != nil {
		t.Fatal(err)
	}
	if p, ok, err := s.PolicyPinFor(ctx, "u"); err != nil || !ok || p.SHA != sha || p.PinnedBy != "a" {
		t.Fatalf("pin %+v %v %v", p, ok, err)
	}
	if err := s.ApplyWrite(ctx, PolicyWrite{Op: PolicyClear, Subject: "u", Actor: "a"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.PolicyPinFor(ctx, "u"); ok {
		t.Fatal("pin not cleared")
	}
	put := func(scope, subj, key string) {
		if err := s.ApplyWrite(ctx, PolicyWrite{Op: PolicySet, Scope: scope, Subject: subj, Key: key, Value: []byte(`1`), Actor: "a"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	put("user", "u", "b")
	put("user", "u", "a")
	put("team", "u", "z")
	rows, err := s.PolicyRows(ctx, "user", "u")
	if err != nil || len(rows) != 2 || rows[0].Key != "a" {
		t.Fatalf("scoped, ordered rows: %+v %v", rows, err)
	}
	if err := s.ApplyWrite(ctx, PolicyWrite{Op: PolicyUnset, Scope: "user", Subject: "u", Key: "a", Actor: "a"}, nil); err != nil {
		t.Fatal(err)
	}
	if rows, _ := s.PolicyRows(ctx, "user", "u"); len(rows) != 1 {
		t.Fatalf("unset: %+v", rows)
	}
}
