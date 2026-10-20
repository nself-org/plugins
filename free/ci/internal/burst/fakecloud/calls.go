package fakecloud

// Purpose: the burst.Provider calls of the fake cloud (Estimate, Provision,
// Teardown, List) with their fault switches applied.
// Constraints: see fakecloud.go; each call checks the context and the outage
// switch first, so a cancelled or unreachable call changes nothing.

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
)

func (c *Cloud) enter(ctx context.Context, call string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Calls[call]++
	if c.f.outage {
		return fmt.Errorf("fakecloud %s: %w", call, burst.ErrUnavailable)
	}
	return nil
}

// Estimate implements burst.Provider.
func (c *Cloud) Estimate(ctx context.Context, _ burst.Spec) (burst.Estimate, error) {
	if err := c.enter(ctx, "Estimate"); err != nil {
		return burst.Estimate{}, err
	}
	return burst.Estimate{LatencyMs: c.Latency, Class: "ephemeral"}, nil
}

// Provision implements burst.Provider: idempotent by the intent label.
func (c *Cloud) Provision(ctx context.Context, in burst.Intent) (burst.Resource, error) {
	if err := c.enter(ctx, "Provision"); err != nil {
		return burst.Resource{}, err
	}
	for _, k := range []string{burst.LabelRole, burst.LabelCoordinator, burst.LabelIntent} {
		if in.Labels[k] == "" {
			return burst.Resource{}, fmt.Errorf("%w: label %s missing", ErrBadIntent, k)
		}
	}
	for k, v := range in.Labels {
		if len(v) > burst.MaxLabelValue || len(k) > burst.MaxLabelValue {
			return burst.Resource{}, fmt.Errorf("%w: label %s too long", ErrBadIntent, k)
		}
	}
	if in.Token == "" || in.Labels[burst.LabelIntent] != in.Token {
		return burst.Resource{}, fmt.Errorf("%w: intent label does not match the token", ErrBadIntent)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f.provisionErrN > 0 {
		c.f.provisionErrN--
		return burst.Resource{}, c.f.provisionErr
	}
	for _, r := range c.res {
		if r.Labels[burst.LabelIntent] == in.Token && r.Labels[burst.LabelRole] == burst.RoleEphemeral {
			return clone(r), nil
		}
	}
	r := c.create("nselfci-"+in.Token, in.Labels, nil, c.clock.Now())
	if c.f.slow > 0 {
		c.clock.Advance(c.f.slow)
	}
	if c.f.lose > 0 {
		c.f.lose--
		return burst.Resource{}, ErrLostResponse
	}
	return r, nil
}

// Teardown implements burst.Provider: a missing machine is success.
func (c *Cloud) Teardown(ctx context.Context, id string) error {
	if err := c.enter(ctx, "Teardown"); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f.teardownErr != nil {
		return c.f.teardownErr
	}
	delete(c.res, id)
	return nil
}

// List implements burst.Provider: machines carrying every requested label.
func (c *Cloud) List(ctx context.Context, labels map[string]string) ([]burst.Resource, error) {
	if err := c.enter(ctx, "List"); err != nil {
		return nil, err
	}
	var out []burst.Resource
	for _, r := range c.All() {
		if burst.Matches(r.Labels, labels) {
			out = append(out, r)
		}
	}
	return out, nil
}

var _ burst.Provider = (*Cloud)(nil)

// Elapsed is the clock time since t (a test helper for the slow-provision case).
func (c *Cloud) Elapsed(t time.Time) time.Duration { return c.clock.Now().Sub(t) }
