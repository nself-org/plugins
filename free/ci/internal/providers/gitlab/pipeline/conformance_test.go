package pipeline_test

// Purpose: TestConformance of P7-HOST-06: the GitLab Dispatcher passes conformance.Run on recorded-shape
// fixtures for gitlab.com (a current version that takes pipeline inputs) and for a self-managed base URL
// (an older version that gets variables): pipeline create, status, cancel including an already
// finished pipeline, jobs list, artifact download and the repository file raw at the pipeline sha.
// Inputs:  an ordered exchange list written to a temp directory (the bodies that depend on the template
//          and the evidence archive are generated so a template change never leaves a stale fixture).
// Outputs: pass or fail.
// Constraints: no live network; the kit's replay server speaks plain HTTP and the funnel refuses it, so
//          a TLS front relays to it; downloads read the fixture through the same front; the archive
//          travels base64 inside the JSON fixture. Replay compares the decoded request path, so the
//          project appears as acme/widgets in the paths below.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/conformance"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/pipeline"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

const (
	fixtureNonce = "00000000000000000000000000000001"
	fixtureSpec  = `{"kind":"test","run":["true"]}`
)

type variant struct {
	name, version string
	prefix        string // the API root: gitlab.com is /api/v4, a self-managed instance under a relative URL root is /gitlab/api/v4
	inputs        bool   // the instance takes pipeline inputs and the main file declares them
	listing       bool   // the create answers 502 and the pipeline is found by nonce
}

func pipeBody(status string, inputs bool) string {
	name := ""
	if inputs {
		name = "nself-ci " + fixtureNonce
	}
	b, _ := json.Marshal(map[string]any{"id": 9001, "status": status, "source": "api", "ref": "main", "sha": testHeadSHA, "name": name,
		"user": map[string]any{"username": testLogin}, "created_at": "2026-01-01T00:00:00Z"})
	return string(b)
}

func x(method, path, query string, status int, body string) conformance.Exchange {
	return conformance.Exchange{Request: conformance.Request{Method: method, Path: path, Query: query}, Response: conformance.Response{Status: status, Body: body}}
}

func writeFixtures(t *testing.T, v variant) string {
	t.Helper()
	dir := t.TempDir()
	project := v.prefix + "/projects/acme/widgets"
	sum := sha256.Sum256([]byte(fixtureSpec))
	vars, _ := json.Marshal([]map[string]string{{"key": "NSELF_CI_NONCE", "value": fixtureNonce}, {"key": "NSELF_CI_SPEC_DIGEST", "value": "sha256:" + hex.EncodeToString(sum[:])}})
	zipB64 := base64.StdEncoding.EncodeToString(evidenceZip(t, "attempt-1", testRevision, nil))
	jobs, _ := json.Marshal([]map[string]any{{"id": 5001, "name": "nself-ci", "status": "success", "artifacts": []map[string]any{{"file_type": "archive", "size": 1200}, {"file_type": "metadata", "size": 40}}}})
	lint, _ := json.Marshal(map[string]any{"valid": true, "merged_yaml": mergedFrom(t, pinnedInclude(t), nil)})
	pipe := project + "/pipelines/9001"
	ex := []conformance.Exchange{
		x("GET", project, "", 200, `{"id":77,"default_branch":"main","visibility":"public"}`),
		x("GET", v.prefix+"/version", "", 200, fmt.Sprintf(`{"version":%q,"revision":"abcdef0"}`, v.version)),
	}
	if v.inputs { // 17.10 and later: the main file at the ref is read to see whether it declares the inputs
		ex = append(ex, x("GET", project+"/repository/files/.gitlab-ci.yml/raw", "ref=main", 200, string(rootFile())))
	}
	if !v.listing {
		ex = append(ex, x("POST", project+"/pipeline", "", 201, pipeBody("created", v.inputs)))
	} else {
		ex = append(ex,
			x("POST", project+"/pipeline", "", 502, ""),
			x("GET", v.prefix+"/user", "", 200, `{"username":"`+testLogin+`"}`),
			x("GET", project+"/pipelines", "order_by=id&page=1&per_page=100&ref=main&sort=desc&source=api&updated_after=2025-12-31T23%3A58%3A00Z&updated_before=2026-01-01T00%3A02%3A00Z", 200,
				`[{"id":9001,"status":"created","source":"api","ref":"main","sha":"`+testHeadSHA+`"}]`),
			x("GET", pipe, "", 200, pipeBody("created", v.inputs)))
		if !v.inputs { // no workflow:name on the older instance: the nonce is read from the variables
			ex = append(ex, x("GET", pipe+"/variables", "", 200, string(vars)))
		}
	}
	for _, st := range []string{"pending", "running", "success"} {
		ex = append(ex, x("GET", pipe, "", 200, pipeBody(st, v.inputs)))
	}
	ex = append(ex, x("GET", pipe, "", 404, `{"message":"404 Not found"}`),
		x("POST", pipe+"/cancel", "", 200, pipeBody("canceled", v.inputs)),
		x("POST", pipe+"/cancel", "", 200, pipeBody("success", v.inputs)), // already finished: still success
		x("GET", pipe, "", 200, pipeBody("success", v.inputs)))
	if !v.listing {
		ex = append(ex, x("GET", v.prefix+"/user", "", 200, `{"username":"`+testLogin+`"}`))
	}
	ex = append(ex, x("GET", pipe+"/jobs", "include_retried=true&page=1&per_page=100", 200, string(jobs)))
	if !v.inputs {
		ex = append(ex, x("GET", pipe+"/variables", "", 200, string(vars)))
	}
	ex = append(ex,
		x("GET", project+"/repository/files/"+gitlabtmpl.Path+"/raw", "ref="+testHeadSHA, 200, string(pinnedInclude(t))),
		x("GET", project+"/jobs/5001/artifacts", "", 200, zipB64),
		x("GET", project+"/ci/lint", "content_ref="+testHeadSHA, 200, string(lint)))
	for i, e := range ex {
		raw, err := json.MarshalIndent(e, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d.json", i+1)), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestConformance proves acceptance line 1 of P7-HOST-06: the GitLab Dispatcher passes conformance.Run on
// fixtures for gitlab.com and a self-managed base URL (pipeline create, status, cancel incl. already
// finished, jobs list, artifact download, repository file raw at sha), in both transports and with the
// created-then-502 path that finds the pipeline by nonce.
func TestConformance(t *testing.T) {
	v15(t)
	for _, v := range []variant{
		{"gitlab.com inputs", "18.2.1-ee", "/api/v4", true, false},
		{"gitlab.com inputs, create then 502", "18.2.1-ee", "/api/v4", true, true},
		{"self-managed variables", "15.11.13-ee", "/gitlab/api/v4", false, false},
		{"self-managed variables, create then 502", "15.11.13-ee", "/gitlab/api/v4", false, true},
	} {
		t.Run(v.name, func(t *testing.T) { runConformance(t, v) })
	}
}

func runConformance(t *testing.T, v variant) {
	conformance.Run(t, conformance.Subject{
		Name: "gitlab", Fixtures: writeFixtures(t, v), Secrets: [][]byte{[]byte(testToken)},
		Attempt: providers.Attempt{ID: "attempt-1", N: 2, Project: "widgets", Repo: "acme/widgets", Revision: testRevision, Trust: "owner",
			Label: "saas-linux-small-amd64", CompatMode: "v1.5", Spec: []byte(fixtureSpec), TimeoutMs: 20 * 60 * 1000},
		New: func(base string, clk conformance.Clock) providers.Provider {
			u, err := url.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			front := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(u))
			t.Cleanup(front.Close)
			client := &rest.Client{BaseURL: front.URL + v.prefix, AllowPrivateBase: true, UserAgent: "pipeline-test", Provider: "gitlab",
				TLSConfig: front.Client().Transport.(*http.Transport).TLSClientConfig, Auth: func(context.Context) (string, error) { return testToken, nil }}
			return pipeline.New(pipeline.Config{Client: func(context.Context) (*rest.Client, error) { return client, nil },
				Clock: clk.Now, Nonce: func() (string, error) { return fixtureNonce, nil },
				Download: func(ctx context.Context, _ *rest.Client, link string, max int64, w io.Writer) error {
					req, _ := http.NewRequestWithContext(ctx, "GET", link, nil)
					resp, err := front.Client().Do(req)
					if err != nil {
						return err
					}
					defer func() { _ = resp.Body.Close() }()
					data, _ := io.ReadAll(io.LimitReader(resp.Body, max*2))
					if strings.HasSuffix(strings.SplitN(link, "?", 2)[0], "/artifacts") { // the archive travels base64
						if data, err = base64.StdEncoding.DecodeString(string(data)); err != nil {
							return err
						}
					}
					_, err = w.Write(data)
					return err
				}})
		},
	})
}
