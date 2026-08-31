package internal

// dbPool is an interface satisfied by the concrete pool type so handlers
// can be tested without a real database. The concrete implementation is
// pgPool (wrapping pgxpool.Pool) defined below.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// dbPool defines the data-access operations used by handlers.
type dbPool interface {
	ping() error
	upsertToken(ctx context.Context, sourceID, accessToken string, expiresAt time.Time, linkedInID, name string, email *string) error
	loadToken(ctx context.Context, sourceID string) (*Token, error)
	deleteToken(ctx context.Context, sourceID string) error
	insertPost(ctx context.Context, sourceID string, linkedInPostID *string, text string, imageURL *string, visibility, status string, errMsg *string) (id string, publishedAt time.Time, err error)
	listPosts(ctx context.Context, sourceID string) ([]PostRecord, error)
}

// pgPool wraps pgxpool.Pool and implements dbPool.
type pgPool struct {
	p *pgxpool.Pool
}

// NewPgPool wraps a *pgxpool.Pool as a dbPool.
func NewPgPool(p *pgxpool.Pool) dbPool {
	return &pgPool{p: p}
}

func (pg *pgPool) ping() error {
	return pg.p.Ping(context.Background())
}

func (pg *pgPool) upsertToken(ctx context.Context, sourceID, accessToken string, expiresAt time.Time, linkedInID, name string, email *string) error {
	_, err := pg.p.Exec(ctx, `
		INSERT INTO np_linkedin_tokens
		    (source_account_id, access_token, expires_at, linkedin_id, name, email)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (source_account_id) DO UPDATE SET
		    access_token = EXCLUDED.access_token,
		    expires_at   = EXCLUDED.expires_at,
		    linkedin_id  = EXCLUDED.linkedin_id,
		    name         = EXCLUDED.name,
		    email        = EXCLUDED.email,
		    updated_at   = NOW()
	`, sourceID, accessToken, expiresAt, linkedInID, name, email)
	return err
}

func (pg *pgPool) loadToken(ctx context.Context, sourceID string) (*Token, error) {
	var tok Token
	err := pg.p.QueryRow(ctx, `
		SELECT id, source_account_id, access_token, expires_at, linkedin_id, name, email, created_at, updated_at
		FROM np_linkedin_tokens
		WHERE source_account_id = $1
	`, sourceID).Scan(
		&tok.ID, &tok.SourceAccountID, &tok.AccessToken, &tok.ExpiresAt,
		&tok.LinkedInID, &tok.Name, &tok.Email, &tok.CreatedAt, &tok.UpdatedAt,
	)
	if err != nil {
		// pgx returns pgx.ErrNoRows; treat as not-found.
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("load token: %w", err)
	}
	return &tok, nil
}

func (pg *pgPool) deleteToken(ctx context.Context, sourceID string) error {
	_, err := pg.p.Exec(ctx, `DELETE FROM np_linkedin_tokens WHERE source_account_id = $1`, sourceID)
	return err
}

func (pg *pgPool) insertPost(ctx context.Context, sourceID string, linkedInPostID *string, text string, imageURL *string, visibility, status string, errMsg *string) (string, time.Time, error) {
	var id string
	var publishedAt time.Time
	err := pg.p.QueryRow(ctx, `
		INSERT INTO np_linkedin_posts
		    (source_account_id, linkedin_post_id, text, image_url, visibility, status, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, published_at
	`, sourceID, linkedInPostID, text, imageURL, visibility, status, errMsg,
	).Scan(&id, &publishedAt)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("insert post: %w", err)
	}
	return id, publishedAt, nil
}

func (pg *pgPool) listPosts(ctx context.Context, sourceID string) ([]PostRecord, error) {
	rows, err := pg.p.Query(ctx, `
		SELECT id, linkedin_post_id, text, image_url, visibility, status, error_message, published_at
		FROM np_linkedin_posts
		WHERE source_account_id = $1
		ORDER BY published_at DESC
		LIMIT 100
	`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()

	posts := make([]PostRecord, 0)
	for rows.Next() {
		var p PostRecord
		if err := rows.Scan(
			&p.ID, &p.LinkedInPostID, &p.Text, &p.ImageURL,
			&p.Visibility, &p.Status, &p.ErrorMessage, &p.PublishedAt,
		); err != nil {
			return nil, fmt.Errorf("scan post: %w", err)
		}
		posts = append(posts, p)
	}
	return posts, nil
}
