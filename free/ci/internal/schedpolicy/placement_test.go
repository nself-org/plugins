package schedpolicy

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// d10Keys is the scheduling key list of Epic D10 with each key's tightening.
var d10Keys = map[string]policy.Tightening{
	KeyPreset: policy.TightenAny, KeyLocalOnly: policy.TightenOnlyOn, KeyLANOnly: policy.TightenOnlyOn, KeyPrivateOnly: policy.TightenOnlyOn,
	KeyProvidersAllow: policy.TightenSubset, KeyProvidersDeny: policy.TightenSuperset, KeyLabels: policy.TightenSuperset,
	KeyRunner: policy.TightenFixed, KeyOS: policy.TightenFixed, KeyArch: policy.TightenFixed,
	KeyMaxCostMicro: policy.TightenLower, KeyMaxQueue: policy.TightenLower, KeyMinIsolation: policy.TightenHigher,
	KeyLimitPipeline: policy.TightenLower, KeyLimitJobMatrix: policy.TightenLower, KeyLimitProvider: policy.TightenLower,
	KeyLimitProject: policy.TightenLower, KeyLimitUser: policy.TightenLower, KeyLimitTeam: policy.TightenLower, KeyLimitGlobal: policy.TightenLower,
	KeyStarvationAfter: policy.TightenLower, KeyQueueMaxDepth: policy.TightenLower,
}

func TestKeysRegistered(t *testing.T) {
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatalf("init could not register: %v", errs)
	}
	specs := Keys()
	if len(specs) != len(d10Keys) {
		t.Errorf("%d specs, D10 lists %d keys", len(specs), len(d10Keys))
	}
	for key, kind := range d10Keys {
		spec, ok := policy.Default.Find(strings.ReplaceAll(key, "*", "x"))
		if !ok {
			t.Errorf("%s is not in the process-wide key set", key)
			continue
		}
		if spec.Key != key || spec.Owner != Owner || spec.Kind != kind || spec.Spend || spec.Decode == nil {
			t.Errorf("%s registered as %+v", key, spec)
		}
		if spec.Default == nil && key != KeyProvidersAllow && key != KeyRunner && key != KeyOS && key != KeyArch {
			t.Errorf("%s has no default", key)
		}
	}
	// A second registration is refused, not a panic.
	if err := policy.RegisterKey(Keys()[0]); err == nil {
		t.Error("duplicate registration accepted")
	}
	// Spend keys belong to P7-SCHED-17.
	if _, ok := policy.Default.Find("sched.economics.paid_enabled"); ok {
		t.Error("a spend-enabling key is registered here")
	}
}

func TestSchedPolicyIsolationOrderMatchesTrust(t *testing.T) {
	order := IsolationOrder()
	if len(order) < 2 {
		t.Fatalf("order %v", order)
	}
	for i, v := range order {
		if got := trust.RankIsolation(modelIsolation(v)); got != i {
			t.Errorf("%s ranks %d in trust and %d here", v, got, i)
		}
	}
}

func TestSchedPolicyDecodeRefusals(t *testing.T) {
	cases := []struct{ key, raw, pointer string }{
		{KeyLimitGlobal, `-1`, "/sched/limits/global"},
		{KeyLimitGlobal, `0`, "/sched/limits/global"},
		{KeyLimitGlobal, `2.5`, "/sched/limits/global"},
		{KeyLimitGlobal, `"4"`, "/sched/limits/global"},
		{KeyLimitGlobal, `9223372036854775807`, "/sched/limits/global"},
		{KeyLimitGlobal, `99999999999999999999`, "/sched/limits/global"},
		{KeyLimitProvider, `-3`, "/sched/limits/provider"},
		{KeyMaxCostMicro, `-1`, "/sched/overrides/max_cost_micro"},
		{KeyQueueMaxDepth, `null`, "/sched/queue/max_depth"},
		{KeyPreset, `"fastest-ever"`, "/sched/preset"},
		{KeyPreset, `7`, "/sched/preset"},
		{KeyMaxQueue, `"soon"`, "/sched/overrides/max_queue"},
		{KeyMaxQueue, `"0s"`, "/sched/overrides/max_queue"},
		{KeyMaxQueue, `"-5m"`, "/sched/overrides/max_queue"},
		{KeyMaxQueue, `"1.9ms"`, "/sched/overrides/max_queue"},
		{KeyMaxQueue, `"1ms999us"`, "/sched/overrides/max_queue"},
		{KeyRunner, `"  "`, "/sched/overrides/runner"},
		{KeyMaxQueue, `600`, "/sched/overrides/max_queue"},
		{KeyStarvationAfter, `"-1h"`, "/sched/fairness/starvation_after"},
		{KeyMinIsolation, `"fortress"`, "/sched/overrides/min_isolation"},
		{KeyLocalOnly, `"yes"`, "/sched/overrides/local_only"},
		{KeyProvidersAllow, `"hetzner"`, "/sched/overrides/providers/allow"},
		{KeyProvidersAllow, `["a", 3]`, "/sched/overrides/providers/allow"},
		{KeyProvidersAllow, `[]`, "/sched/overrides/providers/allow"},
		{KeyProvidersDeny, `["a", "a"]`, "/sched/overrides/providers/deny"},
		{KeyLabels, `[""]`, "/sched/overrides/labels"},
		{KeyRunner, `""`, "/sched/overrides/runner"},
		{KeyOS, `{"name": "linux"}`, "/sched/overrides/os"},
		{KeyArch, `{`, "/sched/overrides/arch"},
		{KeyArch, `"arm64" "amd64"`, "/sched/overrides/arch"},
	}
	byKey := map[string]policy.KeySpec{}
	for _, s := range Keys() {
		byKey[s.Key] = s
	}
	for _, tc := range cases {
		_, err := byKey[tc.key].Decode(json.RawMessage(tc.raw))
		if err == nil || !strings.HasPrefix(err.Error(), "E691 "+tc.pointer+":") {
			t.Errorf("%s %s: %v", tc.key, tc.raw, err)
		}
	}
}

func TestSchedPolicyDecodeValues(t *testing.T) {
	byKey := map[string]policy.KeySpec{}
	for _, s := range Keys() {
		byKey[s.Key] = s
	}
	for _, tc := range []struct {
		key, raw string
		want     any
	}{
		{KeyLimitGlobal, `4`, int64(4)},
		{KeyMaxQueue, `"10m"`, int64(600000)},
		{KeyMaxQueue, `"1500ms"`, int64(1500)},
		{KeyStarvationAfter, `"1h"`, int64(3600000)},
		{KeyPreset, `"prefer-local"`, "prefer-local"},
		{KeyMinIsolation, `"vm"`, "vm"},
		{KeyLocalOnly, `true`, true},
		{KeyRunner, `"ms5"`, "ms5"},
	} {
		got, err := byKey[tc.key].Decode(json.RawMessage(tc.raw))
		if err != nil || got != tc.want {
			t.Errorf("%s %s = %#v, %v want %#v", tc.key, tc.raw, got, err, tc.want)
		}
	}
	list, err := byKey[KeyProvidersDeny].Decode(json.RawMessage(`["a","b"]`))
	if err != nil || len(list.([]string)) != 2 {
		t.Errorf("list: %v %v", list, err)
	}
}

func merge(t *testing.T, scopes ...trust.Scope) (policy.Effective, error) {
	t.Helper()
	reg, err := trust.NewRegistry(policy.Default.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", scopes...)
	return eff, err
}

func scope(kind trust.Source, kv map[string]string) trust.Scope {
	v := map[string]json.RawMessage{}
	for k, raw := range kv {
		v[k] = json.RawMessage(raw)
	}
	return trust.Scope{Kind: kind, Values: v}
}

// TestSchedPolicyPlantedRevision: a checkout may only tighten. Raising a
// limit, loosening the isolation floor or widening the allow list changes
// nothing and is recorded; lowering max_queue or adding to deny applies.
func TestSchedPolicyPlantedRevision(t *testing.T) {
	operator := scope(trust.SourceProject, map[string]string{
		KeyLimitGlobal: `8`, KeyMinIsolation: `"vm"`, KeyProvidersAllow: `["a","b"]`, KeyMaxQueue: `"20m"`, KeyProvidersDeny: `["x"]`,
	})
	revision := scope(trust.SourceRevision, map[string]string{
		KeyLimitGlobal: `16`, KeyMinIsolation: `"container"`, KeyProvidersAllow: `["a","b","c"]`, KeyMaxQueue: `"5m"`, KeyProvidersDeny: `["x","y"]`,
	})
	eff, err := merge(t, operator, revision)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Placement(&eff)
	if err != nil {
		t.Fatal(err)
	}
	if p.Limits.Global != 8 || p.Overrides.MinIsolation != "vm" || strings.Join(p.Overrides.ProvidersAllow, ",") != "a,b" {
		t.Errorf("a revision loosened the policy: %+v", p.Overrides)
	}
	if p.Overrides.MaxQueueMs != 300000 || strings.Join(p.Overrides.ProvidersDeny, ",") != "x,y" {
		t.Errorf("a tightening revision was not applied: %+v", p.Overrides)
	}
	// An empty allow list reads as "every provider" in the scheduler, so a
	// revision cannot narrow to [] (refused by Decode) and a stored [] is refused too.
	if _, err := merge(t, operator, scope(trust.SourceRevision, map[string]string{KeyProvidersAllow: `[]`})); err == nil || !strings.Contains(err.Error(), "E691") {
		t.Errorf("an empty allow list was accepted: %v", err)
	}
	ignored := map[string]bool{}
	for _, ig := range eff.Ignored() {
		ignored[ig.Key] = true
	}
	for _, key := range []string{KeyLimitGlobal, KeyMinIsolation, KeyProvidersAllow} {
		if !ignored[key] {
			t.Errorf("%s: ignored change not recorded: %v", key, eff.Ignored())
		}
	}
	if ignored[KeyMaxQueue] || ignored[KeyProvidersDeny] {
		t.Errorf("a tightening was recorded as ignored: %v", eff.Ignored())
	}
}

func TestSchedPolicyLoaderErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		kv   map[string]string
		code string
	}{
		"negative limit":           {map[string]string{KeyLimitGlobal: `-1`}, "E691 /sched/limits/global"},
		"unknown preset":           {map[string]string{KeyPreset: `"warp"`}, "E691 /sched/preset"},
		"unknown key in overrides": {map[string]string{"sched.overrides.turbo": `true`}, "E672"},
		"unknown key in limits":    {map[string]string{"sched.limits.nope.deep": `3`}, "E672"},
		"bad provider limit":       {map[string]string{"sched.limits.provider.hetzner": `0`}, "E691"},
		"unknown min isolation":    {map[string]string{KeyMinIsolation: `"moat"`}, "E691 /sched/overrides/min_isolation"},
		"duration with a bad unit": {map[string]string{KeyStarvationAfter: `"3 fortnights"`}, "E691"},
		"a list where a bool goes": {map[string]string{KeyLANOnly: `[true]`}, "E691"},
		"float project limit":      {map[string]string{KeyLimitProject: `1.5`}, "E691"},
	} {
		_, err := merge(t, scope(trust.SourceProject, tc.kv))
		if err == nil || !strings.Contains(err.Error(), tc.code) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSchedPolicyDefaultsAndViews(t *testing.T) {
	eff, err := merge(t)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Placement(&eff)
	if err != nil {
		t.Fatal(err)
	}
	if p.Preset != "balanced" || p.Limits.Global != 0 || p.Limits.Pipeline != 0 || p.Overrides.MaxQueueMs != 0 || p.Overrides.MaxCostMicro != 0 || p.Overrides.MinIsolation != "process" || p.Overrides.LocalOnly {
		t.Errorf("defaults: %+v", p)
	}
	q, err := Queue(&eff)
	if err != nil || q.StarvationAfterMs != 1800000 || q.MaxDepth != DefaultQueueMaxDepth {
		t.Errorf("queue defaults: %+v %v", q, err)
	}
	tight, err := merge(t, scope(trust.SourceProject, map[string]string{KeyQueueMaxDepth: `50`, KeyStarvationAfter: `"10m"`, KeyLimitJobMatrix: `3`, KeyLimitTeam: `6`, KeyMaxCostMicro: `250000`, KeyLANOnly: `true`}))
	if err != nil {
		t.Fatal(err)
	}
	if q, _ = Queue(&tight); q.StarvationAfterMs != 600000 || q.MaxDepth != 50 {
		t.Errorf("queue: %+v", q)
	}
	if p, _ = Placement(&tight); p.Limits.Job != 3 || p.Limits.Team != 6 || p.Overrides.MaxCostMicro != 250000 || !p.Overrides.LANOnly {
		t.Errorf("tightened: %+v", p)
	}
	if _, err := Placement(nil); err == nil || !strings.HasPrefix(err.Error(), "E691") {
		t.Errorf("nil policy: %v", err)
	}
	if _, err := Queue(nil); err == nil {
		t.Error("nil policy accepted by Queue")
	}
}

// TestSchedPolicyMissingKeys: a policy merged without the sched keys is an
// error, never "no limit".
func TestSchedPolicyMissingKeys(t *testing.T) {
	reg, err := trust.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Placement(&eff); err == nil || !strings.HasPrefix(err.Error(), "E691") {
		t.Errorf("Placement of a policy with no sched keys: %v", err)
	}
	if _, err = Queue(&eff); err == nil || !strings.HasPrefix(err.Error(), "E691") {
		t.Errorf("Queue of a policy with no sched keys: %v", err)
	}
}

func TestSchedPolicyProviderLimits(t *testing.T) {
	got, err := ProviderLimits(map[string]any{
		"sched.limits.provider.github": int64(4), "sched.limits.provider.hetzner": NoLimit,
		"sched.limits.global": int64(2), "sched.limits.provider.a.b": int64(1), "sched.limits.provider.": int64(1),
	})
	if err != nil || len(got) != 1 || got["github"] != 4 {
		t.Errorf("%v %v", got, err)
	}
	if _, err = ProviderLimits(map[string]any{"sched.limits.provider.x": int64(0)}); err == nil {
		t.Error("a zero provider limit was accepted")
	}
	if limit(math.MaxInt64) != math.MaxInt {
		t.Error("limit does not clamp to int")
	}
}
