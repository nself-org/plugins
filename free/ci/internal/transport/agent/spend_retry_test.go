package agent

// Purpose: pin the handling of a failed TokenSpend after a result was committed (debt D-1377): the spend is
//          retried, a spend that keeps failing fails the Result frame so the agent resends it, and the resend
//          spends the token without committing the result a second time.
// Inputs:  the rig and a Bindings fake whose TokenSpend fails a set number of times.
// Constraints: no sleeps in the test; the code under test waits a few milliseconds between attempts.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// flakySpend fails TokenSpend while fails > 0.
type flakySpend struct {
	Bindings
	fails atomic.Int32
	calls atomic.Int32
}

func (f *flakySpend) TokenSpend(ctx context.Context, jti string) error {
	f.calls.Add(1)
	if f.fails.Add(-1) >= 0 {
		return errors.New("db locked")
	}
	f.fails.Store(0)
	return f.Bindings.TokenSpend(ctx, jti)
}

// startRun delivers a lease through d and waits until its token was minted.
func startRun(t *testing.T, r *rig, d *Driver, rs *runSink) (done chan error, res protocol.Result, jti string) {
	t.Helper()
	lease, att, ep := r.seedLease("1", "n-1")
	done = make(chan error, 1)
	go func() { done <- d.Run(context.Background(), nodeCap("n-1"), protoLease(lease, att, ep), rs) }()
	var l *protocol.Lease
	eventually(t, 3*time.Second, "queued", func() bool { l, _ = d.Next(context.Background(), "n-1"); return l != nil })
	cl, _ := r.tokens.Verify(l.Token)
	if err := d.Gate.Check(context.Background(), call(lease, l.Token, ep)); err != nil { // the agent's first call binds the token
		t.Fatal(err)
	}
	res = protocol.Result{LeaseID: lease, Epoch: ep, Evidence: []byte(`{}`), Signature: protocol.Signature{KeyID: "k", Value: "v"}}
	return done, res, cl.JTI
}

func TestResultRetriesATransientSpendFailure(t *testing.T) {
	r := newRig(t)
	fs := &flakySpend{Bindings: r.st}
	fs.fails.Store(2)
	d := &Driver{Gate: &Gate{Tokens: r.tokens, Bindings: fs, Leases: r.st}, Minter: r.tokens, Leases: r.st, Poll: 10 * time.Millisecond}
	rs := &runSink{eng: r.eng}
	done, res, jti := startRun(t, r, d, rs)
	if err := d.Sink().Result(context.Background(), res); err != nil {
		t.Fatalf("Result = %v, want nil after the spend succeeded on a retry", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if b, _, _ := r.st.TokenCheck(context.Background(), jti); !b.Spent {
		t.Fatal("the token was not spent in the store")
	}
}

func TestResultFailsWhenTheSpendKeepsFailingAndTheResendSpendsOnly(t *testing.T) {
	r := newRig(t)
	fs := &flakySpend{Bindings: r.st}
	fs.fails.Store(1000)
	d := &Driver{Gate: &Gate{Tokens: r.tokens, Bindings: fs, Leases: r.st}, Minter: r.tokens, Leases: r.st, Poll: 10 * time.Millisecond}
	rs := &runSink{eng: r.eng}
	done, res, jti := startRun(t, r, d, rs)
	if err := d.Sink().Result(context.Background(), res); err == nil {
		t.Fatal("Result returned nil although the token could not be spent")
	}
	if rs.nResults() != 1 {
		t.Fatalf("%d results reached the run", rs.nResults())
	}
	select {
	case err := <-done:
		t.Fatalf("Run returned (%v) before the result was acknowledged", err)
	default:
	}
	fs.fails.Store(0) // the store recovers; the agent resends
	if err := d.Sink().Result(context.Background(), res); err != nil {
		t.Fatalf("resend = %v", err)
	}
	if rs.nResults() != 1 {
		t.Fatalf("the resend committed the result again: %d results reached the run", rs.nResults())
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if b, _, _ := r.st.TokenCheck(context.Background(), jti); !b.Spent {
		t.Fatal("the token was not spent in the store after the resend")
	}
}

func TestGateSpendFailureLeavesTheTokenSpendableAgain(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	fs := &flakySpend{Bindings: r.st}
	g := &Gate{Tokens: r.tokens, Bindings: fs, Leases: r.st}
	ctx := context.Background()
	if err := g.Check(ctx, call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	fs.fails.Store(1)
	if err := g.Spend(ctx, "jti-1"); err == nil {
		t.Fatal("Spend hid the store failure")
	}
	// The resend of the result must still reach the sink: a failed spend is not a spent token.
	if err := g.Check(ctx, call(lease, tok, ep)); err != nil {
		t.Fatalf("Check after a failed Spend = %v, want admitted", err)
	}
	if err := g.Spend(ctx, "jti-1"); err != nil {
		t.Fatal(err)
	}
	wantDenial(t, g.Check(ctx, call(lease, tok, ep)), ReasonReplayed)
}
