package watch

// Purpose: one watch pass (P7-AUTO-11): discover the heads of every watched repository, consider them
//          newest default-branch head first, run at most one, then start a due background pipeline when
//          nothing ran, and publish the demand document and the heartbeat.
// Inputs:  Config and Ports.
// Outputs: PassReport; the demand document, heartbeat and log events through the ports.
// Constraints: a quota error stops the pass and records the reset time; no scan is made before it; a
//          304 pass reuses the previous heads; demand is published every pass, ready or not; a not-ready
//          host claims nothing.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/demand"
)

// Watcher is one host's watch loop. Build it with New.
type Watcher struct {
	C Config
	P Ports

	mu         sync.Mutex
	heads      map[string][]Head    // the last complete scan per repository
	firstSeen  map[string]time.Time // repo@sha -> provider time it was first seen without a final result
	dups       map[string]bool      // repo@sha whose duplicate final was reported
	quotaUntil time.Time
}

// New checks the required ports and fills the defaults of c.
func New(c Config, p Ports) (*Watcher, error) {
	if p.Scanner == nil || p.Ledger == nil || p.Reporter == nil || p.Runner == nil {
		return nil, &Error{Code: CodeConfig, Message: "the watcher needs a scanner, a ledger, a reporter and a runner"}
	}
	if c.Host == "" || strings.ContainsAny(c.Host, " \t\n") || strings.ContainsAny(c.Incarnation, " \t\n") {
		return nil, &Error{Code: CodeConfig, Message: "the watcher needs a host name without spaces"}
	}
	d := Defaults()
	if c.Interval <= 0 {
		c.Interval = d.Interval
	}
	if c.RetryGap <= 0 {
		c.RetryGap = d.RetryGap
	}
	if c.Step <= 0 {
		c.Step = d.Step
	}
	if c.RunLimit <= 0 {
		c.RunLimit = d.RunLimit
	}
	if c.Incarnation == "" {
		c.Incarnation = "i1"
	}
	return &Watcher{C: c, P: p, heads: map[string][]Head{}, firstSeen: map[string]time.Time{}, dups: map[string]bool{}}, nil
}

func (w *Watcher) now() time.Time {
	if w.P.Now != nil {
		return w.P.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *Watcher) after(d time.Duration) <-chan time.Time {
	if w.P.After != nil {
		return w.P.After(d)
	}
	return time.After(d)
}

func (w *Watcher) log(event string, fields map[string]any) {
	if w.P.Log != nil {
		w.P.Log(event, fields)
	}
}

// QuotaUntil is the recorded reset time of the last quota error (zero when none).
func (w *Watcher) QuotaUntil() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.quotaUntil
}

// Pass runs one pass.
func (w *Watcher) Pass(ctx context.Context) PassReport {
	rep := PassReport{At: w.now(), ReadyWhy: []string{}, Decisions: []Decision{}, Errors: []string{}}
	if w.P.Ready != nil {
		rep.Ready, rep.ReadyWhy = w.P.Ready(ctx)
		if rep.ReadyWhy == nil {
			rep.ReadyWhy = []string{}
		}
	} else {
		rep.ReadyWhy = []string{"no readiness source"}
	}
	heads, stopped := w.discover(ctx, &rep)
	if !stopped {
		Order(heads)
		ran := false
		for _, h := range heads {
			if ctx.Err() != nil {
				break
			}
			d := w.consider(ctx, h, rep.Ready, ran, &rep)
			if d.Action == "run" {
				ran = true
				rep.Ran++
			}
			rep.Decisions = append(rep.Decisions, d)
		}
		if !ran && rep.Ready && w.P.Idle != nil && ctx.Err() == nil {
			started, err := w.P.Idle(ctx)
			rep.Idle = started && err == nil
			if err != nil {
				rep.Errors = append(rep.Errors, "background: "+err.Error())
			}
		}
	}
	w.publish(rep)
	w.log("pass", map[string]any{"ran": rep.Ran, "queued": rep.Queued, "held": rep.Held, "ready": rep.Ready, "decisions": len(rep.Decisions), "errors": len(rep.Errors)})
	return rep
}

// discover returns the heads to consider; stopped is true when a quota error ended the pass.
func (w *Watcher) discover(ctx context.Context, rep *PassReport) (heads []Head, stopped bool) {
	w.mu.Lock()
	until := w.quotaUntil
	w.mu.Unlock()
	if until.After(rep.At) {
		rep.QuotaUntil = until
		return nil, true
	}
	for _, repo := range w.C.Repos {
		s, err := w.P.Scanner.Scan(ctx, repo)
		var q *QuotaError
		switch {
		case errors.As(err, &q):
			w.mu.Lock()
			w.quotaUntil = q.RetryAt.UTC()
			w.mu.Unlock()
			rep.QuotaUntil = q.RetryAt.UTC()
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %s", CodeQuota, q.Error()))
			w.log("quota", map[string]any{"repo": repo, "retry_at": q.RetryAt.UTC().Format(time.RFC3339)})
			return nil, true
		case err != nil:
			rep.Errors = append(rep.Errors, repo+": "+err.Error())
		case !s.NotModified:
			for i := range s.Heads {
				s.Heads[i].Repo = repo
			}
			w.mu.Lock()
			w.heads[repo] = s.Heads
			w.mu.Unlock()
		}
		w.mu.Lock()
		heads = append(heads, w.heads[repo]...)
		w.mu.Unlock()
	}
	return heads, false
}

// publish writes the demand document and the heartbeat; failures are logged, never fatal.
func (w *Watcher) publish(rep PassReport) {
	if w.P.Publish != nil {
		doc := demand.Document{Host: w.C.Host, Queued: rep.Queued, Held: rep.Held, Ready: rep.Ready, ReadyWhy: rep.ReadyWhy,
			Watcher: demand.Watcher{Alive: true, LastPass: rep.At.Format(time.RFC3339), IntervalSec: w.C.Interval.Seconds()}}
		if err := w.P.Publish(doc); err != nil {
			w.log("demand_error", map[string]any{"error": err.Error()})
		}
	}
	if w.P.Heartbeat != nil {
		if err := w.P.Heartbeat(rep.At); err != nil {
			w.log("heartbeat_error", map[string]any{"error": err.Error()})
		}
	}
}

func key(h Head) string { return strings.ToLower(h.Repo) + "@" + h.SHA }
