package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewDB creates a connection pool and verifies connectivity.
func NewDB(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging db: %w", err)
	}
	return pool, nil
}

// InitSchema creates all documents plugin tables if they do not exist.
func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS docs_documents (
			id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			owner_id          VARCHAR(255) NOT NULL,
			title             VARCHAR(500) NOT NULL,
			description       TEXT,
			doc_type          VARCHAR(50)  NOT NULL,
			category          VARCHAR(100),
			tags              TEXT[]       DEFAULT '{}',
			template_id       UUID,
			file_url          TEXT,
			file_size_bytes   BIGINT,
			mime_type         VARCHAR(100),
			version           INTEGER      DEFAULT 1,
			status            VARCHAR(20)  DEFAULT 'draft',
			generated_from    JSONB,
			metadata          JSONB        DEFAULT '{}',
			search_vector     tsvector,
			created_at        TIMESTAMPTZ  DEFAULT NOW(),
			updated_at        TIMESTAMPTZ  DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_docs_documents_source_app
			ON docs_documents(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_docs_documents_owner
			ON docs_documents(source_account_id, owner_id);
		CREATE INDEX IF NOT EXISTS idx_docs_documents_type
			ON docs_documents(source_account_id, doc_type);
		CREATE INDEX IF NOT EXISTS idx_docs_documents_category
			ON docs_documents(source_account_id, category);
		CREATE INDEX IF NOT EXISTS idx_docs_documents_search
			ON docs_documents USING GIN(search_vector);

		CREATE TABLE IF NOT EXISTS docs_templates (
			id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			name              VARCHAR(255) NOT NULL,
			description       TEXT,
			doc_type          VARCHAR(50)  NOT NULL,
			output_format     VARCHAR(20)  DEFAULT 'pdf',
			template_engine   VARCHAR(20)  DEFAULT 'handlebars',
			template_content  TEXT         NOT NULL,
			css_content       TEXT,
			header_content    TEXT,
			footer_content    TEXT,
			variables         JSONB        DEFAULT '{}',
			sample_data       JSONB        DEFAULT '{}',
			is_default        BOOLEAN      DEFAULT false,
			version           INTEGER      DEFAULT 1,
			created_at        TIMESTAMPTZ  DEFAULT NOW(),
			updated_at        TIMESTAMPTZ  DEFAULT NOW(),
			UNIQUE(source_account_id, name, version)
		);

		CREATE INDEX IF NOT EXISTS idx_docs_templates_source_app
			ON docs_templates(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_docs_templates_type
			ON docs_templates(source_account_id, doc_type);

		CREATE TABLE IF NOT EXISTS docs_versions (
			id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			document_id       UUID         NOT NULL REFERENCES docs_documents(id) ON DELETE CASCADE,
			version           INTEGER      NOT NULL,
			file_url          TEXT         NOT NULL,
			file_size_bytes   BIGINT,
			change_summary    TEXT,
			created_by        VARCHAR(255),
			created_at        TIMESTAMPTZ  DEFAULT NOW(),
			UNIQUE(source_account_id, document_id, version)
		);

		CREATE INDEX IF NOT EXISTS idx_docs_versions_source_app
			ON docs_versions(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_docs_versions_document
			ON docs_versions(document_id, version DESC);

		CREATE TABLE IF NOT EXISTS docs_shares (
			id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
			source_account_id    VARCHAR(128) NOT NULL DEFAULT 'primary',
			document_id          UUID         NOT NULL REFERENCES docs_documents(id) ON DELETE CASCADE,
			shared_with_user_id  VARCHAR(255),
			shared_with_email    VARCHAR(255),
			share_token          VARCHAR(255),
			permission           VARCHAR(20)  DEFAULT 'view',
			expires_at           TIMESTAMPTZ,
			accessed_at          TIMESTAMPTZ,
			created_at           TIMESTAMPTZ  DEFAULT NOW(),
			UNIQUE(source_account_id, document_id, share_token)
		);

		CREATE INDEX IF NOT EXISTS idx_docs_shares_source_app
			ON docs_shares(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_docs_shares_document
			ON docs_shares(document_id);
		CREATE INDEX IF NOT EXISTS idx_docs_shares_user
			ON docs_shares(source_account_id, shared_with_user_id);
		CREATE INDEX IF NOT EXISTS idx_docs_shares_token
			ON docs_shares(share_token);

		CREATE TABLE IF NOT EXISTS docs_webhook_events (
			id                VARCHAR(255) PRIMARY KEY,
			source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
			event_type        VARCHAR(128) NOT NULL,
			payload           JSONB        NOT NULL,
			processed         BOOLEAN      DEFAULT false,
			processed_at      TIMESTAMPTZ,
			error             TEXT,
			retry_count       INTEGER      DEFAULT 0,
			created_at        TIMESTAMPTZ  DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_docs_webhook_events_source_app
			ON docs_webhook_events(source_account_id);
	`)
	return err
}
