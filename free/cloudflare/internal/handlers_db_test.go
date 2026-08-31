// Purpose: DB-backed handler tests for the cloudflare plugin using pgxmock —
// exercises the real query/scan/branch logic in handlers.go (success, not-found,
// and DB-error paths) without a live Postgres instance.
// Inputs: httptest requests routed through chi so URL params resolve.
// Outputs: asserts HTTP status + JSON body shape per handler.
// Constraints: pgxmock.PgxPoolIface satisfies the internal.Querier seam in db.go.
package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pashagolub/pgxmock/v4"
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

// ─── Health / Ready ───────────────────────────────────────────────────────────

func TestHandleReady_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectPing().WillReturnError(nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	HandleReady(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestHandleReady_DBDown(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectPing().WillReturnError(assertErr("db down"))

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	HandleReady(db)(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", w.Code)
	}
}

// ─── Zones ────────────────────────────────────────────────────────────────────

func TestHandleListZones_OK(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "name", "status", "created_at", "synced_at"}).
		AddRow("z1", "primary", (*string)(nil), "example.com", (*string)(nil), now, now)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_zones").WithArgs("primary").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/zones", nil)
	w := httptest.NewRecorder()
	HandleListZones(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["total"].(float64) != 1 {
		t.Errorf("total=%v want 1", body["total"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestHandleListZones_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_zones").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/zones", nil)
	w := httptest.NewRecorder()
	HandleListZones(db)(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestHandleCreateZone_OK(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "name", "status", "created_at", "synced_at"}).
		AddRow("zone_123", "primary", (*string)(nil), "example.com", (*string)(nil), now, now)
	mock.ExpectQuery("INSERT INTO cf_zones").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(rows)

	body := `{"id":"zone_123","name":"example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones", strings.NewReader(body))
	w := httptest.NewRecorder()
	HandleCreateZone(db)(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleCreateZone_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	HandleCreateZone(db)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestHandleCreateZone_MissingName(t *testing.T) {
	db, _ := newMockDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones", strings.NewReader(`{"id":"z1"}`))
	w := httptest.NewRecorder()
	HandleCreateZone(db)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestHandleCreateZone_AutoID(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "name", "status", "created_at", "synced_at"}).
		AddRow("zone_999", "primary", (*string)(nil), "auto.com", (*string)(nil), now, now)
	mock.ExpectQuery("INSERT INTO cf_zones").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones", strings.NewReader(`{"name":"auto.com"}`))
	w := httptest.NewRecorder()
	HandleCreateZone(db)(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleCreateZone_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO cf_zones").WillReturnError(assertErr("insert failed"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/zones", strings.NewReader(`{"name":"x.com"}`))
	w := httptest.NewRecorder()
	HandleCreateZone(db)(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func withChiParam(req *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return req.WithContext(withChiContext(req, rctx))
}

func TestHandleGetZone_Found(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "name", "status", "created_at", "synced_at"}).
		AddRow("z1", "primary", (*string)(nil), "example.com", (*string)(nil), now, now)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_zones").WithArgs("primary", "z1").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/zones/z1", nil)
	req = withChiParam(req, "id", "z1")
	w := httptest.NewRecorder()
	HandleGetZone(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleGetZone_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_zones").WillReturnError(assertErr("no rows"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/zones/missing", nil)
	req = withChiParam(req, "id", "missing")
	w := httptest.NewRecorder()
	HandleGetZone(db)(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestHandleDeleteZone_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM cf_zones").WithArgs("primary", "z1").WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/zones/z1", nil)
	req = withChiParam(req, "id", "z1")
	w := httptest.NewRecorder()
	HandleDeleteZone(db)(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", w.Code)
	}
}

func TestHandleDeleteZone_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM cf_zones").WillReturnError(assertErr("delete failed"))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/zones/z1", nil)
	req = withChiParam(req, "id", "z1")
	w := httptest.NewRecorder()
	HandleDeleteZone(db)(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── DNS Records ──────────────────────────────────────────────────────────────

func TestHandleListDNSRecords_NoFilter(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "record_id", "type", "name", "content", "ttl", "proxied", "created_at", "synced_at"}).
		AddRow("d1", "primary", "z1", (*string)(nil), "A", "www", "1.2.3.4", 1, true, now, now)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_dns_records").WithArgs("primary").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dns-records", nil)
	w := httptest.NewRecorder()
	HandleListDNSRecords(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleListDNSRecords_WithZoneFilter(t *testing.T) {
	db, mock := newMockDB(t)
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "record_id", "type", "name", "content", "ttl", "proxied", "created_at", "synced_at"})
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_dns_records").WithArgs("primary", "z1").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dns-records?zone_id=z1", nil)
	w := httptest.NewRecorder()
	HandleListDNSRecords(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleListDNSRecords_ScanError(t *testing.T) {
	db, mock := newMockDB(t)
	// Wrong column count forces a Scan error path.
	rows := pgxmock.NewRows([]string{"id"}).AddRow("d1")
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_dns_records").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dns-records", nil)
	w := httptest.NewRecorder()
	HandleListDNSRecords(db)(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500 body=%s", w.Code, w.Body.String())
	}
}

func TestHandleCreateDNSRecord_OK(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "record_id", "type", "name", "content", "ttl", "proxied", "created_at", "synced_at"}).
		AddRow("dns_1", "primary", "z1", (*string)(nil), "A", "www", "1.1.1.1", 1, false, now, now)
	mock.ExpectQuery("INSERT INTO cf_dns_records").WithArgs(
		pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
		pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnRows(rows)

	body := `{"id":"dns_1","zone_id":"z1","type":"A","name":"www","content":"1.1.1.1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dns-records", strings.NewReader(body))
	w := httptest.NewRecorder()
	HandleCreateDNSRecord(db)(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleCreateDNSRecord_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dns-records", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	HandleCreateDNSRecord(db)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestHandleCreateDNSRecord_MissingFields(t *testing.T) {
	db, _ := newMockDB(t)
	cases := []string{
		`{"type":"A","name":"www","content":"1.1.1.1"}`,
		`{"zone_id":"z1","name":"www","content":"1.1.1.1"}`,
		`{"zone_id":"z1","type":"A","content":"1.1.1.1"}`,
		`{"zone_id":"z1","type":"A","name":"www"}`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/dns-records", strings.NewReader(body))
		w := httptest.NewRecorder()
		HandleCreateDNSRecord(db)(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d want 400", body, w.Code)
		}
	}
}

func TestHandleUpdateDNSRecord_OK(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	existing := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "record_id", "type", "name", "content", "ttl", "proxied", "created_at", "synced_at"}).
		AddRow("d1", "primary", "z1", (*string)(nil), "A", "www", "1.1.1.1", 1, true, now, now)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_dns_records").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(existing)

	updated := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "record_id", "type", "name", "content", "ttl", "proxied", "created_at", "synced_at"}).
		AddRow("d1", "primary", "z1", (*string)(nil), "A", "www", "2.2.2.2", 1, true, now, now)
	mock.ExpectQuery("UPDATE cf_dns_records").WithArgs(
		pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
		pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnRows(updated)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/dns-records/d1", strings.NewReader(`{"content":"2.2.2.2"}`))
	req = withChiParam(req, "id", "d1")
	w := httptest.NewRecorder()
	HandleUpdateDNSRecord(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateDNSRecord_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/dns-records/d1", strings.NewReader("{bad"))
	req = withChiParam(req, "id", "d1")
	w := httptest.NewRecorder()
	HandleUpdateDNSRecord(db)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestHandleUpdateDNSRecord_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_dns_records").WillReturnError(assertErr("no rows"))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/dns-records/missing", strings.NewReader(`{}`))
	req = withChiParam(req, "id", "missing")
	w := httptest.NewRecorder()
	HandleUpdateDNSRecord(db)(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestHandleDeleteDNSRecord_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM cf_dns_records").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dns-records/d1", nil)
	req = withChiParam(req, "id", "d1")
	w := httptest.NewRecorder()
	HandleDeleteDNSRecord(db)(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", w.Code)
	}
}

func TestHandleDeleteDNSRecord_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM cf_dns_records").WillReturnError(assertErr("delete failed"))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/dns-records/d1", nil)
	req = withChiParam(req, "id", "d1")
	w := httptest.NewRecorder()
	HandleDeleteDNSRecord(db)(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Cache ────────────────────────────────────────────────────────────────────

func TestHandlePurgeCache_All(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "purge_type", "urls", "tags", "prefixes", "status", "cf_response", "created_at"}).
		AddRow("p1", "primary", "z1", "all", []string(nil), []string(nil), []string(nil), "completed", (*string)(nil), now)
	mock.ExpectQuery("INSERT INTO cf_cache_purge_log").WithArgs(anyArgs(7)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cache/purge", strings.NewReader(`{"zone_id":"z1"}`))
	w := httptest.NewRecorder()
	HandlePurgeCache(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandlePurgeCache_ByFiles(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "purge_type", "urls", "tags", "prefixes", "status", "cf_response", "created_at"}).
		AddRow("p2", "primary", "z1", "urls", []string{"https://x/1"}, []string(nil), []string(nil), "completed", (*string)(nil), now)
	mock.ExpectQuery("INSERT INTO cf_cache_purge_log").WithArgs(anyArgs(7)...).WillReturnRows(rows)

	body := `{"zone_id":"z1","files":["https://x/1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cache/purge", strings.NewReader(body))
	w := httptest.NewRecorder()
	HandlePurgeCache(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandlePurgeCache_ByTags(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "purge_type", "urls", "tags", "prefixes", "status", "cf_response", "created_at"}).
		AddRow("p3", "primary", "z1", "tags", []string(nil), []string{"tag1"}, []string(nil), "completed", (*string)(nil), now)
	mock.ExpectQuery("INSERT INTO cf_cache_purge_log").WithArgs(anyArgs(7)...).WillReturnRows(rows)

	body := `{"zone_id":"z1","tags":["tag1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cache/purge", strings.NewReader(body))
	w := httptest.NewRecorder()
	HandlePurgeCache(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandlePurgeCache_ByPrefixes(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "purge_type", "urls", "tags", "prefixes", "status", "cf_response", "created_at"}).
		AddRow("p4", "primary", "z1", "prefixes", []string(nil), []string(nil), []string{"/api"}, "completed", (*string)(nil), now)
	mock.ExpectQuery("INSERT INTO cf_cache_purge_log").WithArgs(anyArgs(7)...).WillReturnRows(rows)

	body := `{"zone_id":"z1","prefixes":["/api"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cache/purge", strings.NewReader(body))
	w := httptest.NewRecorder()
	HandlePurgeCache(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandlePurgeCache_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cache/purge", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	HandlePurgeCache(db)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestHandlePurgeCache_MissingZoneID(t *testing.T) {
	db, _ := newMockDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cache/purge", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	HandlePurgeCache(db)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestHandlePurgeCache_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO cf_cache_purge_log").WillReturnError(assertErr("insert failed"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cache/purge", strings.NewReader(`{"zone_id":"z1"}`))
	w := httptest.NewRecorder()
	HandlePurgeCache(db)(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestHandleListCachePurgeLog_NoFilter(t *testing.T) {
	db, mock := newMockDB(t)
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "purge_type", "urls", "tags", "prefixes", "status", "cf_response", "created_at"})
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_cache_purge_log").WithArgs("primary").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cache/purge-log", nil)
	w := httptest.NewRecorder()
	HandleListCachePurgeLog(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleListCachePurgeLog_WithZoneFilter(t *testing.T) {
	db, mock := newMockDB(t)
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "purge_type", "urls", "tags", "prefixes", "status", "cf_response", "created_at"})
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_cache_purge_log").WithArgs("primary", "z1").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cache/purge-log?zone_id=z1", nil)
	w := httptest.NewRecorder()
	HandleListCachePurgeLog(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleListCachePurgeLog_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_cache_purge_log").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cache/purge-log", nil)
	w := httptest.NewRecorder()
	HandleListCachePurgeLog(db)(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── R2 Buckets ───────────────────────────────────────────────────────────────

func TestHandleListR2Buckets_OK(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "name", "location", "storage_class", "object_count", "total_size_bytes", "created_at", "synced_at"}).
		AddRow("b1", "primary", "mybucket", (*string)(nil), "Standard", int64(0), int64(0), now, now)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_r2_buckets").WithArgs("primary").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/r2/buckets", nil)
	w := httptest.NewRecorder()
	HandleListR2Buckets(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleListR2Buckets_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_r2_buckets").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/r2/buckets", nil)
	w := httptest.NewRecorder()
	HandleListR2Buckets(db)(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestHandleCreateR2Bucket_OK(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "name", "location", "storage_class", "object_count", "total_size_bytes", "created_at", "synced_at"}).
		AddRow("b1", "primary", "mybucket", (*string)(nil), "Standard", int64(0), int64(0), now, now)
	mock.ExpectQuery("INSERT INTO cf_r2_buckets").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/r2/buckets", strings.NewReader(`{"name":"mybucket"}`))
	w := httptest.NewRecorder()
	HandleCreateR2Bucket(db)(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleCreateR2Bucket_InvalidJSON(t *testing.T) {
	db, _ := newMockDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/r2/buckets", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	HandleCreateR2Bucket(db)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestHandleCreateR2Bucket_MissingName(t *testing.T) {
	db, _ := newMockDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/r2/buckets", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	HandleCreateR2Bucket(db)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestHandleCreateR2Bucket_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("INSERT INTO cf_r2_buckets").WillReturnError(assertErr("insert failed"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/r2/buckets", strings.NewReader(`{"name":"x"}`))
	w := httptest.NewRecorder()
	HandleCreateR2Bucket(db)(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestHandleDeleteR2Bucket_OK(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM cf_r2_buckets").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/r2/buckets/b1", nil)
	req = withChiParam(req, "id", "b1")
	w := httptest.NewRecorder()
	HandleDeleteR2Bucket(db)(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", w.Code)
	}
}

func TestHandleDeleteR2Bucket_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM cf_r2_buckets").WillReturnError(assertErr("delete failed"))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/r2/buckets/b1", nil)
	req = withChiParam(req, "id", "b1")
	w := httptest.NewRecorder()
	HandleDeleteR2Bucket(db)(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Analytics ────────────────────────────────────────────────────────────────

func TestHandleGetAnalytics_DefaultRange(t *testing.T) {
	db, mock := newMockDB(t)
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "date", "requests_total", "requests_cached", "bandwidth_total", "threats_total", "unique_visitors", "synced_at"})
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_analytics").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics", nil)
	w := httptest.NewRecorder()
	HandleGetAnalytics(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleGetAnalytics_WithZoneAndRange(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now()
	rows := pgxmock.NewRows([]string{"id", "source_account_id", "zone_id", "date", "requests_total", "requests_cached", "bandwidth_total", "threats_total", "unique_visitors", "synced_at"}).
		AddRow("a1", "primary", "z1", "2026-01-01", int64(100), int64(50), int64(2048), int64(1), int64(10), now)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_analytics").WithArgs(anyArgs(4)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics?zone_id=z1&since=2026-01-01&until=2026-01-31", nil)
	w := httptest.NewRecorder()
	HandleGetAnalytics(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleGetAnalytics_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM cf_analytics").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics", nil)
	w := httptest.NewRecorder()
	HandleGetAnalytics(db)(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Stats ────────────────────────────────────────────────────────────────────

func TestHandleStats_OK(t *testing.T) {
	db, mock := newMockDB(t)
	for _, table := range []string{"cf_zones", "cf_dns_records", "cf_r2_buckets", "cf_cache_purge_log", "cf_analytics"} {
		rows := pgxmock.NewRows([]string{"count"}).AddRow(int64(1))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM " + table).WithArgs(pgxmock.AnyArg()).WillReturnRows(rows)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	HandleStats(db)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var stats Stats
	json.Unmarshal(w.Body.Bytes(), &stats)
	if stats.TotalZones != 1 || stats.TotalAnalytics != 1 {
		t.Errorf("stats=%+v", stats)
	}
}

func TestHandleStats_FirstCountErrors(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM cf_zones").WithArgs(pgxmock.AnyArg()).WillReturnError(assertErr("count failed"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	HandleStats(db)(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}
