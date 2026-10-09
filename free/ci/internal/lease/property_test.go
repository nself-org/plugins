package lease

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"math/rand"
	"path/filepath"
	"strconv"
	"testing"
	"testing/quick"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// runPropertySequences exercises durable leases under seeded event sequences.
func runPropertySequences(t *testing.T) {
	t.Helper()
	const seed int64 = 20261009
	checks, err := strconv.Atoi(flag.Lookup("quickchecks").Value.String())
	if err != nil || checks < 1 {
		checks = 100
	}
	now := time.Unix(100, 0)
	clock := func() time.Time { return now }
	s, err := store.Open(filepath.Join(t.TempDir(), "property.db"), store.Options{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	e := New(s, nil, Config{Clock: clock})
	ctx := context.Background()
	sequence, restores, releases := 0, 0, 0
	var exercised [8]int
	check := func(bits uint64) bool {
		sequence++
		id := fmt.Sprintf("seq-%d", sequence)
		p, j, a, l := "p-"+id, "j-"+id, "a-"+id, "l-"+id
		release := bits&1 == 0
		kind := "test"
		if release {
			kind = "release"
			releases++
		}
		fail := func(step string, err error) bool {
			t.Errorf("seed=%d sequence=%d step=%s: %v", seed, sequence, step, err)
			return false
		}
		if err := s.CreatePipeline(ctx, store.PipelineRow{ID: p, Project: "p", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
			return fail("pipeline", err)
		}
		if err := s.CreateJob(ctx, store.JobRow{ID: j, PipelineID: p, Name: id, Kind: kind, Idempotent: !release, InfraMax: 2, InputDigest: "i"}); err != nil {
			return fail("job", err)
		}
		if err := s.CreateAttempt(ctx, store.AttemptRow{ID: a, JobID: j, N: 1}, ""); err != nil {
			return fail("attempt", err)
		}
		d := store.Demand{AttemptID: a, LeaseID: l, RunnerID: "runner", CPU: 1, MemMB: 1, CapacityCPU: 2, CapacityMemMB: 2, TTL: e.Config.TTL}
		incarnation := int64(1)
		epoch, err := e.Grant(ctx, id, d, incarnation)
		if err != nil {
			return fail("grant", err)
		}
		if err := s.Transition(ctx, a, "leased", "running", store.TransitionDetail{Epoch: 1}); err != nil {
			return fail("running", err)
		}
		live, deterministicN := true, 0
		for step := 0; step < 12; step++ {
			event := byte(bits>>uint((step%8)*8)) % 8
			switch event {
			case 0: // Duplicate grant, or grant a store-authorized retry.
				if live {
					exercised[0]++
					if _, err := e.Grant(ctx, id, d, incarnation); !code(err, "E605") {
						return fail("double lease accepted", err)
					}
					break
				}
				attempts, err := s.AttemptsByJob(ctx, j)
				if err != nil {
					return fail("retry lookup", err)
				}
				if len(attempts) == 2 && attempts[1].State == "queued" {
					oldEpoch := epoch
					a, l = attempts[1].ID, "retry-"+id
					d.AttemptID, d.LeaseID = a, l
					incarnation++
					epoch, err = e.Grant(ctx, id, d, incarnation)
					if err != nil || epoch <= oldEpoch {
						return fail("retry epoch", err)
					}
					if err := s.Transition(ctx, a, "leased", "running", store.TransitionDetail{Epoch: 1}); err != nil {
						return fail("retry running", err)
					}
					live = true
				}
			case 1: // Heartbeat writes through the engine and store.
				if live && e.Renew(ctx, l, epoch, 0) != nil {
					return fail("heartbeat", fmt.Errorf("renew rejected"))
				}
				if live {
					exercised[1]++
				}
			case 2: // Silence crosses the durable expiry and sweeps.
				if live {
					exercised[2]++
					now = now.Add(46 * time.Second)
					if n, err := e.Sweep(ctx); err != nil || n < 1 {
						return fail("sweep", err)
					}
					live = false
				}
			case 3: // A cancel intent must beat a carrier result.
				if live {
					exercised[3]++
					_, stateEpoch, err := s.AttemptStateEpoch(ctx, a)
					if err != nil {
						return fail("cancel epoch", err)
					}
					if err := s.RequestCancel(ctx, a, "operator", stateEpoch); err != nil {
						return fail("cancel", err)
					}
					if err := e.Result(ctx, l, epoch); !code(err, "E605") {
						return fail("cancel/result CAS", err)
					}
					if err := s.Transition(ctx, a, "running", "cancelled", store.TransitionDetail{Epoch: stateEpoch}); err != nil {
						return fail("cancel transition", err)
					}
					live = false
				}
			case 4: // Deterministic result must never queue a retry.
				if live {
					exercised[4]++
					if err := e.Result(ctx, l, epoch); err != nil {
						return fail("result", err)
					}
					_, stateEpoch, err := s.AttemptStateEpoch(ctx, a)
					if err != nil {
						return fail("result epoch", err)
					}
					if err := s.Transition(ctx, a, "finalizing", "failed", store.TransitionDetail{Epoch: stateEpoch, FailureClass: "code"}); err != nil {
						return fail("code failure", err)
					}
					live = false
					deterministicN = 1
					if a != "a-"+id {
						deterministicN = 2
					}
				}
			case 5: // Restart uses real grace in the store.
				exercised[5]++
				incarnation++
				if _, err := e.Start(ctx, "restart-"+id, incarnation); err != nil {
					return fail("restart", err)
				}
			case 6: // Periodically restore a real SQLite snapshot, then start the engine.
				exercised[6]++
				incarnation++
				if sequence%1024 == 0 {
					var backup bytes.Buffer
					if err := s.Backup(ctx, &backup); err != nil {
						return fail("backup", err)
					}
					restored, err := s.Restore(ctx, bytes.NewReader(backup.Bytes()))
					if err != nil {
						return fail("restore", err)
					}
					s, e = restored, New(restored, nil, Config{Clock: clock})
					restores++
				}
				if _, err := e.Start(ctx, "restore-"+id, incarnation); err != nil {
					return fail("restore grace", err)
				}
			case 7: // Wake revalidates before the next action.
				exercised[7]++
				wall, mono := now, time.Duration(0)
				f := NewAgentFence(func() time.Time { return wall }, func() time.Duration { return mono }, 45*time.Second, 10*time.Second)
				wall = wall.Add(10 * time.Minute)
				if !f.Check() {
					return fail("wake fence", fmt.Errorf("not fenced"))
				}
				if live {
					if err := e.Reconnect(ctx, l, epoch, "self_fenced"); err != nil {
						return fail("self fence", err)
					}
					live = false
				}
			}
		}
		attempts, err := s.AttemptsByJob(ctx, j)
		if err != nil {
			return fail("assert attempts", err)
		}
		active := 0
		for _, attempt := range attempts {
			if attempt.State == "leased" || attempt.State == "running" {
				active++
			}
		}
		if active > 1 || deterministicN > 0 && len(attempts) > deterministicN {
			return fail("double lease or deterministic retry", fmt.Errorf("attempts=%+v", attempts))
		}
		if release && len(attempts) != 1 {
			return fail("release retried", fmt.Errorf("attempts=%+v", attempts))
		}
		if release && attempts[0].State == "needs-operator" {
			if err := s.CreateAttempt(ctx, store.AttemptRow{ID: "unaudited-" + id, JobID: j, N: 2}, ""); !code(err, "E609") {
				return fail("unaudited release retry", err)
			}
			if err := s.AppendAudit(ctx, "operator", "operator-retry", attempts[0].ID, "approved"); err != nil {
				return fail("operator audit", err)
			}
			if err := s.CreateAttempt(ctx, store.AttemptRow{ID: "audited-" + id, JobID: j, N: 2}, ""); err != nil {
				return fail("audited release retry", err)
			}
		}
		return true
	}
	if err := quick.Check(check, &quick.Config{MaxCount: checks, Rand: rand.New(rand.NewSource(seed))}); err != nil {
		t.Fatalf("seed=%d checks=%d: %v", seed, checks, err)
	}
	if sequence != checks || releases == 0 || checks >= 1024 && restores == 0 {
		t.Fatalf("vacuous property: sequences=%d restores=%d releases=%d", sequence, restores, releases)
	}
	for event, count := range exercised {
		if count == 0 {
			t.Fatalf("event %d never exercised real engine/store path", event)
		}
	}
	t.Logf("seed=%d sequences=%d SQLite restores=%d release sequences=%d", seed, sequence, restores, releases)
}
