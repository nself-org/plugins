package reconcile

// Purpose: the trigger facts a reconciliation enqueues, and the step from facts to a coordinator.Delivery
// (the same step the runtime ingress adapter takes for a verified webhook, including the runner-demand
// handler call that fills Delivery.Synthetic).
// Inputs:  repository, head or job data read by the coordinator's own token.
// Outputs: model.TriggerFacts (source "github", auth "per-repo", every API-read claim stamped source api) and
//          coordinator.Delivery.
// Constraints: the delivery id is derived from the repository and revision (or job), so the same head or
//          job is one intake row however often it is seen. A revision read through the API is stamped
//          RevisionSource "payload" because the model allows only payload or caller, as the webhook source
//          does for a tag it resolved itself. A pull request's Ref is refs/pull/N/head; a runner demand
//          carries no Ref (a branch name is the sender's choice).

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// ErrDeclined means the demand handler (or its absence) declined a job; the job is not retried.
var ErrDeclined = errors.New("reconcile: runner demand declined")

func apiFact[T any](v T, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "api", ObservedAt: now, Confidence: "known"}
}

func baseFacts(repo string, kind model.TriggerKind, deliveryID, revision string, now time.Time) model.TriggerFacts {
	return model.TriggerFacts{
		Schema: "ci.trigger-facts/v1", Source: Source, Kind: kind, DeliveryID: deliveryID, ReceivedAt: now, Auth: "per-repo",
		Repo: repo, Revision: revision, RevisionSource: model.FactPayload,
	}
}

// repoInfo is what the repository object tells every head of the repository.
type repoInfo struct {
	ID            int64  `json:"id"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Visibility    string `json:"visibility"`
}

func (r repoInfo) visibility() (string, bool) {
	switch r.Visibility {
	case "public", "private", "internal":
		return r.Visibility, true
	}
	if r.ID > 0 && r.Private {
		return "private", true
	}
	return "", false
}

// head is one commit the head scan may enqueue.
type head struct {
	SHA       string
	Branch    string // push: the branch name
	Protected bool
	PR        *pullInfo // pull_request: the pull request
}

// pullInfo is the part of a pull request list entry the facts use.
type pullInfo struct {
	Number            int    `json:"number"`
	AuthorAssociation string `json:"author_association"`
	Head              struct {
		SHA  string `json:"sha"`
		Repo *struct {
			ID       int64  `json:"id"`
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
}

func headFacts(repo string, info repoInfo, h head, now time.Time) model.TriggerFacts {
	kind := model.TriggerKind("push")
	if h.PR != nil {
		kind = "pull_request"
	}
	f := baseFacts(repo, kind, "reconcile:"+lower(repo)+":"+h.SHA, h.SHA, now)
	if info.ID > 0 {
		f.RepoID = strconv.FormatInt(info.ID, 10)
	}
	if v, ok := info.visibility(); ok {
		f.Visibility = apiFact(v, now)
	}
	f.HeadCurrent = apiFact(true, now) // read from the API moments ago
	if h.PR == nil {
		f.Ref = "refs/heads/" + h.Branch
		f.ProtectedRef = apiFact(h.Protected, now)
		return f
	}
	p := h.PR
	f.Ref, f.PRNumber = "refs/pull/"+strconv.Itoa(p.Number)+"/head", p.Number
	f.BaseRef = "refs/heads/" + p.Base.Ref
	f.BaseRevision = p.Base.SHA
	if p.AuthorAssociation != "" {
		f.AuthorAssociation = apiFact(p.AuthorAssociation, now)
	}
	if hr := p.Head.Repo; hr != nil && hr.FullName != "" && hr.ID > 0 && info.ID > 0 { // a deleted fork names no head repository: unknown, not "not a fork"
		f.HeadRepo = apiFact(hr.FullName, now)
		f.Fork = apiFact(hr.ID != info.ID, now)
	}
	return f
}

// delivery turns validated facts into a Delivery. For a runner demand it calls the registered handler;
// a missing handler or a handler that refuses is ErrDeclined.
func delivery(ctx context.Context, f model.TriggerFacts) (coordinator.Delivery, error) {
	if err := f.Validate(); err != nil {
		return coordinator.Delivery{}, err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return coordinator.Delivery{}, err
	}
	class := "change"
	if f.Kind == "runner_demand" {
		class = "runner_demand"
	}
	d := coordinator.Delivery{Source: f.Source, DeliveryID: f.DeliveryID, Project: f.Repo, Revision: f.Revision, TriggerClass: class, Facts: raw}
	if f.Kind != "runner_demand" {
		return d, nil
	}
	h := providers.DemandHandlerFor(f.Source)
	if h == nil {
		return d, ErrDeclined
	}
	p, err := h.Synthesize(ctx, f)
	if err != nil || p == nil {
		return d, ErrDeclined
	}
	d.Synthetic = p
	return d, nil
}

// demandFacts are the facts of one queued job, read from its run object.
func demandFacts(repo string, now time.Time, run ghRun, j ghJob) model.TriggerFacts {
	id := strconv.FormatInt(j.ID, 10)
	f := baseFacts(repo, "runner_demand", "poll:job:"+id, strings.ToLower(run.HeadSHA), now)
	if run.Repository.ID > 0 {
		f.RepoID = strconv.FormatInt(run.Repository.ID, 10)
		if run.Repository.Private != nil {
			f.Visibility = apiFact(map[bool]string{true: "private", false: "public"}[*run.Repository.Private], now)
		}
	}
	f.RunnerLabels = apiFact(append([]string{}, j.Labels...), now)
	f.ProviderJobID = apiFact(id, now)
	f.ProviderRunID = apiFact(strconv.FormatInt(run.ID, 10), now)
	if run.Event != "" {
		f.RunEvent = apiFact(run.Event, now)
	}
	if h := run.HeadRepository; h != nil && h.FullName != "" && h.ID > 0 && run.Repository.ID > 0 {
		f.HeadRepo = apiFact(h.FullName, now)
		f.Fork = apiFact(h.ID != run.Repository.ID, now) // another repository, not "a repository that is a fork"
	}
	if run.Actor.Login != "" {
		f.Actor = apiFact(run.Actor.Login, now)
	}
	return f
}
