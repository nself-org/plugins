/**
 * GitHub Webhook Handlers
 * Process incoming GitHub webhook events
 */

import { createLogger } from '@nself/plugin-utils';
import { GitHubClient } from './client.js';
import { GitHubDatabase } from './database.js';
import type { GitHubWebhookEventRecord } from './types.js';

const logger = createLogger('github:webhooks');

export type WebhookPayload = Record<string, unknown>;
export type WebhookHandlerFn = (payload: WebhookPayload, action?: string) => Promise<void>;

export class GitHubWebhookHandler {
  private client: GitHubClient;
  private db: GitHubDatabase;
  private handlers: Map<string, WebhookHandlerFn>;

  constructor(client: GitHubClient, db: GitHubDatabase) {
    this.client = client;
    this.db = db;
    this.handlers = new Map();

    this.registerDefaultHandlers();
  }

  private registerDefaultHandlers(): void {
    // Push events
    this.register('push', this.handlePush.bind(this));

    // Pull request events
    this.register('pull_request', this.handlePullRequest.bind(this));
    this.register('pull_request_review', this.handlePullRequestReview.bind(this));

    // Issue events
    this.register('issues', this.handleIssues.bind(this));
    this.register('issue_comment', this.handleIssueComment.bind(this));

    // Release events
    this.register('release', this.handleRelease.bind(this));

    // Workflow events
    this.register('workflow_run', this.handleWorkflowRun.bind(this));
    this.register('workflow_job', this.handleWorkflowJob.bind(this));

    // Deployment events
    this.register('deployment', this.handleDeployment.bind(this));
    this.register('deployment_status', this.handleDeploymentStatus.bind(this));

    // Repository events
    this.register('repository', this.handleRepository.bind(this));
    this.register('create', this.handleCreate.bind(this));
    this.register('delete', this.handleDelete.bind(this));

    // Star events
    this.register('star', this.handleStar.bind(this));
    this.register('fork', this.handleFork.bind(this));

    // Branch events
    this.register('branch_protection_rule', this.handleBranchProtectionRule.bind(this));

    // Check events
    this.register('check_suite', this.handleCheckSuite.bind(this));
    this.register('check_run', this.handleCheckRun.bind(this));

    // Label and Milestone events
    this.register('label', this.handleLabel.bind(this));
    this.register('milestone', this.handleMilestone.bind(this));

    // Team and Member events
    this.register('team', this.handleTeam.bind(this));
    this.register('member', this.handleMember.bind(this));

    // Pull request review comment
    this.register('pull_request_review_comment', this.handlePullRequestReviewComment.bind(this));

    // Commit comment
    this.register('commit_comment', this.handleCommitComment.bind(this));
  }

  register(event: string, handler: WebhookHandlerFn): void {
    this.handlers.set(event, handler);
  }

  async handle(deliveryId: string, event: string, payload: WebhookPayload): Promise<void> {
    const action = payload.action as string | undefined;
    const repository = payload.repository as { id: number; full_name: string } | undefined;
    const sender = payload.sender as { login: string } | undefined;

    const eventRecord: GitHubWebhookEventRecord = {
      id: deliveryId,
      source_account_id: 'primary',
      event,
      action: action ?? null,
      repo_id: repository?.id ?? null,
      repo_full_name: repository?.full_name ?? null,
      sender_login: sender?.login ?? null,
      data: payload,
      processed: false,
      processed_at: null,
      error: null,
      received_at: new Date(),
    };

    // Store the event
    await this.db.insertWebhookEvent(eventRecord);
    logger.info('Webhook event received', { event, action, deliveryId });

    // Find and execute handler
    const handler = this.handlers.get(event);

    if (handler) {
      try {
        await handler(payload, action);
        await this.db.markEventProcessed(deliveryId);
        logger.success('Webhook event processed', { event, action, deliveryId });
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        await this.db.markEventProcessed(deliveryId, message);
        logger.error('Webhook event processing failed', { event, deliveryId, error: message });
        throw error;
      }
    } else {
      await this.db.markEventProcessed(deliveryId);
      logger.debug('No handler for event', { event });
    }
  }

  // =========================================================================
  // Push Handler
  // =========================================================================

  private async handlePush(payload: WebhookPayload): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const commits = payload.commits as Array<Record<string, unknown>> | undefined;

    logger.info('Push event', {
      repo: repository.full_name,
      commits: commits?.length ?? 0,
    });

    // Sync the repository data
    const [owner, repo] = repository.full_name.split('/');
    const repoData = await this.client.getRepository(owner, repo);
    if (repoData) {
      await this.db.upsertRepository(repoData);
    }

    // Sync recent commits
    const recentCommits = await this.client.listCommits(owner, repo);
    await this.db.upsertCommits(recentCommits.slice(0, 50));
  }

  // =========================================================================
  // Pull Request Handlers
  // =========================================================================

  private async handlePullRequest(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const pullRequest = payload.pull_request as { number: number };

    logger.info('Pull request event', {
      action,
      repo: repository.full_name,
      number: pullRequest.number,
    });

    const [owner, repo] = repository.full_name.split('/');
    const pr = await this.client.getPullRequest(owner, repo, pullRequest.number);
    if (pr) {
      await this.db.upsertPullRequest(pr);
    }
  }

  private async handlePullRequestReview(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const pullRequest = payload.pull_request as { number: number };

    logger.info('Pull request review', {
      action,
      repo: repository.full_name,
      number: pullRequest.number,
    });

    // Re-sync the PR to get updated review status
    const [owner, repo] = repository.full_name.split('/');
    const pr = await this.client.getPullRequest(owner, repo, pullRequest.number);
    if (pr) {
      await this.db.upsertPullRequest(pr);
    }
  }

  // =========================================================================
  // Issue Handlers
  // =========================================================================

  private async handleIssues(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const issue = payload.issue as { number: number };

    logger.info('Issue event', {
      action,
      repo: repository.full_name,
      number: issue.number,
    });

    const [owner, repo] = repository.full_name.split('/');
    const issueData = await this.client.getIssue(owner, repo, issue.number);
    if (issueData) {
      await this.db.upsertIssue(issueData);
    }
  }

  private async handleIssueComment(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const issue = payload.issue as { number: number };

    logger.info('Issue comment', {
      action,
      repo: repository.full_name,
      issue: issue.number,
    });

    // Re-sync the issue to get updated comment count
    const [owner, repo] = repository.full_name.split('/');
    const issueData = await this.client.getIssue(owner, repo, issue.number);
    if (issueData) {
      await this.db.upsertIssue(issueData);
    }
  }

  // =========================================================================
  // Release Handler
  // =========================================================================

  private async handleRelease(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const release = payload.release as { id: number; tag_name: string };

    logger.info('Release event', {
      action,
      repo: repository.full_name,
      tag: release.tag_name,
    });

    const [owner, repo] = repository.full_name.split('/');
    const releaseData = await this.client.getRelease(owner, repo, release.id);
    if (releaseData) {
      await this.db.upsertRelease(releaseData);
    }
  }

  // =========================================================================
  // Workflow Handlers
  // =========================================================================

  private async handleWorkflowRun(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const workflowRun = payload.workflow_run as { id: number; name: string; conclusion: string | null };

    logger.info('Workflow run', {
      action,
      repo: repository.full_name,
      name: workflowRun.name,
      conclusion: workflowRun.conclusion,
    });

    const [owner, repo] = repository.full_name.split('/');
    const run = await this.client.getWorkflowRun(owner, repo, workflowRun.id);
    if (run) {
      await this.db.upsertWorkflowRun(run);
    }
  }

  private async handleWorkflowJob(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const workflowJob = payload.workflow_job as { run_id: number; name: string };

    logger.info('Workflow job', {
      action,
      repo: repository.full_name,
      name: workflowJob.name,
    });

    // Sync the workflow run
    const [owner, repo] = repository.full_name.split('/');
    const run = await this.client.getWorkflowRun(owner, repo, workflowJob.run_id);
    if (run) {
      await this.db.upsertWorkflowRun(run);
    }
  }

  // =========================================================================
  // Deployment Handlers
  // =========================================================================

  private async handleDeployment(payload: WebhookPayload): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const deployment = payload.deployment as { id: number; environment: string };

    logger.info('Deployment created', {
      repo: repository.full_name,
      environment: deployment.environment,
    });

    // Sync deployments for this repo
    const [owner, repo] = repository.full_name.split('/');
    const deployments = await this.client.listDeployments(owner, repo);
    await this.db.upsertDeployments(deployments);
  }

  private async handleDeploymentStatus(payload: WebhookPayload): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const deploymentStatus = payload.deployment_status as { state: string };
    const deployment = payload.deployment as { id: number; environment: string };

    logger.info('Deployment status', {
      repo: repository.full_name,
      environment: deployment.environment,
      status: deploymentStatus.state,
    });

    // Sync deployments to get latest status
    const [owner, repo] = repository.full_name.split('/');
    const deployments = await this.client.listDeployments(owner, repo);
    await this.db.upsertDeployments(deployments);
  }

  // =========================================================================
  // Repository Handlers
  // =========================================================================

  private async handleRepository(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };

    logger.info('Repository event', {
      action,
      repo: repository.full_name,
    });

    const [owner, repo] = repository.full_name.split('/');
    const repoData = await this.client.getRepository(owner, repo);
    if (repoData) {
      await this.db.upsertRepository(repoData);
    }
  }

  private async handleCreate(payload: WebhookPayload): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const refType = payload.ref_type as string;
    const ref = payload.ref as string;

    logger.info('Create event', {
      repo: repository.full_name,
      refType,
      ref,
    });

    // Sync repository to get updated refs
    const [owner, repo] = repository.full_name.split('/');
    const repoData = await this.client.getRepository(owner, repo);
    if (repoData) {
      await this.db.upsertRepository(repoData);
    }
  }

  private async handleDelete(payload: WebhookPayload): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const refType = payload.ref_type as string;
    const ref = payload.ref as string;

    logger.info('Delete event', {
      repo: repository.full_name,
      refType,
      ref,
    });
  }

  // =========================================================================
  // Other Handlers
  // =========================================================================

  private async handleStar(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string; stargazers_count: number };

    logger.info('Star event', {
      action,
      repo: repository.full_name,
      stars: repository.stargazers_count,
    });

    // Update repo stats
    const [owner, repo] = repository.full_name.split('/');
    const repoData = await this.client.getRepository(owner, repo);
    if (repoData) {
      await this.db.upsertRepository(repoData);
    }
  }

  private async handleFork(payload: WebhookPayload): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string; forks_count: number };
    const forkee = payload.forkee as { full_name: string };

    logger.info('Fork event', {
      repo: repository.full_name,
      forkee: forkee.full_name,
      forks: repository.forks_count,
    });

    // Update repo stats
    const [owner, repo] = repository.full_name.split('/');
    const repoData = await this.client.getRepository(owner, repo);
    if (repoData) {
      await this.db.upsertRepository(repoData);
    }
  }

  // =========================================================================
  // Branch Handler
  // =========================================================================

  private async handleBranchProtectionRule(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };

    logger.info('Branch protection rule event', {
      action,
      repo: repository.full_name,
    });

    // Sync branches to get updated protection status
    const [owner, repo] = repository.full_name.split('/');
    const branches = await this.client.listBranches(owner, repo);
    await this.db.upsertBranches(branches);
  }

  // =========================================================================
  // Check Handlers
  // =========================================================================

  private async handleCheckSuite(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const checkSuite = payload.check_suite as { id: number; head_sha: string };

    logger.info('Check suite event', {
      action,
      repo: repository.full_name,
      id: checkSuite.id,
    });

    // Sync check suites for this ref
    const [owner, repo] = repository.full_name.split('/');
    const suites = await this.client.listCheckSuites(owner, repo, checkSuite.head_sha);
    await this.db.upsertCheckSuites(suites);
  }

  private async handleCheckRun(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const checkRun = payload.check_run as { id: number; head_sha: string; name: string };

    logger.info('Check run event', {
      action,
      repo: repository.full_name,
      name: checkRun.name,
    });

    // Sync check runs for this ref
    const [owner, repo] = repository.full_name.split('/');
    const runs = await this.client.listCheckRuns(owner, repo, checkRun.head_sha);
    await this.db.upsertCheckRuns(runs);
  }

  // =========================================================================
  // Label and Milestone Handlers
  // =========================================================================

  private async handleLabel(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };

    logger.info('Label event', {
      action,
      repo: repository.full_name,
    });

    // Sync all labels
    const [owner, repo] = repository.full_name.split('/');
    const labels = await this.client.listLabels(owner, repo);
    await this.db.upsertLabels(labels);
  }

  private async handleMilestone(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const milestone = payload.milestone as { number: number; title: string };

    logger.info('Milestone event', {
      action,
      repo: repository.full_name,
      title: milestone.title,
    });

    // Sync all milestones
    const [owner, repo] = repository.full_name.split('/');
    const milestones = await this.client.listMilestones(owner, repo);
    await this.db.upsertMilestones(milestones);
  }

  // =========================================================================
  // Team and Member Handlers
  // =========================================================================

  private async handleTeam(payload: WebhookPayload, action?: string): Promise<void> {
    const organization = payload.organization as { login: string } | undefined;
    const team = payload.team as { name: string };

    logger.info('Team event', {
      action,
      org: organization?.login,
      team: team.name,
    });

    // Sync all teams if org is available
    if (organization) {
      const teams = await this.client.listTeams(organization.login);
      await this.db.upsertTeams(teams);
    }
  }

  private async handleMember(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const member = payload.member as { login: string };

    logger.info('Member event', {
      action,
      repo: repository.full_name,
      member: member.login,
    });

    // Sync collaborators
    const [owner, repo] = repository.full_name.split('/');
    const collaborators = await this.client.listCollaborators(owner, repo);
    await this.db.upsertCollaborators(collaborators);
  }

  // =========================================================================
  // Comment Handlers
  // =========================================================================

  private async handlePullRequestReviewComment(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const pullRequest = payload.pull_request as { number: number };

    logger.info('PR review comment event', {
      action,
      repo: repository.full_name,
      pr: pullRequest.number,
    });

    // Sync PR review comments for the repo
    const [owner, repo] = repository.full_name.split('/');
    const comments = await this.client.listPullRequestReviewComments(owner, repo);
    await this.db.upsertPRReviewComments(comments);
  }

  private async handleCommitComment(payload: WebhookPayload, action?: string): Promise<void> {
    const repository = payload.repository as { id: number; full_name: string };
    const comment = payload.comment as { commit_id: string };

    logger.info('Commit comment event', {
      action,
      repo: repository.full_name,
      commit: comment.commit_id,
    });

    // Sync commit comments for the repo
    const [owner, repo] = repository.full_name.split('/');
    const comments = await this.client.listCommitComments(owner, repo);
    await this.db.upsertCommitComments(comments);
  }
}
