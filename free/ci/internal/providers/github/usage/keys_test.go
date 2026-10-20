package usage_test

// Tests of the two policy keys: registered, operator scope only (a repository can never switch the
// import on or choose the account), strict decoding, and the account forms.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/usage"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func merge(t *testing.T, kind trust.Source, kv map[string]string) (trust.Effective, error) {
	t.Helper()
	reg, err := trust.NewRegistry(usage.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	vals := map[string]json.RawMessage{}
	for k, v := range kv {
		vals[k] = json.RawMessage(v)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: kind, Values: vals})
	return eff, err
}

func TestKeysAreOperatorScopeOnly(t *testing.T) {
	if errs := usage.RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, key := range []string{usage.KeyBillingImport, usage.KeyBillingAccount} {
		spec, ok := policy.Default.Find(key)
		if !ok || !spec.Spend || spec.Kind != policy.TightenFixed || spec.Default != nil {
			t.Fatalf("%s: registered=%v spend=%v kind=%v default=%v", key, ok, spec.Spend, spec.Kind, spec.Default)
		}
	}
	kv := map[string]string{usage.KeyBillingImport: `true`, usage.KeyBillingAccount: `"org:acme"`}
	for _, kind := range []trust.Source{trust.SourceRevision, trust.SourceProtectedBranch, trust.SourcePipeline, trust.SourceJob} {
		eff, err := merge(t, kind, kv)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		set, err := usage.SettingsFrom(&eff)
		if err != nil || set.Import || set.Account != "" || len(eff.Ignored()) != 2 {
			t.Fatalf("%s scope changed the import: %+v %v ignored=%v", kind, set, err, eff.Ignored())
		}
	}
	eff, err := merge(t, trust.SourceUser, kv)
	if err != nil {
		t.Fatal(err)
	}
	if set, err := usage.SettingsFrom(&eff); err != nil || !set.Import || set.Account != "org:acme" {
		t.Fatalf("operator scope: %+v %v", set, err)
	}
	if _, err := usage.SettingsFrom(nil); err == nil {
		t.Fatal("a nil policy is an error, never the defaults")
	}
}

func TestKeysDecodeStrictly(t *testing.T) {
	bad := []map[string]string{
		{usage.KeyBillingImport: `"true"`}, {usage.KeyBillingImport: `1`}, {usage.KeyBillingImport: `true false`},
		{usage.KeyBillingAccount: `"acme"`}, {usage.KeyBillingAccount: `"team:acme"`}, {usage.KeyBillingAccount: `"org:"`},
		{usage.KeyBillingAccount: `"org:a/b"`}, {usage.KeyBillingAccount: `"org:` + strings.Repeat("a", 40) + `"`}, {usage.KeyBillingAccount: `5`},
	}
	for _, kv := range bad {
		if _, err := merge(t, trust.SourceUser, kv); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("%v: want E672, got %v", kv, err)
		}
	}
	for _, s := range []string{"org:acme", "user:octo-cat", "user:octo_cat"} {
		if a, err := usage.ParseAccount(s); err != nil || a.String() != s {
			t.Errorf("%s: %+v %v", s, a, err)
		}
	}
}
