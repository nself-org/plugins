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
// coordinator, operator-owned secrets); logs and errors go to stderr, which
// the SSH carrier keeps apart from the frame stream.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"

	"github.com/nself-org/plugins/free/ci/internal/agent/agekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func init() {
	must(RegisterSubcommand("serve", "serve protocol v1 on stdin/stdout (--stdio)", runServe))
}

var idRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func runServe(ctx context.Context, args []string, sio IO) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	stdio := fs.Bool("stdio", false, "serve over stdin and stdout (carrier A)")
	nslots := fs.Int("slots", 0, "concurrent leases this host allows (default: half the CPUs)")
	coordinator := fs.String("coordinator", "stdio", "id of the coordinator on the other end")
	fingerprint := fs.String("root-fingerprint", "", "fingerprint of that coordinator's root")
	secretValues := fs.Bool("accept-secret-values", false, "accept value-bearing secrets frames (PERSONAL coordinator)")
	if err := fs.Parse(args); err != nil || !*stdio || fs.NArg() != 0 || !idRE.MatchString(*coordinator) {
		fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent serve --stdio [--slots N] [--coordinator ID] [--root-fingerprint FP] [--accept-secret-values]")
		return 2
	}
	if err := RunPreStart(ctx, "serve"); err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	state, err := stateDir()
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	key, err := nodekey.LoadOrCreate(state)
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error: node key:", err)
		return 1
	}
	age, err := agekey.LoadOrCreate(ctx, state)
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error: age identity:", err)
		return 1
	}
	capacity := ciexec.Probe()
	total := *nslots
	if total <= 0 {
		total = max(1, capacity.CPUs/2)
	}
	table, err := slots.Open(state, total)
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	policy := protocol.CarrierPolicy{Carrier: "A", Personal: *secretValues, OperatorOwned: *secretValues}
	car, err := session.NewStreamCarrier(sio.Stdin, sio.Stdout, policy, true)
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	s, err := session.New(session.Config{
		Carrier: car, Key: key, Slots: table, StateDir: state,
		Facts:       session.Facts{AgentVersion: Version, MachineID: machineID(), CapabilityDigest: capabilityDigest(), AgeRecipient: age.Recipient},
		Runner:      leaseRunner{home: state, coordinator: *coordinator},
		Coordinator: session.Coordinator{ID: *coordinator, RootFingerprint: *fingerprint},
		Status:      hostStatus,
	})
	if err != nil {
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	var rejected *session.RejectError
	switch err := s.Run(ctx); {
	case err == nil, errors.Is(err, context.Canceled):
		return 0
	case errors.As(err, &rejected):
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	default:
		fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
}

// leaseRunner runs a lease's job through the exec path.
type leaseRunner struct{ home, coordinator string }

func (r leaseRunner) Run(ctx context.Context, j session.JobRun) (session.JobResult, error) {
	if len(j.Credential) > 0 {
		// The executor cannot yet hand a credential to a container on stdin
		// (HOST review R3), and putting it in argv or -e is what this must not do.
		return session.JobResult{}, errors.New("runner_credential is not deliverable yet: the executor has no stdin path")
	}
	l := j.Lease
	spec := execSpec{AttemptID: l.AttemptID, CoordinatorID: r.coordinator, JobID: "job", Job: l.Job, Revision: l.Checkout.Revision, SourceDir: j.Setup.SourceDir}
	return runSpec(ctx, r.home, spec, jobInputs{Slot: j.Slot, Env: j.Setup.Env, Secrets: j.Setup.Secrets, Mounts: j.Setup.Mounts, Output: j.Output})
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
