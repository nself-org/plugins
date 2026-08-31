package main

// handlers_join_test.go — team-invite accept (GET/POST /v1/join): valid
// accept mints a member JWT for the INVITING tenant, bad/expired/consumed
// tokens all 404 generically, seat-full accepts 402, and a spoofed tenant
// header can never steer the issued session (P0 isolation).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

const rawJoinToken = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// inviteCols are the columns lookupInvite scans.
var inviteCols = []string{"id", "tenant_id", "email", "name", "role",
	"owner_name", "owner_email", "tier"}

// expectInviteLookup queues the lookupInvite SELECT for the raw token.
func expectInviteLookup(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectQuery(`FROM np_saas_team_members m\s+JOIN np_saas_tenants t`).
		WithArgs(sha256Hex(rawJoinToken), inviteTTLDays).
		WillReturnRows(rows)
}

func validInviteRows() *sqlmock.Rows {
	return sqlmock.NewRows(inviteCols).
		AddRow(memberID, testTenant, "dev@example.com", "Dev", "member",
			"Boss", "boss@example.com", "bundle")
}

// joinReq drives an UNAUTHENTICATED request through the full router (join is
// public); a spoofed tenant header is included to prove it is inert.
func joinReq(t *testing.T, g *gateway, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("X-Hasura-Tenant-Id", secondTenant) // must be stripped/ignored
	rec := httptest.NewRecorder()
	g.router().ServeHTTP(rec, req)
	return rec
}

func TestJoinInfoValidToken(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	expectInviteLookup(mock, validInviteRows())

	rec := joinReq(t, g, http.MethodGet, "/v1/join/"+rawJoinToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("join info = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Invite struct {
			TenantName string `json:"tenant_name"`
			Email      string `json:"email"`
			Name       string `json:"name"`
			Role       string `json:"role"`
		} `json:"invite"`
	}
	decodeBody(t, rec, &env)
	if env.Invite.TenantName != "Boss" || env.Invite.Email != "dev@example.com" ||
		env.Invite.Role != "member" {
		t.Errorf("invite info wrong: %+v", env.Invite)
	}
	if strings.Contains(rec.Body.String(), testTenant) {
		t.Error("join info leaked the tenant id to an unauthenticated caller")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestJoinInfoBadToken404 — unknown, expired, and consumed tokens all take
// the no-rows path and must answer the SAME generic 404.
func TestJoinInfoBadToken404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	mock.ExpectQuery(`FROM np_saas_team_members m\s+JOIN np_saas_tenants t`).
		WithArgs(sqlmock.AnyArg(), inviteTTLDays).
		WillReturnRows(sqlmock.NewRows(inviteCols)) // no match

	rec := joinReq(t, g, http.MethodGet, "/v1/join/deadbeef", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad token = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not_found") {
		t.Errorf("missing generic not_found envelope: %s", rec.Body.String())
	}
}

// TestJoinAcceptHappyPath — valid token + password activates the member,
// consumes the token, and returns a login JWT whose tenant is the INVITING
// tenant (taken from the invite row, never from the spoofed header).
func TestJoinAcceptHappyPath(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	expectInviteLookup(mock, validInviteRows())
	// EffectiveLimits → bundle (3 seats)
	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("bundle", []byte(`{}`), nil, time.Now()))
	// active members: 1 → owner + 1 active + this accept fits in 3 seats
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_saas_team_members WHERE tenant_id = \$1 AND status = 'active'`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// atomic consume: guarded UPDATE re-checks token hash + invited status
	mock.ExpectExec(`UPDATE np_saas_team_members\s+SET status = 'active'`).
		WithArgs(sqlmock.AnyArg(), "Devi", memberID, sha256Hex(rawJoinToken)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rec := joinReq(t, g, http.MethodPost, "/v1/join",
		`{"token":"`+rawJoinToken+`","password":"s3cret-pw","name":"Devi"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("accept = %d: %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Token    string `json:"token"`
		TenantID string `json:"tenant_id"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}
	decodeBody(t, rec, &res)
	if res.TenantID != testTenant || res.Email != "dev@example.com" || res.Role != "member" {
		t.Errorf("accept response wrong: %+v", res)
	}

	// The JWT verifies with the session secret and is scoped to the inviting
	// tenant with role member — NOT the spoofed header tenant.
	claims, err := saas.VerifyHS256Claims(res.Token, []byte(testJWTSecret))
	if err != nil {
		t.Fatalf("returned JWT does not verify: %v", err)
	}
	str := func(k string) string {
		var s string
		_ = json.Unmarshal(claims[k], &s)
		return s
	}
	if str("tenant_id") != testTenant {
		t.Errorf("JWT tenant = %q, want inviting tenant %q", str("tenant_id"), testTenant)
	}
	if str("tenant_id") == secondTenant {
		t.Error("P0: spoofed X-Hasura-Tenant-Id steered the session tenant")
	}
	if str("role") != "member" || str("member_id") != memberID {
		t.Errorf("JWT member claims wrong: role=%q member_id=%q", str("role"), str("member_id"))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestJoinAcceptSeatFull402 — a tier downgrade after the invite means no
// free seat at accept time: 402, no consume, no activation.
func TestJoinAcceptSeatFull402(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	rows := sqlmock.NewRows(inviteCols).
		AddRow(memberID, testTenant, "dev@example.com", "Dev", "member",
			"Boss", "boss@example.com", "free")
	expectInviteLookup(mock, rows)
	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("free", []byte(`{}`), nil, time.Now())) // free = 1 seat (owner)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_saas_team_members WHERE tenant_id = \$1 AND status = 'active'`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// NOTE: no UPDATE expectation — consuming the token here fails the test.

	rec := joinReq(t, g, http.MethodPost, "/v1/join",
		`{"token":"`+rawJoinToken+`","password":"s3cret-pw"}`)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("seat-full accept = %d, want 402: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "quota_exceeded") {
		t.Errorf("missing quota_exceeded marker: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestJoinAcceptExpiredOrConsumed404 — the lookup finds nothing (expired
// window or already-consumed hash cleared) → generic 404, nothing written.
func TestJoinAcceptExpiredOrConsumed404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	expectInviteLookup(mock, sqlmock.NewRows(inviteCols)) // no rows

	rec := joinReq(t, g, http.MethodPost, "/v1/join",
		`{"token":"`+rawJoinToken+`","password":"s3cret-pw"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expired/consumed accept = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestJoinAcceptRacedConsume404 — the guarded UPDATE hits 0 rows (someone
// consumed the token between lookup and consume) → 404, no JWT issued.
func TestJoinAcceptRacedConsume404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	expectInviteLookup(mock, validInviteRows())
	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("bundle", []byte(`{}`), nil, time.Now()))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_saas_team_members WHERE tenant_id = \$1 AND status = 'active'`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`UPDATE np_saas_team_members\s+SET status = 'active'`).
		WithArgs(sqlmock.AnyArg(), "", memberID, sha256Hex(rawJoinToken)).
		WillReturnResult(sqlmock.NewResult(0, 0)) // raced away

	rec := joinReq(t, g, http.MethodPost, "/v1/join",
		`{"token":"`+rawJoinToken+`","password":"s3cret-pw"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("raced consume = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"token"`) {
		t.Error("JWT issued despite failed consume")
	}
}

// TestJoinAcceptWeakPassword422 — password policy enforced before any DB
// work (no lookup expectation queued).
func TestJoinAcceptWeakPassword422(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := joinReq(t, g, http.MethodPost, "/v1/join",
		`{"token":"`+rawJoinToken+`","password":"short"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("weak password = %d, want 422: %s", rec.Code, rec.Body.String())
	}
}
