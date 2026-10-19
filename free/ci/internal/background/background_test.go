package background

import (
	"context"
	"math/rand"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/fairq"
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
		{"interactive unrecognised, operator", signals{Plugged: ptr(true), Interactive: ptr("away"), Operator: true}, model.BackgroundContention},
		{"interactive unrecognised, hosted", signals{Plugged: ptr(true), Interactive: ptr("away")}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reasonOf(t, step(runner("r", c.s), job("bg", BandName))); got != c.want {
				t.Fatalf("reason = %q, want %q", got, c.want)
			}
		})
	}
	t.Run("detail names every cause", func(t *testing.T) {
		err := step(runner("r", signals{Plugged: ptr(false), Interactive: ptr("active"), Operator: true}), job("bg", BandName))
		ref := err.(*dispatch.Refusal)
		for _, want := range []string{string(OnBattery), string(InteractiveActive)} {
			if !contains(ref.Detail, want) {
				t.Fatalf("detail %q lacks %q", ref.Detail, want)
			}
		}
	})
	// either ownership source marks an operator machine (the declaration, the capability fact)
	t.Run("ownership sources", func(t *testing.T) {
		for _, c := range []struct {
			name       string
			decl, fact string
			want       model.PlacementReason
		}{
			{"declaration only", "operator", "", model.BackgroundContention},
			{"fact only", "", "operator", model.BackgroundContention},
			{"unknown ownership fails closed", "", "", model.BackgroundContention},
			{"both foreign", "provider", "team", ""},
			{"operator wins over foreign", "operator", "provider", model.BackgroundContention},
		} {
			r := runner("r", signals{Interactive: ptr("idle")}) // power unknown
			r.Decl.Ownership, r.Capability.Identity.Ownership.Value = c.decl, nil
			if c.fact != "" {
				r.Capability.Identity.Ownership.Value = ptr(c.fact)
			}
			if got := reasonOf(t, step(r, job("bg", BandName))); got != c.want {
				t.Fatalf("%s: reason = %q, want %q", c.name, got, c.want)
			}
		}
	})
	t.Run("hosted and ephemeral runners are never operator-owned", func(t *testing.T) {
		for name, mutate := range map[string]func(*sched.Runner){
			"hosted location": func(r *sched.Runner) { r.Location = "hosted" },
			"ephemeral":       func(r *sched.Runner) { r.Ephemeral = true },
			"hosted decl":     func(r *sched.Runner) { r.Decl.Hosted = true },
			"hosted fact":     func(r *sched.Runner) { r.Capability.Location.Kind.Value = ptr("hosted") },
		} {
			r := runner("r", signals{Interactive: ptr("idle")}) // power unknown, ownership unknown
			r.Decl.Ownership, r.Capability.Identity.Ownership.Value = "", nil
			if reasonOf(t, step(r, job("bg", BandName))) != model.BackgroundContention {
				t.Fatalf("%s: control: the same runner not marked hosted must refuse", name)
			}
			mutate(&r)
			if got := reasonOf(t, step(r, job("bg", BandName))); got != "" {
				t.Fatalf("%s: unknown power on a hosted runner must admit, got %q", name, got)
			}
		}
	})
	t.Run("online flag alone is not contention", func(t *testing.T) {
		if got := reasonOf(t, step(runner("r", signals{Plugged: ptr(true), Interactive: ptr("idle"), Operator: true, Active: true}), job("bg", BandName))); got != "" {
			t.Fatalf("Runner.Active is the world's online flag; reason = %q", got)
		}
	})
	t.Run("band name matches fairq", func(t *testing.T) {
		if BandName != fairq.Background.String() {
			t.Fatalf("BandName %q != fairq %q", BandName, fairq.Background)
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
	s.Operator = rng.Intn(2) == 0
	s.OwnerUnknown = rng.Intn(5) == 0
	return s, rng.Intn(3) == 0
}

// oracle restates the rule as a truth table, written apart from Contended.
func oracle(s signals, queued bool) model.PlacementReason {
	if queued {
		return model.BackgroundInteractiveQueued
	}
	switch {
	case s.Plugged != nil && !*s.Plugged, s.Plugged == nil && (s.Operator || s.OwnerUnknown):
		return model.BackgroundContention
	case s.Interactive != nil && *s.Interactive == "active":
		return model.BackgroundContention
	case (s.Operator || s.OwnerUnknown) && (s.Interactive == nil || *s.Interactive != "idle"):
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
		s.OwnerUnknown = false // the one deliberate difference: sched counts unknown ownership as not owned, the step fails closed
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
