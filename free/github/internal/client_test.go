package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHasNextPage verifies detection of the Link: rel="next" pagination header.
func TestHasNextPage(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{`<https://api.github.com/repos?page=2>; rel="next", <https://api.github.com/repos?page=5>; rel="last"`, true},
		{`<https://api.github.com/repos?page=5>; rel="last"`, false},
		{"", false},
		{`rel="next"`, true},
		{`rel="prev"`, false},
	}
	for _, tc := range cases {
		got := hasNextPage(tc.header)
		if got != tc.want {
			t.Errorf("hasNextPage(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

// TestNewGitHubClient verifies that NewGitHubClient sets the token field.
func TestNewGitHubClient_Token(t *testing.T) {
	c := NewGitHubClient("ghp_test_token")
	if c.token != "ghp_test_token" {
		t.Errorf("token = %q, want %q", c.token, "ghp_test_token")
	}
}

// TestDoRequest_AuthHeader verifies that doRequest sends the Authorization
// header with the Bearer token.
func TestDoRequest_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-token")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"id": "repo_1"})
	}))
	defer srv.Close()

	c := NewGitHubClient("test-token")
	c.baseURL = srv.URL

	resp, err := c.doRequest("/repos")
	if err != nil {
		t.Fatalf("doRequest error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestDoRequest_ErrorStatus verifies that doRequest propagates non-2xx
// responses (it should not swallow them — the caller checks status).
func TestDoRequest_Returns4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewGitHubClient("token")
	c.baseURL = srv.URL

	resp, err := c.doRequest("/not-found")
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
