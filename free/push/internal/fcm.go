package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	fcmSendURL = "https://fcm.googleapis.com/v1/projects/%s/messages:send"
	fcmScope   = "https://www.googleapis.com/auth/firebase.messaging"
)

// FCMClient sends push notifications via the FCM v1 HTTP API (not the legacy API).
// The service account JSON is loaded once at startup; token refresh is handled
// transparently by the oauth2 token source.
type FCMClient struct {
	projectID   string
	tokenSource oauth2.TokenSource
	http        *http.Client
}

// NewFCMClient constructs an FCMClient from the given config.
// Returns nil (not an error) when FCM is not configured; callers check for nil.
func NewFCMClient(ctx context.Context, cfg *Config) (*FCMClient, error) {
	if !cfg.FCMEnabled() {
		return nil, nil
	}

	// Parse service account JSON — fail fast on malformed credentials.
	creds, err := google.CredentialsFromJSON(
		ctx,
		[]byte(cfg.FCMServiceAccountJSON),
		fcmScope,
	)
	if err != nil {
		return nil, fmt.Errorf("fcm: parse service account JSON: %w", err)
	}

	return &FCMClient{
		projectID:   cfg.FCMProjectID,
		tokenSource: creds.TokenSource,
		http:        &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// FCMResult holds the outcome of a single FCM delivery attempt.
type FCMResult struct {
	Success    bool
	MessageID  string
	Error      string
	StatusCode int
}

// fcmMessage is the top-level FCM v1 API request body.
type fcmMessage struct {
	Message fcmMessageBody `json:"message"`
}

// fcmMessageBody mirrors the FCM v1 Message object.
// We forward the caller's payload as the "notification" block if it contains
// a valid {"notification": {...}} shape, or as "data" otherwise.
type fcmMessageBody struct {
	Token        string                 `json:"token"`
	Notification *fcmNotification       `json:"notification,omitempty"`
	Data         map[string]string      `json:"data,omitempty"`
}

type fcmNotification struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
	Image string `json:"image,omitempty"`
}

// Send delivers a notification to the given device token.
// payload must be the JSONB stored in np_push_outbox — we attempt to extract
// a "notification" key for the FCM Notification block; remaining keys become data.
func (c *FCMClient) Send(ctx context.Context, deviceToken string, payload json.RawMessage) FCMResult {
	msg, err := buildFCMMessage(deviceToken, payload)
	if err != nil {
		return FCMResult{Error: fmt.Sprintf("fcm: build message: %v", err)}
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return FCMResult{Error: fmt.Sprintf("fcm: marshal: %v", err)}
	}

	url := fmt.Sprintf(fcmSendURL, c.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return FCMResult{Error: fmt.Sprintf("fcm: build request: %v", err)}
	}
	req.Header.Set("Content-Type", "application/json")

	// Fetch (or refresh) the OAuth2 access token.
	t, err := c.tokenSource.Token()
	if err != nil {
		return FCMResult{Error: fmt.Sprintf("fcm: token: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return FCMResult{Error: fmt.Sprintf("fcm: http: %v", err)}
	}
	defer resp.Body.Close()

	var respBody struct {
		Name  string `json:"name"` // message ID on success
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return FCMResult{StatusCode: resp.StatusCode, Error: fmt.Sprintf("fcm: decode response: %v", err)}
	}

	if resp.StatusCode == http.StatusOK && respBody.Error == nil {
		return FCMResult{Success: true, MessageID: respBody.Name, StatusCode: resp.StatusCode}
	}

	errMsg := fmt.Sprintf("fcm: status %d", resp.StatusCode)
	if respBody.Error != nil {
		errMsg = fmt.Sprintf("fcm: %s (%s)", respBody.Error.Status, respBody.Error.Message)
		// Surface credential errors explicitly so operators know to rotate.
		if respBody.Error.Status == "UNAUTHENTICATED" || respBody.Error.Status == "PERMISSION_DENIED" {
			errMsg += " — verify PUSH_FCM_SERVICE_ACCOUNT_JSON and Firebase project permissions"
		}
	}
	return FCMResult{StatusCode: resp.StatusCode, Error: errMsg}
}

// buildFCMMessage constructs the FCM v1 message from the outbox payload.
// The payload is expected to be a JSONB object that may contain:
//
//	{
//	  "notification": {"title": "...", "body": "...", "image": "..."},
//	  "data": {"key": "value", ...}
//	}
//
// If "notification" is absent, the whole payload (minus "data") is treated as data.
func buildFCMMessage(deviceToken string, payload json.RawMessage) (*fcmMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	msg := &fcmMessage{
		Message: fcmMessageBody{Token: deviceToken},
	}

	// Extract "notification" block.
	if notifRaw, ok := raw["notification"]; ok {
		var n fcmNotification
		if err := json.Unmarshal(notifRaw, &n); err == nil {
			msg.Message.Notification = &n
		}
	}

	// Extract "data" block — FCM data values must be strings.
	if dataRaw, ok := raw["data"]; ok {
		var d map[string]string
		if err := json.Unmarshal(dataRaw, &d); err == nil && len(d) > 0 {
			msg.Message.Data = d
		}
	}

	return msg, nil
}
