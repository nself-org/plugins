//go:build integration

package remote

// Purpose: carrier A (ssh) of the simulation: the real ssh driver on the
// coordinator, a real agent session on the node, joined by in-process pipes.

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sdkremote "github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	sshdrv "github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

type fakeKey struct{}

func (fakeKey) KeyID() string        { return "k1" }
func (fakeKey) PublicString() string { return "pub" }

func sshNode(t *testing.T) (model.Capability, string) {
	t.Helper()
	home := t.TempDir()
	pin := sdkremote.PinnedHostKeys{Path: filepath.Join(home, "known_hosts")}
	if err := pin.Add("nself-ci-node1", "192.0.2.5 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDummyKeyForTestsOnlyDummyKeyForTestsOnly000"); err != nil {
		t.Fatal(err)
	}
	var n model.Capability
	n.Identity.ID = "node1"
	n.Identity.Transport = sshdrv.Kind
	op := "operator"
	n.Identity.Ownership.Value = &op
	n.SecretsEligible = true
	n.Identity.SSH = &model.CapabilitySSH{Hostname: "192.0.2.5", Port: 2222, User: "ci"}
	return n, home
}

// runSSH delivers the rig's lease to a node over the ssh driver and returns the driver's result.
func (r *rig) runSSH(n *node) error {
	r.t.Helper()
	capab, home := sshNode(r.t)
	state := r.t.TempDir()
	var once sync.Once
	starter := func(ctx context.Context, _ sdkremote.Target, _ string) (*sshdrv.Conn, error) {
		cr, cw := io.Pipe()
		ar, aw := io.Pipe()
		er, ew := io.Pipe()
		table, err := slots.Open(state, 2)
		if err != nil {
			return nil, err
		}
		actx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			car, err := session.NewStreamCarrier(cr, aw, protocol.CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}, true)
			if err == nil {
				var s *session.Session
				s, err = session.New(session.Config{Carrier: n.tap(car), Key: fakeKey{}, Slots: table, Runner: n.runner(), StateDir: state, Hooks: n.hooks(),
					Coordinator: session.Coordinator{ID: "coord", RootFingerprint: "fp"}, Facts: session.Facts{AgentVersion: "1.5.0", MachineID: "m1"}})
				if err == nil {
					err = s.Run(actx)
				}
			}
			_ = aw.Close()
			_ = ew.Close()
			done <- err
		}()
		return &sshdrv.Conn{Stdin: cw, Stdout: ar, Stderr: er, Wait: func() error { <-done; return nil },
			Kill: func() error { once.Do(cancel); _ = cr.Close(); _ = aw.Close(); return nil }}, nil
	}
	d := &sshdrv.Driver{Home: home, Start: starter, SSHVersion: func(context.Context) (sdkremote.Version, error) { return sdkremote.Version{Major: 9, Minor: 6}, nil },
		NoRenew: true, Heartbeat: time.Second, AgentTTL: 25 * time.Second, CancelGrace: 3 * time.Second, HelloTimeout: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return d.Run(ctx, capab, r.protoLease(), r.sink())
}

func (r *rig) protoLease() protocol.Lease {
	return protocol.Lease{LeaseID: r.lease.b.LeaseID, AttemptID: r.lease.b.AttemptID, Epoch: uint64(r.lease.b.Epoch), Token: "opaque", CompatMode: protocol.Compat15,
		Job: model.Job{Kind: "test", Run: []string{"true"}}, Cache: r.lease.b.Plan, Secrets: []protocol.SecretRef{}, ExpiresAt: "2099-01-01T00:00:00Z"}
}
