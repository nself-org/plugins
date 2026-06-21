package internal

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncService orchestrates syncing data from GitHub to the local database.
type SyncService struct {
	pool   *pgxpool.Pool
	client *GitHubClient
	config *Config
	acctID string
}

// NewSyncService creates a new sync service.
func NewSyncService(pool *pgxpool.Pool, client *GitHubClient, config *Config, accountID string) *SyncService {
	if accountID == "" {
		accountID = "primary"
	}
	return &SyncService{
		pool:   pool,
		client: client,
		config: config,
		acctID: accountID,
	}
}

// SyncAll syncs all resources: orgs, repos, and per-repo resources.
func (s *SyncService) SyncAll(ctx context.Context) *SyncResult {
	start := time.Now()
	result := &SyncResult{Success: true}

	// Sync organizations
	if s.config.Org != "" {
		if err := s.syncOrganizations(ctx, result); err != nil {
			result.Errors = append(result.Errors, "orgs: "+err.Error())
		}
	}

	// Sync repositories
	repos, err := s.syncRepositories(ctx, result)
	if err != nil {
		result.Errors = append(result.Errors, "repos: "+err.Error())
		result.Success = len(result.Errors) == 0
		result.Duration = time.Since(start).Milliseconds()
		return result
	}

	// Sync per-repo resources
	for _, repo := range repos {
		parts := strings.SplitN(repo.FullName, "/", 2)
		if len(parts) != 2 {
			continue
		}
		owner, name := parts[0], parts[1]

		s.syncBranches(ctx, owner, name, repo.ID, result)
		s.syncIssues(ctx, owner, name, repo.ID, result)
		s.syncPullRequests(ctx, owner, name, repo.ID, result)
		s.syncCommits(ctx, owner, name, repo.ID, result)
		s.syncReleases(ctx, owner, name, repo.ID, result)
		s.syncWorkflows(ctx, owner, name, repo.ID, result)
		s.syncWorkflowRuns(ctx, owner, name, repo.ID, result)
		s.syncDeployments(ctx, owner, name, repo.ID, result)
		s.syncCollaborators(ctx, owner, name, repo.ID, result)
	}

	// Sync teams (org-level)
	if s.config.Org != "" {
		s.syncTeams(ctx, s.config.Org, result)
	}

	// Get final stats
	stats, err := GetSyncStats(ctx, s.pool)
	if err == nil {
		result.Stats = *stats
	}

	result.Success = len(result.Errors) == 0
	result.Duration = time.Since(start).Milliseconds()
	return result
}

// SyncResource syncs a specific resource type.
func (s *SyncService) SyncResource(ctx context.Context, resource string) *SyncResult {
	start := time.Now()
	result := &SyncResult{Success: true}

	switch resource {
	case "organizations", "orgs":
		if err := s.syncOrganizations(ctx, result); err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
	case "repositories", "repos":
		if _, err := s.syncRepositories(ctx, result); err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
	case "teams":
		if s.config.Org != "" {
			s.syncTeams(ctx, s.config.Org, result)
		}
	default:
		// For per-repo resources, sync across all repos
		repos, err := s.getTrackedRepos(ctx)
		if err != nil {
			result.Errors = append(result.Errors, "getting repos: "+err.Error())
		} else {
			for _, repo := range repos {
				parts := strings.SplitN(repo.FullName, "/", 2)
				if len(parts) != 2 {
					continue
				}
				owner, name := parts[0], parts[1]

				switch resource {
				case "branches":
					s.syncBranches(ctx, owner, name, repo.ID, result)
				case "issues":
					s.syncIssues(ctx, owner, name, repo.ID, result)
				case "pulls", "pull_requests":
					s.syncPullRequests(ctx, owner, name, repo.ID, result)
				case "commits":
					s.syncCommits(ctx, owner, name, repo.ID, result)
				case "releases":
					s.syncReleases(ctx, owner, name, repo.ID, result)
				case "workflows":
					s.syncWorkflows(ctx, owner, name, repo.ID, result)
				case "workflow_runs", "runs":
					s.syncWorkflowRuns(ctx, owner, name, repo.ID, result)
				case "deployments":
					s.syncDeployments(ctx, owner, name, repo.ID, result)
				case "collaborators":
					s.syncCollaborators(ctx, owner, name, repo.ID, result)
				default:
					result.Errors = append(result.Errors, "unknown resource: "+resource)
				}
			}
		}
	}

	stats, err := GetSyncStats(ctx, s.pool)
	if err == nil {
		result.Stats = *stats
	}

	result.Success = len(result.Errors) == 0
	result.Duration = time.Since(start).Milliseconds()
	return result
}

