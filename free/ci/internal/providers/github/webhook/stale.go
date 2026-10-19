package webhook

// Purpose: replay defence beyond NODE-10's 72 h dedup window (Epic D8): a delivery whose ref no longer
// points at the payload SHA, or whose pull request head has moved, is stale. The check is one API call
// with a 5 s budget.
// Inputs:  verified facts for a push, tag or pull request, and the object id the payload named.
// Outputs: (current, known). known is false on any API failure (no credential, timeout, 404 which can
//          also mean "no access", bad JSON): the trigger then continues with HeadCurrent unknown and
//          the trust classifier caps it.
// Constraints: only a successful read that shows a different id marks a delivery stale. A tag also
//          resolves here for a `create` event, which carries no commit id.

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// staleBudget is the whole time the check may take.
var staleBudget = 5 * time.Second

type refObject struct {
	Object struct {
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"object"`
}

// refObjectOf reads the object a ref points at (ref is "heads/main" or "tags/v1").
func refObjectOf(ctx context.Context, c *rest.Client, repo, ref string) (refObject, bool) {
	var out refObject
	seg, ok := refSegments(ref)
	if !ok {
		return out, false
	}
	if _, err := get(ctx, c, "/repos/{owner}/{repo}/git/ref/"+seg, repoParams(repo), &out); err != nil {
		return out, false
	}
	return out, out.Object.SHA != ""
}

// headCurrent asks whether the delivery still describes the current head. want is the object id the
// payload named for the ref (the tag object for an annotated tag, which differs from the revision).
func (s *Source) headCurrent(ctx context.Context, f model.TriggerFacts, want string) (current, known bool) {
	ctx, cancel := context.WithTimeout(ctx, staleBudget)
	defer cancel()
	switch f.Kind {
	case "push", "tag":
		c, err := s.api(ctx)
		if err != nil {
			return false, false
		}
		got, ok := refObjectOf(ctx, c, f.Repo, strings.TrimPrefix(f.Ref, "refs/"))
		if !ok {
			return false, false
		}
		return strings.EqualFold(got.Object.SHA, want), true
	case "pull_request":
		c, err := s.api(ctx)
		if err != nil {
			return false, false
		}
		var pr struct {
			Head struct {
				SHA string `json:"sha"`
			} `json:"head"`
		}
		params := repoParams(f.Repo)
		params["number"] = strconv.Itoa(f.PRNumber)
		if _, err = get(ctx, c, "/repos/{owner}/{repo}/pulls/{number}", params, &pr); err != nil || pr.Head.SHA == "" {
			return false, false
		}
		return strings.EqualFold(pr.Head.SHA, want), true
	}
	return false, false
}

// resolveTag returns the commit a tag points at, within the stale budget.
func (s *Source) resolveTag(ctx context.Context, repo, tag string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, staleBudget)
	defer cancel()
	c, err := s.api(ctx)
	if err != nil {
		return "", false
	}
	return resolveTagWith(ctx, c, repo, tag)
}

// resolveTagWith reads the tag ref and dereferences an annotated tag once.
func resolveTagWith(ctx context.Context, c *rest.Client, repo, tag string) (string, bool) {
	got, ok := refObjectOf(ctx, c, repo, "tags/"+tag)
	if !ok {
		return "", false
	}
	sha := got.Object.SHA
	if got.Object.Type == "tag" {
		var t refObject
		params := repoParams(repo)
		params["sha"] = sha
		if _, err := get(ctx, c, "/repos/{owner}/{repo}/git/tags/{sha}", params, &t); err != nil || t.Object.Type != "commit" {
			return "", false
		}
		sha = t.Object.SHA
	}
	sha = strings.ToLower(sha)
	return sha, isHex40(sha)
}
