package probe

// Purpose: the status reads of `ci nodes status` (P7-AUTO-19): the watch host's demand document
// (contract:ci.demand v1) and its readiness block, read over the same SDK remote funnel and D4 option
// set as the capability probe.
// Inputs:  a registered node (its SSH identity), the pin file, an optional key and the local ssh version.
// Outputs: StatusDocs with the raw documents; an empty document means the host publishes none.
// Constraints: both commands are literals. The CI home is expanded by the remote shell from its own
//          environment; no node, registry or inventory value enters a remote command. A missing file is
//          not an error (only watch hosts publish demand.json); a transport failure is.

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

const (
	cmdDemand    = `cat "${NSELF_CI_HOME:-$HOME/.nself/ci}/demand.json" 2>/dev/null || true`
	cmdReadiness = `cat "${NSELF_CI_HOME:-$HOME/.nself/ci}/readiness.json" 2>/dev/null || true`
)

var statusCommands = []string{cmdDemand, cmdReadiness}

// StatusCommands returns the fixed read-only commands of a status read, in the order they run.
func StatusCommands() []string { return append([]string(nil), statusCommands...) }

// StatusRunner runs one literal command on a node and returns its output.
type StatusRunner func(ctx context.Context, command string) (string, error)

// StatusDocs are the raw documents a node publishes; nil when the node has none.
type StatusDocs struct {
	Demand    []byte
	Readiness []byte
}

// ReadStatus runs the two literal reads with run. The first transport error ends the read.
func ReadStatus(ctx context.Context, run StatusRunner) (StatusDocs, error) {
	var docs StatusDocs
	if run == nil {
		return docs, fmt.Errorf("status read requires a runner")
	}
	for _, command := range statusCommands {
		out, err := run(ctx, command)
		if err != nil {
			return StatusDocs{}, err
		}
		if ctx.Err() != nil {
			return StatusDocs{}, ctx.Err()
		}
		var body []byte
		if out != "" {
			body = []byte(out)
		}
		if command == cmdDemand {
			docs.Demand = body
		} else {
			docs.Readiness = body
		}
	}
	return docs, nil
}

// SSHStatusRunner builds the runner of one registered SSH node with the pinned D4 option set.
// The node's address picks the destination only; it never enters the command.
func SSHStatusRunner(n registry.Node, pinnedFile, keyPath string, v remote.Version) (StatusRunner, error) {
	ssh := n.Capability.Identity.SSH
	if ssh == nil {
		return nil, fmt.Errorf("no SSH identity registered (agent nodes report through their agent)")
	}
	if pinnedFile == "" {
		return nil, fmt.Errorf("status read requires the pinned host key file")
	}
	host := ssh.Alias
	if host == "" {
		host = ssh.Hostname
	}
	if ssh.User != "" {
		host = ssh.User + "@" + host
	}
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return nil, err
	}
	if ssh.Port > 0 {
		if spec, err = remote.ParseHostSpec(spec.String() + fmt.Sprintf(":%d", ssh.Port)); err != nil {
			return nil, err
		}
	}
	opts := append(remote.CISSHFlags(), remote.CIOptions(n.Record.ID, pinnedFile, v)...)
	if keyPath != "" {
		opts = append(opts, "-i", keyPath)
	}
	target, err := spec.Target(opts)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, command string) (string, error) {
		if !statusCommand(command) {
			return "", fmt.Errorf("refused: not a fixed status command")
		}
		return remote.Run(ctx, target, "sh -c "+remote.ShellQuote(command))
	}, nil
}

// statusCommand reports whether command is one of the literal status reads.
func statusCommand(command string) bool {
	for _, c := range statusCommands {
		if command == c {
			return true
		}
	}
	return false
}
