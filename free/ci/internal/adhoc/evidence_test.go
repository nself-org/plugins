package adhoc

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

type recordingBuilder struct{ got model.Evidence }

func (r *recordingBuilder) Begin(context.Context, string, cirun.Revision, cirun.Capture) any {
	return "tok"
}
func (r *recordingBuilder) Complete(_ context.Context, begin any, _ cirun.EvidenceFacts, ev model.Evidence) (model.Evidence, error) {
	r.got = ev
	return ev, nil
}

func TestEvidenceStampReachesTheBuilder(t *testing.T) {
	inner := &recordingBuilder{}
	s := evidenceStamp{inner: inner, mode: Mode{Level: "container", Network: "restricted", Trust: "collaborator"}, argv: "go test ./...", image: "img@sha256:abc"}
	base := model.Evidence{Runner: model.RunnerRef{Isolation: "process"}, SourceTrust: "owner", ToolVersions: map[string]string{"git": "2.4"},
		Checks: []model.Check{{ID: "adhoc"}}}
	if s.Begin(context.Background(), "/r", cirun.Revision{}, nil) != "tok" {
		t.Fatal("Begin must pass through")
	}
	out, err := s.Complete(context.Background(), "tok", cirun.EvidenceFacts{}, base)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []model.Evidence{out, inner.got} { // the builder derives per-attempt documents from what it is given
		if ev.Runner.Isolation != "container" || ev.SourceTrust != "collaborator" || ev.Checks[0].Tool != "go test ./..." ||
			ev.ToolVersions[KeyNetwork] != "restricted" || ev.ToolVersions[KeyImage] != "img@sha256:abc" || ev.ToolVersions["git"] != "2.4" {
			t.Fatalf("not stamped: %+v", ev)
		}
	}
	if base.Runner.Isolation != "process" || base.Checks[0].Tool != "" || base.ToolVersions[KeyNetwork] != "" {
		t.Fatal("the base document was edited in place")
	}
}

func TestEvidenceStampWithoutBuilder(t *testing.T) {
	out, err := evidenceStamp{mode: Mode{Level: "process", Network: "lan"}}.Complete(context.Background(), nil, cirun.EvidenceFacts{}, model.Evidence{})
	if err != nil || out.Runner.Isolation != "process" || out.ToolVersions[KeyNetwork] != "lan" {
		t.Fatalf("%+v %v", out, err)
	}
}

// D-1021: the evidence names the level the registry served, not the one asked for.
func TestEvidenceStampsServedLevel(t *testing.T) {
	out, err := evidenceStamp{mode: Mode{Level: "container", Served: "sandboxed-container", Network: "none"}}.
		Complete(context.Background(), nil, cirun.EvidenceFacts{}, model.Evidence{})
	if err != nil || out.Runner.Isolation != "sandboxed-container" {
		t.Fatalf("isolation %q, want sandboxed-container (%v)", out.Runner.Isolation, err)
	}
}
