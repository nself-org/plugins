package main

// Purpose: the node-side secret resolver of `pull` (P7-NODE-08, X8): the lease carries references, this node
//          resolves them with its own `nself config secrets resolve` under its own age identity.
// Inputs:  --project, --secrets-eligible, --dedicated; the node's age key path.
// Outputs: a agent.SecretResolver; values live only in the returned map and the session masks them first.
// Constraints: a node that was not marked --secrets-eligible (E677 secrets.node_not_eligible), has no
//          --project or no age identity resolves nothing (the session then skips the job as secrets_missing);
//          a TEAM coordinator's lease reaches this resolver with references only (D-1534); release and deploy classes need
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

// nodeSecretsRunner runs this node's own nself cli; a variable so a test can stand in a fake cli.
var nodeSecretsRunner = func(state, coordinator, attemptID string) secrets.Runner {
	return &secrets.ExecRunner{Home: state, CoordinatorID: coordinator, Attempt: attemptID}
}

func nodeResolver(state string, f pullFlags, nodeID, agePath string) agent.SecretResolver {
	return func(ctx context.Context, attemptID string, class model.TrustClass, refs []string) (map[string]string, error) {
		if f.project == "" {
			return nil, errors.New("this node was not started with --project")
		}
		// a node not marked --secrets-eligible is refused by the Source (E677) before any cli call
		src := secrets.New(secrets.Config{
			Mode: secrets.ModeAgentNode, Attempt: attemptID,
			Resolver: &secrets.Resolver{Runner: nodeSecretsRunner(state, f.coordinator, attemptID), Project: f.project, AgeKeyPath: agePath},
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
