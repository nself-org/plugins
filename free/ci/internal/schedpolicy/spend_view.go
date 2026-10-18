package schedpolicy

// Purpose: the typed view of the effective spend policy (Epic D10-D13):
// Spend builds SpendPolicy, PaidAllowed is the paid-enable rule.
// Inputs: *policy.Resolved from the P7-TRUST-27 resolver.
// Outputs: SpendPolicy and a bool.
// Constraints: pure; a value of the wrong type or a missing registered key is
// an E691 error (never a silent zero, which would read as free or unlimited);
// a nil Resolved is an error because defaults would drop every override.

import (
	"slices"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// SpendPolicy is the typed spend policy of one effective policy. P7-SCHED-07's
// economics.Policy is an alias of it (economics imports schedpolicy, so the
// type cannot live in economics). Budget fields with Set false are unset, which
// is different from a budget of 0.
type SpendPolicy struct {
	Currency               string
	PaidEnabled            bool
	MaxPaidTimeoutMs       int64
	ApprovalThresholdMicro Budget
	MarginMicro            int64
	Rates                  map[string]Rate
	CeilingsMicro          map[string]int64
	Budgets                Budgets
	OwnerCoordinator       map[string]string
	Burst                  BurstPolicy
}

// Budget is an optional amount in micro-units.
type Budget struct {
	Micro int64
	Set   bool
}

// Budgets are the D11 limits; Provider is per provider id.
type Budgets struct {
	Job, Pipeline, Day, Month Budget
	Provider                  map[string]int64
}

// Rate is one provider's declared price; exactly one of the per-hour and
// per-minute rates is non-zero for a paid class.
type Rate struct {
	Class                                               string
	PerHourMicro, PerMinuteMicro                        int64
	BillingIncrementMs, MinimumChargeMicro, ExtrasMicro int64
}

// BurstPolicy is D12-D13: enabled providers, caps, trust and the cloud VM.
type BurstPolicy struct {
	Enabled       map[string]bool
	MaxConcurrent int64
	MarginMs      int64
	Trust         BurstTrust
	CloudVM       CloudVM
}

// BurstTrust is what an enrolled burst VM is trusted with (D13 defaults).
type BurstTrust struct {
	Accepts, Secrets       []string
	Isolation, PrivacyZone string
}

// CloudVM is the cloud VM recipe; Agent is by arch.
type CloudVM struct {
	Type, Location, Image, EnrollAddr, EnrollURL, TokenFile string
	Agent                                                   map[string]AgentBinary
}

// AgentBinary is the https URL and sha256 of the agent for one arch.
type AgentBinary struct{ URL, SHA256 string }

// PaidAllowed reports whether a paid class may be placed on provider: the
// operator turned paid spend on AND set a day or month budget (overall) or a
// budget for that provider above zero (review F6; I19).
func PaidAllowed(p SpendPolicy, provider string) bool {
	return p.PaidEnabled && (p.Budgets.Day.Micro > 0 || p.Budgets.Month.Micro > 0 || p.Budgets.Provider[provider] > 0)
}

// Spend builds the typed spend policy. Resolved.Values carries the per-provider
// family entries (Effective has no key iterator).
func Spend(res *policy.Resolved) (SpendPolicy, error) {
	if res == nil {
		return SpendPolicy{}, fail("economics", "no resolved policy")
	}
	r := &spendReader{eff: &res.Effective}
	p := SpendPolicy{
		Currency: r.text(KeyCurrency), PaidEnabled: r.flag(KeyPaidEnabled), MaxPaidTimeoutMs: r.number(KeyMaxPaidTimeout), MarginMicro: r.number(KeyMargin),
		Rates: map[string]Rate{}, CeilingsMicro: map[string]int64{}, OwnerCoordinator: map[string]string{},
		Budgets: Budgets{Provider: map[string]int64{}},
		Burst:   BurstPolicy{Enabled: map[string]bool{}, MaxConcurrent: r.number(KeyBurstConcurrent), MarginMs: r.number(KeyBurstMargin), Trust: r.trust(), CloudVM: CloudVM{Agent: map[string]AgentBinary{}}},
	}
	for _, b := range []struct {
		key string
		to  *Budget
	}{{KeyBudgetJob, &p.Budgets.Job}, {KeyBudgetPipeline, &p.Budgets.Pipeline}, {KeyBudgetDay, &p.Budgets.Day}, {KeyBudgetMonth, &p.Budgets.Month}, {KeyApproval, &p.ApprovalThresholdMicro}} {
		if v, ok := r.optional(b.key); ok {
			*b.to = Budget{Micro: v.(int64), Set: true}
		}
	}
	for _, s := range []struct {
		key string
		to  *string
	}{{KeyCloudType, &p.Burst.CloudVM.Type}, {KeyCloudLocation, &p.Burst.CloudVM.Location}, {KeyCloudImage, &p.Burst.CloudVM.Image},
		{KeyCloudEnrollAddr, &p.Burst.CloudVM.EnrollAddr}, {KeyCloudEnrollURL, &p.Burst.CloudVM.EnrollURL}, {KeyCloudTokenFile, &p.Burst.CloudVM.TokenFile}} {
		if v, ok := r.optional(s.key); ok {
			*s.to = v.(string)
		}
	}
	keys := make([]string, 0, len(res.Values))
	for key := range res.Values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if spec, ok := policy.Default.Find(key); !ok || !spec.Spend || !strings.HasSuffix(spec.Key, ".*") {
			continue
		}
		v := res.Values[key]
		if err := checkFamily(key, v); err != nil {
			return SpendPolicy{}, err
		}
		id := key[strings.LastIndex(key, ".")+1:]
		switch spec, _ := policy.Default.Find(key); spec.Key {
		case KeyRates:
			p.Rates[id] = rateFrom(v.(map[string]any))
		case KeyCeilings:
			p.CeilingsMicro[id], _ = trust.IntValue(v)
		case KeyBudgetProvider:
			p.Budgets.Provider[id], _ = trust.IntValue(v)
		case KeyEconProviders:
			p.OwnerCoordinator[id], _ = v.(map[string]any)["owner_coordinator"].(string)
		case KeyBurstProviders:
			p.Burst.Enabled[id], _ = v.(map[string]any)["enabled"].(bool)
		case KeyCloudAgent:
			m := v.(map[string]any)
			p.Burst.CloudVM.Agent[id] = AgentBinary{URL: m["url"].(string), SHA256: m["sha256"].(string)}
		}
	}
	if r.err != nil {
		return SpendPolicy{}, r.err
	}
	return p, nil
}

func rateFrom(m map[string]any) Rate {
	n := func(name string) int64 { v, _ := m[name].(int64); return v }
	class, _ := m["class"].(string)
	return Rate{Class: class, PerHourMicro: n("rate_per_hour_micro"), PerMinuteMicro: n("rate_per_minute_micro"),
		BillingIncrementMs: n("billing_increment"), MinimumChargeMicro: n("minimum_charge_micro"), ExtrasMicro: n("extras_micro")}
}

// spendReader reads scalar keys; a key with a registered default that is
// missing means the keys were never registered, which is an error, never a
// silent zero (a zero would read as "no limit" or "free").
type spendReader struct {
	eff *policy.Effective
	err error
}

func (r *spendReader) optional(key string) (any, bool) {
	v, _, ok := r.eff.Value(key)
	return v, ok
}

func (r *spendReader) required(key string) any {
	v, ok := r.optional(key)
	if !ok && r.err == nil {
		r.err = fail(key, "missing from the effective policy (are the spend keys registered?)")
	}
	return v
}

func (r *spendReader) text(key string) string { s, _ := r.required(key).(string); return s }
func (r *spendReader) flag(key string) bool   { b, _ := r.required(key).(bool); return b }

func (r *spendReader) number(key string) int64 {
	n, ok := trust.IntValue(r.required(key))
	if !ok && r.err == nil {
		r.err = fail(key, "effective value is not a whole number")
	}
	return n
}

func (r *spendReader) trust() BurstTrust {
	d := BurstTrust{Accepts: slices.Clone(burstAccepts), Secrets: []string{"none"}, Isolation: defaultBurstIsolation, PrivacyZone: defaultBurstPrivacyZone}
	v, ok := r.optional(KeyBurstTrust)
	if !ok {
		return d
	}
	m, _ := v.(map[string]any)
	iso, _ := m["isolation"].(string)
	zone, _ := m["privacy_zone"].(string)
	return BurstTrust{Accepts: asStrings(m["accepts"]), Secrets: asStrings(m["secrets"]), Isolation: iso, PrivacyZone: zone}
}
