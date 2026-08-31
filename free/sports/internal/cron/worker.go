// Package cron provides a lightweight periodic sync worker for the sports plugin.
// It registers with the nSelf cron infrastructure (or runs standalone) and
// schedules live-score polling every 60 seconds during configured game hours.
package cron

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-sports/internal"
	"github.com/nself-org/nself-sports/internal/provider"
)

// Worker runs periodic sync jobs.
type Worker struct {
	pool     *pgxpool.Pool
	prov     provider.Provider
	accountID string

	// liveInterval controls how often live scores are refreshed.
	liveInterval time.Duration
	// syncInterval controls how often the full league/event sync runs.
	syncInterval time.Duration

	// Game hours gate: only poll live scores between gameHourStart and gameHourEnd (UTC).
	gameHourStart int
	gameHourEnd   int
}

// New creates a Worker. Intervals and game hours are read from env:
//
//	SPORTS_LIVE_POLL_INTERVAL   — seconds between live-score polls (default 60)
//	SPORTS_SYNC_INTERVAL        — seconds between full syncs (default 1800 / 30 min)
//	SPORTS_GAME_HOUR_START      — UTC hour when game window opens (default 10)
//	SPORTS_GAME_HOUR_END        — UTC hour when game window closes (default 4 next day, stored as 28)
func New(pool *pgxpool.Pool, prov provider.Provider, accountID string) *Worker {
	if accountID == "" {
		accountID = "primary"
	}
	return &Worker{
		pool:          pool,
		prov:          prov,
		accountID:     accountID,
		liveInterval:  parseDuration("SPORTS_LIVE_POLL_INTERVAL", 60),
		syncInterval:  parseDuration("SPORTS_SYNC_INTERVAL", 1800),
		gameHourStart: parseInt("SPORTS_GAME_HOUR_START", 10),
		gameHourEnd:   parseInt("SPORTS_GAME_HOUR_END", 28), // 28 = 04:00 next day
	}
}

// Start runs the worker until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	log.Printf("sports cron worker started (live=%s, full=%s)", w.liveInterval, w.syncInterval)

	liveTicker := time.NewTicker(w.liveInterval)
	syncTicker := time.NewTicker(w.syncInterval)
	defer liveTicker.Stop()
	defer syncTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("sports cron worker stopped")
			return

		case <-liveTicker.C:
			if w.inGameWindow() {
				go w.runLiveScores(ctx)
			}

		case <-syncTicker.C:
			go w.runFullSync(ctx)
		}
	}
}

func (w *Worker) inGameWindow() bool {
	hour := time.Now().UTC().Hour()
	end := w.gameHourEnd
	if end > 23 {
		// wraps midnight
		return hour >= w.gameHourStart || hour < (end-24)
	}
	return hour >= w.gameHourStart && hour < end
}

func (w *Worker) runLiveScores(ctx context.Context) {
	s := internal.NewSyncer(w.pool, w.prov, w.accountID)
	n, err := s.SyncLiveScores(ctx)
	if err != nil {
		log.Printf("sports cron: SyncLiveScores error: %v", err)
		return
	}
	if n > 0 {
		log.Printf("sports cron: updated %d live score(s)", n)
	}
}

func (w *Worker) runFullSync(ctx context.Context) {
	s := internal.NewSyncer(w.pool, w.prov, w.accountID)

	sports := []provider.Sport{
		provider.SportSoccer,
		provider.SportFootball,
		provider.SportBasketball,
		provider.SportBaseball,
		provider.SportHockey,
	}

	for _, sport := range sports {
		n, err := s.SyncLeagues(ctx, sport)
		if err != nil {
			log.Printf("sports cron: SyncLeagues(%s): %v", sport, err)
			continue
		}
		log.Printf("sports cron: synced %d leagues for %s", n, sport)
	}

	// Sync events for next 7 days across all known leagues.
	now := time.Now().UTC()
	from := now
	to := now.AddDate(0, 0, 7)

	rows, err := w.pool.Query(ctx,
		`SELECT external_id FROM np_sports_leagues WHERE source_account_id=$1 AND provider=$2`,
		w.accountID, w.prov.Name())
	if err != nil {
		log.Printf("sports cron: query leagues: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var extID string
		if err := rows.Scan(&extID); err != nil {
			continue
		}
		n, err := s.SyncEvents(ctx, extID, from, to)
		if err != nil {
			log.Printf("sports cron: SyncEvents(%s): %v", extID, err)
			continue
		}
		if n > 0 {
			log.Printf("sports cron: synced %d events for league %s", n, extID)
		}
	}
}

func parseDuration(key string, defaultSec int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return time.Duration(defaultSec) * time.Second
}

func parseInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
