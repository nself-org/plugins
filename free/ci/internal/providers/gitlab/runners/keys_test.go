package runners_test

// Purpose: tests of the providers.gitlab.runner.* keys.
// Inputs:  the key specs and scopes built in the test.
// Outputs: pass or fail.
// Constraints: no network, no environment.

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/webhook"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func scope(kind trust.Source, kv map[string]string) trust.Scope {
	vals := map[string]json.RawMessage{}
	for k, v := range kv {
		vals[k] = json.RawMessage(v)
	}
	return trust.Scope{Kind: kind, Values: vals}
}

func merge(t *testing.T, scopes ...trust.Scope) (trust.Effective, error) {
	t.Helper()
	reg, err := trust.NewRegistry(runners.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", scopes...)
	return eff, err
}

// TestKeysOperatorOnly: both keys are registered, operator-only (Spend, fixed), a repository scope changes
// nothing, an operator value is used, and the demand tag is the Job Hook source's.
func TestKeysOperatorOnly(t *testing.T) {
	if errs := runners.RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	if runners.DemandTag != webhook.DemandTag {
		t.Fatalf("demand tag %q differs from the Job Hook source's %q", runners.DemandTag, webhook.DemandTag)
	}
	for _, spec := range runners.Keys() {
		got, ok := policy.Default.Find(spec.Key)
		if !ok || !got.Spend || got.Kind != policy.TightenFixed {
			t.Errorf("%s: registered=%v operator-only=%v kind=%v", spec.Key, ok, got.Spend, got.Kind)
		}
	}
	for _, kind := range []trust.Source{trust.SourceRevision, trust.SourceProtectedBranch, trust.SourcePipeline, trust.SourceJob} {
		eff, err := merge(t, scope(kind, map[string]string{runners.KeyTags: `["nself","evil"]`, runners.KeyMaxPending: `50`}))
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		s, err := runners.SettingsFrom(&eff)
		if err != nil || !reflect.DeepEqual(s, runners.DefaultSettings()) || len(eff.Ignored()) != 2 {
			t.Fatalf("%s scope changed the settings: %+v %v ignored=%v", kind, s, err, eff.Ignored())
		}
	}
	eff, err := merge(t, scope(trust.SourceUser, map[string]string{runners.KeyTags: `["nself","fast"]`, runners.KeyMaxPending: `5`}))
	if err != nil {
		t.Fatal(err)
	}
	s, err := runners.SettingsFrom(&eff)
	if err != nil || !reflect.DeepEqual(s, runners.Settings{Tags: []string{"nself", "fast"}, MaxPending: 5}) || len(eff.Ignored()) != 0 {
		t.Fatalf("operator values: %+v %v ignored=%v", s, err, eff.Ignored())
	}
	if _, err = runners.SettingsFrom(nil); err == nil {
		t.Fatal("a nil policy is an error, never the defaults")
	}
	runners.UsePolicy(nil)
	if s, err := runners.CurrentSettings(context.Background()); err != nil || !reflect.DeepEqual(s, runners.DefaultSettings()) {
		t.Fatalf("no policy source: %+v %v", s, err)
	}
}

// TestKeyDecodersRefuseBadShapes: a bad value is E672.
func TestKeyDecodersRefuseBadShapes(t *testing.T) {
	for _, bad := range []struct{ key, raw string }{
		{runners.KeyTags, `"nself"`}, {runners.KeyTags, `[]`}, {runners.KeyTags, `null`}, {runners.KeyTags, `["fast"]`},
		{runners.KeyTags, `["nself","nself"]`}, {runners.KeyTags, `["nself","nself-glr-01"]`}, {runners.KeyTags, `["nself","a b"]`},
		{runners.KeyTags, `["nself",1]`}, {runners.KeyTags, `["nself","a","b","c","d","e","f","g","h"]`}, {runners.KeyTags, `["nself"] ["x"]`},
		{runners.KeyMaxPending, `0`}, {runners.KeyMaxPending, `51`}, {runners.KeyMaxPending, `2.5`}, {runners.KeyMaxPending, `"2"`},
		{runners.KeyMaxPending, `null`}, {runners.KeyMaxPending, `[2]`}, {runners.KeyMaxPending, `2 3`},
	} {
		if _, err := merge(t, scope(trust.SourceUser, map[string]string{bad.key: bad.raw})); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("%s=%s must be refused with E672, got %v", bad.key, bad.raw, err)
		}
	}
}
