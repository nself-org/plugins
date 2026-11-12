package main

// Purpose: bind TEAM dispatch to verified forge provenance and live release authority.
// Inputs: durable intake and the selected plan. Outputs: a pinned classifier and permission.
// Constraints: cached authority claims and the local checkout never raise submitted trust.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/serve"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

// Dispatch seams keep security wiring testable without cloning or executing submitted code.
var (
	dispatchCloneRef    = serve.CloneRef
	dispatchPreparePlan = preparePlan
	dispatchRun         = func(ctx context.Context, c *cirun.Coordinator) (cirun.Result, error) { return c.Run(ctx) }
)

type forgeDispatchTrust struct {
	facts           model.TriggerFacts
	hook            cirun.SourceTrust
	home, submitter string
}

// Classify verifies the checkout SHA matches the intake before asking the forge.
func (h forgeDispatchTrust) Classify(ctx context.Context, r cirun.TrustRequest) (model.TrustClass, error) {
	if !strings.EqualFold(r.Revision, h.facts.Revision) || r.Revision == "" {
		return "untrusted", fmt.Errorf("submitted revision does not match checkout")
	}
	if h.facts.Source == "manual" && h.facts.Kind == "api" {
		return h.classifyAPI(ctx, r)
	}
	got, err := h.hook.ClassifyForge(ctx, r.Root, h.facts)
	if h.facts.Kind == "pull_request" {
		if err != nil {
			return "untrusted", err
		}
		return pullRequestClass(got), nil // approval is not recorded by this dispatch path
	}
	return got.Class, err
}

// pullRequestClass admits a pull request at one of two classes (ADR 0025 §1, §6). A same-repo PR
// from an admin whose only ceiling is provenance gets "collaborator": its own class, never owner,
// so repo secrets stay withheld. Every other PR (fork, write-access author, first-timer, bot,
// lockfile-only, unverified facts, machine cap) is "untrusted". The coordinator then admits either
// class only on a disposable hosted runner (Option A); a PR never gets owner from this path.
func pullRequestClass(r source.Result) model.TrustClass {
	if r.Class != "owner" && r.Class != "collaborator" {
		return "untrusted"
	}
	for _, reason := range r.Reasons {
		if reason != source.ReasonVerified && reason != source.ReasonProvenance {
			return "untrusted"
		}
	}
	return "collaborator"
}

// classifyAPI combines current TEAM access with forge-proven default-branch reachability.
// A member may run default-branch code, but cannot gain owner or release authority.
func (h forgeDispatchTrust) classifyAPI(ctx context.Context, r cirun.TrustRequest) (model.TrustClass, error) {
	st, err := cirun.OpenStore(filepath.Join(h.home, "state.db"))
	if err != nil {
		return "untrusted", err
	}
	defer st.Close()
	svc := roles.New(st, &audit.Log{St: st})
	actor, err := svc.ActorFromSubmitter(ctx, h.submitter)
	if err != nil {
		return "untrusted", err
	}
	// Revalidate the captured role as well as scope so a concurrent downgrade
	// cannot leave an old admin snapshot supplying owner trust.
	if err := roles.Authorize(ctx, actor, actor.Role(), h.facts.Repo); err != nil {
		return "untrusted", err
	}
	hook := h.hook
	refetch := hook.Refetch
	if refetch == nil {
		refetch = refetchDispatchAPI
	}
	facts := h.facts
	// Manual API submissions use the same GitHub repository as SafeCloneURL.
	facts.Source, facts.Auth = "github", "per-repo"
	facts.Actor = model.Fact[string]{}
	hook.Refetch = func(ctx context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
		fresh, err := refetch(ctx, f)
		if err != nil {
			return f, err
		}
		// This permission comes from the live local grant, not a forge login claim.
		permission := "admin"
		fresh.ActorPermission = model.Fact[string]{Value: &permission, Source: "local"}
		return fresh, nil
	}
	got, err := hook.ClassifyForge(ctx, r.Root, facts)
	if actor.Role() != roles.Admin && got.Class == "owner" {
		got.Class = "internal"
	}
	return got.Class, err
}

// refetchDispatchAPI requires an installed trust provider before using the API route.
func refetchDispatchAPI(ctx context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
	p, ok := providers.Lookup(f.Source)
	if _, can := p.(providers.TrustFacts); !ok || !can {
		return f, fmt.Errorf("no forge trust provider for %s", f.Source)
	}
	return providers.RefetchFacts(ctx, f)
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

// authorizeDispatchRelease checks every selected job before the coordinator can execute any. A submitter
// without release.start is refused a job that reads secrets with E677 (secrets.RefuseSubmitter); an
// authorized TEAM run's secrets are then resolved on the leased node, never here (D-1534).
func authorizeDispatchRelease(ctx context.Context, st *store.Store, submitter, project string, p *plan.Plan) (bool, error) {
	if submitter == "" {
		return false, nil
	}
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
			if j.Job.Kind != "release" { // a job that only reads secrets: E677 before any lease, no value read (D-1534)
				return false, secrets.RefuseSubmitter(err.Error())
			}
			return false, err
		}
		return true, nil
	}
	return false, nil
}
