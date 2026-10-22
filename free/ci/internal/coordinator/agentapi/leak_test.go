package agentapi

// Purpose: TestShutdownLeavesNoServerGoroutines: after Shutdown no goroutine of the server (guard refresh,
//          stream watchers, handlers) is left, even with a long-poll, a control stream and an upstream
//          stream open.
// Inputs:  the rig. Outputs: pass or fail; a failure prints the stacks that remain.
// Constraints: the check looks for this package's functions in the goroutine dump, not at a count.

import (
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

func serverStacks() string {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	var out []string
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "internal/coordinator/agentapi.(*Server)") && !strings.Contains(g, "testing.tRunner") {
			out = append(out, g)
		}
	}
	return strings.Join(out, "\n\n")
}

func TestShutdownLeavesNoServerGoroutines(t *testing.T) {
	t.Run("serve", func(t *testing.T) {
		r := newRig(t, nil)
		a := r.newAgent("n-1")
		a.connect()
		pr, pw := io.Pipe()
		t.Cleanup(func() { _ = pw.Close() })
		for _, rq := range []*http.Request{
			a.req(http.MethodGet, "/v1/agent/leases", nil, map[string]string{HeaderWait: "30"}),
			a.req(http.MethodGet, "/v1/agent/control", nil, nil),
			a.req(http.MethodPost, "/v1/agent/stream", pr, nil),
		} {
			go func(rq *http.Request) {
				if resp, err := a.cl.Do(rq); err == nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
			}(rq)
		}
		eventually(t, 3*time.Second, "three live requests", func() bool { return liveRequests(r.srv, "n-1") == 3 })
	})
	var stacks string
	eventually(t, 3*time.Second, "server goroutines to end", func() bool { stacks = serverStacks(); return stacks == "" })
	if stacks != "" {
		t.Fatalf("goroutines left after Shutdown:\n%s", stacks)
	}
}
