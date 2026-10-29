package main

// Purpose: `nself-ci-agent serve --stdio`: protocol v1 over stdin and stdout,
// the agent half of carrier A (a coordinator starts it over SSH).
// Inputs:  stdin frames from the coordinator; flags --slots, --coordinator,
// --root-fingerprint, --accept-secret-values.
// Outputs: the NSELF-CI-AGENT/1 preamble, then protocol frames on stdout.
// Constraints: PreStart hooks run before anything is read from stdin or
// written to stdout; the node key and age identity are created on first start
// in the 0700 state dir; value-bearing secrets frames are refused unless the
// coordinator started the agent with --accept-secret-values (PERSONAL
// coordinator, operator-owned secrets); carrier A should start it through a
// forced command (`serve --stdio ...`) so the coordinator cannot run `slots --set`; logs and errors go to stderr, which
// the SSH carrier keeps apart from the frame stream.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/agent/agekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func init() {
	must(RegisterSubcommand("serve", "serve protocol v1 on stdin/stdout (--stdio)", runServe))
	must(RegisterSubcommand("slots", "show or set (--set N) this host's slot cap", runSlots))
}

// runSlots is the operator's lever on the host slot cap.
func runSlots(_ context.Context, args []string, sio IO) int {
	fs := flag.NewFlagSet("slots", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	set := fs.Int("set", 0, "set the host slot cap to N")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *set < 0 {
		_, _ = fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent slots [--set N]")
		return 2
	}
	state, err := stateDir()
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	if *set > 0 {
		if err := slots.SetCap(state, *set); err != nil {
			_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
			return 1
		}
	}
	capacity, used, err := slots.Status(state)
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	_, _ = fmt.Fprintf(sio.Stdout, "cap %d used %d\n", capacity, used)
	return 0
}

var idRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func runServe(ctx context.Context, args []string, sio IO) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	stdio := fs.Bool("stdio", false, "serve over stdin and stdout (carrier A)")
	nslots := fs.Int("slots", 0, "lower the concurrent leases this process takes (the host cap is half the CPUs until `slots --set`)")
	coordinator := fs.String("coordinator", "stdio", "id of the coordinator on the other end")
	fingerprint := fs.String("root-fingerprint", "", "fingerprint of that coordinator's root")
	secretValues := fs.Bool("accept-secret-values", false, "accept value-bearing secrets frames (PERSONAL coordinator)")
	if err := fs.Parse(args); err != nil || !*stdio || fs.NArg() != 0 || !idRE.MatchString(*coordinator) {
		_, _ = fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent serve --stdio [--slots N] [--coordinator ID] [--root-fingerprint FP] [--accept-secret-values]")
		return 2
	}
	if err := RunPreStart(ctx, "serve"); err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	state, err := stateDir()
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	key, err := nodekey.LoadOrCreate(state)
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error: node key:", err)
		return 1
	}
	age, err := agekey.LoadOrCreate(ctx, state)
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error: age identity:", err)
		return 1
	}
	capacity := ciexec.Probe()
	// The host cap is recorded by the first start (default: half the CPUs) and
	// changed only by `slots --set`; --slots, which the coordinator chooses, can
	// only lower this process's share.
	table, err := slots.Open(state, max(1, capacity.CPUs/2))
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	table.Lower(*nslots)
	policy := protocol.CarrierPolicy{Carrier: "A", Personal: *secretValues, OperatorOwned: *secretValues}
	car, err := session.NewStreamCarrier(sio.Stdin, sio.Stdout, policy, true)
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	car = withCheckout(car, state)
	s, err := session.New(session.Config{
		Carrier: car, Key: key, Slots: table, StateDir: state,
		Facts:       session.Facts{AgentVersion: Version, MachineID: machineID(), CapabilityDigest: capabilityDigest(), AgeRecipient: age.Recipient},
		Runner:      leaseRunner{coordinator: *coordinator},
		Coordinator: session.Coordinator{ID: *coordinator, RootFingerprint: *fingerprint},
		Status:      hostStatus,
	})
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	if err := s.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	return 0
}

// leaseRunner runs a lease's job through the exec path.
type leaseRunner struct{ coordinator string }

func (r leaseRunner) Run(ctx context.Context, j session.JobRun) (session.JobResult, error) {
	l := j.Lease
	if l.Checkout.Size > 0 && j.Setup.SourceDir == "" {
		// Staging the checkout is a Before hook's job (internal/agent/checkout); a job
		// run on an empty tree would report a pass for code it never saw.
		return session.JobResult{}, errors.New("the lease has a checkout but no hook staged it")
	}
	spec := execSpec{AttemptID: l.AttemptID, CoordinatorID: r.coordinator, JobID: "job", Job: l.Job, NetworkScope: l.NetworkScope, Trust: l.Trust, Revision: l.Checkout.Revision, SourceDir: j.Setup.SourceDir, Evidence: l.Evidence}
	if l.Evidence != nil && l.Evidence.Job != "" { // the evidence names the job the coordinator recorded for the attempt
		spec.JobID = l.Evidence.Job
	}
	in := jobInputs{Slot: j.Slot, Env: j.Setup.Env, Secrets: j.Setup.Secrets, Mounts: j.Setup.Mounts, Output: j.Output, Tail: j.Tail, Skip: j.Skip}
	if len(j.Credential) > 0 {
		if err := wrapCredential(&spec, &in, j); err != nil {
			return session.JobResult{}, err
		}
	}
	return runSpec(ctx, spec, in)
}

// entrypointKinds are the credential kinds whose runner image has an entrypoint that is not a pass-through
// (the gitlab-runner image runs `dumb-init /entrypoint`, which execs `gitlab-runner "$@"`), so the wrapper
// must be the entrypoint. The GitHub runner image has none: its job command is appended after the image.
var entrypointKinds = map[string]bool{"gitlab-runner-token": true}

// containerAgentPath is where a container job sees the agent binary (a Linux agent: the
// binary is bind-mounted read-only, so it must match the container's OS).
const containerAgentPath = "/usr/local/bin/nself-ci-agent"

// wrapCredential runs the job's command under `cred-wrapper <kind> --` and feeds the
// credential on the job's stdin, so it reaches the runner's environment only: never
// argv, a container -e flag or a file. The value is also registered for output masking.
func wrapCredential(spec *execSpec, in *jobInputs, j session.JobRun) error {
	rc := j.Lease.RunnerCredential
	if rc == nil || credEnv[rc.Kind] == "" {
		return errors.New("runner_credential has an unknown kind")
	}
	if len(spec.Job.Run) == 0 {
		return errors.New("runner_credential needs a job command to hand it to")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("agent binary: %w", err)
	}
	agent := exe
	wrap := func() []string { return []string{agent, "cred-wrapper", rc.Kind, "--"} }
	if spec.Job.Isolation == "container" || spec.Job.Isolation == "sandboxed-container" { // both run in a container image (gVisor is the same CLI path)
		agent = containerAgentPath
		in.Mounts = append(append([]ciexec.Mount(nil), in.Mounts...), ciexec.Mount{Host: exe, Target: agent, ReadOnly: true})
		in.RunnerRoot = true
		if entrypointKinds[rc.Kind] {
			// the image's own entrypoint is a launcher (gitlab-runner): the agent takes its place and the
			// command starts at the wrapper, so the wrapper still runs first and the credential reaches only the runner
			in.Entrypoint = agent
			wrap = func() []string { return []string{"cred-wrapper", rc.Kind, "--"} }
		}
	}
	spec.Job.Run = append(wrap(), spec.Job.Run...)
	in.Stdin = io.MultiReader(bytes.NewReader(j.Credential), strings.NewReader("\n"))
	in.Masks = append(in.Masks, string(j.Credential))
	in.Masks = append(in.Masks, partialMasks(rc.Kind, string(j.Credential))...)
	return nil
}

// routableToken matches the type marker of a routable runner token (glrt-t1_, glrt-t2_, ...).
var routableToken = regexp.MustCompile(`^t[0-9]+_`)

// partialMasks are the parts of a credential its program prints on its own. gitlab-runner logs the first nine
// characters of the token's random part in "Starting runner for <url> with token <nine>" and in every
// "runner=<nine>" field (measured on v19.4.1), so those nine are masked as well as the whole value.
func partialMasks(kind, value string) []string {
	if kind != "gitlab-runner-token" {
		return nil
	}
	rest := routableToken.ReplaceAllString(strings.TrimPrefix(value, "glrt-"), "")
	if len(rest) < 9 {
		return nil
	}
	return []string{rest[:9]}
}

func hostStatus() session.Status {
	c := ciexec.Probe()
	st := session.Status{Interactive: c.Interactive}
	if c.BatteryPercent >= 0 {
		onBattery := !c.PluggedIn
		st.OnBattery = &onBattery
	}
	return st
}

// machineID is stable per host and name; it carries no hardware serial.
func machineID() string {
	host, _ := os.Hostname()
	sum := sha256.Sum256([]byte("nself-ci-agent/machine/" + host))
	return "m-" + hex.EncodeToString(sum[:8])
}

// capabilityDigest changes when the agent build or platform does; the full
// capability document is advertised by P7-NODE-12's subcommand.
func capabilityDigest() string {
	sum := sha256.Sum256([]byte(Version + "/" + runtime.GOOS + "/" + runtime.GOARCH))
	return hex.EncodeToString(sum[:])
}
