package main

// Purpose: `nself-ci-agent pull` (P7-NODE-08, carrier B): the agent keeps an outbound TLS 1.3 mTLS connection to
//          the coordinator's agent listener, long-polls for leases and runs them under the same session as
//          `serve --stdio`. It is the long-running half of a node that cannot be reached by ssh.
// Inputs:  the enrollment `join` saved under <state>/pki (pinned root, certificate, agent URL); flags --slots,
//          --coordinator, --project, --secrets-eligible, --dedicated.
// Outputs: jobs run on this node; logs, heartbeats and results sent through the carrier; <state>/clock_skew
//          when the coordinator clock is off; exit 0 after a drain, 1 when the coordinator rejects this agent
//          or the node is not enrolled, 2 on usage.
// Constraints: the client accepts only a coordinator-kind server leaf chained to the root pinned at join and
//          reads its own certificate from disk on every handshake, so a renewed one is used without a
//          restart; a lost coordinator never ends the process (the fence kills the jobs after agent_ttl and the
//          session restarts with backoff); secret references are resolved on this node under its own age
//          identity and no value is ever received from the coordinator; the job token never enters the job
//          (the session masks it and the executor never sees it).

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/agekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

// pullResolver builds the node-side secret resolver; tests replace it with a fake that needs no nself cli.
var pullResolver = nodeResolver

func init() {
	must(RegisterSubcommand("pull", "keep an mTLS connection to the coordinator and run the leases it hands out (carrier B)", runPull))
}

// restartHealthy is how long a session must have lasted for the restart backoff to start over.
const restartHealthy = 30 * time.Second

type pullFlags struct {
	slots          int
	coordinator    string
	project        string
	secretsOK      bool
	dedicated      bool
	restartBackoff agent.Backoff
}

func parsePull(args []string, sio IO) (pullFlags, bool) {
	var f pullFlags
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	fs.SetOutput(sio.Stderr)
	fs.IntVar(&f.slots, "slots", 0, "lower the concurrent leases this process takes")
	fs.StringVar(&f.coordinator, "coordinator", "", "id of the coordinator (default: derived from the pinned root)")
	fs.StringVar(&f.project, "project", "", "project root whose secret store this node resolves from (enables node-side secrets)")
	fs.BoolVar(&f.secretsOK, "secrets-eligible", false, "this node may receive project and environment secrets")
	fs.BoolVar(&f.dedicated, "dedicated", false, "this node is dedicated to the project (also release and deploy secrets)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || f.slots < 0 || (f.coordinator != "" && !idRE.MatchString(f.coordinator)) {
		_, _ = fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent pull [--slots N] [--coordinator ID] [--project DIR] [--secrets-eligible] [--dedicated]")
		return f, false
	}
	return f, true
}

// coordinatorID names the coordinator in the agent's records: the first 16 hex digits of the pinned root.
func coordinatorID(fingerprint string) string {
	digits := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'f', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'F':
			return r + ('a' - 'A')
		}
		return -1
	}, strings.TrimPrefix(fingerprint, "SHA256:"))
	return "c-" + digits[:min(16, len(digits))]
}

// pullTLS is the client TLS config: pinned root, coordinator-kind server leaf only, certificate read per handshake.
func pullTLS(state string, cred enroll.Credential, serverName string) (*tls.Config, error) {
	cfg, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: cred.RootDER, Cert: &cred.Cert, ServerName: serverName})
	if err != nil {
		return nil, err
	}
	cfg.Certificates = nil
	cfg.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		c, err := enroll.LoadCredential(state)
		if err != nil {
			return nil, err
		}
		return &c.Cert, nil
	}
	return cfg, nil
}

func runPull(ctx context.Context, args []string, sio IO) int {
	f, ok := parsePull(args, sio)
	if !ok {
		return 2
	}
	if err := RunPreStart(ctx, "pull"); err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	state, err := stateDir()
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	cred, err := enroll.LoadCredential(state)
	if errors.Is(err, os.ErrNotExist) {
		_, _ = fmt.Fprintln(sio.Stderr, "error: this node has not joined a coordinator; run `nself-ci-agent join <enroll-url>` first")
		return 1
	}
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error: enrollment:", err)
		return 1
	}
	base, err := url.Parse(cred.Enrollment.AgentURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" {
		_, _ = fmt.Fprintln(sio.Stderr, "error: the enrollment holds no https agent URL; join again")
		return 1
	}
	tlsc, err := pullTLS(state, cred, base.Hostname())
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error: enrollment:", err)
		return 1
	}
	if f.coordinator == "" {
		f.coordinator = coordinatorID(cred.Enrollment.RootFingerprint)
	}
	return pullLoop(ctx, f, state, cred, base.String(), tlsc, sio)
}

// pullLoop runs sessions until a drain completes, the coordinator rejects the agent, or ctx ends.
func pullLoop(ctx context.Context, f pullFlags, state string, cred enroll.Credential, baseURL string, tlsc *tls.Config, sio IO) int {
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
	table, err := slots.Open(state, max(1, ciexec.Probe().CPUs/2))
	if err != nil {
		_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
		return 1
	}
	table.Lower(f.slots)
	logf := func(format string, a ...any) {
		_, _ = fmt.Fprintf(sio.Stderr, "nself-ci-agent pull: "+format+"\n", a...)
	}
	resolver := pullResolver(state, f, cred.Enrollment.NodeID, age.Path)
	for ctx.Err() == nil {
		car := agent.NewCarrier(agent.CarrierConfig{
			Client: agent.Options{BaseURL: baseURL, TLS: tlsc, OnSkew: func(d time.Duration) { noteSkew(state, d) }},
			Logf:   logf,
		})
		watch := &drainWatch{Frames: agent.WithNodeSecrets(newFenceCarrier(car, state), resolver, logf)}
		s, err := session.New(session.Config{
			Carrier: watch, Key: key, Slots: table, StateDir: state,
			Facts:       session.Facts{AgentVersion: Version, MachineID: machineID(), CapabilityDigest: capabilityDigest(), AgeRecipient: age.Recipient},
			Runner:      leaseRunner{coordinator: f.coordinator},
			Coordinator: session.Coordinator{ID: f.coordinator, RootFingerprint: cred.Enrollment.RootFingerprint},
			Status:      hostStatus,
		})
		if err != nil {
			_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
			return 1
		}
		began := time.Now()
		err = s.Run(ctx)
		if time.Since(began) >= restartHealthy {
			f.restartBackoff.Reset()
		}
		var rej *session.RejectError
		switch {
		case errors.As(err, &rej):
			_, _ = fmt.Fprintln(sio.Stderr, "error:", err)
			return 1
		case watch.drained():
			return 0
		case ctx.Err() != nil:
			return 0
		}
		d := f.restartBackoff.Next()
		logf("session ended (%v); starting a new one in %s", err, d.Round(time.Millisecond))
		select {
		case <-time.After(d):
		case <-ctx.Done():
		}
	}
	return 0
}

// drainWatch remembers that the coordinator asked this agent to drain, so the end of that session is final.
type drainWatch struct {
	agent.Frames
	did atomic.Bool
}

func (d *drainWatch) Recv() (protocol.Message, error) {
	m, err := d.Frames.Recv()
	if _, ok := m.Body.(*protocol.Drain); ok {
		d.did.Store(true)
	}
	return m, err
}

func (d *drainWatch) drained() bool { return d.did.Load() }

// noteSkew leaves a marker the doctor reads when the coordinator clock is off by more than the tolerance.
func noteSkew(state string, d time.Duration) {
	path := filepath.Join(state, "clock_skew")
	if d <= pki.ClockSkew && d >= -pki.ClockSkew {
		_ = os.Remove(path)
		return
	}
	_ = os.WriteFile(path, []byte(d.Round(time.Second).String()+"\n"), 0o600)
}
