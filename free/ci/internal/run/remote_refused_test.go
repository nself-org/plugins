package run

// Purpose: a hosted dispatch the provider refused for a credential, permission or configuration
//          reason ends the attempt as a coded failure that needs the operator; a transient provider
//          error keeps the bounded infra retry (P7-HOST-11, Epic D13).
// Inputs:  a leased attempt the dispatcher marked lost, and the error the carrier ended with.
// Outputs: the job outcome and the durable attempt state.
// Constraints: no network; providers.Error values are built directly.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func lostByDispatcher(t *testing.T) (*remoteEnv, *runState, *runDispatch) {
	t.Helper()
	e, rs, rd := lostEnv(t)
	_, epoch, _ := e.st.AttemptStateEpoch(context.Background(), e.id)
	if err := e.st.Transition(context.Background(), e.id, "leased", "lost", store.TransitionDetail{Epoch: epoch, FailureClass: "infra", Reason: "delivery failed"}); err != nil {
		t.Fatal(err)
	}
	return e, rs, rd
}

// TestRemoteAwaitProviderRefusalIsNotRetried: a retry is on offer (infra_max 1, idempotent) and still
// E700, E701 and E702 are not retried: failure class code, the code kept, the attempt left to the operator.
func TestRemoteAwaitProviderRefusalIsNotRetried(t *testing.T) {
	for _, c := range []struct {
		code  string
		class providers.ErrorClass
	}{{"E701", providers.Auth}, {"E700", providers.Config}, {"E702", providers.Config}} {
		e, rs, rd := lostByDispatcher(t)
		cause := errors.Join(errors.New("deliver"), providers.NewError(c.class, c.code, "credential", 401, time.Time{}))
		job := model.Job{Retry: &model.RetryConfig{InfraMax: 1}}
		o := rs.remoteLost(context.Background(), planJob("unit"), job, rd, e.id, cause, JobOutcome{ID: "unit"}, 1, &tail{})
		if o.State != StateFailed || o.FailureClass != "code" || o.Code != c.code || o.parked || o.retryable {
			t.Errorf("%s: outcome %+v", c.code, o)
		}
		if got := e.state(t); got != "needs-operator" {
			t.Errorf("%s: durable %s, want needs-operator (no retry attempt)", c.code, got)
		}
	}
}

// TestRemoteAwaitTransientProviderErrorIsRetried: the control. A transient provider error (E704) and
// a lease race (E708, class config) keep the existing path: infra, retryable, back in the queue.
func TestRemoteAwaitTransientProviderErrorIsRetried(t *testing.T) {
	for _, cause := range []error{
		providers.NewError(providers.Transient, "E704", "server", 503, time.Time{}),
		providers.NewError(providers.Config, "E708", "lease superseded by a newer epoch", 0, time.Time{}),
	} {
		e, rs, rd := lostByDispatcher(t)
		job := model.Job{Retry: &model.RetryConfig{InfraMax: 1}}
		o := rs.remoteLost(context.Background(), planJob("unit"), job, rd, e.id, cause, JobOutcome{ID: "unit"}, 1, &tail{})
		if o.FailureClass != "infra" || !o.parked {
			t.Errorf("%v: outcome %+v, want an infra failure parked for retry", cause, o)
		}
	}
}
