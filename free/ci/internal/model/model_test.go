package model

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemasCurrent(t *testing.T) {
	want, err := Schemas()
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 3 {
		t.Fatalf("schemas: %d", len(want))
	}
	for name, data := range want {
		got, err := os.ReadFile(filepath.Join("..", "..", "schemas", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, data) {
			t.Errorf("%s drifted; run go generate ./internal/model/...", name)
		}
	}
}

func TestGoldenFixtures(t *testing.T) {
	paths, err := filepath.Glob("testdata/*/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 12 {
		t.Fatalf("too few golden fixtures: %d", len(paths))
	}
	seen := map[string]int{}
	for _, path := range paths {
		if strings.Contains(path, "/trigger_facts/") {
			continue // Provider facts have their own byte-stable golden test.
		}
		parts := strings.Split(filepath.Base(path), ".")
		if len(parts) < 2 {
			t.Fatalf("bad fixture name: %s", path)
		}
		name := "ci-" + parts[0] + ".v1.schema.json"
		schema, err := os.ReadFile(filepath.Join("..", "..", "schemas", name))
		if err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		validation := ValidateJSON(schema, body)
		valid := strings.Contains(path, "/valid/")
		if valid && validation != nil {
			t.Errorf("%s: %v", path, validation)
		}
		if !valid && (validation == nil || !strings.Contains(validation.Error(), "/")) {
			t.Errorf("%s: expected JSON-pointer error, got %v", path, validation)
		}
		seen[parts[0]]++
		if valid && parts[0] == "pipeline-config" {
			var c PipelineConfig
			if err := json.Unmarshal(body, &c); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(path, "requirements") && (c.Jobs["build"].Requirements == nil || c.Jobs["build"].Requirements.CPU != 4) {
				t.Error("requirements did not decode")
			}
		}
	}
	for _, n := range []string{"pipeline-config", "evidence", "events"} {
		if seen[n] == 0 {
			t.Errorf("missing %s fixture", n)
		}
	}
}

func TestResultRule(t *testing.T) {
	pass := Check{Result: "pass", Required: true, Substantive: true}
	cases := []struct {
		name   string
		checks []Check
		want   Result
	}{
		{"pass", []Check{pass}, "pass"},
		{"required skip", []Check{pass, {Result: "skip", Required: true}}, "fail"},
		{"optional skip", []Check{pass, {Result: "skip"}}, "pass"},
		{"zero substantive", []Check{{Result: "pass", Required: true}}, "fail"},
		{"required error", []Check{pass, {Result: "error", Required: true}}, "fail"},
		{"allow failure", []Check{pass, {Result: "fail", Required: true, AllowFailure: true}}, "pass"},
		{"env excluded required", []Check{pass, {Result: "skip", Required: true, Reason: "when.env_missing"}}, "fail"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Verdict(c.checks); got != c.want {
				t.Fatalf("got %s want %s", got, c.want)
			}
		})
	}
	p := Projection(Evidence{Checks: []Check{pass}})
	if p.Result != "pass" || p.Passed != 1 || p.Total != 1 || len(p.Summary) > 140 {
		t.Fatalf("projection: %+v", p)
	}
	allowed := Projection(Evidence{Checks: []Check{pass, {ID: "optional-lint", Result: "fail", AllowFailure: true}}})
	if allowed.Result != "pass" || !strings.Contains(allowed.Summary, "optional-lint") {
		t.Fatalf("allowed failure: %+v", allowed)
	}
}

func TestCodeProblems(t *testing.T) {
	if got := CodeProblems(); len(got) != 0 {
		t.Fatal(got)
	}
	bad := problems([]registration{{Code{ID: "E600", Class: "infra", Summary: "first", Fix: "fix"}, "first"}, {Code{ID: "E600", Class: "infra", Summary: "second", Fix: "fix"}, "second"}, {Code{ID: "E720", Class: "infra", Summary: "outside", Fix: "fix"}, "outside"}})
	joined := strings.Join(bad, "\n")
	if !strings.Contains(joined, "first and second") || !strings.Contains(joined, "E720") {
		t.Fatalf("problems: %s", joined)
	}
	last := 599
	for _, span := range []string{"CI", "NODE", "TRUST", "SCHED", "HOST", "CACHE"} {
		pair := CodeRanges[span]
		if pair[0] != last+1 {
			t.Errorf("range overlap or gap: %s", span)
		}
		last = pair[1]
	}
	if last != 719 {
		t.Fatal(last)
	}
}
