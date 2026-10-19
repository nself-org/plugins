package jit_test

// Purpose: shared fixtures for the jit tests: a fake GitHub API on a local TLS server, a Registrar
// wired to it, and the adapter that lets the conformance kit (which sends empty scopes) reach it.
// Inputs:  synthetic values only (account acme, repository widgets, tokens that exist nowhere).
// Outputs: helpers; no test lives here.
// Constraints: no real network; every request is answered from memory.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

const (
	testToken = "synthetic-test-token-0001"
	testScope = "acme/widgets"
)

var (
	epoch    = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	repoOnly = providers.RunnerScope{Repo: testScope}
)

type runnerRow struct {
	ID   int64
	Name string
	Busy bool
}

// fakeGH is the slice of the GitHub REST API the registrar uses.
type fakeGH struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	runners  []runnerRow
	nextID   int64
	minted   []map[string]any // decoded generate-jitconfig bodies
	calls    []string         // "METHOD path"
	deny403  bool             // generate-jitconfig answers 403 naming admin:org
	keepDel  bool             // DELETE answers 204 but the runner stays listed
	group    map[string]any   // runner group reply
	protect  map[string]bool  // "owner/repo@branch" -> protected
	approval string

	groupStatus, protectStatus int // when non-zero, the group read / branch protection read answers this status
	protectCalls               int
	raceDelete                 bool   // DELETE removes the runner but answers 404 (it went away on its own)
	listBody                   string // when set, the runner list answers this raw 200 body
}

func newFakeGH(t *testing.T) *fakeGH {
	f := &fakeGH{t: t, nextID: 100, protect: map[string]bool{}, approval: "first_time_contributors"}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGH) called(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeGH) reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func (f *fakeGH) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	p := r.URL.Path
	switch {
	case r.Method == "POST" && strings.HasSuffix(p, "/actions/runners/generate-jitconfig"):
		if f.deny403 {
			w.Header().Set("X-Accepted-OAuth-Scopes", "admin:org")
			w.Header().Set("X-OAuth-Scopes", "repo")
			f.reply(w, 403, map[string]string{"message": "Resource not accessible"})
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.minted = append(f.minted, body)
		f.nextID++
		f.runners = append(f.runners, runnerRow{ID: f.nextID, Name: body["name"].(string)})
		f.reply(w, 201, map[string]any{"runner": map[string]any{"id": f.nextID, "name": body["name"]}, "encoded_jit_config": "synthetic-jit-config-" + strconv.FormatInt(f.nextID, 10)})
	case r.Method == "GET" && strings.HasSuffix(p, "/actions/runners") && f.listBody != "":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(f.listBody))
	case r.Method == "GET" && strings.Contains(p, "/actions/runners/"):
		id, _ := strconv.ParseInt(p[strings.LastIndex(p, "/")+1:], 10, 64)
		for _, x := range f.runners {
			if x.ID == id {
				f.reply(w, 200, map[string]any{"id": x.ID, "name": x.Name, "busy": x.Busy})
				return
			}
		}
		f.reply(w, 404, map[string]string{"message": "Not Found"})
	case r.Method == "GET" && strings.HasSuffix(p, "/actions/runners"):
		size, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size = min(size, 100) // GitHub never returns more than 100 per page
		lo, hi := (page-1)*size, page*size
		if lo > len(f.runners) {
			lo = len(f.runners)
		}
		if hi > len(f.runners) {
			hi = len(f.runners)
		}
		rows := []map[string]any{}
		for _, x := range f.runners[lo:hi] {
			rows = append(rows, map[string]any{"id": x.ID, "name": x.Name, "busy": x.Busy})
		}
		f.reply(w, 200, map[string]any{"total_count": len(f.runners), "runners": rows})
	case r.Method == "DELETE" && strings.Contains(p, "/actions/runners/"):
		id, _ := strconv.ParseInt(p[strings.LastIndex(p, "/")+1:], 10, 64)
		for i, x := range f.runners {
			if x.ID == id {
				if !f.keepDel {
					f.runners = append(f.runners[:i], f.runners[i+1:]...)
				}
				if f.raceDelete {
					f.reply(w, 404, map[string]string{"message": "Not Found"})
					return
				}
				w.WriteHeader(204)
				return
			}
		}
		f.reply(w, 404, map[string]string{"message": "Not Found"})
	case r.Method == "GET" && strings.Contains(p, "/actions/runner-groups/"):
		if f.groupStatus != 0 {
			f.reply(w, f.groupStatus, map[string]string{"message": "injected"})
			return
		}
		if f.group == nil {
			f.reply(w, 404, map[string]string{"message": "Not Found"})
			return
		}
		f.reply(w, 200, f.group)
	case r.Method == "GET" && strings.Contains(p, "/branches/") && strings.HasSuffix(p, "/protection"):
		rest := strings.TrimSuffix(strings.TrimPrefix(p, "/repos/"), "/protection")
		f.protectCalls++
		if f.protectStatus != 0 {
			f.reply(w, f.protectStatus, map[string]string{"message": "injected"})
			return
		}
		owner, rest, _ := strings.Cut(rest, "/")
		repo, branch, _ := strings.Cut(rest, "/branches/")
		if f.protect[owner+"/"+repo+"@"+branch] {
			f.reply(w, 200, map[string]any{"url": "x"})
			return
		}
		f.reply(w, 404, map[string]string{"message": "Branch not protected"})
	case r.Method == "GET" && strings.HasSuffix(p, "/actions/permissions/fork-pr-contributor-approval"):
		f.reply(w, 200, map[string]string{"approval_policy": f.approval})
	default:
		f.reply(w, 404, map[string]string{"message": "unexpected " + r.Method + " " + p})
	}
}

func testClient(base string, tlsFrom *httptest.Server) *rest.Client {
	return &rest.Client{BaseURL: base, AllowPrivateBase: true, APIVersion: "2022-11-28", UserAgent: "jit-test",
		TLSConfig: tlsFrom.Client().Transport.(*http.Transport).TLSClientConfig,
		Auth:      func(context.Context) (string, error) { return testToken, nil }}
}

// registrar builds a Registrar over f with a pinned clock and a page size of 2.
func (f *fakeGH) registrar(set jit.Settings, sink jit.FactSink) *jit.Registrar {
	f.t.Helper()
	c := testClient(f.srv.URL, f.srv)
	r, err := jit.New(jit.Config{Client: c, Settings: set, Facts: sink, Clock: func() time.Time { return epoch }, PageSize: 2})
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}
