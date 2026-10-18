package schedpolicy

// Tests of the spend keys (P7-SCHED-17). The four verify-named tests are the
// entry points: TestSpendKeys (registry introspection, defaults, a full typed
// view), TestSpendPlanted (branch and revision never apply a spend key),
// TestPaidAllowed (the paid-enable rule) and TestSpendFlagged (every refusal).
// All values are synthetic; no test opens a file, dials a host or reads a token.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const sha64 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// operatorSpend is a plausible operator-scope spend policy.
var operatorSpend = map[string]string{
	KeyCurrency: `"EUR"`, KeyPaidEnabled: `true`, KeyMaxPaidTimeout: `"3h"`, KeyMargin: `250000`, KeyApproval: `20000000`,
	KeyBudgetJob: `1000000`, KeyBudgetPipeline: `5000000`, KeyBudgetDay: `10000000`, KeyBudgetMonth: `100000000`,
	"economics.budgets.provider.hetzner": `40000000`, "economics.ceilings.github-hosted": `0`, "economics.ceilings.hetzner": `60000000`,
	"economics.rates.hetzner":       `{"class":"ephemeral","rate_per_hour_micro":5000,"billing_increment":"1h","minimum_charge_micro":1000,"extras_micro":200}`,
	"economics.rates.github-hosted": `{"class":"included"}`,
	"economics.providers.hetzner":   `{"owner_coordinator":"coord-1"}`,
	"burst.providers.hetzner":       `{"enabled":true}`,
	KeyBurstConcurrent:              `3`, KeyBurstMargin: `"90s"`,
	KeyCloudType: `"cx22"`, KeyCloudLocation: `"nbg1"`, KeyCloudImage: `"ubuntu-24.04"`,
	"burst.cloudvm.agent.amd64": `{"url":"https://agents.example.test/ci/agent-amd64","sha256":"` + sha64 + `"}`,
	KeyCloudEnrollAddr:          `"0.0.0.0:7443"`, KeyCloudEnrollURL: `"https://ci.example.test/enroll"`, KeyCloudTokenFile: `"/etc/nself/burst.token"`,
}

// resolved merges scopes and lists every concrete key a scope names or a
// registered default gives, as policy.Resolver does.
func resolved(t *testing.T, scopes ...trust.Scope) policy.Resolved {
	t.Helper()
	eff, err := merge(t, scopes...)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	names := map[string]bool{}
	for _, s := range policy.Default.Keys() {
		if !strings.HasSuffix(s.Key, ".*") {
			names[s.Key] = true
		}
	}
	for _, sc := range scopes {
		for k := range sc.Values {
			names[k] = true
		}
	}
	values := map[string]any{}
	for k := range names {
		if v, _, ok := eff.Value(k); ok {
			values[k] = v
		}
	}
	return policy.Resolved{Effective: eff, Values: values}
}

func mustSpend(t *testing.T, scopes ...trust.Scope) SpendPolicy {
	t.Helper()
	res := resolved(t, scopes...)
	p, err := Spend(&res)
	if err != nil {
		t.Fatalf("Spend: %v", err)
	}
	return p
}

func TestSpendKeys(t *testing.T) {
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatalf("init could not register: %v", errs)
	}
	specs := SpendKeys()
	if len(specs) != 24 {
		t.Errorf("%d spend keys, D10-D13 list 24", len(specs))
	}
	seen := map[string]bool{}
	for _, spec := range specs {
		seen[spec.Key] = true
		got, ok := policy.Default.Find(strings.ReplaceAll(spec.Key, "*", "x"))
		if !ok || got.Key != spec.Key || !got.Spend || got.Owner != Owner || got.Decode == nil {
			t.Errorf("%s registered as %+v", spec.Key, got)
		}
	}
	// Registry introspection: nothing under economics.* or burst.* may be a non-Spend key,
	// and no sched.* key may be a Spend key.
	for _, spec := range policy.Default.Keys() {
		spendPrefix := strings.HasPrefix(spec.Key, "economics.") || strings.HasPrefix(spec.Key, "burst.")
		if spendPrefix && (!spec.Spend || !seen[spec.Key]) {
			t.Errorf("%s is under a spend prefix but not a registered Spend key", spec.Key)
		}
		if strings.HasPrefix(spec.Key, "sched.") && spec.Spend {
			t.Errorf("%s is a scheduling key and must not be Spend", spec.Key)
		}
	}
	// Defaults: everything paid is off and nothing is declared.
	p := mustSpend(t)
	if p.PaidEnabled || p.Currency != "USD" || p.MaxPaidTimeoutMs != 7200000 || p.MarginMicro != 0 || p.ApprovalThresholdMicro.Set ||
		p.Budgets.Day.Set || len(p.Rates) != 0 || len(p.Budgets.Provider) != 0 || len(p.Burst.Enabled) != 0 ||
		p.Burst.MaxConcurrent != 2 || p.Burst.MarginMs != 60000 {
		t.Errorf("defaults: %+v", p)
	}
	d := p.Burst.Trust
	if strings.Join(d.Accepts, ",") != "owner,collaborator,internal" || strings.Join(d.Secrets, ",") != "none" || d.Isolation != "ephemeral-vm" || d.PrivacyZone != "provider-allowlist" {
		t.Errorf("burst trust defaults: %+v", d)
	}
	if PaidAllowed(p, "hetzner") {
		t.Error("paid is allowed by default")
	}
	// A full operator policy reads back typed, and the golden pins the shape.
	p = mustSpend(t, scope(trust.SourceProject, operatorSpend))
	if !p.PaidEnabled || p.Currency != "EUR" || p.MaxPaidTimeoutMs != 10800000 || p.MarginMicro != 250000 || p.ApprovalThresholdMicro != (Budget{20000000, true}) {
		t.Errorf("scalars: %+v", p)
	}
	if r := p.Rates["hetzner"]; r != (Rate{Class: "ephemeral", PerHourMicro: 5000, BillingIncrementMs: 3600000, MinimumChargeMicro: 1000, ExtrasMicro: 200}) {
		t.Errorf("rate: %+v", r)
	}
	if !p.Burst.Enabled["hetzner"] || p.Burst.MaxConcurrent != 3 || p.OwnerCoordinator["hetzner"] != "coord-1" || p.Burst.CloudVM.Agent["amd64"].SHA256 != sha64 {
		t.Errorf("burst: %+v", p.Burst)
	}
	if p.CeilingsMicro["github-hosted"] != 0 || p.CeilingsMicro["hetzner"] != 60000000 || p.Budgets.Provider["hetzner"] != 40000000 {
		t.Errorf("ceilings/budgets: %+v", p)
	}
	enc, _ := json.MarshalIndent(p, "", "  ")
	enc = append(enc, '\n')
	golden := "testdata/spend/operator.golden.json"
	if *update {
		if err := os.WriteFile(golden, enc, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if want, err := os.ReadFile(golden); err != nil || string(want) != string(enc) {
		t.Errorf("golden %s differs (run with -update):\nwant %s\ngot  %s (%v)", golden, want, enc, err)
	}
	// A narrower burst.trust applies with only the named field.
	p = mustSpend(t, scope(trust.SourceProject, map[string]string{KeyBurstTrust: `{"accepts":["owner"]}`}))
	if strings.Join(p.Burst.Trust.Accepts, ",") != "owner" || p.Burst.Trust.Isolation != "ephemeral-vm" || strings.Join(p.Burst.Trust.Secrets, ",") != "none" {
		t.Errorf("narrow trust: %+v", p.Burst.Trust)
	}
	// A wider one needs every field named.
	wide := `{"accepts":["owner","untrusted"],"secrets":["project"],"isolation":"vm","privacy_zone":"hosted-allowed"}`
	p = mustSpend(t, scope(trust.SourceProject, map[string]string{KeyBurstTrust: wide}))
	if p.Burst.Trust.Isolation != "vm" || p.Burst.Trust.PrivacyZone != "hosted-allowed" {
		t.Errorf("explicit wide trust: %+v", p.Burst.Trust)
	}
	if _, err := Spend(nil); err == nil {
		t.Error("Spend(nil) accepted")
	}
	res := resolved(t)
	res.Values = nil
	if _, err := Spend(&res); err != nil {
		t.Errorf("an empty value list is the defaults: %v", err)
	}
}

// TestSpendPlanted: a protected-branch commit and a revision (including a
// pipeline or job scope) that set or even lower a spend key change nothing and
// are recorded; the operator value stays.
func TestSpendPlanted(t *testing.T) {
	planted := map[string]string{
		KeyBudgetMonth: `1`, KeyBudgetDay: `999999999`, KeyPaidEnabled: `true`, KeyMargin: `0`, KeyApproval: `1`,
		"economics.rates.hetzner":    `{"class":"ephemeral","rate_per_hour_micro":1,"billing_increment":"1h"}`,
		"burst.providers.hetzner":    `{"enabled":true}`,
		"economics.ceilings.hetzner": `1`,
		KeyCloudTokenFile:            `"/tmp/planted.token"`,
	}
	operator := map[string]string{KeyBudgetMonth: `5000000`, KeyPaidEnabled: `false`, "economics.rates.hetzner": `{"class":"ephemeral","rate_per_hour_micro":9000,"billing_increment":"1h"}`}
	for _, kind := range []trust.Source{trust.SourceProtectedBranch, trust.SourceRevision, trust.SourcePipeline, trust.SourceJob} {
		t.Run(string(kind), func(t *testing.T) {
			res := resolved(t, scope(trust.SourceProject, operator), scope(kind, planted))
			p, err := Spend(&res)
			if err != nil {
				t.Fatal(err)
			}
			if p.Budgets.Month.Micro != 5000000 || p.Budgets.Day.Set || p.PaidEnabled || p.Rates["hetzner"].PerHourMicro != 9000 ||
				p.Burst.Enabled["hetzner"] || p.ApprovalThresholdMicro.Set || p.CeilingsMicro["hetzner"] != 0 || p.Burst.CloudVM.TokenFile != "" {
				t.Errorf("a %s scope changed the spend policy: %+v", kind, p)
			}
			ignored := map[string]bool{}
			for _, ig := range res.Effective.Ignored() {
				if ig.Scope == kind && strings.HasPrefix(ig.Reason, "E671") {
					ignored[ig.Key] = true
				}
			}
			for key := range planted {
				if !ignored[key] {
					t.Errorf("%s from %s: ignored change not recorded: %v", key, kind, res.Effective.Ignored())
				}
			}
			if PaidAllowed(p, "hetzner") {
				t.Errorf("a %s scope enabled paid spend", kind)
			}
		})
	}
	// With no operator value at all, the planted ones still change nothing.
	res := resolved(t, scope(trust.SourceRevision, planted))
	p, err := Spend(&res)
	if err != nil || p.PaidEnabled || p.Budgets.Month.Set || len(p.Rates) != 0 || PaidAllowed(p, "hetzner") {
		t.Errorf("planted revision on empty policy: %+v %v", p, err)
	}
	// An invalid planted value is still refused, not silently skipped.
	if _, err := merge(t, scope(trust.SourceRevision, map[string]string{KeyBudgetMonth: `-5`})); err == nil || !strings.Contains(err.Error(), "E691") {
		t.Errorf("an invalid planted value was accepted: %v", err)
	}
}

func TestPaidAllowed(t *testing.T) {
	for _, tc := range []struct {
		name string
		kv   map[string]string
		want bool
	}{
		{"nothing", nil, false},
		{"paid_enabled alone", map[string]string{KeyPaidEnabled: `true`}, false},
		{"budgets without paid_enabled", map[string]string{KeyBudgetDay: `100`, KeyBudgetMonth: `100`}, false},
		{"only a job budget", map[string]string{KeyPaidEnabled: `true`, KeyBudgetJob: `100`}, false},
		{"only a pipeline budget", map[string]string{KeyPaidEnabled: `true`, KeyBudgetPipeline: `100`}, false},
		{"zero day budget", map[string]string{KeyPaidEnabled: `true`, KeyBudgetDay: `0`}, false},
		{"day budget", map[string]string{KeyPaidEnabled: `true`, KeyBudgetDay: `1`}, true},
		{"month budget", map[string]string{KeyPaidEnabled: `true`, KeyBudgetMonth: `1`}, true},
		{"budget of another provider", map[string]string{KeyPaidEnabled: `true`, "economics.budgets.provider.other": `100`}, false},
		{"budget of this provider", map[string]string{KeyPaidEnabled: `true`, "economics.budgets.provider.hetzner": `100`}, true},
		{"explicitly off", map[string]string{KeyPaidEnabled: `false`, KeyBudgetDay: `100`}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PaidAllowed(mustSpend(t, scope(trust.SourceProject, tc.kv)), "hetzner"); got != tc.want {
				t.Errorf("PaidAllowed = %v want %v", got, tc.want)
			}
		})
	}
	if PaidAllowed(SpendPolicy{}, "") {
		t.Error("the zero policy allows paid")
	}
}

// TestSpendFlagged: every invalid value of the objective is E691 with a JSON
// pointer; the key-aware rules (a GitHub provider is never paid) too.
func TestSpendFlagged(t *testing.T) {
	agent := func(url, sha string) string { return `{"url":"` + url + `","sha256":"` + sha + `"}` }
	rate := func(body string) string { return `{` + body + `}` }
	cases := []struct{ key, raw, pointer string }{
		{KeyBudgetMonth, `-1`, "/economics/budgets/month"},
		{KeyBudgetMonth, `1.5`, "/economics/budgets/month"},
		{KeyBudgetMonth, `"5"`, "/economics/budgets/month"},
		{KeyBudgetMonth, `null`, "/economics/budgets/month"},
		{KeyBudgetMonth, `9223372036854775807`, "/economics/budgets/month"},
		{KeyBudgetDay, `-100`, "/economics/budgets/day"},
		{KeyBudgetJob, `-1`, "/economics/budgets/job"},
		{KeyBudgetProvider, `-1`, "/economics/budgets/provider"},
		{KeyCeilings, `-1`, "/economics/ceilings"},
		{KeyMargin, `-1`, "/economics/margin_micro"},
		{KeyApproval, `0`, "/economics/approval_threshold_micro"},
		{KeyApproval, `-7`, "/economics/approval_threshold_micro"},
		{KeyCurrency, `"usd"`, "/economics/currency"},
		{KeyCurrency, `"US"`, "/economics/currency"},
		{KeyPaidEnabled, `"yes"`, "/economics/paid_enabled"},
		{KeyMaxPaidTimeout, `"0s"`, "/economics/max_paid_timeout"},
		{KeyMaxPaidTimeout, `"200h"`, "/economics/max_paid_timeout"},
		{KeyMaxPaidTimeout, `7200`, "/economics/max_paid_timeout"},
		{KeyRates, rate(`"rate_per_hour_micro":-5,"billing_increment":"1h"`), "/economics/rates/rate_per_hour_micro"},
		{KeyRates, rate(`"rate_per_hour_micro":0,"billing_increment":"1h"`), "/economics/rates"},
		{KeyRates, rate(`"billing_increment":"1h"`), "/economics/rates"},
		{KeyRates, rate(`"class":"ephemeral","rate_per_minute_micro":0,"billing_increment":"1h"`), "/economics/rates"},
		{KeyRates, rate(`"rate_per_hour_micro":100`), "/economics/rates"},
		{KeyRates, rate(`"rate_per_hour_micro":100,"billing_increment":"0s"`), "/economics/rates/billing_increment"},
		{KeyRates, rate(`"rate_per_hour_micro":100,"billing_increment":"-1h"`), "/economics/rates/billing_increment"},
		{KeyRates, rate(`"rate_per_hour_micro":100,"rate_per_minute_micro":2,"billing_increment":"1h"`), "/economics/rates"},
		{KeyRates, rate(`"rate_per_hour_micro":100,"billing_increment":"1h","speed":3`), "/economics/rates/speed"},
		{KeyRates, rate(`"class":"free","rate_per_hour_micro":100,"billing_increment":"1h"`), "/economics/rates/class"},
		{KeyRates, rate(`"rate_per_hour_micro":100,"billing_increment":"1h","extras_micro":-1`), "/economics/rates/extras_micro"},
		{KeyRates, rate(`"rate_per_hour_micro":100,"billing_increment":"1h","minimum_charge_micro":-1`), "/economics/rates/minimum_charge_micro"},
		{KeyRates, rate(`"rate_per_hour_micro":90000000000000000,"billing_increment":"1h"`), "/economics/rates"},
		{KeyRates, rate(`"rate_per_minute_micro":90000000000000000,"billing_increment":"1m"`), "/economics/rates"},
		{KeyRates, `[]`, "/economics/rates"},
		{KeyRates, `7`, "/economics/rates"},
		{KeyEconProviders, `{}`, "/economics/providers/owner_coordinator"},
		{KeyEconProviders, `{"owner_coordinator":"has space"}`, "/economics/providers/owner_coordinator"},
		{KeyBurstProviders, `{}`, "/burst/providers/enabled"},
		{KeyBurstProviders, `true`, "/burst/providers"},
		{KeyBurstProviders, `{"enabled":"yes"}`, "/burst/providers/enabled"},
		{KeyBurstConcurrent, `0`, "/burst/max_concurrent"},
		{KeyBurstConcurrent, `65`, "/burst/max_concurrent"},
		{KeyBurstMargin, `"0s"`, "/burst/margin"},
		{KeyBurstTrust, `{"accepts":["untrusted"]}`, "/burst/trust"},
		{KeyBurstTrust, `{"accepts":["owner","stranger"]}`, "/burst/trust/accepts"},
		{KeyBurstTrust, `{"isolation":"process"}`, "/burst/trust"},
		{KeyBurstTrust, `{"isolation":"fortress"}`, "/burst/trust/isolation"},
		{KeyBurstTrust, `{"privacy_zone":"hosted-allowed"}`, "/burst/trust"},
		{KeyBurstTrust, `{"secrets":["project"]}`, "/burst/trust"},
		{KeyBurstTrust, `{"secrets":["bogus"]}`, "/burst/trust/secrets"},
		{KeyBurstTrust, `{"accepts":[]}`, "/burst/trust/accepts"},
		{KeyBurstTrust, `{"colour":"red"}`, "/burst/trust/colour"},
		{KeyCloudType, `"cx 22"`, "/burst/cloudvm/type"},
		{KeyCloudImage, `""`, "/burst/cloudvm/image"},
		{KeyCloudAgent, agent("http://agents.example.test/a", sha64), "/burst/cloudvm/agent/url"},
		{KeyCloudAgent, agent("https://user:pw@agents.example.test/a", sha64), "/burst/cloudvm/agent/url"},
		{KeyCloudAgent, agent("https://user@agents.example.test/a", sha64), "/burst/cloudvm/agent/url"},
		{KeyCloudAgent, agent("https://agents.example.test/a?token=x", sha64), "/burst/cloudvm/agent/url"},
		{KeyCloudAgent, agent("https://agents.example.test/a?", sha64), "/burst/cloudvm/agent/url"},
		{KeyCloudAgent, agent("https://agents.example.test/a#frag", sha64), "/burst/cloudvm/agent/url"},
		{KeyCloudAgent, agent("https:///a", sha64), "/burst/cloudvm/agent/url"},
		{KeyCloudAgent, agent("https://agents.example.test/a", sha64[:63]), "/burst/cloudvm/agent/sha256"},
		{KeyCloudAgent, agent("https://agents.example.test/a", sha64+"0"), "/burst/cloudvm/agent/sha256"},
		{KeyCloudAgent, agent("https://agents.example.test/a", strings.Repeat("g", 64)), "/burst/cloudvm/agent/sha256"},
		{KeyCloudAgent, `{"url":"https://agents.example.test/a"}`, "/burst/cloudvm/agent/sha256"},
		{KeyCloudEnrollAddr, `"ci.example.test"`, "/burst/cloudvm/enroll_addr"},
		{KeyCloudEnrollAddr, `"ci.example.test:0"`, "/burst/cloudvm/enroll_addr"},
		{KeyCloudEnrollAddr, `"ci.example.test:70000"`, "/burst/cloudvm/enroll_addr"},
		{KeyCloudEnrollAddr, `":8080"`, "/burst/cloudvm/enroll_addr"},
		{KeyCloudEnrollURL, `"http://ci.example.test/enroll"`, "/burst/cloudvm/enroll_url"},
		{KeyCloudEnrollURL, `"https://u:p@ci.example.test/enroll"`, "/burst/cloudvm/enroll_url"},
		{KeyCloudTokenFile, `"burst.token"`, "/burst/cloudvm/token_file"},
		{KeyCloudTokenFile, `"./burst.token"`, "/burst/cloudvm/token_file"},
		{KeyCloudTokenFile, `"~/burst.token"`, "/burst/cloudvm/token_file"},
		{KeyCloudTokenFile, `"/etc/../tmp/burst.token"`, "/burst/cloudvm/token_file"},
		{KeyCloudTokenFile, `""`, "/burst/cloudvm/token_file"},
	}
	byKey := map[string]policy.KeySpec{}
	for _, s := range SpendKeys() {
		byKey[s.Key] = s
	}
	for _, tc := range cases {
		_, err := byKey[tc.key].Decode(json.RawMessage(tc.raw))
		if err == nil || !strings.HasPrefix(err.Error(), "E691 "+tc.pointer) {
			t.Errorf("%s %s: %v (want E691 %s)", tc.key, tc.raw, err, tc.pointer)
		}
	}
	// The loader refuses the same values at merge time, prefixed with the concrete key.
	if _, err := merge(t, scope(trust.SourceProject, map[string]string{"economics.rates.hetzner": rate(`"rate_per_hour_micro":-1,"billing_increment":"1h"`)})); err == nil || !strings.Contains(err.Error(), "E691") {
		t.Errorf("the loader accepted a negative rate: %v", err)
	}
	// Key-aware rules: the provider id, which Decode cannot see.
	paid := rate(`"rate_per_hour_micro":100,"billing_increment":"1h"`)
	keyed := []struct {
		key, raw string
		ok       bool
	}{
		{"economics.rates.github-hosted", paid, false},
		{"economics.rates.github-hosted", rate(`"class":"metered","rate_per_hour_micro":100,"billing_increment":"1h"`), false},
		{"economics.rates.github-jit", rate(`"class":"ephemeral","rate_per_hour_micro":100,"billing_increment":"1h"`), false},
		{"economics.rates.github-hosted", rate(`"class":"included"`), true},
		{"economics.rates.hetzner", paid, true},
		{"economics.rates.Hetzner", paid, false},
		{"economics.rates.-x", paid, false},
		{"burst.providers.github-hosted", `{"enabled":true}`, false},
		{"burst.providers.hetzner", `{"enabled":true}`, true},
		{"burst.cloudvm.agent.riscv", agent("https://agents.example.test/a", sha64), false},
		{"burst.cloudvm.agent.arm64", agent("https://agents.example.test/a", strings.ToUpper(sha64)), true},
		{"economics.budgets.month", `5`, true},
		{"economics.budgets.month", `-5`, false},
		{"sched.preset", `"balanced"`, false},
		{"economics.nothing", `1`, false},
	}
	for _, tc := range keyed {
		err := ValidateSpendKey(tc.key, json.RawMessage(tc.raw))
		if (err == nil) != tc.ok || err != nil && !strings.HasPrefix(err.Error(), "E691 ") {
			t.Errorf("ValidateSpendKey(%s, %s) = %v want ok=%v", tc.key, tc.raw, err, tc.ok)
		}
	}
	// A stored value that breaks a key-aware rule makes Spend fail closed.
	bad := resolved(t, scope(trust.SourceProject, map[string]string{"economics.rates.github-hosted": paid}))
	if _, err := Spend(&bad); err == nil || !strings.HasPrefix(err.Error(), "E691 /economics/rates/github-hosted") {
		t.Errorf("Spend accepted a metered GitHub provider: %v", err)
	}
	// Spend names a missing registered key instead of reading it as zero.
	empty := policy.Resolved{}
	if _, err := Spend(&empty); err == nil || !strings.Contains(err.Error(), "E691") {
		t.Errorf("Spend over an empty effective policy: %v", err)
	}
}

// TestSpendDecodeValues pins the normalized form of valid values.
func TestSpendDecodeValues(t *testing.T) {
	byKey := map[string]policy.KeySpec{}
	for _, s := range SpendKeys() {
		byKey[s.Key] = s
	}
	for _, tc := range []struct {
		key, raw string
		want     any
	}{
		{KeyBudgetDay, `0`, int64(0)},
		{KeyBudgetDay, `12345`, int64(12345)},
		{KeyMaxPaidTimeout, `"90m"`, int64(5400000)},
		{KeyCurrency, `"EUR"`, "EUR"},
		{KeyCloudTokenFile, `"/run/secrets/burst.token"`, "/run/secrets/burst.token"},
		{KeyCloudEnrollAddr, `"[::1]:7443"`, "[::1]:7443"},
		{KeyCloudEnrollURL, `"https://ci.example.test:8443/enroll"`, "https://ci.example.test:8443/enroll"},
	} {
		got, err := byKey[tc.key].Decode(json.RawMessage(tc.raw))
		if err != nil || got != tc.want {
			t.Errorf("%s %s = %#v, %v want %#v", tc.key, tc.raw, got, err, tc.want)
		}
	}
	got, err := byKey[KeyCloudAgent].Decode(json.RawMessage(`{"url":"https://agents.example.test/a","sha256":"` + strings.ToUpper(sha64) + `"}`))
	if m, _ := got.(map[string]any); err != nil || m["sha256"] != sha64 {
		t.Errorf("agent sha256 not normalized: %v %v", got, err)
	}
	got, err = byKey[KeyRates].Decode(json.RawMessage(`{"rate_per_minute_micro":10,"billing_increment":"1m"}`))
	if m, _ := got.(map[string]any); err != nil || m["class"] != "metered" || m["billing_increment"] != int64(60000) {
		t.Errorf("rate class default: %v %v", got, err)
	}
}
