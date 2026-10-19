// Command nself-ci-agent is the small node binary of the CI fabric (ADR 0024 §2):
// it runs leased jobs on a host and speaks runner protocol v1.
//
// Purpose: dispatch to the registered subcommands (version, exec, serve here;
// join, pull, advertise, upgrade, verify-release and job-shim from their own
// files). It is a composition root: it wires internal/agent/*, internal/exec
// and internal/evidence and holds no logic of its own.
// Inputs:  argv; $NSELF_CI_AGENT_HOME for the state dir.
// Outputs: exit code 0 ok, 1 failed, 2 usage.
// Constraints: CGO_ENABLED=0 builds for linux/amd64, linux/arm64, darwin/arm64.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func main() {
	signal.Ignore(syscall.SIGPIPE) // a peer that closed our stdout is a send error, not a kill
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], IO{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr})
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, sio IO) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent <command> [args]")
		for _, l := range usageLines() {
			fmt.Fprintln(sio.Stderr, l)
		}
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	fn, ok := lookup(args[0])
	if !ok {
		fmt.Fprintf(sio.Stderr, "nself-ci-agent: unknown command %q\n", args[0])
		return 2
	}
	return fn(ctx, args[1:], sio)
}

// stateDir is $NSELF_CI_AGENT_HOME, else ~/.nself/ci-agent (user mode). System
// mode (/var/lib/nself-ci-agent) sets the variable in its service unit (P7-NODE-06).
func stateDir() (string, error) {
	if v := os.Getenv("NSELF_CI_AGENT_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nself", "ci-agent"), nil
}

// Job and evidence helpers shared by `exec` and `serve` (exec.go, serve.go).

func unbind(ev *model.Evidence, reason, why string) {
	r := model.BindingReason(reason)
	ev.WorktreeState, ev.Binding = "dirty", model.Binding{State: "unbound", Reason: &r}
	ev.Freshness = model.Freshness{State: "unknown", Reason: &why}
}

// jobOutcome is the run package's record of the job, built from the executor's result.
func jobOutcome(spec execSpec, res ciexec.AttemptResult, tail string) cirun.JobOutcome {
	o := cirun.JobOutcome{ID: spec.JobID, Name: spec.JobID, Check: spec.JobID, Tool: "nself-ci-agent", Kind: spec.Job.Kind,
		Required: true, Substantive: true, FailureClass: res.Attempt.FailureClass, ExitCode: res.ExitCode, AttemptID: spec.AttemptID, N: 1,
		Duration: res.Finished.Sub(res.Started), Tail: tail, Reason: model.Reason(res.Reason)}
	switch res.Attempt.State {
	case "passed":
		o.State = cirun.StatePassed
	case "cancelled":
		o.State = cirun.StateCancelled
	default:
		o.State = cirun.StateFailed
	}
	var ce *ciexec.Error
	if errors.As(res.Err, &ce) {
		o.Code = ce.Code
	}
	return o
}

// baseEvidence is the run document before the P7-CI-38 builder completes it:
// one check for the one job, built by run.SingleJobEvidence. The producer is
// self-reported until the result signer (P7-TRUST-20) signs it.
func baseEvidence(spec execSpec, o cirun.JobOutcome, start, end time.Time) model.Evidence {
	id := sha256.Sum256([]byte(spec.CoordinatorID + "/" + spec.AttemptID))
	e, _ := cirun.SingleJobEvidence(cirun.SingleJob{RunID: hex.EncodeToString(id[:8]), Revision: spec.Revision, Name: spec.JobID, Outcome: o, Start: start, End: end})
	return e
}

// unbindIfOtherRevision unbinds evidence whose named revision is not the commit
// the source tree is at: bound, current evidence must describe what ran.
func unbindIfOtherRevision(ctx context.Context, spec execSpec, ev *model.Evidence) {
	if spec.SourceDir == "" {
		return
	}
	out, exit, err := report.Capture(ctx, "git", "-c", "core.fsmonitor=false", "-C", spec.SourceDir, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	head := strings.ToLower(strings.TrimSpace(out))
	if err == nil && exit == 0 && head == strings.ToLower(spec.Revision) {
		return
	}
	unbind(ev, "sha_mismatch", "the source tree is not at the named revision")
}

// workBase is where per-run executor work areas go: outside the agent state dir
// (which holds the keys), in a 0700 directory of the user's own. It is never a
// shared directory such as /tmp, where another user could pre-create it.
func workBase() (string, error) {
	dir := os.Getenv("NSELF_CI_AGENT_WORK")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".nself", "ci-agent-work")
	}
	if err := nodekey.EnsureDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func mapSecrets(m map[string]string) ciexec.SecretSource { return secretMap(m) }

type secretMap map[string]string

func (s secretMap) Resolve(_ context.Context, refs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range refs {
		v, ok := s[r]
		if !ok {
			return nil, fmt.Errorf("E612: secret %q was not delivered", r)
		}
		out[r] = v
	}
	return out, nil
}

// tailBuffer keeps the last max bytes of output for the failure excerpt.
type tailBuffer struct {
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
}
func (t *tailBuffer) String() string { return string(t.buf) }

const (
	// excerptMax is the excerpt that goes into the evidence.
	excerptMax = 2000
)

func clipTail(s string) string {
	if len(s) <= excerptMax {
		return s
	}
	s = s[len(s)-excerptMax:]
	return strings.ToValidUTF8(s, "")
}
