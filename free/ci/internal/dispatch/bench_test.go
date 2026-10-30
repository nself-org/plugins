package dispatch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// BenchmarkAdmit includes the durable world snapshot, digest, placement and
// admission transaction. Opening the store, queueing and settlement are untimed.
func BenchmarkAdmit(b *testing.B) {
	ctx := context.Background()
	path := filepath.Join(b.TempDir(), "state.db")
	capability, err := planview.LocalCapability(planview.LocalFacts{
		Project: "project", Platform: "linux/amd64", CPUs: 8, MemMB: 8192, Now: time.Now(),
	})
	if err != nil {
		b.Fatal(err)
	}
	deps := world.Deps{Local: &capability,
		Policy: func(context.Context, string) (sched.Policy, error) {
			return sched.Policy{Preset: "balanced"}, nil
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Keep snapshot cardinality fixed rather than growing completed history
		// with b.N. Database creation and deletion are outside the measurement.
		st, err := store.Open(path, store.Options{})
		if err != nil {
			b.Fatal(err)
		}
		deps.Store = st
		id := queueAttempt(b, st, "pipe", "project", "noop")
		job := testJob(id)
		deps.Jobs = []sched.Job{job}
		b.StartTimer()
		w, digest, err := world.Build(ctx, deps, job.Pipeline)
		if err != nil {
			b.Fatal(err)
		}
		placed := sched.Place(w, []sched.Job{job})
		if len(placed.Placements) != 1 || len(w.Runners) != 1 {
			b.Fatal("local fixture did not place exactly one job")
		}
		_, err = Admit(ctx, st, Request{AttemptID: id, Placement: placed.Placements[0],
			Runner: w.Runners[0], Job: job, Local: true, Host: "bench-host",
			CapacityCPU: 8, CapacityMemMB: 8192, WorldDigest: digest})
		if err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		pass(b, st, id)
		if err := st.Close(); err != nil {
			b.Fatal(err)
		}
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
				b.Fatal(err)
			}
		}
		b.StartTimer()
	}
}
