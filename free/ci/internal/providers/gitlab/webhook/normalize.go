package webhook

// Purpose: turn an authenticated GitLab delivery into model.TriggerFacts (Epic D11). Served events:
// Push Hook (branch), Tag Push Hook, Merge Request Hook (open, reopen, and update that carries new
// commits) and Job Hook (pending, with the runner tag nself: a runner demand). Everything else is
// acknowledged and dropped.
// Inputs:  the auth class, the event name, the delivery id and the verified raw body.
// Outputs: facts with payload-sourced fields and, for push, tag, merge request and job, HeadCurrent
//          from one API call; a *Decline for anything not served or stale.
// Constraints: identity is the project the body names (project.path_with_namespace and id); the merge
//          request source project is only a fact. The event header is not signed, so it must agree with
//          the body's object_kind. No trust is classified here (TRUST-13) and nothing is deduplicated
//          (NODE-10).

import (
	"context"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// DemandTag is the runner tag that marks a job as wanting one of our runners.
const DemandTag = "nself"

// kinds maps each served event header to the object_kind its body must carry.
var kinds = map[string]string{
	"Push Hook": "push", "Tag Push Hook": "tag_push", "Merge Request Hook": "merge_request", "Job Hook": "build",
}

func (s *Source) normalize(ctx context.Context, auth model.TriggerAuth, event, delivery string, body []byte) (model.TriggerFacts, error) {
	kind, served := kinds[event]
	if !served {
		return model.TriggerFacts{}, unsupported("event \"" + clip(event) + "\": skip")
	}
	p, err := parsePayload(body)
	if err != nil {
		return model.TriggerFacts{}, err
	}
	if p.ObjectKind != kind {
		return model.TriggerFacts{}, unsupported("event does not match the payload")
	}
	f := s.baseFacts(p, auth, delivery)
	switch event {
	case "Push Hook":
		return s.fromPush(ctx, f, p)
	case "Tag Push Hook":
		return s.fromTag(ctx, f, p)
	case "Merge Request Hook":
		return s.fromMergeRequest(ctx, f, p)
	}
	return s.fromJob(ctx, f, p)
}

func (s *Source) baseFacts(p glPayload, auth model.TriggerAuth, delivery string) model.TriggerFacts {
	now := s.Now()
	name, id := identity(p)
	f := model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: Name, ReceivedAt: now, Auth: auth, Repo: name, DeliveryID: delivery}
	if id > 0 {
		f.RepoID = itoa(id)
	}
	actor := p.UserUsername
	if p.User != nil && p.User.Username != "" {
		actor = p.User.Username
	}
	if actor != "" {
		f.Actor = payloadFact(actor, now)
	}
	if p.Project != nil {
		if v, known := visibilityOf(p.Project.Visibility); known {
			f.Visibility = payloadFact(v, now)
		}
	}
	return f
}

// pushRevision returns the commit a push or tag push names, lower-cased, and the object the ref points
// at (the same for a branch; an annotated tag's push names the tag object in after).
func pushRevision(p glPayload) (object string, ok bool) {
	object = strings.ToLower(p.After)
	return object, isHex40(object) && !allZero(object)
}

func (s *Source) fromPush(ctx context.Context, f model.TriggerFacts, p glPayload) (model.TriggerFacts, error) {
	object, ok := pushRevision(p)
	switch {
	case !strings.HasPrefix(p.Ref, "refs/heads/"):
		return f, unsupported("push to ref \"" + clip(p.Ref) + "\": skip")
	case !ok:
		return f, unsupported("delete push: skip") // after is all zeros for a deleted branch
	}
	f.Kind, f.Ref, f.Revision, f.RevisionSource = "push", p.Ref, object, "payload"
	if b := strings.ToLower(p.Before); isHex40(b) && !allZero(b) {
		f.BaseRevision = b
	}
	return s.checkHead(ctx, f, object)
}

func (s *Source) fromTag(ctx context.Context, f model.TriggerFacts, p glPayload) (model.TriggerFacts, error) {
	if _, ok := pushRevision(p); !ok {
		return f, unsupported("delete push: skip")
	}
	if !strings.HasPrefix(p.Ref, "refs/tags/") {
		return f, unsupported("tag push to ref \"" + clip(p.Ref) + "\": skip")
	}
	// checkout_sha is the commit to build (after names the tag object of an annotated tag).
	rev := strings.ToLower(p.CheckoutSHA)
	if !isHex40(rev) || allZero(rev) {
		resolved, ok := s.resolveTag(ctx, f.Repo, strings.TrimPrefix(p.Ref, "refs/tags/"))
		if !ok {
			return f, unsupported("tag.unresolved")
		}
		rev = resolved
	}
	f.Kind, f.Ref, f.Revision, f.RevisionSource = "tag", p.Ref, rev, "payload"
	if b := strings.ToLower(p.Before); isHex40(b) && !allZero(b) {
		f.BaseRevision = b
	}
	return s.checkHead(ctx, f, rev)
}

func (s *Source) fromMergeRequest(ctx context.Context, f model.TriggerFacts, p glPayload) (model.TriggerFacts, error) {
	mr := p.MR
	if mr == nil {
		return f, unsupported("merge request without attributes")
	}
	switch mr.Action {
	case "open", "reopen":
	case "update":
		if !isHex40(strings.ToLower(mr.OldRev)) { // an update without oldrev changed no code (title, labels, ...)
			return f, unsupported("mr update without new commits: skip")
		}
	default:
		return f, unsupported("mr action \"" + clip(mr.Action) + "\": skip")
	}
	rev := strings.ToLower(mr.LastCommit.ID)
	switch {
	case mr.State != "opened":
		return f, unsupported("mr state \"" + clip(mr.State) + "\": skip")
	case mr.IID <= 0 || mr.SourceBranch == "" || mr.TargetBranch == "" || !isHex40(rev):
		return f, unsupported("merge request without a head and base")
	case p.Project == nil || mr.TargetProjectID != p.Project.ID:
		return f, unsupported("merge request target project differs from the project")
	}
	now := f.ReceivedAt
	f.Kind, f.PRNumber = "pull_request", mr.IID
	// Ref names the merge request, not the source branch: the branch name is the sender's to choose
	// (a fork branch called "main" must not look like a push to main). BaseRef is the target branch.
	f.Ref, f.Revision, f.RevisionSource = "refs/merge-requests/"+strconv.Itoa(mr.IID)+"/head", rev, "payload"
	f.BaseRef = "refs/heads/" + mr.TargetBranch
	if mr.SourceProjectID > 0 {
		f.Fork = payloadFact(mr.SourceProjectID != mr.TargetProjectID, now)
		if mr.Source.Path != "" && mr.Source.ID == mr.SourceProjectID {
			f.HeadRepo = payloadFact(mr.Source.Path, now)
		}
	}
	return s.checkHead(ctx, f, rev)
}

func (s *Source) fromJob(ctx context.Context, f model.TriggerFacts, p glPayload) (model.TriggerFacts, error) {
	if p.BuildStatus != "pending" {
		return f, unsupported("job status \"" + clip(p.BuildStatus) + "\": skip")
	}
	rev := strings.ToLower(p.SHA)
	if p.BuildID <= 0 || p.PipelineID <= 0 || !isHex40(rev) {
		return f, unsupported("job without a job, pipeline and sha")
	}
	tags, tagSource := p.tags(), "payload"
	if len(tags) > 0 && !hasTag(tags, DemandTag) { // the payload already says this job wants no runner of ours
		return f, unsupported("job without the runner tag " + DemandTag)
	}
	job, known := s.jobOf(ctx, f.Repo, p.BuildID)
	switch {
	case known && !job.waiting(): // a replayed demand for a job that is long over
		return f, decline(reasonStale, "trigger.stale")
	case !known && f.Auth == "bearer":
		// a bare token carries no timestamp, so with the API unreadable nothing says the delivery is not a replay
		return f, unsupported("job.unverified")
	}
	if len(tags) == 0 { // a pending job's payload can lack the tags: the Jobs API has them
		tags, tagSource = job.TagList, "api"
		if !known || !hasTag(tags, DemandTag) {
			return f, unsupported("job without the runner tag " + DemandTag)
		}
	}
	now := f.ReceivedAt
	// No Ref: the job's ref is the sender's to choose for a fork merge request, and the revision names the code.
	f.Kind, f.Revision, f.RevisionSource = "runner_demand", rev, "payload"
	f.RunnerLabels = fact(append([]string{}, tags...), tagSource, now)
	f.ProviderJobID = payloadFact(itoa(p.BuildID), now)
	f.ProviderRunID = payloadFact(itoa(p.PipelineID), now)
	return f, nil
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
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
