package runners

// Purpose: Poller, the pending-job poll of the GitLab runner demand (Epic D11, debt D-1389). The Job Hook
// is sent once per status and the ingress answers a declined demand with a 200, so a demand the handler
// turned away for a cap (E703) or because the ledger was not bound yet (E704) would stay pending until
// GitLab's own stuck-job timeout. Poller lists the pending jobs of each named project and offers every
// one that carries the nself tag to the demand handler again, as github/reconcile does for GitHub.
// Inputs:  GET /projects/{id}/jobs?scope[]=pending under the runner-management token
//          (providers.gitlab.runner_token_env, which holds read_api); the projects the operator named in
//          the GitLab webhook secret keys.
// Outputs: one runner_demand Delivery (source gitlab, facts sourced from the Jobs API, auth per-repo) per
//          job the handler accepts, oldest job first.
// Constraints: stateless. The ledger keeps one slot per demand (Reserve is idempotent) and the intake
//          deduplicates a delivery on its synthetic pipeline, so a job offered every tick, or by the Job
//          Hook as well, costs one slot and one pipeline. A refusal (a tag outside the fixed set, a bad
//          pin) is not an error and is offered again only while the job is pending. Transient and quota
//          answers of the handler end that job's turn, never the pass. One list request per project per
//          tick, at most maxPages pages; a quota answer from GitLab ends the pass. Projects come only from
//          the GitLab secret keys, never from the shared allowlist, so a GitHub repository with the same
//          path is never looked up on GitLab. Dormant unless compat.V15().

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/webhook"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

// Poll bounds and the Reconciler's name.
const (
	// PollName is the coordinator Reconciler name.
	PollName = "gitlab-demand-poll"
	// PollEvery is the interval between ticks.
	PollEvery    = 30 * time.Second
	jobsPerPage  = 50 // a GitLab job object is several KB and the funnel caps a body at 1 MiB
	maxPages     = 4
	pendingScope = "pending"
)

// Poller is the coordinator Reconciler of the GitLab pending-job poll. The zero value uses the operator's
// settings, the GitLab secret keys and the registered demand handler.
type Poller struct {
	Client   func(context.Context) (*rest.Client, error) // default: the runner-management token client
	Projects func(context.Context) ([]string, error)     // default: projects named in the GitLab secret keys
	Handler  providers.DemandHandler                     // default providers.DemandHandlerFor("gitlab")
	Now      func() time.Time                            // default time.Now
}

var (
	_ coordinator.Reconciler = (*Poller)(nil)
	_ coordinator.Everyer    = (*Poller)(nil)
)

// Name implements coordinator.Reconciler.
func (*Poller) Name() string { return PollName }

// Every implements coordinator.Everyer.
func (*Poller) Every() time.Duration { return PollEvery }

type pendingJob struct {
	ID      int64    `json:"id"`
	Status  string   `json:"status"`
	TagList []string `json:"tag_list"`
	Commit  struct {
		ID string `json:"id"`
	} `json:"commit"`
	Pipeline struct {
		ID int64 `json:"id"`
	} `json:"pipeline"`
}

// Reconcile implements coordinator.Reconciler.
func (p *Poller) Reconcile(ctx context.Context, enq coordinator.Enqueuer) error {
	if !compat.V15() {
		return nil
	}
	h := p.Handler
	if h == nil {
		h = providers.DemandHandlerFor(Source)
	}
	if h == nil {
		return nil // nobody to hand a demand to yet: read nothing
	}
	projects, err := p.projects(ctx)
	if err != nil || len(projects) == 0 {
		return err
	}
	c, err := p.client(ctx)
	if err != nil || c == nil {
		return err
	}
	var errs []error
	for _, project := range projects {
		err = p.pollProject(ctx, c, h, enq, project)
		var pe *providers.Error
		if errors.As(err, &pe) && pe.Class == providers.Quota {
			return errors.Join(append(errs, err)...)
		}
		if err != nil {
			errs = append(errs, err)
		}
		if ctx.Err() != nil {
			break
		}
	}
	return errors.Join(errs...)
}

func (p *Poller) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func (p *Poller) client(ctx context.Context) (*rest.Client, error) {
	if p.Client != nil {
		return p.Client(ctx)
	}
	rs, err := runtime.CurrentSettings(ctx)
	if err != nil || rs.GitLabRunnerTokenEnv == "" { // no runner token: the minter answers E700, nothing to poll with
		return nil, err
	}
	return NewClient(rs)
}

// projects lists the projects to poll: the group/project names the operator gave a GitLab webhook secret.
func (p *Poller) projects(ctx context.Context) ([]string, error) {
	if p.Projects != nil {
		return p.Projects(ctx)
	}
	eff, err := currentPolicy(ctx)
	if err != nil || eff == nil {
		return nil, err
	}
	return projectsOf(eff)
}

func projectsOf(eff *policy.Effective) ([]string, error) {
	secrets, err := webhook.SecretsFrom(eff)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string]webhook.RepoSecret{secrets.Signing, secrets.Tokens} {
		for name := range m {
			k := strings.ToLower(name)
			if seen[k] || !coordinator.ValidProject(name) || strings.Count(name, "/") != 1 { // a subgroup project is E702 (D-1365)
				continue
			}
			seen[k] = true
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out, nil
}

// pollProject offers the pending nself jobs of one project, oldest first.
func (p *Poller) pollProject(ctx context.Context, c *rest.Client, h providers.DemandHandler, enq coordinator.Enqueuer, project string) error {
	pr, ok := api.ParseProject(project)
	if !ok {
		return nil
	}
	var jobs []pendingJob
	for page := 1; page <= maxPages; page++ {
		var rows []pendingJob
		q := url.Values{"scope[]": {pendingScope}, "per_page": {strconv.Itoa(jobsPerPage)}, "page": {strconv.Itoa(page)}}
		if _, err := c.DoRequest(ctx, rest.Request{Method: "GET", Path: "/projects/{id}/jobs", Params: pr.Params(), Query: q}, &rows); err != nil {
			return err
		}
		jobs = append(jobs, rows...)
		if len(rows) < jobsPerPage {
			break
		}
	}
	slices.SortFunc(jobs, func(a, b pendingJob) int { return int(a.ID - b.ID) })
	var errs []error
	for _, j := range jobs {
		if j.Status != pendingScope || !slices.Contains(j.TagList, DemandTag) {
			continue
		}
		if err := p.offer(ctx, h, enq, project, j); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// offer hands one job to the handler. A refusal or a "try again" from the handler is not an error: the job
// stays pending on GitLab and is offered again on the next tick.
func (p *Poller) offer(ctx context.Context, h providers.DemandHandler, enq coordinator.Enqueuer, project string, j pendingJob) error {
	rev := strings.ToLower(j.Commit.ID)
	if j.ID <= 0 || j.Pipeline.ID <= 0 || !coordinator.ValidRevision(rev) || len(rev) != 40 {
		return nil
	}
	f := p.facts(project, rev, j)
	if err := f.Validate(); err != nil {
		return nil
	}
	pipe, err := h.Synthesize(ctx, f)
	if err != nil || pipe == nil {
		return nil
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	_, err = enq.Enqueue(ctx, coordinator.Delivery{Source: Source, DeliveryID: f.DeliveryID, Project: project, Revision: rev,
		TriggerClass: "runner_demand", Facts: raw, Synthetic: pipe})
	return err
}

// facts are the facts of one pending job, read from the Jobs API. As in the Job Hook source there is no
// Ref: the job's ref is the sender's to choose for a fork merge request and the revision names the code.
func (p *Poller) facts(project, rev string, j pendingJob) model.TriggerFacts {
	now := p.now()
	apiFact := func(v string) model.Fact[string] {
		return model.Fact[string]{Value: &v, Source: "api", ObservedAt: now, Confidence: "known"}
	}
	tags := append([]string{}, j.TagList...)
	return model.TriggerFacts{
		Schema: "ci.trigger-facts/v1", Source: Source, Kind: "runner_demand", DeliveryID: "poll:job:" + strconv.FormatInt(j.ID, 10),
		ReceivedAt: now, Auth: "per-repo", Repo: project, Revision: rev, RevisionSource: model.FactPayload,
		RunnerLabels:  model.Fact[[]string]{Value: &tags, Source: "api", ObservedAt: now, Confidence: "known"},
		ProviderJobID: apiFact(strconv.FormatInt(j.ID, 10)), ProviderRunID: apiFact(strconv.FormatInt(j.Pipeline.ID, 10)),
	}
}
