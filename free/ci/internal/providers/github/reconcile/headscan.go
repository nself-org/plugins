package reconcile

// Purpose: the head scan (Epic D9): enqueue the default and protected branch heads and the open pull
// request heads of a repository that have no terminal nself-ci status, through the same intake path as a
// webhook.
// Inputs:  GET /repos/{o}/{r}, .../branches?protected=true, .../branches/{default}, .../pulls?state=open,
//          .../commits/{sha}/status, all with the coordinator's own token; marks and ETags from the store.
// Outputs: scanResult (heads checked and enqueued); Delivery per head through coordinator.Enqueuer.
// Constraints: at most MaxBranchHeads branch heads and MaxPRs pull request heads per repository; one page
//          of each list; the repository object and the pull list are read conditionally, and the scan runs
//          only when either changed since the last complete scan (a 304 pass costs no primary quota); a
//          head with a terminal nself-ci status (success, failure or error) is never enqueued, and a head
//          is enqueued at most once per mark; ETags are stored only after a scan that finished, so a
//          pass cut short by a bound, a budget or a quota error is repeated, not forgotten.

import (
	"context"
	"errors"
	"net/url"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
)

// StatusContext is the commit status context that marks a head as built.
const StatusContext = "nself-ci"

type scanResult struct{ Checked, Enqueued int }

type ghBranch struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
	Protected bool `json:"protected"`
}

type ghCombined struct {
	Statuses []struct {
		Context string `json:"context"`
		State   string `json:"state"`
	} `json:"statuses"`
}

// terminal reports whether the combined status holds a finished nself-ci result.
func (c ghCombined) terminal() bool {
	for _, s := range c.Statuses {
		if s.Context == StatusContext && (s.State == "success" || s.State == "failure" || s.State == "error") {
			return true
		}
	}
	return false
}

// scan runs the head scan for repo.
func (e *Engine) scan(ctx context.Context, s *session, enq coordinator.Enqueuer, repo string) (scanResult, error) {
	var res scanResult
	params := repoParams(repo)
	oldRepoTag, err := e.etag(ctx, repo, "repo")
	if err != nil {
		return res, err
	}
	oldPullsTag, err := e.etag(ctx, repo, "pulls")
	if err != nil {
		return res, err
	}
	var info repoInfo
	rr, err := s.get(ctx, "/repos/{owner}/{repo}", params, nil, oldRepoTag, &info)
	if err != nil {
		return res, err
	}
	repoChanged := rr.Status != 304
	if repoChanged {
		if err = e.checkIdentity(ctx, repo, info.ID, info.FullName); err != nil {
			return res, err
		}
		if err = e.putFact(ctx, repo, "repoinfo", info); err != nil {
			return res, err
		}
	} else if ok, gerr := e.getFact(ctx, repo, "repoinfo", &info); gerr != nil || !ok { // the ETag survived but the info did not: read it again and keep it
		if rr, err = s.get(ctx, "/repos/{owner}/{repo}", params, nil, "", &info); err != nil {
			return res, err
		}
		repoChanged = true
		if err = e.checkIdentity(ctx, repo, info.ID, info.FullName); err != nil {
			return res, err
		}
		if err = e.putFact(ctx, repo, "repoinfo", info); err != nil {
			return res, err
		}
	}
	var pulls []pullInfo
	pr, err := s.get(ctx, "/repos/{owner}/{repo}/pulls", params, url.Values{"state": {"open"}, "per_page": {"100"}, "sort": {"updated"}, "direction": {"desc"}}, oldPullsTag, &pulls)
	if err != nil {
		return res, err
	}
	pullsChanged := pr.Status != 304
	if !repoChanged && !pullsChanged {
		return res, nil
	}
	var heads []head
	if repoChanged {
		if heads, err = e.branchHeads(ctx, s, params, info); err != nil {
			return res, err
		}
	}
	if pullsChanged {
		for i := range pulls {
			if i >= MaxPRs {
				break
			}
			p := pulls[i]
			if len(p.Head.SHA) == 40 {
				heads = append(heads, head{SHA: p.Head.SHA, PR: &p})
			}
		}
	}
	complete := true
	var firstErr error
	seen := map[string]bool{}
	for _, h := range heads {
		if seen[h.SHA] {
			continue
		}
		seen[h.SHA] = true
		queued, herr := e.sweepHead(ctx, s, enq, repo, info, h)
		res.Checked++
		if queued {
			res.Enqueued++
		}
		if herr == nil {
			continue
		}
		var quota *QuotaStop
		if errors.As(herr, &quota) || errors.Is(herr, ErrCallBound) || errors.Is(herr, ErrBudget) {
			return res, herr
		}
		complete = false // this head is retried on the next pass
		if firstErr == nil {
			firstErr = herr
		}
	}
	if !complete {
		return res, firstErr
	}
	if repoChanged {
		if err = e.setEtag(ctx, repo, "repo", rr.ETag); err != nil {
			return res, err
		}
	}
	if pullsChanged {
		err = e.setEtag(ctx, repo, "pulls", pr.ETag)
	}
	return res, err
}

// branchHeads lists the default branch head and the protected branch heads (at most MaxBranchHeads).
func (e *Engine) branchHeads(ctx context.Context, s *session, params map[string]string, info repoInfo) ([]head, error) {
	var list []ghBranch
	if _, err := s.get(ctx, "/repos/{owner}/{repo}/branches", params, url.Values{"protected": {"true"}, "per_page": {"100"}}, "", &list); err != nil {
		return nil, err
	}
	var out []head
	hasDefault := false
	for _, b := range list {
		if b.Name == info.DefaultBranch {
			hasDefault = true
		}
	}
	if info.DefaultBranch != "" && !hasDefault {
		var b ghBranch
		p := map[string]string{"owner": params["owner"], "repo": params["repo"], "branch": info.DefaultBranch}
		if _, err := s.get(ctx, "/repos/{owner}/{repo}/branches/{branch}", p, nil, "", &b); err != nil {
			return nil, err
		}
		out = append(out, head{SHA: b.Commit.SHA, Branch: info.DefaultBranch, Protected: b.Protected})
	}
	for _, b := range list { // the default branch goes first so the cap can never cut it
		if b.Name == info.DefaultBranch {
			out = append(out, head{SHA: b.Commit.SHA, Branch: b.Name, Protected: true})
		}
	}
	for _, b := range list {
		if b.Name != info.DefaultBranch {
			out = append(out, head{SHA: b.Commit.SHA, Branch: b.Name, Protected: true})
		}
	}
	if len(out) > MaxBranchHeads {
		out = out[:MaxBranchHeads]
	}
	good := out[:0]
	for _, h := range out {
		if len(h.SHA) == 40 {
			good = append(good, h)
		}
	}
	return good, nil
}

// sweepHead enqueues h unless it was handled before or already has a terminal nself-ci status.
func (e *Engine) sweepHead(ctx context.Context, s *session, enq coordinator.Enqueuer, repo string, info repoInfo, h head) (bool, error) {
	if seen, err := e.marked(ctx, "term", repo, h.SHA); err != nil || seen {
		return false, err
	}
	if seen, err := e.markedWithin(ctx, "enq", repo, h.SHA, enqRetry); err != nil || seen {
		return false, err
	}
	var combined ghCombined
	p := repoParams(repo)
	p["sha"] = h.SHA
	if _, err := s.get(ctx, "/repos/{owner}/{repo}/commits/{sha}/status", p, url.Values{"per_page": {"100"}}, "", &combined); err != nil {
		return false, err
	}
	if combined.terminal() {
		return false, e.mark(ctx, "term", repo, h.SHA)
	}
	d, err := delivery(ctx, headFacts(repo, info, h, e.now()))
	if err != nil {
		return false, err
	}
	if _, err = enq.Enqueue(ctx, d); err != nil {
		return false, err
	}
	return true, e.mark(ctx, "enq", repo, h.SHA)
}
