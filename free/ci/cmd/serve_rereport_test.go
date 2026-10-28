package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// D-1320: the stored result of a finished, bound run is queued again; anything the run would not have
// posted is not.
func TestRereportDoneQueuesTheStoredResult(t *testing.T) {
	ctx := context.Background()
	rev := "0123456789abcdef0123456789abcdef01234567"
	for _, c := range []struct {
		name     string
		bind     model.BindingState
		result   model.Result
		pipeline string
		want     report.State
	}{
		{"pass", "bound", "pass", "", report.Success},
		{"fail", "bound", "fail", "", report.Failure},
		{"error", "bound", "error", "", report.Error},
		{"unbound run", "unbound", "pass", "", ""},
		{"partial run", "bound", "pass", "release", ""},
		{"other result", "bound", "cancelled", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			if err = st.CreatePipeline(ctx, store.PipelineRow{ID: "run-1", Revision: rev, Project: "/proj", Trigger: "push", SourceTrust: "trusted", PrivacyZone: "local-only",
				PolicyDigest: "p", InputDigest: "cfg", SelectionMode: "full", Status: "passed"}); err != nil {
				t.Fatal(err)
			}
			ev := model.Evidence{Revision: rev, Binding: model.Binding{State: c.bind}, Result: c.result}
			body, _ := json.Marshal(ev)
			if err = st.RecordRunEvidence(ctx, "ev1", "run-1", rev, "cfg", body); err != nil {
				t.Fatal(err)
			}
			row := store.IntakeRow{Source: "github", Project: "o/r", Revision: rev, PipelineID: "run-1", Pipeline: c.pipeline}
			if err = rereportDone(st)(ctx, row); err != nil {
				t.Logf("delivery error (no remote in the test): %v", err)
			}
			items, err := st.OutboxList(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if c.want == "" {
				if len(items) != 0 {
					t.Fatalf("queued %d status(es), want none", len(items))
				}
				return
			}
			if len(items) != 1 || items[0].Revision != rev || items[0].Context != "nself-ci@o/r" {
				t.Fatalf("outbox = %+v", items)
			}
			var s report.Status
			if err = json.Unmarshal(items[0].Payload, &s); err != nil || s.State != c.want || s.Owner != "o" || s.Repo != "r" {
				t.Fatalf("payload %s (%v), want state %s for o/r", items[0].Payload, err, c.want)
			}
		})
	}
}
