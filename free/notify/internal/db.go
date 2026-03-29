package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Notification represents a row in np_notify_notifications.
type Notification struct {
	ID        string     `json:"id"`
	Channel   string     `json:"channel"`
	Recipient string     `json:"recipient"`
	Subject   string     `json:"subject"`
	Body      string     `json:"body"`
	Status    string     `json:"status"`
	SentAt    *time.Time `json:"sent_at"`
	Error     *string    `json:"error"`
	CreatedAt time.Time  `json:"created_at"`
}

// Template represents a row in np_notify_templates.
type Template struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Channel         string    `json:"channel"`
	SubjectTemplate string    `json:"subject_template"`
	BodyTemplate    string    `json:"body_template"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Migrate creates the required tables if they do not exist.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_notify_notifications (
			id         TEXT PRIMARY KEY,
			channel    TEXT NOT NULL,
			recipient  TEXT NOT NULL,
			subject    TEXT NOT NULL DEFAULT '',
			body       TEXT NOT NULL DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'pending',
			sent_at    TIMESTAMPTZ,
			error      TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_notify_notifications_status
			ON np_notify_notifications (status);

		CREATE INDEX IF NOT EXISTS idx_np_notify_notifications_channel
			ON np_notify_notifications (channel);

		CREATE TABLE IF NOT EXISTS np_notify_templates (
			id               TEXT PRIMARY KEY,
			name             TEXT NOT NULL UNIQUE,
			channel          TEXT NOT NULL,
			subject_template TEXT NOT NULL DEFAULT '',
			body_template    TEXT NOT NULL DEFAULT '',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_notify_templates_name
			ON np_notify_templates (name);
	`)
	return err
}

// InsertNotification inserts a notification record and returns its ID.
func InsertNotification(ctx context.Context, pool *pgxpool.Pool, id, channel, recipient, subject, body, status string, sentAt *time.Time, errMsg *string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_notify_notifications (id, channel, recipient, subject, body, status, sent_at, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, channel, recipient, subject, body, status, sentAt, errMsg)
	return err
}

// ListNotifications returns notifications with optional filtering and pagination.
func ListNotifications(ctx context.Context, pool *pgxpool.Pool, channel, status string, limit, offset int) ([]Notification, error) {
	query := `SELECT id, channel, recipient, subject, body, status, sent_at, error, created_at
		FROM np_notify_notifications WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if channel != "" {
		query += fmt.Sprintf(" AND channel = $%d", argIdx)
		args = append(args, channel)
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Channel, &n.Recipient, &n.Subject, &n.Body, &n.Status, &n.SentAt, &n.Error, &n.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, n)
	}
	return results, rows.Err()
}

// GetNotification returns a single notification by ID.
func GetNotification(ctx context.Context, pool *pgxpool.Pool, id string) (*Notification, error) {
	var n Notification
	err := pool.QueryRow(ctx, `
		SELECT id, channel, recipient, subject, body, status, sent_at, error, created_at
		FROM np_notify_notifications WHERE id = $1
	`, id).Scan(&n.ID, &n.Channel, &n.Recipient, &n.Subject, &n.Body, &n.Status, &n.SentAt, &n.Error, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// InsertTemplate creates a new notification template.
func InsertTemplate(ctx context.Context, pool *pgxpool.Pool, id, name, channel, subjectTpl, bodyTpl string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_notify_templates (id, name, channel, subject_template, body_template)
		VALUES ($1, $2, $3, $4, $5)
	`, id, name, channel, subjectTpl, bodyTpl)
	return err
}

// ListTemplates returns all notification templates.
func ListTemplates(ctx context.Context, pool *pgxpool.Pool) ([]Template, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, channel, subject_template, body_template, created_at, updated_at
		FROM np_notify_templates ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Name, &t.Channel, &t.SubjectTemplate, &t.BodyTemplate, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return results, rows.Err()
}
