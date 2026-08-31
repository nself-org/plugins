package main

// subscriber_notify_test.go — subscriber incident notifications: confirmed-
// only recipients, per-(subscriber,incident,state) dedup, SMTP-failure claim
// release + retry, resolved-once, and tenant isolation. Fake SMTP via the
// shared fakeSender (handlers_team_test.go) + a failing wrapper.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/nself-org/plugins-pro/paid/shared/email"
)

const (
	subIDA     = "aaaa1111-2222-4333-8444-555555555501"
	subIDB     = "bbbb1111-2222-4333-8444-555555555502"
	noticeIncA = "inc-42"
)

// failNSender fails the first n sends (SMTP 4xx), then records like
// fakeSender. Lets a test exercise claim-release + retry.
type failNSender struct {
	fakeSender
	failures int
}

func (f *failNSender) Send(ctx context.Context, to string, tmpl email.Template, data map[string]string) error {
	if f.failures > 0 {
		f.failures--
		return errors.New("smtp: 421 service unavailable")
	}
	return f.fakeSender.Send(ctx, to, tmpl, data)
}

var subCols = []string{"id", "email"}

// expectSubscriberSelect wires the confirmed-subscriber SELECT. The regex
// requires the `confirmed = true` filter — if the query ever dropped it
// (unconfirmed addresses would get mail), the expectation misses and the
// test fails with zero sends.
func expectSubscriberSelect(mock sqlmock.Sqlmock, tenant string, rows *sqlmock.Rows) {
	mock.ExpectQuery(`FROM np_saas_subscribers\s+WHERE tenant_id = \$1 AND confirmed = true`).
		WithArgs(tenant).WillReturnRows(rows)
}

func TestNotifySubscribersEmailsConfirmedOnly(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	sender := &fakeSender{}
	g.mail = sender

	expectSubscriberSelect(mock, testTenant,
		sqlmock.NewRows(subCols).AddRow(subIDA, "a@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(testTenant, subIDA, noticeIncA, "firing").
		WillReturnResult(sqlmock.NewResult(0, 1))

	g.notifySubscribers(context.Background(), testTenant, noticeIncA, "firing",
		"Monitor down: API", "https://api.example.com — 2 consecutive failed checks",
		"https://sentry.nself.org/monitors/mon-1")

	if len(sender.to) != 1 || sender.to[0] != "a@example.com" {
		t.Fatalf("recipients = %v, want [a@example.com]", sender.to)
	}
	if sender.tmpl[0] != email.TemplateIncidentOpened {
		t.Errorf("template = %q, want incident-opened", sender.tmpl[0])
	}
	if sender.data[0]["Title"] != "Monitor down: API" {
		t.Errorf("Title = %q", sender.data[0]["Title"])
	}
	if !strings.Contains(sender.data[0]["Description"], "consecutive failed checks") {
		t.Errorf("Description = %q", sender.data[0]["Description"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestNotifySubscribersDedup — a second pass for the same (incident, state)
// loses the ledger claim (0 rows) and must NOT re-send.
func TestNotifySubscribersDedup(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	sender := &fakeSender{}
	g.mail = sender

	// First call: claim won → send.
	expectSubscriberSelect(mock, testTenant,
		sqlmock.NewRows(subCols).AddRow(subIDA, "a@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(testTenant, subIDA, noticeIncA, "firing").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Second call: ON CONFLICT DO NOTHING → 0 rows → skip.
	expectSubscriberSelect(mock, testTenant,
		sqlmock.NewRows(subCols).AddRow(subIDA, "a@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(testTenant, subIDA, noticeIncA, "firing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	g.notifySubscribers(context.Background(), testTenant, noticeIncA, "firing", "T", "D", "U")
	g.notifySubscribers(context.Background(), testTenant, noticeIncA, "firing", "T", "D", "U")

	if len(sender.to) != 1 {
		t.Fatalf("sends = %d, want exactly 1 (dedup broke)", len(sender.to))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestNotifySubscribersResolvedOnce — the resolved transition uses the
// incident-resolved template and its own dedup key (fires exactly once).
func TestNotifySubscribersResolvedOnce(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	sender := &fakeSender{}
	g.mail = sender

	expectSubscriberSelect(mock, testTenant,
		sqlmock.NewRows(subCols).AddRow(subIDA, "a@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(testTenant, subIDA, noticeIncA, "resolved").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSubscriberSelect(mock, testTenant,
		sqlmock.NewRows(subCols).AddRow(subIDA, "a@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(testTenant, subIDA, noticeIncA, "resolved").
		WillReturnResult(sqlmock.NewResult(0, 0))

	g.notifySubscribers(context.Background(), testTenant, noticeIncA, "resolved", "Monitor recovered: API", "", "U")
	g.notifySubscribers(context.Background(), testTenant, noticeIncA, "resolved", "Monitor recovered: API", "", "U")

	if len(sender.to) != 1 {
		t.Fatalf("resolved sends = %d, want exactly 1", len(sender.to))
	}
	if sender.tmpl[0] != email.TemplateIncidentResolved {
		t.Errorf("template = %q, want incident-resolved", sender.tmpl[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestNotifySubscribersSMTPFailureReleasesClaim — a failed send deletes the
// ledger row; the next pass re-claims and the retry succeeds.
func TestNotifySubscribersSMTPFailureReleasesClaim(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	sender := &failNSender{failures: 1}
	g.mail = sender

	// Pass 1: claim → SMTP fails → claim released (DELETE).
	expectSubscriberSelect(mock, testTenant,
		sqlmock.NewRows(subCols).AddRow(subIDA, "a@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(testTenant, subIDA, noticeIncA, "firing").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM np_saas_subscriber_notices`).
		WithArgs(subIDA, noticeIncA, "firing").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Pass 2: row is gone → claim wins again → send succeeds.
	expectSubscriberSelect(mock, testTenant,
		sqlmock.NewRows(subCols).AddRow(subIDA, "a@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(testTenant, subIDA, noticeIncA, "firing").
		WillReturnResult(sqlmock.NewResult(0, 1))

	g.notifySubscribers(context.Background(), testTenant, noticeIncA, "firing", "T", "D", "U")
	g.notifySubscribers(context.Background(), testTenant, noticeIncA, "firing", "T", "D", "U")

	if len(sender.to) != 1 || sender.to[0] != "a@example.com" {
		t.Fatalf("successful sends = %v, want exactly one retry success", sender.to)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestNotifySubscribersTenantIsolation — tenant A's incident emails ONLY
// subscribers selected with tenant A's id; tenant B's subscriber is reached
// only by tenant B's own notification.
func TestNotifySubscribersTenantIsolation(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	sender := &fakeSender{}
	g.mail = sender

	expectSubscriberSelect(mock, testTenant,
		sqlmock.NewRows(subCols).AddRow(subIDA, "tenant-a@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(testTenant, subIDA, "inc-a", "firing").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectSubscriberSelect(mock, secondTenant,
		sqlmock.NewRows(subCols).AddRow(subIDB, "tenant-b@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(secondTenant, subIDB, "inc-b", "firing").
		WillReturnResult(sqlmock.NewResult(0, 1))

	g.notifySubscribers(context.Background(), testTenant, "inc-a", "firing", "A down", "", "")
	g.notifySubscribers(context.Background(), secondTenant, "inc-b", "firing", "B down", "", "")

	if len(sender.to) != 2 || sender.to[0] != "tenant-a@example.com" || sender.to[1] != "tenant-b@example.com" {
		t.Fatalf("recipients = %v — tenant leak", sender.to)
	}
	if sender.data[0]["Title"] != "A down" || sender.data[1]["Title"] != "B down" {
		t.Errorf("cross-tenant payload mixup: %v / %v", sender.data[0], sender.data[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestNotifySubscribersSkipsWithoutIncidentID — no incident id means no dedup
// key; the notifier must do nothing (no queries, no sends).
func TestNotifySubscribersSkipsWithoutIncidentID(t *testing.T) {
	db, mock := newSQLMock(t)
	g := newTestGateway(db, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
	sender := &fakeSender{}
	g.mail = sender

	g.notifySubscribers(context.Background(), testTenant, "", "firing", "T", "D", "U")

	if len(sender.to) != 0 {
		t.Fatalf("sent %v without an incident id", sender.to)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDetectorNotifiesSubscribers — end-to-end through the detector: a DOWN
// transition emails incident-opened; the recovery emails incident-resolved.
func TestDetectorNotifiesSubscribers(t *testing.T) {
	db, mock := newSQLMock(t)
	f, incident, alert := newFakePlugins(t)
	g := newTestGateway(db, "http://127.0.0.1:1", incident.URL, alert.URL)
	g.cfg.DownThreshold = 2
	sender := &fakeSender{}
	g.mail = sender

	// DOWN sweep: incident + alert + subscriber notify + state upsert.
	mock.ExpectQuery(`FROM np_uptime_targets t`).WithArgs(3).
		WillReturnRows(sqlmock.NewRows(sweepCols).
			AddRow("mon-1", testTenant, "API", "https://api.example.com", "{down,down,up}", "", ""))
	expectSubscriberSelect(mock, testTenant,
		sqlmock.NewRows(subCols).AddRow(subIDA, "a@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(testTenant, subIDA, "inc-42", "firing").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO np_saas_monitor_state`).
		WithArgs("mon-1", testTenant, 2, "inc-42").
		WillReturnResult(sqlmock.NewResult(0, 1))

	g.sweepMonitors(context.Background())

	// RECOVERY sweep: resolve + alert + subscriber notify + state upsert.
	mock.ExpectQuery(`FROM np_uptime_targets t`).WithArgs(3).
		WillReturnRows(sqlmock.NewRows(sweepCols).
			AddRow("mon-1", testTenant, "API", "https://api.example.com", "{up,down,down}", "down", "inc-42"))
	expectSubscriberSelect(mock, testTenant,
		sqlmock.NewRows(subCols).AddRow(subIDA, "a@example.com"))
	mock.ExpectExec(`INSERT INTO np_saas_subscriber_notices`).
		WithArgs(testTenant, subIDA, "inc-42", "resolved").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO np_saas_monitor_state`).
		WithArgs("mon-1", testTenant).
		WillReturnResult(sqlmock.NewResult(0, 1))

	g.sweepMonitors(context.Background())

	if len(f.incidents) != 3 {
		t.Fatalf("incident calls = %d, want open + acknowledge + resolve", len(f.incidents))
	}
	if len(sender.to) != 2 || sender.to[0] != "a@example.com" || sender.to[1] != "a@example.com" {
		t.Fatalf("subscriber emails = %v, want down + recovery to a@example.com", sender.to)
	}
	if sender.tmpl[0] != email.TemplateIncidentOpened || sender.tmpl[1] != email.TemplateIncidentResolved {
		t.Errorf("templates = %v, want [incident-opened incident-resolved]", sender.tmpl)
	}
	if sender.data[0]["Title"] != "Monitor down: API" || sender.data[1]["Title"] != "Monitor recovered: API" {
		t.Errorf("titles = %q / %q", sender.data[0]["Title"], sender.data[1]["Title"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}
