package webhook

// Purpose: shared test rig (P7-HOST-05): a fake GitHub API over TLS fed from recorded responses, a
// Source wired to it, signed-delivery builders and the golden file helper.
// Inputs:  testdata/*.json payloads, testdata/api/routes.json recorded API responses.
// Outputs: rig values the tests drive; goldens under testdata/golden (rewrite with -update).
// Constraints: no live network call (the fake server binds loopback and the client trusts only its
//          certificate); every secret here is an obviously synthetic string.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var (
	shaA = strings.Repeat("a", 40)
	shaB = strings.Repeat("b", 40)
	shaC = strings.Repeat("c", 40)
	shaD = strings.Repeat("d", 40)
	// shaHead is the head commit of the default branch in testdata/api/routes.json.
	shaHead = strings.Repeat("e", 40)

	fixedNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
)

const (
	repoSecret   = "synthetic-per-repo-secret"
	sharedSecret = "synthetic-shared-secret"
)

type route struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
	delay  time.Duration
}

// fakeGH serves recorded API responses by decoded request path.
type fakeGH struct {
	mu     sync.Mutex
	routes map[string]route
	hits   map[string]int
	paths  []string // escaped request paths, in order
	srv    *httptest.Server
}

func newFakeGH(t *testing.T) *fakeGH {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "api", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	g := &fakeGH{routes: map[string]route{}, hits: map[string]int{}}
	if err = json.Unmarshal(raw, &g.routes); err != nil {
		t.Fatal(err)
	}
	g.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		rt, ok := g.routes[r.URL.Path]
		g.hits[r.URL.Path]++
		g.paths = append(g.paths, r.URL.EscapedPath())
		g.mu.Unlock()
		if rt.delay > 0 {
			select {
			case <-time.After(rt.delay):
			case <-r.Context().Done():
				return
			}
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.Status)
		_, _ = w.Write(rt.Body)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGH) set(path string, status int, body string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.routes[path] = route{Status: status, Body: json.RawMessage(body)}
}

func (g *fakeGH) slow(path string, d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	rt := g.routes[path]
	rt.delay = d
	g.routes[path] = rt
}

func (g *fakeGH) drop(path string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.routes, path)
}

func (g *fakeGH) failAll() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for p := range g.routes {
		g.routes[p] = route{Status: 500, Body: json.RawMessage(`{"message":"boom"}`)}
	}
}

func (g *fakeGH) calls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, c := range g.hits {
		n += c
	}
	return n
}

func (g *fakeGH) client(context.Context) (*rest.Client, error) {
	return &rest.Client{
		BaseURL: g.srv.URL, AllowPrivateBase: true, Provider: Name,
		TLSConfig: g.srv.Client().Transport.(*http.Transport).TLSClientConfig,
	}, nil
}

// rig is a Source over a fake API with controllable secrets and environment.
type rig struct {
	src *Source
	gh  *fakeGH
	env map[string]string
}

func newRig(t *testing.T, secrets map[string]RepoSecret) *rig {
	t.Helper()
	r := &rig{gh: newFakeGH(t), env: map[string]string{}}
	r.src = &Source{
		Now:      func() time.Time { return fixedNow },
		Getenv:   func(k string) string { return r.env[k] },
		Secrets:  func(context.Context) (map[string]RepoSecret, error) { return secrets, nil },
		Insecure: func() bool { return false },
		NewAPI:   r.gh.client,
	}
	return r
}

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return sigPrefix + hex.EncodeToString(m.Sum(nil))
}

func request(event, delivery, signature string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set(hdrEvent, event)
	if delivery != "" {
		req.Header.Set(hdrDelivery, delivery)
	}
	if signature != "" {
		req.Header.Set(hdrSignature, signature)
	}
	return req
}

// deliver sends a delivery signed with secret ("" sends it unsigned) straight to Verify.
func (r *rig) deliver(event, secret string, body []byte) (model.TriggerFacts, error) {
	sig := ""
	if secret != "" {
		sig = sign(secret, body)
	}
	return r.src.Verify(context.Background(), request(event, "d-1", sig, body), body)
}

// golden compares v, as indented JSON, to testdata/golden/<name>.json.
func golden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "golden", name+".json")
	if *update {
		if err = os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden %s differs:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

// replace edits one JSON field text of a fixture.
func replace(body []byte, old, new string) []byte {
	return []byte(strings.Replace(string(body), old, new, 1))
}
