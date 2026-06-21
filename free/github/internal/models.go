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
