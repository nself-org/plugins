package runners_test

import "github.com/nself-org/plugins/free/ci/internal/providers/runtime"

// gitlabSettings is the operator's GitLab settings with the API at base and the named runner token variable.
func gitlabSettings(base, runnerEnv string) runtime.Settings {
	s := runtime.DefaultSettings()
	s.GitLabAPIURL, s.GitLabRunnerTokenEnv, s.GitLabAllowPrivateBase = base, runnerEnv, true
	return s
}
