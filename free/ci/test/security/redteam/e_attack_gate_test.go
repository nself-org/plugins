//go:build security

package redteam

// Purpose: red team group E, job-token store and gate (TM-21 stolen job token, TM-13 stale writer). The
// attacker replays a spent token after the in-memory caches were flushed, after a coordinator restart, after
// the pruner ran, across a certificate renewal (reconcile) and in a race between sessions.
// Inputs:  the real store.node_token table, the real Gate and token keys (rig in e_attack_rig_test.go).
// Outputs: pass = the replay was refused (control HELD); fail = the token worked again (BROKEN).
// Constraints: loopback not needed; t.TempDir SQLite only; deterministic; at most a few thousand tiny writes.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

func eCall(lease string, epoch uint64, node, serial, session, tok string, reconcile bool) agentapi.Call {
	return agentapi.Call{Node: node, Session: session, Serial: serial, LeaseID: lease, Epoch: epoch, Type: "log", Token: tok, Reconcile: reconcile}
}

// TestE21SpentTokenSurvivesCacheFlushAndRestart (TM-21): after the result was accepted the token is spent in
// the store; flooding the in-memory spent set past its bound (4096) and building a fresh Gate (a restart) must
// not make it usable again, from the same or another certificate, reconcile or not.
func TestE21SpentTokenSurvivesCacheFlushAndRestart(t *testing.T) {
	r := newERig(t, true)
	ctx := context.Background()
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := r.gate
	if err := g.Check(ctx, eCall(lease, ep, "n-1", "aa01", "s1", tok, false)); err != nil {
		t.Fatalf("control: %v", err)
	}
	if err := g.Spend(ctx, "jti-1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4200; i++ { // flush the bounded in-memory spent set
		if err := g.Spend(ctx, fmt.Sprintf("flood-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	fresh := &agent.Gate{Tokens: r.tokens, Bindings: r.st, Leases: r.st}
	for name, gate := range map[string]*agent.Gate{"same gate, flushed": g, "restarted gate": fresh} {
		for _, c := range []agentapi.Call{
			eCall(lease, ep, "n-1", "aa01", "s1", tok, false),
			eCall(lease, ep, "n-1", "aa01", "s2", tok, true),
			eCall(lease, ep, "n-1", "bb02", "s3", tok, true),
			eCall(lease, ep, "n-1", "bb02", "s3", tok, false),
		} {
			if err := gate.Check(ctx, c); err == nil {
				t.Errorf("%s: spent token admitted (serial %s session %s reconcile %v)", name, c.Serial, c.Session, c.Reconcile)
			}
		}
	}
}

// TestE21PrunerDoesNotReviveSpentToken (TM-21): the pruner deletes rows one hour after exp using the time the
// caller passes. A coordinator clock that stepped forward (NTP, VM resume) and back must not delete the row of
// a still-valid spent token, because the next bind would then create a fresh unspent row and the replay works.
func TestE21PrunerDoesNotReviveSpentToken(t *testing.T) {
	if !openFinding(t, "E01") {
		return
	}
	r := newERig(t, true)
	ctx := context.Background()
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	if err := r.gate.Check(ctx, eCall(lease, ep, "n-1", "aa01", "s1", tok, false)); err != nil {
		t.Fatalf("control: %v", err)
	}
	if err := r.gate.Spend(ctx, "jti-1"); err != nil {
		t.Fatal(err)
	}
	if n, err := r.gate.Prune(ctx, time.Now().Add(2*time.Hour)); err != nil || n != 1 {
		t.Fatalf("prune with a clock two hours ahead removed %d rows (%v), want the one row (precondition of the attack)", n, err)
	}
	fresh := &agent.Gate{Tokens: r.tokens, Bindings: r.st, Leases: r.st} // the coordinator restarted
	if err := fresh.Check(ctx, eCall(lease, ep, "n-1", "cc03", "s9", tok, false)); err == nil {
		t.Error("a spent, still-valid token was admitted again after the pruner ran with a skewed clock")
	}
}

// TestE21RebindIsCompareAndSwap (TM-21): rebind from the wrong (serial, session), after spend, with empty
// targets and for an unknown jti moves nothing; spend of a never-bound jti leaves it unknown; many sessions
// racing to reconcile leave exactly one session able to send plain frames.
func TestE21RebindIsCompareAndSwap(t *testing.T) {
	r := newERig(t, true)
	ctx := context.Background()
	b := store.TokenBinding{JTI: "j", Serial: "aa", Session: "s1", NodeID: "n-1", Exp: time.Now().Add(time.Minute).Unix()}
	if _, created, err := r.st.TokenBind(ctx, b); err != nil || !created {
		t.Fatalf("bind: %v %v", created, err)
	}
	for name, a := range map[string][4]string{
		"wrong serial":  {"zz", "s1", "bb", "s2"},
		"wrong session": {"aa", "zz", "bb", "s2"},
		"unknown jti":   {"aa", "s1", "bb", "s2"},
	} {
		jti := "j"
		if name == "unknown jti" {
			jti = "nope"
		}
		if moved, err := r.st.TokenRebind(ctx, jti, a[0], a[1], a[2], a[3]); moved || err != nil {
			t.Errorf("%s: moved=%v err=%v", name, moved, err)
		}
	}
	if _, err := r.st.TokenRebind(ctx, "j", "aa", "s1", "", ""); err == nil {
		t.Error("a rebind to an empty serial and session was accepted")
	}
	if _, created, err := r.st.TokenBind(ctx, store.TokenBinding{JTI: "k", Serial: "aa", Session: "s1", NodeID: "n-1", Exp: time.Now().Add(time.Minute).Unix()}); err != nil || !created {
		t.Fatalf("bind k: %v %v", created, err)
	}
	if err := r.st.TokenSpend(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if moved, err := r.st.TokenRebind(ctx, "k", "aa", "s1", "bb", "s2"); moved || err != nil {
		t.Errorf("a spent token was rebound: moved=%v err=%v", moved, err)
	}
	if got, _, _ := r.st.TokenCheck(ctx, "k"); got.Serial != "aa" || got.Session != "s1" || !got.Spent {
		t.Errorf("the spent row changed: %+v", got)
	}
	// a plain frame from the right session but another certificate is refused as a wrong certificate, not as a session mix-up
	lease, att, ep := r.seedLease("9", "n-1")
	tok := r.mint("jti-9", "n-1", att, ep)
	if err := r.gate.Check(ctx, eCall(lease, ep, "n-1", "aa09", "s1", tok, false)); err != nil {
		t.Fatalf("control: %v", err)
	}
	if err := r.gate.Check(ctx, eCall(lease, ep, "n-1", "bb09", "s1", tok, false)); err == nil || !strings.Contains(err.Error(), "another certificate") {
		t.Errorf("a new certificate on a plain frame was not refused as a wrong certificate: %v", err)
	}
	if err := r.st.TokenSpend(ctx, "ghost"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := r.st.TokenCheck(ctx, "ghost"); found {
		t.Error("spending an unbound jti created a row")
	}
	if _, created, _ := r.st.TokenBind(ctx, b); created {
		t.Error("a second bind of the same jti wrote a row")
	}
	for _, bad := range []store.TokenBinding{{Serial: "a", Session: "s", NodeID: "n"}, {JTI: "x", Session: "s", NodeID: "n"}, {JTI: "x", Serial: "a", NodeID: "n"}, {JTI: "x", Serial: "a", Session: "s"}} {
		if _, created, err := r.st.TokenBind(ctx, bad); created || err == nil {
			t.Errorf("incomplete binding %+v was stored", bad)
		}
	}
}

// TestE21ConcurrentReconcileLeavesOneHolder (TM-21, run with -race): 12 sessions of one node reconcile the same
// token at once. Afterwards a plain frame must be admitted for the winning session only; a losing session must
// not keep a cached "proved" binding.
func TestE21ConcurrentReconcileLeavesOneHolder(t *testing.T) {
	r := newERig(t, true)
	ctx := context.Background()
	lease, att, ep := r.seedLease("1", "n-1")
	tok := r.mint("jti-1", "n-1", att, ep)
	g := r.gate
	if err := g.Check(ctx, eCall(lease, ep, "n-1", "aa01", "s0", tok, false)); err != nil {
		t.Fatalf("control: %v", err)
	}
	const n = 12
	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = g.Check(ctx, eCall(lease, ep, "n-1", "aa01", fmt.Sprintf("s%d", i), tok, true))
		}(i)
	}
	wg.Wait()
	admitted := 0
	for i := 0; i <= n; i++ {
		if g.Check(ctx, eCall(lease, ep, "n-1", "aa01", fmt.Sprintf("s%d", i), tok, false)) == nil {
			admitted++
		}
	}
	if admitted != 1 {
		t.Errorf("%d sessions of one token pass the gate after the reconcile race, want exactly 1", admitted)
	}
}
