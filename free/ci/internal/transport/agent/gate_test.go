package agent

// Purpose: tests of the job-token gate (R-DS-15): the table of refusals with one reason key each, first-use
//          binding, durability across a coordinator restart, reconnect rebinding and the spent token.
// Inputs:  the rig (real store, CA and token keys).
// Constraints: each refusal is E661 with its own reason; no token text reaches an error message.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func call(leaseID, token string, epoch uint64) agentapi.Call {
	return agentapi.Call{Node: "n-1", Session: "sess-1", Serial: "aa01", LeaseID: leaseID, Epoch: epoch, Type: "log", Token: token}
}

func wantDenial(t *testing.T, err error, reason string) {
	t.Helper()
	if got := DenialReason(err); got != reason {
		t.Fatalf("err = %v, want E661 %s", err, reason)
	}
	d := err.(*Denial)
	if !strings.HasPrefix(err.Error(), "E661 "+reason+":") || d.ErrCode() != "E661" || d.ErrReason() != reason || !d.Refusal() {
		t.Fatalf("denial %q: code %s reason %s refusal %v", err, d.ErrCode(), d.ErrReason(), d.Refusal())
	}
}

func TestGateAdmitsTheSameTokenOnLogHeartbeatAndResult(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := r.gate()
	for _, typ := range []string{"log", "heartbeat", "result", "log"} {
		c := call(lease, tok, ep)
		c.Type = typ
		if err := g.Check(context.Background(), c); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
	}
	b, found, _ := r.st.TokenCheck(context.Background(), "jti-1")
	if !found || b.Serial != "aa01" || b.Session != "sess-1" || b.NodeID != "n-1" {
		t.Fatalf("binding = %+v found=%v", b, found)
	}
}

func TestGateRefusalTable(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	_, att2, ep2 := r.seedLease("2", "n-1")
	g := r.gate()
	ctx := context.Background()
	good := r.mint("jti-1", "n-1", att, ep)
	if err := g.Check(ctx, call(lease, good, ep)); err != nil {
		t.Fatal(err)
	}
	tampered := tamper(good)
	for _, c := range []struct {
		name   string
		mod    func(*agentapi.Call)
		token  string
		reason string
	}{
		{"no token", nil, "", ReasonMissing},
		{"garbage", nil, "not-a-token", ReasonInvalid},
		{"bad signature", nil, tampered, ReasonInvalid},
		{"another node's call", func(c *agentapi.Call) { c.Node = "n-2" }, good, ReasonWrongRunner},
		{"another certificate", func(c *agentapi.Call) { c.Serial = "bb02" }, good, ReasonWrongCertificate},
		{"another session", func(c *agentapi.Call) { c.Session = "sess-2" }, good, ReasonWrongSession},
		{"token of another attempt", nil, r.mint("jti-2", "n-1", att2, ep2), ReasonWrongJob},
		{"token of another epoch", nil, r.mint("jti-3", "n-1", att, ep+1), ReasonWrongJob},
	} {
		cl := call(lease, c.token, ep)
		if c.mod != nil {
			c.mod(&cl)
		}
		err := g.Check(ctx, cl)
		if DenialReason(err) != c.reason {
			t.Errorf("%s: err = %v, want E661 %s", c.name, err, c.reason)
			continue
		}
		if c.token != "" && strings.Contains(err.Error(), c.token) {
			t.Errorf("%s: the message holds the token", c.name)
		}
	}
}

func TestGateExpiredToken(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	r.skew.Store(int64(time.Hour))
	wantDenial(t, r.gate().Check(context.Background(), call(lease, tok, ep)), ReasonExpired)
}

func TestGateTerminalAttempt(t *testing.T) {
	for _, state := range []string{"finalizing", "passed", "failed", "cancelled", "lost"} {
		r := newRig(t)
		lease, att, ep := r.seedLease("1", "n-1")
		tok := r.mint("jti-1", "n-1", att, ep)
		g := r.gate()
		if err := g.Check(context.Background(), call(lease, tok, ep)); err != nil {
			t.Fatal(err)
		}
		finish(t, r, att, ep, state)
		hb := call(lease, tok, ep)
		hb.Type = "heartbeat" // log frames trust the last lease read; a heartbeat reads it again
		wantDenial(t, g.Check(context.Background(), hb), ReasonTerminal)
		wantDenial(t, g.Check(context.Background(), call(lease, tok, ep)), ReasonTerminal) // and the log frames after it read it too
	}
}

// finish moves the running attempt to a terminal state.
func finish(t *testing.T, r *rig, attemptID string, epoch uint64, state string) {
	t.Helper()
	ctx := context.Background()
	steps := map[string][][2]string{
		"finalizing": {{"running", "finalizing"}},
		"cancelled":  {{"running", "cancelled"}},
		"lost":       {{"running", "lost"}},
		"passed":     {{"running", "finalizing"}, {"finalizing", "passed"}},
		"failed":     {{"running", "finalizing"}, {"finalizing", "failed"}},
	}
	for _, s := range steps[state] {
		_, aep, err := r.st.AttemptStateEpoch(ctx, attemptID)
		if err != nil {
			t.Fatal(err)
		}
		d := store.TransitionDetail{Epoch: aep}
		if err := r.st.Transition(ctx, attemptID, jobState(s[0]), jobState(s[1]), d); err != nil {
			t.Fatalf("%s -> %s: %v", s[0], s[1], err)
		}
	}
}

func TestGateUnknownLeaseIsTerminal(t *testing.T) {
	r := newRig(t)
	_, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	wantDenial(t, r.gate().Check(context.Background(), call("no-such-lease", tok, ep)), ReasonTerminal)
}

func TestGateBindingIsDurableAcrossARestart(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	if err := r.gate().Check(context.Background(), call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	r.reopen()
	g := r.gate() // a new gate: nothing cached
	other := call(lease, tok, ep)
	other.Session = "sess-after-restart"
	wantDenial(t, g.Check(context.Background(), other), ReasonWrongSession)
	other = call(lease, tok, ep)
	other.Serial = "cc03"
	wantDenial(t, g.Check(context.Background(), other), ReasonWrongCertificate)
	if err := g.Check(context.Background(), call(lease, tok, ep)); err != nil {
		t.Fatalf("the bound session is refused after a restart: %v", err)
	}
}

func TestGateReconcileMovesTheBindingToTheNewSession(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := r.gate()
	ctx := context.Background()
	if err := g.Check(ctx, call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	next := call(lease, tok, ep)
	next.Session, next.Reconcile, next.Type = "sess-2", true, "heartbeat"
	if err := g.Check(ctx, next); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	next.Reconcile, next.Type = false, "log"
	if err := g.Check(ctx, next); err != nil {
		t.Fatalf("log from the new session: %v", err)
	}
	wantDenial(t, g.Check(ctx, call(lease, tok, ep)), ReasonWrongSession) // the old session no longer holds the token
	// without the reconcile heartbeat another certificate is refused, even from the current session
	other := next
	other.Serial, other.Reconcile, other.Type = "dd04", false, "log"
	wantDenial(t, g.Check(ctx, other), ReasonWrongCertificate)
	// another node's certificate never gets here: the token names its runner
	thief := next
	thief.Node = "n-2"
	wantDenial(t, g.Check(ctx, thief), ReasonWrongRunner)
}

// A renewed certificate of the same node moves the binding with the reconcile heartbeat (7-day certificates); an
// older certificate that is still valid cannot take it back, and a certificate the CA did not issue to this node
// after the bound one never moves it (D-1379).
func TestGateSerialMovesOnlyToACertificateIssuedLater(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := r.gate()
	ctx := context.Background()
	older, newer, newest, foreign := r.issueCert("n-1"), r.issueCert("n-1"), r.issueCert("n-1"), r.issueCert("n-2")
	on := func(serial, session string, reconcile bool) agentapi.Call {
		c := call(lease, tok, ep)
		c.Serial, c.Session, c.Reconcile = serial, session, reconcile
		c.Type = "log"
		if reconcile {
			c.Type = "heartbeat"
		}
		return c
	}
	if err := g.Check(ctx, on(newer, "sess-1", false)); err != nil {
		t.Fatal(err)
	}
	// the older certificate, still valid, cannot move the binding to itself
	wantDenial(t, g.Check(ctx, on(older, "sess-2", true)), ReasonWrongCertificate)
	wantDenial(t, g.Check(ctx, on(older, "sess-1", false)), ReasonWrongCertificate)
	// a certificate of another node, or one the CA never recorded, cannot either
	wantDenial(t, g.Check(ctx, on(foreign, "sess-2", true)), ReasonWrongCertificate)
	wantDenial(t, g.Check(ctx, on("ff99", "sess-2", true)), ReasonWrongCertificate)
	if err := g.Check(ctx, on(newer, "sess-1", false)); err != nil {
		t.Fatalf("the bound certificate lost the binding to a refused move: %v", err)
	}
	// the newest certificate moves it with the reconcile heartbeat and then holds it
	if err := g.Check(ctx, on(newest, "sess-3", true)); err != nil {
		t.Fatalf("renewal reconcile: %v", err)
	}
	if err := g.Check(ctx, on(newest, "sess-3", false)); err != nil {
		t.Fatalf("log after the renewal: %v", err)
	}
	// after the move the older certificates are refused, with or without a reconcile heartbeat
	for _, serial := range []string{newer, older} {
		wantDenial(t, g.Check(ctx, on(serial, "sess-3", false)), ReasonWrongCertificate)
		wantDenial(t, g.Check(ctx, on(serial, "sess-4", true)), ReasonWrongCertificate)
	}
	b, _, _ := r.st.TokenCheck(ctx, "jti-1")
	if b.Serial != newest || b.Session != "sess-3" {
		t.Fatalf("binding = %+v, want %s on sess-3", b, newest)
	}
}

func TestGateWithoutACertViewNeverMovesToAnotherCertificate(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := &Gate{Tokens: r.tokens, Bindings: r.st, Leases: r.st}
	ctx := context.Background()
	if err := g.Check(ctx, call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	next := call(lease, tok, ep)
	next.Serial, next.Session, next.Reconcile, next.Type = r.issueCert("n-1"), "sess-2", true, "heartbeat"
	wantDenial(t, g.Check(ctx, next), ReasonWrongCertificate)
	next.Serial = "aa01" // the same certificate may still reconnect
	if err := g.Check(ctx, next); err != nil {
		t.Fatalf("reconnect on the bound certificate: %v", err)
	}
}

func TestGateReconcileRaceLeavesOneHolder(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	if err := r.gate().Check(context.Background(), call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for _, sess := range []string{"s-a", "s-b", "s-c", "s-d"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := call(lease, tok, ep)
			c.Session, c.Reconcile = sess, true
			if r.gate().Check(context.Background(), c) == nil {
				mu.Lock()
				won++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if won < 1 {
		t.Fatal("no session took the binding")
	}
	holders := 0 // whoever won last holds it alone: every other session is refused now
	for _, sess := range []string{"s-a", "s-b", "s-c", "s-d", "sess-1"} {
		c := call(lease, tok, ep)
		c.Session = sess
		if r.gate().Check(context.Background(), c) == nil {
			holders++
		}
	}
	if holders != 1 {
		t.Fatalf("%d sessions hold the token, want exactly 1", holders)
	}
}

func TestGateSpentTokenIsReplayed(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := r.gate()
	ctx := context.Background()
	if err := g.Check(ctx, call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	if err := g.Spend(ctx, "jti-1"); err != nil {
		t.Fatal(err)
	}
	wantDenial(t, g.Check(ctx, call(lease, tok, ep)), ReasonReplayed)
	wantDenial(t, r.gate().Check(ctx, call(lease, tok, ep)), ReasonReplayed) // and after a restart
}

func TestGateCachesProvedBindings(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	cb := &countBindings{Bindings: r.st}
	g := &Gate{Tokens: r.tokens, Bindings: cb, Leases: r.st}
	for i := 0; i < 20; i++ {
		if err := g.Check(context.Background(), call(lease, tok, ep)); err != nil {
			t.Fatal(err)
		}
	}
	if cb.binds != 1 {
		t.Fatalf("%d store writes for 20 calls of one session, want 1", cb.binds)
	}
}

type countBindings struct {
	Bindings
	binds int
}

func (c *countBindings) TokenBind(ctx context.Context, b store.TokenBinding) (store.TokenBinding, bool, error) {
	c.binds++
	return c.Bindings.TokenBind(ctx, b)
}

func TestGateStoreFailureIsNotARefusal(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := &Gate{Tokens: r.tokens, Bindings: r.st, Leases: r.st}
	_ = r.st.Close()
	err := g.Check(context.Background(), call(lease, tok, ep))
	if err == nil || DenialReason(err) != "" {
		t.Fatalf("a failing store must not look like a token refusal: %v", err)
	}
}

func jobState(s string) model.JobState { return model.JobState(s) }

// tamper changes the first character of the signature part, which always changes the signature.
func tamper(token string) string {
	i := strings.LastIndexByte(token, '.') + 1
	c := byte('A')
	if token[i] == 'A' {
		c = 'B'
	}
	return token[:i] + string(c) + token[i+1:]
}

type countVerifier struct {
	TokenVerifier
	n int
}

func (c *countVerifier) Verify(token string) (pki.TokenClaims, error) {
	c.n++
	return c.TokenVerifier.Verify(token)
}

type countLeases struct {
	LeaseView
	n int
}

func (c *countLeases) LeaseGet(ctx context.Context, id string) (store.RemoteLease, error) {
	c.n++
	return c.LeaseView.LeaseGet(ctx, id)
}

// D-1376: a log line must not cost an Ed25519 verify and a lease read each.
func TestGateVerifiesATokenOnceAndReadsTheLeaseOnStateFrames(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	cv, cl := &countVerifier{TokenVerifier: r.tokens}, &countLeases{LeaseView: r.st}
	g := &Gate{Tokens: cv, Bindings: r.st, Leases: cl}
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		typ := []string{"log", "artifact-ref", "cache-ref"}[i%3]
		c := call(lease, tok, ep)
		c.Type = typ
		if err := g.Check(ctx, c); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
	}
	if cv.n != 1 || cl.n != 1 {
		t.Fatalf("50 bulk frames: %d verifies and %d lease reads, want 1 and 1", cv.n, cl.n)
	}
	for _, typ := range []string{"heartbeat", "result"} {
		c := call(lease, tok, ep)
		c.Type = typ
		before := cl.n
		if err := g.Check(ctx, c); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		if cl.n != before+1 {
			t.Fatalf("%s frame did not read the lease", typ)
		}
	}
	if cv.n != 1 {
		t.Fatalf("%d verifies, want 1", cv.n)
	}
}

func TestGateCachedClaimsExpireWithTheToken(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := &Gate{Tokens: r.tokens, Bindings: r.st, Leases: r.st, Now: func() time.Time { return time.Now().Add(time.Duration(r.skew.Load())) }}
	ctx := context.Background()
	if err := g.Check(ctx, call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	r.skew.Store(int64(time.Hour))
	wantDenial(t, g.Check(ctx, call(lease, tok, ep)), ReasonExpired)
}

func TestGateCachedClaimsDoNotWidenWhatATokenMayDo(t *testing.T) {
	r := newRig(t)
	lease, att, ep := r.seedLease("1", "n-1")
	lease2, _, _ := r.seedLease("2", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := r.gate()
	ctx := context.Background()
	if err := g.Check(ctx, call(lease, tok, ep)); err != nil {
		t.Fatal(err)
	}
	wantDenial(t, g.Check(ctx, call(lease2, tok, ep)), ReasonWrongJob) // same token, another lease
	wantDenial(t, g.Check(ctx, call(lease, tok, ep+1)), ReasonWrongJob)
	n2 := call(lease, tok, ep)
	n2.Node = "n-2"
	wantDenial(t, g.Check(ctx, n2), ReasonWrongRunner)
	other := call(lease, tok, ep)
	other.Session = "sess-2"
	wantDenial(t, g.Check(ctx, other), ReasonWrongSession)
}
