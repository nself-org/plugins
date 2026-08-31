// Package telegram implements a Telegram Bot API publishing client.
//
// Purpose: Send messages to a Telegram channel or chat via the Bot API
//          sendMessage endpoint.
// Inputs: bot_token (Telegram Bot API token from BotFather),
//         chat_id (channel @username or numeric chat ID), text content.
// Outputs: message ID and channel URL on success.
// Constraints: Bot must be added as admin to the target channel. Markdown
//              or HTML parse_mode is applied when content starts with HTML
//              tags, otherwise plain text is sent.
package telegram

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

const telegramAPIBase = "https://api.telegram.org/bot"

var defaultClient = httpclient.New(httpclient.Options{Timeout: 15 * time.Second})

// Client publishes to a Telegram channel via Bot API.
type Client struct {
	botToken   string
	httpClient *http.Client
}

// NewClient constructs a telegram.Client.
func NewClient(botToken string) *Client {
	return &Client{botToken: botToken, httpClient: defaultClient}
}

// PostArgs carries the content to send.
type PostArgs struct {
	ChatID    string // @channelusername or numeric chat_id
	Text      string
	ParseMode string // "HTML", "Markdown", or "" (plain text)
}

// PostResult is the outcome of a successful Telegram message.
type PostResult struct {
	MessageID int
	URL       string // set when chat_id is a @username
}

// Post sends a message to the configured Telegram channel.
func (c *Client) Post(ctx context.Context, args PostArgs) (*PostResult, error) {
	if args.ParseMode == "" {
		// Auto-detect: if content contains HTML tags, use HTML parse mode
		if strings.Contains(args.Text, "<") && strings.Contains(args.Text, ">") {
			args.ParseMode = "HTML"
		}
	}

	payload := map[string]interface{}{
		"chat_id": args.ChatID,
		"text":    args.Text,
	}
	if args.ParseMode != "" {
		payload["parse_mode"] = args.ParseMode
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("telegram: marshal request: %w", err)
	}

	url := telegramAPIBase + c.botToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("telegram: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var apiResp struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
			Chat      struct {
				Username string `json:"username"`
			} `json:"chat"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("telegram: unmarshal response: %w", err)
	}
	if !apiResp.OK {
		return nil, fmt.Errorf("telegram: API error: %s", apiResp.Description)
	}

	postURL := ""
	if username := apiResp.Result.Chat.Username; username != "" {
		postURL = fmt.Sprintf("https://t.me/%s/%d", username, apiResp.Result.MessageID)
	}

	return &PostResult{MessageID: apiResp.Result.MessageID, URL: postURL}, nil
}
