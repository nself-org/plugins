package race

import (
	"encoding/json"
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func ptr[T any](v T) *T { return &v }

func runner(id, loc, class string, slots int) sched.Runner {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects, c.Lifecycle.Eligible = []string{"project"}, true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = ptr("linux"), ptr("amd64")
	c.Resources.CPU.Value, c.Resources.MemMB.Value = ptr(float64(8)), ptr(int64(8192))
	c.Identity.Labels = []string{"arch=amd64"}
	c.Location.Kind.Value = &loc
	return sched.Runner{ID: id, Location: loc, Class: class, Provider: "operator", Avail: sched.Avail{Slots: slots, CPU: 8 * int64(slots), MemMB: 8192 * int64(slots)}, Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner", "internal"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", PersonalOwnNode: loc == "local", NoLANRoute: true}}
}

func job() sched.Job {
	return sched.Job{Key: "job", ID: "job", Project: "project", Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true}
}

func world(rs ...sched.Runner) sched.World {
	return sched.World{Runners: rs, Policy: sched.Policy{Preset: "balanced", PaidEnabled: true}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
}

func TestRaceNeverPaidOrRelease(t *testing.T) {
	paid := runner("paid", "vps", "paid", 4)
	eph := runner("eph", "lan", "owned", 4)
	eph.Ephemeral = true
	untrusted := runner("untrusted", "lan", "owned", 4)
	untrusted.Decl.Accepts = []model.TrustClass{"untrusted"}
	hosted := runner("hosted", "hosted", "included", 4)
	w := world(runner("a", "lan", "owned", 1), runner("b", "local", "local", 1), paid, eph, untrusted)
	w.Hosted = []sched.Runner{hosted}
	c := Select(w, job(), 8, false)
	slices.Sort(c.Runners)
	if !slices.Equal(c.Runners, []string{"a", "b"}) {
		t.Fatalf("runners = %v excluded = %v", c.Runners, c.Excluded)
	}
	want := map[string]string{"paid": ReasonSpend, "eph": ReasonSpend, "hosted": ReasonSpend, "untrusted": ReasonTrust}
	for id, why := range want {
		if c.Excluded[id] != why {
			t.Fatalf("%s excluded for %q, want %q", id, c.Excluded[id], why)
		}
	}
	if len(w.Exclusions) != 0 || w.Runners[0].Avail.Slots != 1 {
		t.Fatal("Select mutated the caller's world")
	}
	j := job()
	if c := Select(w, j, 8, true); c.Raceable() || c.Reason != ReasonRelease {
		t.Fatalf("a release-class job was raced: %+v", c)
	}
	j.Pipeline.Release = true
	if c := Select(w, j, 8, false); c.Raceable() || c.Reason != ReasonRelease {
		t.Fatalf("a release pipeline was raced: %+v", c)
	}
	// one eligible runner left: no race
	if c := Select(world(runner("a", "lan", "owned", 3), paid), job(), 4, false); c.Raceable() || c.Reason != ReasonNoTarget {
		t.Fatalf("one runner raced against itself: %+v", c)
	}
}

func TestRaceWidthBounded(t *testing.T) {
	seed := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(seed))
	classes := []string{"owned", "local", "paid", "ephemeral", "included"}
	for round := 0; round < 200; round++ {
		var rs []sched.Runner
		for i := 0; i < 1+rng.Intn(6); i++ {
			r := runner(string(rune('a'+i)), "lan", classes[rng.Intn(len(classes))], rng.Intn(3))
			r.Ephemeral = rng.Intn(5) == 0
			if rng.Intn(5) == 0 {
				r.Decl.Accepts = []model.TrustClass{"untrusted"}
			}
			rs = append(rs, r)
		}
		w := world(rs...)
		width := 1 + rng.Intn(5)
		c := Select(w, job(), width, false)
		free := 0
		for _, r := range rs {
			if !Spend(w, r) && Trusted(w, job(), r) {
				free += r.Avail.Slots
			}
		}
		if len(c.Runners) > width || len(c.Runners) > free {
			t.Fatalf("seed %d round %d: %d siblings, width %d, free slots %d", seed, round, len(c.Runners), width, free)
		}
		seen := map[string]bool{}
		for _, id := range c.Runners {
			if seen[id] {
				t.Fatalf("seed %d round %d: two siblings on %s", seed, round, id)
			}
			seen[id] = true
			for _, r := range rs {
				if r.ID == id && (Spend(w, r) || r.Ephemeral || !Trusted(w, job(), r)) {
					t.Fatalf("seed %d round %d: sibling on %s (%s)", seed, round, id, r.Class)
				}
			}
		}
	}
}

func TestWidthKey(t *testing.T) {
	if Width(trust.Effective{}) != DefaultWidth {
		t.Fatal("unset width is not the default")
	}
	for raw, ok := range map[string]bool{"3": true, "1": true, "0": false, "17": false, `"2"`: false, "2 3": false} {
		_, err := decodeWidth(json.RawMessage(raw))
		if (err == nil) != ok {
			t.Fatalf("decode %s: %v", raw, err)
		}
	}
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
}
