package diagnose

// Purpose: the table of infrastructure failure signatures. A job that ran and
// exited non-zero is a code failure unless its exit status, the executor's
// error text or the end of its redacted output matches a row here.
// Inputs:  none (a fixed table). Outputs: Signatures, matched by Classify.
// Constraints: pure data; a row names one cause, one remediation, and where it
// may match (an executor error is stronger evidence than program output, so
// output rows match only the last OutputLines lines and never a bare word; an
// exit status counts only when the executor reported no error, and a plain
// exit status chosen by the program (137, 127) is never enough on its own);
// unknown stays code (misclassifying a code failure as infra hides bugs).

import "regexp"

// TailWindow is how much of the end of the output text signatures may match.
const TailWindow = 4 << 10

// OutputLines is how many of the last non-empty output lines signatures may
// match: a test that prints an expected infra message and then goes on to
// report its own assertion is a code failure.
const OutputLines = 3

// Signature IDs recorded in evidence (`check.diagnosis`).
const (
	SigOOM       = "infra.oom"
	SigDisk      = "infra.disk_full"
	SigDNS       = "infra.dns"
	SigDocker    = "infra.docker_daemon"
	SigTool      = "infra.tool_missing"
	SigExecutor  = "infra.executor"
	SigTimeout   = "timeout"
	SigCode      = "code"
	SigCancelled = "cancelled"
	SigLost      = "lost"
)

// signature is one infrastructure cause.
type signature struct {
	id          string
	exits       []int          // exit statuses that alone identify the cause
	errText     *regexp.Regexp // the executor, admission or prepare error text
	output      *regexp.Regexp // the end of the redacted output
	remediation string
}

// testFailure marks output that reports a failing test or assertion: a message
// in the same lines (an expected DNS error, a disk-full path under test) is the
// program's own, so no output row may match there.
var testFailure = re(`(?i)(^|\s)(FAILED|FAIL|--- FAIL)\b|(?m)^not ok\b|✕|✗|assertion ?error|assertion failed|panic:`)

func re(s string) *regexp.Regexp { return regexp.MustCompile(s) }

// signatures are tried in order; the first match wins.
var signatures = []signature{
	{
		id:          SigOOM,
		exits:       []int{-1}, // killed by a signal (the executor reports exit -1, no error); a plain `exit 137` is the job's own code and is not enough
		errText:     re(`(?i)signal: killed|oomkilled|out of memory`),
		output:      re(`(?im)oomkilled|out of memory: kill|oom-kill|cannot allocate memory|^\w*sh: (line \d+: )? *\d+ +killed\b`),
		remediation: "The process was killed by a signal, most likely for memory. Free memory on the runner, lower the job's parallelism, or raise `requires.mem_mb`; then run it again.",
	},
	{
		id:          SigDisk,
		errText:     re(`(?i)no space left on device|disk quota exceeded`),
		output:      re(`(?i)no space left on device|disk quota exceeded`),
		remediation: "The disk is full. Free space on the runner's work and cache volumes (`nself ci runs prune`, Docker prune), then run it again.",
	},
	{
		id:          SigDocker,
		errText:     re(`(?i)cannot connect to the docker daemon|is the docker daemon running|error during connect: .*docker`),
		output:      re(`(?i)cannot connect to the docker daemon|is the docker daemon running`),
		remediation: "The Docker daemon is not reachable. Start Docker (or Colima), check `docker info`, then run it again.",
	},
	{
		id:          SigDNS,
		errText:     re(`(?i)temporary failure in name resolution|could not resolve host|name or service not known|no such host|dial tcp: lookup `),
		output:      re(`(?i)temporary failure in name resolution|could not resolve host|name or service not known|getaddrinfo (enotfound|eai_again)|dial tcp: lookup .* (no such host|server misbehaving)`),
		remediation: "A DNS lookup failed. Check the runner's network and resolver, then run it again; use `--network` to confirm the job may reach the internet.",
	},
	{
		id:          SigTool,
		errText:     re(`(?i)executable file not found|fork/exec .*: no such file or directory|command not found`),
		output:      re(`(?im)command not found|executable file not found|^\S*sh: (line \d+: |\d+: )?\S+: not found$`),
		remediation: "A tool the job needs is not installed on the runner. Install it, or declare it under `requires.tools` so the job only lands where it exists.",
	},
}

// remediations for the classes that carry no signature.
var classRemediation = map[string]string{
	SigCode:      "The job's own command failed. Read the excerpt, fix the code or the test, and reproduce it with `nself ci --job <job>`.",
	SigTimeout:   "The job hit its timeout and was stopped. Find what hangs, or raise the job's `timeout` in .nself/ci.yaml.",
	SigCancelled: "The job was cancelled by the operator or by fail-fast. Nothing to fix; run it again when ready.",
	SigLost:      "The runner or coordinator stopped answering. Run it again; `nself ci runs` shows whether the run was recovered.",
	SigExecutor:  "The CI executor could not run the job (not the job's own code). Check `nself ci doctor`, then run it again.",
}
