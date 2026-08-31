// Package devto implements a Dev.to Articles API publishing client.
//
// Purpose: Publish articles to Dev.to via the Forem Articles API.
// Inputs: api_key (Dev.to API key from Settings > Account), article title,
//         body_markdown content, optional tags (up to 4), optional series.
// Outputs: article ID and URL on success.
// Constraints: Tags must be lowercase slugs. Published articles are live
//              immediately; pass published=false to create as draft.
package devto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/plugins-pro/paid/shared/httpclient"
)

const devtoArticlesURL = "https://dev.to/api/articles"

var defaultClient = httpclient.New(httpclient.Options{Timeout: 15 * time.Second})

// Client publishes articles to Dev.to via the Forem Articles API.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient constructs a devto.Client.
func NewClient(apiKey string) *Client {
	return &Client{apiKey: apiKey, httpClient: defaultClient}
}

// PostArgs carries the article content to publish.
type PostArgs struct {
	Title    string
	Content  string   // Markdown body
	Tags     []string // up to 4 lowercase slugs
	Series   string   // optional series name
	Published bool    // true to publish immediately
}

// PostResult is the outcome of a successful Dev.to article creation.
type PostResult struct {
	ArticleID int
	URL       string
}

// Post creates or publishes an article on Dev.to.
func (c *Client) Post(ctx context.Context, args PostArgs) (*PostResult, error) {
	// Clamp to 4 tags and lowercase them
	tags := args.Tags
	if len(tags) > 4 {
		tags = tags[:4]
	}
	for i, tag := range tags {
		tags[i] = strings.ToLower(strings.ReplaceAll(tag, " ", ""))
	}

	article := map[string]interface{}{
		"title":         args.Title,
		"body_markdown": args.Content,
		"published":     args.Published,
	}
	if len(tags) > 0 {
		article["tags"] = tags
	}
	if args.Series != "" {
		article["series"] = args.Series
	}

	payload := map[string]interface{}{"article": article}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("devto: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, devtoArticlesURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("devto: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("devto: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("devto: API error %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var result struct {
		ID  int    `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("devto: unmarshal response: %w", err)
	}

	return &PostResult{ArticleID: result.ID, URL: result.URL}, nil
}
