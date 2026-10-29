package report

// Purpose: which context carries a run's result (P7-AUTO-10). An unpinned
// repository gets "nself-ci"; when the required check is app-pinned (only that
// app may write it) or protection cannot be read, the result goes to
// "nself-ci/watch" so a claim or result never fights the pin.
// Inputs:  a repo and an optional override. Outputs: a context name.
// Constraints: the protection read goes through the REST funnel and is cached
// for one hour; an unreadable answer is cached too, so a failing provider is
// not hammered; the rule itself is pure.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// WatchContext is the result context for pinned or unreadable repositories.
const WatchContext = Context + "/watch"

// ProtectionTTL is how long a protection read is reused.
const ProtectionTTL = time.Hour

// ResultContextFor is the pure rule.
func ResultContextFor(pinned, readable bool, override string) string {
	if override != "" {
		return override
	}
	if !readable || pinned {
		return WatchContext
	}
	return Context
}

// ProtectionReader reports whether the required check is app-pinned.
type ProtectionReader func(ctx context.Context, owner, repo string) (pinned bool, err error)

type protEntry struct {
	pinned, readable bool
	at               time.Time
}

// ResultContexts resolves and caches result contexts.
type ResultContexts struct {
	Read ProtectionReader
	Now  func() time.Time // nil: time.Now

	mu    sync.Mutex
	cache map[string]protEntry
}

// ResolveResultContext returns the result context of owner/repo.
func (r *ResultContexts) ResolveResultContext(ctx context.Context, owner, repo, override string) string {
	if override != "" {
		return override
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	k := owner + "/" + repo
	r.mu.Lock()
	e, ok := r.cache[k]
	r.mu.Unlock()
	if !ok || now().Sub(e.at) >= ProtectionTTL {
		e = protEntry{at: now()}
		if r.Read != nil {
			p, err := r.Read(ctx, owner, repo)
			e.pinned, e.readable = p, err == nil
		}
		r.mu.Lock()
		if r.cache == nil {
			r.cache = map[string]protEntry{}
		}
		r.cache[k] = e
		r.mu.Unlock()
	}
	return ResultContextFor(e.pinned, e.readable, "")
}

// RequiredPinned reads the default branch's required status checks and says
// whether "nself-ci" is pinned to an app.
func (g GH) RequiredPinned(ctx context.Context, owner, repo string) (bool, error) {
	path := fmt.Sprintf("repos/%s/%s/branches/main/protection/required_status_checks", owner, repo)
	out, exit, err := g.capture(ctx, "gh", "api", path)
	if err != nil || exit != 0 {
		if strings.Contains(out, "Branch not protected") {
			return false, nil
		}
		return false, fmt.Errorf("report: protection unreadable: %s", firstLine(out))
	}
	var doc struct {
		Checks []struct {
			Context string `json:"context"`
			AppID   *int64 `json:"app_id"`
		} `json:"checks"`
	}
	if err = json.Unmarshal([]byte(out), &doc); err != nil {
		return false, err
	}
	for _, c := range doc.Checks {
		if c.Context == Context && c.AppID != nil && *c.AppID > 0 {
			return true, nil
		}
	}
	return false, nil
}
