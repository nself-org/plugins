// Purpose: Route-wiring tests for the entitlements plugin's cmd/main.
// Inputs: httptest requests against the router built by newRouter, with a mocked pgx pool.
// Outputs: asserts route dispatch (200/404) and JSON health/status bodies.
// Constraints: No real Postgres, no real port binding; exercises newRouter only, never main().
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pashagolub/pgxmock/v3"

	"github.com/nself-org/nself-entitlements/internal"
)

func newTestHandlers(t *testing.T) (*internal.Handlers, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return internal.NewHandlers(mock), mock
}

func TestNewRouter_Health(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["plugin"] != "entitlements" {
		t.Errorf("plugin = %q", out["plugin"])
	}
}

func TestNewRouter_Ready(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectPing()
	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_PluginStatus(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestNewRouter_UnknownRoute404(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestNewRouter_PlansRouteDispatch(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name, slug").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "name", "slug", "description", "billing_interval", "price_cents", "currency",
			"trial_days", "plan_type", "is_public", "is_archived", "features", "quotas", "metadata", "display_order",
			"created_at", "updated_at",
		}))

	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plans/", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_SubscriptionsRouteDispatch(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, workspace_id").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "workspace_id", "user_id", "plan_id", "status", "billing_interval",
			"price_cents", "currency", "trial_start", "trial_end", "current_period_start", "current_period_end",
			"cancel_at_period_end", "canceled_at", "metadata", "created_at", "updated_at",
		}))

	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_FeaturesRouteDispatch(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, key, name").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "key", "name", "description", "feature_type", "is_active", "category",
			"metadata", "created_at", "updated_at",
		}))

	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/features/", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_QuotasRouteDispatch(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, plan_id, feature_id").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "plan_id", "feature_id", "limit_value", "period", "created_at",
		}))

	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/quotas/", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_AddonsRouteDispatch(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, subscription_id, addon_plan_id").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "subscription_id", "addon_plan_id", "quantity", "price_cents", "currency",
			"status", "created_at", "updated_at",
		}))

	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/addons/", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_GrantsRouteDispatch(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, workspace_id, feature_key").
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "user_id", "workspace_id", "feature_key", "reason",
			"granted_by", "expires_at", "revoked_at", "metadata", "created_at", "updated_at",
		}))

	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/grants/", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_UsageGetRouteDispatch(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, quota_key").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "source_account_id", "user_id", "quota_key", "usage_amount", "created_at",
		}))

	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouter_CheckRouteDispatch_MissingFeatureKey(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/check", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
