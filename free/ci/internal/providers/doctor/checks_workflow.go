package doctor

// Purpose: CI-DP006 and CI-DP007, the generated GitHub dispatch workflow on the default branch and on
// the untrusted ref, as local git has them (Epic D5, D14).
// Inputs:  the repository root and the exec funnel; providers.github.untrusted_ref.
// Outputs: one check per ref: ok, missing, not generated, or older than the template this build
// renders (drifted), each with the exact commands that fix it.
// Constraints: offline: `git show origin/<ref>:<path>` only, never a fetch or a push, so a ref that
// exists only on the remote and was never fetched reads as missing (the remediation says to fetch);
// the comparison is the template version and the generated marker, because the full rendering depends
// on a signed release that only the network can give; a repository with no GitHub remote has
// nothing to check and both checks pass.

import (
	"context"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

var githubRemote = regexp.MustCompile(`github\.com[:/]+([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)

// GitHubRepo returns the owner/name of the origin remote when it is on github.com.
func GitHubRepo(ctx context.Context, root string, capture doctor.CaptureFunc) (string, bool) {
	if capture == nil {
		return "", false
	}
	out, exit, err := capture(ctx, "git", "-c", "core.fsmonitor=false", "-C", root, "remote", "get-url", "origin")
	if err != nil || exit != 0 {
		return "", false
	}
	m := githubRemote.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return "", false
	}
	return strings.ToLower(m[1] + "/" + m[2]), true
}

// DefaultBranch is origin's default branch as local git knows it ("main" when unknown).
func DefaultBranch(ctx context.Context, root string, capture doctor.CaptureFunc) string {
	out, exit, err := capture(ctx, "git", "-c", "core.fsmonitor=false", "-C", root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if b := strings.TrimPrefix(strings.TrimSpace(out), "origin/"); err == nil && exit == 0 && b != "" {
		return b
	}
	return "main"
}

// WorkflowState classifies the workflow copy at origin/<branch>: ok, missing, not-generated or drifted.
func WorkflowState(ctx context.Context, root, branch string, capture doctor.CaptureFunc) string {
	out, exit, err := capture(ctx, "git", "-c", "core.fsmonitor=false", "-C", root, "show", "origin/"+branch+":"+githubtmpl.Path)
	switch {
	case err != nil || exit != 0:
		return "missing"
	case !strings.HasPrefix(out, githubtmpl.HeaderMarker):
		return "not-generated"
	case githubtmpl.VersionOf([]byte(out)) != githubtmpl.Current:
		return "drifted"
	}
	return "ok"
}

func (s *state) checkWorkflows(ctx context.Context) (doctor.Check, doctor.Check) {
	def := mk("CI-DP006", "github workflow, default branch", doctor.Pass, "no GitHub remote: nothing to check", "")
	unt := mk("CI-DP007", "github workflow, untrusted ref", doctor.Pass, "no GitHub remote: nothing to check", "")
	if _, ok := GitHubRepo(ctx, s.env.Root, s.env.Capture); !ok {
		return def, unt
	}
	branch := DefaultBranch(ctx, s.env.Root, s.env.Capture)
	ref := s.cfg.Hosted.UntrustedRef
	d := WorkflowState(ctx, s.env.Root, branch, s.env.Capture)
	u := WorkflowState(ctx, s.env.Root, ref, s.env.Capture)
	if d == "missing" && u == "missing" && !s.githubInUse() {
		def.Message, unt.Message = "GitHub-hosted capacity is not set up for this repository (no providers.github.* key, no authorization, no workflow); run `nself ci providers github init` to set it up", "GitHub-hosted capacity is not set up for this repository"
		return def, unt
	}
	fix := "Run `nself ci providers github init`, commit " + githubtmpl.Path + " and push it to " + branch
	describe := func(c *doctor.Check, where, st, remediation string) {
		c.Message = "the dispatch workflow on " + where + " is " + st
		if st == "ok" {
			return
		}
		c.Status = doctor.Warn
		if st == "missing" && where == branch {
			c.Message += " (if it exists only on the remote, run `git fetch origin` first)"
		}
		c.Remediation = remediation
	}
	describe(&def, branch, d, fix)
	untrustedFix := "Run `nself ci providers github init --check` for the exact commands that create or update " + ref
	if d != "ok" {
		untrustedFix = "Fix the default branch copy first (CI-DP006). " + untrustedFix
	}
	describe(&unt, ref, u, untrustedFix)
	return def, unt
}

// githubInUse reports whether the operator set up GitHub-hosted capacity: any providers.github.* key, or a
// GitHub provider in trust.providers. A GitLab-only operator with a GitHub mirror as origin is not nagged.
func (s *state) githubInUse() bool {
	for key := range s.cfg.Set {
		if strings.HasPrefix(key, "providers.github.") {
			return true
		}
	}
	for _, id := range s.cfg.Authorized {
		if strings.HasPrefix(id, "github") {
			return true
		}
	}
	return false
}
