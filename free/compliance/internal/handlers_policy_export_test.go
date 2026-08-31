// Purpose: DB-backed handler tests for PrivacyPolicy, DataExport, and Stats endpoints.
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

func policyRow() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "source_account_id", "version", "version_number", "title", "content",
		"summary", "changes_summary", "is_active", "requires_reacceptance",
		"effective_from", "effective_until", "language", "jurisdiction",
		"created_by", "created_at",
	}).AddRow(
		"p1", "primary", "v1", 1, "Privacy Policy", "content",
		nil, nil, true, false,
		nowTime(), nil, "en", nil,
		nil, nowTime(),
	)
}

func TestListPrivacyPoliciesHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, version, version_number").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(policyRow())
	rec := httptest.NewRecorder()
	ListPrivacyPoliciesHandler(db)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/privacy-policies", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListPrivacyPoliciesHandler_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, version, version_number").WithArgs(pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	ListPrivacyPoliciesHandler(db)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/privacy-policies", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreatePrivacyPolicyHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO compliance_privacy_policies").WithArgs(
		pgxmock.AnyArg(), "v1", 1, "Privacy Policy", "content",
		pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
		pgxmock.AnyArg(), "en", pgxmock.AnyArg(),
	).WillReturnRows(policyRow())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy-policies", strings.NewReader(
		`{"version":"v1","version_number":1,"title":"Privacy Policy","content":"content"}`))
	CreatePrivacyPolicyHandler(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatePrivacyPolicyHandler_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy-policies", strings.NewReader("bogus"))
	CreatePrivacyPolicyHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreatePrivacyPolicyHandler_MissingFields(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy-policies", strings.NewReader(`{}`))
	CreatePrivacyPolicyHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetPrivacyPolicyHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, version, version_number").WithArgs("p1", pgxmock.AnyArg()).
		WillReturnRows(policyRow())
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/privacy-policies/p1", "", "id", "p1")
	GetPrivacyPolicyHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetPrivacyPolicyHandler_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, version, version_number").WithArgs("missing", pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/privacy-policies/missing", "", "id", "missing")
	GetPrivacyPolicyHandler(db)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

// ---- Data Exports ----

func dataExportRow() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "source_account_id", "user_id", "format", "status",
		"export_url", "expires_at", "error_message",
		"requested_at", "completed_at", "created_at",
	}).AddRow(
		"e1", "primary", "u1", "json", "pending",
		nil, nil, nil,
		nowTime(), nil, nowTime(),
	)
}

func TestCreateDataExportHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO compliance_data_exports").WithArgs(
		pgxmock.AnyArg(), "u1", "json", pgxmock.AnyArg(),
	).WillReturnRows(dataExportRow())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/data-exports", strings.NewReader(`{"user_id":"u1"}`))
	CreateDataExportHandler(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateDataExportHandler_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/data-exports", strings.NewReader("bogus"))
	CreateDataExportHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateDataExportHandler_MissingUserID(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/data-exports", strings.NewReader(`{}`))
	CreateDataExportHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetDataExportHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, format, status").WithArgs("e1", pgxmock.AnyArg()).
		WillReturnRows(dataExportRow())
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/data-exports/e1", "", "id", "e1")
	GetDataExportHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetDataExportHandler_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, format, status").WithArgs("missing", pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/data-exports/missing", "", "id", "missing")
	GetDataExportHandler(db)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

// ---- Stats ----

func TestStatsHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	for i := 0; i < 11; i++ {
		mock.ExpectQuery("SELECT COUNT").WithArgs(pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	}
	rec := httptest.NewRecorder()
	StatsHandler(db)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
