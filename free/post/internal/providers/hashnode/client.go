// Package hashnode implements a Hashnode GraphQL API publishing client.
//
// Purpose: Publish blog posts to Hashnode via the public GraphQL API.
// Inputs: api_key (Hashnode Developer API key), publication_host (your
//         Hashnode publication hostname, e.g. blog.example.com), title,
//         and content in Markdown.
// Outputs: post ID and URL on success.
// Constraints: Requires a Hashnode account with at least one publication.
//              The API key must have WRITE scope. Tags are looked up by slug.
package hashnode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/shared/httpclient"
)

const hashnodeGraphQLURL = "https://gql.hashnode.com"

var defaultClient = httpclient.New(httpclient.Options{Timeout: 15 * time.Second})

// Client publishes to Hashnode via the GraphQL API.
type Client struct {
	apiKey          string
	publicationHost string
	httpClient      *http.Client
}

// NewClient constructs a hashnode.Client.
func NewClient(apiKey, publicationHost string) *Client {
	return &Client{apiKey: apiKey, publicationHost: publicationHost, httpClient: defaultClient}
}

// PostArgs carries the article content to publish.
type PostArgs struct {
	Title   string
	Content string   // Markdown body
	Tags    []string // slugs like "go", "webdev"
}

// PostResult is the outcome of a successful Hashnode post.
type PostResult struct {
	PostID string
	URL    string
}

// Post creates a published post on Hashnode.
func (c *Client) Post(ctx context.Context, args PostArgs) (*PostResult, error) {
	// Build tags array
	tagInputs := make([]map[string]string, 0, len(args.Tags))
	for _, tag := range args.Tags {
		tagInputs = append(tagInputs, map[string]string{"slug": strings.ToLower(tag), "name": tag})
	}

	// GraphQL mutation for Hashnode v2 API
	mutation := `mutation PublishPost($input: PublishPostInput!) {
  publishPost(input: $input) {
    post {
      id
      url
    }
  }
}`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"title":           args.Title,
			"contentMarkdown": args.Content,
			"publicationId":   c.publicationHost, // host resolves to ID server-side
			"tags":            tagInputs,
		},
	}

	payload := map[string]interface{}{
		"query":     mutation,
		"variables": variables,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("hashnode: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hashnodeGraphQLURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("hashnode: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hashnode: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hashnode: HTTP error %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var gqlResp struct {
		Data struct {
			PublishPost struct {
				Post struct {
					ID  string `json:"id"`
					URL string `json:"url"`
				} `json:"post"`
			} `json:"publishPost"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &gqlResp); err != nil {
		return nil, fmt.Errorf("hashnode: unmarshal response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("hashnode: GraphQL error: %s", gqlResp.Errors[0].Message)
	}

	post := gqlResp.Data.PublishPost.Post
	return &PostResult{PostID: post.ID, URL: post.URL}, nil
}
