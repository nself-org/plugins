package internal

import (
	"strconv"
)

func (c *GitHubClient) ListOrganizations() ([]ghOrg, error) {
	return fetchAllPages[ghOrg](c, "/user/orgs")
}

func (c *GitHubClient) ListRepositories(org string) ([]ghRepo, error) {
	return fetchAllPages[ghRepo](c, "/orgs/"+org+"/repos")
}

func (c *GitHubClient) ListUserRepositories() ([]ghRepo, error) {
	return fetchAllPages[ghRepo](c, "/user/repos?type=all&sort=updated")
}

func (c *GitHubClient) ListBranches(owner, repo string) ([]ghBranch, error) {
	return fetchAllPages[ghBranch](c, "/repos/"+owner+"/"+repo+"/branches")
}

func (c *GitHubClient) ListIssues(owner, repo, state string) ([]ghIssue, error) {
	if state == "" {
		state = "all"
	}
	return fetchAllPages[ghIssue](c, "/repos/"+owner+"/"+repo+"/issues?state="+state)
}

func (c *GitHubClient) ListPullRequests(owner, repo, state string) ([]ghPullRequest, error) {
	if state == "" {
		state = "all"
	}
	return fetchAllPages[ghPullRequest](c, "/repos/"+owner+"/"+repo+"/pulls?state="+state)
}

func (c *GitHubClient) ListCommits(owner, repo string) ([]ghCommit, error) {
	return fetchAllPages[ghCommit](c, "/repos/"+owner+"/"+repo+"/commits")
}

func (c *GitHubClient) ListReleases(owner, repo string) ([]ghRelease, error) {
	return fetchAllPages[ghRelease](c, "/repos/"+owner+"/"+repo+"/releases")
}

func (c *GitHubClient) ListWorkflows(owner, repo string) ([]ghWorkflow, error) {
	var resp ghWorkflowsResp
	if err := c.fetchJSON("/repos/"+owner+"/"+repo+"/actions/workflows", &resp); err != nil {
		return nil, err
	}
	return resp.Workflows, nil
}

func (c *GitHubClient) ListWorkflowRuns(owner, repo string) ([]ghWorkflowRun, error) {
	var resp ghWorkflowRunsResp
	if err := c.fetchJSON("/repos/"+owner+"/"+repo+"/actions/runs?per_page="+strconv.Itoa(defaultPerPage), &resp); err != nil {
		return nil, err
	}
	return resp.WorkflowRuns, nil
}

func (c *GitHubClient) ListDeployments(owner, repo string) ([]ghDeployment, error) {
	return fetchAllPages[ghDeployment](c, "/repos/"+owner+"/"+repo+"/deployments")
}

func (c *GitHubClient) ListTeams(org string) ([]ghTeam, error) {
	return fetchAllPages[ghTeam](c, "/orgs/"+org+"/teams")
}

func (c *GitHubClient) ListCollaborators(owner, repo string) ([]ghCollaborator, error) {
	return fetchAllPages[ghCollaborator](c, "/repos/"+owner+"/"+repo+"/collaborators")
}
