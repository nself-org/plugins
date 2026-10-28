package run

import (
	"context"
	"errors"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"strings"
	"testing"
)

type failingStampedBuilder struct{}

func (failingStampedBuilder) Begin(context.Context, string, Revision, Capture) any { return nil }
func (failingStampedBuilder) Stamp(ev model.Evidence) model.Evidence {
	ev.Runner.Isolation = "container"
	ev.SourceTrust = "collaborator"
	ev.ToolVersions = map[string]string{"adhoc.network_scope": "none"}
	return ev
}
func (failingStampedBuilder) Complete(context.Context, any, EvidenceFacts, model.Evidence) (model.Evidence, error) {
	return model.Evidence{}, errors.New("fixture builder failure")
}
func TestBuilderFailureKeepsStampedBaseEvidence(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.Evidence = failingStampedBuilder{}
	res := r.run(t)
	ev := res.Summary.Doc
	if ev.Runner.Isolation != "container" || ev.SourceTrust != "collaborator" || ev.ToolVersions["adhoc.network_scope"] != "none" {
		t.Fatalf("lost stamped facts: %+v", ev)
	}
	if ev.InputDigests.Config != r.C.ConfigDigest || len(ev.InputDigests.Lockfiles) != 0 || len(ev.InputDigests.Generated) != 0 {
		t.Fatal("failed builder evidence carries input digests")
	}
	if !strings.Contains(strings.Join(res.Summary.Notes, " "), "fixture builder failure") {
		t.Fatal("builder failure note lost")
	}
}
