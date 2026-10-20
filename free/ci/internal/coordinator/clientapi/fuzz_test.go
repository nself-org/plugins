package clientapi

// Purpose: fuzz targets for the parts of the user API that read untrusted bytes: the submit body and the
// run reference in a path (P7-NODE-19, CRITICAL tier: parsers of untrusted input).
// Inputs:  random bodies and references. Outputs: pass or fail; seeds run in every `go test`.
// Constraints: no panic, no 5xx for any input a client can send, and a submit that is accepted carries a
//          project, a revision and a pipeline that pass the coordinator's own input rules.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func fuzzServer(f *testing.F) (*Server, *fakeBackend) {
	st, err := store.Open(filepath.Join(f.TempDir(), "state.db"), store.Options{})
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { _ = st.Close() })
	be := &fakeBackend{runs: []RunItem{item("run-a", "acme/app", "passed")}}
	svc := roles.New(st, &audit.Log{St: st})
	return New(Config{Backend: be, Roles: svc, Local: true, Operator: "cli:fuzz"}), be
}

func FuzzSubmitBody(f *testing.F) {
	for _, s := range []string{
		`{"project":"acme/app","revision":"` + goodRev + `"}`, `{"project":"acme/app","revision":"` + goodRev + `","pipeline":"release"}`,
		`{"project":"-x/y","revision":"` + goodRev + `"}`, `{"project":"a/b","revision":"--upload-pack=x"}`, `[]`, `null`, `{}`, `{"project":1}`,
		`{"project":"acme/app","revision":"` + goodRev + `"} {"x":1}`, "\x00\xff", strings.Repeat("{", 5000),
	} {
		f.Add(s)
	}
	srv, be := fuzzServer(f)
	h := srv.Handler()
	f.Fuzz(func(t *testing.T, body string) {
		before := len(be.submits)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", PathRuns, strings.NewReader(body))
		h.ServeHTTP(w, req)
		if w.Code >= 500 {
			t.Fatalf("status %d for body %q", w.Code, body)
		}
		if got := len(be.submits); got > before {
			r := be.submits[got-1]
			if !coordinator.ValidProject(r.Project) || !coordinator.ValidRevision(r.Revision) || (r.Pipeline != "" && !coordinator.ValidPipeline(r.Pipeline)) {
				t.Fatalf("an invalid submit reached the backend: %+v from %q", r, body)
			}
		}
	})
}

func FuzzRunReference(f *testing.F) {
	for _, s := range []string{"run-a", "latest", "in-0001", "../../etc/passwd", "run-a/../x", "%00", strings.Repeat("a", 500), "run a", "run-a?x=1", goodRev} {
		f.Add(s)
	}
	srv, _ := fuzzServer(f)
	h := srv.Handler()
	f.Fuzz(func(t *testing.T, ref string) {
		for _, path := range []string{PathRunsPfx + url.PathEscape(ref), PathRunsPfx + url.PathEscape(ref) + "/events?from=1", PathLogsPfx + url.PathEscape(ref), PathEvidence + url.PathEscape(ref)} {
			w := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(context.Background(), "GET", "http://x"+path, nil)
			if err != nil {
				continue
			}
			ctx, cancel := context.WithCancel(req.Context())
			if strings.Contains(path, "/events") {
				cancel() // a stream for a run that exists would run for hours; the fuzzer only needs the parse and auth path
			}
			h.ServeHTTP(w, req.WithContext(ctx))
			cancel()
			if w.Code >= 500 {
				t.Fatalf("status %d for %q", w.Code, path)
			}
		}
	})
}
