package main

// Purpose: P7-WIRE-06. The shipped workflow template free/ci/templates/nself-ci.yml must pass a pull
//          request: its gate step carries the audited NSELF_CI_TRUST override that the plugins repo's
//          own instantiated workflow has, and the two files stay equal except for pinned values and the
//          repo's setup steps (scripts/ci/tests/test-nself-ci-template-parity.sh).
// Inputs:  the template and .github/workflows/nself-ci.yml read from the checkout; bash, awk, diff.
// Outputs: test results only.
// Constraints: no command writes the template today (`policy protect` only changes branch protection),
//          so these tests read the shipped file itself rather than a command's output.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const wire06Template = "../templates/nself-ci.yml"
const wire06Parity = "../../../scripts/ci/tests/test-nself-ci-template-parity.sh"
const wire06TrustExpr = "${{ github.event.pull_request.head.repo.full_name == github.repository && 'owner' || '' }}"

type wire06Workflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Name string            `yaml:"name"`
			Env  map[string]string `yaml:"env"`
			Run  string            `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// TestWire06TemplateCarriesTrustStep: the gate step of the template sets NSELF_CI_TRUST only for a head
// in the same repository and passes it as --trust; no gate call runs without the override.
func TestWire06TemplateCarriesTrustStep(t *testing.T) {
	raw, err := os.ReadFile(wire06Template)
	if err != nil {
		t.Fatal(err)
	}
	var d wire06Workflow
	if err := yaml.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	job, ok := d.Jobs["nself-ci"]
	if !ok {
		t.Fatal("the template has no nself-ci job")
	}
	found := false
	for _, s := range job.Steps {
		if !strings.Contains(s.Run, `"$NSELF_CI_BIN"`) {
			continue
		}
		found = true
		if s.Env["NSELF_CI_TRUST"] != wire06TrustExpr {
			t.Fatalf("step %q: NSELF_CI_TRUST = %q, want %q", s.Name, s.Env["NSELF_CI_TRUST"], wire06TrustExpr)
		}
		if !strings.Contains(s.Run, `args+=(--trust "$NSELF_CI_TRUST")`) || !strings.Contains(s.Run, `"$NSELF_CI_BIN" "${args[@]}"`) {
			t.Fatalf("step %q does not pass the override: %s", s.Name, s.Run)
		}
		if strings.Contains(s.Run, `"$NSELF_CI_BIN" --no-status`) {
			t.Fatalf("step %q still calls the gate without the override", s.Name)
		}
	}
	if !found {
		t.Fatal("the template has no gate step")
	}
}

// TestWire06TemplateParity: the template equals the instantiated workflow except for pinned values,
// and the parity check fails closed when the trust step is missing from one side.
func TestWire06TemplateParity(t *testing.T) {
	if out, err := exec.Command("bash", wire06Parity).CombinedOutput(); err != nil {
		t.Fatalf("parity check failed: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(wire06Template)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, l := range strings.Split(string(raw), "\n") {
		if !strings.Contains(l, "NSELF_CI_TRUST") {
			kept = append(kept, l)
		}
	}
	broken := filepath.Join(t.TempDir(), "nself-ci.yml")
	if err := os.WriteFile(broken, []byte(strings.Join(kept, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	inst, _ := filepath.Abs("../../../.github/workflows/nself-ci.yml")
	if out, err := exec.Command("bash", wire06Parity, broken, inst).CombinedOutput(); err == nil {
		t.Fatalf("parity check passed a template without the trust step:\n%s", out)
	}
}
