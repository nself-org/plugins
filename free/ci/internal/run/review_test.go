package run

// Purpose: regressions found by the cross-family review of P7-CI-37 (Codex,
// two passes): a bare run still consults the classifier, a failing Publisher
// posts nothing, and a panicking cleanup hook cannot end the coordinator.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

func TestBareRunConsultsClassifier(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.Trust = fakeTrust("untrusted") // no --trust flag at all
	res := r.run(t)
	row, ok, err := r.store(t).PipelineByID(context.Background(), res.RunID)
	if err != nil || !ok || row.SourceTrust != "untrusted" {
		t.Fatalf("source trust %q ok=%v err=%v: a bare run must record the classifier's answer", row.SourceTrust, ok, err)
	}
}

type errPublisher struct{}

func (errPublisher) Publish(context.Context, PublishRequest) (report.Status, error) {
	return report.Status{}, fail(CodeOptions, "profile unavailable")
}

// A Publisher that fails posts nothing: the default projection may carry text
// its publication profile would have filtered.
func TestFailingPublisherPostsNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.Publisher = errPublisher{}
	res := r.run(t)
	if p := r.Rep.all(); len(p) != 0 {
		t.Fatalf("posted %+v", p)
	}
	if !strings.Contains(strings.Join(res.Summary.Notes, "|"), "publisher failed") || res.Exit != 0 {
		t.Fatalf("exit %d notes %v", res.Exit, res.Summary.Notes)
	}
}

type boomCache struct{ fakeCache }

func (*boomCache) After(context.Context, AttemptInfo, Outcome) error { panic("cache after") }

// A cleanup hook that panics ends neither the coordinator nor the run.
func TestPanickingCleanupHookIsContained(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.Cache = &boomCache{}
	res := r.run(t)
	if res.Exit != 0 || outcome(res, "a").State != StatePassed {
		t.Fatalf("exit %d jobs %+v", res.Exit, res.Summary.Jobs)
	}
}

func TestDescriptionKeepsEvidenceDigest(t *testing.T) {
	t.Parallel()
	req := PublishRequest{Run: RunInfo{Result: "pass", EvidenceDigest: strings.Repeat("ab", 32)}}
	req.Projection.Summary = strings.Repeat("x", 300)
	d := defaultStatus(req).Description
	if len([]rune(d)) > report.MaxDescription || !strings.HasSuffix(d, "(evidence abababababab)") {
		t.Fatalf("%q", d)
	}
}
