// Purpose: Cover the DB-backed handler paths using pgxmock, so the real
// query/scan/error-handling logic is exercised without a live Postgres.
// Inputs: pgxmock-driven Handlers wired to expected SQL + rows.
// Outputs: asserted HTTP status/body for success, not-found, and DB-error paths.
// Constraints: SQL matching uses pgxmock's default regex matcher (substrings
// of the real query), so refactors that reorder columns need matching updates.
package internal

import (
	"context"
	"errors"
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
	return NewHandlers(mock), mock
}

// reqWithParam builds a request carrying a chi URL param, for handlers that
// call chi.URLParam(r, key). Pass body=nil for GET/DELETE-style requests.
func reqWithParam(method, target, key, val string, body *strings.Reader) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, target, body)
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	rc := chi.NewRouteContext()
	rc.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

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
// Ready
// =============================================================================

func TestReady_DBUp(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectPing()
	r := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, r)
	if w.Code != 200 {
		t.Errorf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestReady_DBDown(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectPing().WillReturnError(errors.New("connection refused"))
	r := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, r)
	if w.Code != 503 {
		t.Errorf("status = %d; want 503", w.Code)
	}
}

// =============================================================================
// Platforms
// =============================================================================

func platformCols() []string {
	return []string{"id", "source_account_id", "name", "abbreviation", "slug", "igdb_id",
		"generation", "manufacturer", "platform_family", "category", "release_date",
		"summary", "is_active", "sort_order", "metadata", "created_at", "updated_at"}
}

func platformRow(now time.Time) []any {
	return []any{"p1", "primary", "SNES", (*string)(nil), "snes", (*int)(nil),
		(*int)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*time.Time)(nil),
		(*string)(nil), true, 0, []byte("{}"), now, now}
}

func TestListPlatforms_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(platformCols()).AddRow(platformRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, name, abbreviation, slug").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/platforms/", nil)
	w := httptest.NewRecorder()
	h.ListPlatforms(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"SNES"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestListPlatforms_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name, abbreviation, slug").WithArgs(anyArgs(3)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/platforms/", nil)
	w := httptest.NewRecorder()
	h.ListPlatforms(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestCreatePlatform_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(platformCols()).AddRow(platformRow(now)...)
	mock.ExpectQuery("INSERT INTO np_game_platforms").WithArgs(anyArgs(14)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/platforms/", strings.NewReader(`{"name":"SNES","slug":"snes"}`))
	w := httptest.NewRecorder()
	h.CreatePlatform(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreatePlatform_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/platforms/", strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	h.CreatePlatform(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreatePlatform_MissingFields(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/platforms/", strings.NewReader(`{"name":""}`))
	w := httptest.NewRecorder()
	h.CreatePlatform(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreatePlatform_WithMetadataAndOverrides(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(platformCols()).AddRow(platformRow(now)...)
	mock.ExpectQuery("INSERT INTO np_game_platforms").WithArgs(anyArgs(14)...).WillReturnRows(rows)

	body := `{"name":"SNES","slug":"snes","is_active":false,"sort_order":5,"metadata":{"a":1}}`
	r := httptest.NewRequest("POST", "/platforms/", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.CreatePlatform(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreatePlatform_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_game_platforms").WithArgs(anyArgs(14)...).WillReturnError(errors.New("unique violation"))

	r := httptest.NewRequest("POST", "/platforms/", strings.NewReader(`{"name":"SNES","slug":"snes"}`))
	w := httptest.NewRecorder()
	h.CreatePlatform(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestGetPlatform_Found(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(platformCols()).AddRow(platformRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, name, abbreviation, slug").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	r := reqWithParam("GET", "/platforms/p1", "id", "p1", nil)
	w := httptest.NewRecorder()
	h.GetPlatform(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestGetPlatform_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name, abbreviation, slug").WithArgs(anyArgs(2)...).WillReturnError(errors.New("no rows"))

	r := reqWithParam("GET", "/platforms/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.GetPlatform(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestDeletePlatform_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_platforms").WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := reqWithParam("DELETE", "/platforms/p1", "id", "p1", nil)
	w := httptest.NewRecorder()
	h.DeletePlatform(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDeletePlatform_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_platforms").WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	r := reqWithParam("DELETE", "/platforms/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.DeletePlatform(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestDeletePlatform_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_platforms").WithArgs(anyArgs(2)...).
		WillReturnError(errors.New("db down"))

	r := reqWithParam("DELETE", "/platforms/p1", "id", "p1", nil)
	w := httptest.NewRecorder()
	h.DeletePlatform(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

// =============================================================================
// Genres
// =============================================================================

func genreCols() []string {
	return []string{"id", "source_account_id", "name", "slug", "igdb_id", "description",
		"parent_id", "is_active", "sort_order", "metadata", "created_at", "updated_at"}
}

func genreRow(now time.Time) []any {
	return []any{"g1", "primary", "Platformer", "platformer", (*int)(nil), (*string)(nil),
		(*string)(nil), true, 0, []byte("{}"), now, now}
}

func TestListGenres_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(genreCols()).AddRow(genreRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, name, slug, igdb_id, description").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/genres/", nil)
	w := httptest.NewRecorder()
	h.ListGenres(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestListGenres_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name, slug, igdb_id, description").WithArgs(anyArgs(3)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/genres/", nil)
	w := httptest.NewRecorder()
	h.ListGenres(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestCreateGenre_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(genreCols()).AddRow(genreRow(now)...)
	mock.ExpectQuery("INSERT INTO np_game_genres").WithArgs(anyArgs(9)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/genres/", strings.NewReader(`{"name":"Platformer","slug":"platformer"}`))
	w := httptest.NewRecorder()
	h.CreateGenre(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateGenre_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/genres/", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateGenre(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateGenre_MissingFields(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/genres/", strings.NewReader(`{"name":""}`))
	w := httptest.NewRecorder()
	h.CreateGenre(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateGenre_WithMetadataAndOverrides(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(genreCols()).AddRow(genreRow(now)...)
	mock.ExpectQuery("INSERT INTO np_game_genres").WithArgs(anyArgs(9)...).WillReturnRows(rows)

	body := `{"name":"Platformer","slug":"platformer","is_active":false,"sort_order":2,"metadata":{"a":1}}`
	r := httptest.NewRequest("POST", "/genres/", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateGenre(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateGenre_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_game_genres").WithArgs(anyArgs(9)...).WillReturnError(errors.New("unique violation"))

	r := httptest.NewRequest("POST", "/genres/", strings.NewReader(`{"name":"Platformer","slug":"platformer"}`))
	w := httptest.NewRecorder()
	h.CreateGenre(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestGetGenre_Found(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(genreCols()).AddRow(genreRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, name, slug, igdb_id, description").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	r := reqWithParam("GET", "/genres/g1", "id", "g1", nil)
	w := httptest.NewRecorder()
	h.GetGenre(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestGetGenre_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name, slug, igdb_id, description").WithArgs(anyArgs(2)...).WillReturnError(errors.New("no rows"))

	r := reqWithParam("GET", "/genres/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.GetGenre(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestDeleteGenre_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_genres").WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := reqWithParam("DELETE", "/genres/g1", "id", "g1", nil)
	w := httptest.NewRecorder()
	h.DeleteGenre(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteGenre_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_genres").WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	r := reqWithParam("DELETE", "/genres/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.DeleteGenre(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestDeleteGenre_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_genres").WithArgs(anyArgs(2)...).
		WillReturnError(errors.New("db down"))

	r := reqWithParam("DELETE", "/genres/g1", "id", "g1", nil)
	w := httptest.NewRecorder()
	h.DeleteGenre(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

// =============================================================================
// Games
// =============================================================================

func gameCols() []string {
	return []string{"id", "source_account_id", "title", "slug", "platform_id", "genre_id",
		"release_date", "developer", "publisher", "description", "igdb_id",
		"rom_hash_md5", "rom_hash_sha1", "rom_hash_sha256", "rom_hash_crc32",
		"rom_filename", "rom_size_bytes", "tier", "rating", "players_min", "players_max",
		"is_verified", "metadata", "created_at", "updated_at"}
}

func gameRow(now time.Time) []any {
	return []any{"gm1", "primary", "Super Game", "super-game", (*string)(nil), (*string)(nil),
		(*time.Time)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*int)(nil),
		(*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil),
		(*string)(nil), (*int64)(nil), (*string)(nil), (*float64)(nil), 1, 1,
		false, []byte("{}"), now, now}
}

func TestListGames_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameCols()).AddRow(gameRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/games/", nil)
	w := httptest.NewRecorder()
	h.ListGames(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"Super Game"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestListGames_WithFilters(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameCols()).AddRow(gameRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(7)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/games/?platform_id=p1&genre_id=g1&tier=A&is_verified=true", nil)
	w := httptest.NewRecorder()
	h.ListGames(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestListGames_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(3)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/games/", nil)
	w := httptest.NewRecorder()
	h.ListGames(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestCreateGame_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameCols()).AddRow(gameRow(now)...)
	mock.ExpectQuery("INSERT INTO np_game_catalog").WithArgs(anyArgs(22)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/games/", strings.NewReader(`{"title":"Super Game","slug":"super-game"}`))
	w := httptest.NewRecorder()
	h.CreateGame(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateGame_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/games/", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateGame(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateGame_MissingFields(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("POST", "/games/", strings.NewReader(`{"title":""}`))
	w := httptest.NewRecorder()
	h.CreateGame(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateGame_WithOverridesAndMetadata(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameCols()).AddRow(gameRow(now)...)
	mock.ExpectQuery("INSERT INTO np_game_catalog").WithArgs(anyArgs(22)...).WillReturnRows(rows)

	body := `{"title":"Super Game","slug":"super-game","players_min":2,"players_max":4,"is_verified":true,"metadata":{"a":1}}`
	r := httptest.NewRequest("POST", "/games/", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateGame(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateGame_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_game_catalog").WithArgs(anyArgs(22)...).WillReturnError(errors.New("unique violation"))

	r := httptest.NewRequest("POST", "/games/", strings.NewReader(`{"title":"Super Game","slug":"super-game"}`))
	w := httptest.NewRecorder()
	h.CreateGame(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestGetGame_Found(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameCols()).AddRow(gameRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	r := reqWithParam("GET", "/games/gm1", "id", "gm1", nil)
	w := httptest.NewRecorder()
	h.GetGame(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestGetGame_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(2)...).WillReturnError(errors.New("no rows"))

	r := reqWithParam("GET", "/games/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.GetGame(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestDeleteGame_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_catalog").WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := reqWithParam("DELETE", "/games/gm1", "id", "gm1", nil)
	w := httptest.NewRecorder()
	h.DeleteGame(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteGame_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_catalog").WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	r := reqWithParam("DELETE", "/games/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.DeleteGame(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestDeleteGame_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_catalog").WithArgs(anyArgs(2)...).
		WillReturnError(errors.New("db down"))

	r := reqWithParam("DELETE", "/games/gm1", "id", "gm1", nil)
	w := httptest.NewRecorder()
	h.DeleteGame(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestSearchGames_MissingQuery(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("GET", "/games/search", nil)
	w := httptest.NewRecorder()
	h.SearchGames(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestSearchGames_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameCols()).AddRow(gameRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/games/search?q=super", nil)
	w := httptest.NewRecorder()
	h.SearchGames(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestSearchGames_WithFilters(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameCols()).AddRow(gameRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(5)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/games/search?q=super&platform_id=p1&tier=A&limit=10", nil)
	w := httptest.NewRecorder()
	h.SearchGames(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestSearchGames_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(3)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/games/search?q=super", nil)
	w := httptest.NewRecorder()
	h.SearchGames(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestLookupByHash_MissingParams(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("GET", "/games/lookup", nil)
	w := httptest.NewRecorder()
	h.LookupByHash(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestLookupByHash_InvalidType(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := httptest.NewRequest("GET", "/games/lookup?hash=abc&type=bogus", nil)
	w := httptest.NewRecorder()
	h.LookupByHash(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestLookupByHash_Found(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameCols()).AddRow(gameRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/games/lookup?hash=abc123&type=md5", nil)
	w := httptest.NewRecorder()
	h.LookupByHash(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestLookupByHash_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(2)...).WillReturnError(errors.New("no rows"))

	r := httptest.NewRequest("GET", "/games/lookup?hash=missing&type=sha256", nil)
	w := httptest.NewRecorder()
	h.LookupByHash(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestLookupByHash_AllTypes(t *testing.T) {
	for _, ty := range []string{"md5", "sha1", "sha256", "crc32"} {
		h, mock := newMockHandlers(t)
		now := time.Now()
		rows := mock.NewRows(gameCols()).AddRow(gameRow(now)...)
		mock.ExpectQuery("SELECT id, source_account_id, title, slug, platform_id, genre_id").WithArgs(anyArgs(2)...).WillReturnRows(rows)

		r := httptest.NewRequest("GET", "/games/lookup?hash=abc&type="+ty, nil)
		w := httptest.NewRecorder()
		h.LookupByHash(w, r)
		if w.Code != 200 {
			t.Errorf("type=%s: status = %d; body=%s", ty, w.Code, w.Body.String())
		}
	}
}

// =============================================================================
// Game Metadata
// =============================================================================

func gameMetadataCols() []string {
	return []string{"id", "source_account_id", "game_id", "source", "igdb_id", "igdb_url",
		"summary", "storyline", "total_rating", "total_rating_count",
		"aggregated_rating", "aggregated_rating_count", "first_release_date",
		"genres", "themes", "keywords", "game_modes", "franchises", "alternative_names",
		"websites", "age_ratings", "involved_companies", "raw_data", "fetched_at", "created_at", "updated_at"}
}

func gameMetadataRow(now time.Time) []any {
	return []any{"gmd1", "primary", "gm1", "igdb", (*int)(nil), (*string)(nil),
		(*string)(nil), (*string)(nil), (*float64)(nil), (*int)(nil),
		(*float64)(nil), (*int)(nil), (*time.Time)(nil),
		[]string{}, []string{}, []string{}, []string{}, []string{}, []string{},
		[]byte("{}"), []byte("{}"), []byte("[]"), []byte("{}"), now, now, now}
}

func TestUpsertGameMetadata_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameMetadataCols()).AddRow(gameMetadataRow(now)...)
	mock.ExpectQuery("INSERT INTO np_game_metadata").WithArgs(anyArgs(22)...).WillReturnRows(rows)

	r := reqWithParam("PUT", "/games/gm1/metadata/", "game_id", "gm1", strings.NewReader(`{"source":"igdb","summary":"desc"}`))
	w := httptest.NewRecorder()
	h.UpsertGameMetadata(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestUpsertGameMetadata_DefaultsSource(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameMetadataCols()).AddRow(gameMetadataRow(now)...)
	mock.ExpectQuery("INSERT INTO np_game_metadata").WithArgs(anyArgs(22)...).WillReturnRows(rows)

	r := reqWithParam("PUT", "/games/gm1/metadata/", "game_id", "gm1", strings.NewReader(`{"genres":["RPG"],"themes":["Fantasy"]}`))
	w := httptest.NewRecorder()
	h.UpsertGameMetadata(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestUpsertGameMetadata_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := reqWithParam("PUT", "/games/gm1/metadata/", "game_id", "gm1", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.UpsertGameMetadata(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestUpsertGameMetadata_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_game_metadata").WithArgs(anyArgs(22)...).WillReturnError(errors.New("fk violation"))

	r := reqWithParam("PUT", "/games/gm1/metadata/", "game_id", "gm1", strings.NewReader(`{"source":"igdb"}`))
	w := httptest.NewRecorder()
	h.UpsertGameMetadata(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestGetGameMetadata_Found(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameMetadataCols()).AddRow(gameMetadataRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, game_id, source, igdb_id, igdb_url").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	r := reqWithParam("GET", "/games/gm1/metadata/", "game_id", "gm1", nil)
	w := httptest.NewRecorder()
	h.GetGameMetadata(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestGetGameMetadata_CustomSource(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(gameMetadataCols()).AddRow(gameMetadataRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, game_id, source, igdb_id, igdb_url").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	r := reqWithParam("GET", "/games/gm1/metadata/?source=manual", "game_id", "gm1", nil)
	w := httptest.NewRecorder()
	h.GetGameMetadata(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestGetGameMetadata_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, game_id, source, igdb_id, igdb_url").WithArgs(anyArgs(3)...).WillReturnError(errors.New("no rows"))

	r := reqWithParam("GET", "/games/gm1/metadata/", "game_id", "gm1", nil)
	w := httptest.NewRecorder()
	h.GetGameMetadata(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

// =============================================================================
// Artwork
// =============================================================================

func artworkCols() []string {
	return []string{"id", "source_account_id", "game_id", "artwork_type", "url", "local_path",
		"width", "height", "mime_type", "file_size_bytes", "source", "igdb_image_id",
		"is_primary", "sort_order", "metadata", "created_at", "updated_at"}
}

func artworkRow(now time.Time) []any {
	return []any{"a1", "primary", "gm1", "boxart", (*string)(nil), (*string)(nil),
		(*int)(nil), (*int)(nil), (*string)(nil), (*int64)(nil), "igdb", (*string)(nil),
		false, 0, []byte("{}"), now, now}
}

func TestListArtwork_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(artworkCols()).AddRow(artworkRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, game_id, artwork_type, url, local_path").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	r := reqWithParam("GET", "/games/gm1/artwork/", "game_id", "gm1", nil)
	w := httptest.NewRecorder()
	h.ListArtwork(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestListArtwork_WithTypeFilter(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(artworkCols()).AddRow(artworkRow(now)...)
	mock.ExpectQuery("SELECT id, source_account_id, game_id, artwork_type, url, local_path").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	r := reqWithParam("GET", "/games/gm1/artwork/?type=boxart", "game_id", "gm1", nil)
	w := httptest.NewRecorder()
	h.ListArtwork(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestListArtwork_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, game_id, artwork_type, url, local_path").WithArgs(anyArgs(2)...).WillReturnError(errors.New("db down"))

	r := reqWithParam("GET", "/games/gm1/artwork/", "game_id", "gm1", nil)
	w := httptest.NewRecorder()
	h.ListArtwork(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestCreateArtwork_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(artworkCols()).AddRow(artworkRow(now)...)
	mock.ExpectQuery("INSERT INTO np_game_artwork").WithArgs(anyArgs(14)...).WillReturnRows(rows)

	r := reqWithParam("POST", "/games/gm1/artwork/", "game_id", "gm1", strings.NewReader(`{"artwork_type":"boxart"}`))
	w := httptest.NewRecorder()
	h.CreateArtwork(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateArtwork_BadJSON(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := reqWithParam("POST", "/games/gm1/artwork/", "game_id", "gm1", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateArtwork(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateArtwork_MissingType(t *testing.T) {
	h, _ := newMockHandlers(t)
	r := reqWithParam("POST", "/games/gm1/artwork/", "game_id", "gm1", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.CreateArtwork(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateArtwork_WithOverridesAndMetadata(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(artworkCols()).AddRow(artworkRow(now)...)
	mock.ExpectQuery("INSERT INTO np_game_artwork").WithArgs(anyArgs(14)...).WillReturnRows(rows)

	body := `{"artwork_type":"screenshot","source":"manual","is_primary":true,"sort_order":3,"metadata":{"a":1}}`
	r := reqWithParam("POST", "/games/gm1/artwork/", "game_id", "gm1", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateArtwork(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateArtwork_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO np_game_artwork").WithArgs(anyArgs(14)...).WillReturnError(errors.New("fk violation"))

	r := reqWithParam("POST", "/games/gm1/artwork/", "game_id", "gm1", strings.NewReader(`{"artwork_type":"boxart"}`))
	w := httptest.NewRecorder()
	h.CreateArtwork(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestDeleteArtwork_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_artwork").WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := reqWithParam("DELETE", "/artwork/a1", "id", "a1", nil)
	w := httptest.NewRecorder()
	h.DeleteArtwork(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteArtwork_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_artwork").WithArgs(anyArgs(2)...).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	r := reqWithParam("DELETE", "/artwork/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.DeleteArtwork(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestDeleteArtwork_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM np_game_artwork").WithArgs(anyArgs(2)...).
		WillReturnError(errors.New("db down"))

	r := reqWithParam("DELETE", "/artwork/a1", "id", "a1", nil)
	w := httptest.NewRecorder()
	h.DeleteArtwork(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

// =============================================================================
// Stats
// =============================================================================

func TestGetStats_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	statRows := mock.NewRows([]string{"total_games", "verified_games", "total_platforms",
		"total_genres", "total_artwork", "total_metadata", "games_with_igdb", "games_with_hashes"}).
		AddRow(10, 5, 3, 4, 20, 8, 6, 7)
	mock.ExpectQuery("SELECT").WithArgs(anyArgs(1)...).WillReturnRows(statRows)

	tierRows := mock.NewRows([]string{"tier", "cnt"}).AddRow("A", 3).AddRow("untiered", 7)
	mock.ExpectQuery("SELECT COALESCE").WithArgs(anyArgs(1)...).WillReturnRows(tierRows)

	r := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()
	h.GetStats(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"total_games":10`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestGetStats_MainQueryError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT").WithArgs(anyArgs(1)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()
	h.GetStats(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestGetStats_TierQueryError(t *testing.T) {
	h, mock := newMockHandlers(t)
	statRows := mock.NewRows([]string{"total_games", "verified_games", "total_platforms",
		"total_genres", "total_artwork", "total_metadata", "games_with_igdb", "games_with_hashes"}).
		AddRow(1, 1, 1, 1, 1, 1, 1, 1)
	mock.ExpectQuery("SELECT").WithArgs(anyArgs(1)...).WillReturnRows(statRows)
	mock.ExpectQuery("SELECT COALESCE").WithArgs(anyArgs(1)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()
	h.GetStats(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

