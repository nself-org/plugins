package main

// Purpose: tests of the user API and local-socket listeners inside a running V15 `ci serve`
// (P7-NODE-19): the TLS API answers a scoped token, the socket is 0600 and holds the serve lock, a second
// serve on the same home is refused, and everything is released at shutdown.
// Inputs:  serveV15 with a fake webhook listener, a short-path data dir, a free TCP port.
// Outputs: pass or fail.
// Constraints: 127.0.0.1 only; the data dir is under the system temp dir (sun_path is 104 bytes).

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}

func TestServeStartsUserAPIAndLocalSocket(t *testing.T) {
	serveFixture(t, quietHost())
	data, err := os.MkdirTemp("", "ci-serve")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(data) })
	api := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	args := []string{"--insecure", "--workdir", filepath.Join(data, "wd"), "--data-dir", data, "--api-addr", api}
	hook := func(c *coordinator.Config) {
		c.Listen = func(network, _ string) (net.Listener, error) { return net.Listen(network, "127.0.0.1:0") }
	}
	var code int
	wg.Add(1)
	go func() { defer wg.Done(); code = serveV15(ctx, args, hook) }()
	sock := coordclient.SocketPath(data)
	waitUntil(t, "the serve lock and the socket", func() bool {
		_, err := os.Stat(sock)
		return err == nil && coordclient.ServeHoldsLock(data)
	})
	if fi, _ := os.Stat(sock); fi.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v", fi.Mode().Perm())
	}
	// a token issued through the same store, a client that pins the coordinator root
	st, err := store.Open(filepath.Join(data, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	svc := roles.New(st, &audit.Log{St: st})
	svc.Self = "alice"
	tok, _, err := svc.IssueToken(ctx, svc.Operator("cli:alice"), roles.Request{Principal: "mia", Role: roles.Member, Projects: []string{"acme/app"}})
	if err != nil {
		t.Fatal(err)
	}
	ca, err := pki.New(ctx, pki.Options{Store: st, DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: ca.RootDER(), ServerName: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 10 * time.Second}
	var status int
	var body string
	waitUntil(t, "the user API", func() bool {
		req, _ := http.NewRequest("GET", "https://"+api+"/v1/ping", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := hc.Do(req)
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		status, body = resp.StatusCode, string(b)
		return true
	})
	if status != 200 || !strings.Contains(body, `"principal":"mia"`) {
		t.Fatalf("ping: %d %s", status, body)
	}
	// a second serve on this home is refused while the first runs
	if c2 := serveV15(ctx, []string{"--insecure", "--workdir", filepath.Join(data, "wd2"), "--data-dir", data, "--api-addr", freePort(t)}, hook); c2 == 0 {
		t.Fatal("a second serve on the same CI home must not start")
	}
	cancel()
	wg.Wait()
	if code != 0 {
		t.Fatalf("serve exit = %d", code)
	}
	if coordclient.ServeHoldsLock(data) {
		t.Fatal("the serve lock outlived the serve")
	}
	if _, err = os.Stat(sock); err == nil {
		t.Fatal("the socket outlived the serve")
	}
	_ = tls.VersionTLS13
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The lock outlives a failed socket and an off socket: a serve that runs without it would let a second
// `nself ci` write its store.
func TestServeKeepsTheLockWhenTheSocketIsOffOrFails(t *testing.T) {
	serveFixture(t, quietHost())
	hook := func(c *coordinator.Config) {
		c.Listen = func(network, _ string) (net.Listener, error) { return net.Listen(network, "127.0.0.1:0") }
	}
	for _, c := range []struct {
		name string
		long bool     // a data dir whose default socket path is over the 104-byte limit
		sock []string // extra flags
	}{{"off", false, []string{"--api-socket", ""}}, {"unbindable default path", true, nil}} {
		base, err := os.MkdirTemp("", "ci-lock")
		if err != nil {
			t.Fatal(err)
		}
		data := base
		if c.long {
			data = filepath.Join(base, strings.Repeat("x", 60), strings.Repeat("y", 60))
			if err = os.MkdirAll(data, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		args := func(wd string) []string {
			return append([]string{"--insecure", "--workdir", filepath.Join(base, wd), "--data-dir", data, "--api-addr", freePort(t)}, c.sock...)
		}
		done := make(chan int, 1)
		go func() { done <- serveV15(ctx, args("wd"), hook) }()
		waitUntil(t, c.name+": the serve lock", func() bool { return coordclient.ServeHoldsLock(data) })
		if c2 := serveV15(ctx, args("wd2"), hook); c2 == 0 {
			t.Errorf("%s: a second serve started on a home whose serve has no socket", c.name)
		}
		cancel()
		if code := <-done; code != 0 {
			t.Errorf("%s: exit %d", c.name, code)
		}
		_ = os.RemoveAll(base)
	}
}

// A serve started with another --api-socket path says so in its lock file, and clients follow it.
func TestClientFollowsTheSocketPathInTheLock(t *testing.T) {
	serveFixture(t, quietHost())
	data, err := os.MkdirTemp("", "ci-sockpath")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(data) }()
	sock := filepath.Join(data, "elsewhere.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	hook := func(c *coordinator.Config) {
		c.Listen = func(network, _ string) (net.Listener, error) { return net.Listen(network, "127.0.0.1:0") }
	}
	go func() {
		done <- serveV15(ctx, []string{"--insecure", "--workdir", filepath.Join(data, "wd"), "--data-dir", data, "--api-addr", freePort(t), "--api-socket", sock}, hook)
	}()
	waitUntil(t, "the socket", func() bool { _, err := os.Stat(sock); return err == nil && coordclient.ServeHoldsLock(data) })
	if got := coordclient.SocketFor(data); got != sock {
		t.Fatalf("SocketFor = %q, want %q", got, sock)
	}
	tg, _, err := coordclient.Resolve(coordclient.Inputs{Getenv: func(string) string { return "" }, Home: data})
	if err != nil || tg.Kind != coordclient.LocalServe || tg.Socket != sock {
		t.Fatalf("target %+v %v", tg, err)
	}
	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("exit %d", code)
	}
}

// D-1406: a listener of a running serve reads the serve flags while the next serve parses its own flag set.
// Building and parsing a flag set must not touch what a running serve's listeners read.
func TestParsingAServeFlagSetDoesNotRaceWithRunningListeners(t *testing.T) {
	fs, _, _ := newServeV15Flags()
	if _, err := parseInterspersed(fs, []string{"--data-dir", "/first"}); err != nil {
		t.Fatal(err)
	}
	publishServeFlags(fs)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // a listener of the first serve, still starting or shutting down
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = serveFlag("data-dir")
				_ = explicitFlag("api-socket")
			}
		}
	}()
	for i := 0; i < 200; i++ { // a second serve builds and parses its flags
		next, _, _ := newServeV15Flags()
		if _, err := parseInterspersed(next, []string{"--data-dir", "/second", "--api-socket", ""}); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
	if got := serveFlag("data-dir"); got != "/first" {
		t.Fatalf("data-dir = %q; only a published flag set may change what listeners read", got)
	}
}
