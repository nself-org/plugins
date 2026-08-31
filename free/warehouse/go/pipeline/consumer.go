// Package pipeline implements CDC event consumption, batching, and backfill.
package pipeline

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/shared-utils/httpclient"
)

// consumerClient is the package-level HTTP client used to subscribe to the
// CDC plugin's SSE stream. SSE is long-lived: the request ctx is the
// canonical lifetime, so we Wrap a zero-timeout client (httpclient.New
// would default to 30s, which would terminate the stream). Wrap still
// propagates X-Request-ID for cross-plugin tracing.
// nolint:forbidigo // intentional: SSE needs zero client timeout, Wrap preserves request-ID propagation
var consumerClient = httpclient.Wrap(&http.Client{})

// CDCEvent represents one row-level change delivered by the cdc plugin.
type CDCEvent struct {
	Table     string                 `json:"table"`
	Op        string                 `json:"op"` // INSERT | UPDATE | DELETE
	LSN       string                 `json:"lsn"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp time.Time              `json:"ts"`
}

// cdcPluginPort reads the CDC plugin port from the environment.
// The cdc plugin defaults to port 8209.
func cdcPluginPort() string {
	if p := os.Getenv("CDC_PLUGIN_PORT"); p != "" {
		return p
	}
	return "8209"
}

// Consumer reads CDC events from the cdc plugin's SSE stream and hands
// them to the Batcher.
type Consumer struct {
	cfg     *Config
	batcher *Batcher
}

func newConsumer(cfg *Config, b *Batcher) *Consumer {
	return &Consumer{cfg: cfg, batcher: b}
}

// Run subscribes to the CDC stream, reconnecting on transient failures.
func (c *Consumer) Run(ctx context.Context) error {
	backoff := time.Second
	maxBackoff := 60 * time.Second

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := c.subscribe(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		slog.Warn("cdc consumer disconnected; reconnecting", "err", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// subscribe opens the SSE stream from the cdc plugin and feeds events to
// the batcher until the context is cancelled or the connection drops.
func (c *Consumer) subscribe(ctx context.Context) error {
	url := fmt.Sprintf("http://127.0.0.1:%s/cdc/events", cdcPluginPort())
	if len(c.cfg.Tables) > 0 {
		url += "?tables=" + strings.Join(c.cfg.Tables, ",")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := consumerClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect cdc stream: %w", err)
	}
	defer resp.Body.Close()

	// SSE format: lines starting with "data: " followed by JSON, separated by blank lines.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := strings.TrimPrefix(line, "data: ")

		var evt CDCEvent
		if err := json.Unmarshal([]byte(raw), &evt); err != nil {
			slog.Warn("malformed cdc event", "raw", raw, "err", err)
			continue
		}
		if err := c.batcher.Add(ctx, evt); err != nil {
			slog.Warn("batcher rejected event", "table", evt.Table, "err", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("sse read: %w", err)
	}
	return nil
}
