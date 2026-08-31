package destinations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ElasticDestination ships events to Elasticsearch using the Bulk API.
// Elastic Bulk API expects alternating action/source NDJSON lines.
type ElasticDestination struct {
	EndpointURL string
	APIKey      string
	Index       string
	client      *http.Client
}

// NewElastic creates an ElasticDestination.
// endpointURL is typically "https://<elastic-host>:9200".
func NewElastic(endpointURL, apiKey, index string) *ElasticDestination {
	if index == "" {
		index = "nself-audit"
	}
	return &ElasticDestination{
		EndpointURL: endpointURL,
		APIKey:      apiKey,
		Index:       index,
		client:      &http.Client{Timeout: 20 * time.Second},
	}
}

type elasticAction struct {
	Index struct {
		Index string `json:"_index"`
		ID    string `json:"_id"`
	} `json:"index"`
}

// Ship sends events to Elasticsearch via the Bulk API.
// Returns an error if the bulk response contains any errors.
func (d *ElasticDestination) Ship(ctx context.Context, events [][]byte) error {
	var buf bytes.Buffer
	for _, e := range events {
		act := elasticAction{}
		act.Index.Index = d.Index
		act.Index.ID = uuid.New().String()

		actionLine, err := json.Marshal(act)
		if err != nil {
			continue
		}
		buf.Write(actionLine)
		buf.WriteByte('\n')
		buf.Write(e)
		buf.WriteByte('\n')
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.EndpointURL+"/_bulk", &buf)
	if err != nil {
		return fmt.Errorf("elastic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	if d.APIKey != "" {
		req.Header.Set("Authorization", "ApiKey "+d.APIKey)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("elastic do: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("elastic status %d: %s", resp.StatusCode, string(body))
	}

	// Check the bulk response for item-level errors.
	var bulkResp struct {
		Errors bool `json:"errors"`
	}
	if err := json.Unmarshal(body, &bulkResp); err == nil && bulkResp.Errors {
		return fmt.Errorf("elastic bulk had item errors — check index mapping")
	}
	return nil
}
