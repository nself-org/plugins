package hosted_test

// Purpose: the template acceptance tests of P7-HOST-03 that live beside the provider: the golden
// renderings, the choice inputs of the workflow and the drift of the plugins repository's own
// generated workflow.
// Inputs:  templates/github (renderer, goldens), .github/workflows/nself-ci-dispatch.yml.
// Outputs: pass or fail; `go test ./internal/providers/github/hosted -run TestTemplateGolden -update`
//          rewrites the goldens (review the diff).
// Constraints: no network; the goldens use a fixed fixture commit and fixture release pins.

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const fixtureCommit = "0123456789abcdef0123456789abcdef01234567"

func sourceOptions() githubtmpl.Options {
	return githubtmpl.Options{AgentSource: githubtmpl.SourceMode, AgentCommit: fixtureCommit}
}

func releaseOptions() githubtmpl.Options {
	sum := strings.Repeat("ab", 32)
	return githubtmpl.Options{AgentSource: githubtmpl.ReleaseMode, ReleaseVersion: "1.5.0", Pins: []githubtmpl.ReleasePin{
		{RunnerOS: "Linux", RunnerArch: "X64", URL: "https://example.test/releases/v1.5.0/nself-ci-agent-linux-amd64", SHA256: sum},
		{RunnerOS: "macOS", RunnerArch: "ARM64", URL: "https://example.test/releases/v1.5.0/nself-ci-agent-darwin-arm64", SHA256: sum},
	}}
}

// goldenPath is where the renderings are pinned: beside the templates they render.
func goldenPath(name string) string {
	return filepath.Join("..", "..", "..", "..", "templates", "github", "testdata", name)
}

// TestTemplateGolden proves the rendered workflow is byte-stable for both agent sources (acceptance:
// `providers github init --stdout` output is byte-stable, contains permissions: with only
// contents: read, no actions/cache, no secrets, no pull_request_target, no larger runner label, the
// GENERATED marker and the template version).
func TestTemplateGolden(t *testing.T) {
	for _, c := range []struct {
		name string
		o    githubtmpl.Options
	}{{"dispatch.source.golden.yml", sourceOptions()}, {"dispatch.release.golden.yml", releaseOptions()}} {
		t.Run(c.name, func(t *testing.T) {
			got, err := githubtmpl.Render(c.o)
			if err != nil {
				t.Fatal(err)
			}
			if again, _ := githubtmpl.Render(c.o); string(again) != string(got) {
				t.Fatal("two renders differ")
			}
			if *update {
				if err = os.WriteFile(goldenPath(c.name), got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(goldenPath(c.name))
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != string(got) {
				t.Fatalf("%s drifted; run with -update and review the diff", c.name)
			}
			doc := string(got)
			if !strings.HasPrefix(doc, githubtmpl.HeaderMarker+"\n# template: v1\n") || githubtmpl.VersionOf(got) != githubtmpl.Current {
				t.Fatalf("header: %q", doc[:100])
			}
			for _, banned := range []string{"actions/cache", "secrets.", "pull_request_target", "-large", "-xlarge", "write-all"} {
				if strings.Contains(doc, banned) {
					t.Errorf("the workflow contains %q", banned)
				}
			}
			if strings.Count(doc, "permissions:") != 1 || !strings.Contains(doc, "permissions:\n  contents: read\n") {
				t.Error("want exactly one permissions block with contents: read only")
			}
			for _, line := range strings.Split(doc, "\n") {
				if m := regexp.MustCompile(`uses: ([^ ]+)`).FindStringSubmatch(line); m != nil && !regexp.MustCompile(`@[0-9a-f]{40}$`).MatchString(m[1]) {
					t.Errorf("an action is not pinned by a full commit SHA: %s", line)
				}
			}
		})
	}
}

// TestChoiceInputs proves runner_label, compat_mode and timeout_minutes are `type: choice` with
// exactly the allowed options and that no run: script carries an inputs expression (review F9).
func TestChoiceInputs(t *testing.T) {
	out, err := githubtmpl.Render(sourceOptions())
	if err != nil {
		t.Fatal(err)
	}
	doc := string(out)
	names, _ := githubtmpl.LabelNames()
	options := func(input string) []string {
		start := strings.Index(doc, "\n      "+input+":\n")
		if start < 0 {
			t.Fatalf("input %s missing", input)
		}
		block := doc[start+1:]
		var got []string
		sawChoice := false
		for i, ln := range strings.Split(block, "\n") {
			if i > 0 && strings.HasPrefix(ln, "      ") && !strings.HasPrefix(ln, "        ") {
				break
			}
			sawChoice = sawChoice || strings.TrimSpace(ln) == "type: choice"
			if v, ok := strings.CutPrefix(ln, "          - "); ok {
				got = append(got, strings.Trim(v, "'"))
			}
		}
		if !sawChoice {
			t.Errorf("%s is not type: choice", input)
		}
		return got
	}
	for input, want := range map[string][]string{"runner_label": names, "compat_mode": {"v1.4", "v1.5"}, "timeout_minutes": {"10", "30", "60", "120", "360"}} {
		if got := options(input); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s options = %v, want %v", input, got, want)
		}
	}
	inScript := false
	for _, ln := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(trimmed, "run: "):
			inScript = strings.HasSuffix(trimmed, "|")
			if strings.Contains(ln, "${{ inputs.") {
				t.Errorf("an inputs expression in a run line: %q", ln)
			}
			continue
		case strings.HasPrefix(trimmed, "- name:") || strings.HasPrefix(trimmed, "env:") || strings.HasPrefix(trimmed, "with:") || strings.HasPrefix(trimmed, "uses:"):
			inScript = false
		}
		if inScript && strings.Contains(ln, "${{") {
			t.Errorf("an expression inside a run script: %q", ln)
		}
	}
}

// TestCommittedWorkflowDrift proves the plugins repository's own .github/workflows/nself-ci-dispatch.yml
// is exactly what `providers github init --stdout --agent-source source` renders for the commit it
// pins, that the commit is on the allowlist, and that evidence from this very file would be accepted
// by the evidence check's own rendering.
func TestCommittedWorkflowDrift(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", ".github", "workflows", "nself-ci-dispatch.yml"))
	if err != nil {
		t.Fatal(err)
	}
	commit := githubtmpl.AgentCommitOf(committed)
	allowed, err := githubtmpl.SourceCommits()
	if err != nil || commit == "" || !strings.Contains(strings.Join(allowed, " "), commit) {
		t.Fatalf("the committed workflow pins %q; allowed %v (%v)", commit, allowed, err)
	}
	want, err := githubtmpl.Render(githubtmpl.Options{AgentSource: githubtmpl.SourceMode, AgentCommit: commit})
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(committed) {
		t.Fatal("the committed workflow drifted from the template; run: nself ci providers github init --agent-source source")
	}
	if hostedev.MaxArtifactBytes == 0 || githubtmpl.VersionOf(committed) < githubtmpl.Minimum {
		t.Fatal("the committed workflow is below the minimum template version")
	}
}
