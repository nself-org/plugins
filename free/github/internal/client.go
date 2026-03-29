package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	githubBaseURL  = "https://api.github.com"
	githubAccept   = "application/vnd.github+json"
	githubVersion  = "2022-11-28"
	defaultPerPage = 100
	maxPages       = 10
)

// GitHubClient wraps the GitHub REST API.
type GitHubClient struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// NewGitHubClient creates a new GitHub API client.
func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		token:   token,
		baseURL: githubBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest performs an authenticated GET request to the GitHub API.
func (c *GitHubClient) doRequest(path string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", githubAccept)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", githubVersion)
	return c.httpClient.Do(req)
}

// fetchJSON performs a GET request and decodes the JSON response into v.
func (c *GitHubClient) fetchJSON(path string, v interface{}) error {
	resp, err := c.doRequest(path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api %s: status %d: %s", path, resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(v)
}

// fetchAllPages fetches paginated results and collects them into a slice.
func fetchAllPages[T any](c *GitHubClient, path string) ([]T, error) {
	var all []T
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	url := path + sep + "per_page=" + strconv.Itoa(defaultPerPage)

	for page := 1; page <= maxPages; page++ {
		pageURL := url + "&page=" + strconv.Itoa(page)
		resp, err := c.doRequest(pageURL)
		if err != nil {
			return all, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return all, fmt.Errorf("github api %s: status %d: %s", pageURL, resp.StatusCode, string(body))
		}

		var items []T
		if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
			resp.Body.Close()
			return all, fmt.Errorf("decoding response: %w", err)
		}
		resp.Body.Close()

		all = append(all, items...)

		if len(items) < defaultPerPage {
			break
		}

		if !hasNextPage(resp.Header.Get("Link")) {
			break
		}
	}
	return all, nil
}

// hasNextPage checks the Link header for a rel="next" link.
func hasNextPage(linkHeader string) bool {
	if linkHeader == "" {
		return false
	}
	return strings.Contains(linkHeader, `rel="next"`)
}

// --- GitHub API response types -----------------------------------------------

type ghOwner struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	AvatarURL string `json:"avatar_url"`
	Type      string `json:"type"`
}

type ghRepo struct {
	ID              int64            `json:"id"`
	NodeID          string           `json:"node_id"`
	Name            string           `json:"name"`
	FullName        string           `json:"full_name"`
	Owner           ghOwner          `json:"owner"`
	Private         bool             `json:"private"`
	Description     *string          `json:"description"`
	Fork            bool             `json:"fork"`
	URL             string           `json:"url"`
	HTMLURL         string           `json:"html_url"`
	CloneURL        string           `json:"clone_url"`
	SSHURL          string           `json:"ssh_url"`
	Homepage        *string          `json:"homepage"`
	Language        *string          `json:"language"`
	DefaultBranch   string           `json:"default_branch"`
	Size            int              `json:"size"`
	StargazersCount int              `json:"stargazers_count"`
	WatchersCount   int              `json:"watchers_count"`
	ForksCount      int              `json:"forks_count"`
	OpenIssuesCount int              `json:"open_issues_count"`
	Topics          json.RawMessage  `json:"topics"`
	Visibility      string           `json:"visibility"`
	Archived        bool             `json:"archived"`
	Disabled        bool             `json:"disabled"`
	HasIssues       bool             `json:"has_issues"`
	HasProjects     bool             `json:"has_projects"`
	HasWiki         bool             `json:"has_wiki"`
	HasPages        bool             `json:"has_pages"`
	HasDownloads    bool             `json:"has_downloads"`
	HasDiscussions  bool             `json:"has_discussions"`
	AllowForking    bool             `json:"allow_forking"`
	IsTemplate      bool             `json:"is_template"`
	License         *json.RawMessage `json:"license"`
	PushedAt        *time.Time       `json:"pushed_at"`
	CreatedAt       *time.Time       `json:"created_at"`
	UpdatedAt       *time.Time       `json:"updated_at"`
}

type ghOrg struct {
	ID              int64            `json:"id"`
	NodeID          string           `json:"node_id"`
	Login           string           `json:"login"`
	Name            *string          `json:"name"`
	Description     *string          `json:"description"`
	Company         *string          `json:"company"`
	Blog            *string          `json:"blog"`
	Location        *string          `json:"location"`
	Email           *string          `json:"email"`
	TwitterUsername *string          `json:"twitter_username"`
	IsVerified      bool             `json:"is_verified"`
	HTMLURL         string           `json:"html_url"`
	AvatarURL       string           `json:"avatar_url"`
	PublicRepos     int              `json:"public_repos"`
	PublicGists     int              `json:"public_gists"`
	Followers       int              `json:"followers"`
	Following       int              `json:"following"`
	Type            string           `json:"type"`
	Plan            *json.RawMessage `json:"plan"`
	CreatedAt       *time.Time       `json:"created_at"`
	UpdatedAt       *time.Time       `json:"updated_at"`
}

type ghBranch struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
	Protected bool `json:"protected"`
}

type ghIssue struct {
	ID          int64            `json:"id"`
	NodeID      string           `json:"node_id"`
	Number      int              `json:"number"`
	Title       string           `json:"title"`
	Body        *string          `json:"body"`
	State       string           `json:"state"`
	StateReason *string          `json:"state_reason"`
	Locked      bool             `json:"locked"`
	User        *ghOwner         `json:"user"`
	Labels      json.RawMessage  `json:"labels"`
	Assignees   json.RawMessage  `json:"assignees"`
	Milestone   *json.RawMessage `json:"milestone"`
	Comments    int              `json:"comments"`
	Reactions   *json.RawMessage `json:"reactions"`
	HTMLURL     string           `json:"html_url"`
	ClosedAt    *time.Time       `json:"closed_at"`
	CreatedAt   *time.Time       `json:"created_at"`
	UpdatedAt   *time.Time       `json:"updated_at"`
	PullRequest *struct{}        `json:"pull_request"`
}

type ghPullRequest struct {
	ID     int64   `json:"id"`
	NodeID string  `json:"node_id"`
	Number int     `json:"number"`
	Title  string  `json:"title"`
	Body   *string `json:"body"`
	State  string  `json:"state"`
	Draft  bool    `json:"draft"`
	Locked bool    `json:"locked"`
	User   *ghOwner `json:"user"`
	Head   struct {
		Ref  string  `json:"ref"`
		SHA  string  `json:"sha"`
		Repo *ghRepo `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
	Merged         bool             `json:"merged"`
	Mergeable      *bool            `json:"mergeable"`
	MergeableState *string          `json:"mergeable_state"`
	MergedBy       *ghOwner         `json:"merged_by"`
	MergedAt       *time.Time       `json:"merged_at"`
	MergeCommitSHA *string          `json:"merge_commit_sha"`
	Labels         json.RawMessage  `json:"labels"`
	Assignees      json.RawMessage  `json:"assignees"`
	Reviewers      json.RawMessage  `json:"requested_reviewers"`
	Milestone      *json.RawMessage `json:"milestone"`
	Comments       int              `json:"comments"`
	ReviewComments int              `json:"review_comments"`
	Commits        int              `json:"commits"`
	Additions      int              `json:"additions"`
	Deletions      int              `json:"deletions"`
	ChangedFiles   int              `json:"changed_files"`
	HTMLURL        string           `json:"html_url"`
	DiffURL        string           `json:"diff_url"`
	ClosedAt       *time.Time       `json:"closed_at"`
	CreatedAt      *time.Time       `json:"created_at"`
	UpdatedAt      *time.Time       `json:"updated_at"`
}

type ghCommit struct {
	SHA    string `json:"sha"`
	NodeID string `json:"node_id"`
	Commit struct {
		Message   string `json:"message"`
		Author    struct {
			Name  string     `json:"name"`
			Email string     `json:"email"`
			Date  *time.Time `json:"date"`
		} `json:"author"`
		Committer struct {
			Name  string     `json:"name"`
			Email string     `json:"email"`
			Date  *time.Time `json:"date"`
		} `json:"committer"`
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
		Verification struct {
			Verified bool   `json:"verified"`
			Reason   string `json:"reason"`
		} `json:"verification"`
	} `json:"commit"`
	Author    *ghOwner        `json:"author"`
	Committer *ghOwner        `json:"committer"`
	Parents   json.RawMessage `json:"parents"`
	Stats     *struct {
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
		Total     int `json:"total"`
	} `json:"stats"`
	HTMLURL string `json:"html_url"`
}

type ghRelease struct {
	ID              int64            `json:"id"`
	NodeID          string           `json:"node_id"`
	TagName         string           `json:"tag_name"`
	TargetCommitish string           `json:"target_commitish"`
	Name            *string          `json:"name"`
	Body            *string          `json:"body"`
	Draft           bool             `json:"draft"`
	Prerelease      bool             `json:"prerelease"`
	Author          *ghOwner         `json:"author"`
	HTMLURL         string           `json:"html_url"`
	TarballURL      string           `json:"tarball_url"`
	ZipballURL      string           `json:"zipball_url"`
	Assets          *json.RawMessage `json:"assets"`
	CreatedAt       *time.Time       `json:"created_at"`
	PublishedAt     *time.Time       `json:"published_at"`
}

type ghWorkflow struct {
	ID        int64      `json:"id"`
	NodeID    string     `json:"node_id"`
	Name      string     `json:"name"`
	Path      string     `json:"path"`
	State     string     `json:"state"`
	BadgeURL  string     `json:"badge_url"`
	HTMLURL   string     `json:"html_url"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type ghWorkflowsResp struct {
	Workflows []ghWorkflow `json:"workflows"`
}

type ghWorkflowRun struct {
	ID              int64    `json:"id"`
	NodeID          string   `json:"node_id"`
	WorkflowID      int64    `json:"workflow_id"`
	Name            string   `json:"name"`
	HeadBranch      string   `json:"head_branch"`
	HeadSHA         string   `json:"head_sha"`
	RunNumber       int      `json:"run_number"`
	RunAttempt      int      `json:"run_attempt"`
	Event           string   `json:"event"`
	Status          string   `json:"status"`
	Conclusion      *string  `json:"conclusion"`
	Actor           *ghOwner `json:"actor"`
	TriggeringActor *ghOwner `json:"triggering_actor"`
	HTMLURL         string   `json:"html_url"`
	JobsURL         string   `json:"jobs_url"`
	LogsURL         string   `json:"logs_url"`
	RunStartedAt    *time.Time `json:"run_started_at"`
	CreatedAt       *time.Time `json:"created_at"`
	UpdatedAt       *time.Time `json:"updated_at"`
}

type ghWorkflowRunsResp struct {
	WorkflowRuns []ghWorkflowRun `json:"workflow_runs"`
}

type ghDeployment struct {
	ID                    int64            `json:"id"`
	NodeID                string           `json:"node_id"`
	SHA                   string           `json:"sha"`
	Ref                   string           `json:"ref"`
	Task                  string           `json:"task"`
	Environment           string           `json:"environment"`
	Description           *string          `json:"description"`
	Creator               *ghOwner         `json:"creator"`
	ProductionEnvironment bool             `json:"production_environment"`
	TransientEnvironment  bool             `json:"transient_environment"`
	Payload               *json.RawMessage `json:"payload"`
	CreatedAt             *time.Time       `json:"created_at"`
	UpdatedAt             *time.Time       `json:"updated_at"`
}

type ghTeam struct {
	ID          int64   `json:"id"`
	NodeID      string  `json:"node_id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
	Privacy     string  `json:"privacy"`
	Permission  string  `json:"permission"`
	Parent      *struct {
		ID int64 `json:"id"`
	} `json:"parent"`
	MembersCount int        `json:"members_count"`
	ReposCount   int        `json:"repos_count"`
	HTMLURL      string     `json:"html_url"`
	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

type ghCollaborator struct {
	ID          int64            `json:"id"`
	Login       string           `json:"login"`
	Type        string           `json:"type"`
	SiteAdmin   bool             `json:"site_admin"`
	Permissions *json.RawMessage `json:"permissions"`
	RoleName    string           `json:"role_name"`
}

// --- Public API methods ------------------------------------------------------

func (c *GitHubClient) ListOrganizations() ([]ghOrg, error) {
	return fetchAllPages[ghOrg](c, "/user/orgs")
}

func (c *GitHubClient) ListRepositories(org string) ([]ghRepo, error) {
	return fetchAllPages[ghRepo](c, "/orgs/"+org+"/repos")
}

func (c *GitHubClient) ListUserRepositories() ([]ghRepo, error) {
	return fetchAllPages[ghRepo](c, "/user/repos?type=all&sort=updated")
}

func (c *GitHubClient) ListBranches(owner, repo string) ([]ghBranch, error) {
	return fetchAllPages[ghBranch](c, "/repos/"+owner+"/"+repo+"/branches")
}

func (c *GitHubClient) ListIssues(owner, repo, state string) ([]ghIssue, error) {
	if state == "" {
		state = "all"
	}
	return fetchAllPages[ghIssue](c, "/repos/"+owner+"/"+repo+"/issues?state="+state)
}

func (c *GitHubClient) ListPullRequests(owner, repo, state string) ([]ghPullRequest, error) {
	if state == "" {
		state = "all"
	}
	return fetchAllPages[ghPullRequest](c, "/repos/"+owner+"/"+repo+"/pulls?state="+state)
}

func (c *GitHubClient) ListCommits(owner, repo string) ([]ghCommit, error) {
	return fetchAllPages[ghCommit](c, "/repos/"+owner+"/"+repo+"/commits")
}

func (c *GitHubClient) ListReleases(owner, repo string) ([]ghRelease, error) {
	return fetchAllPages[ghRelease](c, "/repos/"+owner+"/"+repo+"/releases")
}

func (c *GitHubClient) ListWorkflows(owner, repo string) ([]ghWorkflow, error) {
	var resp ghWorkflowsResp
	if err := c.fetchJSON("/repos/"+owner+"/"+repo+"/actions/workflows", &resp); err != nil {
		return nil, err
	}
	return resp.Workflows, nil
}

func (c *GitHubClient) ListWorkflowRuns(owner, repo string) ([]ghWorkflowRun, error) {
	var resp ghWorkflowRunsResp
	if err := c.fetchJSON("/repos/"+owner+"/"+repo+"/actions/runs?per_page="+strconv.Itoa(defaultPerPage), &resp); err != nil {
		return nil, err
	}
	return resp.WorkflowRuns, nil
}

func (c *GitHubClient) ListDeployments(owner, repo string) ([]ghDeployment, error) {
	return fetchAllPages[ghDeployment](c, "/repos/"+owner+"/"+repo+"/deployments")
}

func (c *GitHubClient) ListTeams(org string) ([]ghTeam, error) {
	return fetchAllPages[ghTeam](c, "/orgs/"+org+"/teams")
}

func (c *GitHubClient) ListCollaborators(owner, repo string) ([]ghCollaborator, error) {
	return fetchAllPages[ghCollaborator](c, "/repos/"+owner+"/"+repo+"/collaborators")
}
