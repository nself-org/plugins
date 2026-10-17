package main

// json_harness_test.go: in-process harness for the --json contract tests.
//
// Purpose: run the real command tree against a mocked ɳSentry API and a mocked
//   status page, capturing the real os.Stdout and os.Stderr streams and the
//   exit status, so goldens pin both the v1.4 bytes and the v1.5 envelopes.
// Inputs: the global command tree (sentryCmd), httptest servers.
// Outputs: a run result (stdout, stderr, exit status) per invocation.
// Constraints: no network; every key, URL and id is a synthetic fixture. Flag
//   values are reset after each run because the commands are package globals.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/nself-sentry-cli/cmd/jsonout"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// fixtureKey is built at run time so no key-shaped literal sits in the source.
var fixtureKey = "nsk_" + strings.Repeat("a", 12)

// fixtureAPI serves a minimal ɳSentry API plus two status pages. Any request
// that does not carry the fixture key gets 401. A monitor id of "boom" gets 500.
func fixtureAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	reply := func(code int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+fixtureKey {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"bad key"}}`))
				return
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}
	}
	const mon = `{"id":"m1","name":"api","url":"https://api.example.test","kind":"http","interval_seconds":60,"status":"up","paused":false,"created_at":"2026-01-02T03:04:05Z"}`
	const inc = `{"id":"i1","monitor_id":"m1","title":"api down","status":"open","severity":"high","started_at":"2026-01-02T03:04:05Z"}`
	const page = `{"id":"p1","name":"Public","slug":"public","url":"https://status.example.test/s/public","public":true,"created_at":"2026-01-02T03:04:05Z"}`
	mux.HandleFunc("GET /v1/me", reply(200, `{"tenant_id":"t-fixture","email":"dev@example.test","tier":"bundle","quotas":{"monitors":{"used":1,"limit":50},"status_pages":{"used":0,"limit":3}}}`))
	mux.HandleFunc("GET /v1/monitors", reply(200, `{"monitors":[`+mon+`]}`))
	mux.HandleFunc("POST /v1/monitors", reply(201, `{"monitor":`+mon+`}`))
	mux.HandleFunc("DELETE /v1/monitors/boom", reply(500, `{"error":{"message":"database exploded"}}`))
	mux.HandleFunc("DELETE /v1/monitors/{id}", reply(204, ``))
	mux.HandleFunc("POST /v1/monitors/{id}/pause", reply(200, `{"monitor":`+strings.Replace(mon, `"paused":false`, `"paused":true`, 1)+`}`))
	mux.HandleFunc("POST /v1/monitors/{id}/resume", reply(200, `{"monitor":`+mon+`}`))
	mux.HandleFunc("GET /v1/incidents", reply(200, `{"incidents":[`+inc+`]}`))
	mux.HandleFunc("POST /v1/incidents/{id}/ack", reply(200, `{"incident":`+strings.Replace(inc, `"open"`, `"acknowledged"`, 1)+`}`))
	mux.HandleFunc("POST /v1/incidents/{id}/resolve", reply(200, `{"incident":`+strings.Replace(inc, `"open"`, `"resolved"`, 1)+`}`))
	mux.HandleFunc("GET /v1/status-pages", reply(200, `{"status_pages":[`+page+`]}`))
	mux.HandleFunc("POST /v1/status-pages", reply(201, `{"status_page":`+page+`}`))
	mux.HandleFunc("GET /v1/alerts/channels", reply(200, `{"channels":[{"id":"c1","kind":"email","target":"ops@example.test","enabled":true}]}`))
	mux.HandleFunc("POST /v1/alerts/channels/{id}/test", reply(200, `{"delivered":true,"detail":"queued"}`))
	mux.HandleFunc("GET /ok.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"site_name":"Fixture Status","overall_status":"operational","components":[{"id":"c1","name":"API","status":"operational","updated_at":"2026-01-02T03:04:05Z"}]}`))
	})
	mux.HandleFunc("GET /degraded.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"site_name":"Fixture Status","overall_status":"degraded_performance","components":[{"id":"c1","name":"API","status":"operational"},{"id":"c2","name":"DB","status":"partial_outage"},{"id":"c3","name":"Auth","status":"degraded_performance"}]}`))
	})
	mux.HandleFunc("GET /down.json", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// runResult is what one invocation produced.
type runResult struct {
	stdout, stderr string
	code           int
}

// captureRun executes the command tree with args and captures the real
// os.Stdout and os.Stderr into temp files (no pipe, so no deadlock on size).
func captureRun(t *testing.T, args []string) runResult {
	t.Helper()
	dir := t.TempDir()
	so, err := os.Create(dir + "/stdout")
	if err != nil {
		t.Fatal(err)
	}
	se, err := os.Create(dir + "/stderr")
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = so, se
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()

	setup()
	code := jsonout.Execute(sentryCmd, args)
	os.Stdout, os.Stderr = oldOut, oldErr
	_ = so.Close()
	_ = se.Close()
	ob, _ := os.ReadFile(dir + "/stdout")
	eb, _ := os.ReadFile(dir + "/stderr")
	resetFlags(sentryCmd)
	return runResult{stdout: string(ob), stderr: string(eb), code: code}
}

// resetFlags puts every flag in the tree back to its default.
func resetFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	for _, s := range c.Commands() {
		resetFlags(s)
	}
}
