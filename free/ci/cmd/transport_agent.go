package main

// Purpose: the composition root of the `agent` transport (carrier B, pull). It links the driver into the
//          `nself ci` binary so transport.Lookup("agent") finds it, and wires it to the agent listener when
//          `ci serve` starts one: the token gate and signer, the lease source, the run-facing sink with the
//          durable per-lease logs, and the cancel path (P7-NODE-08).
// Inputs:  the listener's CA, store and data dir (agentDeps).
// Outputs: agentapi.Config.{Sink, Source, Admit} set; agent.Default wired.
// Constraints: the job token is signed with the coordinator's token key and gated on every lease-scoped call;
//          the lease logs live under <data dir>/agent-logs (0700); the sink commits results through the run's
//          sink, so the listener does not commit them a second time; nothing here sends a secret value (the
//          lease carries references, the node resolves them).

import (
	"context"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

func init() { agentWire = wireAgentTransport }

// agentLogDir is the directory of the per-lease logs under the coordinator's data dir.
func agentLogDir(dataDir string) string { return filepath.Join(dataDir, "agent-logs") }

func wireAgentTransport(cfg *agentapi.Config, d agentDeps) (func(*agentapi.Server), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tokens, err := d.CA.Tokens(ctx)
	if err != nil {
		return nil, err
	}
	gate := &agent.Gate{Tokens: tokens, Bindings: d.Store, Leases: d.Store}
	drv := agent.Default
	drv.Wire(agent.Wiring{Gate: gate, Minter: tokens, Leases: d.Store, LogDir: agentLogDir(d.DataDir), Logf: agentLogf})
	cfg.Sink, cfg.Source, cfg.Admit, cfg.Blobs = drv.Sink(), drv, drv.Admit, drv.Blobs()
	return func(s *agentapi.Server) { drv.SetCancel(s.Cancel); drv.SetPush(s.Push) }, nil
}
