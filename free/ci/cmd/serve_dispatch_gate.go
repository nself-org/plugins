package main

// Purpose: the dispatch gate of D-1337. A run submitted through the user API keeps the submitter's audit
// label in the trigger facts; before the run starts, a release-kind job or a secret reference needs the
// submitter to pass roles.AuthorizeAction(release.start) again, so a role lowered or revoked since the
// submit, or a Member who submitted a commit whose own .nself/ci.yaml defines a release job, gets no run.
// Inputs:  the roles Service, the stored trigger facts, the project and the built plan.
// Outputs: nil, or a refusal that fails the dispatch (the run never starts).
// Constraints: a webhook trigger has no submitter and is covered by the source-trust classifier; a plan
//          without release or secret work is not gated; with no roles Service the gate fails closed.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

// submitterGate refuses release-kind or secret-referencing work for a submitter without release.start.
func submitterGate(ctx context.Context, svc *roles.Service, facts pipelineFacts, project string, p *plan.Plan) error {
	if facts.Submitter == "" || !needsRelease(p) {
		return nil
	}
	if svc == nil {
		return errors.New("refused: the plan has release or secret work and the roles service is not available to authorize the submitter")
	}
	label, _, _ := strings.Cut(facts.Submitter, " role=") // the stored role is a claim; the current grant decides
	a, err := svc.ActorFromLabel(ctx, label)
	if err == nil {
		err = roles.AuthorizeAction(ctx, a, roles.ActReleaseStart, project)
	}
	if err != nil {
		return fmt.Errorf("refused: release or secret work needs release.start and the submitter does not hold it: %w", err)
	}
	return nil
}

// needsRelease reports whether any planned job is a release job or references a secret.
func needsRelease(p *plan.Plan) bool {
	if p == nil {
		return false
	}
	for _, pj := range p.Jobs {
		if pj.Job.Kind == "release" || len(pj.Job.Secrets) > 0 {
			return true
		}
	}
	return false
}
