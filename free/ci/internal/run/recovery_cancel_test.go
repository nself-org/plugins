package run

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

// A lost attempt that got a cancel intent is ended cancelled by recovery, for
// a retryable job and for one that must not run twice, and its run closes.
func TestRecoveryCancelsALostAttemptWithACancelIntent(t *testing.T) {
	for _, idempotent := range []bool{true, false} {
		t.Run("idempotent="+strconv.FormatBool(idempotent), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("NSELF_CI_HOME", home)
			st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			ctx := context.Background()
			dead, err := st.Register(ctx, 2147483000, strconv.Itoa(1), bootID())
			if err != nil {
				t.Fatal(err)
			}
			must := func(e error) {
				t.Helper()
				if e != nil {
					t.Fatal(e)
				}
			}
			must(st.CreatePipeline(ctx, store.PipelineRow{ID: "p1", Project: "x", Status: "running"}))
			must(st.CreateJob(ctx, store.JobRow{ID: "p1.j", PipelineID: "p1", Name: "j", Kind: "test", Idempotent: idempotent, Required: true, InfraMax: 2}))
			must(st.CreateAttempt(ctx, store.AttemptRow{ID: "p1.j.a1", JobID: "p1.j", CoordinatorID: dead.ID, N: 1}, ""))
			must(storetest.Admit(ctx, st, "p1.j.a1"))
			_, epoch, err := st.AttemptStateEpoch(ctx, "p1.j.a1")
			must(err)
			must(st.Transition(ctx, "p1.j.a1", "leased", "lost", store.TransitionDetail{Epoch: epoch, FailureClass: "lost"}))
			must(st.RequestCancel(ctx, "p1.j.a1", "operator", epoch+1)) // the cancel arrives after the attempt was lost
			rep := Recover(ctx, st, "", home)
			if len(rep.Errors) != 0 {
				t.Fatalf("report %+v", rep)
			}
			if state, _, _ := st.AttemptStateEpoch(ctx, "p1.j.a1"); state != "cancelled" {
				t.Fatalf("attempt state %s", state)
			}
			if row, _, _ := st.PipelineByID(ctx, "p1"); row.Status == "running" {
				t.Fatalf("run still running")
			}
		})
	}
}
