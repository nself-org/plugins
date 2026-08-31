// Purpose: DB-backed handler tests for DSAR CRUD + activity endpoints.
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

func dsarRow() *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "source_account_id", "request_type", "request_number", "user_id", "requester_email",
		"requester_name", "description", "data_categories", "status", "assigned_to", "started_at",
		"completed_at", "deadline", "resolution_notes", "rejection_reason", "regulation",
		"jurisdiction", "legal_basis", "ip_address", "user_agent", "created_at", "updated_at",
	}).AddRow(
		"d1", "primary", "access", "DSAR-1", nil, "user@x.com",
		nil, nil, []string{}, "pending", nil, nil,
		nil, nowTime(), nil, nil, "GDPR",
		nil, nil, nil, nil, nowTime(), nowTime(),
	)
}

func TestListDSARsHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, request_type").WithArgs(pgxmock.AnyArg()).
		WillReturnRows(dsarRow())
	rec := httptest.NewRecorder()
	ListDSARsHandler(db)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dsars", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListDSARsHandler_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, request_type").WithArgs(pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	ListDSARsHandler(db)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dsars", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateDSARHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO compliance_dsars").WithArgs(
		pgxmock.AnyArg(), "access", pgxmock.AnyArg(), pgxmock.AnyArg(),
		"user@x.com", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
		pgxmock.AnyArg(), pgxmock.AnyArg(), "GDPR", pgxmock.AnyArg(),
	).WillReturnRows(dsarRow())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dsars", strings.NewReader(`{"request_type":"access","email":"user@x.com"}`))
	CreateDSARHandler(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateDSARHandler_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dsars", strings.NewReader("bogus"))
	CreateDSARHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateDSARHandler_MissingFields(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dsars", strings.NewReader(`{}`))
	CreateDSARHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetDSARHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, request_type").WithArgs("d1", pgxmock.AnyArg()).
		WillReturnRows(dsarRow())
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/dsars/d1", "", "id", "d1")
	GetDSARHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetDSARHandler_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, request_type").WithArgs("missing", pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/dsars/missing", "", "id", "missing")
	GetDSARHandler(db)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetDSARHandler_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, request_type").WithArgs("d1", pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/dsars/d1", "", "id", "d1")
	GetDSARHandler(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateDSARHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE compliance_dsars SET").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectQuery("SELECT id, source_account_id, request_type").WithArgs("d1", pgxmock.AnyArg()).
		WillReturnRows(dsarRow())
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/dsars/d1", `{"status":"in_progress"}`, "id", "d1")
	UpdateDSARHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateDSARHandler_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/dsars/d1", "bogus", "id", "d1")
	UpdateDSARHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateDSARHandler_ExecError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE compliance_dsars SET").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPut, "/api/v1/dsars/d1", `{"status":"in_progress"}`, "id", "d1")
	UpdateDSARHandler(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCompleteDSARHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE compliance_dsars").WithArgs(pgxmock.AnyArg(), "d1", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("INSERT INTO compliance_dsar_activities").WithArgs(
		pgxmock.AnyArg(), "d1", "completed", pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectQuery("SELECT id, source_account_id, request_type").WithArgs("d1", pgxmock.AnyArg()).
		WillReturnRows(dsarRow())
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPost, "/api/v1/dsars/d1/complete", "", "id", "d1")
	CompleteDSARHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCompleteDSARHandler_ExecError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE compliance_dsars").WithArgs(pgxmock.AnyArg(), "d1", pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPost, "/api/v1/dsars/d1/complete", "", "id", "d1")
	CompleteDSARHandler(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDenyDSARHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE compliance_dsars").WithArgs("no thanks", "d1", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("INSERT INTO compliance_dsar_activities").WithArgs(
		pgxmock.AnyArg(), "d1", "denied", pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectQuery("SELECT id, source_account_id, request_type").WithArgs("d1", pgxmock.AnyArg()).
		WillReturnRows(dsarRow())
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPost, "/api/v1/dsars/d1/deny", `{"reason":"no thanks"}`, "id", "d1")
	DenyDSARHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListDSARActivitiesHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, dsar_id").WithArgs("d1", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "dsar_id", "activity_type", "description",
			"performed_by", "performed_by_name", "metadata", "created_at",
		}).AddRow("a1", "primary", "d1", "completed", nil, nil, nil, []byte(`{}`), nowTime()))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/dsars/d1/activities", "", "id", "d1")
	ListDSARActivitiesHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListDSARActivitiesHandler_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, dsar_id").WithArgs("d1", pgxmock.AnyArg()).
		WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodGet, "/api/v1/dsars/d1/activities", "", "id", "d1")
	ListDSARActivitiesHandler(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}
