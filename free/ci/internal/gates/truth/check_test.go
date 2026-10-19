package truth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/gates"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func byID(checks []model.Check) map[string]model.Check {
	m := map[string]model.Check{}
	for _, c := range checks {
		m[c.ID] = c
	}
	return m
}

// TestChecksFailOnePerKind: a planted literal, retired term and dead host each
// fail their own kind's check and leave the others green.
func TestChecksFailOnePerKind(t *testing.T) {
	dir := writeTree(t, map[string]string{"rules.yaml": fixtureRules(), "ok.md": "nothing here\n"})
	clean := Run(dir, []string{"rules.yaml"}, "")
	if len(clean) != 3 {
		t.Fatalf("want one check per rule kind, got %d", len(clean))
	}
	for i, k := range Kinds {
		c := clean[i]
		if c.ID != "truth."+k || c.Result != "pass" || !c.Required || !c.Substantive || c.Kind != "truth" || c.JobID != "truth" {
			t.Errorf("clean %s: %+v", k, c)
		}
	}
	for _, tc := range []struct{ kind, body string }{
		{KindBannedLiteral, "x SEKRET_MARKER y\n"},
		{KindRetiredTerm, "the Old Name product\n"},
		{KindDeadHost, "host " + plantedHost() + "\n"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			d := writeTree(t, map[string]string{"rules.yaml": fixtureRules(), "bad.md": tc.body})
			got := byID(Run(d, []string{"rules.yaml"}, ""))
			for _, k := range Kinds {
				c := got["truth."+k]
				if k == tc.kind {
					if c.Result != "fail" || c.Findings != 1 || c.FailureClass != "code" || !strings.Contains(c.Excerpt, "bad.md:1") {
						t.Errorf("%s: %+v", k, c)
					}
				} else if c.Result != "pass" {
					t.Errorf("%s must stay green: %+v", k, c)
				}
			}
		})
	}
}

// TestInvalidRuleFileIsE644: all three checks error and name the file and key.
func TestInvalidRuleFileIsE644(t *testing.T) {
	dir := writeTree(t, map[string]string{"bad.yaml": "version: 1\nrules:\n  - {id: a, kind: dead-host, message: m}\n"})
	checks := Run(dir, []string{"bad.yaml"}, "")
	if len(checks) != 3 {
		t.Fatalf("%d checks", len(checks))
	}
	for _, c := range checks {
		if c.Result != "error" || !strings.HasPrefix(c.Excerpt, "E644 bad.yaml: rules[0].sha256") {
			t.Errorf("%+v", c)
		}
	}
}

// TestRulesLiveApartFromTheTree: the sweep reads rule files from one tree and
// scans another; a rule file inside the scanned tree is never scanned itself.
func TestRulesLiveApartFromTheTree(t *testing.T) {
	rulesDir := writeTree(t, map[string]string{"org.yaml": fixtureRules()})
	target := writeTree(t, map[string]string{"a.md": "Old Name\n", "org.yaml": "Old Name in a same-named file\n"})
	got := byID(Run(target, []string{"org.yaml"}, rulesDir))
	if c := got["truth.retired-term"]; c.Findings != 2 {
		t.Errorf("a same-named file in another tree is scanned: %+v", c)
	}
	same := writeTree(t, map[string]string{"org.yaml": fixtureRules()})
	if c := byID(Run(same, []string{"org.yaml"}, ""))["truth.retired-term"]; c.Result != "pass" {
		t.Errorf("the rule file must not be scanned in its own tree: %+v", c)
	}
}

// TestRegisterAndImpliedJob: with only `checks.truth.rules` and no jobs, the
// catalogue plans the check as required job `truth` (TRUTH review F17).
func TestRegisterAndImpliedJob(t *testing.T) {
	r := gates.NewRegistry()
	if err := Register(r); err != nil {
		t.Fatal(err)
	}
	if err := Register(r); err == nil {
		t.Error("a second registration must be refused")
	}
	d, ok := r.Get(CheckID)
	if !ok || d.Kind != "truth" || d.ImpliedBy != "checks.truth.rules" || !d.RequiredDefault || !d.Substantive {
		t.Fatalf("def = %+v", d)
	}
	cfg := model.PipelineConfig{Checks: map[string]model.CheckDef{"truth": {Rules: []string{".github/truth.yaml"}}}}
	jobs := r.ImpliedJobs(cfg)
	j, ok := jobs["truth"]
	if !ok || len(jobs) != 1 || len(j.Run) != 1 || j.Run[0] != "truth" || j.Required == nil || !*j.Required {
		t.Fatalf("implied jobs = %+v", jobs)
	}
	if got := r.ImpliedJobs(model.PipelineConfig{}); len(got) != 0 {
		t.Errorf("absent key must plan nothing: %+v", got)
	}
	cfg.Jobs = map[string]model.Job{"mine": {Kind: "static", Run: []string{"truth"}}}
	if got := r.ImpliedJobs(cfg); len(got) != 1 {
		t.Errorf("a job that already runs the check must suppress the implied job: %+v", got)
	}
}

// TestOrgRulesFile: the shipped org rules load, hold only hashes for dead hosts,
// and the hashes are those of the ex-server addresses and staging host name
// (built from parts here so no file of this Ticket holds a literal).
func TestOrgRulesFile(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(root, ".github", "truth", "org.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rs, err := Load(root, []string{".github/truth/org.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	dead := []string{
		strings.Join([]string{"167", "235", "233", "65"}, "."),
		strings.Join([]string{"49", "13", "140", "12"}, "."),
		"staging." + "nself.org",
	}
	have := map[string]bool{}
	kinds := map[string]int{}
	for _, r := range rs.Rules {
		kinds[r.Kind]++
		for _, h := range r.SHA256 {
			have[h] = true
		}
	}
	for _, lit := range dead {
		if !have[HashLiteral(lit)] {
			t.Errorf("no dead-host rule holds the hash of an ex-server literal (%s...)", HashLiteral(lit)[:8])
		}
		if strings.Contains(strings.ToLower(string(raw)), lit) {
			t.Error("org.yaml holds a dead-host literal in clear")
		}
	}
	if kinds[KindDeadHost] < 3 || kinds[KindRetiredTerm] < 4 {
		t.Errorf("rule counts: %v", kinds)
	}
	for _, term := range []string{"Plugins Pro", "ɳTV", "flutter sdk", "ci.verdict"} {
		found := false
		for _, r := range rs.Rules {
			found = found || strings.EqualFold(r.Term, term)
		}
		if !found {
			t.Errorf("retired term %q has no rule", term)
		}
	}
}
