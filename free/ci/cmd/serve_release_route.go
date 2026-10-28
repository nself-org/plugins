package main

// Purpose: the TEAM coordinator's two release routes (P7-CACHE-17, Epic CACHE D15):
// GET /v1/runs/{id}/release-inputs and GET /v1/runs/{id}/release-blobs/{sha256}. A TEAM coordinator
// holds secret references only (Epic X8), so it cannot sign; these routes let a release-role
// operator fetch what a release is built from and sign it on their own machine
// (`nself ci artifacts sign <run> --coordinator <url>`, artifacts_sign_remote.go).
// Inputs:  the user API (clientapi.RegisterRoute), the coordinator's store and blob store, which a
// default-on listener hands over when `ci serve` starts.
// Outputs: the two routes on every user API server; until the serve has started they answer not found.
// Constraints: the handlers (internal/release/remote) authorize with roles.Authorize(actor, release,
// project) before they read a run; the project and the blob realm come from the stored run, never from
// the caller; the coordinator never calls the secret resolver and holds no key; this file does not
// edit the user API or the serve listeners, it only registers beside them.

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/release/remote"
)

// releaseRouteDeps is set by the listener below while a serve runs.
var releaseRouteDeps atomic.Pointer[remote.Deps]

func init() {
	h := remote.Handlers{Deps: func() (remote.Deps, bool) {
		d := releaseRouteDeps.Load()
		if d == nil {
			return remote.Deps{}, false
		}
		return *d, true
	}}
	// Registered with the member role so the wrapper's project-less check passes for a caller scoped to
	// one project; the handler itself requires the release role on the run's project (E687).
	for _, r := range []struct {
		pattern string
		h       http.Handler
	}{{remote.PatternInputs, h.InputsHandler()}, {remote.PatternBlob, h.BlobHandler()}} {
		if err := clientapi.RegisterRoute(http.MethodGet, r.pattern, "member", r.h); err != nil {
			panic("nself-ci: " + err.Error())
		}
	}
	if err := coordinator.RegisterListener("release-routes", "release-routes", "on", startReleaseRoutes); err != nil {
		panic("nself-ci: " + err.Error())
	}
}

// startReleaseRoutes captures the coordinator's store and blob store for the handlers. It binds nothing.
func startReleaseRoutes(ctx context.Context, addr string, c *coordinator.Coordinator) (func(context.Context) error, error) {
	if ctx.Err() != nil || addr == "off" {
		return noop, nil
	}
	if c.Store() == nil { // a nil *store.Store would pass the interface's nil check and panic per request
		c.Logf("release routes are off: the coordinator has no store")
		return noop, nil
	}
	home := serveHome()
	blobs, err := cas.New(records.StoreDir(home))
	if err != nil {
		c.Logf("release routes are off: the blob store could not be opened: %v", err)
		return noop, nil
	}
	releaseRouteDeps.Store(&remote.Deps{Store: c.Store(), Blobs: blobs, Cache: &remote.InputsCache{},
		Runs: &clientapi.StoreBackend{Store: c.Store(), Home: home}})
	return func(context.Context) error { releaseRouteDeps.Store(nil); return nil }, nil
}
