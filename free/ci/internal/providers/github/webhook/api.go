package webhook

// Purpose: the GitHub REST calls of this source, all through the one provider funnel (rest.Client:
// timeouts, SSRF and redirect rules, credential from the operator policy).
// Inputs:  the operator's GitHub settings and credential (runtime.CurrentSettings, runtime.Credential).
// Outputs: a *rest.Client; get, which returns the status and decodes JSON.
// Constraints: no query string reaches the funnel (it refuses '?'), so list endpoints that need a filter
//          are not used. A ref name goes into the path one escaped segment at a time, never raw. A
//          missing credential is E700 from the funnel and reads as "unknown" to every caller here.

import (
	"context"
	"net/url"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

// defaultAPI builds the client from the operator's settings.
func defaultAPI(ctx context.Context) (*rest.Client, error) {
	st, err := runtime.CurrentSettings(ctx)
	if err != nil {
		return nil, err
	}
	return &rest.Client{
		BaseURL: st.GitHubAPIURL, Auth: runtime.Credential(ctx, Name), Provider: Name,
		UserAgent: "ci-webhook", APIVersion: "2022-11-28", AllowPrivateBase: st.GitHubAllowPrivateBase,
	}, nil
}

func (s *Source) api(ctx context.Context) (*rest.Client, error) {
	return s.NewAPI(ctx)
}

// get sends one GET and decodes the JSON body into out. The status is returned even on error.
func get(ctx context.Context, c *rest.Client, template string, params map[string]string, out any) (int, error) {
	resp, err := c.Do(ctx, "GET", template, params, nil, out)
	return resp.Status, err
}

// repoParams splits owner/repo into the path parameters every repository call uses.
func repoParams(full string) map[string]string {
	owner, repo, _ := strings.Cut(full, "/")
	return map[string]string{"owner": owner, "repo": repo}
}

// refSegments escapes a git ref or branch name for use inside a path template, one segment at a time.
// It refuses empty and dot segments.
func refSegments(name string) (string, bool) {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		if p == "" || p == "." || p == ".." {
			return "", false
		}
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/"), true
}
