package fakecloud_test

import (
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst/conformance"
	"github.com/nself-org/plugins/free/ci/internal/burst/fakecloud"
)

// TestConformance: the fake cloud passes the lifecycle kit.
func TestConformance(t *testing.T) {
	conformance.Run(t, func(t *testing.T) conformance.Subject {
		c := fakecloud.New(fakecloud.NewClock(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)))
		return conformance.Subject{
			Provider: c,
			Foreign:  func(labels map[string]string) string { return c.AddForeign(labels) },
			Outage:   c.Outage,
		}
	})
}
