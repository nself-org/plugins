package probe

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

// SSHProber uses the pinned D4 option set and the SDK's remote exec funnel.
type SSHProber struct {
	Registry   *registry.Registry
	PinnedFile string
	KeyPath    string
	Version    remote.Version
	runner     commandRunner // test hook; production always uses remote.Run
}

func (p *SSHProber) Probe(ctx context.Context, id string) (registry.Node, error) {
	return run(ctx, p.Registry, id, func(ctx context.Context, c *model.Capability) error {
		runner := p.runner
		if runner == nil {
			if c.Identity.SSH == nil || p.PinnedFile == "" {
				return fmt.Errorf("ssh probe requires a registered SSH host and pinned file")
			}
			ssh := c.Identity.SSH
			host := ssh.Alias
			if host == "" {
				host = ssh.Hostname
			}
			if ssh.User != "" {
				host = ssh.User + "@" + host
			}
			spec, err := remote.ParseHostSpec(host)
			if err != nil {
				return err
			}
			if ssh.Port > 0 {
				spec, err = remote.ParseHostSpec(spec.String() + fmt.Sprintf(":%d", ssh.Port))
				if err != nil {
					return err
				}
			}
			opts := append(remote.CISSHFlags(), remote.CIOptions(id, p.PinnedFile, p.Version)...)
			if p.KeyPath != "" {
				opts = append(opts, "-i", p.KeyPath)
			}
			target, err := spec.Target(opts)
			if err != nil {
				return err
			}
			runner = func(ctx context.Context, command string) (string, error) {
				return remote.Run(ctx, target, "sh -c "+remote.ShellQuote(command))
			}
		}
		now := time.Now().UTC()
		c.Resources.CPU = observed[float64](nil, now)
		c.Resources.MemMB = observed[int64](nil, now)
		c.Availability.BatteryPct = observed[int](nil, now)
		c.Availability.PluggedIn = observed[bool](nil, now)
		c.Availability.Interactive = observed[string](nil, now)
		return collect(ctx, c, runner, "", now, false)
	})
}
