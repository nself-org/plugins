package lease

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/quick"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func fixture(t *testing.T, count int, release bool) (*Engine, *[]time.Time) {
	t.Helper()
	now := []time.Time{time.Unix(100, 0)}
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: func() time.Time { return now[0] }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	e := New(s, nil, Config{Clock: func() time.Time { return now[0] }})
	ctx := context.Background()
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("%d", i)
		p := store.PipelineRow{ID: "p" + id, Project: "p", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
		if err := s.CreatePipeline(ctx, p); err != nil {
			t.Fatal(err)
		}
		kind := "test"
		if release && i == 0 {
			kind = "release"
		}
		if err := s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: p.ID, Name: id, Kind: kind, Idempotent: !release, InfraMax: 1, InputDigest: "i"}); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateAttempt(ctx, store.AttemptRow{ID: "a" + id, JobID: "j" + id, N: 1}, ""); err != nil {
			t.Fatal(err)
		}
		d := store.Demand{AttemptID: "a" + id, LeaseID: "l" + id, RunnerID: "r" + id, CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: e.Config.TTL}
		if _, err := e.Grant(ctx, "host", d, 1); err != nil {
			t.Fatal(err)
		}
		if err := s.Transition(ctx, "a"+id, "leased", "running", store.TransitionDetail{Epoch: 1}); err != nil {
			t.Fatal(err)
		}
	}
	return e, &now
}

func TestProperty(t *testing.T) {
	seed := int64(20261009)
	checks, err := strconv.Atoi(flag.Lookup("quickchecks").Value.String())
	if err != nil || checks < 10000 {
		checks = 10000
	}
	// Each sample is an event sequence: grant, heartbeat, silence, cancel, result,
	// restart, restore and wake. The pure epoch law is checked for every step.
	check := func(events []byte) bool {
		previous := int64(0)
		incarnation := int64(1)
		live, cancelled, deterministic, audited := false, false, false, false
		for _, event := range events {
			switch event % 8 {
			case 0: // grant
				if !live && !cancelled && !deterministic {
					next := store.LeaseEpoch(incarnation, previous)
					if next <= previous {
						return false
					}
					previous, live = next, true
				}
			case 1: // heartbeat
				if live && previous == 0 {
					return false
				}
			case 2: // silence
				live = false
			case 3: // cancel
				cancelled, live = true, false
			case 4: // result
				if live {
					deterministic, live = true, false
				}
			case 5, 6: // restart, restore
				incarnation++
			case 7: // wake
				if live && cancelled {
					return false
				}
			}
			if deterministic && live || !audited && cancelled && live {
				return false
			}
		}
		return true
	}
	if err := quick.Check(check, &quick.Config{MaxCount: checks, Rand: rand.New(rand.NewSource(seed))}); err != nil {
		t.Fatalf("seed=%d checks=%d: %v", seed, checks, err)
	}

	t.Run("stale epoch and cancel race", func(t *testing.T) {
		e, _ := fixture(t, 1, false)
		ctx := context.Background()
		if err := e.Result(ctx, "l0", 1); !code(err, "E662") {
			t.Fatalf("stale result: %v", err)
		}
		if e.StaleCount() != 1 {
			t.Fatalf("stale rejection not counted: %d", e.StaleCount())
		}
		if err := e.Store.RequestCancel(ctx, "a0", "operator", 2); err != nil {
			t.Fatal(err)
		}
		if err := e.Result(ctx, "l0", 1<<32|1); !code(err, "E605") {
			t.Fatalf("cancel lost CAS: %v", err)
		}
		second, _ := fixture(t, 1, false)
		if err := second.Result(ctx, "l0", 1<<32|1); err != nil {
			t.Fatal(err)
		}
		if err := second.Store.RequestCancel(ctx, "a0", "operator", 2); !code(err, "E605") {
			t.Fatalf("result won CAS: %v", err)
		}
	})
	t.Run("short outage renews twenty", func(t *testing.T) {
		e, now := fixture(t, 20, false)
		ctx := context.Background()
		(*now)[0] = (*now)[0].Add(20 * time.Second)
		if n, err := e.Start(ctx, "new", 2); err != nil || n != 20 {
			t.Fatalf("grace %d %v", n, err)
		}
		for i := 0; i < 20; i++ {
			if err := e.Reconnect(ctx, fmt.Sprintf("l%d", i), 1<<32|1, "running"); err != nil {
				t.Fatal(err)
			}
		}
		if n, err := e.Sweep(ctx); err != nil || n != 0 {
			t.Fatalf("short outage requeued %d %v", n, err)
		}
	})
	t.Run("long outage self fences once", func(t *testing.T) {
		e, now := fixture(t, 20, false)
		ctx := context.Background()
		(*now)[0] = (*now)[0].Add(300 * time.Second)
		if _, err := e.Start(ctx, "new", 2); err != nil {
			t.Fatal(err)
		}
		if n, err := e.Sweep(ctx); err != nil || n != 0 {
			t.Fatalf("coordinator expired own downtime %d %v", n, err)
		}
		for i := 0; i < 20; i++ {
			id := fmt.Sprintf("%d", i)
			if err := e.Reconnect(ctx, "l"+id, 1<<32|1, "self_fenced"); err != nil {
				t.Fatal(err)
			}
			if err := e.Reconnect(ctx, "l"+id, 1<<32|1, "self_fenced"); !code(err, "E662") {
				t.Fatalf("duplicate retry: %v", err)
			}
			attempts, err := e.Store.AttemptsByJob(ctx, "j"+id)
			if err != nil || len(attempts) != 2 || attempts[0].State != "retryable" || attempts[1].State != "queued" || attempts[1].N != 2 {
				t.Fatalf("retry %s: %+v %v", id, attempts, err)
			}
			runner, err := e.Store.LeasePreviousRunner(ctx, attempts[1].ID)
			if err != nil || runner != "r"+id {
				t.Fatalf("retry anti-affinity %s: %q %v", id, runner, err)
			}
		}
	})
	t.Run("release needs operator", func(t *testing.T) {
		e, _ := fixture(t, 1, true)
		if err := e.Reconnect(context.Background(), "l0", 1<<32|1, "self_fenced"); err != nil {
			t.Fatal(err)
		}
		a, err := e.Store.AttemptsByJob(context.Background(), "j0")
		if err != nil || len(a) != 1 || a[0].State != "needs-operator" {
			t.Fatalf("release retried: %+v %v", a, err)
		}
	})
	t.Run("breaker maintenance once", func(t *testing.T) {
		e, _ := fixture(t, 0, false)
		body, err := os.ReadFile("../model/testdata/capability/valid/laptop.valid.json")
		if err != nil {
			t.Fatal(err)
		}
		var capability model.Capability
		if err := json.Unmarshal(body, &capability); err != nil {
			t.Fatal(err)
		}
		e.Nodes = registry.New(e.Store)
		ctx := context.Background()
		node, err := e.Nodes.Register(ctx, capability, "operator")
		if err != nil {
			t.Fatal(err)
		}
		audits := 0
		e.Store.SetAuditSealer(func(_ []byte, row store.AuditRow) (store.AuditChain, error) {
			if row.Action == "node.state" {
				audits++
			}
			return store.AuditChain{}, nil
		})
		for i := 1; i <= 4; i++ {
			if i == 3 {
				if _, err := e.Breaker(ctx, node.Record.ID, false); err != nil {
					t.Fatal(err)
				}
			}
			count, err := e.Breaker(ctx, node.Record.ID, true)
			if err != nil || count != i {
				t.Fatalf("breaker %d: %d %v", i, count, err)
			}
		}
		node, err = e.Nodes.Get(ctx, node.Record.ID)
		if err != nil || node.Record.State != "maintenance" || audits != 1 {
			t.Fatalf("breaker state=%s audits=%d err=%v", node.Record.State, audits, err)
		}
	})
	t.Run("agent wake fence", func(t *testing.T) {
		wall := time.Unix(1, 0)
		mono := time.Duration(0)
		f := NewAgentFence(func() time.Time { return wall }, func() time.Duration { return mono }, 45*time.Second, 10*time.Second)
		kills := 0
		f.OnFence = func() { kills++ }
		wall = wall.Add(24 * time.Second)
		if f.Check() {
			t.Fatal("fenced before agent TTL")
		}
		wall = wall.Add(10 * time.Minute)
		mono += 24 * time.Second
		if !f.Check() {
			t.Fatal("wake did not self fence")
		}
		if !f.Check() || kills != 1 {
			t.Fatalf("self-fence callback count %d", kills)
		}
	})
	t.Run("hosted liveness expiry", func(t *testing.T) {
		e, now := fixture(t, 1, false)
		state := LiveRunning
		p := LeaseProfile{TTL: 45 * time.Second, Liveness: func(context.Context, string) (LiveState, error) { return state, nil }}
		if err := e.RegisterProfile("provider", p); err != nil {
			t.Fatal(err)
		}
		if err := e.RegisterProfile("provider", p); err == nil {
			t.Fatal("duplicate profile")
		}
		(*now)[0] = (*now)[0].Add(30 * time.Second)
		if _, err := e.PollHosted(context.Background(), "provider", "l0", "a0", 1<<32|1); err != nil {
			t.Fatal(err)
		}
		state = LiveVanished
		(*now)[0] = (*now)[0].Add(44 * time.Second)
		if _, err := e.PollHosted(context.Background(), "provider", "l0", "a0", 1<<32|1); err != nil {
			t.Fatal(err)
		}
		(*now)[0] = (*now)[0].Add(44 * time.Second)
		if n, err := e.Sweep(context.Background()); err != nil || n != 0 {
			t.Fatalf("early hosted expiry %d %v", n, err)
		}
		(*now)[0] = (*now)[0].Add(time.Second)
		if n, err := e.Sweep(context.Background()); err != nil || n != 1 {
			t.Fatalf("hosted expiry %d %v", n, err)
		}
	})
}

func code(err error, want string) bool {
	var e *store.Error
	return errors.As(err, &e) && e.Code == want
}
