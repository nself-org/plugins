//go:build scenarios

package scenarios

// Purpose: Epic acceptance 3, scenario 6 (hybrid). Local, two LAN fake nodes and the P7-HOST-01
// fake hosted provider under the presets balanced and preserve-hosted-quota: placements follow
// the D4 fixture orders, `ci explain` data lists every alternative, switching presets moves work
// off hosted within a bias read from the fixture, and the result is deterministic.
// Inputs:  internal/sched/testdata/presets/world.json, sched.Place, a real dispatcher and store.
// Outputs: assertions only.
// Constraints: in process, no sleeps; expected scores for L, N and H are the ones
//          internal/sched/preset_test.go pins for the same fixture (N2 is N plus its extra wait).

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// s6Expected are the fixture scores (preset_test.go) and the order they imply; N2 is N + 5000.
var s6Expected = map[string]struct {
	scores map[string]int64
	order  string
}{
	"balanced":              {map[string]int64{"L": 90000, "N": 57000, "N2": 62000, "H": 118000}, "N N2 L H"},
	"preserve-hosted-quota": {map[string]int64{"L": 90000, "N": 57000, "N2": 62000, "H": 253000}, "N N2 L H"},
}

// s6Place places one job (key j) over the hybrid world under preset.
func s6Place(t testing.TB, preset string, extraWait int64) sched.Result {
	t.Helper()
	rs, rows := s6Hybrid(t, extraWait)
	return sched.Place(s6World(preset, rs, rows, "j"), []sched.Job{job("j", "linux/amd64")})
}

// s6Order sorts the explanation's eligible alternatives by score, ties by id, as the fixture does.
func s6Order(x sched.Explanation) string {
	scores, _ := s6Scores(x)
	ids := make([]string, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, k int) bool {
		if scores[ids[i]] == scores[ids[k]] {
			return ids[i] < ids[k]
		}
		return scores[ids[i]] < scores[ids[k]]
	})
	return strings.Join(ids, " ")
}

// TestScenario6PlacementsFollowTheFixtureOrder covers (a) and (b): the placement is the fixture's
// first choice and `ci explain` lists every runner, the ineligible one with its reason.
func TestScenario6PlacementsFollowTheFixtureOrder(t *testing.T) {
	for preset, want := range s6Expected {
		rs, rows := s6Hybrid(t, 0)
		off := runner("N3", "lan", "linux/amd64", 1) // a fifth runner that is offline: it must still be listed
		off.Capability.Availability.State.Value = ptr("offline")
		rs = append(rs, off)
		rows["N3"] = rows["N"]
		res := sched.Place(s6World(preset, rs, rows, "j"), []sched.Job{job("j", "linux/amd64")})
		if len(res.Placements) != 1 || res.Placements[0].RunnerID != strings.Fields(want.order)[0] {
			t.Fatalf("%s: placement %+v, want %s", preset, res.Placements, want.order)
		}
		x := res.Explanations[0]
		scores, ids := s6Scores(x)
		sort.Strings(ids)
		if strings.Join(ids, " ") != "H L N N2 N3" {
			t.Fatalf("%s: explain lists %v, want all five runners (none dropped)", preset, ids)
		}
		for _, a := range x.Alternatives {
			switch {
			case a.RunnerID == "N3" && (a.Eligible || !hasCode(a.Reasons, string(model.NodeOffline)) || a.Score != nil):
				t.Fatalf("%s: offline N3 must be listed ineligible with node.offline: %+v", preset, a)
			case a.RunnerID != "N3" && (!a.Eligible || a.Score == nil || len(a.Components) < 9):
				t.Fatalf("%s: eligible %s lacks its score components: %+v", preset, a.RunnerID, a)
			}
		}
		for id, sc := range want.scores {
			if scores[id] != sc {
				t.Errorf("%s: %s scores %d, fixture says %d", preset, id, scores[id], sc)
			}
		}
		if got := s6Order(x); got != want.order {
			t.Fatalf("%s: order %q, want %q", preset, got, want.order)
		}
	}
}

// TestScenario6PreserveHostedQuotaMovesWorkOffHosted covers (c). The bias that pushes hosted back
// is read from the two presets' scores for H; the switch point is derived from it, not guessed.
func TestScenario6PreserveHostedQuotaMovesWorkOffHosted(t *testing.T) {
	score := func(res sched.Result, id string) int64 {
		sc, _ := s6Scores(res.Explanations[0])
		return sc[id]
	}
	bal, pres := s6Place(t, "balanced", 0), s6Place(t, "preserve-hosted-quota", 0)
	bias := score(pres, "H") - score(bal, "H")
	p, b, scarcity := sched.Presets()["preserve-hosted-quota"], sched.Presets()["balanced"], s6Fixture(t)["H"].Scarcity
	if want := (p.Weights[7]-b.Weights[7])*scarcity/1000 + p.ClassBiasMs[2] - b.ClassBiasMs[2]; bias != want || bias <= 0 {
		t.Fatalf("hosted bias %d, presets say %d", bias, want)
	}
	lan := score(bal, "N")                 // N is the best non-hosted runner under both presets
	balanceSwitch := score(bal, "H") - lan // extra wait at which balanced prefers hosted
	presSwitch := score(pres, "H") - lan   // extra wait at which preserve-hosted-quota still prefers the LAN
	if balanceSwitch >= presSwitch {
		t.Fatalf("no window: balanced switches at %d, preserve at %d", balanceSwitch, presSwitch)
	}
	for _, c := range []struct {
		extra               int64
		balanced, preserved string
	}{
		{balanceSwitch + 1, "H", "N"}, {(balanceSwitch + presSwitch) / 2, "H", "N"}, {presSwitch - 1, "H", "N"}, {presSwitch + 1, "H", "H"},
	} {
		for preset, want := range map[string]string{"balanced": c.balanced, "preserve-hosted-quota": c.preserved} {
			res := s6Place(t, preset, c.extra)
			if len(res.Placements) != 1 || res.Placements[0].RunnerID != want {
				t.Fatalf("%s with %d ms extra wait placed %+v, want %s (window %d..%d)", preset, c.extra, res.Placements, want, balanceSwitch, presSwitch)
			}
		}
	}
}

// TestScenario6ResultIsDeterministic covers (d): shuffled runner lists give byte-equal JSON.
func TestScenario6ResultIsDeterministic(t *testing.T) {
	seed := invariants.Seed()
	rng := rand.New(rand.NewSource(seed))
	for _, preset := range []string{"balanced", "preserve-hosted-quota"} {
		rs, rows := s6Hybrid(t, 0)
		var first []byte
		for i := 0; i < 12; i++ {
			rng.Shuffle(len(rs), func(a, b int) { rs[a], rs[b] = rs[b], rs[a] })
			raw, err := json.Marshal(sched.Place(s6World(preset, rs, rows, "j"), []sched.Job{job("j", "linux/amd64")}))
			if err != nil {
				t.Fatal(err)
			}
			if first == nil {
				first = raw
			} else if !bytes.Equal(first, raw) {
				t.Fatalf("seed %d: %s differs after permutation %d:\n%s\n%s", seed, preset, i, first, raw)
			}
		}
	}
}

// TestScenario6DispatcherMovesOffHosted drives the same switch through the real dispatcher: the
// store records H under balanced and the LAN node N under preserve-hosted-quota; a hosted lease
// is delivered through the fake provider.
func TestScenario6DispatcherMovesOffHosted(t *testing.T) {
	const extra = int64(100000) // inside the window the previous test derives
	for preset, wantRunner := range map[string]string{"balanced": "H", "preserve-hosted-quota": "N"} {
		st := openStore(t)
		rs, rows := s6Hybrid(t, extra)
		prov := fake.New(fake.Options{})
		tr := &transport{finish: func(d dispatch.Delivery) error {
			if !d.Hosted {
				return nil
			}
			_, err := prov.Dispatch(context.Background(), providers.Attempt{ID: d.AttemptID, Revision: d.Revision})
			return err
		}}
		attempt := queueAttempt(t, st, "pipe", "project", "build")
		granted := s6World(preset, nil, nil, "").Policy.Trust
		tweak := func(w *sched.World) {
			w.Policy.Preset, w.Policy.Trust, w.Estimates = preset, granted, s6Estimates(rows, attempt)
		}
		d := newDispatcher(t, dispatch.Config{Store: st, World: liveWorld(t, st, tweak, rs...), Poll: time.Millisecond, Transport: tr, Clock: newClock().Now})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		a, err := d.Submit(ctx, dispatch.Job{AttemptID: attempt, Sched: job(attempt, "linux/amd64")})
		cancel()
		if err != nil || a.RunnerID != wantRunner {
			t.Fatalf("%s: runner %q (%v), want %s", preset, a.RunnerID, err, wantRunner)
		}
		if _, _, _, runnerID, _, ok, err := st.AdmittedIdentity(context.Background(), attempt); err != nil || !ok || runnerID != wantRunner {
			t.Fatalf("%s: the store records runner %q (%v, %v)", preset, runnerID, ok, err)
		}
		d.Close()
		got := tr.deliveries()
		if len(got) != 1 || got[0].Hosted != (wantRunner == "H") {
			t.Fatalf("%s: deliveries %+v", preset, got)
		}
	}
}
