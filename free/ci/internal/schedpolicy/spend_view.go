package schedpolicy

// Purpose: the typed view of the effective spend policy (Epic D10-D13):
// Spend builds SpendPolicy, PaidAllowed is the paid-enable rule.
// Inputs: *policy.Resolved from the P7-TRUST-27 resolver.
// Outputs: SpendPolicy and a bool.
// Constraints: pure; a value of the wrong type or a missing registered key is
// an E691 error (never a silent zero, which would read as free or unlimited);
// a nil Resolved is an error because defaults would drop every override.

import (
	"reflect"
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

// PaidAllowed reports whether a paid class may be placed on provider (never GitHub): the
// operator turned paid spend on AND set a day or month budget (overall) or a
// budget for that provider above zero (review F6; I19).
func PaidAllowed(p SpendPolicy, provider string) bool {
	return p.PaidEnabled && !isGitHubID(provider) && (p.Budgets.Day.Set && p.Budgets.Day.Micro > 0 || p.Budgets.Month.Set && p.Budgets.Month.Micro > 0 || p.Budgets.Provider[provider] > 0)
}

// Spend builds the typed spend policy. Resolved.Values carries the per-provider
// family entries (Effective has no key iterator), so a nil Values is refused:
// reading it as "no providers" would silently drop every rate and budget.
// Every read is checked; a wrong-typed or missing value is E691, never a zero.
func Spend(res *policy.Resolved) (SpendPolicy, error) {
	if res == nil || res.Values == nil {
		return SpendPolicy{}, fail("economics", "no resolved policy values (Spend needs Resolved.Values for the per-provider keys)")
	}
	// Resolver output lists every registered default key; a map without them was built
	// by hand or stripped, and its per-provider entries cannot be trusted either.
	for _, spec := range SpendKeys() {
		if _, listed := res.Values[spec.Key]; spec.Default != nil && !listed {
			return SpendPolicy{}, fail(spec.Key, "missing from Resolved.Values (incomplete value list)")
		}
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
			*b.to = Budget{Micro: r.amount(b.key, v), Set: true}
		}
	}
	for _, s := range []struct {
		key string
		to  *string
	}{{KeyCloudType, &p.Burst.CloudVM.Type}, {KeyCloudLocation, &p.Burst.CloudVM.Location}, {KeyCloudImage, &p.Burst.CloudVM.Image},
		{KeyCloudEnrollAddr, &p.Burst.CloudVM.EnrollAddr}, {KeyCloudEnrollURL, &p.Burst.CloudVM.EnrollURL}, {KeyCloudTokenFile, &p.Burst.CloudVM.TokenFile}} {
		if v, ok := r.optional(s.key); ok {
			str, isStr := v.(string)
			if !isStr || str == "" {
				r.fail(s.key, "effective value is not a non-empty string")
			}
			*s.to = str
		}
	}
	keys := make([]string, 0, len(res.Values))
	for key := range res.Values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		spec, ok := policy.Default.Find(key)
		if !ok || !spec.Spend || !strings.HasSuffix(spec.Key, ".*") {
			continue
		}
		v := res.Values[key]
		if ev, _, found := res.Effective.Value(key); !found || !reflect.DeepEqual(ev, v) {
			return SpendPolicy{}, fail(key, "Resolved.Values differs from the effective policy")
		}
		if err := checkFamily(key, v); err != nil {
			return SpendPolicy{}, err
		}
		id := key[strings.LastIndex(key, ".")+1:]
		switch spec.Key {
		case KeyCeilings:
			p.CeilingsMicro[id] = r.amount(key, v)
		case KeyBudgetProvider:
			p.Budgets.Provider[id] = r.amount(key, v)
		default:
			o := r.object(key, v)
			switch spec.Key {
			case KeyRates:
				p.Rates[id] = o.rate()
			case KeyEconProviders:
				p.OwnerCoordinator[id] = o.text("owner_coordinator")
			case KeyBurstProviders:
				p.Burst.Enabled[id] = o.flag("enabled")
			case KeyCloudAgent:
				p.Burst.CloudVM.Agent[id] = AgentBinary{URL: o.text("url"), SHA256: o.text("sha256")}
			}
		}
	}
	if r.err != nil {
		return SpendPolicy{}, r.err
	}
	return p, nil
}

// spendReader reads scalar keys; a key with a registered default that is
// missing means the keys were never registered, which is an error, never a
// silent zero (a zero would read as "no limit" or "free"). The first error wins.
type spendReader struct {
	eff *policy.Effective
	err error
}

func (r *spendReader) fail(key, format string, args ...any) {
	if r.err == nil {
		r.err = fail(key, format, args...)
	}
}

func (r *spendReader) optional(key string) (any, bool) {
	v, _, ok := r.eff.Value(key)
	return v, ok
}

func (r *spendReader) required(key string) any {
	v, ok := r.optional(key)
	if !ok {
		r.fail(key, "missing from the effective policy (are the spend keys registered?)")
	}
	return v
}

func (r *spendReader) text(key string) string {
	s, ok := r.required(key).(string)
	if !ok || s == "" {
		r.fail(key, "effective value is not a non-empty string")
	}
	return s
}

func (r *spendReader) flag(key string) bool {
	b, ok := r.required(key).(bool)
	if !ok {
		r.fail(key, "effective value is not a boolean")
	}
	return b
}

func (r *spendReader) number(key string) int64 { return r.amount(key, r.required(key)) }

// amount returns v as a non-negative whole number.
func (r *spendReader) amount(key string, v any) int64 {
	n, ok := trust.IntValue(v)
	if !ok || n < 0 {
		r.fail(key, "effective value %v is not a non-negative whole number", v)
		return 0
	}
	return n
}

// fields reads the members of one object-valued family entry.
type fields struct {
	r   *spendReader
	key string
	m   map[string]any
}

func (r *spendReader) object(key string, v any) fields {
	m, ok := v.(map[string]any)
	if !ok {
		r.fail(key, "effective value is not an object")
	}
	return fields{r: r, key: key, m: m}
}

// int returns a member that may be absent (0); a wrong type is E691.
func (f fields) int(name string) int64 {
	v, ok := f.m[name]
	if !ok {
		return 0
	}
	return f.r.amount(f.key+"."+name, v)
}

func (f fields) text(name string) string {
	s, ok := f.m[name].(string)
	if !ok || s == "" {
		f.r.fail(f.key+"."+name, "effective member is missing or not a non-empty string")
	}
	return s
}

func (f fields) flag(name string) bool {
	b, ok := f.m[name].(bool)
	if !ok {
		f.r.fail(f.key+"."+name, "effective member is missing or not a boolean")
	}
	return b
}

// rate reads a declared price and re-checks what Decode guarantees: a paid
// class has a positive rate and a billing increment (never a free paid class).
func (f fields) rate() Rate {
	rt := Rate{Class: f.text("class"), PerHourMicro: f.int("rate_per_hour_micro"), PerMinuteMicro: f.int("rate_per_minute_micro"),
		BillingIncrementMs: f.int("billing_increment"), MinimumChargeMicro: f.int("minimum_charge_micro"), ExtrasMicro: f.int("extras_micro")}
	if isPaidClass(rt.Class) && (rt.PerHourMicro+rt.PerMinuteMicro < 1 || rt.BillingIncrementMs < 1) {
		f.r.fail(f.key, "a paid class needs a positive rate and a billing_increment")
	}
	return rt
}

func (r *spendReader) trust() BurstTrust {
	d := BurstTrust{Accepts: slices.Clone(burstAccepts), Secrets: []string{"none"}, Isolation: defaultBurstIsolation, PrivacyZone: defaultBurstPrivacyZone}
	v, ok := r.optional(KeyBurstTrust)
	if !ok {
		return d
	}
	o := r.object(KeyBurstTrust, v)
	t := BurstTrust{Accepts: asStrings(o.m["accepts"]), Secrets: asStrings(o.m["secrets"]), Isolation: o.text("isolation"), PrivacyZone: o.text("privacy_zone")}
	if len(t.Accepts) == 0 || len(t.Secrets) == 0 {
		r.fail(KeyBurstTrust, "effective accepts and secrets must be non-empty string lists")
	}
	return t
}
