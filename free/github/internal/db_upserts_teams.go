package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func UpsertTeam(ctx context.Context, pool *pgxpool.Pool, t *Team) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_teams (
			id, source_account_id, node_id, org_login, name, slug, description,
			privacy, permission, parent_id, members_count, repos_count, html_url,
			created_at, updated_at, synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			org_login = EXCLUDED.org_login,
			name = EXCLUDED.name,
			slug = EXCLUDED.slug,
			description = EXCLUDED.description,
			privacy = EXCLUDED.privacy,
			permission = EXCLUDED.permission,
			parent_id = EXCLUDED.parent_id,
			members_count = EXCLUDED.members_count,
			repos_count = EXCLUDED.repos_count,
			html_url = EXCLUDED.html_url,
			updated_at = EXCLUDED.updated_at,
			synced_at = NOW()
	`, t.ID, t.SourceAccountID, t.NodeID, t.OrgLogin, t.Name, t.Slug,
		t.Description, t.Privacy, t.Permission, t.ParentID,
		t.MembersCount, t.ReposCount, t.HTMLURL, t.CreatedAt, t.UpdatedAt)
	return err
}

// UpsertCollaborator inserts or updates a collaborator record.
func UpsertCollaborator(ctx context.Context, pool *pgxpool.Pool, c *Collaborator) error {
	permissions := defaultJSONB(c.Permissions)
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_collaborators (
			id, source_account_id, repo_id, login, type, site_admin,
			permissions, role_name, synced_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (id) DO UPDATE SET
			source_account_id = EXCLUDED.source_account_id,
			repo_id = EXCLUDED.repo_id,
			login = EXCLUDED.login,
			type = EXCLUDED.type,
			site_admin = EXCLUDED.site_admin,
			permissions = EXCLUDED.permissions,
			role_name = EXCLUDED.role_name,
			synced_at = NOW()
	`, c.ID, c.SourceAccountID, c.RepoID, c.Login, c.Type,
		c.SiteAdmin, permissions, c.RoleName)
	return err
}

// InsertWebhookEvent inserts a webhook event record.
func InsertWebhookEvent(ctx context.Context, pool *pgxpool.Pool, e *WebhookEvent) error {
	data := defaultJSONB(e.Data)
	_, err := pool.Exec(ctx, `
		INSERT INTO np_github_webhook_events (
			id, source_account_id, event, action, repo_id, repo_full_name,
			sender_login, data, processed, processed_at, error, received_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, COALESCE($12, NOW()))
		ON CONFLICT (id) DO NOTHING
	`, e.ID, e.SourceAccountID, e.Event, e.Action, e.RepoID, e.RepoFullName,
		e.SenderLogin, data, e.Processed, e.ProcessedAt, e.Error, e.ReceivedAt)
	return err
}

// --- Query functions ---------------------------------------------------------

// ListRepositories returns repositories with pagination.
