package readiness

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func merge(t *testing.T, scopes ...trust.Scope) (policy.Effective, error) {
	t.Helper()
	reg, err := trust.NewRegistry(Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", scopes...)
	return eff, err
}

func user(kv map[string]string) trust.Scope {
	vals := map[string]json.RawMessage{}
	for k, v := range kv {
		vals[k] = json.RawMessage(v)
	}
	return trust.Scope{Kind: trust.SourceUser, Values: vals}
}

func TestKeysRegisteredInTheProcessSet(t *testing.T) {
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, key := range []string{KeyMinMemMB, KeyMaxLoadPerCore, KeyMinIdlePct, KeyMinDiskMB, KeyCmd, KeyYieldTo} {
		spec, ok := policy.Default.Find(key)
		if !ok || !spec.Spend {
			t.Errorf("%s: registered=%v operator-only=%v", key, ok, spec.Spend)
		}
	}
}

func TestConfigFromDefaultsAndOperatorValues(t *testing.T) {
	eff, err := merge(t)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ConfigFrom(&eff)
	if err != nil {
		t.Fatal(err)
	}
	if want := DefaultConfig(); c.MinMemMB != want.MinMemMB || c.MaxLoadPerCore != want.MaxLoadPerCore || c.MinIdlePct != want.MinIdlePct || c.MinDiskMB != want.MinDiskMB || len(c.Cmd) != 0 || len(c.YieldTo) != 0 {
		t.Fatalf("defaults %+v", c)
	}

	eff, err = merge(t, user(map[string]string{
		KeyMinMemMB: `4096`, KeyMaxLoadPerCore: `1.5`, KeyMinIdlePct: `35`, KeyMinDiskMB: `0`,
		KeyCmd: `["/usr/local/bin/busy","--quiet"]`, KeyYieldTo: `["zoom.us","obs"]`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c, err = ConfigFrom(&eff); err != nil {
		t.Fatal(err)
	}
	if c.MinMemMB != 4096 || c.MaxLoadPerCore != 1.5 || c.MinIdlePct != 35 || c.MinDiskMB != 0 ||
		strings.Join(c.Cmd, " ") != "/usr/local/bin/busy --quiet" || strings.Join(c.YieldTo, ",") != "zoom.us,obs" {
		t.Fatalf("operator values %+v", c)
	}
	// A string decimal is the same key.
	if eff, err = merge(t, user(map[string]string{KeyMaxLoadPerCore: `"0.75"`})); err != nil {
		t.Fatal(err)
	}
	if c, _ = ConfigFrom(&eff); c.MaxLoadPerCore != 0.75 {
		t.Fatalf("decimal string %+v", c)
	}
	if _, err = ConfigFrom(nil); err == nil {
		t.Fatal("a nil policy is an error, never the defaults")
	}
}

func TestKeyDecodersRefuseBadShapes(t *testing.T) {
	bad := map[string]string{
		KeyMinMemMB:       `"lots"`,
		KeyMinIdlePct:     `101`,
		KeyMinDiskMB:      `-1`,
		KeyMaxLoadPerCore: `"fast"`,
		KeyCmd:            `[]`,
		KeyYieldTo:        `["", "obs"]`,
	}
	for key, raw := range bad {
		if _, err := merge(t, user(map[string]string{key: raw})); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("%s=%s must be refused with E672, got %v", key, raw, err)
		}
	}
	for _, raw := range []string{`1.5`, `0`, `1000`} {
		if _, err := merge(t, user(map[string]string{KeyMaxLoadPerCore: raw})); err != nil {
			t.Errorf("max_load_per_core=%s: %v", raw, err)
		}
	}
	if _, err := merge(t, user(map[string]string{KeyMaxLoadPerCore: `1001`})); err == nil {
		t.Error("a ceiling above 1000 must be refused")
	}
}

// readiness.cmd is a command the host runs, so a repository must never be able to set it.
func TestRepositoryScopesCannotSetReadinessKeys(t *testing.T) {
	for _, kind := range []trust.Source{trust.SourceProtectedBranch, trust.SourceRevision, trust.SourcePipeline, trust.SourceJob, trust.SourceRun} {
		eff, err := merge(t, trust.Scope{Kind: kind, Values: map[string]json.RawMessage{KeyCmd: json.RawMessage(`["/tmp/evil"]`), KeyMinMemMB: json.RawMessage(`1`)}})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		c, err := ConfigFrom(&eff)
		if err != nil || len(c.Cmd) != 0 || c.MinMemMB != DefaultMinMemMB {
			t.Errorf("%s scope changed the readiness config: %+v %v", kind, c, err)
		}
		if len(eff.Ignored()) == 0 {
			t.Errorf("%s scope must be reported as ignored", kind)
		}
	}
}
