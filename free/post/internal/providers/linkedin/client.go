// Package linkedin implements a LinkedIn UGC Posts API publishing client.
//
// Purpose: Publish text posts to a LinkedIn member or organization page via
//          the UGC (User Generated Content) Posts v2 API.
// Inputs: access_token (OAuth 2.0 Bearer), person_urn (urn:li:person:<id>) or
//         organization_urn (urn:li:organization:<id>), and post content.
// Outputs: post URN and URL on success.
// Constraints: Requires w_member_social OAuth2 scope. The access_token is
//              obtained via the linkedin plugin's OAuth flow and passed here
//              as a credential. Images delegated to a future adapter.
package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/shared-utils/httpclient"
)

const linkedinUGCPostsURL = "https://api.linkedin.com/v2/ugcPosts"

var defaultClient = httpclient.New(httpclient.Options{Timeout: 15 * time.Second})

// Client publishes to LinkedIn via UGC Posts API.
type Client struct {
	accessToken string
	httpClient  *http.Client
}

// NewClient constructs a linkedin.Client.
func NewClient(accessToken string) *Client {
	return &Client{accessToken: accessToken, httpClient: defaultClient}
}

// PostArgs carries the content to publish.
type PostArgs struct {
	AuthorURN string // urn:li:person:<id> or urn:li:organization:<id>
	Text      string
}

// PostResult is the outcome of a successful LinkedIn post.
type PostResult struct {
	PostURN string
	URL     string
}

// Post creates a LinkedIn post and returns the post URN.
func (c *Client) Post(ctx context.Context, args PostArgs) (*PostResult, error) {
	if args.AuthorURN == "" {
		args.AuthorURN = "urn:li:person:me"
	}

	payload := map[string]interface{}{
		"author": args.AuthorURN,
		"lifecycleState": "PUBLISHED",
		"specificContent": map[string]interface{}{
			"com.linkedin.ugc.ShareContent": map[string]interface{}{
				"shareCommentary": map[string]string{
					"text": args.Text,
				},
				"shareMediaCategory": "NONE",
			},
		},
		"visibility": map[string]string{
			"com.linkedin.ugc.MemberNetworkVisibility": "PUBLIC",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("linkedin: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, linkedinUGCPostsURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("linkedin: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedin: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("linkedin: API error %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	postURN := resp.Header.Get("X-RestLi-Id")
	if postURN == "" {
		// Fallback: try to parse from body if present
		var result struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &result)
		postURN = result.ID
	}

	postURL := ""
	if postURN != "" {
		// LinkedIn post URLs require the numeric ID segment
		parts := strings.Split(postURN, ":")
		if len(parts) > 0 {
			postURL = "https://www.linkedin.com/feed/update/" + parts[len(parts)-1]
		}
	}

	return &PostResult{PostURN: postURN, URL: postURL}, nil
}
