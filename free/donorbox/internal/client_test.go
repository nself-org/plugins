package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestDonorboxClient creates a DonorboxClient pointing at the given test
// server URL. We override the httpClient transport indirectly by replacing
// the embedded httpClient's transport, but since DonorboxClient.get builds a
// full URL from donorboxBaseURL we instead use a monkey-patch approach: create
// the client normally, then swap its httpClient to use the test-server URL.
//
// Because the DonorboxClient builds URLs as donorboxBaseURL+path and we cannot
// override donorboxBaseURL from outside the package, we use a custom
// http.RoundTripper that rewrites the host to point at the test server.
type rewriteTransport struct {
	host string
}

func (rt *rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Host = rt.host
	r2.URL.Scheme = "http"
	return http.DefaultTransport.RoundTrip(r2)
}

func newTestDonorboxClient(serverAddr string) *DonorboxClient {
	c := NewDonorboxClient("test@example.com", "test-api-key")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{host: serverAddr},
	}
	return c
}

// TestNewDonorboxClient verifies that the constructor sets a Basic Auth header
// derived from the email and API key.
func TestNewDonorboxClient_AuthHeader(t *testing.T) {
	c := NewDonorboxClient("user@example.com", "apikey123")
	if c.authHeader == "" {
		t.Error("expected non-empty authHeader")
	}
	// Must start with "Basic " prefix.
	if len(c.authHeader) < 6 || c.authHeader[:6] != "Basic " {
		t.Errorf("expected authHeader to start with 'Basic ', got %q", c.authHeader)
	}
}

// TestGet_HappyPath verifies that get() sends the Authorization header and
// returns the response body on HTTP 200.
func TestGet_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header to be set")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]string{{"id": "c_1"}})
	}))
	defer srv.Close()

	c := newTestDonorboxClient(srv.Listener.Addr().String())
	// Reset lastCall to avoid the 1-second rate-limit sleep.
	c.lastCall = c.lastCall.Add(-2)

	body, err := c.get("/campaigns", nil)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
}

// TestGet_HTTPError verifies that a 4xx response returns an error.
func TestGet_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestDonorboxClient(srv.Listener.Addr().String())
	c.lastCall = c.lastCall.Add(-2)

	_, err := c.get("/missing", nil)
	if err == nil {
		t.Error("expected error for 404 response, got nil")
	}
}

// TestListAllPaginated_SinglePage verifies that a single page with fewer items
// than perPage terminates pagination correctly.
func TestListAllPaginated_SinglePage(t *testing.T) {
	type item struct {
		ID string `json:"id"`
	}
	items := []item{{"a"}, {"b"}, {"c"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(items)
	}))
	defer srv.Close()

	c := newTestDonorboxClient(srv.Listener.Addr().String())
	c.lastCall = c.lastCall.Add(-2)

	got, err := listAllPaginated[item](c, "/campaigns", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(items) {
		t.Errorf("got %d items, want %d", len(got), len(items))
	}
}

// TestListAllPaginated_EmptyPage verifies that an empty first page returns
// an empty slice without error.
func TestListAllPaginated_EmptyPage(t *testing.T) {
	type item struct{ ID string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := newTestDonorboxClient(srv.Listener.Addr().String())
	c.lastCall = c.lastCall.Add(-2)

	got, err := listAllPaginated[item](c, "/donations", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d items", len(got))
	}
}
