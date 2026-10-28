package main

// Purpose: the node-side secret resolver of `pull` (P7-NODE-08, X8): the lease carries references, this node
//          resolves them with its own `nself config secrets resolve` under its own age identity.
// Inputs:  --project, --secrets-eligible, --dedicated; the node's age key path.
// Outputs: a agent.SecretResolver; values live only in the returned map and the session masks them first.
// Constraints: a node that was not marked --secrets-eligible, has no --project or no age identity resolves
//          nothing (the session then skips the job as secrets_missing); release and deploy classes need
//          --dedicated; the job's trust class is the one the lease carries (an absent or unknown class is
//          untrusted, so nothing resolves); the error text carries codes and refs, never a value.

import (
	"context"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// noRegistry takes the place of the redaction registry: the session masks every received value itself,
// before the job can see it.
type noRegistry struct{}

func (noRegistry) Register([]byte) error { return nil }

func nodeResolver(state string, f pullFlags, nodeID, agePath string) agent.SecretResolver {
	return func(ctx context.Context, attemptID string, class model.TrustClass, refs []string) (map[string]string, error) {
		if f.project == "" || !f.secretsOK {
			return nil, errors.New("this node was not started with --project and --secrets-eligible")
		}
		src := secrets.New(secrets.Config{
			Mode: secrets.ModeAgentNode, Attempt: attemptID,
			Resolver: &secrets.Resolver{Runner: &secrets.ExecRunner{Home: state, CoordinatorID: f.coordinator, Attempt: attemptID}, Project: f.project, AgeKeyPath: agePath},
			Registry: noRegistry{}, RedactorInstalled: func() bool { return true },
			Facts: secrets.Facts{
				Job:    trust.JobFacts{Trust: trust.DecodeTrust(string(class))}, // the class the coordinator admitted the job at; none or unknown is untrusted
				Runner: trust.RunnerFacts{Dedicated: f.dedicated, Isolation: model.Isolation("process")},
			},
			NodeID: nodeID, NodeEligible: f.secretsOK, AgeKeyPath: agePath,
		})
		defer src.Zero()
		return src.Resolve(ctx, refs)
	}
}
