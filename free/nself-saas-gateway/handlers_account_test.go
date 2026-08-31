package main

// handlers_account_test.go — GDPR export + account deletion (sqlmock):
// every query scoped to the VERIFIED tenant, security artifacts excluded,
// purge covers discovered tables + the tenants row.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAccountExportTenantScopedAndCurated(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	// Curated profile — password hash never selected.
	mock.ExpectQuery(`SELECT tier, owner_email, owner_name, stripe_customer_id`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "owner_email", "owner_name", "stripe_customer_id", "verified", "created_at"}).
			AddRow("bundle", "boss@example.com", "Boss", "cus_42", true, time.Now()))
	// Discovery: three tables incl. the token table (must be skipped) and
	// api keys (must be curated).
	mock.ExpectQuery(`information_schema.columns`).
		WillReturnRows(sqlmock.NewRows([]string{"t"}).
			AddRow("public.np_saas_api_keys").
			AddRow("public.np_saas_email_tokens").
			AddRow("public.np_saas_status_pages"))
	// api keys → curated json_build_object dump.
	mock.ExpectQuery(`json_build_object`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"json"}).
			AddRow(`[{"id":"k-1","name":"default","key_prefix":"nsk_abc"}]`))
	// generic table → row_to_json dump (email_tokens skipped: NO query).
	mock.ExpectQuery(`row_to_json`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"json"}).
			AddRow(`[{"id":"p-1","slug":"acme"}]`))

	rec := doReq(t, g, http.MethodGet, "/v1/account/export", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("export = %d: %s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	if !strings.Contains(raw, `"boss@example.com"`) || !strings.Contains(raw, `"acme"`) {
		t.Errorf("export missing tenant data: %s", raw)
	}
	if strings.Contains(raw, "np_saas_email_tokens") {
		t.Error("export leaked the email-token table")
	}
	if strings.Contains(raw, "password") || strings.Contains(raw, "key_hash") {
		t.Errorf("export leaked security columns: %s", raw)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestAccountDeletePurgesVerifiedTenantOnly(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`SELECT stripe_customer_id FROM np_saas_tenants`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"stripe_customer_id"}).AddRow("cus_42"))
	mock.ExpectQuery(`information_schema.columns`).
		WillReturnRows(sqlmock.NewRows([]string{"t"}).
			AddRow("public.np_uptime_targets").
			AddRow("public.np_saas_status_pages"))
	// ISOLATION: every DELETE is parameterized by the VERIFIED tenant.
	mock.ExpectExec(`DELETE FROM public.np_uptime_targets WHERE tenant_id = \$1`).
		WithArgs(testTenant).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(`DELETE FROM public.np_saas_status_pages WHERE tenant_id = \$1`).
		WithArgs(testTenant).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM np_saas_tenants WHERE tenant_id = \$1`).
		WithArgs(testTenant).WillReturnResult(sqlmock.NewResult(0, 1))

	rec := doReq(t, g, http.MethodDelete, "/v1/account", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete account = %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Deleted          bool             `json:"deleted"`
		TenantID         string           `json:"tenant_id"`
		Tables           map[string]int64 `json:"tables"`
		StripeCustomerID string           `json:"stripe_customer_id"`
		BillingNote      string           `json:"billing_note"`
	}
	decodeBody(t, rec, &resp)
	if !resp.Deleted || resp.TenantID != testTenant {
		t.Errorf("delete response = %+v", resp)
	}
	if resp.Tables["public.np_uptime_targets"] != 3 || resp.Tables["np_saas_tenants"] != 1 {
		t.Errorf("purge counts = %+v", resp.Tables)
	}
	// Stripe-cancel hook note: the billing layer needs the customer id.
	if resp.StripeCustomerID != "cus_42" || resp.BillingNote == "" {
		t.Errorf("stripe note missing: %+v", resp)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestAccountUnauthenticated401(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	for method, path := range map[string]string{
		http.MethodGet:    "/v1/account/export",
		http.MethodDelete: "/v1/account",
	} {
		req, rec := plainRequest(method, path)
		g.router().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s unauthenticated = %d, want 401", method, path, rec.Code)
		}
	}
}
