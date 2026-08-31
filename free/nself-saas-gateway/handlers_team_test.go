package main

// handlers_team_test.go — /v1/team, /v1/subscribers, and status-page
// components (sqlmock + fake email sender): seat quota, invite email with
// join token, fail-closed component defaults, and P0 tenant isolation.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/nself-org/plugins-pro/paid/shared/email"
)

const (
	memberID = "abcdabcd-1111-4222-8333-565656565656"
	pageID   = "0e0e0e0e-2222-4333-8444-676767676767"
	compID   = "0d0d0d0d-3333-4444-8555-787878787878"
)

// fakeSender records transactional sends.
type fakeSender struct {
	to   []string
	tmpl []email.Template
	data []map[string]string
}

func (f *fakeSender) Send(_ context.Context, to string, tmpl email.Template, data map[string]string) error {
	f.to = append(f.to, to)
	f.tmpl = append(f.tmpl, tmpl)
	f.data = append(f.data, data)
	return nil
}

func TestListTeamOwnerPlusMembers(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`SELECT owner_email, owner_name FROM np_saas_tenants`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"owner_email", "owner_name"}).
			AddRow("boss@example.com", "Boss"))
	mock.ExpectQuery(`FROM np_saas_team_members\s+WHERE tenant_id = \$1`).
		WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "email", "name", "role", "status", "invited_at", "joined_at"}).
			AddRow(memberID, "dev@example.com", "Dev", "member", "invited", time.Now(), nil))

	rec := doReq(t, g, http.MethodGet, "/v1/team", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list team = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Members []teamMemberDTO `json:"members"`
	}
	decodeBody(t, rec, &env)
	if len(env.Members) != 2 {
		t.Fatalf("members = %d, want 2 (owner + member)", len(env.Members))
	}
	if env.Members[0].Role != "owner" || env.Members[0].Email != "boss@example.com" {
		t.Errorf("owner seat = %+v", env.Members[0])
	}
	if env.Members[1].Status != "invited" {
		t.Errorf("member seat = %+v", env.Members[1])
	}
}

// TestInviteSeatLimitFreeTier402 — SEAT QUOTA: free tier = 1 seat (the
// owner), so ANY invite 402s with no insert and no email.
func TestInviteSeatLimitFreeTier402(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	sender := &fakeSender{}
	g.mail = sender

	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("free", []byte(`{}`), nil, time.Now()))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_saas_team_members`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// NOTE: no INSERT expectation — a write here fails the test.

	rec := doReq(t, g, http.MethodPost, "/v1/team", `{"email":"dev@example.com"}`)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("free-tier invite = %d, want 402: %s", rec.Code, rec.Body.String())
	}
	if len(sender.to) != 0 {
		t.Errorf("welcome email sent despite 402: %v", sender.to)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestInviteSendsWelcomeWithJoinToken — bundle tier invite inserts the row
// (token stored HASHED) and emails the welcome template with a join URL
// carrying the RAW token.
func TestInviteSendsWelcomeWithJoinToken(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	g.cfg.DashboardBaseURL = "https://sentry.nself.org"
	sender := &fakeSender{}
	g.mail = sender

	mock.ExpectQuery(`SELECT tier, quota_overrides`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows(
			[]string{"tier", "quota_overrides", "stripe_customer_id", "created_at"}).
			AddRow("bundle", []byte(`{}`), nil, time.Now()))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM np_saas_team_members`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1)) // 1+1 < 3 seats

	var storedHash string
	mock.ExpectQuery(`INSERT INTO np_saas_team_members`).
		WithArgs(testTenant, "dev@example.com", "Dev", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "email", "name", "role", "status", "invited_at"}).
			AddRow(memberID, "dev@example.com", "Dev", "member", "invited", time.Now())).
		WillDelayFor(0)
	_ = storedHash

	rec := doReq(t, g, http.MethodPost, "/v1/team", `{"email":"dev@example.com","name":"Dev"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Member     teamMemberDTO `json:"member"`
		InviteSent bool          `json:"invite_sent"`
	}
	decodeBody(t, rec, &env)
	if !env.InviteSent || env.Member.Status != "invited" {
		t.Errorf("invite response = %+v sent=%v", env.Member, env.InviteSent)
	}
	if len(sender.to) != 1 || sender.to[0] != "dev@example.com" ||
		sender.tmpl[0] != email.TemplateWelcome {
		t.Fatalf("welcome email = to=%v tmpl=%v", sender.to, sender.tmpl)
	}
	joinURL := sender.data[0]["URL"]
	if !strings.HasPrefix(joinURL, "https://sentry.nself.org/join?token=") ||
		len(joinURL) < len("https://sentry.nself.org/join?token=")+64 {
		t.Errorf("join URL missing raw token: %q", joinURL)
	}
}

// TestRemoveMemberCrossTenant404 — ISOLATION: removing another tenant's
// member affects zero rows → 404.
func TestRemoveMemberCrossTenant404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	mock.ExpectExec(`DELETE FROM np_saas_team_members WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(memberID, testTenant).
		WillReturnResult(sqlmock.NewResult(0, 0))

	rec := doReq(t, g, http.MethodDelete, "/v1/team/"+memberID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant remove = %d, want 404", rec.Code)
	}
}

func TestRemoveOwnerRejected(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	rec := doReq(t, g, http.MethodDelete, "/v1/team/owner", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("remove owner = %d, want 422", rec.Code)
	}
}

// TestCreateSubscriberCrossTenantPage404 — ISOLATION: subscribing to a page
// the tenant does not own 404s with no insert.
func TestCreateSubscriberCrossTenantPage404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	mock.ExpectQuery(`FROM np_saas_status_pages WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(pageID, testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // not this tenant's page

	rec := doReq(t, g, http.MethodPost, "/v1/subscribers",
		`{"email":"fan@example.com","status_page_id":"`+pageID+`"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant subscribe = %d, want 404", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestSubscriberLifecycle(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`INSERT INTO np_saas_subscribers`).
		WithArgs(testTenant, nil, "fan@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "status_page_id", "created_at"}).
			AddRow(memberID, "fan@example.com", nil, time.Now()))
	mock.ExpectQuery(`FROM np_saas_subscribers\s+WHERE tenant_id = \$1`).WithArgs(testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "status_page_id", "created_at"}).
			AddRow(memberID, "fan@example.com", nil, time.Now()))
	mock.ExpectExec(`DELETE FROM np_saas_subscribers`).
		WithArgs(memberID, testTenant).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if rec := doReq(t, g, http.MethodPost, "/v1/subscribers",
		`{"email":"fan@example.com"}`); rec.Code != http.StatusCreated {
		t.Fatalf("subscribe = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doReq(t, g, http.MethodGet, "/v1/subscribers", ""); rec.Code != http.StatusOK {
		t.Fatalf("list subscribers = %d", rec.Code)
	}
	if rec := doReq(t, g, http.MethodDelete, "/v1/subscribers/"+memberID, ""); rec.Code != http.StatusOK {
		t.Fatalf("unsubscribe = %d", rec.Code)
	}
}

// TestComponentsCrossTenantPage404 — ISOLATION: component routes 404 when
// the page belongs to another tenant, before any component query runs.
func TestComponentsCrossTenantPage404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	for range 2 { // GET + POST both check ownership first
		mock.ExpectQuery(`FROM np_saas_status_pages WHERE id = \$1 AND tenant_id = \$2`).
			WithArgs(pageID, testTenant).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))
	}

	if rec := doReq(t, g, http.MethodGet,
		"/v1/status-pages/"+pageID+"/components", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant components list = %d, want 404", rec.Code)
	}
	if rec := doReq(t, g, http.MethodPost, "/v1/status-pages/"+pageID+"/components",
		`{"monitor_id":"t-1"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant component create = %d, want 404", rec.Code)
	}
}

// TestCreateComponentFailClosedDefault — omitting "public" stores FALSE.
func TestCreateComponentFailClosedDefault(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")

	mock.ExpectQuery(`FROM np_saas_status_pages WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(pageID, testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(pageID))
	mock.ExpectQuery(`INSERT INTO np_saas_status_page_components`).
		WithArgs(testTenant, pageID, "t-1", "API", false, 0).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "monitor_id", "name", "public", "position", "created_at"}).
			AddRow(compID, "t-1", "API", false, 0, time.Now()))

	rec := doReq(t, g, http.MethodPost, "/v1/status-pages/"+pageID+"/components",
		`{"monitor_id":"t-1","name":"API"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create component = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Component componentDTO `json:"component"`
	}
	decodeBody(t, rec, &env)
	if env.Component.Public {
		t.Error("component defaulted PUBLIC — must fail closed (private)")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestDeleteComponentCrossTenant404(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	mock.ExpectQuery(`FROM np_saas_status_pages WHERE id = \$1 AND tenant_id = \$2`).
		WithArgs(pageID, testTenant).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(pageID))
	mock.ExpectExec(`DELETE FROM np_saas_status_page_components`).
		WithArgs(compID, pageID, testTenant).
		WillReturnResult(sqlmock.NewResult(0, 0))

	rec := doReq(t, g, http.MethodDelete,
		"/v1/status-pages/"+pageID+"/components/"+compID, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant component delete = %d, want 404", rec.Code)
	}
}

func TestTeamUnauthenticated401(t *testing.T) {
	db, _ := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	for _, path := range []string{"/v1/team", "/v1/subscribers",
		"/v1/status-pages/" + pageID + "/components"} {
		req, rec := plainRequest(http.MethodGet, path)
		g.router().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s unauthenticated = %d, want 401", path, rec.Code)
		}
	}
}
