package clientapi

// Purpose: GET /v1/stats (P7-SCHED-20, Epic SCHED D23): contract:ci.stats v1 over the user API, the read
// behind `nself ci runs stats --coordinator <url>`. Needs the member role; the document covers only the
// projects the caller may read, and the blocks that carry no project (nodes, status outbox, store size) only
// for a caller who may read the whole coordinator.
// Inputs:  ?since=<duration, default 24h, at most 90d> and ?project=<owner/repo>.
// Outputs: the stats document as JSON; E664 on refusal.
// Constraints: read only; a project the caller may not read is refused with the usual role error; the
//          document is built by internal/stats from one read transaction, the same code the local command
//          runs, so both paths print one shape; the store is the one NSELF_CI_HOME names unless the serve
//          hands its own through SetStatsSource.

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/stats"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// PathStats is the stats route.
const PathStats = "/v1/stats"

// MaxStatsSince bounds the window a client may ask for.
const MaxStatsSince = 90 * 24 * time.Hour

// StatsSource builds the document for the options the route derived from the caller.
type StatsSource func(ctx context.Context, o stats.Options) (stats.Doc, error)

var (
	statsMu  sync.Mutex
	statsSrc StatsSource = localStats
)

// SetStatsSource replaces the document source (the serve hands in its own store); nil restores the default.
func SetStatsSource(s StatsSource) {
	statsMu.Lock()
	defer statsMu.Unlock()
	if s == nil {
		s = localStats
	}
	statsSrc = s
}

func localStats(ctx context.Context, o stats.Options) (stats.Doc, error) {
	st, err := store.Open("", store.Options{})
	if err != nil {
		return stats.Doc{}, err
	}
	defer func() { _ = st.Close() }()
	return stats.Compute(ctx, st, o)
}

func init() {
	if err := RegisterRoute(http.MethodGet, PathStats, string(roles.Member), http.HandlerFunc(serveStats)); err != nil {
		panic(err)
	}
}

// ParseSince reads a window: a Go duration, or whole days as "7d". Empty is the default.
func ParseSince(v string) (time.Duration, error) {
	if v == "" {
		return stats.DefaultSince, nil
	}
	d, err := time.ParseDuration(v)
	if n, ok := strings.CutSuffix(v, "d"); err != nil && ok {
		days, perr := strconv.Atoi(n)
		d, err = time.Duration(days)*24*time.Hour, perr
	}
	if err != nil || d <= 0 || d > MaxStatsSince {
		return 0, Failf(ReasonBadRequest, "since must be a duration between 1s and 90d")
	}
	return d, nil
}

func serveStats(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorFrom(r.Context())
	if !ok {
		errServer.fail(w, Failf(ReasonInternal, "no caller"))
		return
	}
	since, err := ParseSince(r.URL.Query().Get("since"))
	project := r.URL.Query().Get("project")
	switch {
	case err != nil:
		errServer.fail(w, err)
		return
	case project != "" && !coordinator.ValidProject(project):
		errServer.fail(w, Failf(ReasonBadRequest, "project must be owner/repo"))
		return
	case project != "":
		if err = actor.Can(roles.Member, project); err != nil {
			errServer.fail(w, err)
			return
		}
	}
	statsMu.Lock()
	src := statsSrc
	statsMu.Unlock()
	doc, err := src(r.Context(), stats.Options{Now: time.Now(), Since: since, Project: project, Global: wholeCoordinator(actor),
		Allow: func(p string) bool { return actor.Can(roles.Member, p) == nil }})
	if err != nil {
		errServer.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}
