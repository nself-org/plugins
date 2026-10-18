package destinations

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DatadogDestination ships events to the Datadog Logs HTTP API.
// Expects HTTP 202 on success.
type DatadogDestination struct {
	EndpointURL string
	APIKey      string
	client      *http.Client
}

// NewDatadog creates a DatadogDestination.
// endpointURL is typically "https://http-intake.logs.datadoghq.com/api/v2/logs".
func NewDatadog(endpointURL, apiKey string) *DatadogDestination {
	return &DatadogDestination{
		EndpointURL: endpointURL,
		APIKey:      apiKey,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

// Ship sends a JSON array of events to the Datadog Logs API.
// Datadog accepts a JSON array as the POST body.
func (d *DatadogDestination) Ship(ctx context.Context, events [][]byte) error {
	// Build a JSON array body: [event, event, ...]
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
		return fmt.Errorf("datadog request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", d.APIKey)

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("datadog do: %w", err)
	}
	defer resp.Body.Close()

	// Datadog returns 202 Accepted on success.
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("datadog status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
