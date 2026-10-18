package run

import ciexec "github.com/nself-org/plugins/free/ci/internal/exec"

// newExecutor builds the job executor: jobs get an allowlisted environment
// and every output chunk passes the redactors before it is stored or shown.
// extra (the registered streaming redactor, P7-TRUST-19) runs first, on the raw
// stream; the default line redactor then catches named secrets it cannot know.
func newExecutor(stale ciexec.StaleChecker, extra func() ciexec.Redactor) *ciexec.Executor {
	newR := ciexec.NewOutputRedactor
	if extra != nil {
		newR = func() ciexec.Redactor { return chainRedactor{extra(), ciexec.NewOutputRedactor()} }
	}
	return &ciexec.Executor{Stale: stale, Env: ciexec.AllowlistEnv{}, NewRedactor: newR}
}

// chainRedactor feeds the output of first into second.
type chainRedactor struct{ first, second ciexec.Redactor }

func (c chainRedactor) Redact(p []byte) []byte { return c.second.Redact(c.first.Redact(p)) }
func (c chainRedactor) Flush() []byte {
	return append(c.second.Redact(c.first.Flush()), c.second.Flush()...)
}
