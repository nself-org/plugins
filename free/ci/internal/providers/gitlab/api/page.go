package api

// Purpose: the REST client of the GitLab providers and a paginated listing over it.
// Inputs:  the operator's provider policy (api_url, allow_private_base) and the "gitlab" credential rule.
// Outputs: *rest.Client; every item of a list route, up to MaxPages pages.
// Constraints: the funnel returns no response headers, so x-next-page cannot be read; a listing stops at
//          the first page shorter than PageSize, which is the same end as an empty x-next-page and costs
//          one extra request when the last page is exactly full; the token is resolved per call by the
//          funnel and is never held here; a client is built per call so a policy change takes effect
//          without a restart.

import (
	"context"
	"net/url"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

// NewClient builds the client from the operator's policy and the "gitlab" credential rule.
func NewClient(ctx context.Context, userAgent string) (*rest.Client, error) {
	s, err := runtime.CurrentSettings(ctx)
	if err != nil {
		return nil, Failure(providers.Config, "E700", "provider policy unreadable")
	}
	hosts, err := gitlabtmpl.RedirectHosts()
	if err != nil {
		return nil, Misconfig("redirect_hosts")
	}
	return &rest.Client{BaseURL: s.GitLabAPIURL, Auth: runtime.Credential(ctx, "gitlab"), RedirectHosts: hosts,
		UserAgent: userAgent, Provider: "gitlab", AllowPrivateBase: s.GitLabAllowPrivateBase}, nil
}

// List reads a list route page by page and returns all items.
func List[T any](ctx context.Context, c *rest.Client, path string, params map[string]string, query url.Values) ([]T, error) {
	var all []T
	for page := 1; page <= MaxPages; page++ {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("per_page", strconv.Itoa(PageSize))
		q.Set("page", strconv.Itoa(page))
		var items []T
		if _, err := c.DoQuery(ctx, "GET", path, params, q, nil, &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < PageSize {
			break
		}
	}
	return all, nil
}
