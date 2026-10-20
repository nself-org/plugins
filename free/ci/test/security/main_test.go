//go:build security

package security

// Purpose: the telemetry fake of the security suite (X15, TM-36). One HTTP server is the
// ping_api endpoint (every endpoint variable the product has ever read) and the HTTP(S)
// proxy of the whole test process, so a request to any host from the plugin or the agent
// reaches it. The suite fails if it records one request, however the test that caused it
// ended.
// Inputs:  none. Outputs: telemetryRequests(); a non-zero exit when any request arrived.
// Constraints: set before any HTTP client is created (net/http reads the proxy variables
//          once); loopback targets bypass a proxy by design, so the fixtures that talk to
//          their own 127.0.0.1 servers are not counted.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

var telemetry = struct {
	mu   sync.Mutex
	seen []string
	srv  *httptest.Server
}{}

func telemetryRequests() []string {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	return append([]string(nil), telemetry.seen...)
}

// endpointVars are the variables a telemetry client could read its destination from.
var endpointVars = []string{"PING_API_URL", "NSELF_PING_API", "NSELF_PING_URL", "NSELF_TELEMETRY_URL", "NSELF_CI_TELEMETRY_URL", "NSELF_ANALYTICS_URL"}

// proxyVars route every non-loopback request to the fake.
var proxyVars = []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"}

func TestMain(m *testing.M) {
	telemetry.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		telemetry.mu.Lock()
		telemetry.seen = append(telemetry.seen, fmt.Sprintf("%s %s%s", r.Method, r.Host, r.URL.Path))
		telemetry.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, k := range endpointVars {
		_ = os.Setenv(k, telemetry.srv.URL)
	}
	for _, k := range proxyVars {
		_ = os.Setenv(k, telemetry.srv.URL)
	}
	_ = os.Setenv("NO_PROXY", "")
	_ = os.Setenv("no_proxy", "")
	code := m.Run()
	telemetry.srv.Close()
	if seen := telemetryRequests(); len(seen) != 0 {
		fmt.Fprintf(os.Stderr, "FAIL: the telemetry fake recorded %d request(s): %v\n", len(seen), seen)
		code = 1
	}
	os.Exit(code)
}
