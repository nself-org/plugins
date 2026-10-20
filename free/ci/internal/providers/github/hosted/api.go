package hosted

// Purpose: the GitHub REST shapes and small helpers the provider uses (API version 2022-11-28).
// Inputs:  repository names, run ids, statuses.
// Outputs: typed responses, the handle encoding, and E7xx errors with safe reasons.
// Constraints: only the fields the provider reads are declared; a repository is "owner/name" with
//          GitHub's own character set; the handle's RunID is "owner/name/<run id>" because a
//          providers.Handle has no repository field (the attestation carries the same string);
//          errors carry a reason key and never response data.

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// ProviderID is the provider id and the identity.provider of every runner it describes.
const ProviderID = "github-hosted"

// WorkflowFile is the workflow's name under .github/workflows.
const WorkflowFile = "nself-ci-dispatch.yml"

// WorkflowPath is where the workflow lives in the repository.
const WorkflowPath = ".github/workflows/" + WorkflowFile

const (
	repoPath     = "/repos/{owner}/{repo}"
	dispatchPath = "/repos/{owner}/{repo}/actions/workflows/" + WorkflowFile + "/dispatches"
	workflowRuns = "/repos/{owner}/{repo}/actions/workflows/" + WorkflowFile + "/runs"
	runPath      = "/repos/{owner}/{repo}/actions/runs/{run}"
	cancelPath   = "/repos/{owner}/{repo}/actions/runs/{run}/cancel"
	artifactsFor = "/repos/{owner}/{repo}/actions/runs/{run}/artifacts"
	contentsPath = "/repos/{owner}/{repo}/contents/" + WorkflowPath
	userPath     = "/user"
)

type repoInfo struct {
	Private       bool   `json:"private"`
	Visibility    string `json:"visibility"`
	DefaultBranch string `json:"default_branch"`
}

type actor struct {
	Login string `json:"login"`
}

type runInfo struct {
	ID           int64  `json:"id"`
	Status       string `json:"status"`
	Event        string `json:"event"`
	Path         string `json:"path"`
	HeadBranch   string `json:"head_branch"`
	HeadSHA      string `json:"head_sha"`
	DisplayTitle string `json:"display_title"`
	RunAttempt   int    `json:"run_attempt"`
	Actor        actor  `json:"actor"`
}

type contentsInfo struct {
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

type artifactInfo struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Size    int64  `json:"size_in_bytes"`
	Expired bool   `json:"expired"`
}

func failure(class providers.ErrorClass, code, reason string) error {
	return providers.NewError(class, code, reason, 0, time.Time{})
}

func refuse(reason string) error      { return failure(providers.Config, "E708", reason) }
func misconfig(reason string) error   { return failure(providers.Config, "E702", reason) }
func notEvidence(reason string) error { return failure(providers.Config, "E705", reason) }

// repoRef is an "owner/name" repository.
type repoRef struct{ Owner, Name string }

func (r repoRef) String() string { return r.Owner + "/" + r.Name }

func (r repoRef) params() map[string]string {
	return map[string]string{"owner": r.Owner, "repo": r.Name}
}

func okName(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 100 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func parseRepo(s string) (repoRef, bool) {
	owner, name, found := strings.Cut(s, "/")
	if !found || !okName(owner) || !okName(name) {
		return repoRef{}, false
	}
	return repoRef{owner, name}, true
}

// runHandle encodes a repository and a run id as the handle's RunID.
func runHandle(r repoRef, id int64) string { return r.String() + "/" + strconv.FormatInt(id, 10) }

// parseHandle splits a handle's RunID back into the repository and the numeric run id.
func parseHandle(runID string) (repoRef, string, bool) {
	i := strings.LastIndex(runID, "/")
	if i < 0 {
		return repoRef{}, "", false
	}
	repo, ok := parseRepo(runID[:i])
	n, err := strconv.ParseInt(runID[i+1:], 10, 64)
	if !ok || err != nil || n <= 0 {
		return repoRef{}, "", false
	}
	return repo, runID[i+1:], true
}

func isStatus(err error, status int) bool {
	var pe *providers.Error
	return errors.As(err, &pe) && pe.Status == status
}

func transient(err error) bool {
	switch providers.ClassOf(err) {
	case providers.Transient, providers.Outage:
		return true
	}
	return false
}
