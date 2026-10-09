package rest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type brokenReader struct{ sent bool }

func (r *brokenReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, "partial"), nil
	}
	return 0, io.ErrUnexpectedEOF
}

// TestDownloadDifferentPortCredential proves only the exact base authority receives a token.
func TestDownloadDifferentPortCredential(t *testing.T) {
	c := &Client{BaseURL: "https://api.example:443", Auth: func(context.Context) (string, error) { return "secret", nil }}
	var got string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got = req.Header.Get("Authorization")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})
	var output bytes.Buffer
	_, err := c.download(context.Background(), "https://api.example:8443/artifact", 10, &output, &http.Client{Transport: transport})
	if err != nil || got != "" || output.String() != "ok" {
		t.Fatalf("different port: auth=%q output=%q err=%v", got, output.String(), err)
	}
	_, err = c.download(context.Background(), "https://api.example/artifact", 10, &output, &http.Client{Transport: transport})
	if err != nil || got != "Bearer secret" {
		t.Fatalf("base authority: auth=%q err=%v", got, err)
	}
}

// TestBodyReadErrors proves truncated API and download bodies are transient, while caps remain config.
func TestBodyReadErrors(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("partial"))
	}))
	defer s.Close()
	_, err := testClient(s).Do(context.Background(), "GET", "/partial", nil, nil, nil)
	assertError(t, err, "E704", "network")
	c := &Client{BaseURL: "https://api.example"}
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(&brokenReader{}), Header: make(http.Header)}, nil
	})
	var output bytes.Buffer
	_, err = c.download(context.Background(), "https://api.example/artifact", 100, &output, &http.Client{Transport: transport})
	assertError(t, err, "E704", "network")
	if output.Len() != 0 {
		t.Fatal("partial artifact was written")
	}
	transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(&brokenReader{}), Header: make(http.Header)}, nil
	})
	_, err = c.download(context.Background(), "https://api.example/artifact", 100, &output, &http.Client{Transport: transport})
	assertError(t, err, "E704", "network")
}

// TestDownloadRetryAtObserver proves quota retry time reaches the response and observer.
func TestDownloadRetryAtObserver(t *testing.T) {
	c := &Client{BaseURL: "https://api.example"}
	var outcome Outcome
	c.SetObserver(func(o Outcome) { outcome = o })
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{"Retry-After": []string{"60"}}}, nil
	})
	response, err := c.download(context.Background(), "https://api.example/artifact", 100, io.Discard, &http.Client{Transport: transport})
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != "E703" || response.RetryAt.IsZero() || !response.RetryAt.Equal(pe.RetryAt) || !outcome.RetryAt.Equal(pe.RetryAt) || outcome.Class != providers.Quota {
		t.Fatalf("retry metadata: response=%+v outcome=%+v err=%v", response, outcome, err)
	}
	if time.Until(response.RetryAt) < 50*time.Second {
		t.Fatalf("retry time too soon: %v", response.RetryAt)
	}
}

// TestReviewRedirectAndScopeRules proves exact host entries, safe methods, path segments and missing scopes.
func TestReviewRedirectAndScopeRules(t *testing.T) {
	if hostMatch("child.example.com", "example.com") || !hostMatch("child.example.com", "*.example.com") || hostMatch("example.com", "*.example.com") {
		t.Fatal("redirect host matching")
	}
	c := &Client{BaseURL: "https://api.example", RedirectHosts: []string{"other.example"}}
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		req, _ := http.NewRequest(method, "https://other.example/path", nil)
		if err := c.checkRedirect(req, []*http.Request{req}); err == nil {
			t.Fatalf("accepted %s redirect", method)
		}
	}
	for _, segment := range []string{".", ".."} {
		_, err := c.Do(context.Background(), "GET", "/repos/{repo}", map[string]string{"repo": segment}, nil, nil)
		assertError(t, err, "E702", "request.path")
	}
	for _, tc := range []struct{ accepted, granted, reason string }{
		{"repo, admin:org", "repo", "missing_scope:admin:org"},
		{"repo", "repo", "permission"},
		{"", "repo", "permission"},
	} {
		header := http.Header{"X-Accepted-Oauth-Scopes": []string{tc.accepted}, "X-Oauth-Scopes": []string{tc.granted}}
		if got := classify(403, header, nil, time.Now()).Reason; got != tc.reason {
			t.Fatalf("scope reason=%q, want %q", got, tc.reason)
		}
	}
}
