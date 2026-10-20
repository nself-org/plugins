//go:build scenarios

package scenarios

// Purpose: P7-SCHED-15 acceptance for P7-TRUST-26 on the spend switch. `nself ci policy set
// economics.paid_enabled true` is refused for a TEAM member (role member; E687) and leaves no policy row
// and no policy audit row; an admin's write lands with exactly one policy audit-chain row, which the
// chain verifies; on a PERSONAL coordinator the local operator (who holds admin) succeeds with one row.
// Inputs:  policy.Writer (the function behind cmd/policy.go's set) over a real store, roles actors.
// Outputs: assertions only.
// Constraints: the command itself is in package main (cmd/policy.go) and cannot be imported; these tests
//          call policy.Writer.Apply, which is the whole of the command's authorize-then-write path.
//          Other audit rows (the role decision's own row) are not policy rows and are counted apart.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// paidSet applies `policy set economics.paid_enabled true` (user scope) as actor.
func paidSet(t *testing.T, st *store.Store, svc *roles.Service, actor roles.Actor) error {
	t.Helper()
	w := &policy.Writer{Store: st, Log: svc.Log}
	_, err := w.Apply(context.Background(), actor, policy.Change{Op: policy.OpSet, Scope: "user", Key: schedpolicy.KeyPaidEnabled, Value: json.RawMessage(`true`)})
	return err
}

// policyAuditRows counts the policy.* rows of the audit chain and verifies the chain.
func policyAuditRows(t *testing.T, st *store.Store) int {
	t.Helper()
	entries, err := st.AuditEntries(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.Action == "policy.set" {
			n++
			if e.Target != schedpolicy.KeyPaidEnabled {
				t.Fatalf("policy audit row names %q", e.Target)
			}
		}
	}
	if res, err := (&audit.Log{St: st}).Verify(context.Background(), 0); err != nil || !res.OK() {
		t.Fatalf("audit chain: %+v %v", res, err)
	}
	return n
}

func storedPaidRows(t *testing.T, st *store.Store) int {
	t.Helper()
	rows, err := st.PolicyRows(context.Background(), "user", policy.LocalSubject)
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func TestPolicyPaidEnabledTeamMemberRefused(t *testing.T) {
	clk := newClock()
	st := econStore(t, clk)
	act := newEconActors(t, st, clk)
	err := paidSet(t, st, act.svc, act.member)
	if err == nil || roles.ReasonOf(err) != roles.ReasonRole {
		t.Fatalf("a TEAM member must be refused with E687 (role), got %v", err)
	}
	if n := storedPaidRows(t, st); n != 0 {
		t.Fatalf("a refused write left %d policy rows", n)
	}
	if n := policyAuditRows(t, st); n != 0 {
		t.Fatalf("a refused write left %d policy audit rows", n)
	}
}

func TestPolicyPaidEnabledAdminAudited(t *testing.T) {
	clk := newClock()
	st := econStore(t, clk)
	act := newEconActors(t, st, clk)
	before, _ := st.AuditEntries(context.Background(), 0, 0)
	if err := paidSet(t, st, act.svc, act.admin); err != nil {
		t.Fatal(err)
	}
	if n := storedPaidRows(t, st); n != 1 {
		t.Fatalf("policy rows %d, want 1", n)
	}
	if n := policyAuditRows(t, st); n != 1 {
		t.Fatalf("policy audit rows %d, want exactly 1", n)
	}
	// The chain grows by the role decision (authorize, written by roles.Authorize for every admin action)
	// and the one policy.set row; nothing else.
	after, _ := st.AuditEntries(context.Background(), 0, 0)
	var added []string
	for _, e := range after[len(before):] {
		added = append(added, e.Action)
		if e.Action == "policy.set" && e.Actor != act.admin.Label() {
			t.Fatalf("the audit row names actor %q, want %q", e.Actor, act.admin.Label())
		}
	}
	if len(added) != 2 || added[0] != "authorize" || added[1] != "policy.set" {
		t.Fatalf("audit rows added by the write: %v, want [authorize policy.set]", added)
	}
}

func TestPolicyPaidEnabledPersonalOperator(t *testing.T) {
	clk := newClock()
	st := econStore(t, clk)
	act := newEconActors(t, st, clk)
	if err := paidSet(t, st, act.svc, act.operator); err != nil {
		t.Fatal(err)
	}
	if n := storedPaidRows(t, st); n != 1 {
		t.Fatalf("policy rows %d, want 1", n)
	}
	if n := policyAuditRows(t, st); n != 1 {
		t.Fatalf("policy audit rows %d, want exactly 1", n)
	}
}
