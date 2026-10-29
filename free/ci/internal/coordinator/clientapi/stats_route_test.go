package clientapi

// Purpose: tests of GET /v1/stats (P7-SCHED-20): the member role is enough, a scoped caller gets an Allow
// that admits only the projects it may read and no coordinator-wide view, an all-project caller does, a
// project the caller may not read is refused with the role error, and bad windows and projects are 400.
// Inputs:  the rig of rig_test.go and a recording StatsSource. Outputs: pass or fail.
// Constraints: the route registers once per process from init; real TLS on 127.0.0.1.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/stats"
)

type statsSeen struct {
	sync.Mutex
	opts stats.Options
}

func statsRig(t *testing.T) (*rig, *statsSeen) {
	t.Helper()
	seen := &statsSeen{}
	SetStatsSource(func(_ context.Context, o stats.Options) (stats.Doc, error) {
		seen.Lock()
		defer seen.Unlock()
		seen.opts = o
		return stats.Doc{Contract: stats.Contract, Version: stats.Version, SinceMs: o.Since.Milliseconds()}, nil
	})
	t.Cleanup(func() { SetStatsSource(nil) })
	return newRig(t), seen
}

func TestStatsRouteScopesTheDocumentToTheCaller(t *testing.T) {
	r, seen := statsRig(t)
	c := r.client()
	scoped := r.token("mia", roles.Member, time.Hour, "acme/app")
	status, body := r.do(c, http.MethodGet, PathStats+"?since=6h", scoped, "")
	var doc stats.Doc
	if status != 200 || json.Unmarshal(body, &doc) != nil || doc.Contract != "ci.stats" || doc.SinceMs != 6*3600*1000 {
		t.Fatalf("scoped member: %d %s", status, body)
	}
	if seen.opts.Global || !seen.opts.Allow("acme/app") || seen.opts.Allow("acme/other") {
		t.Errorf("a scoped caller must see only its project and nothing coordinator-wide: global=%v", seen.opts.Global)
	}
	all := r.token("ops", roles.Member, time.Hour, roles.AllProjects)
	if status, body = r.do(c, http.MethodGet, PathStats, all, ""); status != 200 || !seen.opts.Global || seen.opts.Since != 24*time.Hour {
		t.Fatalf("all-project member: %d %s global=%v since=%s", status, body, seen.opts.Global, seen.opts.Since)
	}
}

func TestStatsRouteRefusals(t *testing.T) {
	r, seen := statsRig(t)
	c := r.client()
	scoped := r.token("mia", roles.Member, time.Hour, "acme/app")
	if status, body := r.do(c, http.MethodGet, PathStats+"?project="+url.QueryEscape("acme/other"), scoped, ""); status == 200 {
		t.Fatalf("a project the caller may not read was served: %s", body)
	}
	if status, body := r.do(c, http.MethodGet, PathStats+"?project=acme/app", scoped, ""); status != 200 || seen.opts.Project != "acme/app" {
		t.Fatalf("own project: %d %s", status, body)
	}
	for _, q := range []string{"?since=banana", "?since=0s", "?since=91d", "?project=not-a-project"} {
		status, body := r.do(c, http.MethodGet, PathStats+q, scoped, "")
		refused(t, status, body, http.StatusBadRequest, ReasonBadRequest)
	}
	if status, _ := r.do(c, http.MethodGet, PathStats, "", ""); status == 200 {
		t.Error("an anonymous caller was served")
	}
}

func TestParseSince(t *testing.T) {
	for in, want := range map[string]time.Duration{"": 24 * time.Hour, "90m": 90 * time.Minute, "7d": 7 * 24 * time.Hour, "2160h": MaxStatsSince} {
		if got, err := ParseSince(in); err != nil || got != want {
			t.Errorf("ParseSince(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"-1h", "0", "xd", "100d", "1.5"} {
		if _, err := ParseSince(in); err == nil {
			t.Errorf("ParseSince(%q) accepted", in)
		}
	}
}
