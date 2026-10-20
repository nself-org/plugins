package doctor

// Purpose: the connection checks CI-DN001..CI-DN006: reachability, pinned host key, local OpenSSH client,
// certificate expiry, clock skew and agent/protocol versions.
// Inputs:  Facts.
// Outputs: one result per node per check, or one pass naming why there was nothing to check.
// Constraints: pure; the thresholds are named constants below; clock skew above the 120 s the certificates tolerate
//          is a failure (the node's certificates are refused, E660 clock_skew), above half of it a warning.

import (
	"fmt"
	"slices"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/update"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// CertWarnWindow is how close to expiry a node certificate warns (the plan: under 2 days).
const CertWarnWindow = 48 * time.Hour

// none is the pass for a check that had nothing to look at.
func (f Facts) none(id, what string) []doctor.Check {
	msg := "no " + what
	if f.StoreNote != "" {
		msg += " (" + f.StoreNote + ")"
	}
	return []doctor.Check{result(id, doctor.Pass, "", msg)}
}

func checkReach(f Facts) []doctor.Check {
	var out []doctor.Check
	if f.StoreErr != nil {
		out = append(out, result(IDReach, doctor.Warn, "", "the node registry could not be read: "+f.StoreErr.Error()))
	}
	for _, n := range f.Nodes {
		id := n.ID()
		err, tried := f.Reach[id]
		switch {
		case tried && err != nil:
			out = append(out, result(IDReach, doctor.Fail, id, fmt.Sprintf("node %s is not reachable: %v", id, err)))
		case tried:
			out = append(out, result(IDReach, doctor.Pass, id, fmt.Sprintf("node %s answers", id)))
		case n.State == "offline":
			out = append(out, result(IDReach, doctor.Fail, id, fmt.Sprintf("node %s is offline in the registry", id)))
		default:
			out = append(out, result(IDReach, doctor.Pass, id, fmt.Sprintf("node %s is %s in the registry (no live probe for this transport)", id, n.State)))
		}
	}
	if len(f.Nodes) == 0 && f.StoreErr == nil {
		out = append(out, f.none(IDReach, "nodes registered")...)
	}
	return out
}

func checkHostKey(f Facts) []doctor.Check {
	nodes := f.sshNodes()
	if len(nodes) == 0 {
		return f.none(IDHostKey, "ssh nodes")
	}
	if f.PinsErr != nil {
		return []doctor.Check{result(IDHostKey, doctor.Fail, "", "the pinned host key file could not be read: "+f.PinsErr.Error())}
	}
	var out []doctor.Check
	for _, n := range nodes {
		id, pinned := n.ID(), f.Pins[n.ID()]
		switch {
		case len(pinned) == 0:
			out = append(out, result(IDHostKey, doctor.Fail, id, fmt.Sprintf("node %s has no pinned host key", id)))
		case len(n.Cap.Identity.HostKeyFingerprints) > 0 && !overlaps(pinned, n.Cap.Identity.HostKeyFingerprints):
			out = append(out, withDetail(result(IDHostKey, doctor.Fail, id, fmt.Sprintf("the pinned host key of node %s differs from the registered fingerprint", id)),
				"pinned: "+fmt.Sprint(pinned)+"\nregistered: "+fmt.Sprint(n.Cap.Identity.HostKeyFingerprints)))
		default:
			out = append(out, result(IDHostKey, doctor.Pass, id, fmt.Sprintf("node %s host key is pinned", id)))
		}
	}
	return out
}

func overlaps(a, b []string) bool {
	return slices.ContainsFunc(a, func(x string) bool { return slices.Contains(b, x) })
}

func checkOpenSSH(f Facts) []doctor.Check {
	switch {
	case len(f.sshNodes()) == 0 && f.SSHVersionErr != nil:
		return []doctor.Check{result(IDOpenSSH, doctor.Pass, "", "no ssh nodes registered, so the local OpenSSH client is not needed")}
	case f.SSHVersionErr != nil:
		return []doctor.Check{result(IDOpenSSH, doctor.Fail, "", "cannot determine the local OpenSSH version: "+f.SSHVersionErr.Error())}
	case !f.SSHVersion.AtLeast(8, 0):
		return []doctor.Check{result(IDOpenSSH, doctor.Fail, "", fmt.Sprintf("local OpenSSH %d.%d is older than 8.0", f.SSHVersion.Major, f.SSHVersion.Minor))}
	}
	return []doctor.Check{result(IDOpenSSH, doctor.Pass, "", fmt.Sprintf("local OpenSSH %d.%d", f.SSHVersion.Major, f.SSHVersion.Minor))}
}

func checkCert(f Facts) []doctor.Check {
	nodes := f.agentNodes()
	if len(nodes) == 0 {
		return f.none(IDCert, "agent nodes (certificates apply to pull nodes)")
	}
	if f.CertsErr != nil {
		return []doctor.Check{result(IDCert, doctor.Warn, "", "node certificates could not be read: "+f.CertsErr.Error())}
	}
	var out []doctor.Check
	for _, n := range nodes {
		id := n.ID()
		exp, ok := f.CertExpiry[id]
		left := exp.Sub(f.Now)
		switch {
		case !ok:
			out = append(out, result(IDCert, doctor.Fail, id, fmt.Sprintf("node %s has no certificate on record", id)))
		case left <= 0:
			out = append(out, result(IDCert, doctor.Fail, id, fmt.Sprintf("the certificate of node %s expired %s ago", id, (-left).Round(time.Minute))))
		case left < CertWarnWindow:
			out = append(out, result(IDCert, doctor.Warn, id, fmt.Sprintf("the certificate of node %s expires in %s", id, left.Round(time.Minute))))
		default:
			out = append(out, result(IDCert, doctor.Pass, id, fmt.Sprintf("the certificate of node %s is valid for %s", id, left.Round(time.Hour))))
		}
	}
	return out
}

func checkSkew(f Facts) []doctor.Check {
	if len(f.Nodes) == 0 {
		return f.none(IDSkew, "nodes registered")
	}
	var out []doctor.Check
	for _, n := range f.Nodes {
		id := n.ID()
		d, ok := f.Skew[id]
		if !ok {
			why := "not measured for this transport (agent nodes are checked at join)"
			if err := f.SkewErr[id]; err != nil {
				why = "not measured: " + err.Error()
			}
			out = append(out, result(IDSkew, doctor.Pass, id, fmt.Sprintf("clock of node %s %s", id, why)))
			continue
		}
		abs := d
		if abs < 0 {
			abs = -abs
		}
		msg := fmt.Sprintf("clock of node %s differs by %s", id, d.Round(time.Second))
		switch {
		case abs > pki.ClockSkew:
			out = append(out, result(IDSkew, doctor.Fail, id, msg+" (more than 120 s)"))
		case abs > pki.ClockSkew/2:
			out = append(out, result(IDSkew, doctor.Warn, id, msg))
		default:
			out = append(out, result(IDSkew, doctor.Pass, id, msg))
		}
	}
	return out
}

func checkVersion(f Facts) []doctor.Check {
	if len(f.Nodes) == 0 {
		return f.none(IDVersion, "nodes registered")
	}
	var out []doctor.Check
	for _, n := range f.Nodes {
		id, p := n.ID(), n.Cap.Protocol
		switch {
		case n.Cap.Identity.Transport == "local" || n.Cap.Identity.Provider == "local":
			out = append(out, result(IDVersion, doctor.Pass, id, fmt.Sprintf("node %s is this machine", id)))
		case p.AgentVersion == "" || !verify.ValidVersion(p.AgentVersion):
			out = append(out, result(IDVersion, doctor.Warn, id, fmt.Sprintf("node %s reports no valid agent version", id)))
		case f.MinAgentVersion != "" && update.Compare(p.AgentVersion, f.MinAgentVersion) < 0:
			out = append(out, result(IDVersion, doctor.Fail, id, fmt.Sprintf("agent %s on node %s is older than the minimum %s", p.AgentVersion, id, f.MinAgentVersion)))
		case f.MinProtocol > 0 && !slices.ContainsFunc(p.Versions, func(v int) bool { return v >= f.MinProtocol }):
			out = append(out, result(IDVersion, doctor.Fail, id, fmt.Sprintf("node %s speaks protocol %v; the coordinator needs %d or higher", id, p.Versions, f.MinProtocol)))
		default:
			out = append(out, result(IDVersion, doctor.Pass, id, fmt.Sprintf("agent %s on node %s", p.AgentVersion, id)))
		}
	}
	return out
}
