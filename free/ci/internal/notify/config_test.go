package notify

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestKeysRegisteredOperatorOnly(t *testing.T) {
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, k := range []string{KeySinks, KeyDigest, KeyQuiet, KeyAllowPrivate} {
		spec, ok := policy.Default.Find(k)
		if !ok || !spec.Spend || spec.Owner != "notify" {
			t.Errorf("%s: registered=%v spend=%v", k, ok, spec.Spend)
		}
	}
	if spec, _ := policy.Default.Find(KeySinks); !spec.Secret {
		t.Error("notify.sinks must be a secret key")
	}
}

func TestDecodeSinks(t *testing.T) {
	v, err := decodeSinks(json.RawMessage(`[{"type":"stdout"},{"type":"stdout"},{"type":"webhook","name":"ops","url":"https://example.org/h/tok","secret":"s"},{"type":"command","argv":["/bin/true","a"]}]`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	var specs []SinkSpec
	if err = json.Unmarshal(b, &specs); err != nil || len(specs) != 4 {
		t.Fatalf("%s %v", b, err)
	}
	if specs[0].Name != "stdout" || specs[1].Name != "stdout-2" || specs[2].Name != "ops" || specs[3].Name != "command" || specs[3].Argv[1] != "a" {
		t.Fatalf("names: %+v", specs)
	}
	for in, why := range map[string]string{
		`{"type":"stdout"}`:                                           "not a list",
		`[{"type":"nope"}]`:                                           "unknown type",
		`[{"type":"stdout","extra":1}]`:                               "unknown field",
		`[{"type":"webhook","url":"http://h.test/x"}]`:                "plain http",
		`[{"type":"command"}]`:                                        "no argv",
		`[{"type":"stdout","name":"a"},{"type":"stdout","name":"a"}]`: "duplicate name",
		`[{"type":"stdout","name":"a b"}]`:                            "space in name",
		`null`:                                                        "null",
	} {
		_, err := decodeSinks(json.RawMessage(in))
		if err == nil || !strings.HasPrefix(err.Error(), "E672 notify.sinks") {
			t.Errorf("%s: want E672 (%s), got %v", why, in, err)
		} else if strings.Contains(err.Error(), "h.test") {
			t.Errorf("%s: the error echoes the url: %v", why, err)
		}
	}
}

func TestDecodeScalars(t *testing.T) {
	if v, err := decodeDigest(json.RawMessage(`"daily"`)); err != nil || v != "daily" {
		t.Fatal(v, err)
	}
	for _, bad := range []string{`"weekly"`, `true`, `1`} {
		if _, err := decodeDigest(json.RawMessage(bad)); err == nil {
			t.Errorf("digest %s must be refused", bad)
		}
	}
	if v, err := decodeQuiet(json.RawMessage(`["failure","digest"]`)); err != nil || len(v.([]string)) != 2 {
		t.Fatal(v, err)
	}
	if _, err := decodeQuiet(json.RawMessage(`["failure","loud"]`)); err == nil {
		t.Fatal("an unknown kind must be refused")
	}
	if v, err := decodeBool(json.RawMessage(`true`)); err != nil || v != true {
		t.Fatal(v, err)
	}
	if _, err := decodeBool(json.RawMessage(`"yes"`)); err == nil {
		t.Fatal("a string is not a bool")
	}
}

func TestConfigFromRejectsNilAndDefaultsToOff(t *testing.T) {
	if _, err := ConfigFrom(nil); err == nil {
		t.Fatal("a nil policy is an error")
	}
	c, err := ConfigFrom(&policy.Effective{})
	if err != nil || len(c.Sinks) != 0 || c.Digest || c.AllowPrivate || len(c.Quiet) != 0 {
		t.Fatalf("defaults: %+v %v", c, err)
	}
}

func merge(t *testing.T, scopes ...trust.Scope) policy.Effective {
	t.Helper()
	reg, err := trust.NewRegistry(Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", scopes...)
	if err != nil {
		t.Fatal(err)
	}
	return eff
}

func scope(kind trust.Source, kv map[string]string) trust.Scope {
	vals := map[string]json.RawMessage{}
	for k, v := range kv {
		vals[k] = json.RawMessage(v)
	}
	return trust.Scope{Kind: kind, Values: vals}
}

func TestConfigFromOperatorPolicy(t *testing.T) {
	op := scope(trust.SourceUser, map[string]string{
		KeySinks:        `[{"type":"webhook","name":"ops","url":"https://example.org/h","secret":"s"},{"type":"command","argv":["/bin/true"]}]`,
		KeyDigest:       `"daily"`,
		KeyQuiet:        `["recovery"]`,
		KeyAllowPrivate: `true`,
	})
	c, err := ConfigFrom(&[]policy.Effective{merge(t, op)}[0])
	if err != nil || len(c.Sinks) != 2 || c.Sinks[0].Name != "ops" || c.Sinks[0].URL != "https://example.org/h" || c.Sinks[1].Argv[0] != "/bin/true" || !c.Digest || !c.Quiet[KindRecovery] || c.Quiet[KindFailure] || !c.AllowPrivate {
		t.Fatalf("config: %+v %v", c, err)
	}
	// a repository branch or revision cannot set any of the keys: they are operator-only (Spend)
	repo := scope(trust.SourceProtectedBranch, map[string]string{KeySinks: `[{"type":"stdout"}]`, KeyDigest: `"daily"`, KeyAllowPrivate: `true`})
	c, err = ConfigFrom(&[]policy.Effective{merge(t, repo)}[0])
	if err != nil || len(c.Sinks) != 0 || c.Digest || c.AllowPrivate {
		t.Fatalf("a repository scope must not reach the notify keys: %+v %v", c, err)
	}
}
