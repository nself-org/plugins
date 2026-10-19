package run

import ciexec "github.com/nself-org/plugins/free/ci/internal/exec"

// newExecutor builds the job executor: jobs get an allowlisted environment
// and every output chunk passes the redactors before it is stored or shown.
// extra (the registered streaming redactor, P7-TRUST-19) runs first, on the raw
// stream; the default line redactor then catches named secrets it cannot know.
// Resolved job secrets are registered with Redactions before the job starts, so
// the artifact and cache scanners see them (D-1013).
func newExecutor(stale ciexec.StaleChecker, extra func() ciexec.Redactor, iso ciexec.Isolator) *ciexec.Executor {
	newR := ciexec.NewOutputRedactor
	if extra != nil {
		newR = func() ciexec.Redactor { return chainRedactor{extra(), ciexec.NewOutputRedactor()} }
	}
	return &ciexec.Executor{Stale: stale, Isolation: iso, Env: ciexec.AllowlistEnv{}, NewRedactor: newR, RegisterSecret: RegisterSecretValue}
}

// chainRedactor feeds the output of first into second.
type chainRedactor struct{ first, second ciexec.Redactor }

func (c chainRedactor) Redact(p []byte) []byte { return c.second.Redact(c.first.Redact(p)) }
func (c chainRedactor) Flush() []byte {
	return append(c.second.Redact(c.first.Flush()), c.second.Flush()...)
}
