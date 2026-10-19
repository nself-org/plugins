package rest

// Purpose: tests of Client.DoQuery (P7-HOST-04 lead ruling: the funnel had no way to send a query,
// so a paged list such as the runner list could not be read).
// Inputs:  a local TLS server and the synthetic values below.
// Outputs: pass or fail.
// Constraints: no real network; the values carry no secret.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestDoQuery proves the query reaches the server encoded, that Do sends none, and that a "?" in a
// path template or a parameter value still cannot form a query of its own.
func TestDoQuery(t *testing.T) {
	var gotPath, gotQuery string
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{}`))
	}))
	defer s.Close()
	c := testClient(s)
	q := url.Values{"per_page": {"100"}, "page": {"2"}, "name": {"a b&c"}}
	if _, err := c.DoQuery(context.Background(), "GET", "/runners", nil, q, nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/runners" || gotQuery != "name=a+b%26c&page=2&per_page=100" {
		t.Fatalf("path=%q query=%q", gotPath, gotQuery)
	}
	if _, err := c.Do(context.Background(), "GET", "/runners", nil, nil, nil); err != nil || gotQuery != "" {
		t.Fatalf("Do sent a query %q (%v)", gotQuery, err)
	}
	_, err := c.DoQuery(context.Background(), "GET", "/runners?x=1", nil, nil, nil, nil)
	assertError(t, err, "E702", "request.path")
	if _, err = c.DoQuery(context.Background(), "GET", "/runners/{id}", map[string]string{"id": "1?x=1"}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "" {
		t.Fatalf("a parameter formed a query: %q", gotQuery)
	}
}
