package destinations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SplunkDestination ships events to the Splunk HTTP Event Collector (HEC).
// HEC accepts a JSON object per line (NDJSON) at /services/collector/event.
type SplunkDestination struct {
	EndpointURL string
	HECToken    string
	client      *http.Client
}

// NewSplunk creates a SplunkDestination.
// endpointURL is typically "https://<splunk-host>:8088".
func NewSplunk(endpointURL, hecToken string) *SplunkDestination {
	return &SplunkDestination{
		EndpointURL: endpointURL,
		HECToken:    hecToken,
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

// splunkEvent wraps a raw event in the HEC envelope.
type splunkEvent struct {
	Time  float64 `json:"time"`
	Host  string  `json:"host"`
	Index string  `json:"index,omitempty"`
	Event any     `json:"event"`
}

// Ship sends events to Splunk HEC as an NDJSON batch.
func (d *SplunkDestination) Ship(ctx context.Context, events [][]byte) error {
	now := float64(time.Now().UnixMilli()) / 1000.0
	var buf bytes.Buffer

	for _, e := range events {
		var raw any
		if err := json.Unmarshal(e, &raw); err != nil {
			raw = string(e)
		}
		env := splunkEvent{
			Time:  now,
			Host:  "nself",
			Index: "nself_audit",
			Event: raw,
		}
		line, err := json.Marshal(env)
		if err != nil {
			continue
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.EndpointURL+"/services/collector/event", &buf)
	if err != nil {
		return fmt.Errorf("splunk request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Splunk "+d.HECToken)

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("splunk do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("splunk status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
