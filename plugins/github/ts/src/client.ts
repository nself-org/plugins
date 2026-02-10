/**
 * GitHub API Client
 * Wrapper around Octokit with pagination support
 */

import { Octokit } from '@octokit/rest';
import { createLogger } from '@nself/plugin-utils';
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
  GitHubLabel,
  GitHubUser,
  GitHubMilestone,
  GitHubReactions,
  GitHubReleaseAsset,
  GitHubDeploymentStatus,
} from './types.js';

const logger = createLogger('github:client');

export class GitHubClient {
  private octokit: Octokit;

  constructor(token: string) {
    this.octokit = new Octokit({
      auth: token,
      userAgent: 'nself-github-plugin/1.0.0',
    });
    logger.info('GitHub client initialized');
  }

  // =========================================================================
  // Repositories
  // =========================================================================

  async listOrgRepos(org: string): Promise<GitHubRepositoryRecord[]> {
    logger.info('Listing organization repositories', { org });
    const repos: GitHubRepositoryRecord[] = [];

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.repos.listForOrg,
      { org, per_page: 100, type: 'all' }
    )) {
      repos.push(...response.data.map(r => this.mapRepository(r)));
      logger.debug('Fetched repos batch', { count: response.data.length, total: repos.length });
    }

    return repos;
  }

  async listUserRepos(): Promise<GitHubRepositoryRecord[]> {
    logger.info('Listing user repositories');
    const repos: GitHubRepositoryRecord[] = [];

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.repos.listForAuthenticatedUser,
      { per_page: 100, affiliation: 'owner,collaborator,organization_member' }
    )) {
      repos.push(...response.data.map(r => this.mapRepository(r)));
      logger.debug('Fetched repos batch', { count: response.data.length, total: repos.length });
    }

    return repos;
  }

  async getRepository(owner: string, repo: string): Promise<GitHubRepositoryRecord | null> {
    try {
      const { data } = await this.octokit.repos.get({ owner, repo });
      return this.mapRepository(data);
    } catch (error) {
      logger.error('Failed to get repository', { owner, repo, error });
      return null;
    }
  }

  async getRepositoryLanguages(owner: string, repo: string): Promise<Record<string, number>> {
    try {
      const { data } = await this.octokit.repos.listLanguages({ owner, repo });
      return data;
    } catch {
      return {};
    }
  }

  private mapRepository(repo: Record<string, unknown>): GitHubRepositoryRecord {
    const owner = repo.owner as { login: string; type: string } | undefined;
    return {
      id: repo.id as number,
      source_account_id: 'primary',
      node_id: repo.node_id as string,
      name: repo.name as string,
      full_name: repo.full_name as string,
      owner_login: owner?.login ?? '',
      owner_type: owner?.type ?? 'User',
      private: repo.private as boolean,
      description: repo.description as string | null,
      fork: repo.fork as boolean,
      url: repo.url as string,
      html_url: repo.html_url as string,
      clone_url: repo.clone_url as string,
      ssh_url: repo.ssh_url as string,
      homepage: repo.homepage as string | null,
      language: repo.language as string | null,
      languages: {},
      default_branch: repo.default_branch as string ?? 'main',
      size: repo.size as number ?? 0,
      stargazers_count: repo.stargazers_count as number ?? 0,
      watchers_count: repo.watchers_count as number ?? 0,
      forks_count: repo.forks_count as number ?? 0,
      open_issues_count: repo.open_issues_count as number ?? 0,
      topics: (repo.topics as string[]) ?? [],
      visibility: repo.visibility as string ?? 'public',
      archived: repo.archived as boolean ?? false,
      disabled: repo.disabled as boolean ?? false,
      has_issues: repo.has_issues as boolean ?? true,
      has_projects: repo.has_projects as boolean ?? true,
      has_wiki: repo.has_wiki as boolean ?? true,
      has_pages: repo.has_pages as boolean ?? false,
      has_downloads: repo.has_downloads as boolean ?? true,
      has_discussions: repo.has_discussions as boolean ?? false,
      is_template: repo.is_template as boolean ?? false,
      allow_forking: repo.allow_forking as boolean ?? true,
      web_commit_signoff_required: repo.web_commit_signoff_required as boolean ?? false,
      license: repo.license ? {
        key: (repo.license as Record<string, unknown>).key as string,
        name: (repo.license as Record<string, unknown>).name as string,
        spdx_id: (repo.license as Record<string, unknown>).spdx_id as string | null,
        url: (repo.license as Record<string, unknown>).url as string | null,
      } : null,
      pushed_at: repo.pushed_at ? new Date(repo.pushed_at as string) : null,
      created_at: new Date(repo.created_at as string),
      updated_at: new Date(repo.updated_at as string),
    };
  }

  // =========================================================================
  // Branches
  // =========================================================================

  async listBranches(owner: string, repo: string): Promise<GitHubBranchRecord[]> {
    logger.info('Listing branches', { owner, repo });
    const branches: GitHubBranchRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.repos.listBranches,
      { owner, repo, per_page: 100 }
    )) {
      for (const b of response.data) {
        let protection = null;
        if (b.protected) {
          try {
            const { data: protData } = await this.octokit.repos.getBranchProtection({
              owner, repo, branch: b.name
            });
            protection = {
              required_status_checks: protData.required_status_checks ? {
                strict: protData.required_status_checks.strict ?? false,
                contexts: protData.required_status_checks.contexts,
              } : null,
              required_pull_request_reviews: protData.required_pull_request_reviews ? {
                required_approving_review_count: protData.required_pull_request_reviews.required_approving_review_count ?? 1,
                dismiss_stale_reviews: protData.required_pull_request_reviews.dismiss_stale_reviews ?? false,
                require_code_owner_reviews: protData.required_pull_request_reviews.require_code_owner_reviews ?? false,
              } : null,
              enforce_admins: protData.enforce_admins?.enabled ?? false,
              required_signatures: protData.required_signatures?.enabled ?? false,
              allow_force_pushes: protData.allow_force_pushes?.enabled ?? false,
              allow_deletions: protData.allow_deletions?.enabled ?? false,
            };
          } catch {
            // Branch protection not accessible
          }
        }
        branches.push({
          id: `${repoId}_${b.name}`,
          source_account_id: 'primary',
          repo_id: repoId,
          name: b.name,
          sha: b.commit.sha,
          protected: b.protected,
          protection_enabled: b.protected,
          protection,
          updated_at: new Date(),
        });
      }
      logger.debug('Fetched branches batch', { count: response.data.length, total: branches.length });
    }

    return branches;
  }

  // =========================================================================
  // Issues
  // =========================================================================

  async listIssues(owner: string, repo: string, state: 'open' | 'closed' | 'all' = 'all'): Promise<GitHubIssueRecord[]> {
    logger.info('Listing issues', { owner, repo, state });
    const issues: GitHubIssueRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.issues.listForRepo,
      { owner, repo, state, per_page: 100, sort: 'updated', direction: 'desc' }
    )) {
      // Filter out pull requests (they appear in issues list)
      const actualIssues = response.data.filter(i => !i.pull_request);
      issues.push(...actualIssues.map(i => this.mapIssue(i, repoId)));
      logger.debug('Fetched issues batch', { count: actualIssues.length, total: issues.length });
    }

    return issues;
  }

  async getIssue(owner: string, repo: string, issueNumber: number): Promise<GitHubIssueRecord | null> {
    try {
      const { data: repoData } = await this.octokit.repos.get({ owner, repo });
      const { data } = await this.octokit.issues.get({ owner, repo, issue_number: issueNumber });
      return this.mapIssue(data, repoData.id);
    } catch (error) {
      logger.error('Failed to get issue', { owner, repo, issueNumber, error });
      return null;
    }
  }

  private mapIssue(issue: Record<string, unknown>, repoId: number): GitHubIssueRecord {
    const user = issue.user as { login: string; id: number } | null;
    const closedBy = issue.closed_by as { login: string } | null;
    const reactions = issue.reactions as Record<string, number> | undefined;

    return {
      id: issue.id as number,
      source_account_id: 'primary',
      node_id: issue.node_id as string,
      repo_id: repoId,
      number: issue.number as number,
      title: issue.title as string,
      body: issue.body as string | null,
      state: issue.state as string,
      state_reason: issue.state_reason as string | null,
      locked: issue.locked as boolean,
      user_login: user?.login ?? '',
      user_id: user?.id ?? 0,
      labels: this.mapLabels(issue.labels as Array<Record<string, unknown>> | undefined),
      assignees: this.mapUsers(issue.assignees as Array<Record<string, unknown>> | undefined),
      milestone: this.mapMilestone(issue.milestone as Record<string, unknown> | null),
      comments: issue.comments as number ?? 0,
      reactions: this.mapReactions(reactions),
      html_url: issue.html_url as string,
      closed_at: issue.closed_at ? new Date(issue.closed_at as string) : null,
      closed_by_login: closedBy?.login ?? null,
      created_at: new Date(issue.created_at as string),
      updated_at: new Date(issue.updated_at as string),
    };
  }

  // =========================================================================
  // Pull Requests
  // =========================================================================

  async listPullRequests(owner: string, repo: string, state: 'open' | 'closed' | 'all' = 'all'): Promise<GitHubPullRequestRecord[]> {
    logger.info('Listing pull requests', { owner, repo, state });
    const prs: GitHubPullRequestRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.pulls.list,
      { owner, repo, state, per_page: 100, sort: 'updated', direction: 'desc' }
    )) {
      prs.push(...response.data.map(pr => this.mapPullRequest(pr, repoId)));
      logger.debug('Fetched PRs batch', { count: response.data.length, total: prs.length });
    }

    return prs;
  }

  async getPullRequest(owner: string, repo: string, prNumber: number): Promise<GitHubPullRequestRecord | null> {
    try {
      const { data: repoData } = await this.octokit.repos.get({ owner, repo });
      const { data } = await this.octokit.pulls.get({ owner, repo, pull_number: prNumber });
      return this.mapPullRequest(data, repoData.id);
    } catch (error) {
      logger.error('Failed to get pull request', { owner, repo, prNumber, error });
      return null;
    }
  }

  private mapPullRequest(pr: Record<string, unknown>, repoId: number): GitHubPullRequestRecord {
    const user = pr.user as { login: string; id: number } | null;
    const mergedBy = pr.merged_by as { login: string } | null;
    const head = pr.head as { ref: string; sha: string };
    const base = pr.base as { ref: string; sha: string };

    return {
      id: pr.id as number,
      source_account_id: 'primary',
      node_id: pr.node_id as string,
      repo_id: repoId,
      number: pr.number as number,
      title: pr.title as string,
      body: pr.body as string | null,
      state: pr.state as string,
      draft: pr.draft as boolean ?? false,
      locked: pr.locked as boolean ?? false,
      user_login: user?.login ?? '',
      user_id: user?.id ?? 0,
      head_ref: head?.ref ?? '',
      head_sha: head?.sha ?? '',
      head_repo_id: (pr.head as { repo?: { id: number } })?.repo?.id ?? null,
      base_ref: base?.ref ?? '',
      base_sha: base?.sha ?? '',
      merged: pr.merged as boolean ?? false,
      mergeable: pr.mergeable as boolean | null,
      mergeable_state: pr.mergeable_state as string | null,
      merged_by_login: mergedBy?.login ?? null,
      merged_at: pr.merged_at ? new Date(pr.merged_at as string) : null,
      merge_commit_sha: pr.merge_commit_sha as string | null,
      labels: this.mapLabels(pr.labels as Array<Record<string, unknown>> | undefined),
      assignees: this.mapUsers(pr.assignees as Array<Record<string, unknown>> | undefined),
      reviewers: this.mapUsers(pr.requested_reviewers as Array<Record<string, unknown>> | undefined),
      milestone: this.mapMilestone(pr.milestone as Record<string, unknown> | null),
      comments: pr.comments as number ?? 0,
      review_comments: pr.review_comments as number ?? 0,
      commits: pr.commits as number ?? 0,
      additions: pr.additions as number ?? 0,
      deletions: pr.deletions as number ?? 0,
      changed_files: pr.changed_files as number ?? 0,
      html_url: pr.html_url as string,
      diff_url: pr.diff_url as string,
      closed_at: pr.closed_at ? new Date(pr.closed_at as string) : null,
      created_at: new Date(pr.created_at as string),
      updated_at: new Date(pr.updated_at as string),
    };
  }

  // =========================================================================
  // PR Reviews
  // =========================================================================

  async listPullRequestReviews(owner: string, repo: string, prNumber: number): Promise<GitHubPullRequestReviewRecord[]> {
    logger.debug('Listing PR reviews', { owner, repo, prNumber });
    const reviews: GitHubPullRequestReviewRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;
    const { data: prData } = await this.octokit.pulls.get({ owner, repo, pull_number: prNumber });
    const prId = prData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.pulls.listReviews,
      { owner, repo, pull_number: prNumber, per_page: 100 }
    )) {
      reviews.push(...response.data.map(r => this.mapPRReview(r, repoId, prId, prNumber)));
    }

    return reviews;
  }

  private mapPRReview(review: Record<string, unknown>, repoId: number, prId: number, prNumber: number): GitHubPullRequestReviewRecord {
    const user = review.user as { login: string; id: number } | null;
    return {
      id: review.id as number,
      source_account_id: 'primary',
      node_id: review.node_id as string,
      repo_id: repoId,
      pull_request_id: prId,
      pull_request_number: prNumber,
      user_login: user?.login ?? '',
      user_id: user?.id ?? 0,
      body: review.body as string | null,
      state: review.state as string,
      html_url: review.html_url as string,
      commit_id: review.commit_id as string,
      submitted_at: review.submitted_at ? new Date(review.submitted_at as string) : null,
    };
  }

  // =========================================================================
  // Issue Comments
  // =========================================================================

  async listIssueComments(owner: string, repo: string): Promise<GitHubIssueCommentRecord[]> {
    logger.info('Listing issue comments', { owner, repo });
    const comments: GitHubIssueCommentRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.issues.listCommentsForRepo,
      { owner, repo, per_page: 100, sort: 'updated', direction: 'desc' }
    )) {
      comments.push(...response.data.map(c => this.mapIssueComment(c, repoId)));
      if (comments.length >= 1000) break;
    }

    return comments;
  }

  private mapIssueComment(comment: Record<string, unknown>, repoId: number): GitHubIssueCommentRecord {
    const user = comment.user as { login: string; id: number } | null;
    const issueUrl = comment.issue_url as string;
    const issueNumber = parseInt(issueUrl.split('/').pop() ?? '0', 10);
    const reactions = comment.reactions as Record<string, number> | undefined;

    return {
      id: comment.id as number,
      source_account_id: 'primary',
      node_id: comment.node_id as string,
      repo_id: repoId,
      issue_number: issueNumber,
      issue_id: null,
      pull_request_number: null,
      user_login: user?.login ?? '',
      user_id: user?.id ?? 0,
      body: comment.body as string,
      reactions: this.mapReactions(reactions),
      html_url: comment.html_url as string,
      created_at: new Date(comment.created_at as string),
      updated_at: new Date(comment.updated_at as string),
    };
  }

  // =========================================================================
  // PR Review Comments
  // =========================================================================

  async listPullRequestReviewComments(owner: string, repo: string): Promise<GitHubPullRequestReviewCommentRecord[]> {
    logger.info('Listing PR review comments', { owner, repo });
    const comments: GitHubPullRequestReviewCommentRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.pulls.listReviewCommentsForRepo,
      { owner, repo, per_page: 100, sort: 'updated', direction: 'desc' }
    )) {
      comments.push(...response.data.map(c => this.mapPRReviewComment(c, repoId)));
      if (comments.length >= 1000) break;
    }

    return comments;
  }

  private mapPRReviewComment(comment: Record<string, unknown>, repoId: number): GitHubPullRequestReviewCommentRecord {
    const user = comment.user as { login: string; id: number } | null;
    const prUrl = comment.pull_request_url as string;
    const prNumber = parseInt(prUrl.split('/').pop() ?? '0', 10);
    const reactions = comment.reactions as Record<string, number> | undefined;

    return {
      id: comment.id as number,
      source_account_id: 'primary',
      node_id: comment.node_id as string,
      repo_id: repoId,
      pull_request_id: 0,
      pull_request_number: prNumber,
      review_id: comment.pull_request_review_id as number | null,
      user_login: user?.login ?? '',
      user_id: user?.id ?? 0,
      body: comment.body as string,
      path: comment.path as string,
      position: comment.position as number | null,
      original_position: comment.original_position as number | null,
      diff_hunk: comment.diff_hunk as string,
      commit_id: comment.commit_id as string,
      original_commit_id: comment.original_commit_id as string,
      in_reply_to_id: comment.in_reply_to_id as number | null,
      reactions: this.mapReactions(reactions),
      html_url: comment.html_url as string,
      created_at: new Date(comment.created_at as string),
      updated_at: new Date(comment.updated_at as string),
    };
  }

  // =========================================================================
  // Commit Comments
  // =========================================================================

  async listCommitComments(owner: string, repo: string): Promise<GitHubCommitCommentRecord[]> {
    logger.info('Listing commit comments', { owner, repo });
    const comments: GitHubCommitCommentRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.repos.listCommitCommentsForRepo,
      { owner, repo, per_page: 100 }
    )) {
      comments.push(...response.data.map(c => this.mapCommitComment(c, repoId)));
      if (comments.length >= 500) break;
    }

    return comments;
  }

  private mapCommitComment(comment: Record<string, unknown>, repoId: number): GitHubCommitCommentRecord {
    const user = comment.user as { login: string; id: number } | null;
    const reactions = comment.reactions as Record<string, number> | undefined;

    return {
      id: comment.id as number,
      source_account_id: 'primary',
      node_id: comment.node_id as string,
      repo_id: repoId,
      commit_sha: comment.commit_id as string,
      user_login: user?.login ?? '',
      user_id: user?.id ?? 0,
      body: comment.body as string,
      path: comment.path as string | null,
      position: comment.position as number | null,
      line: comment.line as number | null,
      reactions: this.mapReactions(reactions),
      html_url: comment.html_url as string,
      created_at: new Date(comment.created_at as string),
      updated_at: new Date(comment.updated_at as string),
    };
  }

  // =========================================================================
  // Commits
  // =========================================================================

  async listCommits(owner: string, repo: string, since?: Date): Promise<GitHubCommitRecord[]> {
    logger.info('Listing commits', { owner, repo, since });
    const commits: GitHubCommitRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    const params: Record<string, unknown> = { owner, repo, per_page: 100 };
    if (since) {
      params.since = since.toISOString();
    }

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.repos.listCommits,
      params as Parameters<typeof this.octokit.repos.listCommits>[0]
    )) {
      commits.push(...response.data.map(c => this.mapCommit(c, repoId)));
      logger.debug('Fetched commits batch', { count: response.data.length, total: commits.length });

      // Limit to recent commits to avoid massive historical data
      if (commits.length >= 1000) break;
    }

    return commits;
  }

  async getCommit(owner: string, repo: string, sha: string): Promise<GitHubCommitRecord | null> {
    try {
      const { data: repoData } = await this.octokit.repos.get({ owner, repo });
      const { data } = await this.octokit.repos.getCommit({ owner, repo, ref: sha });
      return this.mapCommit(data, repoData.id);
    } catch (error) {
      logger.error('Failed to get commit', { owner, repo, sha, error });
      return null;
    }
  }

  private mapCommit(commit: Record<string, unknown>, repoId: number): GitHubCommitRecord {
    const commitData = commit.commit as Record<string, unknown>;
    const author = commitData.author as { name: string; email: string; date: string };
    const committer = commitData.committer as { name: string; email: string; date: string };
    const authorUser = commit.author as { login: string } | null;
    const committerUser = commit.committer as { login: string } | null;
    const tree = commitData.tree as { sha: string };
    const parents = commit.parents as Array<{ sha: string }> | undefined;
    const stats = commit.stats as { additions: number; deletions: number; total: number } | undefined;
    const verification = commitData.verification as { verified: boolean; reason: string } | undefined;

    return {
      sha: commit.sha as string,
      source_account_id: 'primary',
      node_id: commit.node_id as string,
      repo_id: repoId,
      message: commitData.message as string,
      author_name: author?.name ?? '',
      author_email: author?.email ?? '',
      author_login: authorUser?.login ?? null,
      author_date: new Date(author?.date ?? new Date()),
      committer_name: committer?.name ?? '',
      committer_email: committer?.email ?? '',
      committer_login: committerUser?.login ?? null,
      committer_date: new Date(committer?.date ?? new Date()),
      tree_sha: tree?.sha ?? '',
      parents: parents?.map(p => p.sha) ?? [],
      additions: stats?.additions ?? 0,
      deletions: stats?.deletions ?? 0,
      total: stats?.total ?? 0,
      html_url: commit.html_url as string,
      verified: verification?.verified ?? false,
      verification_reason: verification?.reason ?? null,
    };
  }

  // =========================================================================
  // Releases
  // =========================================================================

  async listReleases(owner: string, repo: string): Promise<GitHubReleaseRecord[]> {
    logger.info('Listing releases', { owner, repo });
    const releases: GitHubReleaseRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.repos.listReleases,
      { owner, repo, per_page: 100 }
    )) {
      releases.push(...response.data.map(r => this.mapRelease(r, repoId)));
      logger.debug('Fetched releases batch', { count: response.data.length, total: releases.length });
    }

    return releases;
  }

  async getRelease(owner: string, repo: string, releaseId: number): Promise<GitHubReleaseRecord | null> {
    try {
      const { data: repoData } = await this.octokit.repos.get({ owner, repo });
      const { data } = await this.octokit.repos.getRelease({ owner, repo, release_id: releaseId });
      return this.mapRelease(data, repoData.id);
    } catch (error) {
      logger.error('Failed to get release', { owner, repo, releaseId, error });
      return null;
    }
  }

  private mapRelease(release: Record<string, unknown>, repoId: number): GitHubReleaseRecord {
    const author = release.author as { login: string };
    const assets = release.assets as Array<Record<string, unknown>> | undefined;

    return {
      id: release.id as number,
      source_account_id: 'primary',
      node_id: release.node_id as string,
      repo_id: repoId,
      tag_name: release.tag_name as string,
      target_commitish: release.target_commitish as string,
      name: release.name as string | null,
      body: release.body as string | null,
      draft: release.draft as boolean,
      prerelease: release.prerelease as boolean,
      author_login: author?.login ?? '',
      html_url: release.html_url as string,
      tarball_url: release.tarball_url as string | null,
      zipball_url: release.zipball_url as string | null,
      assets: this.mapReleaseAssets(assets),
      created_at: new Date(release.created_at as string),
      published_at: release.published_at ? new Date(release.published_at as string) : null,
    };
  }

  private mapReleaseAssets(assets?: Array<Record<string, unknown>>): GitHubReleaseAsset[] {
    if (!assets) return [];
    return assets.map(a => ({
      id: a.id as number,
      source_account_id: 'primary',
      name: a.name as string,
      content_type: a.content_type as string,
      size: a.size as number,
      download_count: a.download_count as number,
      browser_download_url: a.browser_download_url as string,
    }));
  }

  // =========================================================================
  // Tags
  // =========================================================================

  async listTags(owner: string, repo: string): Promise<GitHubTagRecord[]> {
    logger.info('Listing tags', { owner, repo });
    const tags: GitHubTagRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.repos.listTags,
      { owner, repo, per_page: 100 }
    )) {
      tags.push(...response.data.map(t => ({
        id: `${repoId}_${t.name}`,
        source_account_id: 'primary',
        repo_id: repoId,
        name: t.name,
        sha: t.commit.sha,
        message: null,
        tagger_name: null,
        tagger_email: null,
        tagger_date: null,
        zipball_url: t.zipball_url,
        tarball_url: t.tarball_url,
      })));
    }

    return tags;
  }

  // =========================================================================
  // Milestones
  // =========================================================================

  async listMilestones(owner: string, repo: string): Promise<GitHubMilestoneRecord[]> {
    logger.info('Listing milestones', { owner, repo });
    const milestones: GitHubMilestoneRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.issues.listMilestones,
      { owner, repo, state: 'all', per_page: 100 }
    )) {
      milestones.push(...response.data.map(m => this.mapMilestoneRecord(m, repoId)));
    }

    return milestones;
  }

  private mapMilestoneRecord(m: Record<string, unknown>, repoId: number): GitHubMilestoneRecord {
    const creator = m.creator as { login: string } | null;
    return {
      id: m.id as number,
      source_account_id: 'primary',
      node_id: m.node_id as string,
      repo_id: repoId,
      number: m.number as number,
      title: m.title as string,
      description: m.description as string | null,
      state: m.state as string,
      creator_login: creator?.login ?? '',
      open_issues: m.open_issues as number,
      closed_issues: m.closed_issues as number,
      html_url: m.html_url as string,
      due_on: m.due_on ? new Date(m.due_on as string) : null,
      created_at: new Date(m.created_at as string),
      updated_at: new Date(m.updated_at as string),
      closed_at: m.closed_at ? new Date(m.closed_at as string) : null,
    };
  }

  // =========================================================================
  // Labels
  // =========================================================================

  async listLabels(owner: string, repo: string): Promise<GitHubLabelRecord[]> {
    logger.info('Listing labels', { owner, repo });
    const labels: GitHubLabelRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.issues.listLabelsForRepo,
      { owner, repo, per_page: 100 }
    )) {
      labels.push(...response.data.map(l => ({
        id: l.id,
        source_account_id: 'primary',
        node_id: l.node_id,
        repo_id: repoId,
        name: l.name,
        color: l.color,
        description: l.description ?? null,
        default: l.default ?? false,
      })));
    }

    return labels;
  }

  // =========================================================================
  // Workflows
  // =========================================================================

  async listWorkflows(owner: string, repo: string): Promise<GitHubWorkflowRecord[]> {
    logger.info('Listing workflows', { owner, repo });
    const workflows: GitHubWorkflowRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.actions.listRepoWorkflows,
      { owner, repo, per_page: 100 }
    )) {
      workflows.push(...response.data.map(w => ({
        id: w.id,
        source_account_id: 'primary',
        node_id: w.node_id,
        repo_id: repoId,
        name: w.name,
        path: w.path,
        state: w.state,
        badge_url: w.badge_url,
        html_url: w.html_url,
        created_at: new Date(w.created_at),
        updated_at: new Date(w.updated_at),
      })));
    }

    return workflows;
  }

  // =========================================================================
  // Workflow Runs
  // =========================================================================

  async listWorkflowRuns(owner: string, repo: string): Promise<GitHubWorkflowRunRecord[]> {
    logger.info('Listing workflow runs', { owner, repo });
    const runs: GitHubWorkflowRunRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.actions.listWorkflowRunsForRepo,
      { owner, repo, per_page: 100 }
    )) {
      runs.push(...response.data.map(r => this.mapWorkflowRun(r, repoId)));
      logger.debug('Fetched workflow runs batch', { count: response.data.length, total: runs.length });

      // Limit to recent runs
      if (runs.length >= 500) break;
    }

    return runs;
  }

  async getWorkflowRun(owner: string, repo: string, runId: number): Promise<GitHubWorkflowRunRecord | null> {
    try {
      const { data: repoData } = await this.octokit.repos.get({ owner, repo });
      const { data } = await this.octokit.actions.getWorkflowRun({ owner, repo, run_id: runId });
      return this.mapWorkflowRun(data, repoData.id);
    } catch (error) {
      logger.error('Failed to get workflow run', { owner, repo, runId, error });
      return null;
    }
  }

  private mapWorkflowRun(run: Record<string, unknown>, repoId: number): GitHubWorkflowRunRecord {
    const actor = run.actor as { login: string } | null;
    const triggeringActor = run.triggering_actor as { login: string } | null;

    return {
      id: run.id as number,
      source_account_id: 'primary',
      node_id: run.node_id as string,
      repo_id: repoId,
      workflow_id: run.workflow_id as number,
      workflow_name: run.name as string,
      name: run.display_title as string ?? run.name as string,
      head_branch: run.head_branch as string,
      head_sha: run.head_sha as string,
      run_number: run.run_number as number,
      run_attempt: run.run_attempt as number ?? 1,
      event: run.event as string,
      status: run.status as string | null,
      conclusion: run.conclusion as string | null,
      actor_login: actor?.login ?? '',
      triggering_actor_login: triggeringActor?.login ?? '',
      html_url: run.html_url as string,
      jobs_url: run.jobs_url as string,
      logs_url: run.logs_url as string,
      run_started_at: run.run_started_at ? new Date(run.run_started_at as string) : null,
      created_at: new Date(run.created_at as string),
      updated_at: new Date(run.updated_at as string),
    };
  }

  // =========================================================================
  // Workflow Jobs
  // =========================================================================

  async listWorkflowJobs(owner: string, repo: string, runId: number): Promise<GitHubWorkflowJobRecord[]> {
    logger.debug('Listing workflow jobs', { owner, repo, runId });
    const jobs: GitHubWorkflowJobRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.actions.listJobsForWorkflowRun,
      { owner, repo, run_id: runId, per_page: 100 }
    )) {
      jobs.push(...response.data.map(j => this.mapWorkflowJob(j, repoId)));
    }

    return jobs;
  }

  private mapWorkflowJob(job: Record<string, unknown>, repoId: number): GitHubWorkflowJobRecord {
    const steps = job.steps as Array<Record<string, unknown>> | undefined;
    return {
      id: job.id as number,
      source_account_id: 'primary',
      node_id: job.node_id as string,
      repo_id: repoId,
      run_id: job.run_id as number,
      run_attempt: job.run_attempt as number ?? 1,
      workflow_name: job.workflow_name as string ?? '',
      name: job.name as string,
      status: job.status as string,
      conclusion: job.conclusion as string | null,
      head_sha: job.head_sha as string,
      html_url: job.html_url as string,
      runner_id: job.runner_id as number | null,
      runner_name: job.runner_name as string | null,
      runner_group_id: job.runner_group_id as number | null,
      runner_group_name: job.runner_group_name as string | null,
      labels: (job.labels as string[]) ?? [],
      steps: steps?.map(s => ({
        name: s.name as string,
        status: s.status as string,
        conclusion: s.conclusion as string | null,
        number: s.number as number,
        started_at: s.started_at as string | null,
        completed_at: s.completed_at as string | null,
      })) ?? [],
      started_at: job.started_at ? new Date(job.started_at as string) : null,
      completed_at: job.completed_at ? new Date(job.completed_at as string) : null,
    };
  }

  // =========================================================================
  // Check Suites
  // =========================================================================

  async listCheckSuites(owner: string, repo: string, ref: string): Promise<GitHubCheckSuiteRecord[]> {
    logger.debug('Listing check suites', { owner, repo, ref });
    const suites: GitHubCheckSuiteRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.checks.listSuitesForRef,
      { owner, repo, ref, per_page: 100 }
    )) {
      suites.push(...response.data.map(s => this.mapCheckSuite(s, repoId)));
    }

    return suites;
  }

  private mapCheckSuite(suite: Record<string, unknown>, repoId: number): GitHubCheckSuiteRecord {
    const app = suite.app as { id: number; slug: string } | null;
    const prs = suite.pull_requests as Array<{ number: number }> | undefined;
    return {
      id: suite.id as number,
      source_account_id: 'primary',
      node_id: suite.node_id as string,
      repo_id: repoId,
      head_branch: suite.head_branch as string | null,
      head_sha: suite.head_sha as string,
      status: suite.status as string,
      conclusion: suite.conclusion as string | null,
      app_id: app?.id ?? null,
      app_slug: app?.slug ?? null,
      pull_requests: prs?.map(p => p.number) ?? [],
      before: suite.before as string | null,
      after: suite.after as string | null,
      created_at: new Date(suite.created_at as string),
      updated_at: new Date(suite.updated_at as string),
    };
  }

  // =========================================================================
  // Check Runs
  // =========================================================================

  async listCheckRuns(owner: string, repo: string, ref: string): Promise<GitHubCheckRunRecord[]> {
    logger.debug('Listing check runs', { owner, repo, ref });
    const runs: GitHubCheckRunRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.checks.listForRef,
      { owner, repo, ref, per_page: 100 }
    )) {
      runs.push(...response.data.map(r => this.mapCheckRun(r, repoId)));
    }

    return runs;
  }

  private mapCheckRun(run: Record<string, unknown>, repoId: number): GitHubCheckRunRecord {
    const app = run.app as { id: number; slug: string } | null;
    const prs = run.pull_requests as Array<{ number: number }> | undefined;
    const output = run.output as Record<string, unknown> | undefined;
    return {
      id: run.id as number,
      source_account_id: 'primary',
      node_id: run.node_id as string,
      repo_id: repoId,
      check_suite_id: (run.check_suite as { id: number })?.id ?? 0,
      head_sha: run.head_sha as string,
      name: run.name as string,
      status: run.status as string,
      conclusion: run.conclusion as string | null,
      external_id: run.external_id as string | null,
      html_url: run.html_url as string,
      details_url: run.details_url as string | null,
      app_id: app?.id ?? null,
      app_slug: app?.slug ?? null,
      output: output ? {
        title: output.title as string | null,
        summary: output.summary as string | null,
        text: output.text as string | null,
        annotations_count: output.annotations_count as number ?? 0,
      } : null,
      pull_requests: prs?.map(p => p.number) ?? [],
      started_at: run.started_at ? new Date(run.started_at as string) : null,
      completed_at: run.completed_at ? new Date(run.completed_at as string) : null,
    };
  }

  // =========================================================================
  // Teams
  // =========================================================================

  async listTeams(org: string): Promise<GitHubTeamRecord[]> {
    logger.info('Listing teams', { org });
    const teams: GitHubTeamRecord[] = [];

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.teams.list,
      { org, per_page: 100 }
    )) {
      teams.push(...response.data.map(t => ({
        id: t.id,
        source_account_id: 'primary',
        node_id: t.node_id,
        org_login: org,
        name: t.name,
        slug: t.slug,
        description: t.description ?? null,
        privacy: t.privacy ?? 'closed',
        permission: t.permission ?? 'pull',
        parent_id: t.parent?.id ?? null,
        members_count: (t as Record<string, unknown>).members_count as number ?? 0,
        repos_count: (t as Record<string, unknown>).repos_count as number ?? 0,
        html_url: t.html_url,
        created_at: new Date((t as Record<string, unknown>).created_at as string ?? new Date()),
        updated_at: new Date((t as Record<string, unknown>).updated_at as string ?? new Date()),
      })));
    }

    return teams;
  }

  // =========================================================================
  // Collaborators
  // =========================================================================

  async listCollaborators(owner: string, repo: string): Promise<GitHubCollaboratorRecord[]> {
    logger.info('Listing collaborators', { owner, repo });
    const collaborators: GitHubCollaboratorRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    try {
      for await (const response of this.octokit.paginate.iterator(
        this.octokit.repos.listCollaborators,
        { owner, repo, per_page: 100 }
      )) {
        collaborators.push(...response.data.map(c => ({
          id: c.id,
          source_account_id: 'primary',
          repo_id: repoId,
          login: c.login,
          type: c.type,
          site_admin: c.site_admin,
          permissions: {
            admin: c.permissions?.admin ?? false,
            maintain: c.permissions?.maintain ?? false,
            push: c.permissions?.push ?? false,
            triage: c.permissions?.triage ?? false,
            pull: c.permissions?.pull ?? false,
          },
          role_name: c.role_name ?? 'read',
        })));
      }
    } catch {
      // Collaborators list may not be accessible for all repos
    }

    return collaborators;
  }

  // =========================================================================
  // Deployments
  // =========================================================================

  async listDeployments(owner: string, repo: string): Promise<GitHubDeploymentRecord[]> {
    logger.info('Listing deployments', { owner, repo });
    const deployments: GitHubDeploymentRecord[] = [];

    const { data: repoData } = await this.octokit.repos.get({ owner, repo });
    const repoId = repoData.id;

    for await (const response of this.octokit.paginate.iterator(
      this.octokit.repos.listDeployments,
      { owner, repo, per_page: 100 }
    )) {
      for (const deployment of response.data) {
        const statuses = await this.getDeploymentStatuses(owner, repo, deployment.id);
        deployments.push(this.mapDeployment(deployment, repoId, statuses));
      }
      logger.debug('Fetched deployments batch', { count: response.data.length, total: deployments.length });
    }

    return deployments;
  }

  async getDeploymentStatuses(owner: string, repo: string, deploymentId: number): Promise<GitHubDeploymentStatus[]> {
    try {
      const { data } = await this.octokit.repos.listDeploymentStatuses({
        owner,
        repo,
        deployment_id: deploymentId,
        per_page: 10,
      });
      return data.map(s => ({
        id: s.id,
        source_account_id: 'primary',
        state: s.state,
        description: s.description ?? '',
        environment_url: s.environment_url ?? null,
        created_at: s.created_at,
      }));
    } catch {
      return [];
    }
  }

  private mapDeployment(deployment: Record<string, unknown>, repoId: number, statuses: GitHubDeploymentStatus[]): GitHubDeploymentRecord {
    const creator = deployment.creator as { login: string } | null;

    return {
      id: deployment.id as number,
      source_account_id: 'primary',
      node_id: deployment.node_id as string,
      repo_id: repoId,
      sha: deployment.sha as string,
      ref: deployment.ref as string,
      task: deployment.task as string,
      environment: deployment.environment as string,
      description: deployment.description as string | null,
      creator_login: creator?.login ?? '',
      statuses,
      current_status: statuses[0]?.state ?? null,
      production_environment: deployment.production_environment as boolean ?? false,
      transient_environment: deployment.transient_environment as boolean ?? false,
      payload: (deployment.payload as Record<string, unknown>) ?? {},
      created_at: new Date(deployment.created_at as string),
      updated_at: new Date(deployment.updated_at as string),
    };
  }

  // =========================================================================
  // Helpers
  // =========================================================================

  private mapLabels(labels?: Array<Record<string, unknown>>): GitHubLabel[] {
    if (!labels) return [];
    return labels.map(l => ({
      id: l.id as number,
      source_account_id: 'primary',
      name: l.name as string,
      color: l.color as string,
      description: l.description as string | null,
    }));
  }

  private mapUsers(users?: Array<Record<string, unknown>>): GitHubUser[] {
    if (!users) return [];
    return users.map(u => ({
      id: u.id as number,
      source_account_id: 'primary',
      login: u.login as string,
      type: u.type as string,
      avatar_url: u.avatar_url as string,
    }));
  }

  private mapMilestone(milestone: Record<string, unknown> | null): GitHubMilestone | null {
    if (!milestone) return null;
    return {
      id: milestone.id as number,
      number: milestone.number as number,
      title: milestone.title as string,
      description: milestone.description as string | null,
      state: milestone.state as string,
      due_on: milestone.due_on as string | null,
    };
  }

  private mapReactions(reactions?: Record<string, number>): GitHubReactions {
    return {
      total_count: reactions?.total_count ?? 0,
      '+1': reactions?.['+1'] ?? 0,
      '-1': reactions?.['-1'] ?? 0,
      laugh: reactions?.laugh ?? 0,
      hooray: reactions?.hooray ?? 0,
      confused: reactions?.confused ?? 0,
      heart: reactions?.heart ?? 0,
      rocket: reactions?.rocket ?? 0,
      eyes: reactions?.eyes ?? 0,
    };
  }
}
