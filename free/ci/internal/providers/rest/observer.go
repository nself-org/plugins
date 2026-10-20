package rest

import (
	"context"
	"log/slog"
	"sync/atomic"
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
	// Download is set for an artifact download: its host is a CDN, so it says nothing about the API.
	Download bool
}

// SetObserver installs an in-memory callback. Configure clients before concurrent use.
func (c *Client) SetObserver(observer func(Outcome)) { c.observer = observer }

// defaultObserver sees the outcome of every client (P7-HOST-11:
// the provider health breaker installs itself here, so no client builder has to know about it).
var defaultObserver atomic.Pointer[func(Outcome)]

// SetDefaultObserver installs the process-wide observer; nil removes it. It sees the outcome of
// every client, after the client's own observer (SetObserver) has seen it.
func SetDefaultObserver(observer func(Outcome)) {
	if observer == nil {
		defaultObserver.Store(nil)
		return
	}
	defaultObserver.Store(&observer)
}

func (c *Client) observe(ctx context.Context, method, path string, response Response, err error, start time.Time) {
	out := Outcome{Provider: c.Provider, Class: providers.ClassOf(err), Status: response.Status, Duration: time.Since(start), RetryAt: response.RetryAt, Download: path == "<download>"}
	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("provider request", "provider", c.Provider, "method", method, "path_template", path, "status", response.Status, "duration", out.Duration)
	if ctx.Err() != nil {
		return // the caller gave up or ran out of its own budget: that says nothing about the provider
	}
	if c.observer != nil {
		c.observer(out)
	}
	if fn := defaultObserver.Load(); fn != nil { // the process-wide observer sees every client, also one with its own
		(*fn)(out)
	}
}
