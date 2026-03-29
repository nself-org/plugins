package internal

import (
	"encoding/json"
	"time"
)

// Organization represents a GitHub organization record (np_github_organizations).
type Organization struct {
	ID                int64            `json:"id"`
	SourceAccountID   string           `json:"source_account_id"`
	NodeID            string           `json:"node_id"`
	Login             string           `json:"login"`
	Name              *string          `json:"name"`
	Description       *string          `json:"description"`
	Company           *string          `json:"company"`
	Blog              *string          `json:"blog"`
	Location          *string          `json:"location"`
	Email             *string          `json:"email"`
	TwitterUsername   *string          `json:"twitter_username"`
	IsVerified        bool             `json:"is_verified"`
	HTMLURL           string           `json:"html_url"`
	AvatarURL         string           `json:"avatar_url"`
	PublicRepos       int              `json:"public_repos"`
	PublicGists       int              `json:"public_gists"`
	Followers         int              `json:"followers"`
	Following         int              `json:"following"`
	Type              string           `json:"type"`
	TotalPrivateRepos *int             `json:"total_private_repos"`
	OwnedPrivateRepos *int             `json:"owned_private_repos"`
	Plan              *json.RawMessage `json:"plan"`
	CreatedAt         *time.Time       `json:"created_at"`
	UpdatedAt         *time.Time       `json:"updated_at"`
	SyncedAt          *time.Time       `json:"synced_at"`
}

// Repository represents a GitHub repository record (np_github_repositories).
type Repository struct {
	ID               int64            `json:"id"`
	SourceAccountID  string           `json:"source_account_id"`
	NodeID           string           `json:"node_id"`
	Name             string           `json:"name"`
	FullName         string           `json:"full_name"`
	OwnerLogin       string           `json:"owner_login"`
	OwnerType        *string          `json:"owner_type"`
	Private          bool             `json:"private"`
	Description      *string          `json:"description"`
	Fork             bool             `json:"fork"`
	URL              *string          `json:"url"`
	HTMLURL          *string          `json:"html_url"`
	CloneURL         *string          `json:"clone_url"`
	SSHURL           *string          `json:"ssh_url"`
	Homepage         *string          `json:"homepage"`
	Language         *string          `json:"language"`
	Languages        *json.RawMessage `json:"languages"`
	DefaultBranch    string           `json:"default_branch"`
	Size             int              `json:"size"`
	StargazersCount  int              `json:"stargazers_count"`
	WatchersCount    int              `json:"watchers_count"`
	ForksCount       int              `json:"forks_count"`
	OpenIssuesCount  int              `json:"open_issues_count"`
	Topics           *json.RawMessage `json:"topics"`
	Visibility       string           `json:"visibility"`
	Archived         bool             `json:"archived"`
	Disabled         bool             `json:"disabled"`
	HasIssues        bool             `json:"has_issues"`
	HasProjects      bool             `json:"has_projects"`
	HasWiki          bool             `json:"has_wiki"`
	HasPages         bool             `json:"has_pages"`
	HasDownloads     bool             `json:"has_downloads"`
	HasDiscussions   bool             `json:"has_discussions"`
	AllowForking     bool             `json:"allow_forking"`
	IsTemplate       bool             `json:"is_template"`
	License          *json.RawMessage `json:"license"`
	PushedAt         *time.Time       `json:"pushed_at"`
	CreatedAt        *time.Time       `json:"created_at"`
	UpdatedAt        *time.Time       `json:"updated_at"`
	SyncedAt         *time.Time       `json:"synced_at"`
}

// Branch represents a GitHub branch record (np_github_branches).
type Branch struct {
	ID                string           `json:"id"`
	SourceAccountID   string           `json:"source_account_id"`
	RepoID            *int64           `json:"repo_id"`
	Name              string           `json:"name"`
	SHA               string           `json:"sha"`
	Protected         bool             `json:"protected"`
	ProtectionEnabled bool             `json:"protection_enabled"`
	Protection        *json.RawMessage `json:"protection"`
	UpdatedAt         *time.Time       `json:"updated_at"`
}

// Issue represents a GitHub issue record (np_github_issues).
type Issue struct {
	ID              int64            `json:"id"`
	SourceAccountID string           `json:"source_account_id"`
	NodeID          *string          `json:"node_id"`
	RepoID          *int64           `json:"repo_id"`
	Number          int              `json:"number"`
	Title           string           `json:"title"`
	Body            *string          `json:"body"`
	State           string           `json:"state"`
	StateReason     *string          `json:"state_reason"`
	Locked          bool             `json:"locked"`
	UserLogin       *string          `json:"user_login"`
	UserID          *int64           `json:"user_id"`
	Labels          *json.RawMessage `json:"labels"`
	Assignees       *json.RawMessage `json:"assignees"`
	Milestone       *json.RawMessage `json:"milestone"`
	Comments        int              `json:"comments"`
	Reactions       *json.RawMessage `json:"reactions"`
	HTMLURL         *string          `json:"html_url"`
	ClosedAt        *time.Time       `json:"closed_at"`
	ClosedByLogin   *string          `json:"closed_by_login"`
	CreatedAt       *time.Time       `json:"created_at"`
	UpdatedAt       *time.Time       `json:"updated_at"`
	SyncedAt        *time.Time       `json:"synced_at"`
}

// PullRequest represents a GitHub pull request record (np_github_pull_requests).
type PullRequest struct {
	ID              int64            `json:"id"`
	SourceAccountID string           `json:"source_account_id"`
	NodeID          *string          `json:"node_id"`
	RepoID          *int64           `json:"repo_id"`
	Number          int              `json:"number"`
	Title           string           `json:"title"`
	Body            *string          `json:"body"`
	State           string           `json:"state"`
	Draft           bool             `json:"draft"`
	Locked          bool             `json:"locked"`
	UserLogin       *string          `json:"user_login"`
	UserID          *int64           `json:"user_id"`
	HeadRef         *string          `json:"head_ref"`
	HeadSHA         *string          `json:"head_sha"`
	HeadRepoID      *int64           `json:"head_repo_id"`
	BaseRef         *string          `json:"base_ref"`
	BaseSHA         *string          `json:"base_sha"`
	Merged          bool             `json:"merged"`
	Mergeable       *bool            `json:"mergeable"`
	MergeableState  *string          `json:"mergeable_state"`
	MergedByLogin   *string          `json:"merged_by_login"`
	MergedAt        *time.Time       `json:"merged_at"`
	MergeCommitSHA  *string          `json:"merge_commit_sha"`
	Labels          *json.RawMessage `json:"labels"`
	Assignees       *json.RawMessage `json:"assignees"`
	Reviewers       *json.RawMessage `json:"reviewers"`
	MilestonePR     *json.RawMessage `json:"milestone"`
	CommentCount    int              `json:"comments"`
	ReviewComments  int              `json:"review_comments"`
	Commits         int              `json:"commits"`
	Additions       int              `json:"additions"`
	Deletions       int              `json:"deletions"`
	ChangedFiles    int              `json:"changed_files"`
	HTMLURL         *string          `json:"html_url"`
	DiffURL         *string          `json:"diff_url"`
	ClosedAt        *time.Time       `json:"closed_at"`
	CreatedAt       *time.Time       `json:"created_at"`
	UpdatedAt       *time.Time       `json:"updated_at"`
	SyncedAt        *time.Time       `json:"synced_at"`
}

// PRReview represents a GitHub pull request review record (np_github_pr_reviews).
type PRReview struct {
	ID                int64      `json:"id"`
	SourceAccountID   string     `json:"source_account_id"`
	NodeID            *string    `json:"node_id"`
	RepoID            *int64     `json:"repo_id"`
	PullRequestID     *int64     `json:"pull_request_id"`
	PullRequestNumber int        `json:"pull_request_number"`
	UserLogin         *string    `json:"user_login"`
	UserID            *int64     `json:"user_id"`
	Body              *string    `json:"body"`
	State             string     `json:"state"`
	HTMLURL           *string    `json:"html_url"`
	CommitID          *string    `json:"commit_id"`
	SubmittedAt       *time.Time `json:"submitted_at"`
	SyncedAt          *time.Time `json:"synced_at"`
}

// IssueComment represents a GitHub issue comment record (np_github_issue_comments).
type IssueComment struct {
	ID                int64            `json:"id"`
	SourceAccountID   string           `json:"source_account_id"`
	NodeID            *string          `json:"node_id"`
	RepoID            *int64           `json:"repo_id"`
	IssueNumber       int              `json:"issue_number"`
	IssueID           *int64           `json:"issue_id"`
	PullRequestNumber *int             `json:"pull_request_number"`
	UserLogin         *string          `json:"user_login"`
	UserID            *int64           `json:"user_id"`
	Body              string           `json:"body"`
	Reactions         *json.RawMessage `json:"reactions"`
	HTMLURL           *string          `json:"html_url"`
	CreatedAt         *time.Time       `json:"created_at"`
	UpdatedAt         *time.Time       `json:"updated_at"`
	SyncedAt          *time.Time       `json:"synced_at"`
}

// PRReviewComment represents a GitHub pull request review comment (np_github_pr_review_comments).
type PRReviewComment struct {
	ID                int64            `json:"id"`
	SourceAccountID   string           `json:"source_account_id"`
	NodeID            *string          `json:"node_id"`
	RepoID            *int64           `json:"repo_id"`
	PullRequestID     *int64           `json:"pull_request_id"`
	PullRequestNumber int              `json:"pull_request_number"`
	ReviewID          *int64           `json:"review_id"`
	UserLogin         *string          `json:"user_login"`
	UserID            *int64           `json:"user_id"`
	Body              string           `json:"body"`
	Path              *string          `json:"path"`
	Position          *int             `json:"position"`
	OriginalPosition  *int             `json:"original_position"`
	DiffHunk          *string          `json:"diff_hunk"`
	CommitID          *string          `json:"commit_id"`
	OriginalCommitID  *string          `json:"original_commit_id"`
	InReplyToID       *int64           `json:"in_reply_to_id"`
	Reactions         *json.RawMessage `json:"reactions"`
	HTMLURL           *string          `json:"html_url"`
	CreatedAt         *time.Time       `json:"created_at"`
	UpdatedAt         *time.Time       `json:"updated_at"`
	SyncedAt          *time.Time       `json:"synced_at"`
}

// CommitComment represents a GitHub commit comment record (np_github_commit_comments).
type CommitComment struct {
	ID              int64            `json:"id"`
	SourceAccountID string           `json:"source_account_id"`
	NodeID          *string          `json:"node_id"`
	RepoID          *int64           `json:"repo_id"`
	CommitSHA       string           `json:"commit_sha"`
	UserLogin       *string          `json:"user_login"`
	UserID          *int64           `json:"user_id"`
	Body            string           `json:"body"`
	Path            *string          `json:"path"`
	Position        *int             `json:"position"`
	Line            *int             `json:"line"`
	Reactions       *json.RawMessage `json:"reactions"`
	HTMLURL         *string          `json:"html_url"`
	CreatedAt       *time.Time       `json:"created_at"`
	UpdatedAt       *time.Time       `json:"updated_at"`
	SyncedAt        *time.Time       `json:"synced_at"`
}

// Commit represents a GitHub commit record (np_github_commits).
type Commit struct {
	SHA                string           `json:"sha"`
	SourceAccountID    string           `json:"source_account_id"`
	NodeID             *string          `json:"node_id"`
	RepoID             *int64           `json:"repo_id"`
	Message            *string          `json:"message"`
	AuthorName         *string          `json:"author_name"`
	AuthorEmail        *string          `json:"author_email"`
	AuthorLogin        *string          `json:"author_login"`
	AuthorDate         *time.Time       `json:"author_date"`
	CommitterName      *string          `json:"committer_name"`
	CommitterEmail     *string          `json:"committer_email"`
	CommitterLogin     *string          `json:"committer_login"`
	CommitterDate      *time.Time       `json:"committer_date"`
	TreeSHA            *string          `json:"tree_sha"`
	Parents            *json.RawMessage `json:"parents"`
	CommitAdditions    int              `json:"additions"`
	CommitDeletions    int              `json:"deletions"`
	Total              int              `json:"total"`
	HTMLURL            *string          `json:"html_url"`
	Verified           bool             `json:"verified"`
	VerificationReason *string          `json:"verification_reason"`
	SyncedAt           *time.Time       `json:"synced_at"`
}

// Release represents a GitHub release record (np_github_releases).
type Release struct {
	ID              int64            `json:"id"`
	SourceAccountID string           `json:"source_account_id"`
	NodeID          *string          `json:"node_id"`
	RepoID          *int64           `json:"repo_id"`
	TagName         string           `json:"tag_name"`
	TargetCommitish *string          `json:"target_commitish"`
	Name            *string          `json:"name"`
	Body            *string          `json:"body"`
	Draft           bool             `json:"draft"`
	Prerelease      bool             `json:"prerelease"`
	AuthorLogin     *string          `json:"author_login"`
	HTMLURL         *string          `json:"html_url"`
	TarballURL      *string          `json:"tarball_url"`
	ZipballURL      *string          `json:"zipball_url"`
	Assets          *json.RawMessage `json:"assets"`
	CreatedAt       *time.Time       `json:"created_at"`
	PublishedAt     *time.Time       `json:"published_at"`
	SyncedAt        *time.Time       `json:"synced_at"`
}

// Tag represents a GitHub tag record (np_github_tags).
type Tag struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	RepoID          *int64     `json:"repo_id"`
	Name            string     `json:"name"`
	SHA             string     `json:"sha"`
	Message         *string    `json:"message"`
	TaggerName      *string    `json:"tagger_name"`
	TaggerEmail     *string    `json:"tagger_email"`
	TaggerDate      *time.Time `json:"tagger_date"`
	ZipballURL      *string    `json:"zipball_url"`
	TarballURL      *string    `json:"tarball_url"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Milestone represents a GitHub milestone record (np_github_milestones).
type Milestone struct {
	ID              int64      `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	NodeID          *string    `json:"node_id"`
	RepoID          *int64     `json:"repo_id"`
	Number          int        `json:"number"`
	Title           string     `json:"title"`
	Description     *string    `json:"description"`
	State           string     `json:"state"`
	CreatorLogin    *string    `json:"creator_login"`
	OpenIssues      int        `json:"open_issues"`
	ClosedIssues    int        `json:"closed_issues"`
	HTMLURL         *string    `json:"html_url"`
	DueOn           *time.Time `json:"due_on"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	ClosedAt        *time.Time `json:"closed_at"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Label represents a GitHub label record (np_github_labels).
type Label struct {
	ID              int64      `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	NodeID          *string    `json:"node_id"`
	RepoID          *int64     `json:"repo_id"`
	Name            string     `json:"name"`
	Color           string     `json:"color"`
	Description     *string    `json:"description"`
	IsDefault       bool       `json:"is_default"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Workflow represents a GitHub workflow record (np_github_workflows).
type Workflow struct {
	ID              int64      `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	NodeID          *string    `json:"node_id"`
	RepoID          *int64     `json:"repo_id"`
	Name            string     `json:"name"`
	Path            string     `json:"path"`
	State           string     `json:"state"`
	BadgeURL        *string    `json:"badge_url"`
	HTMLURL         *string    `json:"html_url"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// WorkflowRun represents a GitHub workflow run record (np_github_workflow_runs).
type WorkflowRun struct {
	ID                    int64      `json:"id"`
	SourceAccountID       string     `json:"source_account_id"`
	NodeID                *string    `json:"node_id"`
	RepoID                *int64     `json:"repo_id"`
	WorkflowID            *int64     `json:"workflow_id"`
	WorkflowName          *string    `json:"workflow_name"`
	Name                  *string    `json:"name"`
	HeadBranch            *string    `json:"head_branch"`
	HeadSHA               *string    `json:"head_sha"`
	RunNumber             *int       `json:"run_number"`
	RunAttempt            *int       `json:"run_attempt"`
	Event                 *string    `json:"event"`
	Status                *string    `json:"status"`
	Conclusion            *string    `json:"conclusion"`
	ActorLogin            *string    `json:"actor_login"`
	TriggeringActorLogin  *string    `json:"triggering_actor_login"`
	HTMLURL               *string    `json:"html_url"`
	JobsURL               *string    `json:"jobs_url"`
	LogsURL               *string    `json:"logs_url"`
	RunStartedAt          *time.Time `json:"run_started_at"`
	CreatedAt             *time.Time `json:"created_at"`
	UpdatedAt             *time.Time `json:"updated_at"`
	SyncedAt              *time.Time `json:"synced_at"`
}

// WorkflowJob represents a GitHub workflow job record (np_github_workflow_jobs).
type WorkflowJob struct {
	ID              int64            `json:"id"`
	SourceAccountID string           `json:"source_account_id"`
	NodeID          *string          `json:"node_id"`
	RepoID          *int64           `json:"repo_id"`
	RunID           *int64           `json:"run_id"`
	RunAttempt      *int             `json:"run_attempt"`
	WorkflowName    *string          `json:"workflow_name"`
	Name            string           `json:"name"`
	Status          string           `json:"status"`
	Conclusion      *string          `json:"conclusion"`
	HeadSHA         *string          `json:"head_sha"`
	HTMLURL         *string          `json:"html_url"`
	RunnerID        *int64           `json:"runner_id"`
	RunnerName      *string          `json:"runner_name"`
	RunnerGroupID   *int64           `json:"runner_group_id"`
	RunnerGroupName *string          `json:"runner_group_name"`
	JobLabels       *json.RawMessage `json:"labels"`
	Steps           *json.RawMessage `json:"steps"`
	StartedAt       *time.Time       `json:"started_at"`
	CompletedAt     *time.Time       `json:"completed_at"`
	SyncedAt        *time.Time       `json:"synced_at"`
}

// CheckSuite represents a GitHub check suite record (np_github_check_suites).
type CheckSuite struct {
	ID              int64            `json:"id"`
	SourceAccountID string           `json:"source_account_id"`
	NodeID          *string          `json:"node_id"`
	RepoID          *int64           `json:"repo_id"`
	HeadBranch      *string          `json:"head_branch"`
	HeadSHA         string           `json:"head_sha"`
	Status          string           `json:"status"`
	Conclusion      *string          `json:"conclusion"`
	AppID           *int64           `json:"app_id"`
	AppSlug         *string          `json:"app_slug"`
	PullRequests    *json.RawMessage `json:"pull_requests"`
	BeforeSHA       *string          `json:"before_sha"`
	AfterSHA        *string          `json:"after_sha"`
	CreatedAt       *time.Time       `json:"created_at"`
	UpdatedAt       *time.Time       `json:"updated_at"`
	SyncedAt        *time.Time       `json:"synced_at"`
}

// CheckRun represents a GitHub check run record (np_github_check_runs).
type CheckRun struct {
	ID              int64            `json:"id"`
	SourceAccountID string           `json:"source_account_id"`
	NodeID          *string          `json:"node_id"`
	RepoID          *int64           `json:"repo_id"`
	CheckSuiteID    *int64           `json:"check_suite_id"`
	HeadSHA         string           `json:"head_sha"`
	Name            string           `json:"name"`
	Status          string           `json:"status"`
	Conclusion      *string          `json:"conclusion"`
	ExternalID      *string          `json:"external_id"`
	HTMLURL         *string          `json:"html_url"`
	DetailsURL      *string          `json:"details_url"`
	AppID           *int64           `json:"app_id"`
	AppSlug         *string          `json:"app_slug"`
	Output          *json.RawMessage `json:"output"`
	PullRequests    *json.RawMessage `json:"pull_requests"`
	StartedAt       *time.Time       `json:"started_at"`
	CompletedAt     *time.Time       `json:"completed_at"`
	SyncedAt        *time.Time       `json:"synced_at"`
}

// Deployment represents a GitHub deployment record (np_github_deployments).
type Deployment struct {
	ID                   int64            `json:"id"`
	SourceAccountID      string           `json:"source_account_id"`
	NodeID               *string          `json:"node_id"`
	RepoID               *int64           `json:"repo_id"`
	SHA                  *string          `json:"sha"`
	Ref                  *string          `json:"ref"`
	Task                 *string          `json:"task"`
	Environment          *string          `json:"environment"`
	Description          *string          `json:"description"`
	CreatorLogin         *string          `json:"creator_login"`
	Statuses             *json.RawMessage `json:"statuses"`
	CurrentStatus        *string          `json:"current_status"`
	ProductionEnvironment bool            `json:"production_environment"`
	TransientEnvironment  bool            `json:"transient_environment"`
	Payload              *json.RawMessage `json:"payload"`
	CreatedAt            *time.Time       `json:"created_at"`
	UpdatedAt            *time.Time       `json:"updated_at"`
	SyncedAt             *time.Time       `json:"synced_at"`
}

// Team represents a GitHub team record (np_github_teams).
type Team struct {
	ID              int64      `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	NodeID          *string    `json:"node_id"`
	OrgLogin        string     `json:"org_login"`
	Name            string     `json:"name"`
	Slug            string     `json:"slug"`
	Description     *string    `json:"description"`
	Privacy         *string    `json:"privacy"`
	Permission      *string    `json:"permission"`
	ParentID        *int64     `json:"parent_id"`
	MembersCount    int        `json:"members_count"`
	ReposCount      int        `json:"repos_count"`
	HTMLURL         *string    `json:"html_url"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
	SyncedAt        *time.Time `json:"synced_at"`
}

// Collaborator represents a GitHub collaborator record (np_github_collaborators).
type Collaborator struct {
	ID              int64            `json:"id"`
	SourceAccountID string           `json:"source_account_id"`
	RepoID          *int64           `json:"repo_id"`
	Login           string           `json:"login"`
	Type            *string          `json:"type"`
	SiteAdmin       bool             `json:"site_admin"`
	Permissions     *json.RawMessage `json:"permissions"`
	RoleName        *string          `json:"role_name"`
	SyncedAt        *time.Time       `json:"synced_at"`
}

// WebhookEvent represents a stored webhook event (np_github_webhook_events).
type WebhookEvent struct {
	ID              string           `json:"id"`
	SourceAccountID string           `json:"source_account_id"`
	Event           string           `json:"event"`
	Action          *string          `json:"action"`
	RepoID          *int64           `json:"repo_id"`
	RepoFullName    *string          `json:"repo_full_name"`
	SenderLogin     *string          `json:"sender_login"`
	Data            *json.RawMessage `json:"data"`
	Processed       bool             `json:"processed"`
	ProcessedAt     *time.Time       `json:"processed_at"`
	Error           *string          `json:"error"`
	ReceivedAt      *time.Time       `json:"received_at"`
}

// SyncStats holds counts of all synced entities.
type SyncStats struct {
	Repositories     int        `json:"repositories"`
	Branches         int        `json:"branches"`
	Issues           int        `json:"issues"`
	PullRequests     int        `json:"pullRequests"`
	PRReviews        int        `json:"prReviews"`
	IssueComments    int        `json:"issueComments"`
	PRReviewComments int        `json:"prReviewComments"`
	CommitComments   int        `json:"commitComments"`
	Commits          int        `json:"commits"`
	Releases         int        `json:"releases"`
	Tags             int        `json:"tags"`
	Milestones       int        `json:"milestones"`
	Labels           int        `json:"labels"`
	Workflows        int        `json:"workflows"`
	WorkflowRuns     int        `json:"workflowRuns"`
	WorkflowJobs     int        `json:"workflowJobs"`
	CheckSuites      int        `json:"checkSuites"`
	CheckRuns        int        `json:"checkRuns"`
	Deployments      int        `json:"deployments"`
	Teams            int        `json:"teams"`
	Collaborators    int        `json:"collaborators"`
	LastSyncedAt     *time.Time `json:"lastSyncedAt"`
}

// SyncResult is the response from a sync operation.
type SyncResult struct {
	Success  bool      `json:"success"`
	Stats    SyncStats `json:"stats"`
	Errors   []string  `json:"errors"`
	Duration int64     `json:"duration"`
}

// SyncRequest represents the request body for /sync endpoint.
type SyncRequest struct {
	Resources []string `json:"resources"`
	Repos     []string `json:"repos"`
	Since     string   `json:"since"`
}

// ListResponse wraps a paginated list response.
type ListResponse struct {
	Data   interface{} `json:"data"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}
