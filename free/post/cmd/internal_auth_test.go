package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestInternalAuth_HeaderNames proves X-Internal-Token and every legacy name carry the same secret (D-0198), a
// wrong value under any name is refused, and a legacy header never overrides a present canonical one.
func TestInternalAuth_HeaderNames(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"canonical", map[string]string{"X-Internal-Token": "s3"}, 200},
		{"legacy X-Plugin-Secret", map[string]string{"X-Plugin-Secret": "s3"}, 200},
		{"legacy X-Internal-Secret", map[string]string{"X-Internal-Secret": "s3"}, 200},
		{"legacy X-Internal-Key", map[string]string{"X-Internal-Key": "s3"}, 200},
		{"wrong canonical is not rescued by a right legacy header", map[string]string{"X-Internal-Token": "bad", "X-Plugin-Secret": "s3"}, 403},
		{"wrong legacy", map[string]string{"X-Plugin-Secret": "bad"}, 403},
		{"no header", nil, 401},
	}
	for _, c := range cases {
		h := internalAuthMiddleware("s3")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != c.want {
			t.Errorf("%s: code %d, want %d", c.name, rr.Code, c.want)
		}
	}
}
