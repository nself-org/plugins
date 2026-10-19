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
	waitFor(t, "the serve lock and the socket", func() bool {
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
	waitFor(t, "the user API", func() bool {
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

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}
