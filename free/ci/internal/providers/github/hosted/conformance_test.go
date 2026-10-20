package hosted_test

// Purpose: TestConformance of P7-HOST-03: the provider passes conformance.Run on recorded-shape
// fixtures (dispatch 200 with workflow_run_id, run states, cancel including 409, contents API,
// artifact list and the zip download).
// Inputs:  an ordered exchange list written to a temp directory (the bodies that depend on the
//          template, the workflow bytes and the evidence zip, are generated so a template change
//          never leaves a stale fixture).
// Outputs: pass or fail.
// Constraints: no live network; the kit's replay server speaks plain HTTP and the funnel refuses it,
//          so a TLS front relays to it; the artifact download is a Config.Download that reads the
//          fixture through the same front (the redirect rules are the funnel's own tests'); the zip
//          travels base64 inside the JSON fixture.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/conformance"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

const fixtureNonce = "00000000000000000000000000000001"

func runBody(status string) string {
	b, _ := json.Marshal(map[string]any{"id": 9001, "status": status, "event": "workflow_dispatch", "path": hosted.WorkflowPath, "head_branch": "main",
		"head_sha": testHeadSHA, "display_title": "nself-ci " + fixtureNonce, "run_attempt": 1, "actor": map[string]any{"login": testLogin}})
	return string(b)
}

// writeFixtures writes the ordered exchanges of one kit run.
func writeFixtures(t *testing.T, listing bool) string {
	t.Helper()
	dir := t.TempDir()
	run := "/repos/acme/widgets/actions/runs/9001"
	workflow := base64.StdEncoding.EncodeToString(pinnedWorkflow(t))
	zipB64 := base64.StdEncoding.EncodeToString(evidenceZip(t, "attempt-1", testRevision, nil))
	artifacts, _ := json.Marshal(map[string]any{"artifacts": []map[string]any{{"id": 4001, "name": hosted.EvidenceArtifact(fixtureNonce), "size_in_bytes": 1200, "expired": false}}})
	contents, _ := json.Marshal(map[string]any{"encoding": "base64", "content": workflow})
	dispatch := []conformance.Exchange{
		{Request: conformance.Request{Method: "GET", Path: "/repos/acme/widgets"}, Response: conformance.Response{Status: 200, Body: `{"default_branch":"main","private":false,"visibility":"public"}`}},
		{Request: conformance.Request{Method: "POST", Path: "/repos/acme/widgets/actions/workflows/nself-ci-dispatch.yml/dispatches"}, Response: conformance.Response{Status: 200, Body: `{"workflow_run_id":9001,"run_url":"https://api.github.com/repos/acme/widgets/actions/runs/9001","html_url":"https://github.com/acme/widgets/actions/runs/9001"}`}},
	}
	if listing { // 204: the run is found by nonce and actor in the listing
		list, _ := json.Marshal(map[string]any{"workflow_runs": []json.RawMessage{json.RawMessage(runBody("queued"))}})
		dispatch[1].Response = conformance.Response{Status: 204}
		dispatch = append(dispatch,
			conformance.Exchange{Request: conformance.Request{Method: "GET", Path: "/user"}, Response: conformance.Response{Status: 200, Body: `{"login":"` + testLogin + `"}`}},
			conformance.Exchange{Request: conformance.Request{Method: "GET", Path: "/repos/acme/widgets/actions/workflows/nself-ci-dispatch.yml/runs",
				Query: "created=2025-12-31T23%3A58%3A00Z..2026-01-01T00%3A02%3A00Z&event=workflow_dispatch&page=1&per_page=100"}, Response: conformance.Response{Status: 200, Body: string(list)}})
	}
	exchanges := append(dispatch, []conformance.Exchange{
		{Request: conformance.Request{Method: "GET", Path: run}, Response: conformance.Response{Status: 200, Body: runBody("queued")}},
		{Request: conformance.Request{Method: "GET", Path: run}, Response: conformance.Response{Status: 200, Body: runBody("in_progress")}},
		{Request: conformance.Request{Method: "GET", Path: run}, Response: conformance.Response{Status: 200, Body: runBody("completed")}},
		{Request: conformance.Request{Method: "GET", Path: run}, Response: conformance.Response{Status: 404, Body: `{"message":"Not Found"}`}},
		{Request: conformance.Request{Method: "POST", Path: run + "/cancel"}, Response: conformance.Response{Status: 202, Body: `{}`}},
		{Request: conformance.Request{Method: "POST", Path: run + "/cancel"}, Response: conformance.Response{Status: 409, Body: `{"message":"Cannot cancel a workflow run that is completed."}`}},
		{Request: conformance.Request{Method: "GET", Path: run}, Response: conformance.Response{Status: 200, Body: runBody("completed")}},
	}...)
	if !listing { // the 204 variant already asked who the token is
		exchanges = append(exchanges, conformance.Exchange{Request: conformance.Request{Method: "GET", Path: "/user"}, Response: conformance.Response{Status: 200, Body: `{"login":"` + testLogin + `"}`}})
	}
	exchanges = append(exchanges, []conformance.Exchange{
		{Request: conformance.Request{Method: "GET", Path: "/repos/acme/widgets/contents/" + hosted.WorkflowPath, Query: "ref=" + testHeadSHA}, Response: conformance.Response{Status: 200, Body: string(contents)}},
		{Request: conformance.Request{Method: "GET", Path: run + "/artifacts", Query: "per_page=100"}, Response: conformance.Response{Status: 200, Body: string(artifacts)}},
		{Request: conformance.Request{Method: "GET", Path: "/repos/acme/widgets/actions/artifacts/4001/zip"}, Response: conformance.Response{Status: 200, Body: zipB64}},
	}...)
	for i, x := range exchanges {
		raw, err := json.MarshalIndent(x, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d.json", i+1)), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestConformance proves the GitHub-hosted Dispatcher passes conformance.Run on recorded fixtures:
// dispatch (200 with workflow_run_id), the four run states, cancel including 409, the contents API,
// the artifact list and the zip download, with no token in a log, an error or a request.
func TestConformance(t *testing.T) {
	v15(t)
	for _, listing := range []bool{false, true} {
		t.Run(map[bool]string{false: "dispatch 200", true: "dispatch 204 and listing"}[listing], func(t *testing.T) { runConformance(t, listing) })
	}
}

func runConformance(t *testing.T, listing bool) {
	conformance.Run(t, conformance.Subject{
		Name: "github-hosted", Fixtures: writeFixtures(t, listing), Secrets: [][]byte{[]byte(testToken)},
		Attempt: providers.Attempt{ID: "attempt-1", N: 2, Project: "widgets", Repo: "acme/widgets", Revision: testRevision, Trust: "owner",
			Label: "ubuntu-24.04", CompatMode: "v1.5", Spec: []byte(`{"kind":"test","run":["true"]}`), TimeoutMs: 20 * 60 * 1000},
		New: func(base string, clk conformance.Clock) providers.Provider {
			u, err := url.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			front := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(u))
			t.Cleanup(front.Close)
			client := &rest.Client{BaseURL: front.URL, AllowPrivateBase: true, APIVersion: "2022-11-28", UserAgent: "hosted-test",
				TLSConfig: front.Client().Transport.(*http.Transport).TLSClientConfig, Auth: func(context.Context) (string, error) { return testToken, nil }}
			return hosted.New(hosted.Config{Client: func(context.Context) (*rest.Client, error) { return client, nil },
				Settings: func(context.Context) (hosted.Settings, error) { return hosted.DefaultSettings(), nil },
				Clock:    clk.Now, Nonce: func() (string, error) { return fixtureNonce, nil },
				Download: func(ctx context.Context, _ *rest.Client, link string, max int64, w io.Writer) error {
					req, _ := http.NewRequestWithContext(ctx, "GET", link, nil)
					resp, err := front.Client().Do(req)
					if err != nil {
						return err
					}
					defer func() { _ = resp.Body.Close() }()
					b64, _ := io.ReadAll(io.LimitReader(resp.Body, max*2))
					raw, err := base64.StdEncoding.DecodeString(string(b64))
					if err != nil {
						return err
					}
					_, err = w.Write(raw)
					return err
				}})
		},
	})
}
