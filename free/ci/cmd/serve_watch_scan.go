package main

// Purpose: the head scan port of `nself ci serve watch` (serve_watch.go wires it): HOST-17 scanning and
//          the TRUST classification of each scanned head.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/reconcile"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
	"github.com/nself-org/plugins/free/ci/internal/watch"
)

// watchScanner is the HOST-17 head scan behind watch.Scanner.
type watchScanner struct {
	e    *reconcile.Engine
	opts reconcile.ScanOptions
	days int
}

func (s watchScanner) Scan(ctx context.Context, repo string) (watch.Scan, error) {
	o := s.opts
	if s.days > 0 {
		o.Since = s.e.Now().Add(-time.Duration(s.days) * 24 * time.Hour)
	}
	hs, err := s.e.ScanHeads(ctx, repo, o)
	var q *reconcile.QuotaStop
	if errors.As(err, &q) {
		return watch.Scan{}, &watch.QuotaError{RetryAt: q.RetryAt}
	}
	if err != nil || hs.NotModified {
		return watch.Scan{NotModified: hs.NotModified}, err
	}
	out := watch.Scan{}
	for _, h := range hs.Heads {
		out.Heads = append(out.Heads, watchHead(repo, h))
	}
	return out, nil
}

// watchHead classifies a scanned head by the TRUST source rules and restamps its facts as a watch
// trigger. Only a writer can push a branch of the repository; the default and protected branches are
// the owner's; a pull request's author association names its permission; a fork is untrusted.
func watchHead(repo string, h reconcile.Head) watch.Head {
	f := h.Facts
	now := f.ReceivedAt
	perm := "write"
	switch {
	case h.Fork:
		perm = ""
	case h.PR > 0:
		switch strings.ToUpper(h.AuthorAssociation) {
		case "OWNER":
			perm = "admin"
		case "MEMBER", "COLLABORATOR":
		default:
			perm = ""
		}
	case h.Default || h.Protected:
		perm = "admin"
	}
	if perm != "" {
		f.ActorPermission = model.Fact[string]{Value: &perm, Source: "api", ObservedAt: now, Confidence: "known"}
	}
	if h.Default {
		yes := true
		f.SHAReachable = model.Fact[bool]{Value: &yes, Source: "api", ObservedAt: now, Confidence: "known"}
	}
	class := source.Classify(f).Class
	f.Kind, f.Source, f.Auth = "api", "watch", "per-repo"
	return watch.Head{Repo: repo, SHA: h.SHA, Branch: h.Branch, Default: h.Default, Protected: h.Protected, Extra: h.Extra,
		PR: h.PR, Fork: h.Fork, Trust: class, CommittedAt: h.CommittedAt, Facts: f}
}

// watchRunner runs a commit through the coordinator in a detached worktree of its local checkout.
type watchRunner struct {
	roots map[string]string
	home  string
}

func (r watchRunner) Run(ctx context.Context, j watch.Job) (watch.Outcome, error) {
	root, ok := r.roots[j.Head.Repo]
	if !ok || j.Isolated {
		return watch.Outcome{}, fmt.Errorf("no local runner for %s", j.Head.Repo)
	}
	git := func(c context.Context, args ...string) error {
		out, exit, err := report.Capture(c, append([]string{"git", "-C", root}, args...)...)
		if err == nil && exit != 0 {
			err = fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(out))
		}
		return err
	}
	dir := filepath.Join(r.home, "watch", "work", j.Head.SHA[:12])
	if err := git(ctx, "fetch", "--quiet", "--no-tags", "origin", j.Head.SHA); err != nil {
		return watch.Outcome{}, err
	}
	if err := git(ctx, "worktree", "add", "--detach", "--force", dir, j.Head.SHA); err != nil {
		return watch.Outcome{}, err
	}
	defer func() { _ = git(context.WithoutCancel(ctx), "worktree", "remove", "--force", dir) }()
	jobs := j.Only
	if len(jobs) == 0 {
		jobs = []string{""}
	}
	worst := watch.Outcome{State: report.Success, Description: "passed"}
	for _, job := range jobs {
		o := RunOptions{Root: dir, Home: r.home, Job: job, Trigger: "api", Record: true, NoStatus: true}
		if err := o.Validate(cirun.Hooks{}.Resolved()); err != nil {
			return watch.Outcome{}, err
		}
		p, digest, subst, err := preparePlan(ctx, o)
		if err != nil {
			return watch.Outcome{}, err
		}
		c := &cirun.Coordinator{Opts: o, Plan: p, ConfigDigest: digest, ConfigOutside: configOutside(ctx, dir), Substantive: subst, Out: os.Stderr}
		res, err := c.Run(ctx)
		switch {
		case err != nil:
			return watch.Outcome{}, err
		case res.Exit == 1:
			worst = watch.Outcome{State: report.Failure, Description: "gate failed (run " + res.RunID + ")"}
		case res.Exit != 0 && worst.State == report.Success:
			worst = watch.Outcome{State: report.Error, Description: "infrastructure error (run " + res.RunID + ")"}
		}
	}
	return worst, nil
}
