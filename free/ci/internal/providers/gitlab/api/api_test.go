package api_test

// Purpose: tests of the shared GitLab helpers of P7-HOST-06: project path parsing and encoding, version
// reading, the paginated listing and the job archive lookup.
// Inputs:  a TLS fake of a list route.
// Outputs: pass or fail.
// Constraints: no live network.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

func TestParseProject(t *testing.T) {
	for in, ok := range map[string]bool{"acme/widgets": true, "acme/sub/widgets": true, "a.b/c_d-e": true, "widgets": false, "": false, "acme/": false,
		"/widgets": false, "acme/../x": false, "acme/./x": false, "acme/wid gets": false, "acme/wid?gets": false, "acme/w%2Fx": false,
		strings.Repeat("a/", 20) + "b": false} {
		if _, got := api.ParseProject(in); got != ok {
			t.Errorf("ParseProject(%q) = %v, want %v", in, got, ok)
		}
	}
	p, _ := api.ParseProject("acme/sub/widgets")
	if m := p.With("pipeline", "7"); m["id"] != "acme/sub/widgets" || m["pipeline"] != "7" || len(p.Params()) != 1 {
		t.Errorf("params: %v %v", m, p.Params())
	}
}

func TestVersions(t *testing.T) {
	for v, want := range map[string]bool{"17.10.0": true, "17.10.0-ee": true, "17.9.9": false, "18.0.0": true, "18.2.1-ee": true, "16.11.2": false, "17.10": true,
		"": false, "banana": false, "17": false, "17.x": false, "-1.2": false} {
		if got := api.SupportsInputs(v); got != want {
			t.Errorf("SupportsInputs(%q) = %v, want %v", v, got, want)
		}
	}
	if major, minor, ok := api.ParseVersion("18.4.1-ee+abc"); !ok || major != 18 || minor != 4 {
		t.Errorf("ParseVersion: %d %d %v", major, minor, ok)
	}
}

func TestLinksAreEncodedAndCarryNoToken(t *testing.T) {
	p, _ := api.ParseProject("acme/sub/widgets")
	if got := api.ArtifactsURL("https://gitlab.example/api/v4/", p, 12); got != "https://gitlab.example/api/v4/projects/acme%2Fsub%2Fwidgets/jobs/12/artifacts" {
		t.Errorf("artifacts url %q", got)
	}
	if got := api.RawFileURL("https://gitlab.example/api/v4", p, ".gitlab/nself-ci.gitlab-ci.yml", "abc123"); got !=
		"https://gitlab.example/api/v4/projects/acme%2Fsub%2Fwidgets/repository/files/.gitlab%2Fnself-ci.gitlab-ci.yml/raw?ref=abc123" {
		t.Errorf("raw url %q", got)
	}
	j := api.Job{Artifacts: []api.ArtifactFile{{FileType: "metadata", Size: 5}, {FileType: "archive", Size: 99}}}
	if size, ok := j.ArchiveSize(); !ok || size != 99 {
		t.Errorf("archive size %d %v", size, ok)
	}
	if _, ok := (api.Job{}).ArchiveSize(); ok {
		t.Error("an empty job has an archive")
	}
}

type item struct {
	ID int `json:"id"`
}

// TestListPages proves List follows the pages until a short page, stops at MaxPages, passes the
// caller's filters on every page and surfaces an error from any page.
func TestListPages(t *testing.T) {
	var pages []string
	total := api.PageSize*2 + 7
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.RawQuery)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if r.URL.Query().Get("fail") != "" && page == 2 {
			w.WriteHeader(500)
			return
		}
		var out []string
		for i := (page - 1) * api.PageSize; i < total && i < page*api.PageSize; i++ {
			out = append(out, `{"id":`+strconv.Itoa(i)+`}`)
		}
		_, _ = w.Write([]byte("[" + strings.Join(out, ",") + "]"))
	}))
	defer srv.Close()
	c := &rest.Client{BaseURL: srv.URL, AllowPrivateBase: true, TLSConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig,
		Auth: func(context.Context) (string, error) { return "synthetic", nil }}
	got, err := api.List[item](context.Background(), c, "/things", nil, map[string][]string{"ref": {"main"}})
	if err != nil || len(got) != total || got[total-1].ID != total-1 {
		t.Fatalf("list: %d items, %v", len(got), err)
	}
	if len(pages) != 3 || !strings.Contains(pages[0], "ref=main") || !strings.Contains(pages[2], "page=3") {
		t.Errorf("pages requested: %v", pages)
	}
	total = api.PageSize * 10
	pages = nil
	if got, err = api.List[item](context.Background(), c, "/things", nil, nil); err != nil || len(got) != api.PageSize*api.MaxPages || len(pages) != api.MaxPages {
		t.Errorf("max pages: %d items %d requests %v", len(got), len(pages), err)
	}
	_, err = api.List[item](context.Background(), c, "/things", nil, map[string][]string{"fail": {"1"}})
	if providers.ClassOf(err) != providers.Transient {
		t.Errorf("a failing page: %v", err)
	}
}

func TestErrorHelpers(t *testing.T) {
	if !api.IsTransient(api.Unavailable("x")) || api.IsTransient(api.Refuse("x")) || !api.IsCredentialMissing(api.Failure(providers.Config, "E700", "x")) {
		t.Error("classification")
	}
	for err, code := range map[error]string{api.Refuse("a"): "E708", api.Misconfig("a"): "E702", api.NotEvidence("a"): "E705", api.Unavailable("a"): "E704"} {
		if pe, ok := err.(*providers.Error); !ok || pe.Code != code {
			t.Errorf("%v is not %s", err, code)
		}
	}
}
