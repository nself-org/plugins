package webhook

// Purpose: turn an authenticated GitHub delivery into model.TriggerFacts (Epic D8). Served events:
// push (branch or tag), pull_request opened/synchronize/reopened, create of a tag, repository_dispatch
// with a caller-named revision, and workflow_job queued (a runner demand). Everything else is
// acknowledged and dropped. workflow_dispatch is never a source: manual triggers belong to CI-37 and
// NODE-10, and the coordinator's own hosted dispatches (HOST-03) must not loop back (review F3).
// Inputs:  the auth class, the event name, the delivery id and the verified raw body.
// Outputs: facts with payload-sourced fields and, for push, tag and pull request, HeadCurrent from one
//          API call; a *Decline for anything not served or stale.
// Constraints: identity is the BASE repository (repository.full_name and id); the PR head repository
//          is only a fact. No trust is classified here (TRUST-13) and nothing is deduplicated (NODE-10).

import (
	"context"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func (s *Source) normalize(ctx context.Context, auth model.TriggerAuth, event, delivery string, body []byte) (model.TriggerFacts, error) {
	if event == "workflow_dispatch" {
		return model.TriggerFacts{}, unsupported("workflow_dispatch is not a trigger source")
	}
	switch event {
	case "push", "pull_request", "create", "repository_dispatch", "workflow_job":
	default:
		return model.TriggerFacts{}, unsupported("event \"" + clip(event) + "\": skip")
	}
	p, err := parsePayload(body)
	if err != nil {
		return model.TriggerFacts{}, err
	}
	f := s.baseFacts(p, auth, delivery)
	switch event {
	case "push":
		return s.fromPush(ctx, f, p)
	case "pull_request":
		return s.fromPullRequest(ctx, f, p)
	case "create":
		return s.fromCreate(ctx, f, p)
	case "repository_dispatch":
		return fromDispatch(f, p)
	}
	return s.fromJob(ctx, f, p)
}

func (s *Source) baseFacts(p ghPayload, auth model.TriggerAuth, delivery string) model.TriggerFacts {
	now := s.Now()
	f := model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: Name, ReceivedAt: now, Auth: auth, Repo: p.Repository.FullName}
	if len(delivery) <= 128 {
		f.DeliveryID = delivery
	}
	if p.Repository.ID > 0 {
		f.RepoID = itoa(p.Repository.ID)
	}
	if p.Sender.Login != "" {
		f.Actor = payloadFact(p.Sender.Login, now)
		f.Bot = payloadFact(p.Sender.Type == "Bot", now)
	}
	if v, known := visibilityOf(p.Repository); known {
		f.Visibility = payloadFact(v, now)
	}
	return f
}

func (s *Source) fromPush(ctx context.Context, f model.TriggerFacts, p ghPayload) (model.TriggerFacts, error) {
	if p.Deleted || p.After == "" || allZero(p.After) {
		return f, unsupported("delete push: skip")
	}
	object := strings.ToLower(p.After) // what the ref points at: the tag object for an annotated tag
	rev := object
	switch {
	case strings.HasPrefix(p.Ref, "refs/heads/"):
		f.Kind = "push"
	case strings.HasPrefix(p.Ref, "refs/tags/"):
		f.Kind = "tag"
	default:
		return f, unsupported("push to ref \"" + clip(p.Ref) + "\": skip")
	}
	if f.Kind == "tag" { // an annotated tag's push names the tag object; the commit to build is head_commit
		if c := strings.ToLower(p.HeadCommit.ID); isHex40(c) {
			rev = c
		} else if resolved, ok := s.resolveTag(ctx, f.Repo, strings.TrimPrefix(p.Ref, "refs/tags/")); ok {
			rev = resolved // no head_commit: dereference through the API, a tag object is not buildable
		} else {
			return f, unsupported("tag.unresolved")
		}
	}
	if !isHex40(rev) || !isHex40(object) {
		return f, unsupported("push without a 40 hex commit id")
	}
	f.Ref, f.Revision, f.RevisionSource = p.Ref, rev, "payload"
	if b := strings.ToLower(p.Before); isHex40(b) && !allZero(b) {
		f.BaseRevision = b
	}
	return s.checkHead(ctx, f, object)
}

func (s *Source) fromPullRequest(ctx context.Context, f model.TriggerFacts, p ghPayload) (model.TriggerFacts, error) {
	switch p.Action {
	case "opened", "synchronize", "reopened":
	default:
		return f, unsupported("pr action \"" + clip(p.Action) + "\": skip")
	}
	pr := p.PullRequest
	if pr == nil || pr.Number <= 0 || pr.Head.Ref == "" || pr.Base.Ref == "" || !isHex40(strings.ToLower(pr.Head.SHA)) {
		return f, unsupported("pull_request without a head and base")
	}
	if pr.Base.Repo != nil && !strings.EqualFold(pr.Base.Repo.FullName, p.Repository.FullName) {
		return f, unsupported("pull_request base repository differs from the repository")
	}
	now := f.ReceivedAt
	rev := strings.ToLower(pr.Head.SHA)
	f.Kind, f.PRNumber = "pull_request", pr.Number
	// Ref names the pull request, not the head branch: the head branch name is the sender's to choose
	// (a fork branch called "main" must not look like a push to main). BaseRef is the target branch.
	f.Ref, f.Revision, f.RevisionSource = "refs/pull/"+strconv.Itoa(pr.Number)+"/head", rev, "payload"
	f.BaseRef = "refs/heads/" + pr.Base.Ref
	if b := strings.ToLower(pr.Base.SHA); isHex40(b) {
		f.BaseRevision = b
	}
	if pr.AuthorAssociation != "" {
		f.AuthorAssociation = payloadFact(pr.AuthorAssociation, now)
	}
	if h := pr.Head.Repo; h != nil && h.FullName != "" && h.ID > 0 { // the head repository is only a fact
		f.HeadRepo = payloadFact(h.FullName, now)
		f.Fork = payloadFact(h.ID != p.Repository.ID, now)
	}
	return s.checkHead(ctx, f, rev)
}

// fromCreate handles a tag creation. The event carries no commit id, so the tag is resolved through
// the API; without a resolution the matching push event (GitHub sends both) carries the tag.
func (s *Source) fromCreate(ctx context.Context, f model.TriggerFacts, p ghPayload) (model.TriggerFacts, error) {
	if p.RefType != "tag" || p.Ref == "" {
		return f, unsupported("create " + clip(p.RefType) + ": skip")
	}
	rev, ok := s.resolveTag(ctx, f.Repo, p.Ref)
	if !ok {
		return f, unsupported("tag.unresolved")
	}
	// The model allows only payload or caller as revision source; this revision was resolved through the
	// API from a name the delivery gave, so it is stamped payload and is not bound to the delivery bytes.
	f.Kind, f.Ref, f.Revision, f.RevisionSource = "tag", "refs/tags/"+p.Ref, rev, "payload"
	return f, nil
}

func fromDispatch(f model.TriggerFacts, p ghPayload) (model.TriggerFacts, error) {
	rev := strings.ToLower(p.ClientPayload.Revision)
	if !isHex40(rev) || allZero(rev) {
		return f, unsupported("repository_dispatch needs client_payload.revision (40 hex)")
	}
	f.Kind, f.Revision, f.RevisionSource = "api", rev, "caller"
	return f, nil
}

func (s *Source) fromJob(ctx context.Context, f model.TriggerFacts, p ghPayload) (model.TriggerFacts, error) {
	if p.Action != "queued" {
		return f, unsupported("workflow_job action \"" + clip(p.Action) + "\": skip")
	}
	j := p.WorkflowJob
	rev := ""
	if j != nil {
		rev = strings.ToLower(j.HeadSHA)
	}
	if j == nil || j.ID <= 0 || j.RunID <= 0 || !isHex40(rev) {
		return f, unsupported("workflow_job without a job, run and head sha")
	}
	now := f.ReceivedAt
	// No Ref: head_branch is the sender's to choose for a fork pull request, and the revision names the code.
	f.Kind, f.Revision, f.RevisionSource = "runner_demand", rev, "payload"
	f.RunnerLabels = payloadFact(append([]string{}, j.Labels...), now)
	f.ProviderJobID = payloadFact(itoa(j.ID), now)
	f.ProviderRunID = payloadFact(itoa(j.RunID), now)
	if queued, known := s.jobQueued(ctx, f.Repo, j.ID); known && !queued { // a replayed demand for a job that is long over
		return f, decline(reasonStale, "trigger.stale")
	}
	return f, nil
}

// checkHead runs the stale check: a known mismatch declines the delivery (E706 stale), an unknown
// answer lets it continue with HeadCurrent left empty.
func (s *Source) checkHead(ctx context.Context, f model.TriggerFacts, want string) (model.TriggerFacts, error) {
	current, known := s.headCurrent(ctx, f, want)
	switch {
	case !known:
		return f, nil
	case !current:
		return f, decline(reasonStale, "trigger.stale")
	}
	f.HeadCurrent = fact(true, "api", s.Now())
	return f, nil
}

// clip bounds attacker-chosen text that is echoed back in a decline reason.
func clip(s string) string {
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '?'
		}
		return r
	}, s)
}
