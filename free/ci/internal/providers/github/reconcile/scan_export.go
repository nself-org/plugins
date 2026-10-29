package reconcile

// Purpose: the head scan as a library call (P7-AUTO-11): `nself ci serve watch` discovers its candidate
// heads through the same budgeted, quota-aware and conditional GitHub reads as the sweep, without a
// coordinator Enqueuer.
// Inputs:  a repository and ScanOptions (the operator's watch.branches globs, watch.max_branches and the
//          watch.branch_days cut-off); the Engine's client, store and clock.
// Outputs: HeadScan: the default and protected branch heads, the open pull request heads and the selected
//          extra branch heads, each with the trigger facts the sweep would enqueue; NotModified when every
//          conditional read answered 304.
// Constraints: the ETags live under their own keys ("watch:*"), so the watcher and the sweep never consume
//          each other's 304s; a pass that answers 304 on the repository, the pull list and (when extra
//          branches are configured) the branch list makes no further request; ETags are stored only after
//          a complete scan; a quota error is *QuotaStop with the recorded reset time; no status is read and
//          nothing is enqueued here.

import (
	"context"
	"net/url"
	"path"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// ScanOptions selects the extra branch heads of ScanHeads.
type ScanOptions struct {
	Branches    []string  // names or path.Match globs (watch.branches); empty: no extra branches
	MaxBranches int       // at most this many extra heads (watch.max_branches)
	Since       time.Time // only extra heads whose commit is newer (watch.branch_days); zero: any age
}

// Head is one candidate head of a repository.
type Head struct {
	SHA       string
	Branch    string // the branch of a branch head; "" for a pull request
	Default   bool   // the head of the default branch
	Protected bool
	Extra     bool // selected through ScanOptions.Branches
	PR        int  // the pull request number; 0 for a branch head
	// Fork is true for a pull request whose head repository is not this one, or is unknown (a deleted fork).
	Fork              bool
	AuthorAssociation string
	CommittedAt       time.Time // known for extra branch heads only
	Facts             model.TriggerFacts
}

// HeadScan is the result of one ScanHeads call.
type HeadScan struct {
	Heads         []Head
	NotModified   bool // every conditional read answered 304: the previous heads still stand
	DefaultBranch string
	Calls         int // requests made, 304s included
}

type ghCommit struct {
	Commit struct {
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

// ScanHeads lists the candidate heads of repo for the watcher.
func (e *Engine) ScanHeads(ctx context.Context, repo string, o ScanOptions) (HeadScan, error) {
	var res HeadScan
	s, err := e.session(ctx, kindScan, e.passLimit())
	if err != nil {
		return res, err
	}
	defer func() { res.Calls = s.used; _ = e.flush(ctx) }()
	params := repoParams(repo)
	tags := map[string]string{}
	for _, k := range []string{"watch:repo", "watch:pulls", "watch:branches"} {
		if tags[k], err = e.etag(ctx, repo, k); err != nil {
			return res, err
		}
	}
	var info repoInfo
	rr, err := s.get(ctx, "/repos/{owner}/{repo}", params, nil, tags["watch:repo"], &info)
	if err != nil {
		return res, err
	}
	if rr.Status == 304 {
		if ok, gerr := e.getFact(ctx, repo, "repoinfo", &info); gerr != nil || !ok {
			if rr, err = s.get(ctx, "/repos/{owner}/{repo}", params, nil, "", &info); err != nil {
				return res, err
			}
		}
	}
	if rr.Status != 304 {
		if err = e.checkIdentity(ctx, repo, info.ID, info.FullName); err != nil {
			return res, err
		}
		if err = e.putFact(ctx, repo, "repoinfo", info); err != nil {
			return res, err
		}
	}
	res.DefaultBranch = info.DefaultBranch
	var pulls []pullInfo
	pr, err := s.get(ctx, "/repos/{owner}/{repo}/pulls", params, url.Values{"state": {"open"}, "per_page": {"100"}, "sort": {"updated"}, "direction": {"desc"}}, tags["watch:pulls"], &pulls)
	if err != nil {
		return res, err
	}
	var all []ghBranch
	br := pr
	if len(o.Branches) > 0 {
		if br, err = s.get(ctx, "/repos/{owner}/{repo}/branches", params, url.Values{"per_page": {"100"}}, tags["watch:branches"], &all); err != nil {
			return res, err
		}
	}
	if rr.Status == 304 && pr.Status == 304 && br.Status == 304 {
		res.NotModified = true
		return res, nil
	}
	if pr.Status == 304 { // the list did not change but the heads are needed again: read it in full
		if pr, err = s.get(ctx, "/repos/{owner}/{repo}/pulls", params, url.Values{"state": {"open"}, "per_page": {"100"}, "sort": {"updated"}, "direction": {"desc"}}, "", &pulls); err != nil {
			return res, err
		}
	}
	if len(o.Branches) > 0 && br.Status == 304 {
		if br, err = s.get(ctx, "/repos/{owner}/{repo}/branches", params, url.Values{"per_page": {"100"}}, "", &all); err != nil {
			return res, err
		}
	}
	heads, err := e.branchHeads(ctx, s, params, info)
	if err != nil {
		return res, err
	}
	now := e.now()
	seen := map[string]bool{}
	for _, h := range heads {
		seen[h.Branch] = true
		res.Heads = append(res.Heads, Head{SHA: h.SHA, Branch: h.Branch, Default: h.Branch == info.DefaultBranch, Protected: h.Protected, Facts: headFacts(repo, info, h, now)})
	}
	for i := range pulls {
		if i >= MaxPRs {
			break
		}
		p := pulls[i]
		if len(p.Head.SHA) != 40 {
			continue
		}
		fork := p.Head.Repo == nil || p.Head.Repo.ID == 0 || info.ID == 0 || p.Head.Repo.ID != info.ID
		res.Heads = append(res.Heads, Head{SHA: p.Head.SHA, PR: p.Number, Fork: fork, AuthorAssociation: p.AuthorAssociation, Facts: headFacts(repo, info, head{SHA: p.Head.SHA, PR: &p}, now)})
	}
	extra, err := e.extraHeads(ctx, s, repo, info, all, seen, o)
	if err != nil {
		return res, err
	}
	res.Heads = append(res.Heads, extra...)
	if err = e.setEtag(ctx, repo, "watch:repo", rr.ETag); err != nil {
		return res, err
	}
	if err = e.setEtag(ctx, repo, "watch:pulls", pr.ETag); err != nil {
		return res, err
	}
	if len(o.Branches) > 0 {
		err = e.setEtag(ctx, repo, "watch:branches", br.ETag)
	}
	return res, err
}

// extraHeads resolves the operator's branch globs against the branch listing: at most MaxBranches heads,
// each newer than Since (one commit read per matched branch).
func (e *Engine) extraHeads(ctx context.Context, s *session, repo string, info repoInfo, all []ghBranch, seen map[string]bool, o ScanOptions) ([]Head, error) {
	var out []Head
	now := e.now()
	for _, b := range all {
		if len(out) >= o.MaxBranches {
			break
		}
		if seen[b.Name] || len(b.Commit.SHA) != 40 || !matchAny(o.Branches, b.Name) {
			continue
		}
		var c ghCommit
		p := repoParams(repo)
		p["sha"] = b.Commit.SHA
		if _, err := s.get(ctx, "/repos/{owner}/{repo}/commits/{sha}", p, nil, "", &c); err != nil {
			return nil, err
		}
		at := c.Commit.Committer.Date
		if !o.Since.IsZero() && !at.After(o.Since) {
			continue
		}
		h := head{SHA: b.Commit.SHA, Branch: b.Name, Protected: b.Protected}
		out = append(out, Head{SHA: h.SHA, Branch: h.Branch, Protected: h.Protected, Extra: true, CommittedAt: at, Facts: headFacts(repo, info, h, now)})
	}
	return out, nil
}

// matchAny reports whether name equals a pattern or matches it as a path.Match glob.
func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if p == name {
			return true
		}
		if ok, err := path.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}
