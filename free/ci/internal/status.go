// Package internal — GitHub commit status reporter.
//
// Purpose: Post a nself-ci commit status to GitHub using the gh CLI
//
//	(OAuth — no token in URLs, no hardcoded credentials).
//
// Inputs:  StatusConfig (owner, repo, sha, state, description)
// Outputs: error
// Constraints: Requires gh CLI with repo scope. Never embeds tokens.
// SPORT: PLUGINS-CI-002
package internal

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const ciContext = "nself-ci"

// StatusConfig holds the parameters for posting a GitHub commit status.
type StatusConfig struct {
	Owner       string
	Repo        string
	SHA         string
	State       string // "success" | "failure" | "pending" | "error"
	Description string
	TargetURL   string // optional
}

// PostCommitStatus posts a GitHub commit status via `gh api`.
// Uses gh OAuth — never a token in the URL.
//
// Equivalent shell command:
//
//	gh api repos/{owner}/{repo}/statuses/{sha} \
//	  -f state={state} \
//	  -f context=nself-ci \
//	  -f description={description}
func PostCommitStatus(cfg StatusConfig) error {
	if cfg.Owner == "" || cfg.Repo == "" || cfg.SHA == "" {
		return fmt.Errorf("owner, repo, and sha are required to post a commit status")
	}
	if cfg.State == "" {
		cfg.State = "error"
	}

	endpoint := fmt.Sprintf("repos/%s/%s/statuses/%s", cfg.Owner, cfg.Repo, cfg.SHA)

	args := []string{
		"api",
		"--method", "POST",
		endpoint,
		"-f", fmt.Sprintf("state=%s", cfg.State),
		"-f", fmt.Sprintf("context=%s", ciContext),
		"-f", fmt.Sprintf("description=%s", truncate(cfg.Description, 140)),
	}
	if cfg.TargetURL != "" {
		args = append(args, "-f", fmt.Sprintf("target_url=%s", cfg.TargetURL))
	}

	var stderr bytes.Buffer
	cmd := exec.Command("gh", args...)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh api failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// HeadSHA returns the HEAD commit SHA for the given repo path using git.
func HeadSHA(repoRoot string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RepoOwnerName extracts owner and repo name from the git remote URL.
// Handles both https://github.com/owner/repo and git@github.com:owner/repo.
func RepoOwnerName(repoRoot string) (owner, repo string, err error) {
	cmd := exec.Command("git", "-C", repoRoot, "remote", "get-url", "origin")
	out, outErr := cmd.Output()
	if outErr != nil {
		return "", "", fmt.Errorf("git remote get-url origin: %w", outErr)
	}
	url := strings.TrimSpace(string(out))

	// Strip .git suffix.
	url = strings.TrimSuffix(url, ".git")

	// https://github.com/owner/repo
	if strings.HasPrefix(url, "https://") {
		parts := strings.Split(url, "/")
		if len(parts) >= 5 {
			return parts[len(parts)-2], parts[len(parts)-1], nil
		}
	}

	// git@github.com:owner/repo
	if idx := strings.Index(url, ":"); idx >= 0 {
		rest := url[idx+1:]
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}

	return "", "", fmt.Errorf("cannot parse GitHub remote from URL: %s", url)
}

// truncate cuts s to max runes, appending "…" if truncated.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
