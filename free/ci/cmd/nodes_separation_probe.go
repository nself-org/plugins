package main

// Purpose: the separation proof of a system node (D-1230). After `nodes add --system` installs the layout,
// it runs two probes on the node through the same chain a job step uses (ssh user -> sudo -u nself-ci ->
// sudo -n -u nself-ci-job-1 <agent> job-shim): the chain must run `id -un` as nself-ci-job-1, and the
// job user must get EACCES reading the node key (<state>/node.key: /var/lib/nself-ci-agent on Linux,
// /var/db/nself-ci-agent on macOS). Only with both
// proofs does the add record uid_separation true.
// Inputs:  a runner bound to the node (fixed `sh -c` scripts built from constants, the request
//          single-quoted with shQuote, no backslash).
// Outputs: a probeResult (ok, a stable reason key); shimSeparation, the SeparationProber the add uses.
// Constraints: fail closed. The probe forces LC_ALL=C LANG=C, and the key proof needs both cat's exit
//          code 1 and its C-locale "Permission denied" line for the key path. Each probe prints its
//          exit code and folded output, so the verdict never rests on how the transport reports a
//          failure; any ssh error, unexpected user, a readable or missing key path (ENOENT means the
//          state directory is traversable) is false with a named reason. The fact is never inferred
//          from the installed configuration. What follows from it (signing, secrets) is derived by
//          lifecycle exactly as for any other separated node.

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap/darwin"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// Reason keys of the probe.
const (
	shimProbeOK          = "job_shim.ok"
	shimProbeChainFailed = "job_shim.chain_failed"
	shimProbeWrongUser   = "job_shim.wrong_user"
	shimProbeKeyReadable = "job_shim.key_readable"
	shimProbeKeyNoEACCES = "job_shim.key_not_eacces"
	shimProbeUnreadable  = "job_shim.probe_unreadable"
)

// nodeKeyPath is the node key a job user must not reach on a node of os.
func nodeKeyPath(os string) string {
	if os == "darwin" {
		return darwin.StateDir + "/node.key"
	}
	return bootstrap.StateDir + "/node.key"
}

type probeResult struct {
	OK     bool
	Reason string
}

// shimProbeScript runs one shim request through the job chain and prints "rc=<code>" then the output.
// The locale is C on both sides of sudo (the script's own and the request's env), so the key probe's
// message is the C one on any node; the request is single-quoted with every quote in it closed and
// reopened, so no request text can end the quoting.
func shimProbeScript(argv []string) string {
	var req strings.Builder
	enc := json.NewEncoder(&req)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"argv": argv, "dir": "/", "env": []string{"LC_ALL=C", "LANG=C", "PATH=/usr/bin:/bin"}})
	return "set -u\nLC_ALL=C\nLANG=C\nexport LC_ALL LANG\nout=$(echo " + shQuote(strings.TrimSpace(req.String())) +
		" | sudo -n -u " + bootstrap.AgentUser + " sudo -n -u " + bootstrap.JobUser(1) + " " + bootstrap.AgentPath +
		" job-shim 2>&1) && rc=0 || rc=$?\necho \"rc=$rc\"\necho \"$out\"\n"
}

// shQuote single-quotes s for sh without a backslash: each ' becomes '"'"'.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }

// runShimProbe returns the exit code and output lines of one probe (ok false: unreadable or transport error).
func runShimProbe(ctx context.Context, run bootstrap.RunFunc, argv []string) (int, []string, bool) {
	out, err := run(ctx, "sh", "-c", shimProbeScript(argv))
	if err != nil {
		return 0, nil, false
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i, l := range lines {
		if rc, found := strings.CutPrefix(strings.TrimSpace(l), "rc="); found {
			n := 0
			for _, c := range rc {
				if c < '0' || c > '9' {
					return 0, nil, false
				}
				n = n*10 + int(c-'0')
			}
			if rc == "" {
				return 0, nil, false
			}
			return n, lines[i+1:], true
		}
	}
	return 0, nil, false
}

// probeSeparation proves the sudo chain and the key EACCES; anything else is false with a reason.
func probeSeparation(ctx context.Context, run bootstrap.RunFunc, os string) probeResult {
	key := nodeKeyPath(os)
	rc, out, ok := runShimProbe(ctx, run, []string{"/usr/bin/id", "-un"})
	if !ok {
		return probeResult{Reason: shimProbeUnreadable}
	}
	if rc != 0 {
		return probeResult{Reason: shimProbeChainFailed}
	}
	if len(out) != 1 || strings.TrimSpace(out[0]) != bootstrap.JobUser(1) {
		return probeResult{Reason: shimProbeWrongUser}
	}
	rc, out, ok = runShimProbe(ctx, run, []string{"/bin/cat", key})
	if !ok {
		return probeResult{Reason: shimProbeUnreadable}
	}
	if rc == 0 {
		return probeResult{Reason: shimProbeKeyReadable}
	}
	if rc != 1 { // cat exits 1 on EACCES (GNU, BSD, BusyBox); anything else is the shim or sudo failing
		return probeResult{Reason: shimProbeKeyNoEACCES}
	}
	// GNU/BSD cat: "cat: <path>: Permission denied"; BusyBox: "cat: can't open '<path>': Permission denied"
	if len(out) != 1 || !strings.HasPrefix(out[0], "cat: ") || !strings.Contains(out[0], key) ||
		!strings.HasSuffix(strings.TrimSpace(out[0]), ": Permission denied") {
		return probeResult{Reason: shimProbeKeyNoEACCES}
	}
	return probeResult{OK: true, Reason: shimProbeOK}
}

// shimSeparation records the probe: uid_separation true only when it passed, false with the probe's
// reason otherwise (the inner prober's userns answer is kept as user_ns and never makes it true).
type shimSeparation struct {
	inner  lifecycle.SeparationProber
	result probeResult
}

// Detect implements lifecycle.SeparationProber.
func (s shimSeparation) Detect(ctx context.Context, run func(context.Context, ...string) (string, error), p lifecycle.Platform) lifecycle.SeparationFacts {
	var f lifecycle.SeparationFacts
	if s.inner != nil {
		f = s.inner.Detect(ctx, run, p)
	}
	return lifecycle.SeparationFacts{UIDSeparation: s.result.OK, UserNS: f.UserNS, Reason: s.result.Reason}
}
