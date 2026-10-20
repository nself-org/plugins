package webhook

// Purpose: the GitLab REST calls of this source, all through the one provider funnel (rest.Client via
// gitlab/api.NewClient: timeouts, SSRF and redirect rules, credential from the operator policy).
// Inputs:  the operator's GitLab settings and credential.
// Outputs: a *rest.Client; get and query, which return the status and decode JSON.
// Constraints: a project is addressed by its path, which the funnel escapes into one path segment; a
//          branch or tag name is a path parameter and is escaped the same way; a missing credential is
//          E700 from the funnel and reads as "unknown" to every caller here.

import (
	"context"
	"net/url"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// Routes this source reads (the ones gitlab/api names are used from there).
const (
	tagPath     = "/projects/{id}/repository/tags/{tag}"
	mrPath      = "/projects/{id}/merge_requests/{iid}"
	mrDiffsPath = "/projects/{id}/merge_requests/{iid}/diffs"
	comparePath = "/projects/{id}/repository/compare"
	memberPath  = "/projects/{id}/members/all/{user}"
	jobPath     = "/projects/{id}/jobs/{job}"
	usersPath   = "/users"
	userAgent   = "ci-webhook"
)

// defaultAPI builds the client from the operator's settings.
func defaultAPI(ctx context.Context) (*rest.Client, error) {
	return api.NewClient(ctx, userAgent)
}

func (s *Source) api(ctx context.Context) (*rest.Client, error) { return s.NewAPI(ctx) }

// get sends one GET and decodes the JSON body into out. The status is returned even on error.
func get(ctx context.Context, c *rest.Client, template string, params map[string]string, out any) (int, error) {
	resp, err := c.Do(ctx, "GET", template, params, nil, out)
	return resp.Status, err
}

// query is get with a URL query.
func query(ctx context.Context, c *rest.Client, template string, params map[string]string, q url.Values, out any) (int, error) {
	resp, err := c.DoQuery(ctx, "GET", template, params, q, nil, out)
	return resp.Status, err
}

// projectParams is the {id} parameter of a project route.
func projectParams(project string, kv ...string) map[string]string {
	m := map[string]string{"id": project}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}
