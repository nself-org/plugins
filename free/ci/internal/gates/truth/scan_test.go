package truth

import (
	"strings"
	"testing"
)

func scanFixture(t *testing.T, files map[string]string, rules string) (Result, RuleSet) {
	t.Helper()
	files["rules.yaml"] = rules
	dir := writeTree(t, files)
	rs, err := Load(dir, []string{"rules.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Scan(dir, rs, rs.Files)
	if err != nil {
		t.Fatal(err)
	}
	return res, rs
}

// TestTruthHashedHost: the rule file holds only sha256 values, yet a planted
// literal in a fixture is found, and a sub-domain or a longer number is not.
func TestTruthHashedHost(t *testing.T) {
	rules := fixtureRules()
	if strings.Contains(rules, plantedHost()) || strings.Contains(rules, plantedName()) {
		t.Fatal("the rule file must not hold the literal")
	}
	res, _ := scanFixture(t, map[string]string{
		"a/deploy.sh":   "ssh root@" + plantedHost() + " uptime\n",
		"b/README.md":   "see https://" + strings.ToUpper(plantedName()) + "/x for details\n",
		"c/clean.txt":   "sub." + plantedName() + " and 1." + plantedHost() + ".9 and v1.2.3.4.5\n",
		"d/version.txt": "version 203.0.113.70\n",
	}, rules)
	got := map[string]string{}
	for _, f := range res.Findings {
		got[f.Path] = f.RuleID
		if f.Hash == "" || f.Kind != KindDeadHost {
			t.Errorf("finding %+v", f)
		}
	}
	if len(got) != 2 || got["a/deploy.sh"] != "dead-ip" || got["b/README.md"] != "dead-name" {
		t.Fatalf("findings = %+v", res.Findings)
	}
	if ex := Excerpt(res.Findings, 5); strings.Contains(ex, plantedHost()) || strings.Contains(strings.ToLower(ex), plantedName()) {
		t.Errorf("the excerpt prints the dead host: %q", ex)
	}
}

func TestBannedLiteralAndRetiredTerm(t *testing.T) {
	res, _ := scanFixture(t, map[string]string{
		"x.md":    "uses SEKRET_MARKER here\nsekret_marker lower case is fine\n",
		"y.md":    "The old name was OLD NAME.\nFolded; bold name; Old Names; the Old-Name.\n\nOld Name\n",
		"z.go":    "// nothing to see\n",
		"bin.dat": "SEKRET_MARKER\x00binary",
	}, fixtureRules())
	type hit struct {
		path string
		line int
		rule string
	}
	var got []hit
	for _, f := range res.Findings {
		got = append(got, hit{f.Path, f.Line, f.RuleID})
	}
	want := []hit{{"x.md", 1, "no-secret-word"}, {"y.md", 1, "retired-oldname"}, {"y.md", 4, "retired-oldname"}}
	if len(got) != len(want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("finding %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if res.Skipped != 1 {
		t.Errorf("the binary file must be skipped and counted: %d", res.Skipped)
	}
}

func TestAllowlistSkipScopeAndSelfExclusion(t *testing.T) {
	rules := fixtureRules() + `skip: ["third_party/**"]
allow:
  - rule: "retired-*"
    paths: ["CHANGELOG*"]
    reason: historic notes keep the old names
`
	res, _ := scanFixture(t, map[string]string{
		"CHANGELOG.md":          "Old Name shipped\n",
		"docs/CHANGELOG.md":     "Old Name shipped\n",
		"docs/guide.md":         "Old Name\n",
		"third_party/lib.md":    "Old Name\n",
		".git/config":           "Old Name\n",
		"node_modules/m/x.md":   "Old Name\n",
		".claude/worktrees/w/x": "Old Name\n",
	}, rules)
	if len(res.Findings) != 1 || res.Findings[0].Path != "docs/guide.md" {
		t.Fatalf("findings = %+v", res.Findings)
	}
	if res.Allowed != 2 {
		t.Errorf("allowed = %d, want 2", res.Allowed)
	}
}

func TestRuleScopePaths(t *testing.T) {
	rules := `version: 1
rules:
  - {id: only-md, kind: banned-literal, literal: BAD, message: m, paths: ["**/*.md"]}
`
	res, _ := scanFixture(t, map[string]string{"a.md": "BAD\n", "b.txt": "BAD\n", "d/c.md": "BAD\n"}, rules)
	if len(res.Findings) != 2 {
		t.Fatalf("%+v", res.Findings)
	}
}
