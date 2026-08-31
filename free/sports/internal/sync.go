package internal

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-sports/internal/provider"
)

// Syncer orchestrates data sync from a provider into Postgres.
type Syncer struct {
	pool     *pgxpool.Pool
	provider provider.Provider
	// sourceAccountID scopes all writes to a specific account.
	sourceAccountID string
}

// NewSyncer returns a Syncer for the given provider and account.
func NewSyncer(pool *pgxpool.Pool, p provider.Provider, accountID string) *Syncer {
	if accountID == "" {
		accountID = "primary"
	}
	return &Syncer{pool: pool, provider: p, sourceAccountID: accountID}
}

// SyncLeagues fetches all leagues for the given sport and upserts into np_sports_leagues.
// Returns the number of records synced and any error.
func (s *Syncer) SyncLeagues(ctx context.Context, sport provider.Sport) (int, error) {
	leagues, err := s.provider.ListLeagues(ctx, sport)
	if err != nil {
		return 0, fmt.Errorf("SyncLeagues(%s): fetch: %w", sport, err)
	}

	synced := 0
	for _, l := range leagues {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO np_sports_leagues
				(source_account_id, external_id, provider, name, sport, country)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (source_account_id, provider, external_id)
			DO UPDATE SET
				name        = EXCLUDED.name,
				country     = EXCLUDED.country,
				updated_at  = NOW(),
				synced_at   = NOW()
		`, s.sourceAccountID, l.ID, s.provider.Name(), l.Name, string(l.Sport), l.Country)
		if err != nil {
			log.Printf("SyncLeagues: upsert league %s: %v", l.ID, err)
			continue
		}
		synced++
	}
	return synced, nil
}

// SyncTeams fetches all teams for the given external leagueID and upserts into np_sports_teams.
func (s *Syncer) SyncTeams(ctx context.Context, leagueID string) (int, error) {
	teams, err := s.provider.ListTeams(ctx, leagueID)
	if err != nil {
		return 0, fmt.Errorf("SyncTeams(%s): fetch: %w", leagueID, err)
	}

	synced := 0
	for _, t := range teams {
		// Resolve internal league UUID by external_id.
		var leagueUUID *string
		var uid string
		err := s.pool.QueryRow(ctx,
			`SELECT id FROM np_sports_leagues WHERE external_id=$1 AND source_account_id=$2 AND provider=$3`,
			t.LeagueID, s.sourceAccountID, s.provider.Name(),
		).Scan(&uid)
		if err == nil {
			leagueUUID = &uid
		}

		_, err = s.pool.Exec(ctx, `
			INSERT INTO np_sports_teams
				(source_account_id, league_id, external_id, provider, name, logo_url)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (source_account_id, provider, external_id)
			DO UPDATE SET
				league_id   = COALESCE(EXCLUDED.league_id, np_sports_teams.league_id),
				name        = EXCLUDED.name,
				logo_url    = EXCLUDED.logo_url,
				updated_at  = NOW(),
				synced_at   = NOW()
		`, s.sourceAccountID, leagueUUID, t.ID, s.provider.Name(), t.Name, nullString(t.LogoURL))
		if err != nil {
			log.Printf("SyncTeams: upsert team %s: %v", t.ID, err)
			continue
		}
		synced++
	}
	return synced, nil
}

// SyncEvents fetches events for the given leagueID within [from, to] and upserts into np_sports_events.
func (s *Syncer) SyncEvents(ctx context.Context, leagueID string, from, to time.Time) (int, error) {
	events, err := s.provider.ListEvents(ctx, leagueID, from, to)
	if err != nil {
		return 0, fmt.Errorf("SyncEvents(%s): fetch: %w", leagueID, err)
	}
	return s.upsertEvents(ctx, events)
}

// SyncLiveScores iterates today's events for the given account, calls LiveScore for
// each in-progress event, and updates scores in np_sports_events.
func (s *Syncer) SyncLiveScores(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	rows, err := s.pool.Query(ctx, `
		SELECT external_id FROM np_sports_events
		WHERE source_account_id=$1
		  AND provider=$2
		  AND scheduled_at >= $3
		  AND scheduled_at < $4
		  AND is_final = FALSE
		  AND deleted_at IS NULL
	`, s.sourceAccountID, s.provider.Name(), dayStart, dayEnd)
	if err != nil {
		return 0, fmt.Errorf("SyncLiveScores: query events: %w", err)
	}
	defer rows.Close()

	var externalIDs []string
	for rows.Next() {
		var extID string
		if err := rows.Scan(&extID); err == nil {
			externalIDs = append(externalIDs, extID)
		}
	}

	updated := 0
	for _, extID := range externalIDs {
		ev, err := s.provider.LiveScore(ctx, extID)
		if err != nil {
			log.Printf("SyncLiveScores: LiveScore(%s): %v", extID, err)
			continue
		}
		isFinal := ev.Status == "FT" || ev.Status == "AET" || ev.Status == "PEN" || ev.Status == "final"
		_, err = s.pool.Exec(ctx, `
			UPDATE np_sports_events
			SET home_score = $1,
			    away_score = $2,
			    status     = $3,
			    is_final   = $4,
			    updated_at = NOW(),
			    synced_at  = NOW()
			WHERE external_id=$5 AND source_account_id=$6 AND provider=$7
		`, ev.HomeScore, ev.AwayScore, ev.Status, isFinal,
			extID, s.sourceAccountID, s.provider.Name())
		if err != nil {
			log.Printf("SyncLiveScores: update event %s: %v", extID, err)
			continue
		}
		updated++
	}
	return updated, nil
}

// upsertEvents is the shared helper used by SyncEvents.
func (s *Syncer) upsertEvents(ctx context.Context, events []provider.Event) (int, error) {
	synced := 0
	for _, e := range events {
		// Resolve home/away team UUIDs.
		homeUUID := s.resolveTeamUUID(ctx, e.HomeTeamID)
		awayUUID := s.resolveTeamUUID(ctx, e.AwayTeamID)
		leagueUUID := s.resolveLeagueUUID(ctx, e.LeagueID)

		_, err := s.pool.Exec(ctx, `
			INSERT INTO np_sports_events
				(source_account_id, external_id, provider, league_id,
				 home_team_id, away_team_id, event_type, status,
				 scheduled_at, venue_name, broadcast_channel, season, week,
				 home_score, away_score)
			VALUES ($1,$2,$3,$4,$5,$6,'regular',$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (source_account_id, provider, external_id)
			DO UPDATE SET
				status            = EXCLUDED.status,
				home_score        = EXCLUDED.home_score,
				away_score        = EXCLUDED.away_score,
				venue_name        = EXCLUDED.venue_name,
				broadcast_channel = EXCLUDED.broadcast_channel,
				season            = EXCLUDED.season,
				week              = EXCLUDED.week,
				updated_at        = NOW(),
				synced_at         = NOW()
		`, s.sourceAccountID, e.ID, s.provider.Name(), leagueUUID,
			homeUUID, awayUUID, e.Status,
			e.StartTime, nullString(e.VenueName), nullString(e.BroadcastCh),
			nullString(e.Season), nullInt(e.Week),
			e.HomeScore, e.AwayScore)
		if err != nil {
			log.Printf("upsertEvents: upsert event %s: %v", e.ID, err)
			continue
		}
		synced++
	}
	return synced, nil
}

func (s *Syncer) resolveTeamUUID(ctx context.Context, externalID string) *string {
	if externalID == "" {
		return nil
	}
	var uid string
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM np_sports_teams WHERE external_id=$1 AND source_account_id=$2 AND provider=$3`,
		externalID, s.sourceAccountID, s.provider.Name(),
	).Scan(&uid); err != nil {
		return nil
	}
	return &uid
}

func (s *Syncer) resolveLeagueUUID(ctx context.Context, externalID string) *string {
	if externalID == "" {
		return nil
	}
	var uid string
	if err := s.pool.QueryRow(ctx,
		`SELECT id FROM np_sports_leagues WHERE external_id=$1 AND source_account_id=$2 AND provider=$3`,
		externalID, s.sourceAccountID, s.provider.Name(),
	).Scan(&uid); err != nil {
		return nil
	}
	return &uid
}

// nullString returns a pointer suitable for a nullable text column.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nullInt returns a pointer suitable for a nullable integer column.
func nullInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
