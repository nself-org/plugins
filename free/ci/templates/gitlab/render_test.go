package gitlabtmpl

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const fixtureCommit = "0123456789abcdef0123456789abcdef01234567"

func TestDataFiles(t *testing.T) {
	tags, err := Tags()
	if err != nil || len(tags) == 0 {
		t.Fatalf("tags: %d %v", len(tags), err)
	}
	for _, tg := range tags {
		if strings.Contains(tg.Tag, "large") {
			t.Errorf("larger runner tag %s", tg.Tag)
		}
	}
	if hosts, err := RedirectHosts(); err != nil || len(hosts) == 0 {
		t.Errorf("hosts = %v %v", hosts, err)
	}
	if commits, err := SourceCommits(); err != nil || len(commits) == 0 {
		t.Errorf("agent pins: %v %v", commits, err)
	}
	if img, err := Image(); err != nil || !strings.Contains(img, "@sha256:") {
		t.Errorf("image: %q %v", img, err)
	}
}

func TestRenderRefuses(t *testing.T) {
	for name, o := range map[string]Options{
		"no commit":       {},
		"short commit":    {AgentCommit: "abc"},
		"unknown version": {Version: 7, AgentCommit: fixtureCommit},
	} {
		if _, err := Render(o); err == nil {
			t.Errorf("%s: rendered", name)
		}
	}
}

// TestRenderShape holds the rendered file to what the provider reads back from it: the marker on the
// first line, the version and the agent commit, valid YAML in both documents, one job named Job that
// only runs for a nonce, and no GitLab interpolation eaten by the Go template.
func TestRenderShape(t *testing.T) {
	out, err := Render(Options{AgentCommit: fixtureCommit})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), HeaderMarker+"\n") || VersionOf(out) != Current || AgentCommitOf(out) != fixtureCommit {
		t.Fatalf("header: %q v=%d commit=%q", strings.SplitN(string(out), "\n", 2)[0], VersionOf(out), AgentCommitOf(out))
	}
	if again, _ := Render(Options{AgentCommit: fixtureCommit}); string(again) != string(out) {
		t.Error("render is not deterministic")
	}
	dec := yaml.NewDecoder(strings.NewReader(string(out)))
	var docs []map[string]any
	for {
		var d map[string]any
		if err := dec.Decode(&d); err != nil {
			break
		}
		docs = append(docs, d)
	}
	if len(docs) != 2 {
		t.Fatalf("documents = %d, want the spec header and the body", len(docs))
	}
	job, ok := docs[1][Job].(map[string]any)
	if !ok {
		t.Fatalf("no job %q in %v", Job, docs[1])
	}
	rules, _ := job["rules"].([]any)
	if len(rules) != 1 || !strings.Contains(rules[0].(map[string]any)["if"].(string), VarNonce) {
		t.Errorf("the job must run only for a nonce: %v", job["rules"])
	}
	for _, want := range []string{"$[[ inputs.nonce ]]", "$NSELF_CI_RUNNER_TAG", "@sha256:"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("rendered file lost %q", want)
		}
	}
	if strings.Contains(string(out), "{{") || strings.Contains(string(out), "<no value>") {
		t.Error("template syntax leaked into the output")
	}
}

// TestGoldenFile keeps testdata/include.golden.yml equal to the rendering; the pipeline package's
// TestTemplateGolden and the init command read the same file. Set UPDATE_GOLDEN=1 to rewrite it.
func TestGoldenFile(t *testing.T) {
	commits, _ := SourceCommits()
	out, err := Render(Options{AgentCommit: commits[len(commits)-1]})
	if err != nil {
		t.Fatal(err)
	}
	const path = "testdata/include.golden.yml"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil || string(want) != string(out) {
		t.Fatalf("golden differs (run UPDATE_GOLDEN=1 go test ./templates/gitlab): %v", err)
	}
}
