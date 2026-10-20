package checks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
)

func TestKeysRegistered(t *testing.T) {
	if errs := RegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, s := range Keys() {
		if !s.Secret || !s.Spend || s.Kind != policy.TightenFixed || s.Default != nil {
			t.Errorf("%s: Secret, Spend and Fixed with a nil default expected: %+v", s.Key, s)
		}
	}
	if len(Keys()) != 3 {
		t.Fatal("three keys expected")
	}
}

func TestDecoders(t *testing.T) {
	keys := map[string]func(json.RawMessage) (any, error){KeyAppID: decodeID(KeyAppID), KeyInstallationID: decodeID(KeyInstallationID), KeyKeyEnv: decodeEnvName}
	ok := map[string][][2]string{
		KeyAppID:          {{`12345`, "12345"}, {`"12345"`, "12345"}},
		KeyInstallationID: {{`777`, "777"}},
		KeyKeyEnv:         {{`"NSELF_CI_APP_KEY"`, "NSELF_CI_APP_KEY"}, {`""`, ""}},
	}
	for key, rows := range ok {
		for _, r := range rows {
			if got, err := keys[key](json.RawMessage(r[0])); err != nil || got != r[1] {
				t.Errorf("%s %s: %v %v", key, r[0], got, err)
			}
		}
	}
	bad := map[string][]string{
		KeyAppID:  {`0`, `-1`, `1.5`, `"12a"`, `""`, `true`, `null`, `[1]`, `12 13`, `"0123"`, `99999999999999999999`},
		KeyKeyEnv: {`"` + dashes + `BEGIN RSA PRIVATE KEY` + dashes + `"`, `"has space"`, `1`, `null`, `"1ABC"`},
	}
	for key, rows := range bad {
		for _, v := range rows {
			_, err := keys[key](json.RawMessage(v))
			if err == nil || !strings.HasPrefix(err.Error(), "E672") || strings.Contains(err.Error(), "BEGIN") {
				t.Errorf("%s %s: want a quiet E672, got %v", key, v, err)
			}
		}
	}
}

// TestConfigDisabled: no key set means no App (nothing to attempt); partial is flagged.
func TestConfigDisabled(t *testing.T) {
	if (Config{}).Enabled() || (Config{}).Partial() {
		t.Fatal("empty config is neither enabled nor partial")
	}
	if c := (Config{AppID: "1"}); c.Enabled() || !c.Partial() {
		t.Fatal("one key is partial")
	}
	if !(Config{AppID: "1", InstallationID: "2", KeyEnv: "K"}).Enabled() {
		t.Fatal("all keys enable")
	}
	if _, err := ConfigFrom(nil); err == nil {
		t.Fatal("a nil policy is an error, not 'no App'")
	}
}

func TestCheckNameIsNotTheRequiredCheck(t *testing.T) {
	if CheckName != "nself-ci/gates" {
		t.Fatal(CheckName)
	}
}

func TestErrorCodeRegistered(t *testing.T) {
	for _, p := range model.CodeProblems() {
		if strings.Contains(p, "E816") {
			t.Fatal(p)
		}
	}
	if !strings.HasPrefix(coded("x").Error(), "E816: ") {
		t.Fatal("E816")
	}
}

func TestAnnotationRules(t *testing.T) {
	ev := fixtureEvidence(
		check("a", "fail", func(c *model.Check) {
			c.Excerpt = "x/y.go:1:2: ok one\n  ./x/z.go:9: ok two\n/abs/p.go:1: no\n../p.go:1: no\nhttp://h.example/p:80: no\nnoext:5: no\nx/y.go:0: zero\nx/y.go:1:2: ok one"
		}),
		check("b", "error", func(c *model.Check) { c.Excerpt = "m/n.go:3: infra note" }),
		check("c", "pass", func(c *model.Check) { c.Excerpt = "m/n.go:3: from a passing check" }),
	)
	got := Annotations(ev)
	if len(got) != 3 || got[0].Path != "x/y.go" || got[1].Path != "x/z.go" || got[1].Start != 9 || got[2].Level != "warning" || got[2].Title != "b" {
		t.Fatalf("%+v", got)
	}
}

func TestCellSafety(t *testing.T) {
	if got := cell("a|b\n`c`\x00d"); got != `a\|b 'c'd` {
		t.Fatalf("%q", got)
	}
	if n := len([]rune(cell(strings.Repeat("é", 500)))); n != maxCell {
		t.Fatalf("%d", n)
	}
}

func TestTableTruncates(t *testing.T) {
	var cs []model.Check
	for i := 0; i < 2000; i++ {
		cs = append(cs, check("check-"+strings.Repeat("x", 50)+itoa(int64(i)), "pass", func(c *model.Check) { c.Remediation = strings.Repeat("r", 150) }))
	}
	ev := fixtureEvidence(cs...)
	out := Table(ev, model.Projection(ev))
	if len(out) > MaxText || !strings.Contains(out, "more checks are not shown") {
		t.Fatalf("len %d", len(out))
	}
}
