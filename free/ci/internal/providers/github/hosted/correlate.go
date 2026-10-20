package hosted

// Purpose: find the run a dispatch started (Epic D5, review F10) and stop the run of a previous
//          attempt number.
// Inputs:  the repository, the nonce and the time of the dispatch.
// Outputs: the numeric run id, or "not found yet".
// Constraints: a run matches only when its display title carries the nonce; a listing must hold
//          exactly one such run, started by the token's own login, else E705 run.ambiguous (a run
//          with our nonce by another actor is a planted run, never adopted); the search window is
//          the dispatch time plus or minus two minutes; the search repeats every five seconds for
//          up to FindWindow and never resends the dispatch.

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/hostedev"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

const (
	window   = 2 * time.Minute
	pageSize = 100
	maxPages = 3
)

// findRun lists workflow_dispatch runs created in [from, to] and returns the one run titled with nonce.
func (p *Provider) findRun(ctx context.Context, c *rest.Client, repo repoRef, nonce string, from, to time.Time) (int64, bool, error) {
	login, err := p.whoami(ctx, c)
	if err != nil {
		return 0, false, err
	}
	var runs []runInfo
	for page := 1; page <= maxPages; page++ {
		q := url.Values{}
		q.Set("event", "workflow_dispatch")
		q.Set("created", from.UTC().Format(time.RFC3339)+".."+to.UTC().Format(time.RFC3339))
		q.Set("per_page", strconv.Itoa(pageSize))
		q.Set("page", strconv.Itoa(page))
		var list struct {
			Runs []runInfo `json:"workflow_runs"`
		}
		if _, err = c.DoQuery(ctx, "GET", workflowRuns, repo.params(), q, nil, &list); err != nil {
			return 0, false, err
		}
		runs = append(runs, list.Runs...)
		if len(list.Runs) < pageSize {
			break
		}
	}
	var titled, mine []runInfo
	for _, r := range runs {
		if strings.Contains(r.DisplayTitle, nonce) {
			titled = append(titled, r)
			if r.Actor.Login == login {
				mine = append(mine, r)
			}
		}
	}
	switch {
	case len(titled) == 0:
		return 0, false, nil
	case len(titled) != 1 || len(mine) != 1:
		return 0, false, notEvidence(hostedev.ReasonAmbiguous)
	}
	return mine[0].ID, true, nil
}

// awaitRun repeats findRun around the dispatch time until the run shows or the window passes.
func (p *Provider) awaitRun(ctx context.Context, c *rest.Client, repo repoRef, nonce string, at time.Time) (int64, bool, error) {
	var last error
	for i := 0; i <= int(p.cfg.FindWindow/findStep); i++ {
		id, found, err := p.findRun(ctx, c, repo, nonce, at.Add(-window), at.Add(window))
		switch {
		case found:
			return id, true, nil
		case err != nil && !transient(err):
			return 0, false, err
		}
		last = err
		if p.cfg.Wait(ctx, findStep) != nil {
			break
		}
	}
	return 0, false, last
}

// cancelNonce stops the run of an earlier attempt number, if it exists. It is best effort: a run that
// is gone or finished is fine, and a failure leaves a run whose result nobody will accept (its nonce
// is no longer the attempt's).
func (p *Provider) cancelNonce(ctx context.Context, c *rest.Client, repo repoRef, nonce string, at time.Time) {
	id, found, err := p.findRun(ctx, c, repo, nonce, at.Add(-leaseTTL-window), at.Add(window))
	if err != nil || !found {
		return
	}
	_ = p.cancelRun(ctx, c, repo, id)
}

func (p *Provider) cancelRun(ctx context.Context, c *rest.Client, repo repoRef, id int64) error {
	params := repo.params()
	params["run"] = intToString(id)
	_, err := c.Do(ctx, "POST", cancelPath, params, nil, nil)
	if isStatus(err, 409) || isStatus(err, 404) { // finished or gone: cancelled enough
		return nil
	}
	return err
}
