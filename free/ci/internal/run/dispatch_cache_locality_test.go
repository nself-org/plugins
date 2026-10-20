package run

import (
	"github.com/nself-org/plugins/free/ci/internal/cache/locality"
	"github.com/nself-org/plugins/free/ci/internal/world"
	"path/filepath"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestDispatchWorldIncludesCacheLocalityForAttemptKeys(t *testing.T) {
	if err := world.Register(world.Contributor{Name: locality.Name, Field: locality.Field, Fn: locality.Contribute}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a1", "retry.a2"} {
		t.Run(id, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			row := store.PipelineRow{ID: "r1", Project: t.TempDir(), Revision: "rev", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
			if err = st.CreatePipeline(t.Context(), row); err != nil {
				t.Fatal(err)
			}
			if err = st.CreateJob(t.Context(), store.JobRow{ID: "j1", PipelineID: "r1", Name: "build", Kind: "test", InputDigest: "i"}); err != nil {
				t.Fatal(err)
			}
			if err = st.CreateAttempt(t.Context(), store.AttemptRow{ID: id, JobID: "j1", N: 1}, ""); err != nil {
				t.Fatal(err)
			}
			sess := newSession()
			sess.Store = st
			sess.RunID = "r1"
			sess.Capacity = func() ciexec.Capacity {
				return ciexec.Capacity{CPUs: 4, MemoryMB: 8192, FreeMemoryMB: 8192, BatteryPercent: -1}
			}
			sess.bind(&model.Job{Kind: "test"}, "j1")
			pol := sched.Policy{Preset: "balanced"}
			rd := &runDispatch{sess: sess, row: row, pol: &pol}
			w, _, err := rd.world(t.Context(), sched.Pipeline{ID: "r1"})
			if err != nil {
				t.Fatal(err)
			}
			if fact, ok := w.Cache["local"][id]; !ok || fact.Confidence != "unknown" || fact.MissBytes != nil {
				t.Fatalf("missing unknown locality for attempt %q: %+v", id, w.Cache)
			}
		})
	}
}
