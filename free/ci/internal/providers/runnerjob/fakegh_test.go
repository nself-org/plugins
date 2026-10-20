//go:build integration

package runnerjob_test

// Purpose: (a copy of github/demand's fakegh_test.go, kept in step by hand) a fake GitHub REST API on a local TLS server for the demand tests: generate-jitconfig, the
// runner read, list and delete. It records every minted body so a test can check the labels, the group
// and the name; an injected runner can be older than the orphan cutoff.
// Inputs:  synthetic runner names and ids.
// Outputs: a Registrar (jit.New) over the fake; helpers to read what the forge holds.
// Constraints: no real network; every answer comes from memory.

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

	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

type fakeRunner struct {
	ID   int64
	Name string
	Busy bool
}

type fakeGH struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	runners []fakeRunner
	nextID  int64
	minted  []map[string]any
	calls   []string
	keepDel bool // DELETE answers 204 but the runner stays
}

func newFakeGH(t *testing.T) *fakeGH {
	f := &fakeGH{t: t, nextID: 100}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGH) reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func idOf(p string) int64 {
	n, _ := strconv.ParseInt(p[strings.LastIndex(p, "/")+1:], 10, 64)
	return n
}

func (f *fakeGH) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := r.URL.Path
	f.calls = append(f.calls, r.Method+" "+p)
	switch {
	case r.Method == "POST" && strings.HasSuffix(p, "/actions/runners/generate-jitconfig"):
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.minted = append(f.minted, body)
		f.nextID++
		f.runners = append(f.runners, fakeRunner{ID: f.nextID, Name: body["name"].(string)})
		f.reply(w, 201, map[string]any{"runner": map[string]any{"id": f.nextID, "name": body["name"]}, "encoded_jit_config": "synthetic-jitconfig-" + strconv.FormatInt(f.nextID, 10)})
	case r.Method == "GET" && strings.HasSuffix(p, "/actions/runners"):
		rows := []map[string]any{}
		for _, x := range f.runners {
			rows = append(rows, map[string]any{"id": x.ID, "name": x.Name, "busy": x.Busy})
		}
		f.reply(w, 200, map[string]any{"total_count": len(f.runners), "runners": rows})
	case r.Method == "GET" && strings.Contains(p, "/actions/runners/"):
		for _, x := range f.runners {
			if x.ID == idOf(p) {
				f.reply(w, 200, map[string]any{"id": x.ID, "name": x.Name, "busy": x.Busy})
				return
			}
		}
		f.reply(w, 404, map[string]string{"message": "Not Found"})
	case r.Method == "DELETE" && strings.Contains(p, "/actions/runners/"):
		for i, x := range f.runners {
			if x.ID == idOf(p) {
				if !f.keepDel {
					f.runners = append(f.runners[:i], f.runners[i+1:]...)
				}
				w.WriteHeader(204)
				return
			}
		}
		f.reply(w, 404, map[string]string{"message": "Not Found"})
	default:
		f.reply(w, 404, map[string]string{"message": "unexpected " + r.Method + " " + p})
	}
}

// registrar is a JIT registrar over the fake with the given clock (the runner name carries its birth).
func (f *fakeGH) registrar(t *testing.T, set jit.Settings, now func() time.Time) *jit.Registrar {
	t.Helper()
	c := &rest.Client{BaseURL: f.srv.URL, AllowPrivateBase: true, APIVersion: "2022-11-28", UserAgent: "demand-test",
		TLSConfig: f.srv.Client().Transport.(*http.Transport).TLSClientConfig,
		Auth:      func(context.Context) (string, error) { return "synthetic-test-token-0001", nil }}
	r, err := jit.New(jit.Config{Client: c, Settings: set, Clock: now})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *fakeGH) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.runners) }

func (f *fakeGH) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.runners {
		out = append(out, r.Name)
	}
	return out
}
