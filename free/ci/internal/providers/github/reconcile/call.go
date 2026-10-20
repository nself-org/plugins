package reconcile

// Purpose: one counted, budgeted and quota-aware GitHub session for a pass over one repository.
// Inputs:  the Engine (client, ledger, backoff) and the kind of spending (scan or demand).
// Outputs: session.get / session.send; ErrCallBound, ErrBudget and *QuotaStop as the reasons a pass stops.
// Constraints: every request goes through the provider REST funnel; a 304 costs nothing against the hourly
//          ledger; a quota error (E703) records RetryAt once and ends the pass; while RetryAt is in the
//          future no request is made at all.

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

var (
	errNoStore = errors.New("reconcile: no store installed")
	// ErrCallBound means the pass reached its request bound for the repository.
	ErrCallBound = errors.New("reconcile: request bound reached for this pass")
	// ErrBudget means the hourly allowance for this kind of spending is used up.
	ErrBudget = errors.New("reconcile: hourly request budget reached")
)

// QuotaStop is returned when GitHub answered with a quota error; RetryAt is the time recorded.
type QuotaStop struct{ RetryAt time.Time }

func (q *QuotaStop) Error() string {
	return "reconcile: quota reached, retry at " + q.RetryAt.Format(time.RFC3339)
}

type session struct {
	e           *Engine
	c           *rest.Client
	kind        string
	limit, used int
}

// session opens a session, or returns *QuotaStop when RetryAt has not passed.
func (e *Engine) session(ctx context.Context, kind string, limit int) (*session, error) {
	if until, blocked := e.blockedUntil(ctx); blocked {
		return nil, &QuotaStop{RetryAt: until}
	}
	c, err := e.NewAPI(ctx)
	if err != nil {
		return nil, err
	}
	return &session{e: e, c: c, kind: kind, limit: limit}, nil
}

// do sends one request through the funnel and keeps the books.
func (s *session) do(ctx context.Context, rq rest.Request, out any) (rest.Response, error) {
	if s.used >= s.limit {
		return rest.Response{}, ErrCallBound
	}
	if !s.e.allow(ctx, s.kind) {
		return rest.Response{}, ErrBudget
	}
	s.used++
	resp, err := s.c.DoRequest(ctx, rq, out)
	if resp.Status != 0 && resp.Status != 304 {
		s.e.spend(ctx, s.kind)
	}
	var pe *providers.Error
	if errors.As(err, &pe) && pe.Class == providers.Quota {
		return resp, &QuotaStop{RetryAt: s.e.noteQuota(ctx, pe.RetryAt)}
	}
	return resp, err
}

// get is a GET; a non-empty etag makes it conditional (a 304 returns Status 304 and no error).
func (s *session) get(ctx context.Context, tmpl string, params map[string]string, q url.Values, etag string, out any) (rest.Response, error) {
	return s.do(ctx, rest.Request{Method: "GET", Path: tmpl, Params: params, Query: q, IfNoneMatch: etag}, out)
}

func (s *session) post(ctx context.Context, tmpl string, params map[string]string) (rest.Response, error) {
	return s.do(ctx, rest.Request{Method: "POST", Path: tmpl, Params: params}, nil)
}

// denied reports whether err is GitHub refusing the token for this call (403 not caused by a quota, or 404):
// the cue to fall back from hook deliveries to the head scan.
func denied(err error) bool {
	var pe *providers.Error
	return errors.As(err, &pe) && pe.Class != providers.Quota && (pe.Status == 403 || pe.Status == 404)
}

func repoParams(full string) map[string]string {
	owner, repo, _ := strings.Cut(full, "/")
	return map[string]string{"owner": owner, "repo": repo}
}
