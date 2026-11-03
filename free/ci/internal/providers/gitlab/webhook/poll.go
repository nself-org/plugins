package webhook

// Purpose: the GitLab pending-job poll (debt D-1389). GitLab sends a Job Hook once per job status and the
// ingress answers a declined demand with a 200, so a demand the handler declines for a retryable reason
// (E703 runner.demand_capped while max_pending jobs are live, E704 before the ledger is bound) would stay
// pending until GitLab's own stuck-job timeout. Each tick lists the pending jobs of every configured
// project and offers each one that carries the nself tag to the registered DemandHandler, the contract
// the ingress uses, then enqueues the delivery the handler accepts.
// Inputs:  the configured projects (providers.gitlab.webhook_signing_secrets and webhook_tokens), GET
//          /projects/{id}/jobs?scope[]=pending, the "gitlab" DemandHandler, the coordinator Enqueuer.
// Outputs: one runner_demand Delivery per job: source gitlab, delivery id poll:job:<id>, auth per-repo (the
//          coordinator's own token read every fact, so none is a sender's claim), facts sourced api, no Ref,
//          and the handler's synthetic pipeline.
// Constraints: one request per project per tick, sent conditionally (If-None-Match); on a 304 the jobs of
//          the last read are reused, so an unchanged list costs no further reads. A full page (100) is used
//          but its ETag is not kept, so the next tick reads it again. A job is handed over at most once per
//          status (a mark per job id and status), also when the handler refuses it for good; a retryable
//          answer (class Transient, Outage, Quota or Auth: capped, not ready) marks nothing and the job is
//          offered again on the next tick. A rate-limited list (E703) stops the tick. Marks of jobs that left
//          the list are dropped. A project is read by its configured id, else by the id a signed delivery
//          pinned, else by path; a job whose pipeline names another project id ends that project's pass.
//          Nothing is read when compat.V15() is off, no handler is registered or no project is configured.
//          A job the Job Hook already enqueued folds into the same intake row through the semantic key
//          (same project, revision and synthetic pipeline), and the ledger accepts a live demand again
//          without counting it twice. State is in memory: a restart offers pending jobs again, which the
//          same dedup absorbs.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// Poll settings.
const (
	NamePoll       = "gitlab-demand-poll"
	PollEvery      = 30 * time.Second
	pendingPath    = "/projects/{id}/jobs"
	pendingPerPage = 100
)

// Poll is the pending-job Reconciler. The zero value is not usable: call NewPoll.
type Poll struct {
	src     *Source
	Handler func(source string) providers.DemandHandler // default providers.DemandHandlerFor

	mu    sync.Mutex
	lists map[string]pendingList     // project -> last list read
	marks map[string]map[string]bool // project -> "job:status" handed over
}

var (
	_ coordinator.Reconciler = (*Poll)(nil)
	_ coordinator.Everyer    = (*Poll)(nil)
)

// NewPoll returns a Poll over s (its clock, policy and API client).
func NewPoll(s *Source) *Poll {
	return &Poll{src: s, Handler: providers.DemandHandlerFor, lists: map[string]pendingList{}, marks: map[string]map[string]bool{}}
}

// DefaultPoll is the Poll registered with the coordinator.
var DefaultPoll = NewPoll(Default)

// Name implements coordinator.Reconciler.
func (*Poll) Name() string { return NamePoll }

// Every implements coordinator.Everyer.
func (*Poll) Every() time.Duration { return PollEvery }

type pendingJob struct {
	ID      int64    `json:"id"`
	Status  string   `json:"status"`
	TagList []string `json:"tag_list"`
	User    *struct {
		Username string `json:"username"`
	} `json:"user"`
	Pipeline struct {
		ID        int64  `json:"id"`
		ProjectID int64  `json:"project_id"`
		SHA       string `json:"sha"`
	} `json:"pipeline"`
}

type pendingList struct {
	etag string
	jobs []pendingJob
}

// target is one configured project and the id it is bound to (0: none).
type target struct {
	name string
	id   int64
}

// targets are the configured projects, sorted; the signing entry wins for a project listed under both.
func (p *Poll) targets(pol Secrets) []target {
	ids := map[string]int64{}
	for _, m := range []map[string]RepoSecret{pol.Tokens, pol.Signing} {
		for name, rs := range m {
			ids[name] = rs.ID
		}
	}
	out := make([]target, 0, len(ids))
	for name, id := range ids {
		if !validName(name) {
			continue
		}
		if id == 0 {
			id, _ = p.src.pins.pinned(name)
		}
		out = append(out, target{name: name, id: id})
	}
	slices.SortFunc(out, func(a, b target) int { return strings.Compare(a.name, b.name) })
	return out
}

// Reconcile implements coordinator.Reconciler.
func (p *Poll) Reconcile(ctx context.Context, enq coordinator.Enqueuer) error {
	if !compat.V15() {
		return nil
	}
	h := p.Handler(Name)
	if h == nil { // nobody to hand a demand to: read nothing
		return nil
	}
	pol, err := p.src.Policy(ctx)
	if err != nil {
		return err
	}
	targets := p.targets(pol)
	if len(targets) == 0 {
		return nil
	}
	c, err := p.src.api(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var errs []error
	for _, t := range targets {
		err := p.pollProject(ctx, c, h, enq, t)
		if err == nil {
			continue
		}
		errs = append(errs, err)
		if providers.ClassOf(err) == providers.Quota || ctx.Err() != nil { // rate limited: the other projects wait for the next tick
			break
		}
	}
	return errors.Join(errs...)
}

// pollProject reads one project's pending jobs (conditionally) and offers each new one.
func (p *Poll) pollProject(ctx context.Context, c *rest.Client, h providers.DemandHandler, enq coordinator.Enqueuer, t target) error {
	ref := t.name
	if t.id > 0 {
		ref = itoa(t.id)
	}
	prev := p.lists[t.name]
	var jobs []pendingJob
	resp, err := c.DoRequest(ctx, rest.Request{Method: "GET", Path: pendingPath, Params: projectParams(ref),
		Query: url.Values{"scope[]": {"pending"}, "per_page": {strconv.Itoa(pendingPerPage)}}, IfNoneMatch: prev.etag}, &jobs)
	if err != nil {
		return err
	}
	if resp.Status == 304 {
		jobs = prev.jobs
	} else {
		etag := resp.ETag
		if len(jobs) >= pendingPerPage { // a full page may hide more jobs: never remember it as fully read
			etag = ""
		}
		p.lists[t.name] = pendingList{etag: etag, jobs: jobs}
		p.forget(t.name, jobs)
	}
	var firstErr error
	for _, j := range jobs {
		if t.id > 0 && j.Pipeline.ProjectID != 0 && j.Pipeline.ProjectID != t.id {
			return fmt.Errorf("gitlab poll: %s answers as project %d, bound to %d", t.name, j.Pipeline.ProjectID, t.id)
		}
		if err := p.offer(ctx, h, enq, t, j); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// offer hands one job to the handler unless it was handed over at this status already.
func (p *Poll) offer(ctx context.Context, h providers.DemandHandler, enq coordinator.Enqueuer, t target, j pendingJob) error {
	if j.Status != "pending" || j.ID <= 0 || !hasTag(j.TagList, DemandTag) {
		return nil
	}
	key := markKey(j)
	if p.marks[t.name][key] {
		return nil
	}
	f, ok := p.facts(t, j)
	if !ok {
		return nil
	}
	pl, err := h.Synthesize(ctx, f)
	var pe *providers.Error
	switch {
	case err == nil && pl == nil, errors.As(err, &pe) && pe.Code != "" && refusal(pe.Class):
		// refused for good: marked below, so it is not offered every tick
	case errors.As(err, &pe):
		return nil // capped or not ready: nothing marked, the job is offered again next tick
	case err != nil:
		return fmt.Errorf("gitlab poll: demand handler failed for %s job %d: %w", t.name, j.ID, err)
	default:
		raw, err := json.Marshal(f)
		if err != nil {
			return err
		}
		d := coordinator.Delivery{Source: Name, DeliveryID: f.DeliveryID, Project: f.Repo, Revision: f.Revision,
			TriggerClass: "runner_demand", Facts: raw, Synthetic: pl}
		if _, err = enq.Enqueue(ctx, d); err != nil {
			return err
		}
	}
	if p.marks[t.name] == nil {
		p.marks[t.name] = map[string]bool{}
	}
	p.marks[t.name][key] = true
	return nil
}

// facts are the runner_demand facts of one pending job, every claim read through the API.
func (p *Poll) facts(t target, j pendingJob) (model.TriggerFacts, bool) {
	rev := strings.ToLower(j.Pipeline.SHA)
	if j.Pipeline.ID <= 0 || !isHex40(rev) || allZero(rev) {
		return model.TriggerFacts{}, false
	}
	now := p.src.Now()
	f := model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: Name, Kind: "runner_demand", DeliveryID: "poll:job:" + itoa(j.ID),
		ReceivedAt: now, Auth: "per-repo", Repo: t.name, Revision: rev, RevisionSource: "payload"}
	if t.id > 0 {
		f.RepoID = itoa(t.id)
	}
	f.RunnerLabels = fact(append([]string{}, j.TagList...), "api", now)
	f.ProviderJobID = fact(itoa(j.ID), "api", now)
	f.ProviderRunID = fact(itoa(j.Pipeline.ID), "api", now)
	if j.User != nil && j.User.Username != "" {
		f.Actor = fact(j.User.Username, "api", now)
	}
	return f, f.Validate() == nil
}

// forget drops the marks of jobs no longer in the list.
func (p *Poll) forget(project string, jobs []pendingJob) {
	keep := map[string]bool{}
	for _, j := range jobs {
		keep[markKey(j)] = true
	}
	for k := range p.marks[project] {
		if !keep[k] {
			delete(p.marks[project], k)
		}
	}
}

func markKey(j pendingJob) string { return itoa(j.ID) + ":" + j.Status }

// refusal reports whether an error class means "this job is not for us" rather than "try again".
func refusal(c providers.ErrorClass) bool {
	return c != providers.Transient && c != providers.Outage && c != providers.Quota && c != providers.Auth
}
