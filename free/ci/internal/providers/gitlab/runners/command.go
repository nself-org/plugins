package runners

// Purpose: RunnerCommand and InstanceOf, how the pinned gitlab-runner image starts the one runner (Epic D11).
// Inputs:  the GitLab API base URL from the operator's policy (providers.gitlab.api_url).
// Outputs: the job command and the host the restricted network scope must allow.
// Constraints: `gitlab-runner run-single` reads the authentication token from CI_SERVER_TOKEN, which the
//          credential wrapper sets from the job's stdin (the agent's cred-wrapper maps the
//          gitlab-runner-token kind to that variable; confirmed against the pinned image's
//          `run-single --help`). The token is never an argument and never in the container environment or
//          a config file. The server URL is public and travels as --url. The shell executor builds in the
//          job workspace (/repo), the one place a hardened container can write that is not memory.
//          The runner takes one build and leaves (--max-builds 1) and gives up after 120 s with no job
//          (--wait-timeout 120), so a runner whose job was cancelled does not linger.

import (
	"net/url"
	"strings"
)

const (
	// WaitTimeoutSeconds is how long the runner waits for its first job.
	WaitTimeoutSeconds = "120"
	apiSuffix          = "/api/v4"
)

// InstanceOf turns the API base URL into the instance URL and its host name. Only an https URL with a
// host, no user info, query or fragment is accepted.
func InstanceOf(apiURL string) (instance, host string, ok bool) {
	u, err := url.Parse(strings.TrimRight(apiURL, "/"))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", false
	}
	u.Path = strings.TrimSuffix(u.Path, apiSuffix)
	u.RawPath = ""
	host = strings.ToLower(u.Hostname())
	if strings.ContainsAny(host, " ,") {
		return "", "", false
	}
	return strings.TrimRight(u.String(), "/"), host, true
}

// RunnerCommand is the job command of a GitLab runner job. It starts with the program name, not a path:
// the wrapper resolves it through the image's PATH.
func RunnerCommand(instance string) []string {
	return []string{"gitlab-runner", "run-single", "--url", instance, "--executor", "shell",
		"--max-builds", "1", "--wait-timeout", WaitTimeoutSeconds,
		"--builds-dir", "/repo/builds", "--cache-dir", "/repo/cache"}
}
