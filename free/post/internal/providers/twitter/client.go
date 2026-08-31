// Package twitter implements a Twitter/X v2 API publishing client.
//
// Purpose: Publish text tweets via the Twitter v2 API using OAuth2 Bearer token
//          or OAuth1 user context.
// Inputs: endpoint (unused, always api.twitter.com), bearer_token or
//         oauth_consumer_key + oauth_consumer_secret + oauth_access_token +
//         oauth_access_secret for user context posting.
// Outputs: tweet ID and URL on success.
// Constraints: Text is truncated to 280 chars. Images not supported in this
//              adapter (media upload requires a separate API call). Rate limit
//              is 50 posts/24h on the Basic tier.
package twitter

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

const (
	twitterV2TweetsURL = "https://api.twitter.com/2/tweets"
	maxTweetLength     = 280
)

var defaultClient = httpclient.New(httpclient.Options{Timeout: 15 * time.Second})

// Client publishes to Twitter/X via the v2 API.
type Client struct {
	bearerToken string
	httpClient  *http.Client
}

// NewClient constructs a twitter.Client using a Bearer token (app-only auth).
// For user context posting set oauth_* credentials in account config — the
// caller uses bearerToken from account.Credentials["bearer_token"].
func NewClient(bearerToken string) *Client {
	return &Client{bearerToken: bearerToken, httpClient: defaultClient}
}

// PostArgs carries the content to tweet.
type PostArgs struct {
	Text string
}

// PostResult is the outcome of a successful tweet.
type PostResult struct {
	TweetID string
	URL     string
}

// Post creates a tweet and returns the tweet ID and URL.
func (c *Client) Post(ctx context.Context, args PostArgs) (*PostResult, error) {
	text := args.Text
	if len([]rune(text)) > maxTweetLength {
		runes := []rune(text)
		text = string(runes[:maxTweetLength-1]) + "…"
	}

	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, fmt.Errorf("twitter: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, twitterV2TweetsURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("twitter: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("twitter: API error %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var response struct {
		Data struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("twitter: unmarshal response: %w", err)
	}

	tweetURL := "https://twitter.com/i/web/status/" + response.Data.ID
	return &PostResult{TweetID: response.Data.ID, URL: tweetURL}, nil
}
