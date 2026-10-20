package webhook

// Purpose: shared test rig (P7-HOST-21): a fake GitLab API over TLS fed from recorded responses, a
// Source wired to it, Standard Webhooks signing, request builders and the golden file helper.
// Inputs:  testdata/*.json payloads (shaped after the GitLab webhook documentation),
//          testdata/api/routes.json recorded API responses keyed by escaped path (+ "?query").
// Outputs: rig values the tests drive; goldens under testdata/golden (rewrite with -update).
// Constraints: no live network call (the fake server binds loopback and the client trusts only its
//          certificate); every secret here is an obviously synthetic string.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// The secrets are built at run time so no whsec_ literal sits in the source for a secret scanner to judge.
var (
	signingSecret = secretPrefix + base64.StdEncoding.EncodeToString([]byte("synthetic-signing-secret-for-tests"))
	otherSecret   = secretPrefix + base64.StdEncoding.EncodeToString([]byte("another-synthetic-key"))

	shaA = strings.Repeat("a", 40)
	shaB = strings.Repeat("b", 40)
	shaC = strings.Repeat("c", 40)
	shaD = strings.Repeat("d", 40)

	fixedNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
)

const (
	bareToken  = "synthetic-bare-token"
	signingEnv = "SYNTH_SIGNING_SECRET"
	tokenEnv   = "SYNTH_BARE_TOKEN"
)

type route struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
	delay  time.Duration
}

// fakeGL serves recorded API responses by escaped request path (+ "?" + sorted query when present).
type fakeGL struct {
	mu     sync.Mutex
	routes map[string]route
	hits   map[string]int
	paths  []string // escaped request URIs, in order
	srv    *httptest.Server
}

func newFakeGL(t *testing.T) *fakeGL {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "api", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	g := &fakeGL{routes: map[string]route{}, hits: map[string]int{}}
	if err = json.Unmarshal(raw, &g.routes); err != nil {
		t.Fatal(err)
	}
	g.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.Query().Encode()
		}
		g.mu.Lock()
		rt, ok := g.routes[key]
		g.hits[key]++
		g.paths = append(g.paths, key)
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
			_, _ = w.Write([]byte(`{"message":"404 Not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.Status)
		_, _ = w.Write(rt.Body)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGL) set(key string, status int, body string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.routes[key] = route{Status: status, Body: json.RawMessage(body)}
}

func (g *fakeGL) drop(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.routes, key)
}

func (g *fakeGL) slow(key string, d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	rt := g.routes[key]
	rt.delay = d
	g.routes[key] = rt
}

func (g *fakeGL) failAll() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for k := range g.routes {
		g.routes[k] = route{Status: 500, Body: json.RawMessage(`{"message":"boom"}`)}
	}
}

func (g *fakeGL) calls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.paths)
}

func (g *fakeGL) saw(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.hits[key] > 0
}

func (g *fakeGL) client(context.Context) (*rest.Client, error) {
	return &rest.Client{
		BaseURL: g.srv.URL, AllowPrivateBase: true, Provider: Name,
		TLSConfig: g.srv.Client().Transport.(*http.Transport).TLSClientConfig,
	}, nil
}

// rig is a Source over a fake API with controllable secrets and environment.
type rig struct {
	src *Source
	gl  *fakeGL
	env map[string]string
	now time.Time
}

// both is the usual policy: acme/app has a signing secret, acme/tokened only a bare token.
func both() Secrets {
	return Secrets{
		Signing: map[string]RepoSecret{"acme/app": {Env: signingEnv}},
		Tokens:  map[string]RepoSecret{"acme/tokened": {Env: tokenEnv}},
	}
}

func newRig(t *testing.T, pol Secrets) *rig {
	t.Helper()
	r := &rig{gl: newFakeGL(t), env: map[string]string{signingEnv: signingSecret, tokenEnv: bareToken}, now: fixedNow}
	r.src = &Source{
		Now:    func() time.Time { return r.now },
		Getenv: func(k string) string { return r.env[k] },
		Policy: func(context.Context) (Secrets, error) { return pol, nil },
		NewAPI: r.gl.client,
	}
	return r
}

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer // one line, so replace() edits are exact
	if err = json.Compact(&compact, b); err != nil {
		t.Fatal(err)
	}
	return compact.Bytes()
}

// stdSignature is the webhook-signature value GitLab sends for (id, ts, body) under a whsec_ secret.
func stdSignature(secret, id, ts string, body []byte) string {
	key, ok := decodeKey(secret)
	if !ok {
		panic("bad synthetic secret")
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(id + "." + ts + "." + string(body)))
	return "v1," + base64.StdEncoding.EncodeToString(m.Sum(nil))
}

// delivery describes the headers of one request.
type delivery struct {
	event, id, uuid string
	ts              time.Time
	signSecret      string // "" sends no signature
	signature       string // overrides the computed one
	token           string
}

func (d delivery) request(body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set(hdrEvent, d.event)
	if d.uuid != "" {
		req.Header.Set(hdrEventUUID, d.uuid)
	}
	if d.token != "" {
		req.Header.Set(hdrToken, d.token)
	}
	if d.id != "" {
		req.Header.Set(hdrMsgID, d.id)
		ts := strconv.FormatInt(d.ts.Unix(), 10)
		req.Header.Set(hdrTimestamp, ts)
		switch {
		case d.signature != "":
			req.Header.Set(hdrSignature, d.signature)
		case d.signSecret != "":
			req.Header.Set(hdrSignature, stdSignature(d.signSecret, d.id, ts, body))
		}
	}
	return req
}

// signed sends a Standard Webhooks delivery stamped at the rig's clock.
func (r *rig) signed(event string, body []byte) (model.TriggerFacts, error) {
	d := delivery{event: event, id: "msg-1", ts: r.now, signSecret: signingSecret}
	return r.src.Verify(context.Background(), d.request(body), body)
}

// bearer sends a delivery with only X-Gitlab-Token.
func (r *rig) bearer(event, token string, body []byte) (model.TriggerFacts, error) {
	d := delivery{event: event, uuid: "uuid-1", token: token}
	return r.src.Verify(context.Background(), d.request(body), body)
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

// replace edits one JSON text of a fixture.
func replace(body []byte, old, new string) []byte {
	if !bytes.Contains(body, []byte(old)) {
		panic("fixture has no " + old)
	}
	return []byte(strings.Replace(string(body), old, new, 1))
}
