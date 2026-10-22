package main

// Purpose: TestEnrollNoOfferNoSocket and TestEnrollBurstGate (P7-NODE-20): the burst enrollment listener of
//          `ci serve` binds nothing until an intent-bound offer is live and closes within 1 s of its end;
//          it is closed unless the OPERATOR-scope policy enables an ephemeral burst provider, and a value
//          from the revision under test never enables it.
// Inputs:  a real SQLite store, pki.CA and pairing.Service under t.TempDir(); an in-process `serve`
//          (serveV15) on loopback ports taken from the OS, never :3866.
// Outputs: pass or fail.
// Constraints: no real secrets (synthetic values only); every goroutine the test starts is awaited.

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type enrollApprover struct{}

func (enrollApprover) Kind() string { return "policy:test" }
func (enrollApprover) Approve(context.Context, pairing.ApprovalRequest) (pairing.Decision, error) {
	return pairing.Decision{Approve: true, Reason: "ok", Trust: pairing.DefaultTrust()}, nil
}

type enrollRig struct {
	t    *testing.T
	home string
	st   *store.Store
	svc  *pairing.Service
}

// newEnrollRig opens the CI home store, registers a pairing service as the process default and points the
// enroll gate at the store. burst lists the providers written as enabled at operator (user) scope.
func newEnrollRig(t *testing.T, burst ...string) *enrollRig {
	t.Helper()
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ca, err := pki.New(context.Background(), pki.Options{Store: st, DataDir: home})
	if err != nil {
		t.Fatal(err)
	}
	svc := pairing.New(pairing.Config{CA: ca, Store: st, Registry: registry.New(st), CoordinatorID: "local", AgentURL: "https://agent.test:3865",
		Actor: "cli:test", Poll: 20 * time.Millisecond})
	pairing.Use(svc)
	oldStore, oldLogf := enrollPolicyStore, enrollLogf
	useEnrollStore(st)
	enrollLogf = func(string, ...any) {}
	t.Cleanup(func() { pairing.Use(nil); enrollPolicyStore, enrollLogf = oldStore, oldLogf })
	r := &enrollRig{t: t, home: home, st: st, svc: svc}
	for _, id := range burst {
		r.setBurst(id, true)
	}
	return r
}

func (r *enrollRig) setBurst(id string, enabled bool) {
	r.t.Helper()
	val, _ := json.Marshal(map[string]bool{"enabled": enabled})
	err := r.st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject,
		Key: schedpolicy.KeyBurstProviders[:len(schedpolicy.KeyBurstProviders)-1] + id, Value: val, Actor: "test"}, nil)
	if err != nil {
		r.t.Fatal(err)
	}
}

func (r *enrollRig) offer(intent string) pairing.OfferResult {
	r.t.Helper()
	res, err := r.svc.Offer(context.Background(), pairing.OfferOptions{Approver: enrollApprover{}, Intent: intent, Label: "ephemeral"})
	if err != nil {
		r.t.Fatal(err)
	}
	return res
}

func enrollFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(cleanup func() error) { _ = cleanup() }(ln.Close)
	return ln.Addr().String()
}

func enrollOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func enrollWait(t *testing.T, within time.Duration, what string, cond func() bool) time.Duration {
	t.Helper()
	start := time.Now()
	for time.Since(start) <= within {
		if cond() {
			return time.Since(start)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("not within %s: %s", within, what)
	return 0
}

// enrollNever asserts cond stays false for d.
func enrollNever(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if cond() {
			t.Fatalf("happened: %s", what)
		}
	}
}

func TestEnrollNoOfferNoSocket(t *testing.T) {
	t.Run("ci serve with burst enabled binds nothing until an offer, and an offer's end closes it within 1 s", func(t *testing.T) {
		t.Setenv("NSELF_V15", "1")
		dir := serveFixture(t, quietHost())
		r := newEnrollRig(t, "fakecloud")
		enrollAddr := enrollFreeAddr(t)
		gated := make(chan bool, 1)
		realGate := enrollGate
		enrollGate = func(ctx context.Context) (burstGate, error) {
			g, err := realGate(ctx)
			gated <- g.Enabled
			return g, err
		}
		t.Cleanup(func() { enrollGate = realGate })

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		exit := make(chan int, 1)
		hook := func(c *coordinator.Config) {
			c.Listen = func(network, _ string) (net.Listener, error) { return net.Listen(network, "127.0.0.1:0") }
		}
		go func() {
			exit <- serveV15(ctx, []string{"--insecure", "--workdir", filepath.Join(dir, "wd"), "--data-dir", r.home, "--enroll-addr", enrollAddr}, hook)
		}()
		select {
		case enabled := <-gated:
			if !enabled {
				t.Fatal("the burst gate is closed although a provider is enabled at operator scope")
			}
		case code := <-exit:
			t.Fatalf("serve exited %d before the enroll listener started", code)
		case <-time.After(10 * time.Second):
			t.Fatal("the enroll listener never started")
		}
		enrollNever(t, 400*time.Millisecond, "a socket on the enroll address with no live offer", func() bool { return enrollOpen(enrollAddr) })

		// the serve process registered its own pairing service (serveWire): offers go through the default one
		off, err := pairing.Offer(context.Background(), pairing.OfferOptions{Approver: enrollApprover{}, Intent: "burst-1", Label: "ephemeral"})
		if err != nil {
			t.Fatal(err)
		}
		enrollWait(t, time.Second, "the offer opens the enroll listener", func() bool { return enrollOpen(enrollAddr) })
		if err := pairing.Withdraw(context.Background(), off.Code); err != nil {
			t.Fatal(err)
		}
		if took := enrollWait(t, time.Second, "the offer's end closes the listener", func() bool { return !enrollOpen(enrollAddr) }); took > time.Second {
			t.Fatalf("closing took %v", took)
		}
		cancel()
		select {
		case code := <-exit:
			if code != 0 {
				t.Fatalf("serve exit = %d", code)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("serve did not stop")
		}
		if enrollOpen(enrollAddr) {
			t.Fatal("the enroll socket outlived serve")
		}
	})

	t.Run("an offer without an intent never opens the listener", func(t *testing.T) {
		r := newEnrollRig(t, "fakecloud")
		addr := enrollFreeAddr(t)
		stop := mustStartEnroll(t, addr)
		if _, err := r.svc.Offer(context.Background(), pairing.OfferOptions{Approver: enrollApprover{}}); err != nil {
			t.Fatal(err)
		}
		enrollNever(t, 400*time.Millisecond, "a socket for an offer with no intent", func() bool { return enrollOpen(addr) })
		stop()
	})
}

func mustStartEnroll(t *testing.T, addr string) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stopFn, err := startEnrollListener(ctx, addr, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	var stopped bool
	stop = func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		if err := stopFn(sctx); err != nil {
			t.Errorf("stop: %v", err)
		}
	}
	t.Cleanup(stop)
	return stop
}

func TestEnrollBurstGate(t *testing.T) {
	t.Run("no ephemeral provider enabled at operator scope: not bound even with an offer", func(t *testing.T) {
		r := newEnrollRig(t)
		addr := enrollFreeAddr(t)
		stop := mustStartEnroll(t, addr)
		r.offer("burst-1")
		enrollNever(t, 500*time.Millisecond, "a socket with the burst gate closed", func() bool { return enrollOpen(addr) })
		stop()
	})

	t.Run("a provider written as enabled:false keeps the gate closed", func(t *testing.T) {
		r := newEnrollRig(t)
		r.setBurst("fakecloud", false)
		addr := enrollFreeAddr(t)
		mustStartEnroll(t, addr)
		r.offer("burst-1")
		enrollNever(t, 400*time.Millisecond, "a socket with the only provider disabled", func() bool { return enrollOpen(addr) })
	})

	t.Run("a revision-scoped .nself/ci.yaml value enabling burst has no effect", func(t *testing.T) {
		r := newEnrollRig(t)
		work := t.TempDir()
		if err := os.MkdirAll(filepath.Join(work, ".nself"), 0o755); err != nil {
			t.Fatal(err)
		}
		yaml := "version: 1\npolicy:\n  burst:\n    providers:\n      fakecloud:\n        enabled: true\n"
		if err := os.WriteFile(filepath.Join(work, ".nself", "ci.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Chdir(work) // the revision under test is the working directory of the process
		g, err := readBurstGate(context.Background())
		if err != nil || g.Enabled {
			t.Fatalf("gate = %+v, %v; a revision value opened it", g, err)
		}
		addr := enrollFreeAddr(t)
		mustStartEnroll(t, addr)
		r.offer("burst-1")
		enrollNever(t, 400*time.Millisecond, "a socket opened by a revision-scoped value", func() bool { return enrollOpen(addr) })

		// the policy layer itself refuses the same value from the revision scope
		res := policy.Resolver{Store: r.st, Run: noGit}
		got, err := res.Resolve(context.Background(), policy.Repo{Root: work, Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: work})
		if err == nil {
			if sp, serr := schedpolicy.Spend(&got); serr != nil || sp.Burst.Enabled["fakecloud"] {
				t.Fatalf("the resolver let a revision value enable burst: %+v, %v", sp.Burst.Enabled, serr)
			}
		}
	})

	t.Run("an operator-scope provider opens the gate", func(t *testing.T) {
		r := newEnrollRig(t, "fakecloud")
		g, err := readBurstGate(context.Background())
		if err != nil || !g.Enabled {
			t.Fatalf("gate = %+v, %v", g, err)
		}
		addr := enrollFreeAddr(t)
		mustStartEnroll(t, addr)
		r.offer("burst-1")
		enrollWait(t, time.Second, "an offer opens the listener", func() bool { return enrollOpen(addr) })
	})

	t.Run("an unreadable policy fails closed without failing serve", func(t *testing.T) {
		r := newEnrollRig(t, "fakecloud")
		enrollPolicyStore = func() (policy.Reader, func(), error) { return nil, nil, os.ErrPermission }
		addr := enrollFreeAddr(t)
		mustStartEnroll(t, addr)
		r.offer("burst-1")
		enrollNever(t, 400*time.Millisecond, "a socket while the policy is unreadable", func() bool { return enrollOpen(addr) })
	})

	t.Run("the listener is registered with the documented flag and default", func(t *testing.T) {
		fs, _, _ := newServeV15Flags()
		f := fs.Lookup("enroll-addr")
		if f == nil || f.DefValue != ":3866" {
			t.Fatalf("--enroll-addr = %+v", f)
		}
		if fs.Lookup("drain-deadline") == nil {
			t.Fatal("--drain-deadline is not registered")
		}
	})
}
