package sdk

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectDB connects to PostgreSQL with retry logic (3 attempts, 2s backoff).
// Returns a connection pool ready for queries.
func ConnectDB(databaseURL string) (*pgxpool.Pool, error) {
	const (
		maxAttempts = 3
		backoff     = 2 * time.Second
	)

	var pool *pgxpool.Pool
	var err error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pool, err = pgxpool.New(ctx, databaseURL)
		if err != nil {
			cancel()
			log.Printf("plugin-sdk: db connect attempt %d/%d failed: %v", attempt, maxAttempts, err)
			if attempt < maxAttempts {
				time.Sleep(backoff)
			}
			continue
		}

		// Verify the connection is usable.
		err = pool.Ping(ctx)
		cancel()
		if err != nil {
			pool.Close()
			log.Printf("plugin-sdk: db ping attempt %d/%d failed: %v", attempt, maxAttempts, err)
			if attempt < maxAttempts {
				time.Sleep(backoff)
			}
			continue
		}

		log.Printf("plugin-sdk: connected to database")
		return pool, nil
	}

	return nil, fmt.Errorf("plugin-sdk: failed to connect after %d attempts: %w", maxAttempts, err)
}
