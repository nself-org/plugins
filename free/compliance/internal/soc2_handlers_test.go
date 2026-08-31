// Purpose: DB-backed handler tests for SOC 2 controls, change-log, and access-review endpoints.
// Inputs: httptest requests routed through handler-factory HandlerFuncs with a mocked pgx pool.
// Outputs: asserts HTTP status codes, JSON response bodies, license-gate behavior, and SQL expectations met.
// Constraints: No real Postgres; pgxmock.PgxPoolIface satisfies internal.PgxIface.
package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
)

func licensedReq(method, url, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, url, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	r.Header.Set("X-Nself-License-Tier", "business")
	return r
}

func TestSOC2ControlsHandler_Licensed(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	SOC2ControlsHandler(db)(rec, licensedReq(http.MethodGet, "/api/v1/compliance/soc2/controls", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestSOC2ControlsHandler_Unlicensed(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	SOC2ControlsHandler(db)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/compliance/soc2/controls", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestListChangeLogHandler_Licensed(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, tenant_id, changed_by").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "tenant_id", "changed_by", "change_type", "description",
			"ticket_ref", "approved_by", "applied_at", "rollback_at", "created_at",
		}).AddRow("cl1", nil, nil, "config", "did a thing", nil, nil, nowTime(), nil, nowTime()))
	rec := httptest.NewRecorder()
	ListChangeLogHandler(db)(rec, licensedReq(http.MethodGet, "/api/v1/compliance/change-log", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListChangeLogHandler_Unlicensed(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	ListChangeLogHandler(db)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/compliance/change-log", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestListChangeLogHandler_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, tenant_id, changed_by").WillReturnError(errBoom)
	rec := httptest.NewRecorder()
	ListChangeLogHandler(db)(rec, licensedReq(http.MethodGet, "/api/v1/compliance/change-log", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateChangeLogHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO np_change_log").WithArgs("config", "did a thing", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("cl1"))
	rec := httptest.NewRecorder()
	req := licensedReq(http.MethodPost, "/api/v1/compliance/change-log", `{"change_type":"config","description":"did a thing"}`)
	CreateChangeLogHandler(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateChangeLogHandler_Unlicensed(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	CreateChangeLogHandler(db)(rec, httptest.NewRequest(http.MethodPost, "/api/v1/compliance/change-log", strings.NewReader(`{}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateChangeLogHandler_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	CreateChangeLogHandler(db)(rec, licensedReq(http.MethodPost, "/api/v1/compliance/change-log", "bogus"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateChangeLogHandler_MissingFields(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	CreateChangeLogHandler(db)(rec, licensedReq(http.MethodPost, "/api/v1/compliance/change-log", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestListAccessReviewsHandler_Licensed(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, tenant_id, reviewer_id").
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "tenant_id", "reviewer_id", "period_start", "period_end",
			"status", "findings", "completed_at", "created_at", "updated_at",
		}).AddRow("ar1", nil, nil, "2026-01-01", "2026-03-31", "pending", []byte(`{}`), nil, nowTime(), nowTime()))
	rec := httptest.NewRecorder()
	ListAccessReviewsHandler(db)(rec, licensedReq(http.MethodGet, "/api/v1/compliance/access-reviews", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListAccessReviewsHandler_Unlicensed(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	ListAccessReviewsHandler(db)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/compliance/access-reviews", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateAccessReviewHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO np_access_reviews").WithArgs(pgxmock.AnyArg(), "2026-01-01", "2026-03-31").
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("ar1"))
	rec := httptest.NewRecorder()
	req := licensedReq(http.MethodPost, "/api/v1/compliance/access-reviews", `{"period_start":"2026-01-01","period_end":"2026-03-31"}`)
	CreateAccessReviewHandler(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateAccessReviewHandler_MissingFields(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	CreateAccessReviewHandler(db)(rec, licensedReq(http.MethodPost, "/api/v1/compliance/access-reviews", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPatchAccessReviewHandler_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE np_access_reviews").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), "ar1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPatch, "/api/v1/compliance/access-reviews/ar1", `{"status":"complete"}`, "id", "ar1")
	req.Header.Set("X-Nself-License-Tier", "business")
	PatchAccessReviewHandler(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestPatchAccessReviewHandler_Unlicensed(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPatch, "/api/v1/compliance/access-reviews/ar1", `{}`, "id", "ar1")
	PatchAccessReviewHandler(db)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestPatchAccessReviewHandler_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := reqWithChiParam(http.MethodPatch, "/api/v1/compliance/access-reviews/ar1", "bogus", "id", "ar1")
	req.Header.Set("X-Nself-License-Tier", "business")
	PatchAccessReviewHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateEvidencePackHandler_Unlicensed(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/soc2/evidence", strings.NewReader(`{}`))
	CreateEvidencePackHandler(db)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateEvidencePackHandler_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := licensedReq(http.MethodPost, "/api/v1/compliance/soc2/evidence", "bogus")
	CreateEvidencePackHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateEvidencePackHandler_BadFromDate(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := licensedReq(http.MethodPost, "/api/v1/compliance/soc2/evidence", `{"from":"not-a-date","to":"2026-01-31"}`)
	CreateEvidencePackHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateEvidencePackHandler_BadToDate(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := licensedReq(http.MethodPost, "/api/v1/compliance/soc2/evidence", `{"from":"2026-01-01","to":"not-a-date"}`)
	CreateEvidencePackHandler(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}
