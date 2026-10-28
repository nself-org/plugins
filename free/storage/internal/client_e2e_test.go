package internal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nself-org/plugin-storage/client"
)

// serveRoutes runs the real route table (chi mux, secret gate, account check) on a loopback server.
// No database or S3 is attached: the cases below stop before either, so a reply other than 401 or 503-secret
// proves the request got past the gate.
func serveRoutes(t *testing.T, secret string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(rawRouter(&Server{cfg: &Config{InternalSecret: secret}}))
	t.Cleanup(srv.Close)
	return srv
}

// TestClient_sendsSecretThroughGeneratedRoute is the D-0648 end-to-end check: the shipped client sends
// X-Internal-Token (and the legacy X-Plugin-Secret) on every call, so storage answers it instead of refusing with 401.
func TestClient_sendsSecretThroughGeneratedRoute(t *testing.T) {
	srv := serveRoutes(t, "route-secret")
	c, err := client.New(srv.URL, "route-secret")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Past the gate, a bad bucket name is a 400 from the handler.
	_, err = c.CreateBucket(ctx, "A", "")
	assertStatus(t, err, http.StatusBadRequest)

	// Past the gate, a valid call reaches the "no S3 configured" answer (not the secret 401/503).
	_, err = c.CreateBucket(ctx, "photos", "")
	assertStatus(t, err, http.StatusServiceUnavailable)
	var se *client.StatusError
	if !errors.As(err, &se) || !strings.Contains(se.Body, "object storage not configured") {
		t.Fatalf("want the no-store answer, got %v", err)
	}

	err = c.PutObject(ctx, "photos", "a.txt", "text/plain", strings.NewReader("hi"), 2)
	assertStatus(t, err, http.StatusServiceUnavailable)
}

func TestClient_wrongSecretIsRefused(t *testing.T) {
	srv := serveRoutes(t, "route-secret")
	c, err := client.New(srv.URL, "other-secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateBucket(context.Background(), "photos", "")
	assertStatus(t, err, http.StatusUnauthorized)
}

func TestClient_sendsAccountHeader(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c, _ := client.New(srv.URL, "s3cret", client.WithAccount("tenant-7"))
	if err := c.DeleteObject(context.Background(), "b", "k"); err != nil {
		t.Fatal(err)
	}
	if got.Get("X-Internal-Token") != "s3cret" || got.Get("X-Plugin-Secret") != "s3cret" || got.Get("X-Hasura-Source-Account-Id") != "tenant-7" {
		t.Fatalf("headers = %v", got)
	}
}

func TestClient_requiresSecret(t *testing.T) {
	if _, err := client.New("http://storage:9007", "  "); !errors.Is(err, client.ErrNoSecret) {
		t.Fatalf("want ErrNoSecret, got %v", err)
	}
	t.Setenv("PLUGIN_INTERNAL_SECRET", "")
	if _, err := client.NewFromEnv("http://storage:9007"); !errors.Is(err, client.ErrNoSecret) {
		t.Fatalf("want ErrNoSecret from env, got %v", err)
	}
	t.Setenv("PLUGIN_INTERNAL_SECRET", " from-env \n")
	if _, err := client.NewFromEnv("http://storage:9007"); err != nil {
		t.Fatal(err)
	}
}

func assertStatus(t *testing.T, err error, want int) {
	t.Helper()
	var se *client.StatusError
	if !errors.As(err, &se) || se.Code != want {
		t.Fatalf("want status %d, got %v", want, err)
	}
}
