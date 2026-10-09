package rest

import (
	"log/slog"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// Outcome is safe request metadata for health and breaker observation.
type Outcome struct {
	Provider string
	Class    providers.ErrorClass
	Status   int
	Duration time.Duration
	RetryAt  time.Time
}

// SetObserver installs an in-memory callback. Configure clients before concurrent use.
func (c *Client) SetObserver(observer func(Outcome)) { c.observer = observer }

func (c *Client) observe(method, path string, response Response, err error, start time.Time) {
	out := Outcome{Provider: c.Provider, Class: providers.ClassOf(err), Status: response.Status, Duration: time.Since(start), RetryAt: response.RetryAt}
	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("provider request", "provider", c.Provider, "method", method, "path_template", path, "status", response.Status, "duration", out.Duration)
	if c.observer != nil {
		c.observer(out)
	}
}
