package health_test

// Purpose: shared fixtures of the P7-HOST-11 tests: a fake GitHub API served over TLS with fault
//          modes (5xx, connection reset, 429 with Retry-After, 401), an injected clock, a store and
//          a tracker wired the way production wires the default one (health function and REST
//          observer), and the world and placement builders that `nself ci explain` uses.
// Inputs:  per-test mode switches.
// Outputs: recorded requests and the state of the fake.
// Constraints: no live network (the fixture is on loopback and gh is hidden from PATH); every token
//          is synthetic; tests that install a tracker restore the default one in t.Cleanup.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	testRev   = "0123456789abcdef0123456789abcdef01234567"
	testToken = "synthetic-test-token-0001"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

// Fault modes of the fake API.
const (
	modeOK = iota
	mode5xx
	modeReset
	modeQuota
	modeUnauth
)

// fakeAPI is a GitHub API with a fault mode.
type fakeAPI struct {
	srv  *httptest.Server
	mu   sync.Mutex
	mode int
	reqs []string
	post []postedStatus
}

type postedStatus struct{ Path, State, Context string }

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) setMode(m int) {
	f.mu.Lock()
	f.mode = m
	f.mu.Unlock()
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	mode := f.mode
	f.reqs = append(f.reqs, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	switch mode {
	case mode5xx:
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	case modeReset:
		if c, _, err := w.(http.Hijacker).Hijack(); err == nil {
			_ = c.Close()
		}
		return
	case modeQuota:
		w.Header().Set("Retry-After", "90")
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	case modeUnauth:
		http.Error(w, "bad credentials", http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == "POST" && strings.Contains(r.URL.Path, "/statuses/"):
		var in struct{ State, Context string }
		_ = json.Unmarshal(body, &in)
		f.mu.Lock()
		f.post = append(f.post, postedStatus{Path: r.URL.Path, State: in.State, Context: in.Context})
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("{}"))
	case r.URL.Path == "/user":
		_, _ = w.Write([]byte(`{"login":"ci-operator"}`))
	case strings.HasPrefix(r.URL.Path, "/repos/o/r"):
		_, _ = w.Write([]byte(`{"private":false,"visibility":"public","default_branch":"main"}`))
	default:
		http.NotFound(w, r)
	}
}

// count returns how many requests matched "METHOD /path-prefix".
func (f *fakeAPI) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reqs {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeAPI) total() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.reqs) }

func (f *fakeAPI) posted() []postedStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]postedStatus(nil), f.post...)
}

// client is a REST funnel client for the fake, labelled as the GitHub API like production.
func (f *fakeAPI) client() *rest.Client {
	return &rest.Client{BaseURL: f.srv.URL, AllowPrivateBase: true, Provider: "github", APIVersion: "2022-11-28", UserAgent: "health-test",
		TLSConfig: f.srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone(), // one config per client: net/http edits NextProtos in place
		Auth:      func(context.Context) (string, error) { return testToken, nil }}
}

// clock is an injected time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

// newClock starts at the real time so a Retry-After the REST funnel computes from the wall clock
// lies in the injected clock's future too.
func newClock() *clock { return &clock{t: time.Now().Truncate(time.Second)} }
func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *clock) advance(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func openStore(t *testing.T, now func() time.Time) *store.Store {
	t.Helper()
	opts := store.Options{}
	if now != nil {
		opts.Clock = now
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// install wires tr as the process-wide tracker (health function and REST observer) for the test.
func install(t *testing.T, tr *health.Tracker) {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	t.Setenv("PATH", t.TempDir()) // no gh: the default github-hosted provider has no credential and is skipped
	health.Install(tr)
	t.Cleanup(func() { health.Install(health.Default()) })
}

func outcome(class providers.ErrorClass, status int) rest.Outcome {
	return rest.Outcome{Provider: "github", Class: class, Status: status}
}

func itoa(n int) string { return strconv.Itoa(n) }

func rest0(provider string) rest.Outcome {
	return rest.Outcome{Provider: provider, Class: providers.Transient, Status: 503}
}
