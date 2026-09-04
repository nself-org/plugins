package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew_Defaults(t *testing.T) {
	c := New(ClientOptions{})
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.opts.Timeout != 30*time.Second {
		t.Errorf("default Timeout = %v, want 30s", c.opts.Timeout)
	}
	if c.opts.UserAgent != "nself-plugin-sdk/0.1" {
		t.Errorf("default UserAgent = %q, want %q", c.opts.UserAgent, "nself-plugin-sdk/0.1")
	}
	if c.opts.MaxRetries != 2 {
		t.Errorf("default MaxRetries = %d, want 2", c.opts.MaxRetries)
	}
	if c.opts.RetryDelay != 500*time.Millisecond {
		t.Errorf("default RetryDelay = %v, want 500ms", c.opts.RetryDelay)
	}
}

func TestNew_CustomOptions(t *testing.T) {
	c := New(ClientOptions{
		Timeout:    5 * time.Second,
		UserAgent:  "my-plugin/1.0",
		MaxRetries: 3,
		RetryDelay: 200 * time.Millisecond,
	})
	if c.opts.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", c.opts.Timeout)
	}
	if c.opts.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", c.opts.MaxRetries)
	}
}

func TestNew_NegativeMaxRetries(t *testing.T) {
	// Negative retries are normalized to 0, then the default of 2 is applied.
	c := New(ClientOptions{MaxRetries: -1})
	if c.opts.MaxRetries != 2 {
		t.Errorf("negative MaxRetries should normalize to default 2, got %d", c.opts.MaxRetries)
	}
}

func TestDo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(ClientOptions{Timeout: 5 * time.Second, MaxRetries: 0})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestDo_SetsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(ClientOptions{MaxRetries: 0})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	resp.Body.Close()
	if gotUA != "nself-plugin-sdk/0.1" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "nself-plugin-sdk/0.1")
	}
}
