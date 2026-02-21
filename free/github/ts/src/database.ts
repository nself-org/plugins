/**
 * GitHub Database Operations
 * CRUD operations for all GitHub data in PostgreSQL
 * Supports multi-app isolation via source_account_id composite PKs
 */

import { createDatabase, createLogger, normalizeSourceAccountId, type Database } from '@nself/plugin-utils';
import type {
  GitHubRepositoryRecord,
  GitHubBranchRecord,
  GitHubIssueRecord,
  GitHubPullRequestRecord,
  GitHubPullRequestReviewRecord,
  GitHubIssueCommentRecord,
  GitHubPullRequestReviewCommentRecord,
  GitHubCommitCommentRecord,
  GitHubCommitRecord,
  GitHubReleaseRecord,
  GitHubTagRecord,
  GitHubMilestoneRecord,
  GitHubLabelRecord,
  GitHubWorkflowRecord,
  GitHubWorkflowRunRecord,
  GitHubWorkflowJobRecord,
  GitHubCheckSuiteRecord,
  GitHubCheckRunRecord,
  GitHubDeploymentRecord,
  GitHubTeamRecord,
  GitHubCollaboratorRecord,
  GitHubOrganizationRecord,
  GitHubWebhookEventRecord,
  SyncStats,
} from './types.js';

const logger = createLogger('github:db');

export class GitHubDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): GitHubDatabase {
    return new GitHubDatabase(this.db, sourceAccountId);
  }

  getSourceAccountId(): string {
    return this.sourceAccountId;
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async execute(sql: string, params?: unknown[]): Promise<number> {
    const result = await this.db.query(sql, params);
    return result.rowCount ?? 0;
  }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing GitHub schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

      -- =======================================================================
      -- Organizations
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_organizations (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        login VARCHAR(255) NOT NULL,
        name VARCHAR(255),
        description TEXT,
        company VARCHAR(255),
        blog VARCHAR(2048),
        location VARCHAR(255),
        email VARCHAR(255),
        twitter_username VARCHAR(255),
        is_verified BOOLEAN DEFAULT FALSE,
        html_url VARCHAR(2048),
        avatar_url VARCHAR(2048),
        public_repos INTEGER DEFAULT 0,
        public_gists INTEGER DEFAULT 0,
        followers INTEGER DEFAULT 0,
        following INTEGER DEFAULT 0,
        type VARCHAR(50) DEFAULT 'Organization',
        total_private_repos INTEGER,
        owned_private_repos INTEGER,
        plan JSONB,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_orgs_login ON github_organizations(login);
      CREATE INDEX IF NOT EXISTS idx_github_orgs_source ON github_organizations(source_account_id);

      -- =======================================================================
      -- Repositories
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_repositories (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        name VARCHAR(255) NOT NULL,
        full_name VARCHAR(255) NOT NULL,
        owner_login VARCHAR(255) NOT NULL,
        owner_type VARCHAR(50),
        private BOOLEAN DEFAULT FALSE,
        description TEXT,
        fork BOOLEAN DEFAULT FALSE,
        url VARCHAR(2048),
        html_url VARCHAR(2048),
        clone_url VARCHAR(2048),
        ssh_url VARCHAR(2048),
        homepage VARCHAR(2048),
        language VARCHAR(100),
        languages JSONB DEFAULT '{}',
        default_branch VARCHAR(255) DEFAULT 'main',
        size INTEGER DEFAULT 0,
        stargazers_count INTEGER DEFAULT 0,
        watchers_count INTEGER DEFAULT 0,
        forks_count INTEGER DEFAULT 0,
        open_issues_count INTEGER DEFAULT 0,
        topics JSONB DEFAULT '[]',
        visibility VARCHAR(50) DEFAULT 'public',
        archived BOOLEAN DEFAULT FALSE,
        disabled BOOLEAN DEFAULT FALSE,
        has_issues BOOLEAN DEFAULT TRUE,
        has_projects BOOLEAN DEFAULT TRUE,
        has_wiki BOOLEAN DEFAULT TRUE,
        has_pages BOOLEAN DEFAULT FALSE,
        has_downloads BOOLEAN DEFAULT TRUE,
        has_discussions BOOLEAN DEFAULT FALSE,
        allow_forking BOOLEAN DEFAULT TRUE,
        is_template BOOLEAN DEFAULT FALSE,
        license JSONB,
        pushed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_repos_owner ON github_repositories(owner_login);
      CREATE INDEX IF NOT EXISTS idx_github_repos_name ON github_repositories(full_name);
      CREATE INDEX IF NOT EXISTS idx_github_repos_language ON github_repositories(language);
      CREATE INDEX IF NOT EXISTS idx_github_repos_source ON github_repositories(source_account_id);

      -- =======================================================================
      -- Branches
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_branches (
        id VARCHAR(500) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        repo_id BIGINT,
        name VARCHAR(255) NOT NULL,
        sha VARCHAR(40) NOT NULL,
        protected BOOLEAN DEFAULT FALSE,
        protection_enabled BOOLEAN DEFAULT FALSE,
        protection JSONB,
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_branches_repo ON github_branches(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_branches_name ON github_branches(name);
      CREATE INDEX IF NOT EXISTS idx_github_branches_source ON github_branches(source_account_id);

      -- =======================================================================
      -- Issues
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_issues (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        number INTEGER NOT NULL,
        title TEXT NOT NULL,
        body TEXT,
        state VARCHAR(20) NOT NULL,
        state_reason VARCHAR(50),
        locked BOOLEAN DEFAULT FALSE,
        user_login VARCHAR(255),
        user_id BIGINT,
        labels JSONB DEFAULT '[]',
        assignees JSONB DEFAULT '[]',
        milestone JSONB,
        comments INTEGER DEFAULT 0,
        reactions JSONB DEFAULT '{}',
        html_url VARCHAR(2048),
        closed_at TIMESTAMP WITH TIME ZONE,
        closed_by_login VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_issues_repo ON github_issues(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_issues_state ON github_issues(state);
      CREATE INDEX IF NOT EXISTS idx_github_issues_user ON github_issues(user_login);
      CREATE INDEX IF NOT EXISTS idx_github_issues_created ON github_issues(created_at);
      CREATE INDEX IF NOT EXISTS idx_github_issues_source ON github_issues(source_account_id);

      -- =======================================================================
      -- Pull Requests
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_pull_requests (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        number INTEGER NOT NULL,
        title TEXT NOT NULL,
        body TEXT,
        state VARCHAR(20) NOT NULL,
        draft BOOLEAN DEFAULT FALSE,
        locked BOOLEAN DEFAULT FALSE,
        user_login VARCHAR(255),
        user_id BIGINT,
        head_ref VARCHAR(255),
        head_sha VARCHAR(40),
        head_repo_id BIGINT,
        base_ref VARCHAR(255),
        base_sha VARCHAR(40),
        merged BOOLEAN DEFAULT FALSE,
        mergeable BOOLEAN,
        mergeable_state VARCHAR(50),
        merged_by_login VARCHAR(255),
        merged_at TIMESTAMP WITH TIME ZONE,
        merge_commit_sha VARCHAR(40),
        labels JSONB DEFAULT '[]',
        assignees JSONB DEFAULT '[]',
        reviewers JSONB DEFAULT '[]',
        milestone JSONB,
        comments INTEGER DEFAULT 0,
        review_comments INTEGER DEFAULT 0,
        commits INTEGER DEFAULT 0,
        additions INTEGER DEFAULT 0,
        deletions INTEGER DEFAULT 0,
        changed_files INTEGER DEFAULT 0,
        html_url VARCHAR(2048),
        diff_url VARCHAR(2048),
        closed_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_prs_repo ON github_pull_requests(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_prs_state ON github_pull_requests(state);
      CREATE INDEX IF NOT EXISTS idx_github_prs_user ON github_pull_requests(user_login);
      CREATE INDEX IF NOT EXISTS idx_github_prs_merged ON github_pull_requests(merged);
      CREATE INDEX IF NOT EXISTS idx_github_prs_created ON github_pull_requests(created_at);
      CREATE INDEX IF NOT EXISTS idx_github_prs_source ON github_pull_requests(source_account_id);

      -- =======================================================================
      -- Pull Request Reviews
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_pr_reviews (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        pull_request_id BIGINT,
        pull_request_number INTEGER NOT NULL,
        user_login VARCHAR(255),
        user_id BIGINT,
        body TEXT,
        state VARCHAR(50) NOT NULL,
        html_url VARCHAR(2048),
        commit_id VARCHAR(40),
        submitted_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_pr_reviews_pr ON github_pr_reviews(pull_request_id);
      CREATE INDEX IF NOT EXISTS idx_github_pr_reviews_user ON github_pr_reviews(user_login);
      CREATE INDEX IF NOT EXISTS idx_github_pr_reviews_state ON github_pr_reviews(state);
      CREATE INDEX IF NOT EXISTS idx_github_pr_reviews_source ON github_pr_reviews(source_account_id);

      -- =======================================================================
      -- Issue Comments
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_issue_comments (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        issue_number INTEGER NOT NULL,
        issue_id BIGINT,
        pull_request_number INTEGER,
        user_login VARCHAR(255),
        user_id BIGINT,
        body TEXT NOT NULL,
        reactions JSONB DEFAULT '{}',
        html_url VARCHAR(2048),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_issue_comments_repo ON github_issue_comments(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_issue_comments_issue ON github_issue_comments(issue_number);
      CREATE INDEX IF NOT EXISTS idx_github_issue_comments_user ON github_issue_comments(user_login);
      CREATE INDEX IF NOT EXISTS idx_github_issue_comments_source ON github_issue_comments(source_account_id);

      -- =======================================================================
      -- Pull Request Review Comments
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_pr_review_comments (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        pull_request_id BIGINT,
        pull_request_number INTEGER NOT NULL,
        review_id BIGINT,
        user_login VARCHAR(255),
        user_id BIGINT,
        body TEXT NOT NULL,
        path VARCHAR(1024),
        position INTEGER,
        original_position INTEGER,
        diff_hunk TEXT,
        commit_id VARCHAR(40),
        original_commit_id VARCHAR(40),
        in_reply_to_id BIGINT,
        reactions JSONB DEFAULT '{}',
        html_url VARCHAR(2048),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_pr_review_comments_pr ON github_pr_review_comments(pull_request_id);
      CREATE INDEX IF NOT EXISTS idx_github_pr_review_comments_review ON github_pr_review_comments(review_id);
      CREATE INDEX IF NOT EXISTS idx_github_pr_review_comments_source ON github_pr_review_comments(source_account_id);

      -- =======================================================================
      -- Commit Comments
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_commit_comments (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        commit_sha VARCHAR(40) NOT NULL,
        user_login VARCHAR(255),
        user_id BIGINT,
        body TEXT NOT NULL,
        path VARCHAR(1024),
        position INTEGER,
        line INTEGER,
        reactions JSONB DEFAULT '{}',
        html_url VARCHAR(2048),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_commit_comments_repo ON github_commit_comments(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_commit_comments_commit ON github_commit_comments(commit_sha);
      CREATE INDEX IF NOT EXISTS idx_github_commit_comments_source ON github_commit_comments(source_account_id);

      -- =======================================================================
      -- Commits
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_commits (
        sha VARCHAR(40) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        message TEXT,
        author_name VARCHAR(255),
        author_email VARCHAR(255),
        author_login VARCHAR(255),
        author_date TIMESTAMP WITH TIME ZONE,
        committer_name VARCHAR(255),
        committer_email VARCHAR(255),
        committer_login VARCHAR(255),
        committer_date TIMESTAMP WITH TIME ZONE,
        tree_sha VARCHAR(40),
        parents JSONB DEFAULT '[]',
        additions INTEGER DEFAULT 0,
        deletions INTEGER DEFAULT 0,
        total INTEGER DEFAULT 0,
        html_url VARCHAR(2048),
        verified BOOLEAN DEFAULT FALSE,
        verification_reason VARCHAR(50),
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (sha, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_commits_repo ON github_commits(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_commits_author ON github_commits(author_login);
      CREATE INDEX IF NOT EXISTS idx_github_commits_date ON github_commits(author_date);
      CREATE INDEX IF NOT EXISTS idx_github_commits_source ON github_commits(source_account_id);

      -- =======================================================================
      -- Releases
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_releases (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        tag_name VARCHAR(255) NOT NULL,
        target_commitish VARCHAR(255),
        name VARCHAR(255),
        body TEXT,
        draft BOOLEAN DEFAULT FALSE,
        prerelease BOOLEAN DEFAULT FALSE,
        author_login VARCHAR(255),
        html_url VARCHAR(2048),
        tarball_url VARCHAR(2048),
        zipball_url VARCHAR(2048),
        assets JSONB DEFAULT '[]',
        created_at TIMESTAMP WITH TIME ZONE,
        published_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_releases_repo ON github_releases(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_releases_tag ON github_releases(tag_name);
      CREATE INDEX IF NOT EXISTS idx_github_releases_published ON github_releases(published_at);
      CREATE INDEX IF NOT EXISTS idx_github_releases_source ON github_releases(source_account_id);

      -- =======================================================================
      -- Tags
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_tags (
        id VARCHAR(500) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        repo_id BIGINT,
        name VARCHAR(255) NOT NULL,
        sha VARCHAR(40) NOT NULL,
        message TEXT,
        tagger_name VARCHAR(255),
        tagger_email VARCHAR(255),
        tagger_date TIMESTAMP WITH TIME ZONE,
        zipball_url VARCHAR(2048),
        tarball_url VARCHAR(2048),
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_tags_repo ON github_tags(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_tags_name ON github_tags(name);
      CREATE INDEX IF NOT EXISTS idx_github_tags_source ON github_tags(source_account_id);

      -- =======================================================================
      -- Milestones
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_milestones (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        number INTEGER NOT NULL,
        title VARCHAR(255) NOT NULL,
        description TEXT,
        state VARCHAR(20) NOT NULL,
        creator_login VARCHAR(255),
        open_issues INTEGER DEFAULT 0,
        closed_issues INTEGER DEFAULT 0,
        html_url VARCHAR(2048),
        due_on TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        closed_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_milestones_repo ON github_milestones(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_milestones_state ON github_milestones(state);
      CREATE INDEX IF NOT EXISTS idx_github_milestones_source ON github_milestones(source_account_id);

      -- =======================================================================
      -- Labels
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_labels (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        name VARCHAR(255) NOT NULL,
        color VARCHAR(10) NOT NULL,
        description TEXT,
        is_default BOOLEAN DEFAULT FALSE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_labels_repo ON github_labels(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_labels_name ON github_labels(name);
      CREATE INDEX IF NOT EXISTS idx_github_labels_source ON github_labels(source_account_id);

      -- =======================================================================
      -- Workflows
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_workflows (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        name VARCHAR(255) NOT NULL,
        path VARCHAR(1024) NOT NULL,
        state VARCHAR(50) NOT NULL,
        badge_url VARCHAR(2048),
        html_url VARCHAR(2048),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_workflows_repo ON github_workflows(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_workflows_state ON github_workflows(state);
      CREATE INDEX IF NOT EXISTS idx_github_workflows_source ON github_workflows(source_account_id);

      -- =======================================================================
      -- Workflow Runs
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_workflow_runs (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        workflow_id BIGINT,
        workflow_name VARCHAR(255),
        name VARCHAR(255),
        head_branch VARCHAR(255),
        head_sha VARCHAR(40),
        run_number INTEGER,
        run_attempt INTEGER DEFAULT 1,
        event VARCHAR(50),
        status VARCHAR(50),
        conclusion VARCHAR(50),
        actor_login VARCHAR(255),
        triggering_actor_login VARCHAR(255),
        html_url VARCHAR(2048),
        jobs_url VARCHAR(2048),
        logs_url VARCHAR(2048),
        run_started_at TIMESTAMP WITH TIME ZONE,
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_repo ON github_workflow_runs(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_workflow ON github_workflow_runs(workflow_id);
      CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_status ON github_workflow_runs(status);
      CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_conclusion ON github_workflow_runs(conclusion);
      CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_event ON github_workflow_runs(event);
      CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_created ON github_workflow_runs(created_at);
      CREATE INDEX IF NOT EXISTS idx_github_workflow_runs_source ON github_workflow_runs(source_account_id);

      -- =======================================================================
      -- Workflow Jobs
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_workflow_jobs (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        run_id BIGINT,
        run_attempt INTEGER DEFAULT 1,
        workflow_name VARCHAR(255),
        name VARCHAR(255) NOT NULL,
        status VARCHAR(50) NOT NULL,
        conclusion VARCHAR(50),
        head_sha VARCHAR(40),
        html_url VARCHAR(2048),
        runner_id BIGINT,
        runner_name VARCHAR(255),
        runner_group_id BIGINT,
        runner_group_name VARCHAR(255),
        labels JSONB DEFAULT '[]',
        steps JSONB DEFAULT '[]',
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_workflow_jobs_run ON github_workflow_jobs(run_id);
      CREATE INDEX IF NOT EXISTS idx_github_workflow_jobs_status ON github_workflow_jobs(status);
      CREATE INDEX IF NOT EXISTS idx_github_workflow_jobs_conclusion ON github_workflow_jobs(conclusion);
      CREATE INDEX IF NOT EXISTS idx_github_workflow_jobs_source ON github_workflow_jobs(source_account_id);

      -- =======================================================================
      -- Check Suites
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_check_suites (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        head_branch VARCHAR(255),
        head_sha VARCHAR(40) NOT NULL,
        status VARCHAR(50) NOT NULL,
        conclusion VARCHAR(50),
        app_id BIGINT,
        app_slug VARCHAR(255),
        pull_requests JSONB DEFAULT '[]',
        before_sha VARCHAR(40),
        after_sha VARCHAR(40),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_check_suites_repo ON github_check_suites(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_check_suites_sha ON github_check_suites(head_sha);
      CREATE INDEX IF NOT EXISTS idx_github_check_suites_status ON github_check_suites(status);
      CREATE INDEX IF NOT EXISTS idx_github_check_suites_source ON github_check_suites(source_account_id);

      -- =======================================================================
      -- Check Runs
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_check_runs (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        check_suite_id BIGINT,
        head_sha VARCHAR(40) NOT NULL,
        name VARCHAR(255) NOT NULL,
        status VARCHAR(50) NOT NULL,
        conclusion VARCHAR(50),
        external_id VARCHAR(255),
        html_url VARCHAR(2048),
        details_url VARCHAR(2048),
        app_id BIGINT,
        app_slug VARCHAR(255),
        output JSONB,
        pull_requests JSONB DEFAULT '[]',
        started_at TIMESTAMP WITH TIME ZONE,
        completed_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_check_runs_suite ON github_check_runs(check_suite_id);
      CREATE INDEX IF NOT EXISTS idx_github_check_runs_sha ON github_check_runs(head_sha);
      CREATE INDEX IF NOT EXISTS idx_github_check_runs_status ON github_check_runs(status);
      CREATE INDEX IF NOT EXISTS idx_github_check_runs_name ON github_check_runs(name);
      CREATE INDEX IF NOT EXISTS idx_github_check_runs_source ON github_check_runs(source_account_id);

      -- =======================================================================
      -- Deployments
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_deployments (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        repo_id BIGINT,
        sha VARCHAR(40),
        ref VARCHAR(255),
        task VARCHAR(255) DEFAULT 'deploy',
        environment VARCHAR(255),
        description TEXT,
        creator_login VARCHAR(255),
        statuses JSONB DEFAULT '[]',
        current_status VARCHAR(50),
        production_environment BOOLEAN DEFAULT FALSE,
        transient_environment BOOLEAN DEFAULT FALSE,
        payload JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_deployments_repo ON github_deployments(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_deployments_env ON github_deployments(environment);
      CREATE INDEX IF NOT EXISTS idx_github_deployments_status ON github_deployments(current_status);
      CREATE INDEX IF NOT EXISTS idx_github_deployments_source ON github_deployments(source_account_id);

      -- =======================================================================
      -- Teams
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_teams (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        node_id VARCHAR(255),
        org_login VARCHAR(255) NOT NULL,
        name VARCHAR(255) NOT NULL,
        slug VARCHAR(255) NOT NULL,
        description TEXT,
        privacy VARCHAR(50),
        permission VARCHAR(50),
        parent_id BIGINT,
        members_count INTEGER DEFAULT 0,
        repos_count INTEGER DEFAULT 0,
        html_url VARCHAR(2048),
        created_at TIMESTAMP WITH TIME ZONE,
        updated_at TIMESTAMP WITH TIME ZONE,
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_teams_org ON github_teams(org_login);
      CREATE INDEX IF NOT EXISTS idx_github_teams_slug ON github_teams(slug);
      CREATE INDEX IF NOT EXISTS idx_github_teams_source ON github_teams(source_account_id);

      -- =======================================================================
      -- Collaborators
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_collaborators (
        id BIGINT NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        repo_id BIGINT,
        login VARCHAR(255) NOT NULL,
        type VARCHAR(50),
        site_admin BOOLEAN DEFAULT FALSE,
        permissions JSONB DEFAULT '{}',
        role_name VARCHAR(50),
        synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (repo_id, id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_collaborators_login ON github_collaborators(login);
      CREATE INDEX IF NOT EXISTS idx_github_collaborators_source ON github_collaborators(source_account_id);

      -- =======================================================================
      -- Webhook Events
      -- =======================================================================
      CREATE TABLE IF NOT EXISTS github_webhook_events (
        id VARCHAR(255) NOT NULL,
        source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
        event VARCHAR(100) NOT NULL,
        action VARCHAR(100),
        repo_id BIGINT,
        repo_full_name VARCHAR(255),
        sender_login VARCHAR(255),
        data JSONB NOT NULL,
        processed BOOLEAN DEFAULT FALSE,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        PRIMARY KEY (id, source_account_id)
      );

      CREATE INDEX IF NOT EXISTS idx_github_events_event ON github_webhook_events(event);
      CREATE INDEX IF NOT EXISTS idx_github_events_repo ON github_webhook_events(repo_id);
      CREATE INDEX IF NOT EXISTS idx_github_events_processed ON github_webhook_events(processed);
      CREATE INDEX IF NOT EXISTS idx_github_events_received ON github_webhook_events(received_at);
      CREATE INDEX IF NOT EXISTS idx_github_events_source ON github_webhook_events(source_account_id);

      -- =======================================================================
      -- Views (multi-app aware)
      -- =======================================================================

      CREATE OR REPLACE VIEW github_open_items AS
      SELECT
        r.source_account_id,
        r.full_name AS repo,
        COUNT(DISTINCT i.id) FILTER (WHERE i.state = 'open') AS open_issues,
        COUNT(DISTINCT p.id) FILTER (WHERE p.state = 'open') AS open_prs,
        COUNT(DISTINCT p.id) FILTER (WHERE p.state = 'open' AND p.draft = TRUE) AS draft_prs
      FROM github_repositories r
      LEFT JOIN github_issues i ON r.id = i.repo_id AND r.source_account_id = i.source_account_id
      LEFT JOIN github_pull_requests p ON r.id = p.repo_id AND r.source_account_id = p.source_account_id
      GROUP BY r.source_account_id, r.full_name
      ORDER BY open_issues + open_prs DESC;

      CREATE OR REPLACE VIEW github_recent_activity AS
      SELECT
        c.source_account_id,
        'commit' AS type,
        c.sha AS id,
        LEFT(c.message, 100) AS title,
        c.author_login AS user_login,
        r.full_name AS repo,
        c.author_date AS created_at
      FROM github_commits c
      JOIN github_repositories r ON c.repo_id = r.id AND c.source_account_id = r.source_account_id
      WHERE c.author_date > NOW() - INTERVAL '7 days'
      UNION ALL
      SELECT
        p.source_account_id,
        'pr' AS type,
        p.id::text AS id,
        p.title,
        p.user_login,
        r.full_name AS repo,
        p.created_at
      FROM github_pull_requests p
      JOIN github_repositories r ON p.repo_id = r.id AND p.source_account_id = r.source_account_id
      WHERE p.created_at > NOW() - INTERVAL '7 days'
      UNION ALL
      SELECT
        i.source_account_id,
        'issue' AS type,
        i.id::text AS id,
        i.title,
        i.user_login,
        r.full_name AS repo,
        i.created_at
      FROM github_issues i
      JOIN github_repositories r ON i.repo_id = r.id AND i.source_account_id = r.source_account_id
      WHERE i.created_at > NOW() - INTERVAL '7 days'
      ORDER BY created_at DESC
      LIMIT 100;

      CREATE OR REPLACE VIEW github_workflow_stats AS
      SELECT
        w.source_account_id,
        r.full_name AS repo,
        w.workflow_name,
        COUNT(*) AS total_runs,
        COUNT(*) FILTER (WHERE w.conclusion = 'success') AS success,
        COUNT(*) FILTER (WHERE w.conclusion = 'failure') AS failure,
        COUNT(*) FILTER (WHERE w.conclusion = 'cancelled') AS cancelled,
        ROUND(
          COUNT(*) FILTER (WHERE w.conclusion = 'success')::numeric /
          NULLIF(COUNT(*) FILTER (WHERE w.conclusion IS NOT NULL), 0) * 100, 2
        ) AS success_rate,
        AVG(EXTRACT(EPOCH FROM (w.updated_at - w.run_started_at))) FILTER (WHERE w.conclusion IS NOT NULL) AS avg_duration_seconds
      FROM github_workflow_runs w
      JOIN github_repositories r ON w.repo_id = r.id AND w.source_account_id = r.source_account_id
      WHERE w.created_at > NOW() - INTERVAL '30 days'
      GROUP BY w.source_account_id, r.full_name, w.workflow_name
      ORDER BY total_runs DESC;

      CREATE OR REPLACE VIEW github_pr_review_stats AS
      SELECT
        pr.source_account_id,
        r.full_name AS repo,
        COUNT(DISTINCT pr.id) AS total_prs,
        COUNT(DISTINCT pr.id) FILTER (WHERE pr.merged = TRUE) AS merged_prs,
        COUNT(DISTINCT pr.id) FILTER (WHERE pr.state = 'closed' AND pr.merged = FALSE) AS closed_without_merge,
        AVG(EXTRACT(EPOCH FROM (COALESCE(pr.merged_at, pr.closed_at) - pr.created_at)) / 3600) FILTER (WHERE pr.state = 'closed') AS avg_hours_to_close,
        ROUND(AVG(pr.review_comments)::numeric, 1) AS avg_review_comments,
        ROUND(AVG(pr.commits)::numeric, 1) AS avg_commits
      FROM github_pull_requests pr
      JOIN github_repositories r ON pr.repo_id = r.id AND pr.source_account_id = r.source_account_id
      WHERE pr.created_at > NOW() - INTERVAL '90 days'
      GROUP BY pr.source_account_id, r.full_name
      ORDER BY total_prs DESC;

      CREATE OR REPLACE VIEW github_contributor_stats AS
      SELECT
        c.source_account_id,
        c.author_login AS contributor,
        r.full_name AS repo,
        COUNT(DISTINCT c.sha) AS commit_count,
        SUM(c.additions) AS total_additions,
        SUM(c.deletions) AS total_deletions,
        COUNT(DISTINCT pr.id) AS prs_opened,
        COUNT(DISTINCT pr.id) FILTER (WHERE pr.merged = TRUE) AS prs_merged
      FROM github_commits c
      JOIN github_repositories r ON c.repo_id = r.id AND c.source_account_id = r.source_account_id
      LEFT JOIN github_pull_requests pr ON pr.repo_id = r.id AND pr.user_login = c.author_login AND pr.source_account_id = c.source_account_id
      WHERE c.author_date > NOW() - INTERVAL '90 days'
        AND c.author_login IS NOT NULL
      GROUP BY c.source_account_id, c.author_login, r.full_name
      ORDER BY commit_count DESC;

      CREATE OR REPLACE VIEW github_milestone_progress AS
      SELECT
        m.source_account_id,
        r.full_name AS repo,
        m.title AS milestone,
        m.state,
        m.open_issues,
        m.closed_issues,
        m.open_issues + m.closed_issues AS total_issues,
        ROUND(
          m.closed_issues::numeric / NULLIF(m.open_issues + m.closed_issues, 0) * 100, 2
        ) AS completion_percent,
        m.due_on
      FROM github_milestones m
      JOIN github_repositories r ON m.repo_id = r.id AND m.source_account_id = r.source_account_id
      ORDER BY m.due_on ASC NULLS LAST;

      CREATE OR REPLACE VIEW github_check_status AS
      SELECT
        cs.source_account_id,
        r.full_name AS repo,
        cs.head_branch AS branch,
        cs.head_sha,
        cs.status,
        cs.conclusion,
        COUNT(cr.id) AS total_checks,
        COUNT(cr.id) FILTER (WHERE cr.conclusion = 'success') AS passed,
        COUNT(cr.id) FILTER (WHERE cr.conclusion = 'failure') AS failed,
        cs.updated_at
      FROM github_check_suites cs
      JOIN github_repositories r ON cs.repo_id = r.id AND cs.source_account_id = r.source_account_id
      LEFT JOIN github_check_runs cr ON cr.check_suite_id = cs.id AND cr.source_account_id = cs.source_account_id
      WHERE cs.created_at > NOW() - INTERVAL '7 days'
      GROUP BY cs.source_account_id, r.full_name, cs.head_branch, cs.head_sha, cs.status, cs.conclusion, cs.updated_at
      ORDER BY cs.updated_at DESC;
    `;

    await this.db.executeSqlFile(schema);

    // Migration: add source_account_id to existing tables that lack it
    const migrationCheck = await this.db.queryOne<{ exists: boolean }>(
      `SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'github_repositories' AND column_name = 'source_account_id'
      )`
    );

    if (!migrationCheck?.exists) {
      logger.info('Migrating GitHub schema for multi-app support...');

      const tables = [
        { name: 'github_organizations', pk: 'id' },
        { name: 'github_repositories', pk: 'id' },
        { name: 'github_branches', pk: 'id' },
        { name: 'github_issues', pk: 'id' },
        { name: 'github_pull_requests', pk: 'id' },
        { name: 'github_pr_reviews', pk: 'id' },
        { name: 'github_issue_comments', pk: 'id' },
        { name: 'github_pr_review_comments', pk: 'id' },
        { name: 'github_commit_comments', pk: 'id' },
        { name: 'github_commits', pk: 'sha' },
        { name: 'github_releases', pk: 'id' },
        { name: 'github_tags', pk: 'id' },
        { name: 'github_milestones', pk: 'id' },
        { name: 'github_labels', pk: 'id' },
        { name: 'github_workflows', pk: 'id' },
        { name: 'github_workflow_runs', pk: 'id' },
        { name: 'github_workflow_jobs', pk: 'id' },
        { name: 'github_check_suites', pk: 'id' },
        { name: 'github_check_runs', pk: 'id' },
        { name: 'github_deployments', pk: 'id' },
        { name: 'github_teams', pk: 'id' },
        { name: 'github_webhook_events', pk: 'id' },
      ];

      for (const { name, pk } of tables) {
        await this.db.query(`ALTER TABLE ${name} ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'`);
        await this.db.query(`ALTER TABLE ${name} DROP CONSTRAINT IF EXISTS ${name}_pkey`);
        await this.db.query(`ALTER TABLE ${name} ADD PRIMARY KEY (${pk}, source_account_id)`);
      }

      // Collaborators has a composite PK (repo_id, id)
      await this.db.query(`ALTER TABLE github_collaborators ADD COLUMN IF NOT EXISTS source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'`);
      await this.db.query(`ALTER TABLE github_collaborators DROP CONSTRAINT IF EXISTS github_collaborators_pkey`);
      await this.db.query(`ALTER TABLE github_collaborators ADD PRIMARY KEY (repo_id, id, source_account_id)`);

      logger.success('GitHub multi-app migration complete');
    }

    logger.success('GitHub schema initialized');
  }

  // =========================================================================
  // Organizations
  // =========================================================================

  async upsertOrganization(org: GitHubOrganizationRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_organizations (
        id, source_account_id, node_id, login, name, description, company, blog, location, email,
        twitter_username, is_verified, html_url, avatar_url, public_repos,
        public_gists, followers, following, type, total_private_repos,
        owned_private_repos, plan, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        login = EXCLUDED.login,
        name = EXCLUDED.name,
        description = EXCLUDED.description,
        company = EXCLUDED.company,
        blog = EXCLUDED.blog,
        location = EXCLUDED.location,
        email = EXCLUDED.email,
        twitter_username = EXCLUDED.twitter_username,
        is_verified = EXCLUDED.is_verified,
        html_url = EXCLUDED.html_url,
        avatar_url = EXCLUDED.avatar_url,
        public_repos = EXCLUDED.public_repos,
        public_gists = EXCLUDED.public_gists,
        followers = EXCLUDED.followers,
        following = EXCLUDED.following,
        type = EXCLUDED.type,
        total_private_repos = EXCLUDED.total_private_repos,
        owned_private_repos = EXCLUDED.owned_private_repos,
        plan = EXCLUDED.plan,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        org.id, this.sourceAccountId, org.node_id, org.login, org.name, org.description, org.company,
        org.blog, org.location, org.email, org.twitter_username, org.is_verified,
        org.html_url, org.avatar_url, org.public_repos, org.public_gists,
        org.followers, org.following, org.type, org.total_private_repos,
        org.owned_private_repos, org.plan ? JSON.stringify(org.plan) : null,
        org.created_at, org.updated_at,
      ]
    );
  }

  // =========================================================================
  // Repositories
  // =========================================================================

  async upsertRepository(repo: GitHubRepositoryRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_repositories (
        id, source_account_id, node_id, name, full_name, owner_login, owner_type, private,
        description, fork, url, html_url, clone_url, ssh_url, homepage,
        language, languages, default_branch, size, stargazers_count,
        watchers_count, forks_count, open_issues_count, topics, visibility,
        archived, disabled, has_issues, has_projects, has_wiki, has_pages,
        has_downloads, has_discussions, allow_forking, is_template, license,
        pushed_at, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        node_id = EXCLUDED.node_id,
        name = EXCLUDED.name,
        full_name = EXCLUDED.full_name,
        owner_login = EXCLUDED.owner_login,
        owner_type = EXCLUDED.owner_type,
        private = EXCLUDED.private,
        description = EXCLUDED.description,
        fork = EXCLUDED.fork,
        url = EXCLUDED.url,
        html_url = EXCLUDED.html_url,
        clone_url = EXCLUDED.clone_url,
        ssh_url = EXCLUDED.ssh_url,
        homepage = EXCLUDED.homepage,
        language = EXCLUDED.language,
        languages = EXCLUDED.languages,
        default_branch = EXCLUDED.default_branch,
        size = EXCLUDED.size,
        stargazers_count = EXCLUDED.stargazers_count,
        watchers_count = EXCLUDED.watchers_count,
        forks_count = EXCLUDED.forks_count,
        open_issues_count = EXCLUDED.open_issues_count,
        topics = EXCLUDED.topics,
        visibility = EXCLUDED.visibility,
        archived = EXCLUDED.archived,
        disabled = EXCLUDED.disabled,
        has_issues = EXCLUDED.has_issues,
        has_projects = EXCLUDED.has_projects,
        has_wiki = EXCLUDED.has_wiki,
        has_pages = EXCLUDED.has_pages,
        has_downloads = EXCLUDED.has_downloads,
        has_discussions = EXCLUDED.has_discussions,
        allow_forking = EXCLUDED.allow_forking,
        is_template = EXCLUDED.is_template,
        license = EXCLUDED.license,
        pushed_at = EXCLUDED.pushed_at,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        repo.id, this.sourceAccountId, repo.node_id, repo.name, repo.full_name, repo.owner_login,
        repo.owner_type, repo.private, repo.description, repo.fork, repo.url,
        repo.html_url, repo.clone_url, repo.ssh_url, repo.homepage, repo.language,
        JSON.stringify(repo.languages), repo.default_branch, repo.size,
        repo.stargazers_count, repo.watchers_count, repo.forks_count,
        repo.open_issues_count, JSON.stringify(repo.topics), repo.visibility,
        repo.archived, repo.disabled, repo.has_issues, repo.has_projects,
        repo.has_wiki, repo.has_pages, repo.has_downloads, repo.has_discussions,
        repo.allow_forking, repo.is_template, repo.license ? JSON.stringify(repo.license) : null,
        repo.pushed_at, repo.created_at, repo.updated_at,
      ]
    );
  }

  async upsertRepositories(repos: GitHubRepositoryRecord[]): Promise<number> {
    let count = 0;
    for (const repo of repos) {
      await this.upsertRepository(repo);
      count++;
    }
    return count;
  }

  async getRepository(id: number): Promise<GitHubRepositoryRecord | null> {
    return this.db.queryOne<GitHubRepositoryRecord>(
      'SELECT * FROM github_repositories WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );
  }

  async getRepositoryByFullName(fullName: string): Promise<GitHubRepositoryRecord | null> {
    return this.db.queryOne<GitHubRepositoryRecord>(
      'SELECT * FROM github_repositories WHERE full_name = $1 AND source_account_id = $2',
      [fullName, this.sourceAccountId]
    );
  }

  async listRepositories(limit = 100, offset = 0): Promise<GitHubRepositoryRecord[]> {
    const result = await this.db.query<GitHubRepositoryRecord>(
      'SELECT * FROM github_repositories WHERE source_account_id = $1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3',
      [this.sourceAccountId, limit, offset]
    );
    return result.rows;
  }

  async countRepositories(): Promise<number> {
    return this.db.countScoped('github_repositories', this.sourceAccountId);
  }

  // =========================================================================
  // Branches
  // =========================================================================

  async upsertBranch(branch: GitHubBranchRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_branches (id, source_account_id, repo_id, name, sha, protected, protection_enabled, protection, updated_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        sha = EXCLUDED.sha,
        protected = EXCLUDED.protected,
        protection_enabled = EXCLUDED.protection_enabled,
        protection = EXCLUDED.protection,
        updated_at = NOW()`,
      [
        branch.id, this.sourceAccountId, branch.repo_id, branch.name, branch.sha, branch.protected,
        branch.protection_enabled, branch.protection ? JSON.stringify(branch.protection) : null,
      ]
    );
  }

  async upsertBranches(branches: GitHubBranchRecord[]): Promise<number> {
    let count = 0;
    for (const branch of branches) {
      await this.upsertBranch(branch);
      count++;
    }
    return count;
  }

  async countBranches(): Promise<number> {
    return this.db.countScoped('github_branches', this.sourceAccountId);
  }

  // =========================================================================
  // Issues
  // =========================================================================

  async upsertIssue(issue: GitHubIssueRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_issues (
        id, source_account_id, node_id, repo_id, number, title, body, state, state_reason,
        locked, user_login, user_id, labels, assignees, milestone, comments,
        reactions, html_url, closed_at, closed_by_login, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        node_id = EXCLUDED.node_id,
        repo_id = EXCLUDED.repo_id,
        number = EXCLUDED.number,
        title = EXCLUDED.title,
        body = EXCLUDED.body,
        state = EXCLUDED.state,
        state_reason = EXCLUDED.state_reason,
        locked = EXCLUDED.locked,
        user_login = EXCLUDED.user_login,
        user_id = EXCLUDED.user_id,
        labels = EXCLUDED.labels,
        assignees = EXCLUDED.assignees,
        milestone = EXCLUDED.milestone,
        comments = EXCLUDED.comments,
        reactions = EXCLUDED.reactions,
        html_url = EXCLUDED.html_url,
        closed_at = EXCLUDED.closed_at,
        closed_by_login = EXCLUDED.closed_by_login,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        issue.id, this.sourceAccountId, issue.node_id, issue.repo_id, issue.number, issue.title,
        issue.body, issue.state, issue.state_reason, issue.locked, issue.user_login,
        issue.user_id, JSON.stringify(issue.labels), JSON.stringify(issue.assignees),
        issue.milestone ? JSON.stringify(issue.milestone) : null, issue.comments,
        JSON.stringify(issue.reactions), issue.html_url, issue.closed_at,
        issue.closed_by_login, issue.created_at, issue.updated_at,
      ]
    );
  }

  async upsertIssues(issues: GitHubIssueRecord[]): Promise<number> {
    let count = 0;
    for (const issue of issues) {
      await this.upsertIssue(issue);
      count++;
    }
    return count;
  }

  async listIssues(repoId?: number, state?: string, limit = 100, offset = 0): Promise<GitHubIssueRecord[]> {
    let sql = 'SELECT * FROM github_issues WHERE source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (repoId) {
      sql += ` AND repo_id = $${paramIndex++}`;
      params.push(repoId);
    }
    if (state) {
      sql += ` AND state = $${paramIndex++}`;
      params.push(state);
    }

    sql += ` ORDER BY updated_at DESC LIMIT $${paramIndex++} OFFSET $${paramIndex}`;
    params.push(limit, offset);

    const result = await this.db.query<GitHubIssueRecord>(sql, params);
    return result.rows;
  }

  async countIssues(state?: string): Promise<number> {
    if (state) {
      return this.db.countScoped('github_issues', this.sourceAccountId, 'state = $1', [state]);
    }
    return this.db.countScoped('github_issues', this.sourceAccountId);
  }

  // =========================================================================
  // Pull Requests
  // =========================================================================

  async upsertPullRequest(pr: GitHubPullRequestRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_pull_requests (
        id, source_account_id, node_id, repo_id, number, title, body, state, draft, locked,
        user_login, user_id, head_ref, head_sha, head_repo_id, base_ref, base_sha, merged,
        mergeable, mergeable_state, merged_by_login, merged_at, merge_commit_sha,
        labels, assignees, reviewers, milestone, comments, review_comments,
        commits, additions, deletions, changed_files, html_url, diff_url,
        closed_at, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        node_id = EXCLUDED.node_id,
        repo_id = EXCLUDED.repo_id,
        number = EXCLUDED.number,
        title = EXCLUDED.title,
        body = EXCLUDED.body,
        state = EXCLUDED.state,
        draft = EXCLUDED.draft,
        locked = EXCLUDED.locked,
        user_login = EXCLUDED.user_login,
        user_id = EXCLUDED.user_id,
        head_ref = EXCLUDED.head_ref,
        head_sha = EXCLUDED.head_sha,
        head_repo_id = EXCLUDED.head_repo_id,
        base_ref = EXCLUDED.base_ref,
        base_sha = EXCLUDED.base_sha,
        merged = EXCLUDED.merged,
        mergeable = EXCLUDED.mergeable,
        mergeable_state = EXCLUDED.mergeable_state,
        merged_by_login = EXCLUDED.merged_by_login,
        merged_at = EXCLUDED.merged_at,
        merge_commit_sha = EXCLUDED.merge_commit_sha,
        labels = EXCLUDED.labels,
        assignees = EXCLUDED.assignees,
        reviewers = EXCLUDED.reviewers,
        milestone = EXCLUDED.milestone,
        comments = EXCLUDED.comments,
        review_comments = EXCLUDED.review_comments,
        commits = EXCLUDED.commits,
        additions = EXCLUDED.additions,
        deletions = EXCLUDED.deletions,
        changed_files = EXCLUDED.changed_files,
        html_url = EXCLUDED.html_url,
        diff_url = EXCLUDED.diff_url,
        closed_at = EXCLUDED.closed_at,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        pr.id, this.sourceAccountId, pr.node_id, pr.repo_id, pr.number, pr.title, pr.body, pr.state,
        pr.draft, pr.locked, pr.user_login, pr.user_id, pr.head_ref, pr.head_sha,
        pr.head_repo_id, pr.base_ref, pr.base_sha, pr.merged, pr.mergeable, pr.mergeable_state,
        pr.merged_by_login, pr.merged_at, pr.merge_commit_sha,
        JSON.stringify(pr.labels), JSON.stringify(pr.assignees),
        JSON.stringify(pr.reviewers), pr.milestone ? JSON.stringify(pr.milestone) : null,
        pr.comments, pr.review_comments, pr.commits, pr.additions, pr.deletions,
        pr.changed_files, pr.html_url, pr.diff_url, pr.closed_at, pr.created_at, pr.updated_at,
      ]
    );
  }

  async upsertPullRequests(prs: GitHubPullRequestRecord[]): Promise<number> {
    let count = 0;
    for (const pr of prs) {
      await this.upsertPullRequest(pr);
      count++;
    }
    return count;
  }

  async listPullRequests(repoId?: number, state?: string, limit = 100, offset = 0): Promise<GitHubPullRequestRecord[]> {
    let sql = 'SELECT * FROM github_pull_requests WHERE source_account_id = $1';
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    if (repoId) {
      sql += ` AND repo_id = $${paramIndex++}`;
      params.push(repoId);
    }
    if (state) {
      sql += ` AND state = $${paramIndex++}`;
      params.push(state);
    }

    sql += ` ORDER BY updated_at DESC LIMIT $${paramIndex++} OFFSET $${paramIndex}`;
    params.push(limit, offset);

    const result = await this.db.query<GitHubPullRequestRecord>(sql, params);
    return result.rows;
  }

  async countPullRequests(state?: string): Promise<number> {
    if (state) {
      return this.db.countScoped('github_pull_requests', this.sourceAccountId, 'state = $1', [state]);
    }
    return this.db.countScoped('github_pull_requests', this.sourceAccountId);
  }

  // =========================================================================
  // PR Reviews
  // =========================================================================

  async upsertPRReview(review: GitHubPullRequestReviewRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_pr_reviews (
        id, source_account_id, node_id, repo_id, pull_request_id, pull_request_number, user_login,
        user_id, body, state, html_url, commit_id, submitted_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        body = EXCLUDED.body,
        state = EXCLUDED.state,
        submitted_at = EXCLUDED.submitted_at,
        synced_at = NOW()`,
      [
        review.id, this.sourceAccountId, review.node_id, review.repo_id, review.pull_request_id,
        review.pull_request_number, review.user_login, review.user_id,
        review.body, review.state, review.html_url, review.commit_id,
        review.submitted_at,
      ]
    );
  }

  async upsertPRReviews(reviews: GitHubPullRequestReviewRecord[]): Promise<number> {
    let count = 0;
    for (const review of reviews) {
      await this.upsertPRReview(review);
      count++;
    }
    return count;
  }

  async countPRReviews(): Promise<number> {
    return this.db.countScoped('github_pr_reviews', this.sourceAccountId);
  }

  // =========================================================================
  // Issue Comments
  // =========================================================================

  async upsertIssueComment(comment: GitHubIssueCommentRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_issue_comments (
        id, source_account_id, node_id, repo_id, issue_number, issue_id, pull_request_number,
        user_login, user_id, body, reactions, html_url, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        body = EXCLUDED.body,
        reactions = EXCLUDED.reactions,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        comment.id, this.sourceAccountId, comment.node_id, comment.repo_id, comment.issue_number,
        comment.issue_id, comment.pull_request_number, comment.user_login,
        comment.user_id, comment.body, JSON.stringify(comment.reactions),
        comment.html_url, comment.created_at, comment.updated_at,
      ]
    );
  }

  async upsertIssueComments(comments: GitHubIssueCommentRecord[]): Promise<number> {
    let count = 0;
    for (const comment of comments) {
      await this.upsertIssueComment(comment);
      count++;
    }
    return count;
  }

  async countIssueComments(): Promise<number> {
    return this.db.countScoped('github_issue_comments', this.sourceAccountId);
  }

  // =========================================================================
  // PR Review Comments
  // =========================================================================

  async upsertPRReviewComment(comment: GitHubPullRequestReviewCommentRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_pr_review_comments (
        id, source_account_id, node_id, repo_id, pull_request_id, pull_request_number, review_id,
        user_login, user_id, body, path, position, original_position, diff_hunk,
        commit_id, original_commit_id, in_reply_to_id, reactions, html_url,
        created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        body = EXCLUDED.body,
        reactions = EXCLUDED.reactions,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        comment.id, this.sourceAccountId, comment.node_id, comment.repo_id, comment.pull_request_id,
        comment.pull_request_number, comment.review_id, comment.user_login,
        comment.user_id, comment.body, comment.path, comment.position,
        comment.original_position, comment.diff_hunk, comment.commit_id,
        comment.original_commit_id, comment.in_reply_to_id,
        JSON.stringify(comment.reactions), comment.html_url,
        comment.created_at, comment.updated_at,
      ]
    );
  }

  async upsertPRReviewComments(comments: GitHubPullRequestReviewCommentRecord[]): Promise<number> {
    let count = 0;
    for (const comment of comments) {
      await this.upsertPRReviewComment(comment);
      count++;
    }
    return count;
  }

  async countPRReviewComments(): Promise<number> {
    return this.db.countScoped('github_pr_review_comments', this.sourceAccountId);
  }

  // =========================================================================
  // Commit Comments
  // =========================================================================

  async upsertCommitComment(comment: GitHubCommitCommentRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_commit_comments (
        id, source_account_id, node_id, repo_id, commit_sha, user_login, user_id, body, path,
        position, line, reactions, html_url, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        body = EXCLUDED.body,
        reactions = EXCLUDED.reactions,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        comment.id, this.sourceAccountId, comment.node_id, comment.repo_id, comment.commit_sha,
        comment.user_login, comment.user_id, comment.body, comment.path,
        comment.position, comment.line, JSON.stringify(comment.reactions),
        comment.html_url, comment.created_at, comment.updated_at,
      ]
    );
  }

  async upsertCommitComments(comments: GitHubCommitCommentRecord[]): Promise<number> {
    let count = 0;
    for (const comment of comments) {
      await this.upsertCommitComment(comment);
      count++;
    }
    return count;
  }

  async countCommitComments(): Promise<number> {
    return this.db.countScoped('github_commit_comments', this.sourceAccountId);
  }

  // =========================================================================
  // Commits
  // =========================================================================

  async upsertCommit(commit: GitHubCommitRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_commits (
        sha, source_account_id, node_id, repo_id, message, author_name, author_email, author_login,
        author_date, committer_name, committer_email, committer_login, committer_date,
        tree_sha, parents, additions, deletions, total, html_url, verified,
        verification_reason, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, NOW())
      ON CONFLICT (sha, source_account_id) DO UPDATE SET
        node_id = EXCLUDED.node_id,
        repo_id = EXCLUDED.repo_id,
        message = EXCLUDED.message,
        author_name = EXCLUDED.author_name,
        author_email = EXCLUDED.author_email,
        author_login = EXCLUDED.author_login,
        author_date = EXCLUDED.author_date,
        committer_name = EXCLUDED.committer_name,
        committer_email = EXCLUDED.committer_email,
        committer_login = EXCLUDED.committer_login,
        committer_date = EXCLUDED.committer_date,
        tree_sha = EXCLUDED.tree_sha,
        parents = EXCLUDED.parents,
        additions = EXCLUDED.additions,
        deletions = EXCLUDED.deletions,
        total = EXCLUDED.total,
        html_url = EXCLUDED.html_url,
        verified = EXCLUDED.verified,
        verification_reason = EXCLUDED.verification_reason,
        synced_at = NOW()`,
      [
        commit.sha, this.sourceAccountId, commit.node_id, commit.repo_id, commit.message,
        commit.author_name, commit.author_email, commit.author_login,
        commit.author_date, commit.committer_name, commit.committer_email,
        commit.committer_login, commit.committer_date, commit.tree_sha,
        JSON.stringify(commit.parents), commit.additions, commit.deletions,
        commit.total, commit.html_url, commit.verified, commit.verification_reason,
      ]
    );
  }

  async upsertCommits(commits: GitHubCommitRecord[]): Promise<number> {
    let count = 0;
    for (const commit of commits) {
      await this.upsertCommit(commit);
      count++;
    }
    return count;
  }

  async countCommits(): Promise<number> {
    return this.db.countScoped('github_commits', this.sourceAccountId);
  }

  // =========================================================================
  // Releases
  // =========================================================================

  async upsertRelease(release: GitHubReleaseRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_releases (
        id, source_account_id, node_id, repo_id, tag_name, target_commitish, name, body, draft,
        prerelease, author_login, html_url, tarball_url, zipball_url, assets,
        created_at, published_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        node_id = EXCLUDED.node_id,
        repo_id = EXCLUDED.repo_id,
        tag_name = EXCLUDED.tag_name,
        target_commitish = EXCLUDED.target_commitish,
        name = EXCLUDED.name,
        body = EXCLUDED.body,
        draft = EXCLUDED.draft,
        prerelease = EXCLUDED.prerelease,
        author_login = EXCLUDED.author_login,
        html_url = EXCLUDED.html_url,
        tarball_url = EXCLUDED.tarball_url,
        zipball_url = EXCLUDED.zipball_url,
        assets = EXCLUDED.assets,
        published_at = EXCLUDED.published_at,
        synced_at = NOW()`,
      [
        release.id, this.sourceAccountId, release.node_id, release.repo_id, release.tag_name,
        release.target_commitish, release.name, release.body, release.draft,
        release.prerelease, release.author_login, release.html_url,
        release.tarball_url, release.zipball_url, JSON.stringify(release.assets),
        release.created_at, release.published_at,
      ]
    );
  }

  async upsertReleases(releases: GitHubReleaseRecord[]): Promise<number> {
    let count = 0;
    for (const release of releases) {
      await this.upsertRelease(release);
      count++;
    }
    return count;
  }

  async countReleases(): Promise<number> {
    return this.db.countScoped('github_releases', this.sourceAccountId);
  }

  // =========================================================================
  // Tags
  // =========================================================================

  async upsertTag(tag: GitHubTagRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_tags (id, source_account_id, repo_id, name, sha, message, tagger_name, tagger_email, tagger_date, zipball_url, tarball_url, synced_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        sha = EXCLUDED.sha,
        message = EXCLUDED.message,
        tagger_name = EXCLUDED.tagger_name,
        tagger_email = EXCLUDED.tagger_email,
        tagger_date = EXCLUDED.tagger_date,
        synced_at = NOW()`,
      [
        tag.id, this.sourceAccountId, tag.repo_id, tag.name, tag.sha, tag.message, tag.tagger_name,
        tag.tagger_email, tag.tagger_date, tag.zipball_url, tag.tarball_url,
      ]
    );
  }

  async upsertTags(tags: GitHubTagRecord[]): Promise<number> {
    let count = 0;
    for (const tag of tags) {
      await this.upsertTag(tag);
      count++;
    }
    return count;
  }

  async countTags(): Promise<number> {
    return this.db.countScoped('github_tags', this.sourceAccountId);
  }

  // =========================================================================
  // Milestones
  // =========================================================================

  async upsertMilestone(milestone: GitHubMilestoneRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_milestones (
        id, source_account_id, node_id, repo_id, number, title, description, state, creator_login,
        open_issues, closed_issues, html_url, due_on, created_at, updated_at, closed_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        title = EXCLUDED.title,
        description = EXCLUDED.description,
        state = EXCLUDED.state,
        open_issues = EXCLUDED.open_issues,
        closed_issues = EXCLUDED.closed_issues,
        due_on = EXCLUDED.due_on,
        updated_at = EXCLUDED.updated_at,
        closed_at = EXCLUDED.closed_at,
        synced_at = NOW()`,
      [
        milestone.id, this.sourceAccountId, milestone.node_id, milestone.repo_id, milestone.number,
        milestone.title, milestone.description, milestone.state, milestone.creator_login,
        milestone.open_issues, milestone.closed_issues, milestone.html_url,
        milestone.due_on, milestone.created_at, milestone.updated_at, milestone.closed_at,
      ]
    );
  }

  async upsertMilestones(milestones: GitHubMilestoneRecord[]): Promise<number> {
    let count = 0;
    for (const milestone of milestones) {
      await this.upsertMilestone(milestone);
      count++;
    }
    return count;
  }

  async countMilestones(): Promise<number> {
    return this.db.countScoped('github_milestones', this.sourceAccountId);
  }

  // =========================================================================
  // Labels
  // =========================================================================

  async upsertLabel(label: GitHubLabelRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_labels (id, source_account_id, node_id, repo_id, name, color, description, is_default, synced_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        name = EXCLUDED.name,
        color = EXCLUDED.color,
        description = EXCLUDED.description,
        is_default = EXCLUDED.is_default,
        synced_at = NOW()`,
      [label.id, this.sourceAccountId, label.node_id, label.repo_id, label.name, label.color, label.description, label.default]
    );
  }

  async upsertLabels(labels: GitHubLabelRecord[]): Promise<number> {
    let count = 0;
    for (const label of labels) {
      await this.upsertLabel(label);
      count++;
    }
    return count;
  }

  async countLabels(): Promise<number> {
    return this.db.countScoped('github_labels', this.sourceAccountId);
  }

  // =========================================================================
  // Workflows
  // =========================================================================

  async upsertWorkflow(workflow: GitHubWorkflowRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_workflows (id, source_account_id, node_id, repo_id, name, path, state, badge_url, html_url, created_at, updated_at, synced_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        name = EXCLUDED.name,
        path = EXCLUDED.path,
        state = EXCLUDED.state,
        badge_url = EXCLUDED.badge_url,
        html_url = EXCLUDED.html_url,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        workflow.id, this.sourceAccountId, workflow.node_id, workflow.repo_id, workflow.name,
        workflow.path, workflow.state, workflow.badge_url, workflow.html_url,
        workflow.created_at, workflow.updated_at,
      ]
    );
  }

  async upsertWorkflows(workflows: GitHubWorkflowRecord[]): Promise<number> {
    let count = 0;
    for (const workflow of workflows) {
      await this.upsertWorkflow(workflow);
      count++;
    }
    return count;
  }

  async countWorkflows(): Promise<number> {
    return this.db.countScoped('github_workflows', this.sourceAccountId);
  }

  // =========================================================================
  // Workflow Runs
  // =========================================================================

  async upsertWorkflowRun(run: GitHubWorkflowRunRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_workflow_runs (
        id, source_account_id, node_id, repo_id, workflow_id, workflow_name, name, head_branch,
        head_sha, run_number, run_attempt, event, status, conclusion, actor_login,
        triggering_actor_login, html_url, jobs_url, logs_url, run_started_at,
        created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        node_id = EXCLUDED.node_id,
        repo_id = EXCLUDED.repo_id,
        workflow_id = EXCLUDED.workflow_id,
        workflow_name = EXCLUDED.workflow_name,
        name = EXCLUDED.name,
        head_branch = EXCLUDED.head_branch,
        head_sha = EXCLUDED.head_sha,
        run_number = EXCLUDED.run_number,
        run_attempt = EXCLUDED.run_attempt,
        event = EXCLUDED.event,
        status = EXCLUDED.status,
        conclusion = EXCLUDED.conclusion,
        actor_login = EXCLUDED.actor_login,
        triggering_actor_login = EXCLUDED.triggering_actor_login,
        html_url = EXCLUDED.html_url,
        jobs_url = EXCLUDED.jobs_url,
        logs_url = EXCLUDED.logs_url,
        run_started_at = EXCLUDED.run_started_at,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        run.id, this.sourceAccountId, run.node_id, run.repo_id, run.workflow_id, run.workflow_name,
        run.name, run.head_branch, run.head_sha, run.run_number, run.run_attempt,
        run.event, run.status, run.conclusion, run.actor_login,
        run.triggering_actor_login, run.html_url, run.jobs_url, run.logs_url,
        run.run_started_at, run.created_at, run.updated_at,
      ]
    );
  }

  async upsertWorkflowRuns(runs: GitHubWorkflowRunRecord[]): Promise<number> {
    let count = 0;
    for (const run of runs) {
      await this.upsertWorkflowRun(run);
      count++;
    }
    return count;
  }

  async countWorkflowRuns(): Promise<number> {
    return this.db.countScoped('github_workflow_runs', this.sourceAccountId);
  }

  // =========================================================================
  // Workflow Jobs
  // =========================================================================

  async upsertWorkflowJob(job: GitHubWorkflowJobRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_workflow_jobs (
        id, source_account_id, node_id, repo_id, run_id, run_attempt, workflow_name, name, status,
        conclusion, head_sha, html_url, runner_id, runner_name, runner_group_id,
        runner_group_name, labels, steps, started_at, completed_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        status = EXCLUDED.status,
        conclusion = EXCLUDED.conclusion,
        runner_id = EXCLUDED.runner_id,
        runner_name = EXCLUDED.runner_name,
        steps = EXCLUDED.steps,
        completed_at = EXCLUDED.completed_at,
        synced_at = NOW()`,
      [
        job.id, this.sourceAccountId, job.node_id, job.repo_id, job.run_id, job.run_attempt,
        job.workflow_name, job.name, job.status, job.conclusion, job.head_sha,
        job.html_url, job.runner_id, job.runner_name, job.runner_group_id,
        job.runner_group_name, JSON.stringify(job.labels), JSON.stringify(job.steps),
        job.started_at, job.completed_at,
      ]
    );
  }

  async upsertWorkflowJobs(jobs: GitHubWorkflowJobRecord[]): Promise<number> {
    let count = 0;
    for (const job of jobs) {
      await this.upsertWorkflowJob(job);
      count++;
    }
    return count;
  }

  async countWorkflowJobs(): Promise<number> {
    return this.db.countScoped('github_workflow_jobs', this.sourceAccountId);
  }

  // =========================================================================
  // Check Suites
  // =========================================================================

  async upsertCheckSuite(suite: GitHubCheckSuiteRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_check_suites (
        id, source_account_id, node_id, repo_id, head_branch, head_sha, status, conclusion,
        app_id, app_slug, pull_requests, before_sha, after_sha, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        status = EXCLUDED.status,
        conclusion = EXCLUDED.conclusion,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        suite.id, this.sourceAccountId, suite.node_id, suite.repo_id, suite.head_branch, suite.head_sha,
        suite.status, suite.conclusion, suite.app_id, suite.app_slug,
        JSON.stringify(suite.pull_requests), suite.before, suite.after,
        suite.created_at, suite.updated_at,
      ]
    );
  }

  async upsertCheckSuites(suites: GitHubCheckSuiteRecord[]): Promise<number> {
    let count = 0;
    for (const suite of suites) {
      await this.upsertCheckSuite(suite);
      count++;
    }
    return count;
  }

  async countCheckSuites(): Promise<number> {
    return this.db.countScoped('github_check_suites', this.sourceAccountId);
  }

  // =========================================================================
  // Check Runs
  // =========================================================================

  async upsertCheckRun(run: GitHubCheckRunRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_check_runs (
        id, source_account_id, node_id, repo_id, check_suite_id, head_sha, name, status, conclusion,
        external_id, html_url, details_url, app_id, app_slug, output, pull_requests,
        started_at, completed_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        status = EXCLUDED.status,
        conclusion = EXCLUDED.conclusion,
        output = EXCLUDED.output,
        completed_at = EXCLUDED.completed_at,
        synced_at = NOW()`,
      [
        run.id, this.sourceAccountId, run.node_id, run.repo_id, run.check_suite_id, run.head_sha,
        run.name, run.status, run.conclusion, run.external_id, run.html_url,
        run.details_url, run.app_id, run.app_slug,
        run.output ? JSON.stringify(run.output) : null,
        JSON.stringify(run.pull_requests), run.started_at, run.completed_at,
      ]
    );
  }

  async upsertCheckRuns(runs: GitHubCheckRunRecord[]): Promise<number> {
    let count = 0;
    for (const run of runs) {
      await this.upsertCheckRun(run);
      count++;
    }
    return count;
  }

  async countCheckRuns(): Promise<number> {
    return this.db.countScoped('github_check_runs', this.sourceAccountId);
  }

  // =========================================================================
  // Deployments
  // =========================================================================

  async upsertDeployment(deployment: GitHubDeploymentRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_deployments (
        id, source_account_id, node_id, repo_id, sha, ref, task, environment, description,
        creator_login, statuses, current_status, production_environment,
        transient_environment, payload, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        node_id = EXCLUDED.node_id,
        repo_id = EXCLUDED.repo_id,
        sha = EXCLUDED.sha,
        ref = EXCLUDED.ref,
        task = EXCLUDED.task,
        environment = EXCLUDED.environment,
        description = EXCLUDED.description,
        creator_login = EXCLUDED.creator_login,
        statuses = EXCLUDED.statuses,
        current_status = EXCLUDED.current_status,
        production_environment = EXCLUDED.production_environment,
        transient_environment = EXCLUDED.transient_environment,
        payload = EXCLUDED.payload,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        deployment.id, this.sourceAccountId, deployment.node_id, deployment.repo_id, deployment.sha,
        deployment.ref, deployment.task, deployment.environment, deployment.description,
        deployment.creator_login, JSON.stringify(deployment.statuses),
        deployment.current_status, deployment.production_environment,
        deployment.transient_environment, JSON.stringify(deployment.payload),
        deployment.created_at, deployment.updated_at,
      ]
    );
  }

  async upsertDeployments(deployments: GitHubDeploymentRecord[]): Promise<number> {
    let count = 0;
    for (const deployment of deployments) {
      await this.upsertDeployment(deployment);
      count++;
    }
    return count;
  }

  async countDeployments(): Promise<number> {
    return this.db.countScoped('github_deployments', this.sourceAccountId);
  }

  // =========================================================================
  // Teams
  // =========================================================================

  async upsertTeam(team: GitHubTeamRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_teams (
        id, source_account_id, node_id, org_login, name, slug, description, privacy, permission,
        parent_id, members_count, repos_count, html_url, created_at, updated_at, synced_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        name = EXCLUDED.name,
        slug = EXCLUDED.slug,
        description = EXCLUDED.description,
        privacy = EXCLUDED.privacy,
        permission = EXCLUDED.permission,
        parent_id = EXCLUDED.parent_id,
        members_count = EXCLUDED.members_count,
        repos_count = EXCLUDED.repos_count,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()`,
      [
        team.id, this.sourceAccountId, team.node_id, team.org_login, team.name, team.slug,
        team.description, team.privacy, team.permission, team.parent_id,
        team.members_count, team.repos_count, team.html_url,
        team.created_at, team.updated_at,
      ]
    );
  }

  async upsertTeams(teams: GitHubTeamRecord[]): Promise<number> {
    let count = 0;
    for (const team of teams) {
      await this.upsertTeam(team);
      count++;
    }
    return count;
  }

  async countTeams(): Promise<number> {
    return this.db.countScoped('github_teams', this.sourceAccountId);
  }

  // =========================================================================
  // Collaborators
  // =========================================================================

  async upsertCollaborator(collab: GitHubCollaboratorRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_collaborators (id, source_account_id, repo_id, login, type, site_admin, permissions, role_name, synced_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
      ON CONFLICT (repo_id, id, source_account_id) DO UPDATE SET
        login = EXCLUDED.login,
        type = EXCLUDED.type,
        site_admin = EXCLUDED.site_admin,
        permissions = EXCLUDED.permissions,
        role_name = EXCLUDED.role_name,
        synced_at = NOW()`,
      [
        collab.id, this.sourceAccountId, collab.repo_id, collab.login, collab.type,
        collab.site_admin, JSON.stringify(collab.permissions), collab.role_name,
      ]
    );
  }

  async upsertCollaborators(collabs: GitHubCollaboratorRecord[]): Promise<number> {
    let count = 0;
    for (const collab of collabs) {
      await this.upsertCollaborator(collab);
      count++;
    }
    return count;
  }

  async countCollaborators(): Promise<number> {
    return this.db.countScoped('github_collaborators', this.sourceAccountId);
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(event: GitHubWebhookEventRecord): Promise<void> {
    await this.db.execute(
      `INSERT INTO github_webhook_events (
        id, source_account_id, event, action, repo_id, repo_full_name, sender_login, data,
        processed, processed_at, error, received_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
      ON CONFLICT (id, source_account_id) DO UPDATE SET
        processed = EXCLUDED.processed,
        processed_at = EXCLUDED.processed_at,
        error = EXCLUDED.error`,
      [
        event.id, this.sourceAccountId, event.event, event.action, event.repo_id, event.repo_full_name,
        event.sender_login, JSON.stringify(event.data), event.processed,
        event.processed_at, event.error,
      ]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.db.execute(
      `UPDATE github_webhook_events SET
        processed = TRUE,
        processed_at = NOW(),
        error = $3
      WHERE id = $1 AND source_account_id = $2`,
      [id, this.sourceAccountId, error ?? null]
    );
  }

  // =========================================================================
  // Cleanup
  // =========================================================================

  async cleanupForAccount(sourceAccountId: string): Promise<number> {
    return this.db.cleanupForAccount([
      // Child tables first
      'github_webhook_events',
      'github_check_runs',
      'github_check_suites',
      'github_workflow_jobs',
      'github_workflow_runs',
      'github_workflows',
      'github_deployments',
      'github_collaborators',
      'github_pr_review_comments',
      'github_pr_reviews',
      'github_commit_comments',
      'github_issue_comments',
      'github_commits',
      'github_releases',
      'github_tags',
      'github_milestones',
      'github_labels',
      'github_pull_requests',
      'github_issues',
      'github_branches',
      'github_teams',
      'github_repositories',
      'github_organizations',
    ], sourceAccountId);
  }

  // =========================================================================
  // Stats
  // =========================================================================

  async getStats(): Promise<SyncStats> {
    const [
      repositories, branches, issues, pullRequests, prReviews,
      issueComments, prReviewComments, commitComments, commits,
      releases, tags, milestones, labels, workflows, workflowRuns,
      workflowJobs, checkSuites, checkRuns, deployments, teams, collaborators
    ] = await Promise.all([
      this.countRepositories(),
      this.countBranches(),
      this.countIssues(),
      this.countPullRequests(),
      this.countPRReviews(),
      this.countIssueComments(),
      this.countPRReviewComments(),
      this.countCommitComments(),
      this.countCommits(),
      this.countReleases(),
      this.countTags(),
      this.countMilestones(),
      this.countLabels(),
      this.countWorkflows(),
      this.countWorkflowRuns(),
      this.countWorkflowJobs(),
      this.countCheckSuites(),
      this.countCheckRuns(),
      this.countDeployments(),
      this.countTeams(),
      this.countCollaborators(),
    ]);

    return {
      repositories,
      branches,
      issues,
      pullRequests,
      prReviews,
      issueComments,
      prReviewComments,
      commitComments,
      commits,
      releases,
      tags,
      milestones,
      labels,
      workflows,
      workflowRuns,
      workflowJobs,
      checkSuites,
      checkRuns,
      deployments,
      teams,
      collaborators,
    };
  }
}
