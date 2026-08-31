package main

// quota_enforcement_test.go — regression test for W9 Sev-3 (monthly
// ingest-quota enforcement unverified at the HTTP layer).
//
// Purpose: lock the invariant that once a tenant's monthly usage counter
//   crosses its tier cap, the gateway starts rejecting further ingest with
//   429 Too Many Requests (WriteQuotaThrottled) rather than silently
//   accepting unlimited free-tier volume. spoof_test.go proves tenant
//   identity can't be forged; this file proves the identified tenant's
//   quota is actually enforced end-to-end through the real router, not just
//   at the shared/saas.IncrMonthlyUsage unit level (already covered by
//   shared/saas/quota_test.go's freeCapDrain, which drives the primitive
//   directly with sqlmock but never goes through a gateway handler).
// Covers: POST /v1/ci/events (the only ingest route with IncrMonthlyUsage
//   wired directly in this repo — error/RUM ingest live in their own
//   plugins and read only from np_saas_usage here, see handlers_errors.go
//   and handlers_rum.go doc comments) pushed past the free-tier
//   CIEventsMonth cap (500/mo): every request up to the cap must insert a
//   row and return 201; every request after must be rejected 429 with a
//   Retry-After header and a quota_exceeded envelope, and MUST NOT insert
//   a np_saas_ci_events row (quota check runs before the write).
// Constraints: uses status=success bodies only so the failure-path alert
//   dispatch (best-effort upstream call) never engages — this test asserts
//   quota enforcement, not alert routing.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// quotaTestTenant is a syntactically-valid, non-existent tenant UUID — never
// a real customer id, per the CR-C guidance on this ticket.
const quotaTestTenant = "22222222-4444-4555-8666-777777777777"

const ciEventBody = `{"repo":"nself-org/quota-load-test","status":"success"}`

// expectEffectiveLimitsFree primes the EffectiveLimits() DB round-trip
// (GetTenant's SELECT) to resolve quotaTestTenant as an ordinary free-tier
// tenant with no overrides, matching TestGetTenantUnknownDefaultsFree's
// mock shape in shared/saas/tenant_test.go.
func expectEffectiveLimitsFree(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(quotaTestTenant).
		WillReturnRows(sqlmock.NewRows([]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}))
}

// TestQuotaCapLoadCIEvents drives POST /v1/ci/events through the real
// router past the free-tier CIEventsMonth cap (500) and asserts the
// throttle boundary: requests 1..500 succeed (201, row inserted), request
// 501 onward is rejected 429 with Retry-After and no insert attempted.
func TestQuotaCapLoadCIEvents(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	const cap = 500 // saas.LimitsForTier(saas.TierFree).CIEventsMonth
	if got := saas.LimitsForTier(saas.TierFree).CIEventsMonth; got != cap {
		t.Fatalf("free-tier CIEventsMonth = %d, want %d (test cap is out of date)", got, cap)
	}

	jwt := mintTestJWT(t, quotaTestTenant)
	throttledAt := 0
	loadN := cap + 10 // push comfortably past the cap, like a real burst

	for i := 1; i <= loadN; i++ {
		expectEffectiveLimitsFree(mock)
		mock.ExpectQuery(`INSERT INTO np_saas_usage`).
			WithArgs(quotaTestTenant, saas.MetricCIEvents, sqlmock.AnyArg(), int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(int64(i)))
		if i <= cap {
			mock.ExpectQuery(`INSERT INTO np_saas_ci_events`).
				WithArgs(quotaTestTenant, "nself-org/quota-load-test", "", "success", "", "", "").
				WillReturnRows(sqlmock.NewRows(
					[]string{"id", "repo", "workflow", "status", "run_url", "sha", "title", "created_at"},
				).AddRow(fmt.Sprintf("00000000-0000-4000-8000-%012d", i),
					"nself-org/quota-load-test", "", "success", "", "", "", time.Now().UTC()))
		}

		req := httptest.NewRequest(http.MethodPost, "/v1/ci/events", strings.NewReader(ciEventBody))
		req.Header.Set("Authorization", "Bearer "+jwt)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		g.router().ServeHTTP(rec, req)

		switch {
		case i <= cap && rec.Code != http.StatusCreated:
			t.Fatalf("request %d/%d (under cap) = %d, want 201: %s", i, cap, rec.Code, rec.Body.String())
		case i > cap && rec.Code != http.StatusTooManyRequests:
			t.Fatalf("request %d (over cap %d) = %d, want 429: %s", i, cap, rec.Code, rec.Body.String())
		case i > cap && throttledAt == 0:
			throttledAt = i
			if rec.Header().Get("Retry-After") == "" {
				t.Error("429 response missing Retry-After header")
			}
			var body struct {
				Error    string `json:"error"`
				Resource string `json:"resource"`
				Limit    int64  `json:"limit"`
				Used     int64  `json:"used"`
			}
			decodeBody(t, rec, &body)
			if body.Error != "quota_exceeded" || body.Resource != saas.MetricCIEvents || body.Limit != cap {
				t.Errorf("throttle body = %+v, want quota_exceeded ci_events/%d", body, cap)
			}
		}
	}

	if throttledAt != cap+1 {
		t.Errorf("first throttle at request %d, want %d", throttledAt, cap+1)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v (an over-cap request issued an INSERT INTO np_saas_ci_events — quota check must run before the write)", err)
	}
}

// TestQuotaCapRecoversNextMonth confirms the throttle is a monthly window,
// not a permanent lockout: MonthKey changes -> IncrMonthlyUsage upserts a
// fresh row for the new period -> a request that would have 429'd in the
// prior period succeeds again. This is the "recovery behaviour after
// backoff" the ticket's acceptance/runtime-evidence bar requires.
func TestQuotaCapRecoversNextMonth(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	jwt := mintTestJWT(t, quotaTestTenant)

	// One request already at the cap boundary (simulates end of a throttled month).
	expectEffectiveLimitsFree(mock)
	mock.ExpectQuery(`INSERT INTO np_saas_usage`).
		WithArgs(quotaTestTenant, saas.MetricCIEvents, sqlmock.AnyArg(), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(int64(501)))
	req := httptest.NewRequest(http.MethodPost, "/v1/ci/events", strings.NewReader(ciEventBody))
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("setup request = %d, want 429", rec.Code)
	}

	// Next period: the ON CONFLICT upsert keys on (tenant_id, metric, period),
	// so a new MonthKey means a fresh row starting at used=1 — recovery, not
	// a sticky lockout.
	expectEffectiveLimitsFree(mock)
	mock.ExpectQuery(`INSERT INTO np_saas_usage`).
		WithArgs(quotaTestTenant, saas.MetricCIEvents, sqlmock.AnyArg(), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(int64(1)))
	mock.ExpectQuery(`INSERT INTO np_saas_ci_events`).
		WithArgs(quotaTestTenant, "nself-org/quota-load-test", "", "success", "", "", "").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "repo", "workflow", "status", "run_url", "sha", "title", "created_at"},
		).AddRow("00000000-0000-4000-8000-000000000001",
			"nself-org/quota-load-test", "", "success", "", "", "", time.Now().UTC()))

	req2 := httptest.NewRequest(http.MethodPost, "/v1/ci/events", strings.NewReader(ciEventBody))
	req2.Header.Set("Authorization", "Bearer "+jwt)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	g.router().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("new-period request = %d, want 201 (quota must reset on a new MonthKey period): %s", rec2.Code, rec2.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}
