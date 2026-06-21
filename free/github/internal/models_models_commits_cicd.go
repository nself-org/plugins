package internal

import (
	"encoding/json"
	"time"
)

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
