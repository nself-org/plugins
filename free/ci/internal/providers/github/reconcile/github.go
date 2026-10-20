package reconcile

// Purpose: the GitHub Reconciler (Epic D9): on coordinator start and every 5 minutes, for each allowlisted
// repository, redeliver failed hook deliveries (webhook mode, when the token may list them) or scan the
// heads that have no terminal nself-ci status (poll mode always; webhook mode after a 403/404).
// Inputs:  the Engine (settings, repository list, store, API).
// Outputs: triggers handed to the coordinator's Enqueuer.
// Constraints: implements coordinator.Reconciler and coordinator.Everyer; dormant unless compat.V15();
//          one repository's failure never stops the others, a quota error or an exhausted budget stops
//          the pass (the token is shared), and a pass blocked by an earlier RetryAt makes no request.

import (
	"context"
	"errors"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
)

// Names of the two Reconcilers.
const (
	NameGitHub = "github-reconcile"
	NameDemand = "github-demand-poll"
)

// SweepEvery is the cadence of the head scan and the redelivery.
const SweepEvery = coordinator.DefaultReconcileEvery

// GitHub is the sweep Reconciler of an Engine.
type GitHub struct{ e *Engine }

var (
	_ coordinator.Reconciler = (*GitHub)(nil)
	_ coordinator.Everyer    = (*GitHub)(nil)
)

// GitHub returns the sweep Reconciler of e.
func (e *Engine) GitHub() *GitHub { return &GitHub{e} }

// Name implements coordinator.Reconciler.
func (*GitHub) Name() string { return NameGitHub }

// Every implements coordinator.Everyer.
func (*GitHub) Every() time.Duration { return SweepEvery }

// Reconcile implements coordinator.Reconciler.
func (g *GitHub) Reconcile(ctx context.Context, enq coordinator.Enqueuer) error {
	if !compat.V15() {
		return nil
	}
	e := g.e
	st, err := e.settings(ctx)
	if err != nil {
		return err
	}
	if _, blocked := e.blockedUntil(ctx); blocked { // an earlier quota error: no request before RetryAt
		return nil
	}
	repos, err := e.Repos(ctx)
	if err != nil || len(repos) == 0 {
		return err
	}
	var errs []error
	defer func() { _ = e.flush(ctx) }()
	for _, repo := range repos {
		err := e.sweepRepo(ctx, enq, repo, st)
		var quota *QuotaStop
		switch {
		case err == nil:
		case errors.As(err, &quota):
			return errors.Join(append(errs, err)...) // the token is shared: stop the whole pass
		case errors.Is(err, ErrBudget):
			return errors.Join(errs...) // deferred to a later hour; not a failure
		default:
			errs = append(errs, err)
		}
		if ctx.Err() != nil {
			break
		}
	}
	if len(errs) == 0 {
		e.noteOK(ctx)
	}
	return errors.Join(errs...)
}

// sweepRepo is one repository's pass.
func (e *Engine) sweepRepo(ctx context.Context, enq coordinator.Enqueuer, repo string, st Settings) error {
	s, err := e.session(ctx, kindScan, e.passLimit())
	if err != nil {
		return err
	}
	if st.Ingress == IngressWebhook {
		_, usable, err := e.redeliver(ctx, s, repo, st)
		if err != nil || usable {
			return err
		}
	}
	_, err = e.scan(ctx, s, enq, repo)
	return err
}
