package world

// Purpose: the world digest covers the policy values placement reads (P7-AUTO-05).

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func stepWorld(t *testing.T, sec int64) sched.World {
	t.Helper()
	dec := func(raw json.RawMessage) (any, error) {
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, err
		}
		return n.Int64()
	}
	reg, err := trust.NewRegistry(trust.KeySpec{Key: sched.KeyAutoStepSec, Owner: "sched", Kind: trust.TightenLower, Default: sched.DefaultAutoStepSec, Decode: dec})
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{sched.KeyAutoStepSec: json.RawMessage(strconv.FormatInt(sec, 10))}})
	if err != nil {
		t.Fatal(err)
	}
	return sched.World{Policy: sched.Policy{Preset: sched.AutoPreset, Trust: eff}, Runners: []sched.Runner{{ID: "vps", Location: "remote"}, {ID: "lan", Location: "lan"}}}
}

// TestDigestCoversPolicyValues: two auto worlds that differ only in sched.auto.step_sec
// (one waits, one places) have different digests; the same world has a stable one.
func TestDigestCoversPolicyValues(t *testing.T) {
	zero, thirty := stepWorld(t, 0), stepWorld(t, 30)
	a, err := Digest(zero)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Digest(thirty)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("step_sec 0 and 30 must not share a world digest")
	}
	if again, _ := Digest(stepWorld(t, 0)); again != a {
		t.Fatal("the digest of one world must be stable")
	}
}

func isolationWorld(t *testing.T, preset, min string) sched.World {
	t.Helper()
	dec := func(raw json.RawMessage) (any, error) {
		var s string
		return s, json.Unmarshal(raw, &s)
	}
	reg, err := trust.NewRegistry(trust.KeySpec{Key: "trust.isolation_min", Owner: "trust", Kind: trust.TightenHigher, Order: []string{"process", "container", "sandboxed-container", "vm", "ephemeral-vm"}, Default: "process", Decode: dec})
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.isolation_min": json.RawMessage(strconv.Quote(min))}})
	if err != nil {
		t.Fatal(err)
	}
	return sched.World{Policy: sched.Policy{Preset: preset, Trust: eff}, Runners: []sched.Runner{{ID: "box", Location: "lan", Decl: trust.Declaration{Isolation: "container"}}}}
}

// TestDigestCoversPolicyForEveryPreset: balanced worlds that differ only in
// trust.isolation_min (a container runner is eligible under one and not the other) have
// different digests for every preset; identical policies keep identical digests (D-1527).
func TestDigestCoversPolicyForEveryPreset(t *testing.T) {
	for _, preset := range []string{"balanced", "fast", "cheap", "private", sched.AutoPreset} {
		container, vm := isolationWorld(t, preset, "container"), isolationWorld(t, preset, "vm")
		a, err := Digest(container)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Digest(vm)
		if err != nil {
			t.Fatal(err)
		}
		if a == b {
			t.Fatalf("%s: isolation_min container and vm must not share a world digest", preset)
		}
		if again, _ := Digest(isolationWorld(t, preset, "container")); again != a {
			t.Fatalf("%s: identical policies must keep one digest", preset)
		}
	}
}

func TestWithoutSelfReportedRank(t *testing.T) {
	got := withoutSelfReportedRank([]string{"arch=amd64", "rank=0", "pool.x", "rank=10"})
	if len(got) != 2 || got[0] != "arch=amd64" || got[1] != "pool.x" {
		t.Fatalf("got %v", got)
	}
}
