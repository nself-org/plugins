package run

import ciexec "github.com/nself-org/plugins/free/ci/internal/exec"

// newExecutor builds the job executor: jobs get an allowlisted environment
// and every output chunk passes the redactor before it is stored or shown.
func newExecutor(stale ciexec.StaleChecker) *ciexec.Executor {
	return &ciexec.Executor{Stale: stale, Env: ciexec.AllowlistEnv{}, NewRedactor: ciexec.NewOutputRedactor}
}
