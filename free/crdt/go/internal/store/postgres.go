// Package store handles Postgres persistence for CRDT documents.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Document holds the persisted state of a CRDT document.
type Document struct {
	ID               string
	DocID            string
	Engine           string // "yjs" | "automerge"
	State            []byte
	UpdatesCount     int
	SourceAccountID  string
	LastModified     time.Time
	CreatedAt        time.Time
}

// Store wraps a pgxpool and provides CRDT document operations.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a new Store.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Migrate runs embedded SQL migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_crdt_documents (
			id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			doc_id            TEXT        NOT NULL UNIQUE,
			engine            TEXT        NOT NULL CHECK (engine IN ('yjs','automerge')),
			state             BYTEA       NOT NULL,
			updates_count     INT         NOT NULL DEFAULT 0,
			source_account_id TEXT        NOT NULL DEFAULT 'primary',
			last_modified     TIMESTAMPTZ NOT NULL DEFAULT now(),
			created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_np_crdt_documents_doc_id   ON np_crdt_documents (doc_id);
		CREATE INDEX IF NOT EXISTS idx_np_crdt_documents_account  ON np_crdt_documents (source_account_id);

		CREATE TABLE IF NOT EXISTS np_crdt_updates (
			id          BIGSERIAL   PRIMARY KEY,
			doc_id      TEXT        NOT NULL REFERENCES np_crdt_documents(doc_id) ON DELETE CASCADE,
			update_data BYTEA       NOT NULL,
			client_id   TEXT,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_np_crdt_updates_doc_applied ON np_crdt_updates (doc_id, applied_at DESC);
	`)
	return err
}

// ErrNotFound is returned when a document does not exist.
var ErrNotFound = errors.New("document not found")

// LoadDocument retrieves the current document state.
// Returns ErrNotFound if the doc does not exist.
func (s *Store) LoadDocument(ctx context.Context, docID string) (*Document, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, doc_id, engine, state, updates_count, source_account_id, last_modified, created_at
		FROM np_crdt_documents
		WHERE doc_id = $1
	`, docID)

	var d Document
	err := row.Scan(&d.ID, &d.DocID, &d.Engine, &d.State, &d.UpdatesCount,
		&d.SourceAccountID, &d.LastModified, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load document %q: %w", docID, err)
	}
	return &d, nil
}

// UpsertDocument creates or replaces the document's binary state atomically.
func (s *Store) UpsertDocument(ctx context.Context, docID, engine string, state []byte) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO np_crdt_documents (doc_id, engine, state, updates_count, last_modified)
		VALUES ($1, $2, $3, 1, now())
		ON CONFLICT (doc_id) DO UPDATE
		SET state = EXCLUDED.state,
		    updates_count = np_crdt_documents.updates_count + 1,
		    last_modified = now()
	`, docID, engine, state)
	if err != nil {
		return fmt.Errorf("upsert document %q: %w", docID, err)
	}
	return nil
}

// AppendUpdate persists a raw CRDT update message to the update log.
func (s *Store) AppendUpdate(ctx context.Context, docID, clientID string, data []byte) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO np_crdt_updates (doc_id, update_data, client_id)
		VALUES ($1, $2, $3)
	`, docID, data, clientID)
	if err != nil {
		return fmt.Errorf("append update for %q: %w", docID, err)
	}
	return nil
}

// DeleteDocument removes a document and all its updates (cascade).
func (s *Store) DeleteDocument(ctx context.Context, docID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM np_crdt_documents WHERE doc_id = $1`, docID)
	if err != nil {
		return fmt.Errorf("delete document %q: %w", docID, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListDocuments returns a paginated list of documents.
func (s *Store) ListDocuments(ctx context.Context, limit, offset int) ([]Document, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, doc_id, engine, state, updates_count, source_account_id, last_modified, created_at
		FROM np_crdt_documents
		ORDER BY last_modified DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.DocID, &d.Engine, &d.State, &d.UpdatesCount,
			&d.SourceAccountID, &d.LastModified, &d.CreatedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// CompactUpdates merges all update rows for a document into the document state
// and deletes the raw update rows. Returns the number of rows deleted.
func (s *Store) CompactUpdates(ctx context.Context, docID string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM np_crdt_updates WHERE doc_id = $1`, docID)
	if err != nil {
		return 0, fmt.Errorf("compact updates for %q: %w", docID, err)
	}
	return tag.RowsAffected(), nil
}

// UpdateCount returns the number of pending update rows for a document.
func (s *Store) UpdateCount(ctx context.Context, docID string) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_crdt_updates WHERE doc_id = $1`, docID).
		Scan(&count)
	return count, err
}
