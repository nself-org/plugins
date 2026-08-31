package main

// handlers_billing_test.go — /v1/billing tier + quotas + portal stub.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBillingShape(t *testing.T) {
	db, mock := newSQLMock(t)
	// GetTenant: a bundle-tier row; the quota count/usage reads are
	// best-effort (unexpected queries error → used=0), which is exactly the
	// fresh-box behaviour the handler documents.
	mock.ExpectQuery(`SELECT tier, quota_overrides, stripe_customer_id, created_at`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("bundle", []byte(`{}`), nil, time.Now().UTC()))

	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := doReq(t, g, http.MethodGet, "/v1/billing", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("billing = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Billing struct {
			Tier      string                   `json:"tier"`
			Quotas    map[string]quotaUsageDTO `json:"quotas"`
			PortalURL string                   `json:"portal_url"`
		} `json:"billing"`
	}
	decodeBody(t, rec, &resp)
	if resp.Billing.Tier != "bundle" {
		t.Errorf("tier = %q, want bundle", resp.Billing.Tier)
	}
	if resp.Billing.PortalURL == "" {
		t.Error("portal_url missing")
	}
	// Bundle-tier limits from the PRD table must flow through.
	if q, ok := resp.Billing.Quotas["monitors"]; !ok || q.Limit != 50 {
		t.Errorf("quotas.monitors = %+v, want limit 50", resp.Billing.Quotas["monitors"])
	}
	if q, ok := resp.Billing.Quotas["error_events_month"]; !ok || q.Limit != 100_000 {
		t.Errorf("quotas.error_events_month = %+v, want limit 100000", resp.Billing.Quotas["error_events_month"])
	}
}

func TestBillingRequiresAuth(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodGet, "/v1/billing", nil)
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated billing = %d, want 401", rec.Code)
	}
}
