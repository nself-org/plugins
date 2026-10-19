package schedtel

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestTelemetryColdStartIsMeasuredToRunningNotTeardown (D-1068): the cold start
// is admission to the moment the attempt started running; the 700 ms teardown
// after the job ended must not count towards it.
func TestTelemetryColdStartIsMeasuredToRunningNotTeardown(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sink := NewSink(e.st, e.ms)
	run := "run-cold"
	attempt := e.attempt(run, "build")
	sink.Record(ctx, decision(run, attempt, run+".build", "local", dispatch.OutcomeAdmitted))
	step := func(from, to string, epoch int64) {
		t.Helper()
		if err := e.st.Transition(ctx, attempt, model.JobState(from), model.JobState(to), store.TransitionDetail{Epoch: epoch}); err != nil {
			t.Fatal(err)
		}
	}
	e.advance(300 * time.Millisecond) // lease to start
	step("queued", "leased", 0)
	step("leased", "running", 1)
	e.advance(5 * time.Second) // the job's own runtime
	step("running", "finalizing", 2)
	e.advance(700 * time.Millisecond) // teardown
	step("finalizing", "passed", 3)
	finalize(t, run, attempt, 5000)
	ests, err := e.st.TelEstimators(ctx)
	if err != nil || len(ests) != 1 || ests[0].ColdStartMs != 300 {
		t.Fatalf("estimators %+v (%v), want cold start 300 (admission to running)", ests, err)
	}
}
