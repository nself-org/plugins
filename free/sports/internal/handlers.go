package internal

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-sports/internal/provider"
)

type Handlers struct {
	pool     *pgxpool.Pool
	provider provider.Provider
}

// NewHandlersWithProvider creates Handlers with provider support for sync routes.
func NewHandlersWithProvider(pool *pgxpool.Pool, p provider.Provider) *Handlers {
	return &Handlers{pool: pool, provider: p}
}

func NewHandlers(pool *pgxpool.Pool) *Handlers {
	return &Handlers{pool: pool}
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

// ============================================================
// Health
// ============================================================

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "plugin": "sports"})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	if err := h.pool.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// ============================================================
// Leagues
// ============================================================

func (h *Handlers) ListLeagues(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, external_id, provider, name, abbreviation,
		        sport, country, season_type, current_season, logo_url, metadata,
		        created_at, updated_at, synced_at
		 FROM np_sports_leagues
		 WHERE source_account_id = $1
		 ORDER BY name ASC`, h.sa(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var results []League
	for rows.Next() {
		var l League
		if err := rows.Scan(&l.ID, &l.SourceAccountID, &l.ExternalID, &l.Provider, &l.Name,
			&l.Abbreviation, &l.Sport, &l.Country, &l.SeasonType, &l.CurrentSeason,
			&l.LogoURL, &l.Metadata, &l.CreatedAt, &l.UpdatedAt, &l.SyncedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, l)
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handlers) GetLeague(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var l League
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, external_id, provider, name, abbreviation,
		        sport, country, season_type, current_season, logo_url, metadata,
		        created_at, updated_at, synced_at
		 FROM np_sports_leagues WHERE id = $1 AND source_account_id = $2`, id, h.sa(r)).
		Scan(&l.ID, &l.SourceAccountID, &l.ExternalID, &l.Provider, &l.Name,
			&l.Abbreviation, &l.Sport, &l.Country, &l.SeasonType, &l.CurrentSeason,
			&l.LogoURL, &l.Metadata, &l.CreatedAt, &l.UpdatedAt, &l.SyncedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (h *Handlers) CreateLeague(w http.ResponseWriter, r *http.Request) {
	var l League
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	l.SourceAccountID = h.sa(r)
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_sports_leagues (source_account_id, external_id, provider, name, abbreviation,
		 sport, country, season_type, current_season, logo_url, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (source_account_id, provider, external_id)
		 DO UPDATE SET name=EXCLUDED.name, abbreviation=EXCLUDED.abbreviation,
		   country=EXCLUDED.country, season_type=EXCLUDED.season_type,
		   current_season=EXCLUDED.current_season, logo_url=EXCLUDED.logo_url,
		   metadata=EXCLUDED.metadata, updated_at=NOW(), synced_at=NOW()
		 RETURNING id, created_at, updated_at, synced_at`,
		l.SourceAccountID, l.ExternalID, l.Provider, l.Name, l.Abbreviation,
		l.Sport, l.Country, l.SeasonType, l.CurrentSeason, l.LogoURL, l.Metadata).
		Scan(&l.ID, &l.CreatedAt, &l.UpdatedAt, &l.SyncedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (h *Handlers) UpdateLeague(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var l League
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`UPDATE np_sports_leagues SET name=$1, abbreviation=$2, country=$3,
		 season_type=$4, current_season=$5, logo_url=$6, metadata=$7, updated_at=NOW()
		 WHERE id=$8 AND source_account_id=$9`,
		l.Name, l.Abbreviation, l.Country, l.SeasonType, l.CurrentSeason,
		l.LogoURL, l.Metadata, id, h.sa(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	l.ID = id
	writeJSON(w, http.StatusOK, l)
}

func (h *Handlers) DeleteLeague(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.pool.Exec(r.Context(), `DELETE FROM np_sports_leagues WHERE id=$1 AND source_account_id=$2`, id, h.sa(r))
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Teams
// ============================================================

func (h *Handlers) ListTeams(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, league_id, external_id, provider, name, abbreviation,
		        city, conference, division, logo_url, primary_color, secondary_color,
		        venue_name, venue_city, venue_timezone, metadata, created_at, updated_at, synced_at
		 FROM np_sports_teams WHERE source_account_id = $1 ORDER BY name ASC`, h.sa(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var results []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.SourceAccountID, &t.LeagueID, &t.ExternalID, &t.Provider,
			&t.Name, &t.Abbreviation, &t.City, &t.Conference, &t.Division, &t.LogoURL,
			&t.PrimaryColor, &t.SecondaryColor, &t.VenueName, &t.VenueCity, &t.VenueTimezone,
			&t.Metadata, &t.CreatedAt, &t.UpdatedAt, &t.SyncedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, t)
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handlers) GetTeam(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var t Team
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, league_id, external_id, provider, name, abbreviation,
		        city, conference, division, logo_url, primary_color, secondary_color,
		        venue_name, venue_city, venue_timezone, metadata, created_at, updated_at, synced_at
		 FROM np_sports_teams WHERE id = $1 AND source_account_id = $2`, id, h.sa(r)).
		Scan(&t.ID, &t.SourceAccountID, &t.LeagueID, &t.ExternalID, &t.Provider,
			&t.Name, &t.Abbreviation, &t.City, &t.Conference, &t.Division, &t.LogoURL,
			&t.PrimaryColor, &t.SecondaryColor, &t.VenueName, &t.VenueCity, &t.VenueTimezone,
			&t.Metadata, &t.CreatedAt, &t.UpdatedAt, &t.SyncedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handlers) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var t Team
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	t.SourceAccountID = h.sa(r)
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_sports_teams (source_account_id, league_id, external_id, provider, name, abbreviation,
		 city, conference, division, logo_url, primary_color, secondary_color, venue_name, venue_city, venue_timezone, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 ON CONFLICT (source_account_id, provider, external_id)
		 DO UPDATE SET name=EXCLUDED.name, abbreviation=EXCLUDED.abbreviation,
		   city=EXCLUDED.city, conference=EXCLUDED.conference, division=EXCLUDED.division,
		   logo_url=EXCLUDED.logo_url, primary_color=EXCLUDED.primary_color,
		   secondary_color=EXCLUDED.secondary_color, venue_name=EXCLUDED.venue_name,
		   venue_city=EXCLUDED.venue_city, venue_timezone=EXCLUDED.venue_timezone,
		   metadata=EXCLUDED.metadata, updated_at=NOW(), synced_at=NOW()
		 RETURNING id, created_at, updated_at, synced_at`,
		t.SourceAccountID, t.LeagueID, t.ExternalID, t.Provider, t.Name, t.Abbreviation,
		t.City, t.Conference, t.Division, t.LogoURL, t.PrimaryColor, t.SecondaryColor,
		t.VenueName, t.VenueCity, t.VenueTimezone, t.Metadata).
		Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt, &t.SyncedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *Handlers) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var t Team
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.pool.Exec(r.Context(),
		`UPDATE np_sports_teams SET name=$1, abbreviation=$2, city=$3, conference=$4,
		 division=$5, logo_url=$6, primary_color=$7, secondary_color=$8,
		 venue_name=$9, venue_city=$10, venue_timezone=$11, metadata=$12, updated_at=NOW()
		 WHERE id=$13 AND source_account_id=$14`,
		t.Name, t.Abbreviation, t.City, t.Conference, t.Division, t.LogoURL,
		t.PrimaryColor, t.SecondaryColor, t.VenueName, t.VenueCity, t.VenueTimezone,
		t.Metadata, id, h.sa(r))
	t.ID = id
	writeJSON(w, http.StatusOK, t)
}

func (h *Handlers) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.pool.Exec(r.Context(), `DELETE FROM np_sports_teams WHERE id=$1 AND source_account_id=$2`, id, h.sa(r))
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Events
// ============================================================

func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, external_id, provider, canonical_id,
		        league_id, home_team_id, away_team_id, event_type, status,
		        scheduled_at, started_at, ended_at, venue_name, venue_city, venue_timezone,
		        broadcast_network, broadcast_channel, season, season_type, week,
		        home_score, away_score, period, clock, is_final, is_locked, lock_reason, locked_at,
		        operator_override, operator_notes, recording_trigger_sent, recording_trigger_sent_at,
		        metadata, created_at, updated_at, synced_at, deleted_at
		 FROM np_sports_events
		 WHERE source_account_id = $1 AND deleted_at IS NULL
		 ORDER BY scheduled_at DESC`, h.sa(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var results []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.SourceAccountID, &e.ExternalID, &e.Provider, &e.CanonicalID,
			&e.LeagueID, &e.HomeTeamID, &e.AwayTeamID, &e.EventType, &e.Status,
			&e.ScheduledAt, &e.StartedAt, &e.EndedAt, &e.VenueName, &e.VenueCity, &e.VenueTimezone,
			&e.BroadcastNetwork, &e.BroadcastChannel, &e.Season, &e.SeasonType, &e.Week,
			&e.HomeScore, &e.AwayScore, &e.Period, &e.Clock, &e.IsFinal, &e.IsLocked,
			&e.LockReason, &e.LockedAt, &e.OperatorOverride, &e.OperatorNotes,
			&e.RecordingTriggerSent, &e.RecordingTriggerSentAt,
			&e.Metadata, &e.CreatedAt, &e.UpdatedAt, &e.SyncedAt, &e.DeletedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, e)
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handlers) GetEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var e Event
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, external_id, provider, canonical_id,
		        league_id, home_team_id, away_team_id, event_type, status,
		        scheduled_at, started_at, ended_at, venue_name, venue_city, venue_timezone,
		        broadcast_network, broadcast_channel, season, season_type, week,
		        home_score, away_score, period, clock, is_final, is_locked, lock_reason, locked_at,
		        operator_override, operator_notes, recording_trigger_sent, recording_trigger_sent_at,
		        metadata, created_at, updated_at, synced_at, deleted_at
		 FROM np_sports_events WHERE id=$1 AND source_account_id=$2 AND deleted_at IS NULL`, id, h.sa(r)).
		Scan(&e.ID, &e.SourceAccountID, &e.ExternalID, &e.Provider, &e.CanonicalID,
			&e.LeagueID, &e.HomeTeamID, &e.AwayTeamID, &e.EventType, &e.Status,
			&e.ScheduledAt, &e.StartedAt, &e.EndedAt, &e.VenueName, &e.VenueCity, &e.VenueTimezone,
			&e.BroadcastNetwork, &e.BroadcastChannel, &e.Season, &e.SeasonType, &e.Week,
			&e.HomeScore, &e.AwayScore, &e.Period, &e.Clock, &e.IsFinal, &e.IsLocked,
			&e.LockReason, &e.LockedAt, &e.OperatorOverride, &e.OperatorNotes,
			&e.RecordingTriggerSent, &e.RecordingTriggerSentAt,
			&e.Metadata, &e.CreatedAt, &e.UpdatedAt, &e.SyncedAt, &e.DeletedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *Handlers) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var e Event
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	e.SourceAccountID = h.sa(r)
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_sports_events
		 (source_account_id, external_id, provider, canonical_id, league_id, home_team_id, away_team_id,
		  event_type, status, scheduled_at, venue_name, venue_city, venue_timezone,
		  broadcast_network, broadcast_channel, season, season_type, week, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		 ON CONFLICT (source_account_id, provider, external_id)
		 DO UPDATE SET status=EXCLUDED.status, scheduled_at=EXCLUDED.scheduled_at,
		   home_score=EXCLUDED.home_score, away_score=EXCLUDED.away_score,
		   period=EXCLUDED.period, clock=EXCLUDED.clock, is_final=EXCLUDED.is_final,
		   metadata=EXCLUDED.metadata, updated_at=NOW(), synced_at=NOW()
		 RETURNING id, created_at, updated_at, synced_at`,
		e.SourceAccountID, e.ExternalID, e.Provider, e.CanonicalID, e.LeagueID, e.HomeTeamID, e.AwayTeamID,
		e.EventType, e.Status, e.ScheduledAt, e.VenueName, e.VenueCity, e.VenueTimezone,
		e.BroadcastNetwork, e.BroadcastChannel, e.Season, e.SeasonType, e.Week, e.Metadata).
		Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt, &e.SyncedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handlers) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var e Event
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.pool.Exec(r.Context(),
		`UPDATE np_sports_events SET status=$1, home_score=$2, away_score=$3, period=$4, clock=$5,
		 is_final=$6, is_locked=$7, lock_reason=$8, operator_override=$9, operator_notes=$10,
		 metadata=$11, updated_at=NOW()
		 WHERE id=$12 AND source_account_id=$13`,
		e.Status, e.HomeScore, e.AwayScore, e.Period, e.Clock,
		e.IsFinal, e.IsLocked, e.LockReason, e.OperatorOverride, e.OperatorNotes,
		e.Metadata, id, h.sa(r))
	e.ID = id
	writeJSON(w, http.StatusOK, e)
}

func (h *Handlers) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.pool.Exec(r.Context(),
		`UPDATE np_sports_events SET deleted_at=NOW() WHERE id=$1 AND source_account_id=$2`, id, h.sa(r))
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Standings
// ============================================================

func (h *Handlers) ListStandings(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, league_id, team_id, wins, losses, draws, points, rank, season, synced_at
		 FROM np_sports_standings WHERE source_account_id=$1 ORDER BY rank ASC`, h.sa(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var results []Standing
	for rows.Next() {
		var s Standing
		if err := rows.Scan(&s.ID, &s.SourceAccountID, &s.LeagueID, &s.TeamID,
			&s.Wins, &s.Losses, &s.Draws, &s.Points, &s.Rank, &s.Season, &s.SyncedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, s)
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handlers) UpsertStanding(w http.ResponseWriter, r *http.Request) {
	var s Standing
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.SourceAccountID = h.sa(r)
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_sports_standings (source_account_id, league_id, team_id, wins, losses, draws, points, rank, season)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (source_account_id, league_id, team_id, season)
		 DO UPDATE SET wins=EXCLUDED.wins, losses=EXCLUDED.losses, draws=EXCLUDED.draws,
		   points=EXCLUDED.points, rank=EXCLUDED.rank, synced_at=NOW()
		 RETURNING id, synced_at`,
		s.SourceAccountID, s.LeagueID, s.TeamID, s.Wins, s.Losses, s.Draws, s.Points, s.Rank, s.Season).
		Scan(&s.ID, &s.SyncedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// ============================================================
// Favorites
// ============================================================

func (h *Handlers) ListFavorites(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, user_id, team_id, notify_live, auto_record, created_at
		 FROM np_np_sports_favorites WHERE source_account_id=$1 ORDER BY created_at DESC`, h.sa(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var results []Favorite
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ID, &f.SourceAccountID, &f.UserID, &f.TeamID,
			&f.NotifyLive, &f.AutoRecord, &f.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, f)
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handlers) CreateFavorite(w http.ResponseWriter, r *http.Request) {
	var f Favorite
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	f.SourceAccountID = h.sa(r)
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO np_np_sports_favorites (source_account_id, user_id, team_id, notify_live, auto_record)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (source_account_id, user_id, team_id)
		 DO UPDATE SET notify_live=EXCLUDED.notify_live, auto_record=EXCLUDED.auto_record
		 RETURNING id, created_at`,
		f.SourceAccountID, f.UserID, f.TeamID, f.NotifyLive, f.AutoRecord).
		Scan(&f.ID, &f.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (h *Handlers) DeleteFavorite(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.pool.Exec(r.Context(), `DELETE FROM np_np_sports_favorites WHERE id=$1 AND source_account_id=$2`, id, h.sa(r))
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// Provider Syncs
// ============================================================

func (h *Handlers) ListSyncs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, provider, resource_type, status, started_at, completed_at,
		        records_synced, errors, metadata, created_at
		 FROM sports_provider_syncs WHERE source_account_id=$1 ORDER BY created_at DESC LIMIT 100`, h.sa(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var results []ProviderSync
	for rows.Next() {
		var s ProviderSync
		if err := rows.Scan(&s.ID, &s.SourceAccountID, &s.Provider, &s.ResourceType, &s.Status,
			&s.StartedAt, &s.CompletedAt, &s.RecordsSynced, &s.Errors, &s.Metadata, &s.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, s)
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handlers) CreateSync(w http.ResponseWriter, r *http.Request) {
	var s ProviderSync
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.SourceAccountID = h.sa(r)
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO sports_provider_syncs (source_account_id, provider, resource_type, status)
		 VALUES ($1,$2,$3,$4) RETURNING id, created_at`,
		s.SourceAccountID, s.Provider, s.ResourceType, s.Status).
		Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

// ============================================================
// Webhook Events
// ============================================================

func (h *Handlers) ListWebhookEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, provider, event_type, event_id, payload,
		        processed, processed_at, error, created_at
		 FROM sports_webhook_events WHERE source_account_id=$1 ORDER BY created_at DESC LIMIT 100`, h.sa(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var results []WebhookEvent
	for rows.Next() {
		var we WebhookEvent
		if err := rows.Scan(&we.ID, &we.SourceAccountID, &we.Provider, &we.EventType, &we.EventID,
			&we.Payload, &we.Processed, &we.ProcessedAt, &we.Error, &we.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, we)
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handlers) CreateWebhookEvent(w http.ResponseWriter, r *http.Request) {
	var we WebhookEvent
	if err := json.NewDecoder(r.Body).Decode(&we); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	we.SourceAccountID = h.sa(r)
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO sports_webhook_events (source_account_id, provider, event_type, event_id, payload)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at`,
		we.SourceAccountID, we.Provider, we.EventType, we.EventID, we.Payload).
		Scan(&we.ID, &we.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, we)
}

// ============================================================
// Schedule Cache
// ============================================================

func (h *Handlers) ListCache(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, source_account_id, provider, cache_key, data, fetched_at, expires_at
		 FROM sports_schedule_cache WHERE source_account_id=$1 ORDER BY fetched_at DESC`, h.sa(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	var results []ScheduleCache
	for rows.Next() {
		var sc ScheduleCache
		if err := rows.Scan(&sc.ID, &sc.SourceAccountID, &sc.Provider, &sc.CacheKey,
			&sc.Data, &sc.FetchedAt, &sc.ExpiresAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, sc)
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handlers) UpsertCache(w http.ResponseWriter, r *http.Request) {
	var sc ScheduleCache
	if err := json.NewDecoder(r.Body).Decode(&sc); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	sc.SourceAccountID = h.sa(r)
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO sports_schedule_cache (source_account_id, provider, cache_key, data, expires_at)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (source_account_id, provider, cache_key)
		 DO UPDATE SET data=EXCLUDED.data, fetched_at=NOW(), expires_at=EXCLUDED.expires_at
		 RETURNING id, fetched_at`,
		sc.SourceAccountID, sc.Provider, sc.CacheKey, sc.Data, sc.ExpiresAt).
		Scan(&sc.ID, &sc.FetchedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

// ============================================================
// Sync triggers (POST /sports/sync/*)
// ============================================================

// SyncLeagues triggers a league sync for a given sport (query param: sport).
func (h *Handlers) SyncLeagues(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "provider not configured"})
		return
	}
	sportParam := r.URL.Query().Get("sport")
	if sportParam == "" {
		sportParam = string(provider.SportSoccer)
	}
	s := NewSyncer(h.pool, h.provider, h.sa(r))
	n, err := s.SyncLeagues(r.Context(), provider.Sport(sportParam))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": n, "sport": sportParam})
}

// SyncTeams triggers a team sync for the given league_id query param.
func (h *Handlers) SyncTeams(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "provider not configured"})
		return
	}
	leagueID := r.URL.Query().Get("league_id")
	if leagueID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "league_id is required"})
		return
	}
	s := NewSyncer(h.pool, h.provider, h.sa(r))
	n, err := s.SyncTeams(r.Context(), leagueID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": n, "league_id": leagueID})
}

// SyncEvents triggers an event sync. Query params: league_id, from (YYYY-MM-DD), to (YYYY-MM-DD).
func (h *Handlers) SyncEvents(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "provider not configured"})
		return
	}
	leagueID := r.URL.Query().Get("league_id")
	if leagueID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "league_id is required"})
		return
	}
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = t
		}
	}
	s := NewSyncer(h.pool, h.provider, h.sa(r))
	n, err := s.SyncEvents(r.Context(), leagueID, from, to)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": n, "league_id": leagueID, "from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")})
}

// SyncLiveScores triggers a live-score update pass.
func (h *Handlers) SyncLiveScores(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "provider not configured"})
		return
	}
	s := NewSyncer(h.pool, h.provider, h.sa(r))
	n, err := s.SyncLiveScores(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": n})
}

// ============================================================
// Live scores (GET /sports/live)
// ============================================================

// LiveScores returns all non-final events scheduled for today.
func (h *Handlers) LiveScores(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, external_id, provider, league_id, home_team_id, away_team_id,
		        status, scheduled_at, home_score, away_score, period, clock, is_final
		 FROM np_sports_events
		 WHERE source_account_id=$1
		   AND scheduled_at >= $2
		   AND scheduled_at < $3
		   AND is_final = FALSE
		   AND deleted_at IS NULL
		 ORDER BY scheduled_at ASC`, h.sa(r), dayStart, dayEnd)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type liveEvent struct {
		ID          string     `json:"id"`
		ExternalID  string     `json:"external_id"`
		Provider    string     `json:"provider"`
		LeagueID    *string    `json:"league_id"`
		HomeTeamID  *string    `json:"home_team_id"`
		AwayTeamID  *string    `json:"away_team_id"`
		Status      string     `json:"status"`
		ScheduledAt time.Time  `json:"scheduled_at"`
		HomeScore   *int       `json:"home_score"`
		AwayScore   *int       `json:"away_score"`
		Period      *string    `json:"period"`
		Clock       *string    `json:"clock"`
		IsFinal     bool       `json:"is_final"`
	}
	var results []liveEvent
	for rows.Next() {
		var e liveEvent
		if err := rows.Scan(&e.ID, &e.ExternalID, &e.Provider, &e.LeagueID,
			&e.HomeTeamID, &e.AwayTeamID, &e.Status, &e.ScheduledAt,
			&e.HomeScore, &e.AwayScore, &e.Period, &e.Clock, &e.IsFinal); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		results = append(results, e)
	}
	if results == nil {
		results = []liveEvent{}
	}
	writeJSON(w, http.StatusOK, results)
}

// ============================================================
// EPG endpoint (GET /sports/epg?date=YYYY-MM-DD)
// ============================================================

// EPG returns today's (or a specified date's) events in an EPG-compatible JSON format.
func (h *Handlers) EPG(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	var date time.Time
	if dateStr == "" {
		now := time.Now().UTC()
		date = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	} else {
		var err error
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid date, use YYYY-MM-DD"})
			return
		}
	}
	dayEnd := date.Add(24 * time.Hour)

	rows, err := h.pool.Query(r.Context(),
		`SELECT e.id, e.external_id, e.provider, e.status,
		        e.scheduled_at, e.home_score, e.away_score, e.is_final,
		        e.broadcast_network, e.broadcast_channel, e.venue_name, e.season,
		        ht.name AS home_team, at2.name AS away_team,
		        l.name AS league_name, l.sport
		 FROM np_sports_events e
		 LEFT JOIN np_sports_teams ht  ON ht.id = e.home_team_id
		 LEFT JOIN np_sports_teams at2 ON at2.id = e.away_team_id
		 LEFT JOIN np_sports_leagues l  ON l.id  = e.league_id
		 WHERE e.source_account_id=$1
		   AND e.scheduled_at >= $2
		   AND e.scheduled_at < $3
		   AND e.deleted_at IS NULL
		 ORDER BY e.scheduled_at ASC`, h.sa(r), date, dayEnd)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type epgEntry struct {
		ID               string     `json:"id"`
		ExternalID       string     `json:"external_id"`
		Provider         string     `json:"provider"`
		Status           string     `json:"status"`
		Start            time.Time  `json:"start"`
		HomeScore        *int       `json:"home_score"`
		AwayScore        *int       `json:"away_score"`
		IsFinal          bool       `json:"is_final"`
		BroadcastNetwork *string    `json:"broadcast_network"`
		BroadcastChannel *string    `json:"broadcast_channel"`
		Venue            *string    `json:"venue"`
		Season           *string    `json:"season"`
		HomeTeam         *string    `json:"home_team"`
		AwayTeam         *string    `json:"away_team"`
		League           *string    `json:"league"`
		Sport            *string    `json:"sport"`
		Title            string     `json:"title"`
	}

	var entries []epgEntry
	for rows.Next() {
		var e epgEntry
		if err := rows.Scan(
			&e.ID, &e.ExternalID, &e.Provider, &e.Status,
			&e.Start, &e.HomeScore, &e.AwayScore, &e.IsFinal,
			&e.BroadcastNetwork, &e.BroadcastChannel, &e.Venue, &e.Season,
			&e.HomeTeam, &e.AwayTeam,
			&e.League, &e.Sport,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		// Build human-readable title.
		home := "TBD"
		away := "TBD"
		if e.HomeTeam != nil {
			home = *e.HomeTeam
		}
		if e.AwayTeam != nil {
			away = *e.AwayTeam
		}
		e.Title = home + " vs " + away
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []epgEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"date":    date.Format("2006-01-02"),
		"count":   len(entries),
		"entries": entries,
	})
}
