// Package logs handles log persistence and SSE streaming for edge functions.
package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nself-org/nself-functions-v8/internal/env"
)

// LogEntry represents one log line stored in np_function_logs.
type LogEntry struct {
	ID         uuid.UUID
	FunctionID uuid.UUID
	Level      string
	Message    string
	CreatedAt  time.Time
}

// Logger persists and streams function log output.
type Logger struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// New creates a new Logger.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Logger {
	return &Logger{pool: pool, logger: logger}
}

// Write persists log lines for a function invocation.
// Lines are redacted before storage.
func (l *Logger) Write(ctx context.Context, fnID uuid.UUID, level string, lines []string) {
	for _, line := range lines {
		safe := env.RedactSecrets(line)
		if _, err := l.pool.Exec(ctx, `
			INSERT INTO np_function_logs (function_id, level, message)
			VALUES ($1, $2, $3)
		`, fnID, level, safe); err != nil {
			l.logger.Warn("failed to write log entry", "err", err)
		}
	}
}

// ListRecent returns the most recent log entries for a function.
func (l *Logger) ListRecent(ctx context.Context, fnID uuid.UUID, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := l.pool.Query(ctx, `
		SELECT id, function_id, level, message, created_at
		FROM np_function_logs
		WHERE function_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, fnID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.FunctionID, &e.Level, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// StreamSSE writes an SSE stream of live log entries for a function.
// It polls the DB every 500ms. The client closes the connection to stop.
func (l *Logger) StreamSSE(ctx context.Context, w http.ResponseWriter, fnID uuid.UUID) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Track the latest ID sent.
	var lastID uuid.UUID
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, err := l.pool.Query(ctx, `
				SELECT id, level, message, created_at
				FROM np_function_logs
				WHERE function_id = $1
				  AND ($2::uuid IS NULL OR id > $2)
				ORDER BY created_at ASC, id ASC
				LIMIT 50
			`, fnID, nullUUID(lastID))
			if err != nil {
				l.logger.Warn("sse poll error", "err", err)
				continue
			}

			type event struct {
				ID        uuid.UUID `json:"id"`
				Level     string    `json:"level"`
				Message   string    `json:"message"`
				CreatedAt time.Time `json:"created_at"`
			}

			var events []event
			for rows.Next() {
				var e event
				if err := rows.Scan(&e.ID, &e.Level, &e.Message, &e.CreatedAt); err == nil {
					events = append(events, e)
				}
			}
			rows.Close()

			for _, e := range events {
				data, _ := json.Marshal(e)
				fmt.Fprintf(w, "id: %s\ndata: %s\n\n", e.ID, data)
				lastID = e.ID
			}
			if len(events) > 0 {
				flusher.Flush()
			}
		}
	}
}

// nullUUID returns nil for zero UUID (for SQL $2::uuid IS NULL check).
func nullUUID(id uuid.UUID) interface{} {
	if id == uuid.Nil {
		return nil
	}
	return id
}
