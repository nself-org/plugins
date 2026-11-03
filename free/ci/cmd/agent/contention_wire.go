package main

// Purpose: compose the node contention layer (internal/agent/contention, P7-NODE-13) into `serve` and
//          `pull` (D-1226): one Gate over the host slot table admits every lease, a SleepWatcher checked on
//          each heartbeat tick frees and kills the leases of an intermittent node after a sleep, leased jobs
//          run inside contention.Isolator, and a policy delivered in welcome (carrier B) is saved to
//          <state>/contention.json, the file `nodes set` pushes over ssh to a LAN node.
// Inputs:  the agent state dir, the node id, the host slot table, the capacity probe.
// Outputs: session.Contention; the Isolator share for leaseRunner; one stderr line per wake and per
//          priority report that could not apply everything.
// Constraints: the gate reads <state>/contention.json on every lease (contention.SettingsFrom), so a pushed
//          policy holds from the next lease without a restart; a corrupt file reserves every core, never
//          opens the node; the operator's own foreground run never passes through here.

import (
	"github.com/nself-org/plugins/free/ci/internal/agent/contention"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// contentionProbe is the capacity probe of the gate; tests replace it with a fake power source.
var contentionProbe = ciexec.Probe

// nodeContention is the contention layer of one agent process.
type nodeContention struct {
	gate  *contention.Gate
	watch *contention.SleepWatcher
	state string
	logf  func(format string, a ...any)
}

// newNodeContention builds the gate over table. operatorOwned picks the defaults of a node never set.
func newNodeContention(state, nodeID string, operatorOwned bool, table *slots.Table, logf func(string, ...any)) *nodeContention {
	probe := func() ciexec.Capacity { return contentionProbe() }
	battery := reservation.HasBattery(probe())
	return &nodeContention{
		gate:  &contention.Gate{Dir: state, Table: table, Probe: probe, Settings: contention.SettingsFrom(state, nodeID, operatorOwned, battery)},
		watch: &contention.SleepWatcher{},
		state: state,
		logf:  logf,
	}
}

// session is the session's view of the layer.
func (n *nodeContention) session() *session.Contention {
	return &session.Contention{Gate: n.gate, Wake: n.wake, Policy: n.savePolicy}
}

// wake frees the slots of an intermittent node after a sleep and names the attempts to kill.
func (n *nodeContention) wake() []string {
	if !n.watch.Tick() {
		return nil
	}
	freed, err := n.gate.OnWake()
	if err != nil {
		n.logf("after sleep: releasing leases: %v", err)
	}
	if len(freed) > 0 {
		n.logf("after sleep: stopping %d lease(s) the coordinator has expired", len(freed))
	}
	return freed
}

// savePolicy stores a policy the coordinator delivered.
func (n *nodeContention) savePolicy(p protocol.NodePolicy) {
	s := reservation.Settings{NodeID: p.Node, ReservedCPU: p.ReservedCPU, ReservedMemMB: p.ReservedMemMB,
		Interactive: p.Interactive, OnBattery: p.OnBattery, Persistence: p.Persistence, Version: p.Version}
	if err := contention.SavePolicy(n.state, s); err != nil {
		n.logf("contention policy from the coordinator not saved: %v", err)
	}
}

// share is the reservation share a leased job runs inside.
func (n *nodeContention) share() reservation.Share {
	return n.gate.Admit(ciexec.Remote, contention.Need{}, false).Share
}

// isolate wraps the executor's isolator with background priority and the share.
func (n *nodeContention) isolate(inner ciexec.Isolator) ciexec.Isolator {
	return &contention.Isolator{Inner: inner, Share: n.share, Report: func(attempt string, p contention.Priority) {
		if p.Reason != "" {
			n.logf("attempt %s: priority %s, cpu %s, memory %s (%s)", attempt, p.Nice, p.CPU, p.Memory, p.Reason)
		}
	}}
}
