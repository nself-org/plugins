package adhoc

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

type failedEvidenceBuilder struct{}

func (failedEvidenceBuilder) Begin(context.Context, string, cirun.Revision, cirun.Capture) any {
	return nil
}
func (failedEvidenceBuilder) Complete(context.Context, any, cirun.EvidenceFacts, model.Evidence) (model.Evidence, error) {
	return model.Evidence{}, errors.New("fixture evidence failure")
}

func TestInnerBuilderFailurePersistsAdhocFacts(t *testing.T) {
	r := newHandlerRig(t, Flags{Isolation: "process", Trust: "internal"})
	r.h.Hooks.Evidence = failedEvidenceBuilder{}
	r.opts.Adhoc = []string{"true"}
	if exit, err := r.h.Adhoc(context.Background(), r.opts, r.opts.Adhoc); err != nil || exit != 0 {
		t.Fatalf("exit=%d err=%v", exit, err)
	}
	st, err := cirun.OpenStore(filepath.Join(r.home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	docs, err := st.EvidenceByRevision(context.Background(), fakeSHA, "builtin-defaults")
	if err != nil || len(docs) != 1 {
		t.Fatalf("documents=%d err=%v", len(docs), err)
	}
	var ev model.Evidence
	if err := json.Unmarshal(docs[0].Body, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.SourceTrust != "internal" || ev.Runner.Isolation != "process" || ev.ToolVersions[KeyNetwork] != "lan" || len(ev.Checks) != 1 || ev.Checks[0].Tool != "true" {
		t.Fatalf("lost ad-hoc facts: %+v", ev)
	}
	if len(ev.InputDigests.Lockfiles) != 0 || len(ev.InputDigests.Generated) != 0 {
		t.Fatal("failed builder acquired input digests")
	}
}
