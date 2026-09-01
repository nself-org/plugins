// Purpose: DB-backed handler tests for the tmdb plugin using pgxmock —
// exercises the real query/scan/branch logic in handlers.go (success,
// not-found, and DB-error paths) without a live Postgres instance.
// Inputs: httptest requests routed through chi so URL params resolve.
// Outputs: asserts HTTP status + JSON body shape per handler.
// Constraints: pgxmock.PgxPoolIface satisfies the internal.Querier seam in db.go.
package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

var movieCols = []string{
	"id", "source_account_id", "imdb_id", "title", "original_title", "overview", "tagline",
	"release_date", "runtime", "status", "poster_path", "backdrop_path", "budget", "revenue",
	"vote_average", "vote_count", "popularity", "original_language",
	"genres", "production_companies", "production_countries", "spoken_languages",
	"credits", "keywords", "content_rating", "synced_at",
}

func sampleMovieRow(rows *pgxmock.Rows, id int) *pgxmock.Rows {
	now := time.Now()
	return rows.AddRow(id, "primary", (*string)(nil), "Title", (*string)(nil), (*string)(nil), (*string)(nil),
		(*string)(nil), (*int)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*int64)(nil), (*int64)(nil),
		(*float64)(nil), (*int)(nil), (*float64)(nil), (*string)(nil),
		[]byte("[]"), []byte("[]"), []byte("[]"), []byte("[]"),
		[]byte("{}"), []byte("[]"), (*string)(nil), now)
}

var tvCols = []string{
	"id", "source_account_id", "imdb_id", "name", "original_name", "overview",
	"first_air_date", "last_air_date", "status", "type", "number_of_seasons",
	"number_of_episodes", "episode_run_time", "poster_path", "backdrop_path",
	"vote_average", "vote_count", "popularity", "original_language",
	"genres", "networks", "created_by", "credits", "content_rating", "synced_at",
}

func sampleTVRow(rows *pgxmock.Rows, id int) *pgxmock.Rows {
	now := time.Now()
	return rows.AddRow(id, "primary", (*string)(nil), "Show", (*string)(nil), (*string)(nil),
		(*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*int)(nil),
		(*int)(nil), []byte("{}"), (*string)(nil), (*string)(nil),
		(*float64)(nil), (*int)(nil), (*float64)(nil), (*string)(nil),
		[]byte("[]"), []byte("[]"), []byte("[]"), []byte("{}"), (*string)(nil), now)
}

var seasonCols = []string{
	"id", "source_account_id", "show_id", "season_number", "name", "overview",
	"poster_path", "air_date", "episode_count", "synced_at",
}

func sampleSeasonRow(rows *pgxmock.Rows, id string) *pgxmock.Rows {
	now := time.Now()
	return rows.AddRow(id, "primary", 1, 1, (*string)(nil), (*string)(nil),
		(*string)(nil), (*string)(nil), (*int)(nil), now)
}

var episodeCols = []string{
	"id", "source_account_id", "show_id", "season_number", "episode_number", "name", "overview",
	"still_path", "air_date", "runtime", "vote_average", "crew", "guest_stars", "synced_at",
}

func sampleEpisodeRow(rows *pgxmock.Rows, id string) *pgxmock.Rows {
	now := time.Now()
	return rows.AddRow(id, "primary", 1, 1, 1, (*string)(nil), (*string)(nil),
		(*string)(nil), (*string)(nil), (*int)(nil), (*float64)(nil), []byte("[]"), []byte("[]"), now)
}

var genreCols = []string{"id", "source_account_id", "name", "media_type"}

func sampleGenreRow(rows *pgxmock.Rows, id int) *pgxmock.Rows {
	return rows.AddRow(id, "primary", "Action", "movie")
}

var matchQueueCols = []string{
	"id", "source_account_id", "media_id", "filename", "parsed_title", "parsed_year",
	"parsed_type", "match_results", "best_match_id", "best_match_type", "confidence",
	"status", "reviewed_by", "reviewed_at", "auto_accepted", "created_at", "updated_at",
}

func sampleMatchQueueRow(rows *pgxmock.Rows, id string) *pgxmock.Rows {
	now := time.Now()
	return rows.AddRow(id, "primary", "media-1", (*string)(nil), (*string)(nil), (*int)(nil),
		(*string)(nil), []byte("[]"), (*int)(nil), (*string)(nil), (*float64)(nil),
		"pending", (*string)(nil), (*time.Time)(nil), false, now, now)
}

// ─── Health / Ready ───────────────────────────────────────────────────────────

func TestReady_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectPing().WillReturnError(nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestReady_DBDown(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectPing().WillReturnError(assertErr("down"))

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", w.Code)
	}
}

func TestHealth_OK(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}

// ─── Movies ───────────────────────────────────────────────────────────────────

func TestListMovies_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	countRows := pgxmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_movies").WithArgs(anyArgs(1)...).WillReturnRows(countRows)
	rows := sampleMovieRow(pgxmock.NewRows(movieCols), 1)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_movies WHERE").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/movies", nil)
	w := httptest.NewRecorder()
	h.ListMovies(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListMovies_CountError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_movies").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/movies", nil)
	w := httptest.NewRecorder()
	h.ListMovies(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListMovies_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	countRows := pgxmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_movies").WillReturnRows(countRows)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_movies WHERE").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/movies?limit=10&offset=5", nil)
	w := httptest.NewRecorder()
	h.ListMovies(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListMovies_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	countRows := pgxmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_movies").WillReturnRows(countRows)
	badRows := pgxmock.NewRows([]string{"id"}).AddRow(1)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_movies WHERE").WillReturnRows(badRows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/movies", nil)
	w := httptest.NewRecorder()
	h.ListMovies(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestCreateMovie_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("INSERT INTO np_tmdb_movies").WithArgs(anyArgs(25)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/movies", strings.NewReader(`{"id":1,"title":"Test"}`))
	w := httptest.NewRecorder()
	h.CreateMovie(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateMovie_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/movies", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateMovie(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestCreateMovie_ExecError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("INSERT INTO np_tmdb_movies").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/movies", strings.NewReader(`{"id":1,"title":"Test"}`))
	w := httptest.NewRecorder()
	h.CreateMovie(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestGetMovie_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleMovieRow(pgxmock.NewRows(movieCols), 1)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_movies WHERE id").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/movies/1", nil)
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.GetMovie(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetMovie_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_movies WHERE id").WillReturnError(assertErr("no rows"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/movies/999", nil)
	req = withURLParams(req, map[string]string{"id": "999"})
	w := httptest.NewRecorder()
	h.GetMovie(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestUpdateMovie_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_movies SET").WithArgs(anyArgs(25)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/movies/1", strings.NewReader(`{"title":"New"}`))
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.UpdateMovie(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateMovie_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/movies/1", strings.NewReader(`{bad`))
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.UpdateMovie(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestUpdateMovie_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_movies SET").WithArgs(anyArgs(25)...).WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/movies/999", strings.NewReader(`{}`))
	req = withURLParams(req, map[string]string{"id": "999"})
	w := httptest.NewRecorder()
	h.UpdateMovie(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestUpdateMovie_ExecError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_movies SET").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/movies/1", strings.NewReader(`{}`))
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.UpdateMovie(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestDeleteMovie_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("DELETE FROM np_tmdb_movies").WithArgs(anyArgs(2)...).WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/movies/1", nil)
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.DeleteMovie(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteMovie_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("DELETE FROM np_tmdb_movies").WithArgs(anyArgs(2)...).WillReturnResult(pgxmock.NewResult("DELETE", 0))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/movies/999", nil)
	req = withURLParams(req, map[string]string{"id": "999"})
	w := httptest.NewRecorder()
	h.DeleteMovie(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestDeleteMovie_ExecError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("DELETE FROM np_tmdb_movies").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/movies/1", nil)
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.DeleteMovie(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── TV Shows ─────────────────────────────────────────────────────────────────

func TestListTVShows_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	countRows := pgxmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_tv_shows").WithArgs(anyArgs(1)...).WillReturnRows(countRows)
	rows := sampleTVRow(pgxmock.NewRows(tvCols), 1)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_tv_shows WHERE").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows", nil)
	w := httptest.NewRecorder()
	h.ListTVShows(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListTVShows_CountError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_tv_shows").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows", nil)
	w := httptest.NewRecorder()
	h.ListTVShows(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListTVShows_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	countRows := pgxmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_tv_shows").WillReturnRows(countRows)
	badRows := pgxmock.NewRows([]string{"id"}).AddRow(1)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_tv_shows WHERE").WillReturnRows(badRows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows", nil)
	w := httptest.NewRecorder()
	h.ListTVShows(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestCreateTVShow_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("INSERT INTO np_tmdb_tv_shows").WithArgs(anyArgs(24)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tv-shows", strings.NewReader(`{"id":1,"name":"Test"}`))
	w := httptest.NewRecorder()
	h.CreateTVShow(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateTVShow_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tv-shows", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateTVShow(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestCreateTVShow_ExecError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("INSERT INTO np_tmdb_tv_shows").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tv-shows", strings.NewReader(`{"id":1,"name":"Test"}`))
	w := httptest.NewRecorder()
	h.CreateTVShow(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestGetTVShow_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleTVRow(pgxmock.NewRows(tvCols), 1)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_tv_shows WHERE id").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows/1", nil)
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.GetTVShow(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetTVShow_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_tv_shows WHERE id").WillReturnError(assertErr("no rows"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows/999", nil)
	req = withURLParams(req, map[string]string{"id": "999"})
	w := httptest.NewRecorder()
	h.GetTVShow(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestUpdateTVShow_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_tv_shows SET").WithArgs(anyArgs(24)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tv-shows/1", strings.NewReader(`{"name":"New"}`))
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.UpdateTVShow(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateTVShow_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tv-shows/1", strings.NewReader(`{bad`))
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.UpdateTVShow(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestUpdateTVShow_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_tv_shows SET").WithArgs(anyArgs(24)...).WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tv-shows/999", strings.NewReader(`{}`))
	req = withURLParams(req, map[string]string{"id": "999"})
	w := httptest.NewRecorder()
	h.UpdateTVShow(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestUpdateTVShow_ExecError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_tv_shows SET").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tv-shows/1", strings.NewReader(`{}`))
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.UpdateTVShow(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestDeleteTVShow_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("DELETE FROM np_tmdb_tv_shows").WithArgs(anyArgs(2)...).WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tv-shows/1", nil)
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.DeleteTVShow(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteTVShow_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("DELETE FROM np_tmdb_tv_shows").WithArgs(anyArgs(2)...).WillReturnResult(pgxmock.NewResult("DELETE", 0))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tv-shows/999", nil)
	req = withURLParams(req, map[string]string{"id": "999"})
	w := httptest.NewRecorder()
	h.DeleteTVShow(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestDeleteTVShow_ExecError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("DELETE FROM np_tmdb_tv_shows").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tv-shows/1", nil)
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.DeleteTVShow(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Seasons / Episodes ───────────────────────────────────────────────────────

func TestListSeasons_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleSeasonRow(pgxmock.NewRows(seasonCols), "s1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_tv_seasons WHERE").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows/1/seasons", nil)
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.ListSeasons(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListSeasons_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_tv_seasons WHERE").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows/1/seasons", nil)
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.ListSeasons(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListSeasons_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{"id"}).AddRow("s1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_tv_seasons WHERE").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows/1/seasons", nil)
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.ListSeasons(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestCreateSeason_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("INSERT INTO np_tmdb_tv_seasons").WithArgs(anyArgs(8)...).WillReturnRows(
		pgxmock.NewRows([]string{"id"}).AddRow("s1"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tv-shows/1/seasons", strings.NewReader(`{"season_number":1}`))
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.CreateSeason(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateSeason_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tv-shows/1/seasons", strings.NewReader(`{bad`))
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.CreateSeason(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestCreateSeason_InsertError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("INSERT INTO np_tmdb_tv_seasons").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tv-shows/1/seasons", strings.NewReader(`{"season_number":1}`))
	req = withURLParams(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	h.CreateSeason(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListEpisodes_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleEpisodeRow(pgxmock.NewRows(episodeCols), "e1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_tv_episodes WHERE").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows/1/seasons/1/episodes", nil)
	req = withURLParams(req, map[string]string{"id": "1", "season": "1"})
	w := httptest.NewRecorder()
	h.ListEpisodes(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListEpisodes_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_tv_episodes WHERE").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows/1/seasons/1/episodes", nil)
	req = withURLParams(req, map[string]string{"id": "1", "season": "1"})
	w := httptest.NewRecorder()
	h.ListEpisodes(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListEpisodes_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{"id"}).AddRow("e1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_tv_episodes WHERE").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tv-shows/1/seasons/1/episodes", nil)
	req = withURLParams(req, map[string]string{"id": "1", "season": "1"})
	w := httptest.NewRecorder()
	h.ListEpisodes(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestCreateEpisode_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("INSERT INTO np_tmdb_tv_episodes").WithArgs(anyArgs(12)...).WillReturnRows(
		pgxmock.NewRows([]string{"id"}).AddRow("e1"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tv-shows/1/seasons/1/episodes",
		strings.NewReader(`{"episode_number":1}`))
	req = withURLParams(req, map[string]string{"id": "1", "season": "1"})
	w := httptest.NewRecorder()
	h.CreateEpisode(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateEpisode_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tv-shows/1/seasons/1/episodes", strings.NewReader(`{bad`))
	req = withURLParams(req, map[string]string{"id": "1", "season": "1"})
	w := httptest.NewRecorder()
	h.CreateEpisode(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestCreateEpisode_InsertError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("INSERT INTO np_tmdb_tv_episodes").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tv-shows/1/seasons/1/episodes",
		strings.NewReader(`{"episode_number":1}`))
	req = withURLParams(req, map[string]string{"id": "1", "season": "1"})
	w := httptest.NewRecorder()
	h.CreateEpisode(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Genres ───────────────────────────────────────────────────────────────────

func TestListGenres_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleGenreRow(pgxmock.NewRows(genreCols), 1)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_genres").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/genres", nil)
	w := httptest.NewRecorder()
	h.ListGenres(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListGenres_WithMediaType(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows(genreCols)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_genres").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/genres?media_type=movie", nil)
	w := httptest.NewRecorder()
	h.ListGenres(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListGenres_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_genres").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/genres", nil)
	w := httptest.NewRecorder()
	h.ListGenres(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListGenres_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := pgxmock.NewRows([]string{"id"}).AddRow(1)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_genres").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/genres", nil)
	w := httptest.NewRecorder()
	h.ListGenres(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestCreateGenre_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("INSERT INTO np_tmdb_genres").WithArgs(anyArgs(4)...).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/genres", strings.NewReader(`{"id":1,"name":"Action","media_type":"movie"}`))
	w := httptest.NewRecorder()
	h.CreateGenre(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateGenre_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/genres", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateGenre(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestCreateGenre_ExecError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("INSERT INTO np_tmdb_genres").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/genres", strings.NewReader(`{"id":1,"name":"Action"}`))
	w := httptest.NewRecorder()
	h.CreateGenre(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Match Queue ──────────────────────────────────────────────────────────────

func TestListMatchQueue_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_match_queue").WithArgs(anyArgs(1)...).WillReturnRows(
		pgxmock.NewRows([]string{"count"}).AddRow(1))
	rows := sampleMatchQueueRow(pgxmock.NewRows(matchQueueCols), "m1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_match_queue WHERE").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/match-queue", nil)
	w := httptest.NewRecorder()
	h.ListMatchQueue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListMatchQueue_WithStatus(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_match_queue").WithArgs(anyArgs(2)...).WillReturnRows(
		pgxmock.NewRows([]string{"count"}).AddRow(0))
	rows := pgxmock.NewRows(matchQueueCols)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_match_queue WHERE").WithArgs(anyArgs(4)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/match-queue?status=pending", nil)
	w := httptest.NewRecorder()
	h.ListMatchQueue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListMatchQueue_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_match_queue").WillReturnRows(
		pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_match_queue WHERE").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/match-queue", nil)
	w := httptest.NewRecorder()
	h.ListMatchQueue(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestListMatchQueue_ScanError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_match_queue").WillReturnRows(
		pgxmock.NewRows([]string{"count"}).AddRow(0))
	badRows := pgxmock.NewRows([]string{"id"}).AddRow("m1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_match_queue WHERE").WillReturnRows(badRows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/match-queue", nil)
	w := httptest.NewRecorder()
	h.ListMatchQueue(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestCreateMatchQueue_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("INSERT INTO np_tmdb_match_queue").WithArgs(anyArgs(12)...).WillReturnRows(
		pgxmock.NewRows([]string{"id"}).AddRow("m1"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/match-queue", strings.NewReader(`{"media_id":"media-1"}`))
	w := httptest.NewRecorder()
	h.CreateMatchQueue(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateMatchQueue_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/match-queue", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateMatchQueue(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestCreateMatchQueue_InsertError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("INSERT INTO np_tmdb_match_queue").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/match-queue", strings.NewReader(`{"media_id":"media-1"}`))
	w := httptest.NewRecorder()
	h.CreateMatchQueue(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestUpdateMatchQueue_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_match_queue SET").WithArgs(anyArgs(6)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/match-queue/m1", strings.NewReader(`{"status":"confirmed"}`))
	req = withURLParams(req, map[string]string{"id": "m1"})
	w := httptest.NewRecorder()
	h.UpdateMatchQueue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateMatchQueue_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/match-queue/m1", strings.NewReader(`{bad`))
	req = withURLParams(req, map[string]string{"id": "m1"})
	w := httptest.NewRecorder()
	h.UpdateMatchQueue(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestUpdateMatchQueue_NotFound(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_match_queue SET").WithArgs(anyArgs(6)...).WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/match-queue/missing", strings.NewReader(`{}`))
	req = withURLParams(req, map[string]string{"id": "missing"})
	w := httptest.NewRecorder()
	h.UpdateMatchQueue(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestUpdateMatchQueue_ExecError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_match_queue SET").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/match-queue/m1", strings.NewReader(`{}`))
	req = withURLParams(req, map[string]string{"id": "m1"})
	w := httptest.NewRecorder()
	h.UpdateMatchQueue(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

// ─── Stats ────────────────────────────────────────────────────────────────────

func TestGetStats_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	for range []string{"movies", "tv_shows", "seasons", "episodes", "genres"} {
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_").WithArgs(anyArgs(1)...).WillReturnRows(
			pgxmock.NewRows([]string{"count"}).AddRow(1))
	}
	for range []string{"pending", "accepted", "rejected", "manual"} {
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_match_queue").WithArgs(anyArgs(2)...).WillReturnRows(
			pgxmock.NewRows([]string{"count"}).AddRow(0))
	}
	mock.ExpectQuery("SELECT synced_at FROM np_tmdb_movies").WithArgs(anyArgs(1)...).WillReturnRows(
		pgxmock.NewRows([]string{"synced_at"}).AddRow(time.Now()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	w := httptest.NewRecorder()
	h.GetStats(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

// ─── Plugin action endpoints: TMDB_API_KEY not configured branch ─────────────

func TestSearch_MissingAPIKey(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"query":"Matrix"}`))
	w := httptest.NewRecorder()
	h.Search(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d want 502 body=%s", w.Code, w.Body.String())
	}
}

func TestSearch_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.Search(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestSearch_MissingQuery(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Search(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestMatch_MissingMediaID(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/match", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Match(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestMatch_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/match", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.Match(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestMatch_MissingAPIKey(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/match", strings.NewReader(`{"media_id":"m1","title":"Matrix"}`))
	w := httptest.NewRecorder()
	h.Match(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500 body=%s", w.Code, w.Body.String())
	}
}

func TestMatchBatch_MissingItems(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/match-batch", strings.NewReader(`{"items":[]}`))
	w := httptest.NewRecorder()
	h.MatchBatch(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestMatchBatch_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/match-batch", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.MatchBatch(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestMatchBatch_ItemErrorsCollected(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/match-batch",
		strings.NewReader(`{"items":[{"media_id":"m1","title":"Matrix"}]}`))
	w := httptest.NewRecorder()
	h.MatchBatch(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "error") {
		t.Errorf("expected error status in results: %s", w.Body.String())
	}
}

func TestGetQueue_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	rows := sampleMatchQueueRow(pgxmock.NewRows(matchQueueCols), "m1")
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_match_queue").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/queue", nil)
	w := httptest.NewRecorder()
	h.GetQueue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetQueue_QueryError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_match_queue").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodGet, "/queue", nil)
	w := httptest.NewRecorder()
	h.GetQueue(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestConfirm_MissingFields(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/confirm", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Confirm(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestConfirm_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/confirm", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.Confirm(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestConfirm_UpdateError(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_match_queue").WillReturnError(assertErr("boom"))

	req := httptest.NewRequest(http.MethodPost, "/confirm",
		strings.NewReader(`{"queue_id":"m1","tmdb_id":1,"media_type":"movie"}`))
	w := httptest.NewRecorder()
	h.Confirm(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
}

func TestConfirm_FetchWarning(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectExec("UPDATE np_tmdb_match_queue").WithArgs(anyArgs(5)...).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	req := httptest.NewRequest(http.MethodPost, "/confirm",
		strings.NewReader(`{"queue_id":"m1","tmdb_id":1,"media_type":"movie"}`))
	w := httptest.NewRecorder()
	h.Confirm(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "warning") {
		t.Errorf("expected warning in body since TMDB_API_KEY unset: %s", w.Body.String())
	}
}

func TestRefresh_MissingFields(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Refresh(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestRefresh_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.Refresh(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestRefresh_MissingAPIKey(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{"tmdb_id":1,"media_type":"movie"}`))
	w := httptest.NewRecorder()
	h.Refresh(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d want 502", w.Code)
	}
}

func TestSync_BadJSON(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/sync", strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.Sync(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", w.Code)
	}
}

func TestSync_MissingAPIKey(t *testing.T) {
	h, _ := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/sync", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Sync(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d want 502 body=%s", w.Code, w.Body.String())
	}
}

func TestStatus_OK(t *testing.T) {
	h, mock := newTestHandlers(t)
	for range []string{"movies", "tv_shows", "match_pending", "genres"} {
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_").WithArgs(anyArgs(1)...).WillReturnRows(
			pgxmock.NewRows([]string{"count"}).AddRow(0))
	}

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	h.Status(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
