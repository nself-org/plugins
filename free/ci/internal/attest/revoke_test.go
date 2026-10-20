package attest_test

// Purpose: tests of Revoke (P7-TRUST-20, Epic D13, HOST review R4): the node's keys are revoked first, the
// re-evaluation set comes back, each promoted release gets one quarantine audit event and the posted
// statuses are re-posted with reason node.revoked.
// Inputs:  a fake RevokeLedger, an audit recorder and a fake reporter.
// Outputs: pass or fail; no I/O.
// Constraints: the ledger step runs first and its failure stops everything.

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

type fakeRevokeLedger struct {
	out    attest.Affected
	err    error
	node   string
	reason string
}

func (f *fakeRevokeLedger) RevokeNode(_ context.Context, node, reason string) (attest.Affected, error) {
	f.node, f.reason = node, reason
	return f.out, f.err
}

type fakeReposter struct {
	keys          []attest.StatusKey
	state, reason string
	err           error
	calls         int
}

func (f *fakeReposter) Repost(_ context.Context, keys []attest.StatusKey, state, reason string) error {
	f.calls++
	f.keys, f.state, f.reason = keys, state, reason
	return f.err
}

type auditRow struct{ actor, action, target, detail string }

func fixtureAffected() attest.Affected {
	return attest.Affected{
		Keys:     []string{"ci-node-0123456789abcdef"},
		Gates:    []attest.GateRef{{RepoID: "repo-1", Revision: goodRevision, Job: "build"}},
		Statuses: []attest.StatusKey{{Revision: goodRevision, Context: "nself-ci@acme/widget"}},
		Releases: []attest.ReleaseRef{{RunID: "run-1", Project: "widget", Revision: goodRevision, ManifestSHA256: "ab"}, {RunID: "run-2", Project: "widget", Revision: goodRevision, ManifestSHA256: "cd"}},
	}
}

func TestRevokeReturnsSetAuditsReleasesAndReposts(t *testing.T) {
	led := &fakeRevokeLedger{out: fixtureAffected()}
	rep := &fakeReposter{}
	var rows []auditRow
	r := attest.Revoker{Ledger: led, Reposter: rep, Audit: func(_ context.Context, actor, action, target, detail string) error {
		rows = append(rows, auditRow{actor, action, target, detail})
		return nil
	}}
	got, err := r.Revoke(context.Background(), "node-1", "cli:alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gates) == 0 || len(got.Statuses) == 0 || len(got.Releases) == 0 {
		t.Fatalf("the re-evaluation set is empty: %+v", got)
	}
	if led.node != "node-1" || led.reason != "node.revoked" {
		t.Fatalf("revoked %q for %q", led.node, led.reason)
	}
	quarantine := 0
	for _, a := range rows {
		if a.actor != "cli:alice" {
			t.Errorf("audit actor %q", a.actor)
		}
		if a.action == "release.quarantine" {
			quarantine++
		}
	}
	if quarantine != len(got.Releases) || len(rows) != quarantine+1 {
		t.Fatalf("%d quarantine events for %d releases, %d rows", quarantine, len(got.Releases), len(rows))
	}
	if rep.calls != 1 || !reflect.DeepEqual(rep.keys, got.Statuses) || rep.state != "error" || rep.reason != "node.revoked" {
		t.Fatalf("repost %+v", rep)
	}
}

func TestRevokeFailureOfTheLedgerStopsEverything(t *testing.T) {
	led := &fakeRevokeLedger{err: errors.New("disk full")}
	rep := &fakeReposter{}
	audited := 0
	r := attest.Revoker{Ledger: led, Reposter: rep, Audit: func(context.Context, string, string, string, string) error { audited++; return nil }}
	if _, err := r.Revoke(context.Background(), "node-1", "a"); err == nil {
		t.Fatal("a failed revocation reported success")
	}
	if rep.calls != 0 || audited != 0 {
		t.Fatalf("repost %d, audit %d after a failed revocation", rep.calls, audited)
	}
}

func TestRevokeKeepsGoingAfterAuditOrRepostFailures(t *testing.T) {
	led := &fakeRevokeLedger{out: fixtureAffected()}
	rep := &fakeReposter{err: errors.New("github down")}
	audited := 0
	r := attest.Revoker{Ledger: led, Reposter: rep, Audit: func(context.Context, string, string, string, string) error {
		audited++
		return errors.New("audit down")
	}}
	got, err := r.Revoke(context.Background(), "node-1", "a")
	if err == nil || len(got.Releases) != 2 {
		t.Fatalf("errors must be returned with the set: %v %+v", err, got)
	}
	if audited != 3 || rep.calls != 1 {
		t.Fatalf("a failing audit write stopped the rest: audit %d repost %d", audited, rep.calls)
	}
}

func TestRevokeNodeWithoutKeysStillMarksTheNode(t *testing.T) {
	led := &fakeRevokeLedger{}
	rep := &fakeReposter{}
	got, err := attest.Revoker{Ledger: led, Reposter: rep}.Revoke(context.Background(), "node-1", "a")
	if err != nil || len(got.Gates)+len(got.Statuses)+len(got.Releases) != 0 || rep.calls != 0 || led.node != "node-1" {
		t.Fatalf("%v %+v %+v", err, got, rep)
	}
	if _, err := (attest.Revoker{}).Revoke(context.Background(), "node-1", "a"); err == nil {
		t.Fatal("revoked with no ledger")
	}
}
