// Package destinations implements per-SIEM HTTP shipping logic.
package destinations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// LokiDestination ships events to a Grafana Loki push endpoint.
// Loki expects the /loki/api/v1/push format.
type LokiDestination struct {
	EndpointURL string
	client      *http.Client
}

// NewLoki creates a LokiDestination.
func NewLoki(endpointURL string) *LokiDestination {
	return &LokiDestination{
		EndpointURL: endpointURL,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

// lokiPushBody is the Loki push API body.
type lokiPushBody struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"` // [timestamp_ns, line]
}

// Ship sends events to Loki using the push API.
func (d *LokiDestination) Ship(ctx context.Context, events [][]byte) error {
	now := fmt.Sprintf("%d", time.Now().UnixNano())
	values := make([][]string, 0, len(events))
	for _, e := range events {
		values = append(values, []string{now, string(e)})
	}

	body := lokiPushBody{
		Streams: []lokiStream{
			{
				Stream: map[string]string{"job": "nself-siem", "source": "np_audit_log"},
				Values: values,
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("loki marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.EndpointURL+"/loki/api/v1/push", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("loki request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("loki do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("loki status %d", resp.StatusCode)
	}
	return nil
}
