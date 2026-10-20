package githubtmpl

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const fixtureCommit = "0123456789abcdef0123456789abcdef01234567"

func sourceOptions() Options { return Options{AgentSource: "source", AgentCommit: fixtureCommit} }

func releaseOptions() Options {
	sum := strings.Repeat("ab", 32)
	return Options{AgentSource: "release", ReleaseVersion: "1.5.0", Pins: []ReleasePin{
		{RunnerOS: "Linux", RunnerArch: "X64", URL: "https://example.test/releases/v1.5.0/nself-ci-agent-linux-amd64", SHA256: sum},
		{RunnerOS: "macOS", RunnerArch: "ARM64", URL: "https://example.test/releases/v1.5.0/nself-ci-agent-darwin-arm64", SHA256: sum},
	}}
}

func golden(t *testing.T, name string, o Options) []byte {
	t.Helper()
	got, err := Render(o)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Fatalf("%s drifted; run go test ./templates/github -update and review the diff", name)
	}
	return got
}

// TestTemplateGolden proves the rendered workflow is byte-stable for both agent sources
// (acceptance: `providers github init --stdout` output is byte-stable) and carries the
// invariants the Epic names: only contents: read, no cache, no secrets, no larger runner,
// the generated marker and the template version.
func TestTemplateGolden(t *testing.T) {
	for _, c := range []struct {
		name string
		o    Options
	}{{"dispatch.source.golden.yml", sourceOptions()}, {"dispatch.release.golden.yml", releaseOptions()}} {
		t.Run(c.name, func(t *testing.T) {
			got := string(golden(t, c.name, c.o))
			if again, _ := Render(c.o); string(again) != got {
				t.Fatal("two renders differ")
			}
			if !strings.HasPrefix(got, HeaderMarker+"\n# template: v1\n") {
				t.Fatalf("header: %q", got[:120])
			}
			for _, banned := range []string{"actions/cache", "secrets.", "pull_request_target", "-large", "-xlarge", "write-all"} {
				if strings.Contains(got, banned) {
					t.Errorf("the workflow contains %q", banned)
				}
			}
			if n := strings.Count(got, "permissions:"); n != 1 || !strings.Contains(got, "permissions:\n  contents: read\n") {
				t.Errorf("want exactly one permissions block with contents: read only")
			}
			if VersionOf([]byte(got)) != Current {
				t.Errorf("VersionOf = %d", VersionOf([]byte(got)))
			}
		})
	}
}

// TestChoiceInputs proves runner_label, compat_mode and timeout_minutes are type: choice with exactly
// the allowed options, and that no run: line carries an inputs expression (review F9).
func TestChoiceInputs(t *testing.T) {
	out, err := Render(sourceOptions())
	if err != nil {
		t.Fatal(err)
	}
	doc := string(out)
	names, _ := LabelNames()
	block := func(input string) string {
		i := strings.Index(doc, "\n      "+input+":\n")
		if i < 0 {
			t.Fatalf("input %s missing", input)
		}
		rest := doc[i+1:]
		if j := strings.Index(rest[8:], "\n      "); j >= 0 {
			for _, ln := range strings.Split(rest, "\n")[1:] {
				if strings.HasPrefix(ln, "      ") && !strings.HasPrefix(ln, "        ") {
					return rest[:strings.Index(rest, "\n"+ln)]
				}
				if !strings.HasPrefix(ln, " ") {
					break
				}
			}
		}
		return rest
	}
	opts := func(input string) []string {
		var got []string
		b := block(input)
		if !strings.Contains(b, "type: choice") {
			t.Errorf("%s is not type: choice", input)
		}
		for _, ln := range strings.Split(b, "\n") {
			if v, ok := strings.CutPrefix(ln, "          - "); ok {
				got = append(got, strings.Trim(v, "'"))
			}
		}
		return got
	}
	if got := opts("runner_label"); strings.Join(got, ",") != strings.Join(names, ",") {
		t.Errorf("runner_label options = %v", got)
	}
	if got := opts("compat_mode"); strings.Join(got, ",") != "v1.4,v1.5" {
		t.Errorf("compat_mode options = %v", got)
	}
	if got := opts("timeout_minutes"); strings.Join(got, ",") != "10,30,60,120,360" {
		t.Errorf("timeout_minutes options = %v", got)
	}
	bad := regexp.MustCompile(`run:.*\$\{\{ *inputs\.`)
	inRun := false
	for _, ln := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "run: ") || trimmed == "run: |" {
			inRun = true
		} else if strings.HasPrefix(trimmed, "- name:") || strings.HasPrefix(trimmed, "env:") || strings.HasPrefix(trimmed, "with:") || strings.HasPrefix(trimmed, "uses:") {
			inRun = false
		}
		if bad.MatchString(ln) || inRun && strings.Contains(ln, "${{ inputs.") {
			t.Errorf("an inputs expression in a script: %q", ln)
		}
	}
	for _, free := range []string{"runner_label", "compat_mode", "timeout_minutes"} {
		if strings.Contains(doc, free+":\n        description") && !strings.Contains(block(free), "type: choice") {
			t.Errorf("%s accepts free text", free)
		}
	}
}

func TestDataFiles(t *testing.T) {
	labels, err := Labels()
	if err != nil || len(labels) != 11 {
		t.Fatalf("labels: %d %v", len(labels), err)
	}
	for _, l := range labels {
		if strings.Contains(l.Label, "large") {
			t.Errorf("larger runner label %s", l.Label)
		}
	}
	if m, _ := Multiplier("macos"); m != 10 {
		t.Errorf("macos multiplier = %d", m)
	}
	if m, _ := Multiplier("windows"); m != 2 {
		t.Errorf("windows multiplier = %d", m)
	}
	if m, _ := Multiplier("linux"); m != 1 {
		t.Errorf("linux multiplier = %d", m)
	}
	hosts, _ := RedirectHosts()
	if len(hosts) != 3 || hosts[0] != "api.github.com" {
		t.Errorf("hosts = %v", hosts)
	}
	if _, err := Actions(); err != nil {
		t.Error(err)
	}
}

func TestRenderRefuses(t *testing.T) {
	for name, o := range map[string]Options{
		"no source":       {},
		"short commit":    {AgentSource: "source", AgentCommit: "abc"},
		"no pins":         {AgentSource: "release", ReleaseVersion: "1.5.0"},
		"http pin":        {AgentSource: "release", ReleaseVersion: "1.5.0", Pins: []ReleasePin{{RunnerOS: "Linux", RunnerArch: "X64", URL: "http://x.test/a", SHA256: strings.Repeat("a", 64)}}},
		"quote in url":    {AgentSource: "release", ReleaseVersion: "1.5.0", Pins: []ReleasePin{{RunnerOS: "Linux", RunnerArch: "X64", URL: "https://x.test/a'b", SHA256: strings.Repeat("a", 64)}}},
		"unknown version": {Version: 7, AgentSource: "source", AgentCommit: fixtureCommit},
	} {
		if _, err := Render(o); err == nil {
			t.Errorf("%s: rendered", name)
		}
	}
}
