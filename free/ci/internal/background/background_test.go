package background

import (
	"context"
	"math/rand"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func step(r sched.Runner, j sched.Job) error {
	return Step(context.Background(), dispatch.View{Request: dispatch.Request{Runner: r, Job: j}})
}

func reasonOf(t testing.TB, err error) model.PlacementReason {
	t.Helper()
	if err == nil {
		return ""
	}
	ref, ok := err.(*dispatch.Refusal)
	if !ok {
		t.Fatalf("not a refusal: %v", err)
	}
	return ref.Reason
}

// TestBackgroundAdmission: table of the contention rules, then a property over
// 10 000 seeded signal sets against an independent oracle and against the
// placement stage (sched) so the two stages never disagree.
func TestBackgroundAdmission(t *testing.T) {
	cases := []struct {
		name string
		s    signals
		want model.PlacementReason
	}{
		{"idle desk", idleDesk, ""},
		{"on battery", signals{Plugged: ptr(false), Interactive: ptr("idle"), Operator: true}, model.BackgroundContention},
		{"plugged unknown, operator", signals{Interactive: ptr("idle"), Operator: true}, model.BackgroundContention},
		{"plugged unknown, hosted", signals{Interactive: ptr("idle")}, ""},
		{"interactive active", signals{Plugged: ptr(true), Interactive: ptr("active"), Operator: true}, model.BackgroundContention},
		{"interactive unknown, operator", signals{Plugged: ptr(true), Operator: true}, model.BackgroundContention},
		{"interactive literal unknown, operator", signals{Plugged: ptr(true), Interactive: ptr("unknown"), Operator: true}, model.BackgroundContention},
		{"interactive unknown, hosted", signals{Plugged: ptr(true)}, ""},
		{"interactive active, hosted", signals{Plugged: ptr(true), Interactive: ptr("active")}, model.BackgroundContention},
		{"unplugged, hosted", signals{Plugged: ptr(false), Interactive: ptr("idle")}, model.BackgroundContention},
		{"runner active", signals{Plugged: ptr(true), Interactive: ptr("idle"), Operator: true, Active: true}, model.BackgroundContention},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reasonOf(t, step(runner("r", c.s), job("bg", BandName))); got != c.want {
				t.Fatalf("reason = %q, want %q", got, c.want)
			}
		})
	}
	t.Run("detail names every cause", func(t *testing.T) {
		err := step(runner("r", signals{Plugged: ptr(false), Interactive: ptr("active"), Operator: true, Active: true}), job("bg", BandName))
		ref := err.(*dispatch.Refusal)
		for _, want := range []string{string(OnBattery), string(InteractiveActive), string(RunnerActive)} {
			if !contains(ref.Detail, want) {
				t.Fatalf("detail %q lacks %q", ref.Detail, want)
			}
		}
	})
	t.Run("kinds", func(t *testing.T) {
		for _, k := range []string{"schedule", "drill", "flaky-confirmation", "nightly"} {
			if DeclaredBand(k) != BandName {
				t.Fatalf("%s is not background", k)
			}
		}
		for _, k := range []string{"", "push", "pull_request", "Nightly", "manual"} {
			if DeclaredBand(k) != "" || IsBackgroundKind(k) {
				t.Fatalf("%q must not be background", k)
			}
		}
	})
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// randomSignals draws one contention signal set; each fact is unknown a third of the time.
func randomSignals(rng *rand.Rand) (signals, bool) {
	var s signals
	if rng.Intn(3) > 0 {
		s.Plugged = ptr(rng.Intn(2) == 0)
	}
	if rng.Intn(3) > 0 {
		s.BatteryPct = ptr(rng.Intn(101))
	}
	if rng.Intn(3) > 0 {
		s.Interactive = ptr([]string{"idle", "active", "unknown"}[rng.Intn(3)])
	}
	s.Operator, s.Active = rng.Intn(2) == 0, rng.Intn(4) == 0
	return s, rng.Intn(3) == 0
}

// oracle restates the rule as a truth table, written apart from Contended.
func oracle(s signals, queued bool) model.PlacementReason {
	if queued {
		return model.BackgroundInteractiveQueued
	}
	switch {
	case s.Plugged != nil && !*s.Plugged, s.Plugged == nil && s.Operator:
		return model.BackgroundContention
	case s.Interactive != nil && *s.Interactive == "active":
		return model.BackgroundContention
	case s.Operator && (s.Interactive == nil || *s.Interactive != "idle"):
		return model.BackgroundContention
	case s.Active:
		return model.BackgroundContention
	}
	return ""
}

func TestBackgroundAdmissionProperty(t *testing.T) {
	seed := rand.Int63()
	rng := rand.New(rand.NewSource(seed))
	var queued bool
	old := SetQueueProbe(func(string) bool { return queued })
	t.Cleanup(func() { SetQueueProbe(old) })
	admitted, refused := 0, 0
	for i := 0; i < 10000; i++ {
		var s signals
		s, queued = randomSignals(rng)
		got := reasonOf(t, step(runner("r", s), job("bg", BandName)))
		if want := oracle(s, queued); got != want {
			t.Fatalf("seed %d case %d: %+v queued=%v: reason %q, want %q", seed, i, s, queued, got, want)
		}
		if got == "" {
			admitted++
		} else {
			refused++
		}
		// the same signals never refuse a job outside the background band
		if err := step(runner("r", s), job("fg", "interactive")); err != nil {
			t.Fatalf("seed %d case %d: interactive job refused: %v", seed, i, err)
		}
	}
	if admitted < 200 || refused < 200 {
		t.Fatalf("seed %d: the generator is lopsided: %d admitted, %d refused", seed, admitted, refused)
	}
}

// TestBackgroundAdmissionMatchesPlacement: for a background job the placement
// stage (sched.Place) and this step agree on every contention signal set.
func TestBackgroundAdmissionMatchesPlacement(t *testing.T) {
	seed := rand.Int63()
	rng := rand.New(rand.NewSource(seed))
	contention := map[model.PlacementReason]bool{model.NodeOnBattery: true, model.NodeInteractive: true, model.BackgroundContention: true}
	placedN, blockedN := 0, 0
	for i := 0; i < 2000; i++ {
		s, _ := randomSignals(rng)
		r := runner("r", s)
		res := sched.Place(sched.World{Policy: sched.Policy{Preset: "balanced"}, Runners: []sched.Runner{r}}, []sched.Job{job("bg", BandName)})
		placed := len(res.Placements) == 1
		blocked := false
		for _, x := range res.Explanations {
			for _, alt := range x.Alternatives {
				for _, d := range alt.Reasons {
					blocked = blocked || contention[model.PlacementReason(d.Code)]
				}
			}
		}
		stepRefuses := step(r, job("bg", BandName)) != nil
		if placed {
			placedN++
		}
		if blocked {
			blockedN++
		}
		if placed && stepRefuses || blocked && !stepRefuses {
			t.Fatalf("seed %d case %d: %+v: placed=%v blocked-by-contention=%v step-refuses=%v", seed, i, s, placed, blocked, stepRefuses)
		}
	}
	if placedN < 100 || blockedN < 100 {
		t.Fatalf("seed %d: positive control failed: %d placed, %d blocked", seed, placedN, blockedN)
	}
}
