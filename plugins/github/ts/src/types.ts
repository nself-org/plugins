/**
 * GitHub Plugin Types
 * Complete type definitions for all GitHub resources
 */

export interface GitHubPluginConfig {
  token: string;
  webhookSecret?: string;
  org?: string;
  repos?: string[];
  port: number;
  host: string;
  syncInterval?: number;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Repository Record
// =============================================================================

export interface GitHubRepositoryRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  name: string;
  full_name: string;
  owner_login: string;
  owner_type: string;
  private: boolean;
  description: string | null;
  fork: boolean;
  url: string;
  html_url: string;
  clone_url: string;
  ssh_url: string;
  homepage: string | null;
  language: string | null;
  languages: Record<string, number>;
  default_branch: string;
  size: number;
  stargazers_count: number;
  watchers_count: number;
  forks_count: number;
  open_issues_count: number;
  topics: string[];
  visibility: string;
  archived: boolean;
  disabled: boolean;
  has_issues: boolean;
  has_projects: boolean;
  has_wiki: boolean;
  has_pages: boolean;
  has_downloads: boolean;
  has_discussions: boolean;
  allow_forking: boolean;
  is_template: boolean;
  web_commit_signoff_required: boolean;
  license: GitHubLicense | null;
  pushed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface GitHubLicense {
  key: string;
  name: string;
  spdx_id: string | null;
  url: string | null;
}

// =============================================================================
// Branch Record
// =============================================================================

export interface GitHubBranchRecord {
  id: string; // composite: repo_id + name
  source_account_id: string;
  repo_id: number;
  name: string;
  sha: string;
  protected: boolean;
  protection_enabled: boolean;
  protection: GitHubBranchProtection | null;
  updated_at: Date;
}

export interface GitHubBranchProtection {
  required_status_checks: {
    strict: boolean;
    contexts: string[];
  } | null;
  required_pull_request_reviews: {
    required_approving_review_count: number;
    dismiss_stale_reviews: boolean;
    require_code_owner_reviews: boolean;
  } | null;
  enforce_admins: boolean;
  required_signatures: boolean;
  allow_force_pushes: boolean;
  allow_deletions: boolean;
}

// =============================================================================
// Issue Record
// =============================================================================

export interface GitHubIssueRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  number: number;
  title: string;
  body: string | null;
  state: string;
  state_reason: string | null;
  locked: boolean;
  user_login: string;
  user_id: number;
  labels: GitHubLabel[];
  assignees: GitHubUser[];
  milestone: GitHubMilestone | null;
  comments: number;
  reactions: GitHubReactions;
  html_url: string;
  closed_at: Date | null;
  closed_by_login: string | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Pull Request Record
// =============================================================================

export interface GitHubPullRequestRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  number: number;
  title: string;
  body: string | null;
  state: string;
  draft: boolean;
  locked: boolean;
  user_login: string;
  user_id: number;
  head_ref: string;
  head_sha: string;
  head_repo_id: number | null;
  base_ref: string;
  base_sha: string;
  merged: boolean;
  mergeable: boolean | null;
  mergeable_state: string | null;
  merged_by_login: string | null;
  merged_at: Date | null;
  merge_commit_sha: string | null;
  labels: GitHubLabel[];
  assignees: GitHubUser[];
  reviewers: GitHubUser[];
  milestone: GitHubMilestone | null;
  comments: number;
  review_comments: number;
  commits: number;
  additions: number;
  deletions: number;
  changed_files: number;
  html_url: string;
  diff_url: string;
  closed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// PR Review Record
// =============================================================================

export interface GitHubPullRequestReviewRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  pull_request_id: number;
  pull_request_number: number;
  user_login: string;
  user_id: number;
  body: string | null;
  state: string; // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, PENDING
  html_url: string;
  commit_id: string;
  submitted_at: Date | null;
}

// =============================================================================
// Comment Records
// =============================================================================

export interface GitHubIssueCommentRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  issue_number: number;
  issue_id: number | null;
  pull_request_number: number | null;
  user_login: string;
  user_id: number;
  body: string;
  reactions: GitHubReactions;
  html_url: string;
  created_at: Date;
  updated_at: Date;
}

export interface GitHubPullRequestReviewCommentRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  pull_request_id: number;
  pull_request_number: number;
  review_id: number | null;
  user_login: string;
  user_id: number;
  body: string;
  path: string;
  position: number | null;
  original_position: number | null;
  diff_hunk: string;
  commit_id: string;
  original_commit_id: string;
  in_reply_to_id: number | null;
  reactions: GitHubReactions;
  html_url: string;
  created_at: Date;
  updated_at: Date;
}

export interface GitHubCommitCommentRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  commit_sha: string;
  user_login: string;
  user_id: number;
  body: string;
  path: string | null;
  position: number | null;
  line: number | null;
  reactions: GitHubReactions;
  html_url: string;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Commit Record
// =============================================================================

export interface GitHubCommitRecord {
  sha: string;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  message: string;
  author_name: string;
  author_email: string;
  author_login: string | null;
  author_date: Date;
  committer_name: string;
  committer_email: string;
  committer_login: string | null;
  committer_date: Date;
  tree_sha: string;
  parents: string[];
  additions: number;
  deletions: number;
  total: number;
  html_url: string;
  verified: boolean;
  verification_reason: string | null;
}

// =============================================================================
// Release Record
// =============================================================================

export interface GitHubReleaseRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  tag_name: string;
  target_commitish: string;
  name: string | null;
  body: string | null;
  draft: boolean;
  prerelease: boolean;
  author_login: string;
  html_url: string;
  tarball_url: string | null;
  zipball_url: string | null;
  assets: GitHubReleaseAsset[];
  created_at: Date;
  published_at: Date | null;
}

// =============================================================================
// Tag Record
// =============================================================================

export interface GitHubTagRecord {
  id: string; // composite: repo_id + name
  source_account_id: string;
  repo_id: number;
  name: string;
  sha: string;
  message: string | null;
  tagger_name: string | null;
  tagger_email: string | null;
  tagger_date: Date | null;
  zipball_url: string;
  tarball_url: string;
}

// =============================================================================
// Milestone Record
// =============================================================================

export interface GitHubMilestoneRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  number: number;
  title: string;
  description: string | null;
  state: string;
  creator_login: string;
  open_issues: number;
  closed_issues: number;
  html_url: string;
  due_on: Date | null;
  created_at: Date;
  updated_at: Date;
  closed_at: Date | null;
}

// =============================================================================
// Label Record
// =============================================================================

export interface GitHubLabelRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  name: string;
  color: string;
  description: string | null;
  default: boolean;
}

// =============================================================================
// Workflow and Workflow Run Records
// =============================================================================

export interface GitHubWorkflowRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  name: string;
  path: string;
  state: string;
  badge_url: string;
  html_url: string;
  created_at: Date;
  updated_at: Date;
}

export interface GitHubWorkflowRunRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  workflow_id: number;
  workflow_name: string;
  name: string;
  head_branch: string;
  head_sha: string;
  run_number: number;
  run_attempt: number;
  event: string;
  status: string | null;
  conclusion: string | null;
  actor_login: string;
  triggering_actor_login: string;
  html_url: string;
  jobs_url: string;
  logs_url: string;
  run_started_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface GitHubWorkflowJobRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  run_id: number;
  run_attempt: number;
  workflow_name: string;
  name: string;
  status: string;
  conclusion: string | null;
  head_sha: string;
  html_url: string;
  runner_id: number | null;
  runner_name: string | null;
  runner_group_id: number | null;
  runner_group_name: string | null;
  labels: string[];
  steps: GitHubWorkflowStep[];
  started_at: Date | null;
  completed_at: Date | null;
}

export interface GitHubWorkflowStep {
  name: string;
  status: string;
  conclusion: string | null;
  number: number;
  started_at: string | null;
  completed_at: string | null;
}

// =============================================================================
// Check Run and Check Suite Records
// =============================================================================

export interface GitHubCheckSuiteRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  head_branch: string | null;
  head_sha: string;
  status: string;
  conclusion: string | null;
  app_id: number | null;
  app_slug: string | null;
  pull_requests: number[];
  before: string | null;
  after: string | null;
  created_at: Date;
  updated_at: Date;
}

export interface GitHubCheckRunRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  check_suite_id: number;
  head_sha: string;
  name: string;
  status: string;
  conclusion: string | null;
  external_id: string | null;
  html_url: string;
  details_url: string | null;
  app_id: number | null;
  app_slug: string | null;
  output: GitHubCheckRunOutput | null;
  pull_requests: number[];
  started_at: Date | null;
  completed_at: Date | null;
}

export interface GitHubCheckRunOutput {
  title: string | null;
  summary: string | null;
  text: string | null;
  annotations_count: number;
}

// =============================================================================
// Deployment Record
// =============================================================================

export interface GitHubDeploymentRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  repo_id: number;
  sha: string;
  ref: string;
  task: string;
  environment: string;
  description: string | null;
  creator_login: string;
  statuses: GitHubDeploymentStatus[];
  current_status: string | null;
  production_environment: boolean;
  transient_environment: boolean;
  payload: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

// =============================================================================
// Team and Collaborator Records
// =============================================================================

export interface GitHubTeamRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  org_login: string;
  name: string;
  slug: string;
  description: string | null;
  privacy: string;
  permission: string;
  parent_id: number | null;
  members_count: number;
  repos_count: number;
  html_url: string;
  created_at: Date;
  updated_at: Date;
}

export interface GitHubCollaboratorRecord {
  id: number;
  source_account_id: string;
  repo_id: number;
  login: string;
  type: string;
  site_admin: boolean;
  permissions: GitHubPermissions;
  role_name: string;
}

export interface GitHubPermissions {
  admin: boolean;
  maintain: boolean;
  push: boolean;
  triage: boolean;
  pull: boolean;
}

// =============================================================================
// Organization Record
// =============================================================================

export interface GitHubOrganizationRecord {
  id: number;
  source_account_id: string;
  node_id: string;
  login: string;
  name: string | null;
  description: string | null;
  company: string | null;
  blog: string | null;
  location: string | null;
  email: string | null;
  twitter_username: string | null;
  is_verified: boolean;
  html_url: string;
  avatar_url: string;
  public_repos: number;
  public_gists: number;
  followers: number;
  following: number;
  type: string;
  total_private_repos: number | null;
  owned_private_repos: number | null;
  plan: GitHubPlan | null;
  created_at: Date;
  updated_at: Date;
}

export interface GitHubPlan {
  name: string;
  space: number;
  private_repos: number;
  filled_seats: number | null;
  seats: number | null;
}

// =============================================================================
// Webhook Event Record
// =============================================================================

export interface GitHubWebhookEventRecord {
  id: string;
  source_account_id: string;
  event: string;
  action: string | null;
  repo_id: number | null;
  repo_full_name: string | null;
  sender_login: string | null;
  data: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  received_at: Date;
}

// =============================================================================
// Helper Types
// =============================================================================

export interface GitHubLabel {
  id: number;
  name: string;
  color: string;
  description: string | null;
}

export interface GitHubUser {
  id: number;
  login: string;
  type: string;
  avatar_url: string;
}

export interface GitHubMilestone {
  id: number;
  number: number;
  title: string;
  description: string | null;
  state: string;
  due_on: string | null;
}

export interface GitHubReactions {
  total_count: number;
  '+1': number;
  '-1': number;
  laugh: number;
  hooray: number;
  confused: number;
  heart: number;
  rocket: number;
  eyes: number;
}

export interface GitHubReleaseAsset {
  id: number;
  name: string;
  content_type: string;
  size: number;
  download_count: number;
  browser_download_url: string;
}

export interface GitHubDeploymentStatus {
  id: number;
  state: string;
  description: string | null;
  environment_url: string | null;
  created_at: string;
}

// =============================================================================
// Sync Types
// =============================================================================

export const ALL_RESOURCES = [
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
] as const;

export type SyncResource = (typeof ALL_RESOURCES)[number];

export interface SyncStats {
  repositories: number;
  branches: number;
  issues: number;
  pullRequests: number;
  prReviews: number;
  issueComments: number;
  prReviewComments: number;
  commitComments: number;
  commits: number;
  releases: number;
  tags: number;
  milestones: number;
  labels: number;
  workflows: number;
  workflowRuns: number;
  workflowJobs: number;
  checkSuites: number;
  checkRuns: number;
  deployments: number;
  teams: number;
  collaborators: number;
  lastSyncedAt?: Date | null;
}

export interface SyncOptions {
  incremental?: boolean;
  since?: Date;
  repos?: string[];
  resources?: SyncResource[];
  limit?: number;
}
