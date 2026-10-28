package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type hit struct {
	path, auth, signature string
	body                  []byte
}

// hookServer is a TLS receiver on a loopback address, which is private by the guard's rule.
func hookServer(t *testing.T, status int) (*httptest.Server, *[]hit, *sync.Mutex, *tls.Config) {
	t.Helper()
	var mu sync.Mutex
	var hits []hit
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		hits = append(hits, hit{r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Nself-Signature"), b})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return srv, &hits, &mu, &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

func sampleEvent() Event {
	return Event{Kind: KindFailure, Time: "2026-10-19T09:00:00Z", Message: "nself-ci failed on r1", Repo: "r1", Revision: sha("a"), RunID: "run-1", Result: "fail"}
}

func TestWebhookSignedAndGuarded(t *testing.T) {
	srv, hits, mu, tlsCfg := hookServer(t, 200)
	ctx := context.Background()
	const secret = "synthetic-test-secret"
	spec := SinkSpec{Type: "webhook", Name: "ops", URL: srv.URL + "/hooks/path-token-1/x", Secret: secret}

	// the guard refuses a private (loopback) address without the operator key, and nothing is sent
	n, err := New(spec, Deps{TLSConfig: tlsCfg})
	if err != nil {
		t.Fatal(err)
	}
	err = n.Notify(ctx, sampleEvent())
	if err == nil || !strings.Contains(err.Error(), "E813") || !strings.Contains(err.Error(), "ssrf.private") || strings.Contains(err.Error(), "path-token-1") || strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("private address must be refused with a safe error: %v", err)
	}
	mu.Lock()
	if len(*hits) != 0 {
		t.Fatal("a refused request reached the server")
	}
	mu.Unlock()

	// with notify.allow_private it posts the event, signed over the exact body
	n, err = New(spec, Deps{TLSConfig: tlsCfg, AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = n.Notify(ctx, sampleEvent()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*hits) != 1 {
		t.Fatalf("hits: %d", len(*hits))
	}
	h := (*hits)[0]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(h.body)
	if h.path != "/hooks/path-token-1/x" || h.auth != "" || h.signature != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("path %q auth %q", h.path, h.auth)
	}
	var got Event
	if err = json.Unmarshal(h.body, &got); err != nil || got.Kind != KindFailure || got.RunID != "run-1" {
		t.Fatalf("body: %s (%v)", h.body, err)
	}
	// a tampered body no longer verifies
	mac = hmac.New(sha256.New, []byte(secret))
	mac.Write(append(h.body, ' '))
	if h.signature == hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal("signature must depend on the body")
	}
}

func TestWebhookWithoutSecretSendsNoAuthorization(t *testing.T) {
	srv, hits, mu, tlsCfg := hookServer(t, 200)
	n, err := New(SinkSpec{Type: "webhook", URL: srv.URL}, Deps{TLSConfig: tlsCfg, AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = n.Notify(context.Background(), sampleEvent()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*hits) != 1 || (*hits)[0].auth != "" || (*hits)[0].signature != "" || (*hits)[0].path != "/" {
		t.Fatalf("hits: %+v", *hits)
	}
}

func TestWebhookRefusesBadURLsAndAnswers(t *testing.T) {
	for _, u := range []string{"http://example.org/h", "https://user:pw@example.org/h", "https://example.org/h?token=x", "https://example.org/h#f", "example.org/h", "https://"} {
		if _, err := New(SinkSpec{Type: "webhook", URL: u}, Deps{}); err == nil || strings.Contains(err.Error(), "example.org") {
			t.Errorf("%q must be refused without echoing it: %v", u, err)
		}
	}
	srv, _, _, tlsCfg := hookServer(t, 500)
	n, _ := New(SinkSpec{Type: "webhook", Name: "w", URL: srv.URL + "/h"}, Deps{TLSConfig: tlsCfg, AllowPrivate: true})
	if err := n.Notify(context.Background(), sampleEvent()); err == nil || !strings.Contains(err.Error(), "E813") {
		t.Fatalf("a 500 answer is a failed delivery: %v", err)
	}
}

type fakeRunner struct {
	exit  int
	err   error
	argv  []string
	stdin []byte
}

func (f *fakeRunner) Run(_ context.Context, argv []string, stdin []byte, _ time.Duration) (int, error) {
	f.argv, f.stdin = argv, stdin
	return f.exit, f.err
}

func TestCommandSinkPassesEventOnStdin(t *testing.T) {
	r := &fakeRunner{}
	n, err := New(SinkSpec{Type: "command", Name: "bot", Argv: []string{"/opt/bin/tell", "--room", "ci"}}, Deps{Runner: r})
	if err != nil {
		t.Fatal(err)
	}
	if err = n.Notify(context.Background(), sampleEvent()); err != nil {
		t.Fatal(err)
	}
	var ev Event
	if strings.Join(r.argv, " ") != "/opt/bin/tell --room ci" || json.Unmarshal(r.stdin, &ev) != nil || ev.RunID != "run-1" || !bytes.HasSuffix(r.stdin, []byte("\n")) {
		t.Fatalf("argv %v stdin %s", r.argv, r.stdin)
	}
	r.exit = 3
	if err = n.Notify(context.Background(), sampleEvent()); err == nil || !strings.Contains(err.Error(), "exited 3") || strings.Contains(err.Error(), "/opt/bin") {
		t.Fatalf("a non-zero exit is a failure without argv in the error: %v", err)
	}
}

// The real funnel: the event reaches stdin of a real process, and a command that outlives its
// timeout is killed and reported.
func TestCommandSinkTimeout(t *testing.T) {
	home := t.TempDir()
	out := filepath.Join(t.TempDir(), "stdin.json")
	n, err := New(SinkSpec{Type: "command", Name: "real", Argv: []string{"sh", "-c", `cat > "$0"`, out}}, Deps{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if err = n.Notify(context.Background(), sampleEvent()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	var ev Event
	if json.Unmarshal(b, &ev) != nil || ev.RunID != "run-1" {
		t.Fatalf("stdin of the real command: %q", b)
	}

	slow, err := New(SinkSpec{Type: "command", Name: "slow", Argv: []string{"sleep", "60"}}, Deps{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	slow.(*commandSink).timeout = 300 * time.Millisecond
	start := time.Now()
	err = slow.Notify(context.Background(), sampleEvent())
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "killed") || strings.Contains(err.Error(), "sleep") {
		t.Fatalf("timeout error: %v", err)
	}
	if d := time.Since(start); d > 8*time.Second {
		t.Fatalf("the command was not killed at its timeout: %s", d)
	}
}

func TestStdoutSinkWritesJSONLines(t *testing.T) {
	var buf bytes.Buffer
	n, err := New(SinkSpec{Type: "stdout"}, Deps{Out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	_ = n.Notify(context.Background(), sampleEvent())
	_ = n.Notify(context.Background(), sampleEvent())
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var ev Event
	if len(lines) != 2 || json.Unmarshal([]byte(lines[0]), &ev) != nil || ev.Kind != KindFailure {
		t.Fatalf("output: %q", buf.String())
	}
	if _, err = New(SinkSpec{Type: "stdout", URL: "https://example.org"}, Deps{}); err == nil {
		t.Fatal("a stdout sink takes no url")
	}
}

func TestRegistry(t *testing.T) {
	if got := strings.Join(Types(), ","); got != "command,stdout,webhook" {
		t.Fatalf("built-in types: %s", got)
	}
	if err := Register("stdout", newStdout); err == nil {
		t.Fatal("a duplicate type must be refused")
	}
	if err := Register("", newStdout); err == nil {
		t.Fatal("an empty type must be refused")
	}
	sinks, errs := Build([]SinkSpec{{Type: "stdout"}, {Type: "nope"}, {Type: "command"}}, Deps{})
	if len(sinks) != 1 || len(errs) != 2 {
		t.Fatalf("a bad spec is skipped and reported: %d sinks, %v", len(sinks), errs)
	}
}
