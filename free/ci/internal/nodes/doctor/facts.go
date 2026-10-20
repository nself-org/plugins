package doctor

// Purpose: the facts the node checks read. A check is a pure function of Facts, so every check has a fixture and no
// check touches the network, the store or a process itself (Gather does, behind injectable seams).
// Inputs:  filled by Gather (gather.go) or by a test.
// Outputs: Facts, Node and the result helper every check uses.
// Constraints: a map entry that is absent means "not measured", which a check reports as a pass saying so (never as
//          a clean bill); only a measured bad value warns or fails.

import (
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Node is a registered node as the checks see it.
type Node struct {
	Cap    model.Capability
	State  string // the registry state: online, offline, draining, maintenance, revoked
	Reason string // why the registry holds that state
}

// ID is the node id.
func (n Node) ID() string { return n.Cap.Identity.ID }

// Facts is everything one run of the section knows.
type Facts struct {
	Now time.Time

	// Nodes are the registered, non-revoked nodes in id order.
	Nodes []Node
	// StoreNote says why the registry could not be listed ("" when it was read; "no state database" when none exists yet).
	StoreNote string
	StoreErr  error

	SSHVersion    remote.Version
	SSHVersionErr error

	Reach      map[string]error         // node id to the reachability error; nil error = reachable
	Pins       map[string][]string      // node id to the pinned host key fingerprints
	PinsErr    error                    // the pin file could not be read
	CertExpiry map[string]time.Time     // node id to the latest node certificate expiry
	CertsErr   error                    // certificates could not be read
	Skew       map[string]time.Duration // node id to remote minus local clock
	SkewErr    map[string]error         // node id to why no reading was possible

	ExpiredLeases map[string]int // node id to leases past their deadline whose job root is not reclaimed

	ExclusionWarnings []string // deploy-host refresh failures (deployguard)
	DiscoveryOff      bool
	DiscoveryErr      error  // the discovery switch is unreadable or invalid (fails closed to off)
	TeamMode          bool   // the coordinator mode row says team
	RepoVisibility    string // "public", "private", "internal" or "" (unknown) for the repository doctor ran in

	// Limits from the binary.
	MinAgentVersion string
	MinProtocol     int
	PinnedRelease   string // the agent release this build installs by default ("" when none is pinned)
}

// result builds a check result with its catalog name and remediation.
func result(id string, st doctor.Status, subject, msg string) doctor.Check {
	e, _ := Info(id)
	return doctor.Check{ID: id, Name: e.Name, Status: st, Message: msg, Remediation: e.Remediation, Subject: subject}
}

// withDetail adds the detail lines to a result.
func withDetail(c doctor.Check, detail string) doctor.Check {
	c.Detail = detail
	return c
}

// sshNodes are the nodes reached over ssh.
func (f Facts) sshNodes() []Node {
	var out []Node
	for _, n := range f.Nodes {
		if n.Cap.Identity.Transport == "ssh" {
			out = append(out, n)
		}
	}
	return out
}

// agentNodes are the nodes that pull over mTLS (transport agent).
func (f Facts) agentNodes() []Node {
	var out []Node
	for _, n := range f.Nodes {
		if n.Cap.Identity.Transport == "agent" {
			out = append(out, n)
		}
	}
	return out
}
