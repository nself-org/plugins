package remote_test

// Purpose: the one registration of the two release routes in the test binary. clientapi.RegisterRoute
// is process-wide and refuses a repeat, so the unit tests (serve_test.go) and the integration rig
// (rig_integration_test.go) share this; the Deps a request sees are swapped per test.
// Constraints: tests that use it do not run in parallel.

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/release/remote"
)

var (
	routesOnce sync.Once
	current    atomic.Pointer[remote.Deps]
)

// useDeps registers the routes (once) and serves d until the test ends; nil serves "not started".
func useDeps(t *testing.T, d *remote.Deps) {
	t.Helper()
	routesOnce.Do(func() {
		h := remote.Handlers{Deps: func() (remote.Deps, bool) {
			d := current.Load()
			if d == nil {
				return remote.Deps{}, false
			}
			return *d, true
		}}
		for pattern, handler := range map[string]http.Handler{remote.PatternInputs: h.InputsHandler(), remote.PatternBlob: h.BlobHandler()} {
			if err := clientapi.RegisterRoute(http.MethodGet, pattern, "member", handler); err != nil {
				t.Fatal(err)
			}
		}
	})
	current.Store(d)
	t.Cleanup(func() { current.Store(nil) })
}
