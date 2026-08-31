package main

// handlers_auth_test.go — /v1/signup (password path), /v1/login,
// /v1/session, and the per-IP rate limiter.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

const testJWTSecret = "test-saas-jwt-secret"

// newAuthGateway builds a gateway with a JWT secret + sqlmock DB.
func newAuthGateway(t *testing.T) (*gateway, sqlmock.Sqlmock) {
	t.Helper()
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	g.cfg.JWTSecret = []byte(testJWTSecret)
	return g, mock
}

// doPlainReq runs a request WITHOUT the tenant header (public auth surface).
func doPlainReq(t *testing.T, g *gateway, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	return rec
}

func TestSignupWithPasswordIssuesToken(t *testing.T) {
	g, mock := newAuthGateway(t)
	mock.ExpectExec("INSERT INTO np_saas_tenants").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO np_saas_api_keys").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("k-1"))

	rec := doPlainReq(t, g, http.MethodPost, "/v1/signup",
		`{"email":"owner@example.com","password":"hunter2hunter2","name":"Owner"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		TenantID string `json:"tenant_id"`
		APIKey   string `json:"api_key"`
		Token    string `json:"token"`
	}
	decodeBody(t, rec, &resp)
	if !strings.HasPrefix(resp.APIKey, "nsk_") {
		t.Errorf("api_key %q missing nsk_ prefix", resp.APIKey)
	}
	if resp.Token == "" {
		t.Fatal("password signup did not return a session token")
	}
	// The token must verify against the same secret and carry the tenant.
	tid, err := saas.VerifyJWTTenant(resp.Token, []byte(testJWTSecret))
	if err != nil {
		t.Fatalf("VerifyJWTTenant: %v", err)
	}
	if tid != resp.TenantID {
		t.Errorf("token tenant = %q, want %q", tid, resp.TenantID)
	}
}

func TestSignupWithoutPasswordUnchanged(t *testing.T) {
	g, mock := newAuthGateway(t)
	mock.ExpectExec("INSERT INTO np_saas_tenants").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO np_saas_api_keys").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("k-1"))

	rec := doPlainReq(t, g, http.MethodPost, "/v1/signup", `{"email":"cli@example.com"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	decodeBody(t, rec, &resp)
	if _, hasToken := resp["token"]; hasToken {
		t.Error("passwordless signup must not mint a session token")
	}
	if _, hasKey := resp["api_key"]; !hasKey {
		t.Error("passwordless signup lost its api_key")
	}
}

func TestSignupShortPasswordRejected(t *testing.T) {
	g, _ := newAuthGateway(t)
	rec := doPlainReq(t, g, http.MethodPost, "/v1/signup",
		`{"email":"owner@example.com","password":"short"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("short password = %d, want 422", rec.Code)
	}
}

func TestSignupDuplicateEmailConflict(t *testing.T) {
	g, mock := newAuthGateway(t)
	mock.ExpectExec("INSERT INTO np_saas_tenants").
		WillReturnError(errDuplicateKey{})

	rec := doPlainReq(t, g, http.MethodPost, "/v1/signup",
		`{"email":"owner@example.com","password":"hunter2hunter2"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate email = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

// errDuplicateKey mimics lib/pq's unique-violation error text.
type errDuplicateKey struct{}

func (errDuplicateKey) Error() string {
	return `pq: duplicate key value violates unique constraint "idx_np_saas_tenants_owner_email_login"`
}

func loginRows(t *testing.T, password string) *sqlmock.Rows {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return sqlmock.NewRows([]string{"tenant_id", "tier", "owner_name", "owner_password_hash"}).
		AddRow(testTenant, "free", "Owner", string(hash))
}

func TestLoginSuccessReturnsVerifiableJWT(t *testing.T) {
	g, mock := newAuthGateway(t)
	mock.ExpectQuery("SELECT tenant_id, tier, owner_name, owner_password_hash").
		WithArgs("owner@example.com").
		WillReturnRows(loginRows(t, "hunter2hunter2"))

	rec := doPlainReq(t, g, http.MethodPost, "/v1/login",
		`{"email":"Owner@Example.com","password":"hunter2hunter2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token    string `json:"token"`
		TenantID string `json:"tenant_id"`
		Tier     string `json:"tier"`
		Email    string `json:"email"`
	}
	decodeBody(t, rec, &resp)
	if resp.TenantID != testTenant || resp.Tier != "free" || resp.Email != "owner@example.com" {
		t.Errorf("unexpected identity: %+v", resp)
	}
	// JWT verifies with the shared secret and carries the tenant claim.
	tid, err := saas.VerifyJWTTenant(resp.Token, []byte(testJWTSecret))
	if err != nil || tid != testTenant {
		t.Fatalf("token verify: tid=%q err=%v", tid, err)
	}
	// And its claims echo through /v1/session.
	req := httptest.NewRequest(http.MethodGet, "/v1/session", nil)
	req.Header.Set("Authorization", "Bearer "+resp.Token)
	sessionRec := httptest.NewRecorder()
	g.router().ServeHTTP(sessionRec, req)
	if sessionRec.Code != http.StatusOK {
		t.Fatalf("session = %d, want 200: %s", sessionRec.Code, sessionRec.Body.String())
	}
	var sess struct {
		Authenticated bool   `json:"authenticated"`
		TenantID      string `json:"tenant_id"`
		Email         string `json:"email"`
	}
	decodeBody(t, sessionRec, &sess)
	if !sess.Authenticated || sess.TenantID != testTenant || sess.Email != "owner@example.com" {
		t.Errorf("unexpected session: %+v", sess)
	}
}

func TestLoginBadPassword(t *testing.T) {
	g, mock := newAuthGateway(t)
	mock.ExpectQuery("SELECT tenant_id, tier, owner_name, owner_password_hash").
		WithArgs("owner@example.com").
		WillReturnRows(loginRows(t, "correct-password"))

	rec := doPlainReq(t, g, http.MethodPost, "/v1/login",
		`{"email":"owner@example.com","password":"wrong-password"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad password = %d, want 401", rec.Code)
	}
}

func TestLoginUnknownEmail(t *testing.T) {
	g, mock := newAuthGateway(t)
	mock.ExpectQuery("SELECT tenant_id, tier, owner_name, owner_password_hash").
		WithArgs("nobody@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "tier", "owner_name", "owner_password_hash"}))

	rec := doPlainReq(t, g, http.MethodPost, "/v1/login",
		`{"email":"nobody@example.com","password":"whatever123"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown email = %d, want 401", rec.Code)
	}
}

func TestLoginDisabledWithoutSecret(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	g.cfg.JWTSecret = nil
	rec := doPlainReq(t, g, http.MethodPost, "/v1/login",
		`{"email":"a@b.co","password":"whatever123"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no-secret login = %d, want 503", rec.Code)
	}
}

func TestSessionRejectsBadToken(t *testing.T) {
	g, _ := newAuthGateway(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/session", nil)
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token session = %d, want 401", rec.Code)
	}
}

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(3, time.Minute)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4", now) {
			t.Fatalf("request %d unexpectedly limited", i+1)
		}
	}
	if l.allow("1.2.3.4", now) {
		t.Error("4th request should be limited")
	}
	if !l.allow("5.6.7.8", now) {
		t.Error("other IP must not be limited")
	}
	if !l.allow("1.2.3.4", now.Add(2*time.Minute)) {
		t.Error("new window must reset the counter")
	}
}
