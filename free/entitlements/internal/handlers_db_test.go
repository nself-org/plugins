// Purpose: Behavioral tests for handlers.go and handlers_v2.go DB-backed paths,
//
//	using pgxmock to substitute a real Postgres pool.
//
// Inputs: httptest requests dispatched directly to Handlers methods, with
//
//	pgxmock expectations for the underlying SQL.
//
// Outputs: asserts HTTP status codes + JSON response shape for success and
//
//	error (query/scan/insert/update failure) paths.
//
// Constraints: No live DB. Query text is matched via regexp substrings
//
//	(pgxmock default), so SQL can be reformatted without breaking
//	these tests as long as key clauses are preserved.
package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pashagolub/pgxmock/v3"
)

func newMockHandlers(t *testing.T) (*Handlers, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return NewHandlers(mock), mock
}

func withURLParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// --- Plans ---

func TestListPlans_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name, slug").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "name", "slug", "description", "billing_interval", "price_cents", "currency",
			"trial_days", "plan_type", "is_public", "is_archived", "features", "quotas", "metadata", "display_order",
			"created_at", "updated_at",
		}).AddRow(
			"p1", "primary", "Pro", "pro", (*string)(nil), "month", 999, "USD",
			14, "paid", true, false, []byte(`{}`), []byte(`{}`), []byte(`{}`), 0,
			time.Now(), time.Now(),
		))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plans", nil)
	h.ListPlans(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestListPlans_QueryError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name, slug").
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(errors.New("boom"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plans", nil)
	h.ListPlans(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func anyArgs(n int) []interface{} {
	args := make([]interface{}, n)
	for i := range args {
		args[i] = pgxmock.AnyArg()
	}
	return args
}

func TestCreatePlan_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO entitlement_plans").
		WithArgs(anyArgs(13)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("new-id"))

	body, _ := json.Marshal(map[string]any{
		"name": "Pro", "slug": "pro", "billing_interval": "month", "plan_type": "paid",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans", bytes.NewReader(body))
	h.CreatePlan(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatePlan_MissingRequiredFields(t *testing.T) {
	h, _ := newMockHandlers(t)
	body, _ := json.Marshal(map[string]any{"name": "Pro"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans", bytes.NewReader(body))
	h.CreatePlan(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreatePlan_InvalidJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans", bytes.NewReader([]byte("not-json")))
	h.CreatePlan(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreatePlan_InsertError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO entitlement_plans").
		WithArgs(anyArgs(13)...).
		WillReturnError(errors.New("dup"))

	body, _ := json.Marshal(map[string]any{
		"name": "Pro", "slug": "pro", "billing_interval": "month", "plan_type": "paid",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans", bytes.NewReader(body))
	h.CreatePlan(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetPlan_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name, slug").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("no rows"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plans/x", nil)
	req = withURLParam(req, "id", "x")
	h.GetPlan(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetPlan_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name, slug").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "name", "slug", "description", "billing_interval", "price_cents", "currency",
			"trial_days", "plan_type", "is_public", "is_archived", "features", "quotas", "metadata", "display_order",
			"created_at", "updated_at",
		}).AddRow(
			"p1", "primary", "Pro", "pro", (*string)(nil), "month", 999, "USD",
			14, "paid", true, false, []byte(`{}`), []byte(`{}`), []byte(`{}`), 0,
			time.Now(), time.Now(),
		))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plans/p1", nil)
	req = withURLParam(req, "id", "p1")
	h.GetPlan(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdatePlan_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE entitlement_plans SET").
		WithArgs(anyArgs(7)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	body, _ := json.Marshal(map[string]any{"name": "Renamed"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/plans/p1", bytes.NewReader(body))
	req = withURLParam(req, "id", "p1")
	h.UpdatePlan(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdatePlan_InvalidJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/plans/p1", bytes.NewReader([]byte("bad")))
	req = withURLParam(req, "id", "p1")
	h.UpdatePlan(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdatePlan_ExecError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE entitlement_plans SET").
		WithArgs(anyArgs(7)...).
		WillReturnError(errors.New("fail"))

	body, _ := json.Marshal(map[string]any{"name": "Renamed"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/plans/p1", bytes.NewReader(body))
	req = withURLParam(req, "id", "p1")
	h.UpdatePlan(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeletePlan_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE entitlement_plans SET is_archived").
		WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/plans/p1", nil)
	req = withURLParam(req, "id", "p1")
	h.DeletePlan(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeletePlan_Error(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE entitlement_plans SET is_archived").
		WithArgs(anyArgs(2)...).
		WillReturnError(errors.New("fail"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/plans/p1", nil)
	req = withURLParam(req, "id", "p1")
	h.DeletePlan(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- Subscriptions ---

func TestListSubscriptions_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, workspace_id").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "workspace_id", "user_id", "plan_id", "status", "billing_interval",
			"price_cents", "currency", "trial_start", "trial_end", "current_period_start", "current_period_end",
			"cancel_at_period_end", "canceled_at", "metadata", "created_at", "updated_at",
		}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions", nil)
	h.ListSubscriptions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateSubscription_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO entitlement_subscriptions").
		WithArgs(anyArgs(7)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("sub-1"))

	body, _ := json.Marshal(map[string]any{"plan_id": "p1", "user_id": "u1"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", bytes.NewReader(body))
	h.CreateSubscription(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateSubscription_MissingPlanID(t *testing.T) {
	h, _ := newMockHandlers(t)
	body, _ := json.Marshal(map[string]any{"user_id": "u1"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", bytes.NewReader(body))
	h.CreateSubscription(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateSubscription_MissingTarget(t *testing.T) {
	h, _ := newMockHandlers(t)
	body, _ := json.Marshal(map[string]any{"plan_id": "p1"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", bytes.NewReader(body))
	h.CreateSubscription(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateSubscription_YearlyInterval(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO entitlement_subscriptions").
		WithArgs(anyArgs(7)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("sub-2"))

	body, _ := json.Marshal(map[string]any{"plan_id": "p1", "workspace_id": "w1", "billing_interval": "year"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", bytes.NewReader(body))
	h.CreateSubscription(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetSubscription_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, workspace_id").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(errors.New("no rows"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/x", nil)
	req = withURLParam(req, "id", "x")
	h.GetSubscription(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateSubscription_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE entitlement_subscriptions SET updated_at").
		WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	body, _ := json.Marshal(map[string]any{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/subscriptions/s1", bytes.NewReader(body))
	req = withURLParam(req, "id", "s1")
	h.UpdateSubscription(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCancelSubscription_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE entitlement_subscriptions SET status='canceled'").
		WithArgs(anyArgs(3)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/s1/cancel", nil)
	req = withURLParam(req, "id", "s1")
	h.CancelSubscription(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- Features ---

func TestListFeatures_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, key, name").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "key", "name", "description", "feature_type", "is_active", "category",
			"metadata", "created_at", "updated_at",
		}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/features", nil)
	h.ListFeatures(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateFeature_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO entitlement_features").
		WithArgs(anyArgs(5)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("f1"))

	body, _ := json.Marshal(map[string]any{"key": "k", "name": "n", "feature_type": "boolean"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/features", bytes.NewReader(body))
	h.CreateFeature(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateFeature_MissingFields(t *testing.T) {
	h, _ := newMockHandlers(t)
	body, _ := json.Marshal(map[string]any{"key": "k"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/features", bytes.NewReader(body))
	h.CreateFeature(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateFeature_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE entitlement_features SET").
		WithArgs(anyArgs(4)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	body, _ := json.Marshal(map[string]any{"name": "renamed"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/features/f1", bytes.NewReader(body))
	req = withURLParam(req, "id", "f1")
	h.UpdateFeature(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeleteFeature_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM entitlement_features").
		WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/features/f1", nil)
	req = withURLParam(req, "id", "f1")
	h.DeleteFeature(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- Quotas ---

func TestListQuotas_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, plan_id, feature_id").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "plan_id", "feature_id", "limit_value", "period", "created_at",
		}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/quotas", nil)
	h.ListQuotas(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateQuota_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO entitlement_quotas").
		WithArgs(anyArgs(5)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("q1"))

	body, _ := json.Marshal(map[string]any{"plan_id": "p1", "feature_id": "f1", "limit_value": float64(100)})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/quotas", bytes.NewReader(body))
	h.CreateQuota(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateQuota_MissingFields(t *testing.T) {
	h, _ := newMockHandlers(t)
	body, _ := json.Marshal(map[string]any{"plan_id": "p1"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/quotas", bytes.NewReader(body))
	h.CreateQuota(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUpdateQuota_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE entitlement_quotas SET").
		WithArgs(anyArgs(4)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	body, _ := json.Marshal(map[string]any{"period": "yearly"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/quotas/q1", bytes.NewReader(body))
	req = withURLParam(req, "id", "q1")
	h.UpdateQuota(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDeleteQuota_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM entitlement_quotas").
		WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/quotas/q1", nil)
	req = withURLParam(req, "id", "q1")
	h.DeleteQuota(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- Usage ---

func TestTrackUsage_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO entitlement_usage").
		WithArgs(anyArgs(4)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("u1"))

	body, _ := json.Marshal(map[string]any{"feature_key": "k", "user_id": "u1", "amount": float64(2)})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/usage/track", bytes.NewReader(body))
	h.TrackUsage(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTrackUsage_MissingFeatureKey(t *testing.T) {
	h, _ := newMockHandlers(t)
	body, _ := json.Marshal(map[string]any{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/usage/track", bytes.NewReader(body))
	h.TrackUsage(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestGetUsage_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, quota_key").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "user_id", "quota_key", "usage_amount", "created_at",
		}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage?user_id=u1&feature_key=k", nil)
	h.GetUsage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- CheckEntitlement ---

func TestCheckEntitlement_Allowed(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM entitlement_subscriptions").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(usage_amount\\)").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"sum"}).AddRow(5))
	limitVal := 10
	mock.ExpectQuery("SELECT q.limit_value FROM entitlement_quotas").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"limit_value"}).AddRow(&limitVal))

	body, _ := json.Marshal(map[string]any{"feature_key": "k", "user_id": "u1"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/check", bytes.NewReader(body))
	h.CheckEntitlement(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["allowed"] != true {
		t.Errorf("allowed = %v", out["allowed"])
	}
	if out["remaining"].(float64) != 5 {
		t.Errorf("remaining = %v", out["remaining"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestCheckEntitlement_MissingFeatureKey(t *testing.T) {
	h, _ := newMockHandlers(t)
	body, _ := json.Marshal(map[string]any{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/check", bytes.NewReader(body))
	h.CheckEntitlement(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- Addons ---

func TestListAddons_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, subscription_id, addon_plan_id").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "subscription_id", "addon_plan_id", "quantity", "price_cents", "currency",
			"status", "created_at", "updated_at",
		}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/addons", nil)
	h.ListAddons(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateAddon_OK(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO entitlement_addons").
		WithArgs(anyArgs(4)...).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("a1"))

	body, _ := json.Marshal(map[string]any{"subscription_id": "s1", "addon_plan_id": "ap1"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/addons", bytes.NewReader(body))
	h.CreateAddon(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateAddon_MissingFields(t *testing.T) {
	h, _ := newMockHandlers(t)
	body, _ := json.Marshal(map[string]any{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/addons", bytes.NewReader(body))
	h.CreateAddon(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

// --- helpers ---

func TestNullableHelpers(t *testing.T) {
	if nullStr("") != nil {
		t.Error("nullStr(\"\") should be nil")
	}
	if nullStr("x") != "x" {
		t.Error("nullStr(\"x\") should be x")
	}
	body := map[string]any{"a": "b", "n": float64(5), "flag": true}
	if nullableString(body, "a") != "b" {
		t.Error("nullableString a")
	}
	if nullableString(body, "missing") != nil {
		t.Error("nullableString missing should be nil")
	}
	if nullableInt(body, "n") != 5 {
		t.Error("nullableInt n")
	}
	if nullableInt(body, "missing") != nil {
		t.Error("nullableInt missing should be nil")
	}
	if nullableBool(body, "flag") != true {
		t.Error("nullableBool flag")
	}
	if nullableBool(body, "missing") != nil {
		t.Error("nullableBool missing should be nil")
	}
}
