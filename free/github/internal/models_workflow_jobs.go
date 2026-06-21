package internal

import (
	"encoding/json"
	"time"
)

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
