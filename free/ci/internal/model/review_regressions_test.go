package model

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func validateChanged(t *testing.T, schemaName string, v map[string]any) error {
	t.Helper()
	schemas, err := Schemas()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return ValidateJSON(schemas[schemaName], b)
}

func TestRequiredSkipAndErrorCannotBeAllowed(t *testing.T) {
	for _, result := range []CheckResult{"skip", "error"} {
		checks := []Check{{Result: "pass", Required: true, Substantive: true}, {Result: result, Required: true, AllowFailure: true}}
		if got := Verdict(checks); got != "fail" {
			t.Errorf("%s: %s", result, got)
		}
	}
}

func TestEvidencePassNeedsSubstantiveCheck(t *testing.T) {
	v := fixture(t, "testdata/valid/evidence.provider.json")
	v["checks"] = []any{}
	v["counts"] = map[string]any{"pass": 0, "fail": 0, "skip": 0, "error": 0, "total": 0}
	v["result"] = "pass"
	if err := validateChanged(t, "ci-evidence.v1.schema.json", v); err == nil {
		t.Fatal("empty PASS accepted")
	}
}

func TestEvidenceAllowedFailureRoundTrip(t *testing.T) {
	v := fixture(t, "testdata/valid/evidence.provider.json")
	checks := v["checks"].([]any)
	checks[0].(map[string]any)["allow_failure"] = true
	checks[0].(map[string]any)["result"] = "fail"
	v["result"] = "pass"
	v["counts"] = map[string]any{"pass": 0, "fail": 1, "skip": 0, "error": 0, "total": 1}
	if err := validateChanged(t, "ci-evidence.v1.schema.json", v); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	var e Evidence
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	if !e.Checks[0].AllowFailure {
		t.Fatal("allow_failure lost")
	}
}

func TestEvidenceExcerptByteLimit(t *testing.T) {
	v := fixture(t, "testdata/valid/evidence.provider.json")
	v["checks"].([]any)[0].(map[string]any)["excerpt"] = strings.Repeat("🧪", 2000)
	if err := validateChanged(t, "ci-evidence.v1.schema.json", v); err == nil {
		t.Fatal("8KiB excerpt accepted")
	}
}

func TestValidateJSONRejectsTrailingDocument(t *testing.T) {
	schemas, err := Schemas()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("testdata/valid/events.queued.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSON(schemas["ci-events.v1.schema.json"], bytes.Join([][]byte{b, b}, []byte("\n"))); err == nil {
		t.Fatal("two documents accepted")
	}
}

func TestConfigTimeoutDuration(t *testing.T) {
	v := fixture(t, "testdata/valid/pipeline-config.requirements.json")
	v["jobs"].(map[string]any)["build"].(map[string]any)["timeout"] = "banana"
	if err := validateChanged(t, "ci-pipeline-config.v1.schema.json", v); err == nil {
		t.Fatal("invalid timeout accepted")
	}
}

func TestRegisterReportsCallSites(t *testing.T) {
	code := Code{ID: "E610", Class: "infra", Summary: "same summary", Fix: "fix"}
	Register(code)
	Register(code)
	defer func() { codeMu.Lock(); codes = codes[:len(codes)-2]; codeMu.Unlock() }()
	joined := strings.Join(CodeProblems(), "\n")
	if !strings.Contains(joined, "review_regressions_test.go") || strings.Contains(joined, "same summary and same summary") {
		t.Fatal(joined)
	}
}
