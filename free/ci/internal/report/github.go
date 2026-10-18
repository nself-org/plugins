package report

// Purpose: the GitHub adapter. It posts `repos/{owner}/{repo}/statuses/{sha}`
// through `gh api`, as the v1.4 gate did, and resolves whether posting is
// possible at all.
// Inputs:  a Status; a repository root for Resolve.
// Outputs: a posted status, or the reason line.
// Constraints: gh authenticates itself (no token on argv or in env here);
// only github.com remotes are recognised.

import (
	"context"
	"fmt"
	"strings"
)

// Reasons Resolve gives for not posting. They follow "nself-ci: status not
// posted: ".
const (
	ReasonNoStatus   = "--no-status"
	ReasonNoRemote   = "no GitHub remote"
	ReasonNoAuth     = "gh not authenticated"
	ReasonUnbound    = "worktree is dirty, evidence is not bound to the commit"
	ReasonNoRevision = "no commit to bind the status to"
)

// GH posts statuses through the gh CLI.
type GH struct {
	// Capture runs a process; nil uses the exec funnel.
	Capture func(ctx context.Context, argv ...string) (string, int, error)
}

func (g GH) capture(ctx context.Context, argv ...string) (string, int, error) {
	if g.Capture != nil {
		return g.Capture(ctx, argv...)
	}
	return Capture(ctx, argv...)
}

// Args is the gh argv for a status. It never carries a target_url unless the
// Status has one.
func Args(s Status) []string {
	args := []string{"gh", "api", "--method", "POST", fmt.Sprintf("repos/%s/%s/statuses/%s", s.Owner, s.Repo, s.Revision),
		"-f", "state=" + string(s.State), "-f", "context=" + Context, "-f", "description=" + Truncate(s.Description)}
	if s.TargetURL != "" {
		args = append(args, "-f", "target_url="+s.TargetURL)
	}
	return args
}

// Post implements Reporter.
func (g GH) Post(ctx context.Context, s Status) error {
	if err := s.Validate(); err != nil {
		return err
	}
	out, exit, err := g.capture(ctx, Args(s)...)
	if err != nil {
		return fmt.Errorf("report: gh not run: %w", err)
	}
	if exit != 0 {
		return fmt.Errorf("report: gh api failed (exit %d): %s", exit, firstLine(out))
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// ParseRemote extracts owner and repo from a github.com remote URL.
func ParseRemote(url string) (owner, repo string, ok bool) {
	url = strings.TrimSuffix(strings.TrimSpace(url), ".git")
	var path string
	switch {
	case strings.HasPrefix(url, "https://github.com/"):
		path = strings.TrimPrefix(url, "https://github.com/")
	case strings.HasPrefix(url, "ssh://git@github.com/"):
		path = strings.TrimPrefix(url, "ssh://git@github.com/")
	case strings.HasPrefix(url, "git@github.com:"):
		path = strings.TrimPrefix(url, "git@github.com:")
	default:
		return "", "", false
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || !ownerRepo.MatchString(parts[0]) || !ownerRepo.MatchString(parts[1]) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// Resolve decides whether a status can be posted for the repository at root.
// When it cannot, reason says why and r is nil.
func (g GH) Resolve(ctx context.Context, root string) (r Reporter, owner, repo, reason string) {
	out, exit, err := g.capture(ctx, "git", "-c", "core.fsmonitor=false", "-C", root, "remote", "get-url", "origin")
	if err != nil || exit != 0 {
		return nil, "", "", ReasonNoRemote
	}
	owner, repo, ok := ParseRemote(out)
	if !ok {
		return nil, "", "", ReasonNoRemote
	}
	if _, exit, err = g.capture(ctx, "gh", "auth", "status"); err != nil || exit != 0 {
		return nil, "", "", ReasonNoAuth
	}
	return g, owner, repo, ""
}
