package main

// Purpose: bind TEAM dispatch to verified forge provenance and live release authority.
// Inputs: durable intake and the selected plan. Outputs: a pinned classifier and permission.
// Constraints: cached authority claims and the local checkout never raise submitted trust.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/serve"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Dispatch seams keep security wiring testable without cloning or executing submitted code.
var (
	dispatchCloneRef    = serve.CloneRef
	dispatchPreparePlan = preparePlan
	dispatchRun         = func(ctx context.Context, c *cirun.Coordinator) (cirun.Result, error) { return c.Run(ctx) }
)

type forgeDispatchTrust struct {
	facts model.TriggerFacts
	hook  cirun.SourceTrust
}

// Classify verifies the checkout SHA matches the intake before asking the forge.
func (h forgeDispatchTrust) Classify(ctx context.Context, r cirun.TrustRequest) (model.TrustClass, error) {
	if !strings.EqualFold(r.Revision, h.facts.Revision) || r.Revision == "" {
		return "untrusted", fmt.Errorf("submitted revision does not match checkout")
	}
	got, err := h.hook.ClassifyForge(ctx, r.Root, h.facts)
	if h.facts.Kind == "pull_request" && got.Class != "owner" {
		return "untrusted", err // an unreviewed head never inherits repository collaborator authority
	}
	return got.Class, err
}

// dispatchTrustFacts retains only query inputs and payload claims, never stored authority.
func dispatchTrustFacts(row store.IntakeRow, legacy pipelineFacts) (model.TriggerFacts, error) {
	var f model.TriggerFacts
	if err := json.Unmarshal(row.Facts, &f); err != nil {
		return f, fmt.Errorf("invalid trigger facts: %w", err)
	}
	if f.Schema == "" {
		f = model.TriggerFacts{Schema: "ci.trigger-facts/v1", Kind: model.TriggerKind(legacy.Trigger), Auth: "none"}
	}
	if f.Source != "" && f.Source != row.Source || f.Repo != "" && f.Repo != row.Project || f.Revision != "" && !strings.EqualFold(f.Revision, row.Revision) {
		return f, fmt.Errorf("trigger facts do not match intake source, repository or revision")
	}
	// Actor is a query input for the permission API. Everything the forge can
	// attest must be fetched again; partial API failure cannot reuse stale facts.
	actor := f.Actor
	actor.Source = "payload"
	f = model.TriggerFacts{Schema: f.Schema, Source: row.Source, Repo: row.Project, Revision: row.Revision,
		Kind: f.Kind, Auth: f.Auth, Ref: f.Ref, BaseRef: f.BaseRef, BaseRevision: f.BaseRevision,
		PRNumber: f.PRNumber, Actor: actor}
	return f, f.Validate()
}

// authorizeDispatchRelease checks every selected job before the coordinator can execute any.
func authorizeDispatchRelease(ctx context.Context, st *store.Store, submitter, project string, p *plan.Plan) (bool, error) {
	for _, j := range p.Jobs {
		if j.Job.Kind != "release" && len(j.Job.Secrets) == 0 {
			continue
		}
		svc := roles.New(st, &audit.Log{St: st})
		actor, err := svc.ActorFromSubmitter(ctx, submitter)
		if err != nil {
			return false, err
		}
		if err := roles.AuthorizeAction(ctx, actor, roles.ActReleaseStart, project); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
