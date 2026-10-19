package protect

// Purpose: the gh api calls of `policy protect`: read the protection of a
// branch, write it, find the default branch (P7-CI-07).
// Inputs:  a Runner (the exec funnel in production, a recorder in tests).
// Outputs: raw documents; typed errors for the two answers that change what
// the command does (403: no protection on this plan, 404: not protected).
// Constraints: gh authenticates itself (OAuth); no token on argv. Every gh
// call goes through Runner; this package never starts a process itself.

import (
	"context"
	"fmt"
	"strings"
)

// Runner runs argv and returns its combined output and exit code. err is set
// only when the process could not be run.
type Runner func(ctx context.Context, argv ...string) (out string, exit int, err error)

// Client talks to the GitHub REST API through gh.
type Client struct{ Run Runner }

func (c Client) call(ctx context.Context, argv ...string) (string, error) {
	out, exit, err := c.Run(ctx, append([]string{"gh", "api"}, argv...)...)
	if err != nil {
		return "", fmt.Errorf("protect: gh not run: %w", err)
	}
	if exit != 0 {
		return out, &httpError{status: statusOf(out), text: firstLine(out)}
	}
	return out, nil
}

type httpError struct {
	status int
	text   string
}

func (e *httpError) Error() string { return "protect: gh api failed: " + e.text }

// statusOf extracts the HTTP status gh prints as "(HTTP 403)"; 0 when absent.
func statusOf(out string) int {
	i := strings.LastIndex(out, "(HTTP ")
	if i < 0 {
		return 0
	}
	var n int
	if _, err := fmt.Sscanf(out[i:], "(HTTP %d)", &n); err != nil {
		return 0
	}
	return n
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

func protectionPath(repo, branch string) string {
	return "repos/" + repo + "/branches/" + branch + "/protection"
}

// Protection reads the protection of a branch. A nil document with a nil
// error means the branch is not protected; a 403 is E643.
func (c Client) Protection(ctx context.Context, repo, branch string) ([]byte, error) {
	out, err := c.call(ctx, protectionPath(repo, branch))
	if he, ok := err.(*httpError); ok {
		switch {
		case he.status == 403:
			return nil, fail(CodeNoPlan, "%s: %s", repo, he.text)
		case he.status == 404 && strings.Contains(out, "Branch not protected"):
			return nil, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// Put writes the body saved at bodyFile (gh reads it with --input, so the
// document never travels on argv).
func (c Client) Put(ctx context.Context, repo, branch, bodyFile string) error {
	_, err := c.call(ctx, "--method", "PUT", "-H", "Accept: application/vnd.github+json", "--input", bodyFile, protectionPath(repo, branch))
	if he, ok := err.(*httpError); ok && he.status == 403 {
		return fail(CodeNoPlan, "%s: %s", repo, he.text)
	}
	return err
}

// DefaultBranch returns the repository's default branch name.
func (c Client) DefaultBranch(ctx context.Context, repo string) (string, error) {
	out, err := c.call(ctx, "repos/"+repo, "--jq", ".default_branch")
	if err != nil {
		return "", err
	}
	b := strings.TrimSpace(out)
	if b == "" || strings.ContainsAny(b, " \t\n") {
		return "", fmt.Errorf("protect: %s: no default branch in the answer", repo)
	}
	return b, nil
}
