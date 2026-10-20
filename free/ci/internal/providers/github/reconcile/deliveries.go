package reconcile

// Purpose: webhook mode, first leg of the sweep (Epic D9): when the token may list the coordinator's hook
// deliveries, redeliver each failed delivery of the last 72 hours once.
// Inputs:  GET /repos/{o}/{r}/hooks, GET .../hooks/{id}/deliveries, POST .../deliveries/{id}/attempts.
// Outputs: redelivered(count), and usable=false when the token may not list hooks or deliveries (403/404)
//          or no hook of the coordinator exists, which sends the caller to the head scan.
// Constraints: one delivery id (GUID) is redelivered at most once, remembered by a host-provider mark, and a
//          GUID with any successful attempt is not redelivered; only the newest page (100) of deliveries is
//          read; the admin:repo_hook scope is never requested from code (a refusal of the list or of a
//          redelivery is the signal to fall back); the hook is the coordinator's only when
//          providers.github.hook_url is set, the hook is active, covers push, pull_request or workflow_job
//          events and lives under that URL (scheme and host exact, path on a "/" boundary).

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// redeliverHorizon is how far back a failed delivery is still worth redelivering (GitHub keeps 3 days).
const redeliverHorizon = 72 * time.Hour

type ghHook struct {
	ID     int64    `json:"id"`
	Active bool     `json:"active"`
	Events []string `json:"events"`
	Config struct {
		URL string `json:"url"`
	} `json:"config"`
}

type ghDelivery struct {
	ID          int64     `json:"id"`
	GUID        string    `json:"guid"`
	DeliveredAt time.Time `json:"delivered_at"`
	StatusCode  int       `json:"status_code"`
}

// ours reports whether h is the coordinator's hook under the settings. Without providers.github.hook_url
// no hook is recognised (a repository can hold hooks of other services, and a redelivery would trigger
// them), so the head scan runs instead.
func (h ghHook) ours(st Settings) bool {
	if !h.Active || !urlUnder(h.Config.URL, st.HookURL) {
		return false
	}
	return slices.ContainsFunc(h.Events, func(ev string) bool {
		return ev == "*" || ev == "push" || ev == "pull_request" || ev == "workflow_job"
	})
}

// urlUnder reports whether got is want or lives under it: same scheme and host (exact, case-insensitive)
// and a path equal to want's or continuing it after a "/".
func urlUnder(got, want string) bool {
	if want == "" {
		return false
	}
	g, err1 := url.Parse(got)
	w, err2 := url.Parse(want)
	if err1 != nil || err2 != nil || g.Scheme != w.Scheme || !strings.EqualFold(g.Host, w.Host) {
		return false
	}
	gp, wp := strings.TrimRight(g.Path, "/"), strings.TrimRight(w.Path, "/")
	return gp == wp || strings.HasPrefix(gp, wp+"/")
}

// redeliver runs the webhook-mode leg for repo. usable is false when the head scan must run instead.
func (e *Engine) redeliver(ctx context.Context, s *session, repo string, st Settings) (done int, usable bool, err error) {
	var hooks []ghHook
	if _, err = s.get(ctx, "/repos/{owner}/{repo}/hooks", repoParams(repo), url.Values{"per_page": {"100"}}, "", &hooks); err != nil {
		if denied(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	for _, h := range hooks {
		if !h.ours(st) {
			continue
		}
		n, ok, herr := e.redeliverHook(ctx, s, repo, h)
		done += n
		usable = usable || ok
		if herr != nil {
			return done, usable, herr
		}
	}
	return done, usable, nil
}

func (e *Engine) redeliverHook(ctx context.Context, s *session, repo string, h ghHook) (int, bool, error) {
	params := repoParams(repo)
	params["hook"] = strconv.FormatInt(h.ID, 10)
	var list []ghDelivery
	if _, err := s.get(ctx, "/repos/{owner}/{repo}/hooks/{hook}/deliveries", params, url.Values{"per_page": {"100"}}, "", &list); err != nil {
		if denied(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	cutoff := e.now().Add(-redeliverHorizon)
	latest := map[string]ghDelivery{} // newest attempt of each GUID
	ok := map[string]bool{}           // GUIDs with a successful attempt
	for _, d := range list {
		if d.GUID == "" || d.DeliveredAt.Before(cutoff) {
			continue
		}
		if d.StatusCode >= 200 && d.StatusCode < 300 {
			ok[d.GUID] = true
		}
		if d.ID > latest[d.GUID].ID {
			latest[d.GUID] = d
		}
	}
	guids := make([]string, 0, len(latest))
	for g := range latest {
		if !ok[g] {
			guids = append(guids, g)
		}
	}
	slices.SortFunc(guids, func(a, b string) int { return int(latest[a].ID - latest[b].ID) }) // oldest first
	done := 0
	var firstErr error
	for _, g := range guids {
		id := strconv.FormatInt(h.ID, 10) + ":" + g
		seen, err := e.marked(ctx, "redeliver", repo, id)
		if err != nil {
			return done, true, err
		}
		if seen {
			continue
		}
		params["delivery"] = strconv.FormatInt(latest[g].ID, 10)
		if _, err = s.post(ctx, "/repos/{owner}/{repo}/hooks/{hook}/deliveries/{delivery}/attempts", params); err != nil {
			if forbidden(err) { // the token may list deliveries but not redeliver them: scan the heads instead
				return done, false, nil
			}
			var pe *providers.Error
			if errors.As(err, &pe) && pe.Status == 404 { // that delivery is gone: skip it for good, the newer ones go on
				if merr := e.mark(ctx, "redeliver", repo, id); merr != nil {
					return done, true, merr
				}
				continue
			}
			var quota *QuotaStop
			if errors.As(err, &quota) || errors.Is(err, ErrCallBound) || errors.Is(err, ErrBudget) {
				return done, true, err
			}
			if firstErr == nil { // one refused delivery must not hide the newer ones
				firstErr = err
			}
			continue
		}
		if err = e.mark(ctx, "redeliver", repo, id); err != nil {
			return done, true, err
		}
		done++
	}
	return done, true, firstErr
}
