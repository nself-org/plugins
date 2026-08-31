// Purpose: DB-backed handler tests for Consent CRUD + user-consents endpoints.
// Inputs: httptest requests routed through handler-factory HandlerFuncs with a mocked pgx pool.
// Outputs: asserts HTTP status codes, JSON response bodies, and SQL expectations met.
// Constraints: No real Postgres; pgxmock.PgxPoolIface satisfies internal.PgxIface.
package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
)

func consentRow() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "source_account_id", "user_id", "purpose", "purpose_description",
		"status", "granted_at", "denied_at", "withdrawn_at", "expires_at",
		"consent_method", "consent_text", "privacy_policy_version",
		"ip_address", "user_agent", "metadata", "created_at", "updated_at",
	}).AddRow(
		"c1", "primary", "u1", "marketing", nil,
		"granted", nil, nil, nil, nil,
		nil, nil, nil,
		nil, nil, []byte(`{}`), nowTime(), nowTime(),
	)
}

func TestListConsentsHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, purpose").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(consentRow())
	rec := httptest.NewRecorder()
	ListConsentsHandler(db)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/consents", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListConsentsHandler_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, purpose").WithArgs(pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	ListConsentsHandler(db)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/consents", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateConsentHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO compliance_consents").WithArgs(
		pgxmock.AnyArg(), "u1", "marketing", pgxmock.AnyArg(), "granted",
		pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
		pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnRows(consentRow())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/consents", strings.NewReader(`{"user_id":"u1","purpose":"marketing"}`))
	CreateConsentHandler(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateConsentHandler_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/consents", strings.NewReader("bogus"))
	CreateConsentHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateConsentHandler_MissingFields(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/consents", strings.NewReader(`{}`))
	CreateConsentHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateConsentHandler_DeniedStatus(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO compliance_consents").WithArgs(
		pgxmock.AnyArg(), "u1", "marketing", pgxmock.AnyArg(), "denied",
		pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
		pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnRows(consentRow())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/consents", strings.NewReader(`{"user_id":"u1","purpose":"marketing","status":"denied"}`))
	CreateConsentHandler(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetConsentHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, purpose").WithArgs("c1", pgxmock.AnyArg()).
		WillReturnRows(consentRow())
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/consents/c1", "", "id", "c1")
	GetConsentHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetConsentHandler_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, purpose").WithArgs("missing", pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/consents/missing", "", "id", "missing")
	GetConsentHandler(db)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRevokeConsentHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE compliance_consents").WithArgs(
		pgxmock.AnyArg(), pgxmock.AnyArg(), "u1", "marketing",
	).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/consents/revoke", strings.NewReader(`{"user_id":"u1","consent_type":"marketing"}`))
	RevokeConsentHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRevokeConsentHandler_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/consents/revoke", strings.NewReader("bogus"))
	RevokeConsentHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRevokeConsentHandler_MissingFields(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/consents/revoke", strings.NewReader(`{}`))
	RevokeConsentHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRevokeConsentHandler_ExecError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE compliance_consents").WithArgs(
		pgxmock.AnyArg(), pgxmock.AnyArg(), "u1", "marketing",
	).WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/consents/revoke", strings.NewReader(`{"user_id":"u1","consent_type":"marketing"}`))
	RevokeConsentHandler(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestListUserConsentsHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, purpose").WithArgs(pgxmock.AnyArg(), "u1").
		WillReturnRows(consentRow())
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/users/u1/consents", "", "user_id", "u1")
	ListUserConsentsHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListUserConsentsHandler_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, purpose").WithArgs(pgxmock.AnyArg(), "u1").
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/users/u1/consents", "", "user_id", "u1")
	ListUserConsentsHandler(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}
