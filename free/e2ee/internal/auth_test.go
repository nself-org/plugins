package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPrincipalFrom_HeaderExtraction verifies the gateway-forwarded principal is
// read from the canonical headers, with the source account defaulting to
// 'primary' and the user id read from X-Hasura-User-Id.
func TestPrincipalFrom_HeaderExtraction(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Hasura-User-Id", "alice")
	r.Header.Set("X-Source-Account-ID", "tenantA")
	r.Header.Set("X-Hasura-Role", "user")

	p := principalFrom(r)
	if p.UserID != "alice" {
		t.Fatalf("user id: want alice got %q", p.UserID)
	}
	if p.SourceAccount != "tenantA" {
		t.Fatalf("source account: want tenantA got %q", p.SourceAccount)
	}
	if p.Role != "user" {
		t.Fatalf("role: want user got %q", p.Role)
	}
}

// TestPrincipalFrom_DefaultSourceAccount confirms the isolation key defaults to
// 'primary' when no source-account header is present.
func TestPrincipalFrom_DefaultSourceAccount(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Hasura-User-Id", "bob")
	p := principalFrom(r)
	if p.SourceAccount != "primary" {
		t.Fatalf("default source account: want primary got %q", p.SourceAccount)
	}
}

// TestPrincipalFrom_AltSourceAccountHeaders confirms the alternate spellings of
// the source-account header are accepted in precedence order.
func TestPrincipalFrom_AltSourceAccountHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Hasura-User-Id", "carol")
	r.Header.Set("X-Hasura-Source-Account-Id", "tenantC")
	if got := principalFrom(r).SourceAccount; got != "tenantC" {
		t.Fatalf("alt source-account header: want tenantC got %q", got)
	}
}

// TestRequireAuth_MissingHeaderRejected proves the middleware FAILS CLOSED: a
// request with no authenticated user id is rejected 401 and never reaches the
// wrapped handler.
func TestRequireAuth_MissingHeaderRejected(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/e2ee/audit/alice", nil)
	RequireAuth(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth header: want 401 got %d", rec.Code)
	}
	if called {
		t.Fatal("handler must NOT run when unauthenticated")
	}
}

// TestRequireAuth_PassesPrincipalToHandler proves an authenticated request
// reaches the handler with the principal available on the context.
func TestRequireAuth_PassesPrincipalToHandler(t *testing.T) {
	var seen Principal
	var ok bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, ok = principalOf(r.Context())
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Hasura-User-Id", "alice")
	RequireAuth(next).ServeHTTP(rec, req)

	if !ok || seen.UserID != "alice" {
		t.Fatalf("principal not propagated: ok=%v principal=%+v", ok, seen)
	}
}

// TestRegisterIdentity_CrossUserRejected proves a caller authenticated as alice
// cannot register keys for bob: the ownership check returns 403 BEFORE any DB
// access, so no nil-pool dereference occurs (a real DB is not required here).
func TestRegisterIdentity_CrossUserRejected(t *testing.T) {
	h := &Handlers{db: nil, cfg: LoadConfig()}

	body, _ := json.Marshal(RegisterIdentityRequest{
		UserID:            "bob", // attacker targets a different user
		DeviceID:          "d1",
		IdentityKeyPublic: "",
		RegistrationID:    1,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/e2ee/identity/register", bytes.NewReader(body))
	req.Header.Set("X-Hasura-User-Id", "alice")
	req = req.WithContext(injectPrincipal(req, "alice", "primary"))

	rec := httptest.NewRecorder()
	h.RegisterIdentity(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-user register: want 403 got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestUploadSignedPreKey_CrossUserRejected proves the same ownership guard on
// the signed-prekey write path.
func TestUploadSignedPreKey_CrossUserRejected(t *testing.T) {
	h := &Handlers{db: nil, cfg: LoadConfig()}

	body, _ := json.Marshal(SignedPreKeyUpload{
		UserID:    "victim",
		DeviceID:  "d1",
		KeyID:     1,
		PublicKey: "AA==",
		Signature: "AA==",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/e2ee/signed-prekey", bytes.NewReader(body))
	req = req.WithContext(injectPrincipal(req, "attacker", "primary"))

	rec := httptest.NewRecorder()
	h.UploadSignedPreKey(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-user signed-prekey: want 403 got %d", rec.Code)
	}
}

// TestPostSafetyNumber_CrossUserRejected proves the ownership guard on the
// safety-number write path.
func TestPostSafetyNumber_CrossUserRejected(t *testing.T) {
	h := &Handlers{db: nil, cfg: LoadConfig()}

	body, _ := json.Marshal(SafetyNumberRequest{
		UserID:       "victim",
		PeerUserID:   "peer",
		SafetyNumber: "12345",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/e2ee/safety-number", bytes.NewReader(body))
	req = req.WithContext(injectPrincipal(req, "attacker", "primary"))

	rec := httptest.NewRecorder()
	h.PostSafetyNumber(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-user safety-number: want 403 got %d", rec.Code)
	}
}

// TestHandlersWithoutPrincipal_Rejected proves a handler invoked WITHOUT the
// RequireAuth middleware (no principal on context) fails closed with 401 rather
// than trusting the request body.
func TestHandlersWithoutPrincipal_Rejected(t *testing.T) {
	h := &Handlers{db: nil, cfg: LoadConfig()}
	body, _ := json.Marshal(RegisterIdentityRequest{UserID: "alice", DeviceID: "d1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/e2ee/identity/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.RegisterIdentity(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no principal on context: want 401 got %d", rec.Code)
	}
}

// injectPrincipal returns a context carrying a Principal, simulating what the
// RequireAuth middleware installs, so handler-level ownership checks can be
// unit-tested without spinning up the full router.
func injectPrincipal(r *http.Request, userID, sourceAccount string) context.Context {
	return context.WithValue(r.Context(), principalCtxKey{}, Principal{
		UserID:        userID,
		SourceAccount: sourceAccount,
	})
}
