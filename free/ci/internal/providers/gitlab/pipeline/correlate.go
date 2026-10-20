package pipeline

// Purpose: find the pipeline a dispatch started (Epic D11, review F10) and stop the pipeline of a
//          previous attempt number.
// Inputs:  the project, the journal record (nonce, ref, mode) and the time of the dispatch.
// Outputs: the numeric pipeline id, or "not found yet".
// Constraints: a pipeline matches only when its name carries the nonce or, in variables mode, its
//          NSELF_CI_NONCE variable equals it; the match must be exactly one pipeline, else E705
//          run.ambiguous; candidates are the pipelines of the token's own user (the list is filtered by
//          username, so another member can neither flood the search nor plant a twin that blocks the
//          adoption; Collect still checks the user of the pipeline it is given), on the ref, created by
//          the API, updated inside the window of two minutes either side of the dispatch, newest first;
//          more than maxCandidates of them is a retryable E704, never a silent cut; the search repeats every five seconds for up to FindWindow and never resends
//          the create call.

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

const maxCandidates = 100

// echoes reports whether the pipeline carries the nonce: in its name, or (variables mode) as the
// value of its NSELF_CI_NONCE variable.
func (p *Provider) echoes(ctx context.Context, c *rest.Client, project api.Project, pl api.Pipeline, rec Record) (bool, error) {
	if strings.Contains(pl.Name, rec.Nonce) {
		return true, nil
	}
	if rec.Mode != ModeVariables {
		return false, nil
	}
	vars, err := p.variables(ctx, c, project, pl.ID)
	if err != nil {
		return false, err
	}
	return vars[gitlabtmpl.VarNonce] == rec.Nonce, nil
}

// variables reads the variables of a pipeline; a pipeline whose variables are gone has none.
func (p *Provider) variables(ctx context.Context, c *rest.Client, project api.Project, id int64) (map[string]string, error) {
	var list []api.Variable
	if _, err := c.Do(ctx, "GET", api.PipelineVars, project.With("pipeline", intToString(id)), nil, &list); err != nil {
		if api.IsStatus(err, 404) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	out := make(map[string]string, len(list))
	for _, v := range list {
		out[v.Key] = v.Value
	}
	return out, nil
}

// findPipeline lists API pipelines on the record's ref updated in [from, to] and returns the one that
// carries the record's nonce.
func (p *Provider) findPipeline(ctx context.Context, c *rest.Client, project api.Project, rec Record, from, to time.Time) (int64, bool, error) {
	login, err := p.whoami(ctx, c)
	if err != nil {
		return 0, false, err
	}
	q := url.Values{}
	q.Set("ref", rec.Ref)
	q.Set("source", "api")
	q.Set("username", login) // only the token's own pipelines: another member's flood or copy of the nonce cannot hide ours
	q.Set("order_by", "id")
	q.Set("sort", "desc")
	q.Set("updated_after", from.UTC().Format(time.RFC3339))
	q.Set("updated_before", to.UTC().Format(time.RFC3339))
	candidates, err := api.List[api.Pipeline](ctx, c, api.PipelinesPath, project.Params(), q)
	if err != nil {
		return 0, false, err
	}
	if len(candidates) > maxCandidates { // never drop candidates silently: the target could be among them
		return 0, false, api.Unavailable("run.candidates")
	}
	var titled, mine []api.Pipeline
	for _, cand := range candidates {
		var full api.Pipeline
		if _, err = c.Do(ctx, "GET", api.PipelinePath, project.With("pipeline", intToString(cand.ID)), nil, &full); err != nil {
			return 0, false, err
		}
		ok, eerr := p.echoes(ctx, c, project, full, rec)
		if eerr != nil {
			return 0, false, eerr
		}
		if ok {
			titled = append(titled, full)
			if full.User.Username == login {
				mine = append(mine, full)
			}
		}
	}
	switch {
	case len(titled) == 0:
		return 0, false, nil
	case len(titled) != 1 || len(mine) != 1:
		return 0, false, api.NotEvidence(hostedev.ReasonAmbiguous)
	}
	return mine[0].ID, true, nil
}

// awaitPipeline repeats findPipeline over [from, to] until the pipeline shows or the window passes.
func (p *Provider) awaitPipeline(ctx context.Context, c *rest.Client, project api.Project, rec Record, from, to time.Time) (int64, bool, error) {
	var last error
	for i := 0; i <= int(p.cfg.FindWindow/findStep); i++ {
		id, found, err := p.findPipeline(ctx, c, project, rec, from, to)
		switch {
		case found:
			return id, true, nil
		case err != nil && !api.IsTransient(err):
			return 0, false, err
		}
		last = err
		if p.cfg.Wait(ctx, findStep) != nil {
			break
		}
	}
	return 0, false, last
}

// cancelNonce stops the pipeline of an earlier attempt number, if it exists. It is best effort: a
// pipeline that is gone or finished is fine, and a failure leaves one whose result nobody will accept
// (its nonce is no longer the attempt's).
func (p *Provider) cancelNonce(ctx context.Context, c *rest.Client, project api.Project, rec Record, at time.Time) {
	id, found, err := p.findPipeline(ctx, c, project, rec, at.Add(-leaseTTL-window), at.Add(window))
	if err != nil || !found {
		return
	}
	_ = p.cancelPipeline(ctx, c, project, id)
}

func (p *Provider) cancelPipeline(ctx context.Context, c *rest.Client, project api.Project, id int64) error {
	_, err := c.Do(ctx, "POST", api.PipelineCancel, project.With("pipeline", intToString(id)), nil, nil)
	if api.IsStatus(err, 409) || api.IsStatus(err, 404) { // finished or gone: cancelled enough
		return nil
	}
	return err
}
