// Purpose: Behavioral tests for handlers.go DB-backed paths, using pgxmock
//
//	to substitute a real Postgres pool via the PgxIface seam on DB.Pool.
//
// Inputs: httptest requests dispatched to the handler HandlerFuncs, with
//
//	pgxmock expectations for the underlying SQL.
//
// Outputs: asserts HTTP status codes + JSON response shape for success and
//
//	error (query/scan/insert/update/delete failure, not-found, bad-JSON) paths.
//
// Constraints: No live DB. Query text is matched via regexp substrings
//
//	(pgxmock default), so SQL can be reformatted without breaking
//	these tests as long as key clauses are preserved.
package internal

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
)

func newMockDB(t *testing.T) (*DB, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return &DB{Pool: mock}, mock
}

func withURLParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

var verificationCols = []string{
	"id", "source_account_id", "user_id", "idme_uuid", "email", "verified", "verification_level",
	"first_name", "last_name", "birth_date", "zip", "phone", "access_token", "refresh_token",
	"token_expires_at", "verified_at", "last_synced_at", "created_at", "updated_at",
}

func verificationRow(mock pgxmock.PgxPoolIface, id string) *pgxmock.Rows {
	now := time.Now()
	return pgxmock.NewRows(verificationCols).AddRow(
		id, "primary", "user-1", (*string)(nil), "", false, (*string)(nil),
		(*string)(nil), (*string)(nil), (*time.Time)(nil), (*string)(nil), (*string)(nil),
		(*string)(nil), (*string)(nil), (*time.Time)(nil), (*time.Time)(nil), now, now, now,
	)
}

// --- CreateVerification ---

func TestCreateVerification_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO idme_verifications").
		WithArgs(pgxmock.AnyArg(), "user-1", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(verificationRow(mock, "v1"))

	body := bytes.NewBufferString(`{"user_id":"user-1"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", body)
	CreateVerification(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestCreateVerification_BadJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewBufferString(`{bad`))
	CreateVerification(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateVerification_MissingUserID(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewBufferString(`{}`))
	CreateVerification(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateVerification_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO idme_verifications").
		WithArgs(pgxmock.AnyArg(), "user-1", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications", bytes.NewBufferString(`{"user_id":"user-1"}`))
	CreateVerification(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- ListVerifications ---

func TestListVerifications_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs("primary").
		WillReturnRows(verificationRow(mock, "v1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/verifications", nil)
	ListVerifications(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListVerifications_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs("primary").
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/verifications", nil)
	ListVerifications(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestListVerifications_ScanError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs("primary").
		WillReturnRows(pgxmock.NewRows(verificationCols).AddRow(
			// wrong type in place of bool -> scan error
			"v1", "primary", "user-1", (*string)(nil), "", "not-a-bool", (*string)(nil),
			(*string)(nil), (*string)(nil), (*time.Time)(nil), (*string)(nil), (*string)(nil),
			(*string)(nil), (*string)(nil), (*time.Time)(nil), (*time.Time)(nil), time.Now(), time.Now(), time.Now(),
		))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/verifications", nil)
	ListVerifications(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

// --- GetVerification ---

func TestGetVerification_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs("v1", "primary").
		WillReturnRows(verificationRow(mock, "v1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/verifications/v1", nil)
	req = withURLParam(req, "id", "v1")
	GetVerification(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetVerification_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs("missing", "primary").
		WillReturnError(pgx.ErrNoRows)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/verifications/missing", nil)
	req = withURLParam(req, "id", "missing")
	GetVerification(db)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetVerification_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs("v1", "primary").
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/verifications/v1", nil)
	req = withURLParam(req, "id", "v1")
	GetVerification(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- VerificationCallback ---

func TestVerificationCallback_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO idme_verifications").
		WithArgs("primary", "user-1", pgxmock.AnyArg()).
		WillReturnRows(verificationRow(mock, "v1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications/callback",
		bytes.NewBufferString(`{"code":"abc","user_id":"user-1"}`))
	VerificationCallback(db, Config{})(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestVerificationCallback_BadJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications/callback", bytes.NewBufferString(`{bad`))
	VerificationCallback(db, Config{})(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestVerificationCallback_MissingCode(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications/callback", bytes.NewBufferString(`{}`))
	VerificationCallback(db, Config{})(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestVerificationCallback_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO idme_verifications").
		WithArgs("primary", "user-1", pgxmock.AnyArg()).
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications/callback",
		bytes.NewBufferString(`{"code":"abc","user_id":"user-1"}`))
	VerificationCallback(db, Config{})(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestVerificationCallback_SourceAccountFromBody(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO idme_verifications").
		WithArgs("acct-2", "user-1", pgxmock.AnyArg()).
		WillReturnRows(verificationRow(mock, "v1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/verifications/callback",
		bytes.NewBufferString(`{"code":"abc","user_id":"user-1","source_account_id":"acct-2"}`))
	VerificationCallback(db, Config{})(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

// --- Groups ---

var groupCols = []string{
	"id", "source_account_id", "verification_id", "user_id", "group_type", "group_name",
	"verified", "verified_at", "expires_at", "affiliation", "rank", "status", "created_at", "updated_at",
}

func groupRow(id string) *pgxmock.Rows {
	now := time.Now()
	return pgxmock.NewRows(groupCols).AddRow(
		id, "primary", "v1", "user-1", "military", "Military", false,
		(*time.Time)(nil), (*time.Time)(nil), (*string)(nil), (*string)(nil), (*string)(nil), now, now,
	)
}

func TestListGroups_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, group_type").
		WithArgs("primary").
		WillReturnRows(groupRow("g1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	ListGroups(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListGroups_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, group_type").
		WithArgs("primary").
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	ListGroups(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestListGroups_ScanError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, group_type").
		WithArgs("primary").
		WillReturnRows(pgxmock.NewRows(groupCols).AddRow(
			"g1", "primary", "v1", "user-1", "military", "Military", "not-a-bool",
			(*time.Time)(nil), (*time.Time)(nil), (*string)(nil), (*string)(nil), (*string)(nil), time.Now(), time.Now(),
		))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	ListGroups(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateGroup_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO idme_groups").
		WithArgs("primary", "", "user-1", "military", "military", (*string)(nil), (*string)(nil), (*string)(nil)).
		WillReturnRows(groupRow("g1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups",
		bytes.NewBufferString(`{"group_type":"military","user_id":"user-1"}`))
	CreateGroup(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateGroup_BadJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", bytes.NewBufferString(`{bad`))
	CreateGroup(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateGroup_MissingRequiredFields(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", bytes.NewBufferString(`{}`))
	CreateGroup(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateGroup_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO idme_groups").
		WithArgs("primary", "", "user-1", "military", "military", (*string)(nil), (*string)(nil), (*string)(nil)).
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups",
		bytes.NewBufferString(`{"group_type":"military","user_id":"user-1"}`))
	CreateGroup(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateGroup_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("UPDATE idme_groups SET").
		WithArgs("g1", "primary", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(groupRow("g1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/groups/g1", bytes.NewBufferString(`{"group_name":"Military Updated"}`))
	req = withURLParam(req, "id", "g1")
	UpdateGroup(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateGroup_BadJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/groups/g1", bytes.NewBufferString(`{bad`))
	req = withURLParam(req, "id", "g1")
	UpdateGroup(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateGroup_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("UPDATE idme_groups SET").
		WithArgs("missing", "primary", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/groups/missing", bytes.NewBufferString(`{}`))
	req = withURLParam(req, "id", "missing")
	UpdateGroup(db)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateGroup_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("UPDATE idme_groups SET").
		WithArgs("g1", "primary", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/groups/g1", bytes.NewBufferString(`{}`))
	req = withURLParam(req, "id", "g1")
	UpdateGroup(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeleteGroup_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM idme_groups").
		WithArgs("g1", "primary").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/g1", nil)
	req = withURLParam(req, "id", "g1")
	DeleteGroup(db)(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteGroup_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM idme_groups").
		WithArgs("missing", "primary").
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/missing", nil)
	req = withURLParam(req, "id", "missing")
	DeleteGroup(db)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteGroup_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM idme_groups").
		WithArgs("g1", "primary").
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/groups/g1", nil)
	req = withURLParam(req, "id", "g1")
	DeleteGroup(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- User sub-resources ---

func TestGetUserVerifications_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs("user-1", "primary").
		WillReturnRows(verificationRow(mock, "v1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/user-1/verifications", nil)
	req = withURLParam(req, "user_id", "user-1")
	GetUserVerifications(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetUserVerifications_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, idme_uuid").
		WithArgs("user-1", "primary").
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/user-1/verifications", nil)
	req = withURLParam(req, "user_id", "user-1")
	GetUserVerifications(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

var badgeCols = []string{
	"id", "source_account_id", "verification_id", "user_id", "badge_type", "badge_name",
	"badge_icon", "badge_color", "verified_at", "expires_at", "active", "display_order",
	"created_at", "updated_at",
}

func badgeRow(id string) *pgxmock.Rows {
	now := time.Now()
	return pgxmock.NewRows(badgeCols).AddRow(
		id, "primary", "v1", "user-1", "military", "Military",
		(*string)(nil), (*string)(nil), (*time.Time)(nil), (*time.Time)(nil), true, 0, now, now,
	)
}

func TestGetUserBadges_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, badge_type").
		WithArgs("user-1", "primary").
		WillReturnRows(badgeRow("b1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/user-1/badges", nil)
	req = withURLParam(req, "user_id", "user-1")
	GetUserBadges(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetUserBadges_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, badge_type").
		WithArgs("user-1", "primary").
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/user-1/badges", nil)
	req = withURLParam(req, "user_id", "user-1")
	GetUserBadges(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetUserBadges_ScanError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, badge_type").
		WithArgs("user-1", "primary").
		WillReturnRows(pgxmock.NewRows(badgeCols).AddRow(
			"b1", "primary", "v1", "user-1", "military", "Military",
			(*string)(nil), (*string)(nil), (*time.Time)(nil), (*time.Time)(nil), "not-a-bool", 0, time.Now(), time.Now(),
		))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/user-1/badges", nil)
	req = withURLParam(req, "user_id", "user-1")
	GetUserBadges(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

// --- Badges ---

func TestListBadges_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, badge_type").
		WithArgs("primary").
		WillReturnRows(badgeRow("b1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/badges", nil)
	ListBadges(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListBadges_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT id, source_account_id, verification_id, user_id, badge_type").
		WithArgs("primary").
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/badges", nil)
	ListBadges(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateBadge_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO idme_badges").
		WithArgs("primary", "", "user-1", "military", "military", (*string)(nil), (*string)(nil), 0).
		WillReturnRows(badgeRow("b1"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/badges",
		bytes.NewBufferString(`{"badge_type":"military","user_id":"user-1"}`))
	CreateBadge(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateBadge_BadJSON(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/badges", bytes.NewBufferString(`{bad`))
	CreateBadge(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateBadge_MissingRequiredFields(t *testing.T) {
	db, _ := newMockDB(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/badges", bytes.NewBufferString(`{}`))
	CreateBadge(db)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateBadge_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO idme_badges").
		WithArgs("primary", "", "user-1", "military", "military", (*string)(nil), (*string)(nil), 0).
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/badges",
		bytes.NewBufferString(`{"badge_type":"military","user_id":"user-1"}`))
	CreateBadge(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- Stats ---

func TestGetStats_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM idme_verifications WHERE source_account_id = \\$1$").
		WithArgs("primary").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM idme_verifications WHERE source_account_id = \\$1 AND verified = true").
		WithArgs("primary").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM idme_groups").
		WithArgs("primary").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM idme_badges").
		WithArgs("primary").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(4))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM idme_webhook_events").
		WithArgs("primary").
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	GetStats(db)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetStats_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM idme_verifications WHERE source_account_id = \\$1$").
		WithArgs("primary").
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	GetStats(db)(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
