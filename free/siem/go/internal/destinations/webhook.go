package destinations

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookDestination ships a JSON array of events to an arbitrary HTTP endpoint.
// Useful for Sumo Logic HTTP source, custom SIEM, or any webhook receiver.
type WebhookDestination struct {
	EndpointURL string
	APIKey      string // sent as Bearer token if non-empty
	client      *http.Client
}

// NewWebhook creates a WebhookDestination.
func NewWebhook(endpointURL, apiKey string) *WebhookDestination {
	return &WebhookDestination{
		EndpointURL: endpointURL,
		APIKey:      apiKey,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

// Ship sends events as a JSON array to the webhook URL.
func (d *WebhookDestination) Ship(ctx context.Context, events [][]byte) error {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, e := range events {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(e)
	}
	buf.WriteByte(']')

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.EndpointURL, &buf)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if d.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.APIKey)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("webhook status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
