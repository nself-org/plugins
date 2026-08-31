package main

// handlers_authflows_test.go — email verification + password reset (sqlmock
// + fake sender): token lifecycle, signup integration, anti-enumeration.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/nself-org/plugins-pro/paid/shared/email"
	"golang.org/x/crypto/bcrypt"
)

const flowTenant = "fefefefe-1234-4321-8765-565656565656"

func TestVerifyEmailConsumesTokenAndSetsVerified(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	raw := strings.Repeat("a", 64)
	mock.ExpectQuery(`UPDATE np_saas_email_tokens`).
		WithArgs(sha256Hex(raw), "verify").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(flowTenant))
	mock.ExpectExec(`UPDATE np_saas_tenants SET verified = true`).
		WithArgs(flowTenant).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPost, "/v1/verify-email",
		strings.NewReader(`{"token":"`+raw+`"}`))
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify = %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestVerifyEmailInvalidToken400(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`UPDATE np_saas_email_tokens`).
		WithArgs(sqlmock.AnyArg(), "verify").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"})) // no live token

	req := httptest.NewRequest(http.MethodPost, "/v1/verify-email",
		strings.NewReader(`{"token":"expired-or-bogus"}`))
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad token = %d, want 400", rec.Code)
	}
}

// TestSignupSendsVerifyEmail — signup with a configured sender mints a
// verify token (hash stored) and emails the verify-email template carrying
// the RAW token.
func TestSignupSendsVerifyEmail(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	g.cfg.DashboardBaseURL = "https://sentry.nself.org"
	sender := &fakeSender{}
	g.mail = sender

	mock.ExpectExec(`INSERT INTO np_saas_tenants`).
		WithArgs(sqlmock.AnyArg(), "new@example.com", "", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO np_saas_api_keys`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "default", "full").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("k-1"))
	mock.ExpectExec(`INSERT INTO np_saas_email_tokens`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "verify", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPost, "/v1/signup",
		strings.NewReader(`{"email":"new@example.com"}`))
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup = %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		VerifyEmailSent bool `json:"verify_email_sent"`
	}
	decodeBody(t, rec, &resp)
	if !resp.VerifyEmailSent {
		t.Error("verify_email_sent = false with a working sender")
	}
	if len(sender.tmpl) != 1 || sender.tmpl[0] != email.TemplateVerifyEmail {
		t.Fatalf("templates sent = %v", sender.tmpl)
	}
	if !strings.Contains(sender.data[0]["VerifyURL"], "/verify-email?token=") {
		t.Errorf("VerifyURL = %q", sender.data[0]["VerifyURL"])
	}
}

// TestForgotPasswordAntiEnumeration — known and unknown emails return the
// IDENTICAL 202 body; only the known one sends mail.
func TestForgotPasswordAntiEnumeration(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	g.cfg.DashboardBaseURL = "https://sentry.nself.org"
	sender := &fakeSender{}
	g.mail = sender

	// Known password-login account → token + email.
	mock.ExpectQuery(`FROM np_saas_tenants\s+WHERE owner_email = \$1 AND owner_password_hash IS NOT NULL`).
		WithArgs("known@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "owner_name"}).
			AddRow(flowTenant, "Known"))
	mock.ExpectExec(`INSERT INTO np_saas_email_tokens`).
		WithArgs(sqlmock.AnyArg(), flowTenant, "reset", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPost, "/v1/password/forgot",
		strings.NewReader(`{"email":"known@example.com"}`))
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("forgot known = %d: %s", rec.Code, rec.Body.String())
	}
	knownBody := rec.Body.String()
	if len(sender.tmpl) != 1 || sender.tmpl[0] != email.TemplatePasswordReset {
		t.Fatalf("reset email = %v", sender.tmpl)
	}
	if !strings.Contains(sender.data[0]["ResetURL"], "/reset-password?token=") {
		t.Errorf("ResetURL = %q", sender.data[0]["ResetURL"])
	}

	// Unknown email → same 202 body, no mail.
	mock.ExpectQuery(`FROM np_saas_tenants\s+WHERE owner_email = \$1 AND owner_password_hash IS NOT NULL`).
		WithArgs("ghost@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "owner_name"}))
	req = httptest.NewRequest(http.MethodPost, "/v1/password/forgot",
		strings.NewReader(`{"email":"ghost@example.com"}`))
	rec = httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("forgot unknown = %d", rec.Code)
	}
	if rec.Body.String() != knownBody {
		t.Errorf("202 bodies differ (enumeration signal): %q vs %q", rec.Body.String(), knownBody)
	}
	if len(sender.tmpl) != 1 {
		t.Errorf("unknown email triggered a send: %v", sender.tmpl)
	}
}

func TestPasswordResetConsumesTokenAndSetsHash(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	raw := strings.Repeat("b", 64)
	mock.ExpectQuery(`UPDATE np_saas_email_tokens`).
		WithArgs(sha256Hex(raw), "reset").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(flowTenant))

	var storedHash string
	mock.ExpectExec(`UPDATE np_saas_tenants SET owner_password_hash`).
		WithArgs(sqlmock.AnyArg(), flowTenant).
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPost, "/v1/password/reset",
		strings.NewReader(`{"token":"`+raw+`","password":"new-passw0rd!"}`))
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset = %d: %s", rec.Code, rec.Body.String())
	}
	_ = storedHash
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestPasswordResetWeakPassword422(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodPost, "/v1/password/reset",
		strings.NewReader(`{"token":"x","password":"short"}`))
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("weak password = %d, want 422", rec.Code)
	}
}

func TestPasswordResetInvalidToken400(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	mock.ExpectQuery(`UPDATE np_saas_email_tokens`).
		WithArgs(sqlmock.AnyArg(), "reset").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}))
	req := httptest.NewRequest(http.MethodPost, "/v1/password/reset",
		strings.NewReader(`{"token":"bogus","password":"long-enough-pw"}`))
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bogus token = %d, want 400", rec.Code)
	}
}

// TestPasswordHashRoundTrip sanity-checks the bcrypt helper used by reset.
func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := hashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(h), []byte("correct horse battery")) != nil {
		t.Error("hash does not verify")
	}
}
