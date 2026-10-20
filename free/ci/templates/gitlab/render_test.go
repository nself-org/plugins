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
	if len(docs) != 1 {
		t.Fatalf("documents = %d, want one (the include file has no spec header)", len(docs))
	}
	job, ok := docs[0][Job].(map[string]any)
	if !ok {
		t.Fatalf("no job %q in %v", Job, docs[1])
	}
	rules, _ := job["rules"].([]any)
	if len(rules) != 1 || !strings.Contains(rules[0].(map[string]any)["if"].(string), VarNonce) {
		t.Errorf("the job must run only for a nonce: %v", job["rules"])
	}
	for _, want := range []string{"#   spec:", "#     NSELF_CI_NONCE: '$[[ inputs.nonce ]]'", "$NSELF_CI_RUNNER_TAG", "@sha256:", "${NSELF_CI_SPEC_9:-}"} {
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

// TestInputsTable proves the root snippet and the input table agree: 18 inputs (inside GitLab's limit of
// 20 per pipeline), every spec chunk fits the 1 KB limit of an input string, the chunks together hold
// the 10,000 characters of a variable, the snippet parses, and its variables are the ones the job reads.
func TestInputsTable(t *testing.T) {
	in := Inputs()
	if len(in) != SpecChunks+8 || len(in) > 20 || SpecChunks*ChunkChars < 10000 || ChunkChars >= 1024 {
		t.Fatalf("inputs %d chunks %dx%d", len(in), SpecChunks, ChunkChars)
	}
	dec := yaml.NewDecoder(strings.NewReader(RootSnippet()))
	var docs []map[string]any
	for {
		var d map[string]any
		if err := dec.Decode(&d); err != nil {
			break
		}
		docs = append(docs, d)
	}
	if len(docs) != 2 {
		t.Fatalf("snippet documents = %d", len(docs))
	}
	declared := docs[0]["spec"].(map[string]any)["inputs"].(map[string]any)
	vars := docs[1]["variables"].(map[string]any)
	out, _ := Render(Options{AgentCommit: fixtureCommit})
	for _, i := range in {
		if _, ok := declared[i.Name]; !ok || vars[i.Var] != "$[[ inputs."+i.Name+" ]]" {
			t.Errorf("input %s is not declared and mapped to %s", i.Name, i.Var)
		}
		if i.Var != VarSpec+"_"+strings.TrimPrefix(i.Name, "spec_") && !strings.Contains(string(out), i.Var) {
			t.Errorf("the job never reads %s", i.Var)
		}
	}
}

// TestRenderRoot proves the two generated roots of the untrusted ref: each is deterministic, starts with
// the GENERATED marker, parses as YAML, includes the generated file (the inputs root also declares and
// maps every input), RootKindOf names each one and nothing else, and a single changed byte is no root.
func TestRenderRoot(t *testing.T) {
	v, in := RenderRoot(RootVariables), RenderRoot(RootInputs)
	if string(RenderRoot(RootVariables)) != string(v) || string(RenderRoot(RootInputs)) != string(in) || string(v) == string(in) {
		t.Fatal("roots are not deterministic or not distinct")
	}
	for kind, doc := range map[string][]byte{RootVariables: v, RootInputs: in} {
		if !strings.HasPrefix(string(doc), HeaderMarker+"\n# root: "+kind+"\n") || RootKindOf(doc) != kind {
			t.Errorf("%s: header or kind: %q", kind, strings.SplitN(string(doc), "\n", 3)[1])
		}
		dec := yaml.NewDecoder(strings.NewReader(string(doc)))
		var last map[string]any
		for {
			var d map[string]any
			if err := dec.Decode(&d); err != nil {
				break
			}
			last = d
		}
		inc, _ := last["include"].([]any)
		if len(inc) != 1 || inc[0].(map[string]any)["local"] != Path {
			t.Errorf("%s: include = %v", kind, last["include"])
		}
		if kind == RootVariables && len(last) != 1 {
			t.Errorf("the variables root holds more than the include: %v", last)
		}
		if kind == RootInputs && len(last["variables"].(map[string]any)) != len(Inputs()) {
			t.Errorf("the inputs root maps %d variables, want %d", len(last["variables"].(map[string]any)), len(Inputs()))
		}
		for _, bad := range [][]byte{append(append([]byte{}, doc...), '#'), doc[:len(doc)-1], []byte(strings.Replace(string(doc), "include", "inclusion", 1)), nil} {
			if RootKindOf(bad) != "" {
				t.Errorf("%s: a changed file passed as a root", kind)
			}
		}
	}
}
