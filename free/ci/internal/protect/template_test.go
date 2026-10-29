package protect

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// workflowDoc is the part of a workflow file these tests read.
type workflowDoc struct {
	On   map[string]any    `yaml:"on"`
	Perm map[string]string `yaml:"permissions"`
	Env  map[string]string `yaml:"env"`
	Jobs map[string]struct {
		Name     string `yaml:"name"`
		RunsOn   string `yaml:"runs-on"`
		Strategy any    `yaml:"strategy"`
		Steps    []struct {
			Uses string `yaml:"uses"`
			Run  string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func readWorkflow(t *testing.T, rel string) (workflowDoc, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	var d workflowDoc
	if err := yaml.Unmarshal(raw, &d); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	return d, string(raw)
}

var shaPin = regexp.MustCompile(`@[0-9a-f]{40}( |$)`)

// TestWorkflowContract: Epic D18 for the template and the two plugins workflows.
func TestWorkflowContract(t *testing.T) {
	cases := []struct {
		file string
		jobs map[string]string // job id -> required job name
		pin  bool              // the pin values are filled in
	}{
		{"templates/nself-ci.yml", map[string]string{"nself-ci": "nself-ci"}, false},
		{"../../.github/workflows/nself-ci.yml", map[string]string{"nself-ci": "nself-ci"}, true},
		{"../../.github/workflows/nself-ci-source.yml", map[string]string{"nself-ci-source": "nself-ci/source", "mutation": "nself-ci/mutation"}, true},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			d, raw := readWorkflow(t, c.file)
			for id, name := range c.jobs {
				j, ok := d.Jobs[id]
				if !ok || j.Name != name {
					t.Fatalf("job %s must be named exactly %q, got %+v", id, name, j)
				}
				if j.Strategy != nil {
					t.Fatalf("job %s has a matrix/strategy: a required check must not", id)
				}
				for _, s := range j.Steps {
					if s.Uses != "" && !shaPin.MatchString(s.Uses+" ") {
						t.Fatalf("job %s: action %q is not pinned by commit sha", id, s.Uses)
					}
				}
			}
			if regexp.MustCompile(`(?m)^\s*(paths|paths-ignore):`).MatchString(raw) || strings.Contains(raw, "cancel-in-progress: true") {
				t.Fatal("a required gate has no paths filter and is never cancelled in progress")
			}
			if d.Perm["contents"] != "read" || len(d.Perm) != 1 {
				t.Fatalf("permissions = %v, want contents: read only", d.Perm)
			}
			if d.Env["NSELF_V15"] != "1" || !strings.Contains(raw, "--no-status") {
				t.Fatal("the workflow runs the plugin directly: NSELF_V15=1 and --no-status are required")
			}
			if filled := d.Env["CI_RELEASE_SHA256"] != ""; filled != c.pin {
				t.Fatalf("pin filled = %t, want %t", filled, c.pin)
			}
			if c.pin && !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(d.Env["CI_RELEASE_SHA256"]) {
				t.Fatalf("CI_RELEASE_SHA256 = %q", d.Env["CI_RELEASE_SHA256"])
			}
			if !c.pin && regexp.MustCompile(`nself-org|acamarata`).MatchString(raw) {
				t.Fatal("the template names no organisation")
			}
		})
	}
}

// TestWorkflowPinsAgree: the two plugins workflows pin the same release.
func TestWorkflowPinsAgree(t *testing.T) {
	a, _ := readWorkflow(t, "../../.github/workflows/nself-ci.yml")
	b, _ := readWorkflow(t, "../../.github/workflows/nself-ci-source.yml")
	for _, k := range []string{"CI_VERSION", "CI_RELEASE_URL", "CI_RELEASE_SHA256"} {
		if a.Env[k] == "" || a.Env[k] != b.Env[k] {
			t.Fatalf("%s differs: %q vs %q", k, a.Env[k], b.Env[k])
		}
	}
	if !strings.Contains(a.Env["CI_RELEASE_URL"], "/v"+a.Env["CI_VERSION"]+"/") {
		t.Fatalf("URL %q does not match version %q", a.Env["CI_RELEASE_URL"], a.Env["CI_VERSION"])
	}
}

// TestMutationPatchesApplyShape: every mutation patch touches only the result rule file.
func TestMutationPatchesApplyShape(t *testing.T) {
	patches, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "mutation-self-gate", "*.patch"))
	if len(patches) == 0 {
		t.Fatal("no mutation patch")
	}
	for _, p := range patches {
		b, _ := os.ReadFile(p)
		if !strings.Contains(string(b), "+++ b/free/ci/internal/model/result.go") {
			t.Fatalf("%s does not mutate the result rule", p)
		}
	}
}

// A checkout-built engine cannot infer operator provenance from a fetched PR.
// Its override is scoped to disposable hosted runners, without secrets or writes,
// and only a head in this very repository gets it: a fork head stays untrusted.
func TestHostedSourceWorkflowTrust(t *testing.T) {
	for _, tc := range []struct {
		file, job, command, runner string
		calls                      int
	}{
{"../../.github/workflows/nself-ci-source.yml", "nself-ci-source", `"$NSELF_CI_BIN"`, "ubuntu-latest", 1},
		{"../../.github/workflows/ci-scenarios.yml", "scenarios", ".drill-bin/nself-ci", "${{ matrix.os }}", 2},
} {
		t.Run(tc.job, func(t *testing.T) {
			d, raw := readWorkflow(t, tc.file)
			job, ok := d.Jobs[tc.job]
			if !ok || job.RunsOn != tc.runner {
				t.Fatalf("unexpected runner: %+v", job)
			}
			if len(d.Perm) != 1 || d.Perm["contents"] != "read" || strings.Contains(strings.ReplaceAll(raw, " ", ""), "${{secrets.") || strings.Contains(raw, "self-hosted") {
				t.Fatal("operator override requires disposable hosted execution, no secrets and a read-only token")
			}
			if !strings.Contains(raw, "NSELF_CI_TRUST: ${{ github.event.pull_request.head.repo.full_name == github.repository && 'owner' || '' }}") {
				t.Fatal("the audited override must be set only for a head in this repository")
			}
			calls := 0
			for _, step := range job.Steps {
				for _, line := range strings.Split(step.Run, "\n") {
					if !strings.HasPrefix(strings.TrimSpace(line), tc.command+" ") {
						continue
					}
					calls++
				}
				if strings.Contains(step.Run, tc.command+" ") && !strings.Contains(step.Run, `--trust "$NSELF_CI_TRUST"`) {
					t.Fatalf("missing audited operator override: %s", step.Run)
				}
			}
			if calls != tc.calls {
				t.Fatalf("gate commands %d, want %d", calls, tc.calls)
			}
		})
	}
}
