package main

// Purpose: the two ports of `nself ci serve watch` that P7-WIRE-09 turns on: the hosted tier
//          (P7-AUTO-07, watch.Hosted over hostedtier.Observer) and the idle tick (P7-AUTO-20, one tidy
//          pass when no commit ran and the last tidy is older than idleTidyEvery).
// Inputs:  the gh CLI (check runs and annotations of one commit), the hosted provider's breaker, the
//          run store, the tidy policy and $NSELF_CI_HOME.
// Outputs: hostedPort (watch.Hosted), idleTick (watch.Ports.Idle).
// Constraints: the hosted port only reads; a repository or sha that is not well formed, a gh failure
//          or an unreadable answer is an API error, which hands over to local nodes (fail closed,
//          never a pass); the hosted.* keys are not in the policy key set yet, so the Config is the
//          defaults (mode free, context nself-ci, the Actions app); the idle tidy is the history
//          retention prune and the orphan job-root sweep (no cache prune: that is an admin action)
//          and runs at most once per idleTidyEvery.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/history"
	"github.com/nself-org/plugins/free/ci/internal/hostedtier"
	"github.com/nself-org/plugins/free/ci/internal/selftest"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// hostedProviderID is the provider id whose breaker the hosted tier reads (HOST-11).
const hostedProviderID = "github-hosted"

// idleTidyEvery is the cadence of the idle tidy.
const idleTidyEvery = 24 * time.Hour

var (
	repoName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	shaName  = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
)

type captureFunc func(ctx context.Context, argv ...string) (string, int, error)

// hostedPort is watch.Hosted: one hostedtier.Observer per decision, over the gh funnel of the repository.
type hostedPort struct {
	cfg     hostedtier.Config
	breaker hostedtier.Breaker
	capture captureFunc
	now     func() time.Time
}

// Decide implements watch.Hosted.
func (p hostedPort) Decide(ctx context.Context, repo, sha string, pushed time.Time) hostedtier.Result {
	o := &hostedtier.Observer{Cfg: p.cfg, Funnel: ghFunnel{capture: p.capture, repo: repo}, Breaker: p.breaker, Now: p.now}
	return o.Decide(ctx, sha, pushed)
}

// ghFunnel is hostedtier.Funnel through `gh api` for one repository.
type ghFunnel struct {
	capture captureFunc
	repo    string
}

func (f ghFunnel) get(ctx context.Context, path string, into any) error {
	out, exit, err := f.capture(ctx, "gh", "api", path)
	if err != nil {
		return fmt.Errorf("hosted: gh not run: %w", err)
	}
	if exit != 0 {
		return fmt.Errorf("hosted: gh api exit %d", exit)
	}
	if err := json.Unmarshal([]byte(out), into); err != nil {
		return fmt.Errorf("hosted: unreadable answer: %w", err)
	}
	return nil
}

// CheckRuns implements hostedtier.Funnel. GitHub sets started_at when a check run is created, so it
// is also the creation time the queue timer reads.
func (f ghFunnel) CheckRuns(ctx context.Context, sha string) ([]hostedtier.CheckRun, error) {
	if !repoName.MatchString(f.repo) || !shaName.MatchString(sha) {
		return nil, fmt.Errorf("hosted: %q@%q is not owner/repo@sha", f.repo, sha)
	}
	var body struct {
		CheckRuns []struct {
			ID         int64     `json:"id"`
			Name       string    `json:"name"`
			Status     string    `json:"status"`
			Conclusion string    `json:"conclusion"`
			StartedAt  time.Time `json:"started_at"`
			App        struct {
				ID int64 `json:"id"`
			} `json:"app"`
		} `json:"check_runs"`
	}
	if err := f.get(ctx, fmt.Sprintf("repos/%s/commits/%s/check-runs?per_page=100", f.repo, sha), &body); err != nil {
		return nil, err
	}
	runs := make([]hostedtier.CheckRun, 0, len(body.CheckRuns))
	for _, c := range body.CheckRuns {
		runs = append(runs, hostedtier.CheckRun{ID: c.ID, Name: c.Name, AppID: c.App.ID, Status: c.Status, Conclusion: c.Conclusion, CreatedAt: c.StartedAt, StartedAt: c.StartedAt})
	}
	return runs, nil
}

// Annotations implements hostedtier.Funnel.
func (f ghFunnel) Annotations(ctx context.Context, runID int64) ([]string, error) {
	if !repoName.MatchString(f.repo) || runID <= 0 {
		return nil, fmt.Errorf("hosted: bad check run %s#%d", f.repo, runID)
	}
	var body []struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	if err := f.get(ctx, fmt.Sprintf("repos/%s/check-runs/%d/annotations?per_page=100", f.repo, runID), &body); err != nil {
		return nil, err
	}
	notes := make([]string, 0, len(body))
	for _, a := range body {
		notes = append(notes, strings.TrimSpace(a.Title+" "+a.Message))
	}
	return notes, nil
}

// idleTick is watch.Ports.Idle: one tidy when the last one is older than every.
type idleTick struct {
	mu    sync.Mutex
	last  time.Time
	every time.Duration
	now   func() time.Time
	tidy  func(ctx context.Context) (selftest.Report, error)
	log   func(event string, fields map[string]any)
}

// Run reports whether a tidy started; a tidy that failed still counts as the due one.
func (t *idleTick) Run(ctx context.Context) (bool, error) {
	t.mu.Lock()
	now := t.now()
	if !t.last.IsZero() && now.Sub(t.last) < t.every {
		t.mu.Unlock()
		return false, nil
	}
	t.last = now
	t.mu.Unlock()
	rep, err := t.tidy(ctx)
	if t.log != nil {
		t.log("idle", map[string]any{"job": "tidy", "items": len(rep.Items), "free_mb": rep.FreeAfterMB, "target_mb": rep.TargetMB, "errors": len(rep.Errors)})
	}
	return true, err
}

// watchTidy is the tidy the idle tick runs: retention prune and orphan sweep, applied.
func watchTidy(st *store.Store, home string, cfg selftest.TidyConfig) func(ctx context.Context) (selftest.Report, error) {
	return func(ctx context.Context) (selftest.Report, error) {
		pol, _ := history.LoadPolicy(home) // an unusable file reads as the defaults, as `runs prune --tidy` does
		t := selftest.Tidy{
			Steps: []selftest.Step{
				selftest.HistoryStep(st, home, pol, time.Now),
				selftest.OrphanStep(filepath.Join(home, "work"), home, time.Now),
			},
			TargetMB: cfg.FreeTargetMB,
			FreeMB:   func() (int64, error) { return selftest.FreeMB(home) },
		}
		return t.Run(ctx, true)
	}
}
