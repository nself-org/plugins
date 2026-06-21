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
