package adhoc

// Purpose: the network of a container job that uses --service (Epic D9): an
// internal, per-attempt Docker network that holds the job and the service
// containers and nothing else, so the job reaches `postgres:5432` by service
// name and has no route to the internet or the LAN.
// Inputs:  the attempt id and the compose network the services hook started
// the services on (NSELF_CI_SERVICE_NETWORK).
// Outputs: the network name (nself-ci-<attempt>, the form the container
// backstop accepts) after every service container joined it under its compose
// service name.
// Constraints: every Docker call goes through the exec funnel (services.Runner);
// the network is `--internal` (no gateway) and labelled so a crash leaves
// something Prune removes; Remove is idempotent and runs with a context that
// survives Ctrl-C. A compose network is never given to the job itself: it has
// a gateway, and the service containers keep it for their own traffic only.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/services"
)

// netLabel marks the networks this package creates.
const netLabel = "nself.ci.adhoc=1"

// staleAfter is how long an unused labelled network is left before Prune
// removes it (a job holds its network for at most the job timeout).
const staleAfter = "3h"

// serviceNet creates and removes the per-attempt service networks.
type serviceNet struct {
	docker services.Runner
	mu     sync.Mutex
	made   []string
}

func (n *serviceNet) run(ctx context.Context, argv ...string) (string, error) {
	res, err := n.docker.Run(ctx, services.Call{Argv: append([]string{"docker"}, argv...)})
	if err != nil {
		return res.Output, err
	}
	if res.ExitCode != 0 {
		return res.Output, fmt.Errorf("docker %s exited %d: %s", argv[0], res.ExitCode, strings.TrimSpace(tailOf(res.Output)))
	}
	return res.Output, nil
}

func tailOf(s string) string {
	if len(s) > 300 {
		return s[len(s)-300:]
	}
	return s
}

// Ensure creates the internal network for attempt and attaches the service
// containers found on the compose network.
func (n *serviceNet) Ensure(ctx context.Context, attempt, compose string) (string, error) {
	if compose == "" {
		return "", noIsolation("the services hook did not name a service network (NSELF_CI_SERVICE_NETWORK); a container job cannot reach the services")
	}
	_, _ = n.run(ctx, "network", "prune", "--force", "--filter", "label="+netLabel, "--filter", "until="+staleAfter)
	name := "nself-ci-" + attempt
	if _, err := n.run(ctx, "network", "create", "--internal", "--label", netLabel, "--label", "nself.ci.attempt="+attempt, name); err != nil {
		return "", noIsolation("cannot create the service network: %v", err)
	}
	n.mu.Lock()
	n.made = append(n.made, name)
	n.mu.Unlock()
	out, err := n.run(ctx, "network", "inspect", "--format", "{{range .Containers}}{{.Name}}\n{{end}}", compose)
	if err != nil {
		return "", noIsolation("cannot list the service containers: %v", err)
	}
	joined := 0
	for _, c := range strings.Fields(out) {
		svc, err := n.run(ctx, "inspect", "--format", `{{index .Config.Labels "com.docker.compose.service"}}`, c)
		alias := strings.TrimSpace(svc)
		if err != nil || alias == "" {
			continue // not a compose service container
		}
		if _, err := n.run(ctx, "network", "connect", "--alias", alias, name, c); err != nil {
			return "", noIsolation("cannot attach service %s to the job network: %v", alias, err)
		}
		joined++
	}
	if joined == 0 {
		return "", noIsolation("no service container found on network %s", compose)
	}
	return name, nil
}

// Remove deletes every network this attempt set made.
func (n *serviceNet) Remove(ctx context.Context) {
	n.mu.Lock()
	made := n.made
	n.made = nil
	n.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	for _, name := range made {
		_, _ = n.run(ctx, "network", "rm", name)
	}
}
