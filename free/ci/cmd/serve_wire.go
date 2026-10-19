package main

// Purpose: the composition root of the V15 coordinator's lifecycle pieces (P7-NODE-20): it hands the
//          coordinator's own store to the agent and enroll listeners, registers the pairing service that
//          burst offers use in this process, and builds the graceful drain that runs on SIGTERM.
// Inputs:  the open coordinator store, the data dir and the serve Config being built.
// Outputs: serveWire sets Config.Drain and registers the default pairing service; the returned func undoes
//          the registration.
// Constraints: the drain runs after intake has stopped (coordinator.Run pauses it first) and before the
//          running dispatches are cancelled; cancels reach agents through the live agent server; with no
//          agent listener running there is nothing to cancel, and the drain only waits out the deadline.

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/drain"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// activeDrain is the drain that is running, so agent acks of a cancel reach it.
var activeDrain atomic.Pointer[drain.Drainer]

// noIntake is the drain's Intake: coordinator.Run has already paused the real one.
type noIntake struct{}

func (noIntake) Stop() {}

// liveCanceller sends a cancel through the running agent server, if any.
type liveCanceller struct{}

func (liveCanceller) Cancel(nodeID, leaseID, reason string) error {
	s := activeAgent.Load()
	if s == nil {
		return fmt.Errorf("no agent listener is running")
	}
	return s.Cancel(nodeID, leaseID, reason)
}

// serveWire connects the listeners and the drain to the coordinator's store.
func serveWire(ctx context.Context, st *store.Store, home, coordinatorID string, cfg *coordinator.Config) (undo func()) {
	useEnrollStore(st)
	useAgentStore(st, home, "")
	reg := registry.New(st)
	if ca, err := pki.New(ctx, pki.Options{Store: st, DataDir: home}); err != nil {
		fmt.Fprintf(os.Stderr, "nself-ci-serve: the pairing service is off (coordinator PKI: %v)\n", err)
	} else {
		pairing.Use(pairing.New(pairing.Config{CA: ca, Store: st, Registry: reg, CoordinatorID: "local", Actor: "coordinator"}))
	}
	agentOnAck = func(_, leaseID string) {
		if d := activeDrain.Load(); d != nil {
			d.Ack(leaseID)
		}
	}
	eng := lease.New(st, reg, lease.Config{})
	cfg.Drain = func(dctx context.Context) error {
		d, err := drain.New(drain.Config{
			Intake: noIntake{}, Canceller: liveCanceller{}, Deadline: drain.FlagDeadline(), Logf: cfg.Logf,
			Leases: &drain.EngineLeases{Engine: eng, CoordinatorID: coordinatorID, JobInfo: func(c context.Context, attempt string) (string, bool, error) {
				return st.AttemptJobClass(c, attempt)
			}},
		})
		if err != nil {
			return err
		}
		activeDrain.Store(d)
		rep, err := d.Run(dctx)
		if err == nil && rep.ExitCode() != 0 {
			err = fmt.Errorf("drain finished with errors")
		}
		return err
	}
	return func() { pairing.Use(nil); agentOnAck = nil }
}
