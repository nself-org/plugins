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
	"flag"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/drain"
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// servePublicURL is --public-url: the host names and addresses the agent listener's server certificate carries
// (Epic D5), so a remote agent can verify the coordinator. Empty: loopback and the bind host only.
var servePublicURL string

func init() {
	coordinator.RegisterFlag(func(fs *flag.FlagSet) {
		fs.StringVar(&servePublicURL, "public-url", "", "URL agents reach this coordinator at (https://host:3864); its host becomes a SAN of the agent listener certificate")
	})
}

// activeDrain is the drain that is running, so agent acks of a cancel reach it.
var activeDrain atomic.Pointer[drain.Drainer]

// agentIntake is the drain's Intake: coordinator.Run has already paused the intake queue; this stops the
// agent API from handing out any further lease.
type agentIntake struct{}

func (agentIntake) Stop() {
	if s := activeAgent.Load(); s != nil {
		s.StopOffering()
	}
}

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
func serveWire(ctx context.Context, st *store.Store, home, coordinatorID string, cfg *coordinator.Config) (undo func(), err error) {
	useEnrollStore(st)
	useAgentStore(st, home, servePublicURL)
	reg := registry.New(st)
	if ca, err := pki.New(ctx, pki.Options{Store: st, DataDir: home}); err != nil {
		fmt.Fprintf(os.Stderr, "nself-ci-serve: the pairing service is off (coordinator PKI: %v)\n", err)
	} else {
		pairing.Use(pairing.New(pairing.Config{CA: ca, Store: st, Registry: reg, CoordinatorID: "local", AgentURL: servePublicURL, Actor: "coordinator"}))
	}
	agentOnAck = func(_, leaseID string) {
		if d := activeDrain.Load(); d != nil {
			d.Ack(leaseID)
		}
	}
	eng := lease.New(st, reg, lease.Config{})
	stopBeat, err := registerServe(ctx, st, eng)
	if err != nil { // a serve a restore cannot see would be replaced under its own feet: refuse to run
		return func() {}, err
	}
	cfg.Drain = func(dctx context.Context) error {
		d, err := drain.New(drain.Config{
			Intake: agentIntake{}, Canceller: liveCanceller{}, Deadline: drain.FlagDeadline(), Logf: cfg.Logf,
			Leases: &drain.EngineLeases{Engine: eng, CoordinatorID: coordinatorID, Owned: ownedByThisProcess(st), JobInfo: func(c context.Context, attempt string) (string, bool, error) {
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
	return func() { stopBeat(); pairing.Use(nil); agentOnAck = nil }, nil
}

// registerServe makes this serve process visible in the coordinator table and keeps its heartbeat fresh, so
// `serve restore` sees an idle coordinator and refuses (backup.running), and the dispatcher's incarnation is
// read from a table that has a row. It then gives the leases of earlier coordinators their restart grace
// (Epic D6: expires_at = max(stored, start + TTL)). The returned func stops the heartbeat and waits for it.
func registerServe(ctx context.Context, st *store.Store, eng *lease.Engine) (stop func(), err error) {
	host, _ := os.Hostname()
	reg, err := st.Register(ctx, os.Getpid(), strconv.FormatInt(time.Now().UnixNano(), 10), "serve-"+host)
	if err != nil {
		return func() {}, fmt.Errorf("cannot register in the coordinator table (a restore would not see this process): %w", err)
	}
	inc, _ := st.HighestIncarnation(ctx)
	if _, err = eng.Start(ctx, reg.ID, inc); err != nil {
		fmt.Fprintf(os.Stderr, "nself-ci-serve: restart grace for earlier leases failed: %v\n", err)
	}
	bctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-bctx.Done():
				return
			case <-t.C:
				_ = st.Heartbeat(bctx, reg.ID)
			}
		}
	}()
	return func() { cancel(); wg.Wait() }, nil
}

// ownedByThisProcess reports whether a coordinator row was registered by this process: the pipelines this
// serve dispatches register as coordinators with our pid, while a `nself ci` run in another process on the
// same store has its own. The drain touches only the first kind.
func ownedByThisProcess(st *store.Store) func(context.Context, string) bool {
	return func(ctx context.Context, id string) bool {
		cs, err := st.Coordinators(ctx)
		if err != nil {
			return false // cannot tell: leave the lease alone
		}
		for _, c := range cs {
			if c.ID == id {
				return c.PID == os.Getpid()
			}
		}
		return false
	}
}
