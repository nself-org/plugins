package internal

import (
	"context"
	"fmt"

	"github.com/nself-org/nself-post/internal/providers/devto"
	"github.com/nself-org/nself-post/internal/providers/ghost"
	"github.com/nself-org/nself-post/internal/providers/hashnode"
	"github.com/nself-org/nself-post/internal/providers/linkedin"
	"github.com/nself-org/nself-post/internal/providers/telegram"
	"github.com/nself-org/nself-post/internal/providers/twitter"
	"github.com/nself-org/nself-post/internal/providers/wordpress"
)

// dispatchWordPress publishes to a WordPress site via XML-RPC. Required
// credential fields on the account:
//   - endpoint: site root or full xmlrpc.php URL (falls back to account.Endpoint)
//   - username: WP username
//   - app_password: WP Application Password
//
// Optional:
//   - categories: []string
//   - status: "publish" (default), "draft", "private", "pending"
func dispatchWordPress(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	endpoint := stringCred(account.Credentials, "endpoint")
	if endpoint == "" && account.Endpoint != nil {
		endpoint = *account.Endpoint
	}
	username := stringCred(account.Credentials, "username")
	appPass := stringCred(account.Credentials, "app_password")
	if appPass == "" {
		// Allow the common spelling "application_password" as well.
		appPass = stringCred(account.Credentials, "application_password")
	}
	if endpoint == "" || username == "" || appPass == "" {
		return nil, fmt.Errorf("wordpress credentials incomplete: require endpoint, username, app_password")
	}
	categories := stringsCred(account.Credentials, "categories")
	status := stringCred(account.Credentials, "status")

	client := wordpress.NewClient(endpoint, username, appPass)
	res, err := client.NewPost(ctx, wordpress.NewPostArgs{
		Title:      title,
		Content:    content,
		Categories: categories,
		Tags:       tags,
		Status:     status,
	})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{
		Metadata: map[string]interface{}{"post_id": res.PostID},
	}
	if res.PostID != "" {
		id := res.PostID
		out.PostID = &id
	}
	if res.Permalink != "" {
		link := res.Permalink
		out.URL = &link
	}
	return out, nil
}

// dispatchGhost publishes to a Ghost site via the Admin API. Required
// credential fields:
//   - endpoint: site root (falls back to account.Endpoint)
//   - admin_api_key: "<keyId>:<hexSecret>"
//
// Optional:
//   - status: "published" (default), "draft", "scheduled"
func dispatchGhost(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	endpoint := stringCred(account.Credentials, "endpoint")
	if endpoint == "" && account.Endpoint != nil {
		endpoint = *account.Endpoint
	}
	key := stringCred(account.Credentials, "admin_api_key")
	if endpoint == "" || key == "" {
		return nil, fmt.Errorf("ghost credentials incomplete: require endpoint, admin_api_key")
	}
	status := stringCred(account.Credentials, "status")

	client := ghost.NewClient(endpoint, key)
	res, err := client.NewPost(ctx, ghost.NewPostArgs{
		Title:  title,
		HTML:   content,
		Tags:   tags,
		Status: status,
	})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{
		Metadata: map[string]interface{}{"post_id": res.PostID},
	}
	if res.PostID != "" {
		id := res.PostID
		out.PostID = &id
	}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}

// stringCred reads a string field from the decrypted credentials map. Returns
// "" when the key is missing or of the wrong type.
func stringCred(creds map[string]interface{}, key string) string {
	if creds == nil {
		return ""
	}
	if v, ok := creds[key].(string); ok {
		return v
	}
	return ""
}

// stringsCred reads a []string field from the decrypted credentials map.
// Accepts both []string (unlikely after JSON round-trip) and []interface{}
// (the normal case after json.Unmarshal).
func stringsCred(creds map[string]interface{}, key string) []string {
	if creds == nil {
		return nil
	}
	switch v := creds[key].(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// dispatchTwitter publishes via Twitter/X v2 API.
// Required credential fields: bearer_token.
func dispatchTwitter(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	token := stringCred(account.Credentials, "bearer_token")
	if token == "" {
		return nil, fmt.Errorf("twitter credentials incomplete: require bearer_token")
	}
	text := title
	if content != "" {
		text = title + "\n\n" + content
	}
	client := twitter.NewClient(token)
	res, err := client.Post(ctx, twitter.PostArgs{Text: text})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{Metadata: map[string]interface{}{"tweet_id": res.TweetID}}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}

// dispatchLinkedIn publishes via LinkedIn UGC Posts API.
// Required credential fields: access_token. Optional: author_urn.
func dispatchLinkedIn(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	token := stringCred(account.Credentials, "access_token")
	if token == "" {
		return nil, fmt.Errorf("linkedin credentials incomplete: require access_token")
	}
	authorURN := stringCred(account.Credentials, "author_urn")
	text := title
	if content != "" {
		text = title + "\n\n" + content
	}
	client := linkedin.NewClient(token)
	res, err := client.Post(ctx, linkedin.PostArgs{AuthorURN: authorURN, Text: text})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{Metadata: map[string]interface{}{"post_urn": res.PostURN}}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}

// dispatchTelegram publishes via Telegram Bot API.
// Required credential fields: bot_token, chat_id.
func dispatchTelegram(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	botToken := stringCred(account.Credentials, "bot_token")
	chatID := stringCred(account.Credentials, "chat_id")
	if account.Endpoint != nil && chatID == "" {
		chatID = *account.Endpoint
	}
	if botToken == "" || chatID == "" {
		return nil, fmt.Errorf("telegram credentials incomplete: require bot_token, chat_id")
	}
	text := title
	if content != "" {
		text = "<b>" + title + "</b>\n\n" + content
	}
	client := telegram.NewClient(botToken)
	res, err := client.Post(ctx, telegram.PostArgs{ChatID: chatID, Text: text})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{Metadata: map[string]interface{}{"message_id": res.MessageID}}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}

// dispatchDevTo publishes via Dev.to Articles API.
// Required credential fields: api_key.
func dispatchDevTo(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	apiKey := stringCred(account.Credentials, "api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("devto credentials incomplete: require api_key")
	}
	client := devto.NewClient(apiKey)
	res, err := client.Post(ctx, devto.PostArgs{
		Title:     title,
		Content:   content,
		Tags:      tags,
		Published: true,
	})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{Metadata: map[string]interface{}{"article_id": res.ArticleID}}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}

// dispatchHashnode publishes via Hashnode GraphQL API.
// Required credential fields: api_key, publication_host.
func dispatchHashnode(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	apiKey := stringCred(account.Credentials, "api_key")
	pubHost := stringCred(account.Credentials, "publication_host")
	if account.Endpoint != nil && pubHost == "" {
		pubHost = *account.Endpoint
	}
	if apiKey == "" || pubHost == "" {
		return nil, fmt.Errorf("hashnode credentials incomplete: require api_key, publication_host")
	}
	client := hashnode.NewClient(apiKey, pubHost)
	res, err := client.Post(ctx, hashnode.PostArgs{
		Title:   title,
		Content: content,
		Tags:    tags,
	})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{Metadata: map[string]interface{}{"post_id": res.PostID}}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}
