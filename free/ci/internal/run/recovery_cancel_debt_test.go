package run

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

func TestRecoverySettlesLostCancelIntent(t *testing.T) {
	for _, idempotent := range []bool{false, true} {
		t.Run(map[bool]string{false: "once", true: "retryable"}[idempotent], func(t *testing.T) {
			home := t.TempDir()
			ctx := context.Background()
			st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			dead, err := st.Register(ctx, 2147483000, "1", bootID())
			must(err)
			must(st.CreatePipeline(ctx, store.PipelineRow{ID: "p", Project: "x", Status: "running"}))
			must(st.CreateJob(ctx, store.JobRow{ID: "j", PipelineID: "p", Name: "j", Kind: "test", Idempotent: idempotent, InfraMax: 1}))
			must(st.CreateAttempt(ctx, store.AttemptRow{ID: "a", JobID: "j", CoordinatorID: dead.ID, N: 1}, ""))
			storetest.Lease(t, st, "a")
			must(st.Transition(ctx, "a", "leased", "lost", store.TransitionDetail{Epoch: 1}))
			_, epoch, err := st.AttemptStateEpoch(ctx, "a")
			must(err)
			must(st.RequestCancel(ctx, "a", "operator", epoch))
			for i := 0; i < 2; i++ {
				rep := Recover(ctx, st, "", home)
				if len(rep.Errors) != 0 {
					t.Fatalf("recovery errors: %v", rep.Errors)
				}
				state, _, err := st.AttemptStateEpoch(ctx, "a")
				must(err)
				if state != "cancelled" {
					t.Fatalf("attempt state %s", state)
				}
				row, _, err := st.PipelineByID(ctx, "p")
				must(err)
				if row.Status == "running" {
					t.Fatal("run still running")
				}
			}
		})
	}
}
