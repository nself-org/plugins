package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nself-org/plugins-pro/paid/shared/httpclient"
)

// tmdbClient is the package-level HTTP client used for TMDB API calls.
// 30s budget per spec.
var tmdbClient = httpclient.New(httpclient.Options{Timeout: 30 * time.Second})

type Handlers struct {
	pool Querier
	cfg  Config
}

func NewHandlers(pool Querier, cfg Config) *Handlers {
	return &Handlers{pool: pool, cfg: cfg}
}

func (h *Handlers) sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" {
		return v
	}
	return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Health and Ready

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "tmdb"})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// Movies

func (h *Handlers) ListMovies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	var total int
	if err := h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tmdb_movies WHERE source_account_id=$1`, sa).Scan(&total); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, imdb_id, title, original_title, overview, tagline,
		        release_date, runtime, status, poster_path, backdrop_path, budget, revenue,
		        vote_average, vote_count, popularity, original_language,
		        genres, production_companies, production_countries, spoken_languages,
		        credits, keywords, content_rating, synced_at
		 FROM np_tmdb_movies WHERE source_account_id=$1
		 ORDER BY popularity DESC NULLS LAST LIMIT $2 OFFSET $3`, sa, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	movies := []Movie{}
	for rows.Next() {
		var m Movie
		if err := rows.Scan(&m.ID, &m.SourceAccountID, &m.ImdbID, &m.Title, &m.OriginalTitle,
			&m.Overview, &m.Tagline, &m.ReleaseDate, &m.Runtime, &m.Status,
			&m.PosterPath, &m.BackdropPath, &m.Budget, &m.Revenue,
			&m.VoteAverage, &m.VoteCount, &m.Popularity, &m.OriginalLanguage,
			&m.Genres, &m.ProductionCompanies, &m.ProductionCountries, &m.SpokenLanguages,
			&m.Credits, &m.Keywords, &m.ContentRating, &m.SyncedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		movies = append(movies, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": movies, "total": total})
}

func (h *Handlers) CreateMovie(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var m Movie
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	m.SourceAccountID = sa
	if m.Genres == nil {
		m.Genres = []byte("[]")
	}
	if m.ProductionCompanies == nil {
		m.ProductionCompanies = []byte("[]")
	}
	if m.ProductionCountries == nil {
		m.ProductionCountries = []byte("[]")
	}
	if m.SpokenLanguages == nil {
		m.SpokenLanguages = []byte("[]")
	}
	if m.Credits == nil {
		m.Credits = []byte("{}")
	}
	if m.Keywords == nil {
		m.Keywords = []byte("[]")
	}
	_, err := h.pool.Exec(ctx,
		`INSERT INTO np_tmdb_movies (id, source_account_id, imdb_id, title, original_title, overview, tagline,
		  release_date, runtime, status, poster_path, backdrop_path, budget, revenue,
		  vote_average, vote_count, popularity, original_language,
		  genres, production_companies, production_countries, spoken_languages,
		  credits, keywords, content_rating, synced_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,NOW())
		 ON CONFLICT (id) DO UPDATE SET
		  source_account_id=EXCLUDED.source_account_id, imdb_id=EXCLUDED.imdb_id,
		  title=EXCLUDED.title, original_title=EXCLUDED.original_title,
		  overview=EXCLUDED.overview, tagline=EXCLUDED.tagline,
		  release_date=EXCLUDED.release_date, runtime=EXCLUDED.runtime, status=EXCLUDED.status,
		  poster_path=EXCLUDED.poster_path, backdrop_path=EXCLUDED.backdrop_path,
		  budget=EXCLUDED.budget, revenue=EXCLUDED.revenue,
		  vote_average=EXCLUDED.vote_average, vote_count=EXCLUDED.vote_count,
		  popularity=EXCLUDED.popularity, original_language=EXCLUDED.original_language,
		  genres=EXCLUDED.genres, production_companies=EXCLUDED.production_companies,
		  production_countries=EXCLUDED.production_countries, spoken_languages=EXCLUDED.spoken_languages,
		  credits=EXCLUDED.credits, keywords=EXCLUDED.keywords,
		  content_rating=EXCLUDED.content_rating, synced_at=NOW()`,
		m.ID, sa, m.ImdbID, m.Title, m.OriginalTitle, m.Overview, m.Tagline,
		m.ReleaseDate, m.Runtime, m.Status, m.PosterPath, m.BackdropPath, m.Budget, m.Revenue,
		m.VoteAverage, m.VoteCount, m.Popularity, m.OriginalLanguage,
		m.Genres, m.ProductionCompanies, m.ProductionCountries, m.SpokenLanguages,
		m.Credits, m.Keywords, m.ContentRating)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handlers) GetMovie(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var m Movie
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, imdb_id, title, original_title, overview, tagline,
		        release_date, runtime, status, poster_path, backdrop_path, budget, revenue,
		        vote_average, vote_count, popularity, original_language,
		        genres, production_companies, production_countries, spoken_languages,
		        credits, keywords, content_rating, synced_at
		 FROM np_tmdb_movies WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&m.ID, &m.SourceAccountID, &m.ImdbID, &m.Title, &m.OriginalTitle,
			&m.Overview, &m.Tagline, &m.ReleaseDate, &m.Runtime, &m.Status,
			&m.PosterPath, &m.BackdropPath, &m.Budget, &m.Revenue,
			&m.VoteAverage, &m.VoteCount, &m.Popularity, &m.OriginalLanguage,
			&m.Genres, &m.ProductionCompanies, &m.ProductionCountries, &m.SpokenLanguages,
			&m.Credits, &m.Keywords, &m.ContentRating, &m.SyncedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handlers) UpdateMovie(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var m Movie
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	tag, err := h.pool.Exec(ctx,
		`UPDATE np_tmdb_movies SET imdb_id=$3, title=$4, original_title=$5, overview=$6, tagline=$7,
		  release_date=$8, runtime=$9, status=$10, poster_path=$11, backdrop_path=$12,
		  budget=$13, revenue=$14, vote_average=$15, vote_count=$16, popularity=$17,
		  original_language=$18, genres=$19, production_companies=$20,
		  production_countries=$21, spoken_languages=$22, credits=$23, keywords=$24,
		  content_rating=$25, synced_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, m.ImdbID, m.Title, m.OriginalTitle, m.Overview, m.Tagline,
		m.ReleaseDate, m.Runtime, m.Status, m.PosterPath, m.BackdropPath, m.Budget, m.Revenue,
		m.VoteAverage, m.VoteCount, m.Popularity, m.OriginalLanguage,
		m.Genres, m.ProductionCompanies, m.ProductionCountries, m.SpokenLanguages,
		m.Credits, m.Keywords, m.ContentRating)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "updated": true})
}

func (h *Handlers) DeleteMovie(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	tag, err := h.pool.Exec(ctx,
		`DELETE FROM np_tmdb_movies WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// TV Shows

func (h *Handlers) ListTVShows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	var total int
	if err := h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_tmdb_tv_shows WHERE source_account_id=$1`, sa).Scan(&total); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, imdb_id, name, original_name, overview,
		        first_air_date, last_air_date, status, type, number_of_seasons,
		        number_of_episodes, episode_run_time, poster_path, backdrop_path,
		        vote_average, vote_count, popularity, original_language,
		        genres, networks, created_by, credits, content_rating, synced_at
		 FROM np_tmdb_tv_shows WHERE source_account_id=$1
		 ORDER BY popularity DESC NULLS LAST LIMIT $2 OFFSET $3`, sa, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	shows := []TVShow{}
	for rows.Next() {
		var s TVShow
		if err := rows.Scan(&s.ID, &s.SourceAccountID, &s.ImdbID, &s.Name, &s.OriginalName,
			&s.Overview, &s.FirstAirDate, &s.LastAirDate, &s.Status, &s.Type,
			&s.NumberOfSeasons, &s.NumberOfEpisodes, &s.EpisodeRunTime,
			&s.PosterPath, &s.BackdropPath, &s.VoteAverage, &s.VoteCount,
			&s.Popularity, &s.OriginalLanguage,
			&s.Genres, &s.Networks, &s.CreatedBy, &s.Credits,
			&s.ContentRating, &s.SyncedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		shows = append(shows, s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": shows, "total": total})
}

func (h *Handlers) CreateTVShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var s TVShow
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.SourceAccountID = sa
	if s.Genres == nil {
		s.Genres = []byte("[]")
	}
	if s.Networks == nil {
		s.Networks = []byte("[]")
	}
	if s.CreatedBy == nil {
		s.CreatedBy = []byte("[]")
	}
	if s.Credits == nil {
		s.Credits = []byte("{}")
	}
	if s.EpisodeRunTime == nil {
		s.EpisodeRunTime = []byte("{}")
	}
	_, err := h.pool.Exec(ctx,
		`INSERT INTO np_tmdb_tv_shows (id, source_account_id, imdb_id, name, original_name, overview,
		  first_air_date, last_air_date, status, type, number_of_seasons, number_of_episodes,
		  episode_run_time, poster_path, backdrop_path, vote_average, vote_count, popularity,
		  original_language, genres, networks, created_by, credits, content_rating, synced_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,NOW())
		 ON CONFLICT (id) DO UPDATE SET
		  source_account_id=EXCLUDED.source_account_id, imdb_id=EXCLUDED.imdb_id,
		  name=EXCLUDED.name, original_name=EXCLUDED.original_name,
		  overview=EXCLUDED.overview, first_air_date=EXCLUDED.first_air_date,
		  last_air_date=EXCLUDED.last_air_date, status=EXCLUDED.status,
		  type=EXCLUDED.type, number_of_seasons=EXCLUDED.number_of_seasons,
		  number_of_episodes=EXCLUDED.number_of_episodes, episode_run_time=EXCLUDED.episode_run_time,
		  poster_path=EXCLUDED.poster_path, backdrop_path=EXCLUDED.backdrop_path,
		  vote_average=EXCLUDED.vote_average, vote_count=EXCLUDED.vote_count,
		  popularity=EXCLUDED.popularity, original_language=EXCLUDED.original_language,
		  genres=EXCLUDED.genres, networks=EXCLUDED.networks, created_by=EXCLUDED.created_by,
		  credits=EXCLUDED.credits, content_rating=EXCLUDED.content_rating, synced_at=NOW()`,
		s.ID, sa, s.ImdbID, s.Name, s.OriginalName, s.Overview,
		s.FirstAirDate, s.LastAirDate, s.Status, s.Type, s.NumberOfSeasons, s.NumberOfEpisodes,
		s.EpisodeRunTime, s.PosterPath, s.BackdropPath, s.VoteAverage, s.VoteCount, s.Popularity,
		s.OriginalLanguage, s.Genres, s.Networks, s.CreatedBy, s.Credits, s.ContentRating)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (h *Handlers) GetTVShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var s TVShow
	err := h.pool.QueryRow(ctx,
		`SELECT id, source_account_id, imdb_id, name, original_name, overview,
		        first_air_date, last_air_date, status, type, number_of_seasons,
		        number_of_episodes, episode_run_time, poster_path, backdrop_path,
		        vote_average, vote_count, popularity, original_language,
		        genres, networks, created_by, credits, content_rating, synced_at
		 FROM np_tmdb_tv_shows WHERE id=$1 AND source_account_id=$2`, id, sa).
		Scan(&s.ID, &s.SourceAccountID, &s.ImdbID, &s.Name, &s.OriginalName,
			&s.Overview, &s.FirstAirDate, &s.LastAirDate, &s.Status, &s.Type,
			&s.NumberOfSeasons, &s.NumberOfEpisodes, &s.EpisodeRunTime,
			&s.PosterPath, &s.BackdropPath, &s.VoteAverage, &s.VoteCount,
			&s.Popularity, &s.OriginalLanguage,
			&s.Genres, &s.Networks, &s.CreatedBy, &s.Credits,
			&s.ContentRating, &s.SyncedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) UpdateTVShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var s TVShow
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	tag, err := h.pool.Exec(ctx,
		`UPDATE np_tmdb_tv_shows SET imdb_id=$3, name=$4, original_name=$5, overview=$6,
		  first_air_date=$7, last_air_date=$8, status=$9, type=$10,
		  number_of_seasons=$11, number_of_episodes=$12, episode_run_time=$13,
		  poster_path=$14, backdrop_path=$15, vote_average=$16, vote_count=$17,
		  popularity=$18, original_language=$19, genres=$20, networks=$21,
		  created_by=$22, credits=$23, content_rating=$24, synced_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, s.ImdbID, s.Name, s.OriginalName, s.Overview,
		s.FirstAirDate, s.LastAirDate, s.Status, s.Type,
		s.NumberOfSeasons, s.NumberOfEpisodes, s.EpisodeRunTime,
		s.PosterPath, s.BackdropPath, s.VoteAverage, s.VoteCount,
		s.Popularity, s.OriginalLanguage, s.Genres, s.Networks,
		s.CreatedBy, s.Credits, s.ContentRating)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "updated": true})
}

func (h *Handlers) DeleteTVShow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	tag, err := h.pool.Exec(ctx,
		`DELETE FROM np_tmdb_tv_shows WHERE id=$1 AND source_account_id=$2`, id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// Seasons

func (h *Handlers) ListSeasons(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	showID, _ := strconv.Atoi(chi.URLParam(r, "id"))

	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, show_id, season_number, name, overview,
		        poster_path, air_date, episode_count, synced_at
		 FROM np_tmdb_tv_seasons WHERE show_id=$1 AND source_account_id=$2
		 ORDER BY season_number`, showID, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	seasons := []Season{}
	for rows.Next() {
		var s Season
		if err := rows.Scan(&s.ID, &s.SourceAccountID, &s.ShowID, &s.SeasonNumber,
			&s.Name, &s.Overview, &s.PosterPath, &s.AirDate, &s.EpisodeCount, &s.SyncedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		seasons = append(seasons, s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": seasons, "total": len(seasons)})
}

func (h *Handlers) CreateSeason(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	showID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	var s Season
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.ShowID = showID
	s.SourceAccountID = sa
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_tmdb_tv_seasons (source_account_id, show_id, season_number, name, overview,
		  poster_path, air_date, episode_count, synced_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
		 ON CONFLICT (show_id, season_number) DO UPDATE SET
		  source_account_id=EXCLUDED.source_account_id, name=EXCLUDED.name,
		  overview=EXCLUDED.overview, poster_path=EXCLUDED.poster_path,
		  air_date=EXCLUDED.air_date, episode_count=EXCLUDED.episode_count, synced_at=NOW()
		 RETURNING id`,
		sa, showID, s.SeasonNumber, s.Name, s.Overview, s.PosterPath, s.AirDate, s.EpisodeCount).
		Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.ID = id
	writeJSON(w, http.StatusCreated, s)
}

// Episodes

func (h *Handlers) ListEpisodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	showID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	seasonNum, _ := strconv.Atoi(chi.URLParam(r, "season"))

	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, show_id, season_number, episode_number, name, overview,
		        still_path, air_date, runtime, vote_average, crew, guest_stars, synced_at
		 FROM np_tmdb_tv_episodes WHERE show_id=$1 AND season_number=$2 AND source_account_id=$3
		 ORDER BY episode_number`, showID, seasonNum, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	episodes := []Episode{}
	for rows.Next() {
		var e Episode
		if err := rows.Scan(&e.ID, &e.SourceAccountID, &e.ShowID, &e.SeasonNumber, &e.EpisodeNumber,
			&e.Name, &e.Overview, &e.StillPath, &e.AirDate, &e.Runtime,
			&e.VoteAverage, &e.Crew, &e.GuestStars, &e.SyncedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		episodes = append(episodes, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": episodes, "total": len(episodes)})
}

func (h *Handlers) CreateEpisode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	showID, _ := strconv.Atoi(chi.URLParam(r, "id"))
	seasonNum, _ := strconv.Atoi(chi.URLParam(r, "season"))
	var e Episode
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	e.ShowID = showID
	e.SeasonNumber = seasonNum
	e.SourceAccountID = sa
	if e.Crew == nil {
		e.Crew = []byte("[]")
	}
	if e.GuestStars == nil {
		e.GuestStars = []byte("[]")
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_tmdb_tv_episodes (source_account_id, show_id, season_number, episode_number,
		  name, overview, still_path, air_date, runtime, vote_average, crew, guest_stars, synced_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW())
		 ON CONFLICT (show_id, season_number, episode_number) DO UPDATE SET
		  source_account_id=EXCLUDED.source_account_id, name=EXCLUDED.name,
		  overview=EXCLUDED.overview, still_path=EXCLUDED.still_path,
		  air_date=EXCLUDED.air_date, runtime=EXCLUDED.runtime,
		  vote_average=EXCLUDED.vote_average, crew=EXCLUDED.crew,
		  guest_stars=EXCLUDED.guest_stars, synced_at=NOW()
		 RETURNING id`,
		sa, showID, seasonNum, e.EpisodeNumber, e.Name, e.Overview, e.StillPath,
		e.AirDate, e.Runtime, e.VoteAverage, e.Crew, e.GuestStars).
		Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	e.ID = id
	writeJSON(w, http.StatusCreated, e)
}

// Genres

func (h *Handlers) ListGenres(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	mediaType := r.URL.Query().Get("media_type")

	query := `SELECT id, source_account_id, name, media_type FROM np_tmdb_genres
	           WHERE source_account_id=$1 ORDER BY media_type, name`
	args := []any{sa}
	if mediaType != "" {
		query = `SELECT id, source_account_id, name, media_type FROM np_tmdb_genres
		          WHERE source_account_id=$1 AND media_type=$2 ORDER BY name`
		args = append(args, mediaType)
	}

	rows, err := h.pool.Query(ctx, query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	genres := []Genre{}
	for rows.Next() {
		var g Genre
		if err := rows.Scan(&g.ID, &g.SourceAccountID, &g.Name, &g.MediaType); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		genres = append(genres, g)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": genres, "total": len(genres)})
}

func (h *Handlers) CreateGenre(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var g Genre
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	g.SourceAccountID = sa
	_, err := h.pool.Exec(ctx,
		`INSERT INTO np_tmdb_genres (id, source_account_id, name, media_type)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, media_type=EXCLUDED.media_type`,
		g.ID, sa, g.Name, g.MediaType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

// Match Queue

func (h *Handlers) ListMatchQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	var total int
	countQuery := `SELECT COUNT(*) FROM np_tmdb_match_queue WHERE source_account_id=$1`
	listQuery := `SELECT id, source_account_id, media_id, filename, parsed_title, parsed_year,
	                     parsed_type, match_results, best_match_id, best_match_type, confidence,
	                     status, reviewed_by, reviewed_at, auto_accepted, created_at, updated_at
	              FROM np_tmdb_match_queue WHERE source_account_id=$1
	              ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	countArgs := []any{sa}
	listArgs := []any{sa, limit, offset}

	if status != "" {
		countQuery = `SELECT COUNT(*) FROM np_tmdb_match_queue WHERE source_account_id=$1 AND status=$2`
		listQuery = `SELECT id, source_account_id, media_id, filename, parsed_title, parsed_year,
		                    parsed_type, match_results, best_match_id, best_match_type, confidence,
		                    status, reviewed_by, reviewed_at, auto_accepted, created_at, updated_at
		             FROM np_tmdb_match_queue WHERE source_account_id=$1 AND status=$2
		             ORDER BY created_at DESC LIMIT $3 OFFSET $4`
		countArgs = []any{sa, status}
		listArgs = []any{sa, status, limit, offset}
	}

	h.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
	rows, err := h.pool.Query(ctx, listQuery, listArgs...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	items := []MatchQueue{}
	for rows.Next() {
		var mq MatchQueue
		if err := rows.Scan(&mq.ID, &mq.SourceAccountID, &mq.MediaID, &mq.Filename,
			&mq.ParsedTitle, &mq.ParsedYear, &mq.ParsedType, &mq.MatchResults,
			&mq.BestMatchID, &mq.BestMatchType, &mq.Confidence,
			&mq.Status, &mq.ReviewedBy, &mq.ReviewedAt, &mq.AutoAccepted,
			&mq.CreatedAt, &mq.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		items = append(items, mq)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": total})
}

func (h *Handlers) CreateMatchQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var mq MatchQueue
	if err := json.NewDecoder(r.Body).Decode(&mq); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	mq.SourceAccountID = sa
	if mq.Status == "" {
		mq.Status = "pending"
	}
	if mq.MatchResults == nil {
		mq.MatchResults = []byte("[]")
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO np_tmdb_match_queue (source_account_id, media_id, filename, parsed_title,
		  parsed_year, parsed_type, match_results, best_match_id, best_match_type,
		  confidence, status, auto_accepted)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 RETURNING id`,
		sa, mq.MediaID, mq.Filename, mq.ParsedTitle, mq.ParsedYear, mq.ParsedType,
		mq.MatchResults, mq.BestMatchID, mq.BestMatchType, mq.Confidence,
		mq.Status, mq.AutoAccepted).
		Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	mq.ID = id
	writeJSON(w, http.StatusCreated, mq)
}

func (h *Handlers) UpdateMatchQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body struct {
		Status      string  `json:"status"`
		ReviewedBy  *string `json:"reviewed_by"`
		BestMatchID *int    `json:"best_match_id"`
		BestMatchType *string `json:"best_match_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	tag, err := h.pool.Exec(ctx,
		`UPDATE np_tmdb_match_queue SET
		  status=$3, reviewed_by=$4, reviewed_at=NOW(),
		  best_match_id=COALESCE($5, best_match_id),
		  best_match_type=COALESCE($6, best_match_type),
		  updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2`,
		id, sa, body.Status, body.ReviewedBy, body.BestMatchID, body.BestMatchType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "updated": true})
}

// Stats

func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)

	type counts struct{ label, table, extra string }
	tables := []counts{
		{"movies", "np_tmdb_movies", ""},
		{"tv_shows", "np_tmdb_tv_shows", ""},
		{"seasons", "np_tmdb_tv_seasons", ""},
		{"episodes", "np_tmdb_tv_episodes", ""},
		{"genres", "np_tmdb_genres", ""},
	}

	stats := map[string]any{}
	for _, t := range tables {
		var n int
		h.pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE source_account_id=$1`, t.table), sa).Scan(&n)
		stats[t.label] = n
	}

	matchCounts := map[string]int{}
	for _, status := range []string{"pending", "accepted", "rejected", "manual"} {
		var n int
		h.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM np_tmdb_match_queue WHERE source_account_id=$1 AND status=$2`, sa, status).Scan(&n)
		matchCounts[status] = n
	}
	stats["match_queue"] = matchCounts

	var lastSynced *time.Time
	h.pool.QueryRow(ctx,
		`SELECT synced_at FROM np_tmdb_movies WHERE source_account_id=$1 ORDER BY synced_at DESC LIMIT 1`, sa).Scan(&lastSynced)
	if lastSynced != nil {
		stats["last_synced"] = lastSynced.Format(time.RFC3339)
	} else {
		stats["last_synced"] = nil
	}

	writeJSON(w, http.StatusOK, stats)
}

func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query is required"})
		return
	}
	if req.MediaType == "" {
		req.MediaType = "multi"
	}
	lang := req.Language
	if lang == "" {
		lang = h.cfg.DefaultLanguage
	}
	params := url.Values{"query": {req.Query}, "language": {lang}}
	if req.Year != nil {
		params.Set("year", strconv.Itoa(*req.Year))
	}
	var apiResp TMDBSearchResponse
	if err := h.tmdbGet(r.Context(), "/search/"+req.MediaType, params, &apiResp); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	results := make([]SearchResult, 0, len(apiResp.Results))
	for _, item := range apiResp.Results {
		results = append(results, tmdbItemToSearchResult(item, req.MediaType))
	}
	writeJSON(w, http.StatusOK, SearchResponse{Results: results, TotalPages: apiResp.TotalPages, TotalCount: apiResp.TotalResults})
}

func (h *Handlers) Match(w http.ResponseWriter, r *http.Request) {
	var req MatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.MediaID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "media_id is required"})
		return
	}
	if req.SourceAccountID == "" {
		req.SourceAccountID = h.sa(r)
	}
	resp, err := h.performMatch(r.Context(), &req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) MatchBatch(w http.ResponseWriter, r *http.Request) {
	var req BatchMatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if len(req.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "items is required"})
		return
	}
	sa := req.SourceAccountID
	if sa == "" {
		sa = h.sa(r)
	}
	results := make([]MatchResponse, 0, len(req.Items))
	for i := range req.Items {
		if req.Items[i].SourceAccountID == "" {
			req.Items[i].SourceAccountID = sa
		}
		resp, err := h.performMatch(r.Context(), &req.Items[i])
		if err != nil {
			results = append(results, MatchResponse{Status: "error"})
			continue
		}
		results = append(results, *resp)
	}
	writeJSON(w, http.StatusOK, BatchMatchResponse{Results: results})
}

func (h *Handlers) GetQueue(w http.ResponseWriter, r *http.Request) {
	sa := r.URL.Query().Get("source_account_id")
	if sa == "" {
		sa = h.sa(r)
	}
	statusFilter := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, err := GetQueue(r.Context(), h.pool, sa, statusFilter, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []MatchQueue{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

func (h *Handlers) Confirm(w http.ResponseWriter, r *http.Request) {
	var req ConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.QueueID == "" || req.TMDBId == 0 || req.MediaType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "queue_id, tmdb_id, media_type required"})
		return
	}
	sa := req.SourceAccountID
	if sa == "" {
		sa = h.sa(r)
	}
	if err := ConfirmMatchQueueEntry(r.Context(), h.pool, req.QueueID, req.TMDBId, req.MediaType, req.ReviewedBy, sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := h.fetchAndStore(r.Context(), req.TMDBId, req.MediaType, sa); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "confirmed", "warning": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

func (h *Handlers) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.TMDBID == 0 || req.MediaType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tmdb_id and media_type required"})
		return
	}
	sa := req.SourceAccountID
	if sa == "" {
		sa = h.sa(r)
	}
	if err := h.fetchAndStore(r.Context(), req.TMDBID, req.MediaType, sa); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed"})
}

func (h *Handlers) Sync(w http.ResponseWriter, r *http.Request) {
	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	sa := req.SourceAccountID
	if sa == "" {
		sa = h.sa(r)
	}
	langParams := url.Values{"language": {h.cfg.DefaultLanguage}}
	var movieGenres TMDBGenreList
	if err := h.tmdbGet(r.Context(), "/genre/movie/list", langParams, &movieGenres); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "fetching movie genres: " + err.Error()})
		return
	}
	if err := UpsertGenres(r.Context(), h.pool, movieGenres.Genres, "movie", sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var tvGenres TMDBGenreList
	if err := h.tmdbGet(r.Context(), "/genre/tv/list", langParams, &tvGenres); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "fetching tv genres: " + err.Error()})
		return
	}
	if err := UpsertGenres(r.Context(), h.pool, tvGenres.Genres, "tv", sa); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "synced", "movie_genres": len(movieGenres.Genres), "tv_genres": len(tvGenres.Genres)})
}

func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	sa := r.URL.Query().Get("source_account_id")
	if sa == "" {
		sa = h.sa(r)
	}
	movies, _ := CountMovies(r.Context(), h.pool, sa)
	tvShows, _ := CountTVShows(r.Context(), h.pool, sa)
	pending, _ := CountPendingMatches(r.Context(), h.pool, sa)
	genres, _ := CountGenres(r.Context(), h.pool, sa)
	writeJSON(w, http.StatusOK, StatusResponse{TotalMovies: movies, TotalTVShows: tvShows, PendingMatches: pending, TotalGenres: genres})
}

// ---- internal helpers ----

const tmdbAPIBase = "https://api.themoviedb.org/3"

func (h *Handlers) tmdbGet(ctx context.Context, endpoint string, params url.Values, dest interface{}) error {
	if h.cfg.TMDBApiKey == "" {
		return fmt.Errorf("TMDB_API_KEY not configured")
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("api_key", h.cfg.TMDBApiKey)
	reqURL := tmdbAPIBase + endpoint + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := tmdbClient.Do(req)
	if err != nil {
		return fmt.Errorf("doing request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tmdb returned %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

func (h *Handlers) fetchAndStore(ctx context.Context, tmdbID int, mediaType, sourceAccountID string) error {
	switch mediaType {
	case "movie":
		params := url.Values{"language": {h.cfg.DefaultLanguage}, "append_to_response": {"credits,keywords"}}
		var details TMDBMovieDetails
		if err := h.tmdbGet(ctx, fmt.Sprintf("/movie/%d", tmdbID), params, &details); err != nil {
			return fmt.Errorf("fetching movie %d: %w", tmdbID, err)
		}
		return UpsertMovie(ctx, h.pool, &details, sourceAccountID)
	case "tv":
		params := url.Values{"language": {h.cfg.DefaultLanguage}, "append_to_response": {"credits,external_ids"}}
		var details TMDBTVDetails
		if err := h.tmdbGet(ctx, fmt.Sprintf("/tv/%d", tmdbID), params, &details); err != nil {
			return fmt.Errorf("fetching tv %d: %w", tmdbID, err)
		}
		return UpsertTVShow(ctx, h.pool, &details, sourceAccountID)
	default:
		return fmt.Errorf("unknown media type: %s", mediaType)
	}
}

func (h *Handlers) performMatch(ctx context.Context, req *MatchRequest) (*MatchResponse, error) {
	title := req.Title
	if title == "" && req.Filename != "" {
		title = parseTitle(req.Filename)
	}
	if title == "" {
		return nil, fmt.Errorf("title or filename is required")
	}
	mediaType := req.MediaType
	if mediaType == "" {
		mediaType = "multi"
	}
	sa := req.SourceAccountID
	if sa == "" {
		sa = "primary"
	}
	params := url.Values{"query": {title}, "language": {h.cfg.DefaultLanguage}}
	if req.Year != nil {
		params.Set("year", strconv.Itoa(*req.Year))
	}
	var apiResp TMDBSearchResponse
	if err := h.tmdbGet(ctx, "/search/"+mediaType, params, &apiResp); err != nil {
		return nil, fmt.Errorf("tmdb search: %w", err)
	}
	candidates := make([]SearchResult, 0, len(apiResp.Results))
	for _, item := range apiResp.Results {
		candidates = append(candidates, tmdbItemToSearchResult(item, mediaType))
	}
	var confidence float64
	var bestMatch *SearchResult
	if len(candidates) > 0 {
		best := candidates[0]
		confidence = computeConfidence(title, best.Title, req.Year, best.ReleaseDate)
		bestMatch = &best
	}
	status := "pending"
	autoAccepted := false
	if confidence >= h.cfg.AutoAcceptThreshold && bestMatch != nil {
		status = "auto_accepted"
		autoAccepted = true
	}
	matchResultsJSON, _ := json.Marshal(candidates)
	var bestMatchID *int
	var bestMatchType *string
	if bestMatch != nil {
		id := bestMatch.ID
		mt := bestMatch.MediaType
		bestMatchID = &id
		bestMatchType = &mt
	}
	entry := &MatchQueue{
		SourceAccountID: sa,
		MediaID:         req.MediaID,
		Filename:        strPtr(req.Filename),
		ParsedTitle:     strPtr(title),
		ParsedYear:      req.Year,
		ParsedType:      strPtr(mediaType),
		MatchResults:    matchResultsJSON,
		BestMatchID:     bestMatchID,
		BestMatchType:   bestMatchType,
		Confidence:      &confidence,
		Status:          status,
		AutoAccepted:    autoAccepted,
	}
	queueID, err := InsertMatchQueueEntry(ctx, h.pool, entry)
	if err != nil {
		return nil, fmt.Errorf("inserting queue entry: %w", err)
	}
	return &MatchResponse{
		QueueID:      queueID,
		Status:       status,
		Confidence:   confidence,
		BestMatch:    bestMatch,
		Candidates:   candidates,
		AutoAccepted: autoAccepted,
	}, nil
}

func tmdbItemToSearchResult(item TMDBSearchItem, defaultType string) SearchResult {
	title := item.Title
	if title == "" {
		title = item.Name
	}
	releaseDate := item.ReleaseDate
	if releaseDate == "" {
		releaseDate = item.FirstAirDate
	}
	mediaType := item.MediaType
	if mediaType == "" {
		mediaType = defaultType
	}
	return SearchResult{
		ID: item.ID, MediaType: mediaType, Title: title, Overview: item.Overview,
		ReleaseDate: releaseDate, PosterPath: item.PosterPath, BackdropPath: item.BackdropPath,
		VoteAverage: item.VoteAverage, Popularity: item.Popularity,
	}
}

func parseTitle(filename string) string {
	if i := strings.LastIndex(filename, "."); i > 0 {
		filename = filename[:i]
	}
	filename = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(filename)
	parts := strings.Fields(filename)
	if len(parts) > 1 {
		if last := parts[len(parts)-1]; len(last) == 4 {
			if _, err := strconv.Atoi(last); err == nil {
				parts = parts[:len(parts)-1]
			}
		}
	}
	return strings.Join(parts, " ")
}

func computeConfidence(query, result string, queryYear *int, resultDate string) float64 {
	q := strings.ToLower(strings.TrimSpace(query))
	res := strings.ToLower(strings.TrimSpace(result))
	if q == "" || res == "" {
		return 0
	}
	if q == res {
		return 1.0
	}
	if strings.Contains(res, q) || strings.Contains(q, res) {
		conf := 0.8
		if queryYear != nil && len(resultDate) >= 4 && strings.HasPrefix(resultDate, strconv.Itoa(*queryYear)) {
			conf = 0.9
		}
		return conf
	}
	qParts, rParts := strings.Fields(q), strings.Fields(res)
	if len(qParts) == 0 {
		return 0
	}
	overlap := 0
	for _, qp := range qParts {
		for _, rp := range rParts {
			if qp == rp {
				overlap++
				break
			}
		}
	}
	return float64(overlap) / float64(len(qParts)) * 0.7
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
