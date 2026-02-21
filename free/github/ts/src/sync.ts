/**
 * GitHub Data Synchronization Service
 * Handles historical data sync and incremental updates
 */

import { createLogger } from '@nself/plugin-utils';
import { GitHubClient } from './client.js';
import { GitHubDatabase } from './database.js';
import type { SyncOptions, SyncStats } from './types.js';

const logger = createLogger('github:sync');

export interface SyncResult {
  success: boolean;
  stats: SyncStats;
  errors: string[];
  duration: number;
}

export class GitHubSyncService {
  private client: GitHubClient;
  private db: GitHubDatabase;
  private org?: string;
  private repos?: string[];
  private syncing = false;

  constructor(client: GitHubClient, db: GitHubDatabase, org?: string, repos?: string[]) {
    this.client = client;
    this.db = db;
    this.org = org;
    this.repos = repos;
  }

  async sync(options: SyncOptions = {}): Promise<SyncResult> {
    if (this.syncing) {
      throw new Error('Sync already in progress');
    }

    this.syncing = true;
    const startTime = Date.now();
    const errors: string[] = [];
    const stats: SyncStats = {
      repositories: 0,
      branches: 0,
      issues: 0,
      pullRequests: 0,
      prReviews: 0,
      issueComments: 0,
      prReviewComments: 0,
      commitComments: 0,
      commits: 0,
      releases: 0,
      tags: 0,
      milestones: 0,
      labels: 0,
      workflows: 0,
      workflowRuns: 0,
      workflowJobs: 0,
      checkSuites: 0,
      checkRuns: 0,
      deployments: 0,
      teams: 0,
      collaborators: 0,
    };

    const resources = options.resources ?? [
      'repositories',
      'branches',
      'issues',
      'pull_requests',
      'pr_reviews',
      'issue_comments',
      'pr_review_comments',
      'commit_comments',
      'commits',
      'releases',
      'tags',
      'milestones',
      'labels',
      'workflows',
      'workflow_runs',
      'workflow_jobs',
      'check_suites',
      'check_runs',
      'deployments',
      'teams',
      'collaborators',
    ];

    logger.info('Starting GitHub data sync', { resources, org: this.org, repos: this.repos });

    try {
      // First sync repositories
      if (resources.includes('repositories')) {
        try {
          stats.repositories = await this.syncRepositories();
          logger.success(`Synced ${stats.repositories} repositories`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Repositories sync failed: ${message}`);
          logger.error('Repositories sync failed', { error: message });
        }
      }

      // Sync teams (org-level)
      if (resources.includes('teams') && this.org) {
        try {
          stats.teams = await this.syncTeams(this.org);
          logger.success(`Synced ${stats.teams} teams`);
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Unknown error';
          errors.push(`Teams sync failed: ${message}`);
          logger.error('Teams sync failed', { error: message });
        }
      }

      // Get repos to sync other resources for
      const reposToSync = await this.getReposToSync();
      logger.info(`Syncing data for ${reposToSync.length} repositories`);

      for (const repo of reposToSync) {
        const [owner, repoName] = repo.split('/');

        // Sync Branches
        if (resources.includes('branches')) {
          try {
            const count = await this.syncBranches(owner, repoName);
            stats.branches += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Branches sync failed for ${repo}: ${message}`);
            logger.error('Branches sync failed', { repo, error: message });
          }
        }

        // Sync Issues
        if (resources.includes('issues')) {
          try {
            const count = await this.syncIssues(owner, repoName);
            stats.issues += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Issues sync failed for ${repo}: ${message}`);
            logger.error('Issues sync failed', { repo, error: message });
          }
        }

        // Sync Pull Requests
        if (resources.includes('pull_requests')) {
          try {
            const count = await this.syncPullRequests(owner, repoName);
            stats.pullRequests += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Pull requests sync failed for ${repo}: ${message}`);
            logger.error('Pull requests sync failed', { repo, error: message });
          }
        }

        // Sync PR Reviews
        if (resources.includes('pr_reviews')) {
          try {
            const count = await this.syncPullRequestReviews(owner, repoName);
            stats.prReviews += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`PR reviews sync failed for ${repo}: ${message}`);
            logger.error('PR reviews sync failed', { repo, error: message });
          }
        }

        // Sync Issue Comments
        if (resources.includes('issue_comments')) {
          try {
            const count = await this.syncIssueComments(owner, repoName);
            stats.issueComments += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Issue comments sync failed for ${repo}: ${message}`);
            logger.error('Issue comments sync failed', { repo, error: message });
          }
        }

        // Sync PR Review Comments
        if (resources.includes('pr_review_comments')) {
          try {
            const count = await this.syncPullRequestReviewComments(owner, repoName);
            stats.prReviewComments += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`PR review comments sync failed for ${repo}: ${message}`);
            logger.error('PR review comments sync failed', { repo, error: message });
          }
        }

        // Sync Commit Comments
        if (resources.includes('commit_comments')) {
          try {
            const count = await this.syncCommitComments(owner, repoName);
            stats.commitComments += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Commit comments sync failed for ${repo}: ${message}`);
            logger.error('Commit comments sync failed', { repo, error: message });
          }
        }

        // Sync Commits
        if (resources.includes('commits')) {
          try {
            const count = await this.syncCommits(owner, repoName, options.since);
            stats.commits += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Commits sync failed for ${repo}: ${message}`);
            logger.error('Commits sync failed', { repo, error: message });
          }
        }

        // Sync Releases
        if (resources.includes('releases')) {
          try {
            const count = await this.syncReleases(owner, repoName);
            stats.releases += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Releases sync failed for ${repo}: ${message}`);
            logger.error('Releases sync failed', { repo, error: message });
          }
        }

        // Sync Tags
        if (resources.includes('tags')) {
          try {
            const count = await this.syncTags(owner, repoName);
            stats.tags += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Tags sync failed for ${repo}: ${message}`);
            logger.error('Tags sync failed', { repo, error: message });
          }
        }

        // Sync Milestones
        if (resources.includes('milestones')) {
          try {
            const count = await this.syncMilestones(owner, repoName);
            stats.milestones += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Milestones sync failed for ${repo}: ${message}`);
            logger.error('Milestones sync failed', { repo, error: message });
          }
        }

        // Sync Labels
        if (resources.includes('labels')) {
          try {
            const count = await this.syncLabels(owner, repoName);
            stats.labels += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Labels sync failed for ${repo}: ${message}`);
            logger.error('Labels sync failed', { repo, error: message });
          }
        }

        // Sync Workflows
        if (resources.includes('workflows')) {
          try {
            const count = await this.syncWorkflows(owner, repoName);
            stats.workflows += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Workflows sync failed for ${repo}: ${message}`);
            logger.error('Workflows sync failed', { repo, error: message });
          }
        }

        // Sync Workflow Runs
        if (resources.includes('workflow_runs')) {
          try {
            const count = await this.syncWorkflowRuns(owner, repoName);
            stats.workflowRuns += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Workflow runs sync failed for ${repo}: ${message}`);
            logger.error('Workflow runs sync failed', { repo, error: message });
          }
        }

        // Sync Deployments
        if (resources.includes('deployments')) {
          try {
            const count = await this.syncDeployments(owner, repoName);
            stats.deployments += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Deployments sync failed for ${repo}: ${message}`);
            logger.error('Deployments sync failed', { repo, error: message });
          }
        }

        // Sync Collaborators
        if (resources.includes('collaborators')) {
          try {
            const count = await this.syncCollaborators(owner, repoName);
            stats.collaborators += count;
          } catch (error) {
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Collaborators sync failed for ${repo}: ${message}`);
            logger.error('Collaborators sync failed', { repo, error: message });
          }
        }
      }

      const duration = Date.now() - startTime;

      logger.success('GitHub sync completed', {
        duration: `${(duration / 1000).toFixed(1)}s`,
        stats,
        errors: errors.length,
      });

      return {
        success: errors.length === 0,
        stats,
        errors,
        duration,
      };
    } finally {
      this.syncing = false;
    }
  }

  private async getReposToSync(): Promise<string[]> {
    if (this.repos && this.repos.length > 0) {
      return this.repos;
    }

    const repos = await this.db.listRepositories(1000);
    return repos.map(r => r.full_name);
  }

  private async syncRepositories(): Promise<number> {
    logger.info('Syncing repositories...');

    let repos;
    if (this.org) {
      repos = await this.client.listOrgRepos(this.org);
    } else if (this.repos && this.repos.length > 0) {
      repos = [];
      for (const fullName of this.repos) {
        const [owner, repo] = fullName.split('/');
        const repoData = await this.client.getRepository(owner, repo);
        if (repoData) {
          repos.push(repoData);
        }
      }
    } else {
      repos = await this.client.listUserRepos();
    }

    // Get languages for each repo
    for (const repo of repos) {
      const [owner, repoName] = repo.full_name.split('/');
      repo.languages = await this.client.getRepositoryLanguages(owner, repoName);
    }

    return await this.db.upsertRepositories(repos);
  }

  private async syncIssues(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing issues', { owner, repo });
    const issues = await this.client.listIssues(owner, repo, 'all');
    return await this.db.upsertIssues(issues);
  }

  private async syncPullRequests(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing pull requests', { owner, repo });
    const prs = await this.client.listPullRequests(owner, repo, 'all');
    return await this.db.upsertPullRequests(prs);
  }

  private async syncCommits(owner: string, repo: string, since?: Date): Promise<number> {
    logger.debug('Syncing commits', { owner, repo, since });
    const commits = await this.client.listCommits(owner, repo, since);
    return await this.db.upsertCommits(commits);
  }

  private async syncReleases(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing releases', { owner, repo });
    const releases = await this.client.listReleases(owner, repo);
    return await this.db.upsertReleases(releases);
  }

  private async syncWorkflowRuns(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing workflow runs', { owner, repo });
    const runs = await this.client.listWorkflowRuns(owner, repo);
    return await this.db.upsertWorkflowRuns(runs);
  }

  private async syncDeployments(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing deployments', { owner, repo });
    const deployments = await this.client.listDeployments(owner, repo);
    return await this.db.upsertDeployments(deployments);
  }

  private async syncBranches(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing branches', { owner, repo });
    const branches = await this.client.listBranches(owner, repo);
    return await this.db.upsertBranches(branches);
  }

  private async syncPullRequestReviews(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing PR reviews', { owner, repo });
    // Get repo ID first
    const repoRecord = await this.db.getRepositoryByFullName(`${owner}/${repo}`);
    if (!repoRecord) return 0;

    // Get open PRs to sync reviews for
    const prs = await this.db.listPullRequests(repoRecord.id, undefined, 100);
    let count = 0;
    for (const pr of prs) {
      const reviews = await this.client.listPullRequestReviews(owner, repo, pr.number);
      count += await this.db.upsertPRReviews(reviews);
    }
    return count;
  }

  private async syncIssueComments(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing issue comments', { owner, repo });
    const comments = await this.client.listIssueComments(owner, repo);
    return await this.db.upsertIssueComments(comments);
  }

  private async syncPullRequestReviewComments(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing PR review comments', { owner, repo });
    const comments = await this.client.listPullRequestReviewComments(owner, repo);
    return await this.db.upsertPRReviewComments(comments);
  }

  private async syncCommitComments(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing commit comments', { owner, repo });
    const comments = await this.client.listCommitComments(owner, repo);
    return await this.db.upsertCommitComments(comments);
  }

  private async syncTags(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing tags', { owner, repo });
    const tags = await this.client.listTags(owner, repo);
    return await this.db.upsertTags(tags);
  }

  private async syncMilestones(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing milestones', { owner, repo });
    const milestones = await this.client.listMilestones(owner, repo);
    return await this.db.upsertMilestones(milestones);
  }

  private async syncLabels(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing labels', { owner, repo });
    const labels = await this.client.listLabels(owner, repo);
    return await this.db.upsertLabels(labels);
  }

  private async syncWorkflows(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing workflows', { owner, repo });
    const workflows = await this.client.listWorkflows(owner, repo);
    return await this.db.upsertWorkflows(workflows);
  }

  private async syncTeams(org: string): Promise<number> {
    logger.debug('Syncing teams', { org });
    const teams = await this.client.listTeams(org);
    return await this.db.upsertTeams(teams);
  }

  private async syncCollaborators(owner: string, repo: string): Promise<number> {
    logger.debug('Syncing collaborators', { owner, repo });
    const collaborators = await this.client.listCollaborators(owner, repo);
    return await this.db.upsertCollaborators(collaborators);
  }
}
