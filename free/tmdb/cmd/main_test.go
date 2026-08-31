// Purpose: unit tests for cmd/main.go route wiring (newRouter, newServer,
// serverAddr).
// Inputs: a pgxmock-backed internal.Handlers, httptest requests.
// Outputs: asserts routes are wired to the expected handlers and the server
// struct carries the expected address/timeouts.
// Constraints: does not start a real listener or touch a live database.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nself-org/nself-tmdb/internal"
	"github.com/pashagolub/pgxmock/v4"
)

func newTestHandlers(t *testing.T) (*internal.Handlers, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return internal.NewHandlers(mock, internal.Config{DefaultLanguage: "en-US"}), mock
}

func TestNewRouter_Health(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /health status=%d want 200", w.Code)
	}
}

func TestNewRouter_Ready(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectPing().WillReturnError(nil)
	r := newRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /ready status=%d want 200", w.Code)
	}
}

func TestNewRouter_MoviesRouteWired(t *testing.T) {
	h, mock := newTestHandlers(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_movies").WithArgs(pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT (.|\n)*FROM np_tmdb_movies WHERE").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows([]string{
			"id", "source_account_id", "imdb_id", "title", "original_title", "overview", "tagline",
			"release_date", "runtime", "status", "poster_path", "backdrop_path", "budget", "revenue",
			"vote_average", "vote_count", "popularity", "original_language",
			"genres", "production_companies", "production_countries", "spoken_languages",
			"credits", "keywords", "content_rating", "synced_at",
		}))
	r := newRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/movies", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/movies status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestNewRouter_StatusActionRouteWired(t *testing.T) {
	h, mock := newTestHandlers(t)
	for range []string{"movies", "tv_shows", "match_pending", "genres"} {
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM np_tmdb_").WillReturnRows(
			pgxmock.NewRows([]string{"count"}).AddRow(0))
	}
	r := newRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /status status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestNewRouter_UnknownRoute404(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
}

func TestServerAddr(t *testing.T) {
	cfg := internal.Config{Port: "3122"}
	if got := serverAddr(cfg); got != ":3122" {
		t.Errorf("serverAddr = %q", got)
	}
}

func TestNewServer_Wiring(t *testing.T) {
	h, _ := newTestHandlers(t)
	r := newRouter(h)
	srv := newServer(":9999", r)

	if srv.Addr != ":9999" {
		t.Errorf("Addr = %q", srv.Addr)
	}
	if srv.Handler == nil {
		t.Error("Handler should not be nil")
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %v", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %v", srv.IdleTimeout)
	}
}
