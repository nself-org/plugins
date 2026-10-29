package tiers

// Purpose: the auto keys tighten like every other policy key, decode strictly,
// and the order, rank and bias are what the brief says (P7-AUTO-05).

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func ptr[T any](v T) *T { return &v }

func registry(t *testing.T) trust.Registry {
	t.Helper()
	reg, err := trust.NewRegistry(Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func scope(kind trust.Source, kv ...string) trust.Scope {
	s := trust.Scope{Kind: kind, Values: map[string]json.RawMessage{}}
	for i := 0; i+1 < len(kv); i += 2 {
		s.Values[kv[i]] = json.RawMessage(kv[i+1])
	}
	return s
}

func TestKeysRegistered(t *testing.T) {
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, k := range []string{KeyOrder, KeyStepSec} {
		spec, ok := policy.Default.Find(k)
		if !ok || spec.Owner != Owner || spec.Spend {
			t.Errorf("%s registered as %+v", k, spec)
		}
	}
	if err := world.Register(world.Contributor{Name: "intruder", Field: "Bias", Fn: func(context.Context, *sched.World) error { return nil }}); err == nil {
		t.Fatal("World.Bias has one owner: a second contributor must be refused")
	}
}

func TestTierOrderTighten(t *testing.T) {
	reg := registry(t)
	eff, _, err := reg.Merge("")
	if err != nil || !reflect.DeepEqual(Order(eff), DefaultOrder()) || sched.AutoStepMs(sched.World{Policy: sched.Policy{Trust: eff}}) != 30000 {
		t.Fatalf("defaults: %v %v", Order(eff), err)
	}
	// the operator reorders and drops; the project narrows further.
	eff, _, err = reg.Merge("", scope(trust.SourceUser, KeyOrder, `["lan","vps","local"]`), scope(trust.SourceProject, KeyOrder, `["lan","local"]`, KeyStepSec, `10`))
	if err != nil || !reflect.DeepEqual(Order(eff), []string{"lan", "local"}) || sched.AutoStepMs(sched.World{Policy: sched.Policy{Trust: eff}}) != 10000 {
		t.Fatalf("narrowed: %v %v", Order(eff), err)
	}
	// a lower scope can never add a tier back or raise the wait.
	for name, scopes := range map[string][]trust.Scope{
		"project adds a tier": {scope(trust.SourceUser, KeyOrder, `["lan","local"]`), scope(trust.SourceProject, KeyOrder, `["lan","local","hosted"]`)},
		"project raises step": {scope(trust.SourceUser, KeyStepSec, `5`), scope(trust.SourceProject, KeyStepSec, `20`)},
		"operator over 30 s":  {scope(trust.SourceUser, KeyStepSec, `600`)},
	} {
		if _, _, err := reg.Merge("", scopes...); err == nil || !strings.Contains(err.Error(), "E670") {
			t.Errorf("%s: want E670, got %v", name, err)
		}
	}
	// a revision widening is ignored, not applied.
	eff, _, err = reg.Merge("", scope(trust.SourceProject, KeyOrder, `["vps"]`), scope(trust.SourceRevision, KeyOrder, `["vps","lan"]`))
	if err != nil || !reflect.DeepEqual(Order(eff), []string{"vps"}) || len(eff.Ignored()) != 1 {
		t.Fatalf("revision widening: %v %v %v", Order(eff), eff.Ignored(), err)
	}
	for _, raw := range []string{`[]`, `["cloud"]`, `["lan","lan"]`, `"lan"`, `[1]`, `null`} {
		if _, err := decodeOrder(json.RawMessage(raw)); err == nil {
			t.Errorf("order %s accepted", raw)
		}
	}
	for _, raw := range []string{`-1`, `1.5`, `"30"`, `null`, `1 2`} {
		if _, err := decodeStep(json.RawMessage(raw)); err == nil {
			t.Errorf("step_sec %s accepted", raw)
		}
	}
	if v, err := decodeStep(json.RawMessage(`0`)); err != nil || v != int64(0) {
		t.Fatalf("step_sec 0 means never wait: %v %v", v, err)
	}
}

func TestRankAndBias(t *testing.T) {
	r := func(id, loc string, labels ...string) sched.Runner {
		x := sched.Runner{ID: id, Location: loc, Class: "owned"}
		x.Capability.Identity.Labels = labels
		return x
	}
	w := sched.World{Policy: sched.Policy{Preset: sched.AutoPreset},
		Runners: []sched.Runner{r("a", "lan", "rank=7"), r("b", "lan"), r("c", "remote", "rank=x"), r("d", "local", "rank=999999999")},
		Hosted:  []sched.Runner{{ID: "h", Location: "hosted", Class: "metered"}, {ID: "i", Location: "remote", Class: "included"}},
		Costs:   map[string]sched.Cost{},
	}
	got := Bias(w, DefaultOrder())
	want := map[string]int64{"a": 2*sched.TierStepMs + 7, "b": 2*sched.TierStepMs + 1, "c": sched.TierStepMs + 2, "d": 4*sched.TierStepMs - 1, "h": 0, "i": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bias %v want %v", got, want)
	}
	// a tier the order leaves out goes after every listed tier.
	if b := Bias(w, []string{"local", "lan"}); b["c"] != 2*sched.TierStepMs+2 || b["d"] != sched.TierStepMs-1 || b["a"] != sched.TierStepMs+7 {
		t.Fatalf("narrowed bias %v", b)
	}
	if err := contribute(context.Background(), &w); err != nil || !reflect.DeepEqual(w.Bias, want) {
		t.Fatalf("contributor: %v %v", w.Bias, err)
	}
	w.Bias, w.Policy.Preset = nil, "balanced"
	if err := contribute(context.Background(), &w); err != nil || w.Bias != nil {
		t.Fatalf("other presets get no bias: %v", w.Bias)
	}
	for loc, tier := range map[string]string{"hosted": "hosted", "remote": "vps", "ephemeral": "vps", "lan": "lan", "local": "local"} {
		if got := sched.TierFor(loc, "owned"); got != tier {
			t.Errorf("%s: tier %s want %s", loc, got, tier)
		}
	}
	if sched.TierFor("remote", "included") != "hosted" {
		t.Error("an included-class runner is in the hosted tier")
	}
}
