package controller

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// lastActiveCache caches the last-active timestamps queried from pg_stat_activity
// per role name. Entries are refreshed every 60 seconds.
var lastActiveCache struct {
	mu      sync.RWMutex
	entries map[string]lastActiveCacheEntry
}

type lastActiveCacheEntry struct {
	lastActiveAt time.Time
	fetchedAt    time.Time
}

const lastActiveCacheTTL = 60 * time.Second

func init() {
	lastActiveCache.entries = make(map[string]lastActiveCacheEntry)
}

// queryLastActiveAt returns the most recent state_change timestamp from
// pg_stat_activity for the given role, using a 60-second in-memory cache.
// Falls back to time.Now() with a WARN log if the query fails.
func queryLastActiveAt(ctx context.Context, pool *pgxpool.Pool, roleName string) time.Time {
	// Fast path: check cache under read lock.
	lastActiveCache.mu.RLock()
	entry, ok := lastActiveCache.entries[roleName]
	lastActiveCache.mu.RUnlock()
	if ok && time.Since(entry.fetchedAt) < lastActiveCacheTTL {
		return entry.lastActiveAt
	}

	// Cache miss or stale: query pg_stat_activity.state_change.
	var lastActive time.Time
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(state_change), 'epoch'::timestamptz)
		FROM pg_stat_activity
		WHERE usename = $1
		  AND state_change IS NOT NULL
	`, roleName).Scan(&lastActive)
	if err != nil {
		log.Printf("WARN: controller.queryLastActiveAt: failed to query pg_stat_activity for role %q: %v — using time.Now() as fallback", roleName, err)
		return time.Now()
	}

	// If no rows exist yet (epoch returned), fall back to time.Now().
	if lastActive.IsZero() || lastActive.Unix() <= 0 {
		lastActive = time.Now()
	}

	// Store in cache under write lock.
	lastActiveCache.mu.Lock()
	lastActiveCache.entries[roleName] = lastActiveCacheEntry{
		lastActiveAt: lastActive,
		fetchedAt:    time.Now(),
	}
	lastActiveCache.mu.Unlock()

	return lastActive
}

// ListProjects returns all non-deleted projects.
func ListProjects(ctx context.Context, pool *pgxpool.Pool) ([]Project, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, slug, domain, schema_name, role_name, hasura_source, hasura_port,
		       status, created_at, updated_at, deleted_at
		FROM nself_controller.projects
		WHERE deleted_at IS NULL
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(
			&p.ID, &p.Slug, &p.Domain, &p.SchemaName, &p.RoleName, &p.HasuraSource, &p.HasuraPort,
			&p.Status, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// ProjectStatus returns a health + resource summary for a single project.
func ProjectStatus(ctx context.Context, pool *pgxpool.Pool, slug string) (*StatusReport, error) {
	proj, err := GetProject(ctx, pool, slug)
	if err != nil {
		return nil, err
	}

	// Connection count from pg_stat_activity.
	var connCount int
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_stat_activity
		WHERE usename = $1 AND state != 'idle'
	`, proj.RoleName).Scan(&connCount)

	// Approximate schema size.
	var schemaBytes int64
	_ = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(pg_total_relation_size(c.oid)), 0)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1
	`, proj.SchemaName).Scan(&schemaBytes)

	return &StatusReport{
		Project:        *proj,
		ConnectionCount: connCount,
		SchemaBytes:    schemaBytes,
		LastActiveAt:   queryLastActiveAt(ctx, pool, proj.RoleName),
		Healthy:        proj.Status == "active",
	}, nil
}

// AllProjectsStatus returns status for every active project.
// Used by `nself controller status`.
func AllProjectsStatus(ctx context.Context, pool *pgxpool.Pool, maxProjects int) (*ControllerStatus, error) {
	projects, err := ListProjects(ctx, pool)
	if err != nil {
		return nil, err
	}

	reports := make([]StatusReport, 0, len(projects))
	for _, p := range projects {
		sr, err := ProjectStatus(ctx, pool, p.Slug)
		if err != nil {
			// Non-fatal — include with Healthy=false.
			reports = append(reports, StatusReport{Project: p, Healthy: false})
			continue
		}
		reports = append(reports, *sr)
	}

	activeCount := 0
	for _, r := range reports {
		if r.Project.Status == "active" {
			activeCount++
		}
	}

	return &ControllerStatus{
		Projects:       reports,
		TotalActive:    activeCount,
		MaxProjects:    maxProjects,
		UtilizationPct: float64(activeCount) / float64(maxProjects) * 100,
	}, nil
}
