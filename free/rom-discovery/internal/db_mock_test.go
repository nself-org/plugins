// Purpose: Cover the DB-backed handler paths using pgxmock, so the real
// query/scan/error-handling logic is exercised without a live Postgres.
// Inputs: pgxmock-driven Handlers wired to expected SQL + rows.
// Outputs: asserted HTTP status/body for success, not-found, and DB-error paths.
// Constraints: SQL matching uses pgxmock's default regex matcher (substrings
// of the real query), so refactors that reorder columns need matching updates.
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func newMockHandlers(t *testing.T) (*Handlers, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return &Handlers{DB: &DB{Pool: mock}}, mock
}

// reqWithParam builds a request carrying a chi URL param, for handlers that
// call chi.URLParam(r, key). Pass body=nil for GET/DELETE-style requests.
func reqWithParam(method, target, key, val string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	rc := chi.NewRouteContext()
	rc.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

func strPtr(s string) *string { return &s }

// anyArgs returns n pgxmock.AnyArg() matchers, for queries whose exact
// parameter values aren't under test (only the SQL shape + return rows are).
func anyArgs(n int) []interface{} {
	args := make([]interface{}, n)
	for i := range args {
		args[i] = pgxmock.AnyArg()
	}
	return args
}

// =============================================================================
// Health
// =============================================================================

func TestHealthCheck(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.HealthCheck(w, r)
	if w.Code != 200 {
		t.Errorf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestSa_DefaultsToPrimary(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest("GET", "/x", nil)
	if got := h.sa(r); got != "primary" {
		t.Errorf("sa() = %q; want primary", got)
	}
}

func TestSa_UsesHeader(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-Source-Account-ID", "acct1")
	if got := h.sa(r); got != "acct1" {
		t.Errorf("sa() = %q; want acct1", got)
	}
}

func TestNewHandlers(t *testing.T) {
	db := &DB{}
	h := NewHandlers(db)
	if h.DB != db {
		t.Error("DB not wired")
	}
}

// =============================================================================
// ROM Metadata
// =============================================================================

func metadataCols() []string {
	return []string{
		"id", "source_account_id", "rom_title", "rom_title_normalized", "platform",
		"region", "file_name", "file_size_bytes", "file_hash_md5", "file_hash_sha256",
		"file_hash_crc32", "download_url", "download_source", "download_url_verified_at",
		"download_url_dead", "release_year", "release_month", "release_day", "version",
		"quality_score", "popularity_score", "release_group", "is_verified_dump",
		"is_hack", "is_translation", "is_homebrew", "is_public_domain", "game_title",
		"genre", "publisher", "developer", "description", "igdb_id", "mobygames_id",
		"box_art_url", "screenshot_urls", "is_community_rom", "community_source_url",
		"community_update_year", "scraped_from", "scraped_at", "created_at", "updated_at",
	}
}

func metadataRow(now time.Time) []any {
	return []any{
		"m1", "primary", "Some Game", "some game", "SNES",
		(*string)(nil), "some_game.sfc", (*int64)(nil), (*string)(nil), (*string)(nil),
		(*string)(nil), (*string)(nil), (*string)(nil), (*time.Time)(nil),
		false, (*int)(nil), (*int)(nil), (*int)(nil), (*string)(nil),
		0, 0, (*string)(nil), false,
		false, false, false, false, (*string)(nil),
		(*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*int)(nil), (*int)(nil),
		(*string)(nil), []string{}, false, (*string)(nil),
		(*int)(nil), (*string)(nil), (*time.Time)(nil), now, now,
	}
}

func TestListMetadata_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(metadataCols()).AddRow(metadataRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, rom_title").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/metadata", nil)
	w := httptest.NewRecorder()
	h.ListMetadata(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"Some Game"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestListMetadata_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, rom_title").WithArgs(anyArgs(1)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/metadata", nil)
	w := httptest.NewRecorder()
	h.ListMetadata(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestGetMetadata_Found(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(metadataCols()).AddRow(metadataRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, rom_title").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	r := reqWithParam("GET", "/metadata/m1", "id", "m1", nil)
	w := httptest.NewRecorder()
	h.GetMetadata(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestGetMetadata_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, rom_title").WithArgs(anyArgs(2)...).WillReturnError(errors.New("no rows"))

	r := reqWithParam("GET", "/metadata/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.GetMetadata(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateMetadata_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow("m1", now, now)
	mock.ExpectQuery("INSERT INTO np_romdisc_metadata").WithArgs(anyArgs(37)...).WillReturnRows(rows)

	body := `{"rom_title":"Some Game","platform":"SNES","file_name":"some_game.sfc"}`
	r := httptest.NewRequest("POST", "/metadata", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateMetadata(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateMetadata_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/metadata", strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	h.CreateMetadata(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateMetadata_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_romdisc_metadata").WithArgs(anyArgs(37)...).WillReturnError(errors.New("unique violation"))

	r := httptest.NewRequest("POST", "/metadata", strings.NewReader(`{"rom_title":"X"}`))
	w := httptest.NewRecorder()
	h.CreateMetadata(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestDeleteMetadata_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_romdisc_metadata").WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := reqWithParam("DELETE", "/metadata/m1", "id", "m1", nil)
	w := httptest.NewRecorder()
	h.DeleteMetadata(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteMetadata_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_romdisc_metadata").WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	r := reqWithParam("DELETE", "/metadata/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.DeleteMetadata(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestDeleteMetadata_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_romdisc_metadata").WithArgs(anyArgs(2)...).
		WillReturnError(errors.New("db down"))

	r := reqWithParam("DELETE", "/metadata/m1", "id", "m1", nil)
	w := httptest.NewRecorder()
	h.DeleteMetadata(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

// =============================================================================
// Download Queue
// =============================================================================

func downloadQueueCols() []string {
	return []string{
		"id", "source_account_id", "user_id", "rom_metadata_id", "job_id", "status",
		"download_started_at", "download_completed_at", "download_progress_percent",
		"downloaded_bytes", "total_bytes", "object_storage_path", "checksum_verified",
		"error_message", "retry_count", "max_retries", "created_at", "updated_at",
	}
}

func TestListDownloadQueue_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(downloadQueueCols()).AddRow(
		"d1", "primary", (*string)(nil), "m1", (*string)(nil), "pending",
		(*time.Time)(nil), (*time.Time)(nil), 0,
		int64(0), int64(0), (*string)(nil), false,
		(*string)(nil), 0, 3, now, now,
	)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, rom_metadata_id").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/downloads", nil)
	w := httptest.NewRecorder()
	h.ListDownloadQueue(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestListDownloadQueue_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, rom_metadata_id").WithArgs(anyArgs(1)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/downloads", nil)
	w := httptest.NewRecorder()
	h.ListDownloadQueue(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestCreateDownloadQueue_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow("d1", now, now)
	mock.ExpectQuery("INSERT INTO np_romdisc_download_queue").WithArgs(anyArgs(5)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/downloads", strings.NewReader(`{"rom_metadata_id":"m1"}`))
	w := httptest.NewRecorder()
	h.CreateDownloadQueue(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateDownloadQueue_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/downloads", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateDownloadQueue(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateDownloadQueue_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_romdisc_download_queue").WithArgs(anyArgs(5)...).WillReturnError(errors.New("fk violation"))

	r := httptest.NewRequest("POST", "/downloads", strings.NewReader(`{"rom_metadata_id":"m1"}`))
	w := httptest.NewRecorder()
	h.CreateDownloadQueue(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestUpdateDownloadQueue_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_romdisc_download_queue").WithArgs(anyArgs(8)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	r := reqWithParam("PATCH", "/downloads/d1", "id", "d1", strings.NewReader(`{"status":"completed"}`))
	w := httptest.NewRecorder()
	h.UpdateDownloadQueue(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateDownloadQueue_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := reqWithParam("PATCH", "/downloads/d1", "id", "d1", strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	h.UpdateDownloadQueue(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestUpdateDownloadQueue_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_romdisc_download_queue").WithArgs(anyArgs(8)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	r := reqWithParam("PATCH", "/downloads/missing", "id", "missing", strings.NewReader(`{"status":"failed"}`))
	w := httptest.NewRecorder()
	h.UpdateDownloadQueue(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestUpdateDownloadQueue_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_romdisc_download_queue").WithArgs(anyArgs(8)...).
		WillReturnError(errors.New("db down"))

	r := reqWithParam("PATCH", "/downloads/d1", "id", "d1", strings.NewReader(`{"status":"failed"}`))
	w := httptest.NewRecorder()
	h.UpdateDownloadQueue(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

// =============================================================================
// Scraper Jobs
// =============================================================================

func scraperCols() []string {
	return []string{
		"id", "scraper_name", "scraper_type", "scraper_source_url", "enabled",
		"last_run_at", "last_run_status", "last_run_duration_seconds",
		"roms_found", "roms_added", "roms_updated", "roms_removed", "errors",
		"cron_schedule", "next_run_at", "created_at", "updated_at",
	}
}

func TestListScraperJobs_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(scraperCols()).AddRow(
		"s1", "no-intro", "rss", "https://example.test/feed", true,
		(*time.Time)(nil), (*string)(nil), (*int)(nil),
		0, 0, 0, 0, []string{},
		"0 3 * * *", (*time.Time)(nil), now, now,
	)
	mock.ExpectQuery("SELECT id, scraper_name, scraper_type").WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/scrapers", nil)
	w := httptest.NewRecorder()
	h.ListScraperJobs(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestListScraperJobs_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, scraper_name, scraper_type").WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/scrapers", nil)
	w := httptest.NewRecorder()
	h.ListScraperJobs(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestCreateScraperJob_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow("s1", now, now)
	mock.ExpectQuery("INSERT INTO np_romdisc_scraper_jobs").WithArgs(anyArgs(5)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/scrapers", strings.NewReader(`{"scraper_name":"no-intro","scraper_type":"rss","scraper_source_url":"https://x"}`))
	w := httptest.NewRecorder()
	h.CreateScraperJob(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateScraperJob_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/scrapers", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateScraperJob(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateScraperJob_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_romdisc_scraper_jobs").WithArgs(anyArgs(5)...).WillReturnError(errors.New("unique violation"))

	r := httptest.NewRequest("POST", "/scrapers", strings.NewReader(`{"scraper_name":"dup"}`))
	w := httptest.NewRecorder()
	h.CreateScraperJob(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestUpdateScraperJob_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_romdisc_scraper_jobs").WithArgs(anyArgs(3)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	r := reqWithParam("PATCH", "/scrapers/s1", "id", "s1", strings.NewReader(`{"enabled":false}`))
	w := httptest.NewRecorder()
	h.UpdateScraperJob(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateScraperJob_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := reqWithParam("PATCH", "/scrapers/s1", "id", "s1", strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	h.UpdateScraperJob(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestUpdateScraperJob_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_romdisc_scraper_jobs").WithArgs(anyArgs(3)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	r := reqWithParam("PATCH", "/scrapers/missing", "id", "missing", strings.NewReader(`{"enabled":true}`))
	w := httptest.NewRecorder()
	h.UpdateScraperJob(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestUpdateScraperJob_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("UPDATE np_romdisc_scraper_jobs").WithArgs(anyArgs(3)...).
		WillReturnError(errors.New("db down"))

	r := reqWithParam("PATCH", "/scrapers/s1", "id", "s1", strings.NewReader(`{"enabled":true}`))
	w := httptest.NewRecorder()
	h.UpdateScraperJob(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

// =============================================================================
// Popularity
// =============================================================================

func popularityCols() []string {
	return []string{
		"id", "rom_metadata_id", "download_count", "search_count", "play_count",
		"archive_org_downloads", "computed_popularity_score", "last_score_update_at",
		"created_at", "updated_at",
	}
}

func TestListPopularity_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(popularityCols()).AddRow(
		"p1", "m1", 10, 5, 2, 0, 42, (*time.Time)(nil), now, now,
	)
	mock.ExpectQuery("SELECT p.id, p.rom_metadata_id").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/popularity", nil)
	w := httptest.NewRecorder()
	h.ListPopularity(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestListPopularity_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT p.id, p.rom_metadata_id").WithArgs(anyArgs(1)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/popularity", nil)
	w := httptest.NewRecorder()
	h.ListPopularity(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestUpsertPopularity_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow("p1", now, now)
	mock.ExpectQuery("INSERT INTO np_romdisc_popularity").WithArgs(anyArgs(6)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/popularity", strings.NewReader(`{"rom_metadata_id":"m1","download_count":10}`))
	w := httptest.NewRecorder()
	h.UpsertPopularity(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestUpsertPopularity_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/popularity", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.UpsertPopularity(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestUpsertPopularity_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_romdisc_popularity").WithArgs(anyArgs(6)...).WillReturnError(errors.New("fk violation"))

	r := httptest.NewRequest("POST", "/popularity", strings.NewReader(`{"rom_metadata_id":"missing"}`))
	w := httptest.NewRecorder()
	h.UpsertPopularity(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

// =============================================================================
// Audit Log
// =============================================================================

func auditCols() []string {
	return []string{
		"id", "source_account_id", "user_id", "action", "rom_metadata_id", "rom_name",
		"rom_platform", "rom_source", "ip_address", "user_agent", "details", "created_at",
	}
}

func TestListAuditLog_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(auditCols()).AddRow(
		int64(1), "primary", "u1", "download", strPtr("m1"), strPtr("Some Game"),
		strPtr("SNES"), strPtr("archive.org"), (*string)(nil), (*string)(nil), []byte("{}"), now,
	)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, action").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/audit", nil)
	w := httptest.NewRecorder()
	h.ListAuditLog(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestListAuditLog_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, action").WithArgs(anyArgs(1)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/audit", nil)
	w := httptest.NewRecorder()
	h.ListAuditLog(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestCreateAuditLog_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), now)
	mock.ExpectQuery("INSERT INTO np_romdisc_audit_log").WithArgs(anyArgs(10)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/audit", strings.NewReader(`{"user_id":"u1","action":"download"}`))
	w := httptest.NewRecorder()
	h.CreateAuditLog(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateAuditLog_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/audit", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateAuditLog(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateAuditLog_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_romdisc_audit_log").WithArgs(anyArgs(10)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("POST", "/audit", strings.NewReader(`{"user_id":"u1","action":"download"}`))
	w := httptest.NewRecorder()
	h.CreateAuditLog(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

// =============================================================================
// Legal Acceptance
// =============================================================================

func legalCols() []string {
	return []string{
		"id", "source_account_id", "user_id", "disclaimer_version", "accepted_at",
		"ip_address", "user_agent",
	}
}

func TestListLegalAcceptance_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(legalCols()).AddRow(
		int64(1), "primary", "u1", "1.0", now, (*string)(nil), (*string)(nil),
	)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, disclaimer_version").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/legal", nil)
	w := httptest.NewRecorder()
	h.ListLegalAcceptance(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestListLegalAcceptance_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, user_id, disclaimer_version").WithArgs(anyArgs(1)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/legal", nil)
	w := httptest.NewRecorder()
	h.ListLegalAcceptance(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestCreateLegalAcceptance_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows([]string{"id", "accepted_at"}).AddRow(int64(1), now)
	mock.ExpectQuery("INSERT INTO np_romdisc_legal_acceptance").WithArgs(anyArgs(5)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/legal", strings.NewReader(`{"user_id":"u1"}`))
	w := httptest.NewRecorder()
	h.CreateLegalAcceptance(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateLegalAcceptance_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/legal", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateLegalAcceptance(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateLegalAcceptance_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_romdisc_legal_acceptance").WithArgs(anyArgs(5)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("POST", "/legal", strings.NewReader(`{"user_id":"u1"}`))
	w := httptest.NewRecorder()
	h.CreateLegalAcceptance(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

// writeJSON coverage via a direct call (already exercised indirectly by every
// handler above, but keep an explicit smoke test for the helper itself).
func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusTeapot, map[string]string{"a": "b"})
	if w.Code != http.StatusTeapot {
		t.Errorf("status = %d", w.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["a"] != "b" {
		t.Errorf("body = %v", got)
	}
}
