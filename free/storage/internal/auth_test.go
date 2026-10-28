package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireInternalSecret(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		header     *string // nil = header absent
		wantCode   int
		wantBody   string
		wantNext   bool
	}{
		{"no header", testSecret, nil, 401, `{"error":"unauthorized"}`, false},
		{"empty header", testSecret, ptr(""), 401, `{"error":"unauthorized"}`, false},
		{"wrong secret", testSecret, ptr("test-wrong-secret"), 401, `{"error":"unauthorized"}`, false},
		{"shorter secret", testSecret, ptr("test"), 401, `{"error":"unauthorized"}`, false},
		{"longer secret", testSecret, ptr(testSecret + "-and-more"), 401, `{"error":"unauthorized"}`, false},
		{"right secret", testSecret, ptr(testSecret), 200, "", true},
		{"unset, no header", "", nil, 503, `{"error":"PLUGIN_INTERNAL_SECRET not configured"}`, false},
		{"unset, empty header", "", ptr(""), 503, `{"error":"PLUGIN_INTERNAL_SECRET not configured"}`, false},
		{"unset, any header", "", ptr("test-anything"), 503, `{"error":"PLUGIN_INTERNAL_SECRET not configured"}`, false},
	}
	for _, c := range cases {
		reached := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached = true })
		s := &Server{cfg: &Config{InternalSecret: c.configured}}
		req := httptest.NewRequest(http.MethodGet, "/storage/buckets", nil)
		if c.header != nil {
			req.Header.Set(secretHeader, *c.header)
		}
		rr := httptest.NewRecorder()
		s.requireInternalSecret(next).ServeHTTP(rr, req)
		if rr.Code != c.wantCode || reached != c.wantNext || (c.wantBody != "" && !strings.Contains(rr.Body.String(), c.wantBody)) {
			t.Errorf("%s: code %d (want %d), next reached %v (want %v), body %q", c.name, rr.Code, c.wantCode, reached, c.wantNext, rr.Body.String())
		}
	}
}

func TestRequireInternalSecret_nilConfigFailsClosed(t *testing.T) {
	rr := do(rawRouter(&Server{}), http.MethodGet, "/storage/buckets", "")
	if rr.Code != 503 {
		t.Fatalf("nil cfg: %d", rr.Code)
	}
}

// A forged account header without the secret must be refused before any handler, DB or S3 call runs.
// The store is a nil embedded interface: any call into it panics and fails the test.
func TestStorageRoutes_forgedAccountHeaderNeverReachesHandler(t *testing.T) {
	h := rawRouter(&Server{cfg: &Config{InternalSecret: testSecret}, store: &fakeStore{}})
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/storage/buckets"},
		{http.MethodGet, "/storage/buckets"},
		{http.MethodDelete, "/storage/buckets/bkt"},
		{http.MethodPut, "/storage/buckets/bkt/objects/k"},
		{http.MethodGet, "/storage/buckets/bkt/objects/k"},
		{http.MethodDelete, "/storage/buckets/bkt/objects/k"},
		{http.MethodGet, "/storage/buckets/bkt/objects"},
	} {
		for _, secret := range []*string{nil, ptr("test-wrong-secret")} {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"name":"good-name"}`))
			req.Header.Set("X-Hasura-Source-Account-Id", "test-victim-account")
			if secret != nil {
				req.Header.Set(secretHeader, *secret)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: %d %s", tc.method, tc.path, rr.Code, rr.Body.String())
			}
		}
	}
	// The secret is checked before the account header: a bad account id with no secret is 401, not 400.
	req := httptest.NewRequest(http.MethodGet, "/storage/buckets", nil)
	req.Header["X-Hasura-Source-Account-Id"] = []string{".."}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("bad account id without secret: %d", rr.Code)
	}
}

func ptr(s string) *string { return &s }

// D-0198: the canonical X-Internal-Token and every legacy name carry the same secret; a legacy header never
// overrides a present canonical one, and a wrong value under any name is refused.
func TestRequireInternalSecret_headerNames(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"canonical", map[string]string{"X-Internal-Token": testSecret}, 200},
		{"legacy X-Plugin-Secret", map[string]string{"X-Plugin-Secret": testSecret}, 200},
		{"legacy X-Internal-Secret", map[string]string{"X-Internal-Secret": testSecret}, 200},
		{"legacy X-Internal-Key", map[string]string{"X-Internal-Key": testSecret}, 200},
		{"wrong canonical is not rescued by a right legacy header", map[string]string{"X-Internal-Token": "test-wrong-secret", "X-Plugin-Secret": testSecret}, 401},
		{"wrong legacy", map[string]string{"X-Plugin-Secret": "test-wrong-secret"}, 401},
	}
	for _, c := range cases {
		s := &Server{cfg: &Config{InternalSecret: testSecret}}
		req := httptest.NewRequest(http.MethodGet, "/storage/buckets", nil)
		for k, v := range c.headers {
			req.Header.Set(k, v)
		}
		rr := httptest.NewRecorder()
		s.requireInternalSecret(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rr, req)
		if rr.Code != c.want {
			t.Errorf("%s: code %d, want %d", c.name, rr.Code, c.want)
		}
	}
}
