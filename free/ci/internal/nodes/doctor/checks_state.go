package doctor

// Purpose: the state checks CI-DN007..CI-DN015: toolchain drift, disk and orphan job roots, isolation exposure,
// deploy-host overlap, breaker maintenance, discovery, pending upgrades, persistent GitHub runners on public
// repositories and UID separation under a TEAM coordinator.
// Inputs:  Facts.
// Outputs: one result per node (or per toolchain / per notice) per check, or one pass naming why there was nothing to check.
// Constraints: pure; thresholds are the named constants below. Not covered by data this slice has: a privileged
//          container flag or a docker socket mount (CI-DN009 reads the capability facts: untrusted work, isolation,
//          docker present, user namespace) and a coordinator protocol ceiling (CI-DN013 compares agent versions only).

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/agent/update"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Free disk thresholds in MB: below warn the check warns, below fail it fails.
const (
	DiskWarnMB = 5 * 1024
	DiskFailMB = 1024
)

func hasClass(n Node, c model.TrustClass) bool {
	v := n.Cap.Trust.Accepts.Value
	return v != nil && slices.Contains(*v, c)
}

func boolFact(f model.Fact[bool]) bool { return f.Value != nil && *f.Value }

// majorMinor trims a version to its first two dot fields so a patch difference is not drift.
func majorMinor(v string) string {
	p := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	if len(p) > 2 {
		p = p[:2]
	}
	return strings.Join(p, ".")
}

func checkDrift(f Facts) []doctor.Check {
	if len(f.Nodes) < 2 {
		return []doctor.Check{result(IDDrift, doctor.Pass, "", "fewer than two nodes, so there is nothing to compare")}
	}
	seen := map[string]map[string][]string{} // toolchain -> major.minor -> node ids
	for _, n := range f.Nodes {
		for name, fact := range n.Cap.Tools.Toolchains {
			if fact.Value != nil && *fact.Value != "" {
				if seen[name] == nil {
					seen[name] = map[string][]string{}
				}
				v := majorMinor(*fact.Value)
				seen[name][v] = append(seen[name][v], n.ID())
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []doctor.Check
	for _, name := range names {
		versions := seen[name]
		if len(versions) < 2 {
			continue
		}
		keys := make([]string, 0, len(versions))
		for v := range versions {
			keys = append(keys, v)
		}
		sort.Strings(keys)
		var lines []string
		for _, v := range keys {
			lines = append(lines, fmt.Sprintf("%s: %s", v, strings.Join(versions[v], ", ")))
		}
		out = append(out, withDetail(result(IDDrift, doctor.Warn, name, fmt.Sprintf("toolchain %s differs across nodes (%d versions)", name, len(keys))), strings.Join(lines, "\n")))
	}
	if len(out) == 0 {
		return []doctor.Check{result(IDDrift, doctor.Pass, "", "toolchains agree across nodes")}
	}
	return out
}

func checkDisk(f Facts) []doctor.Check {
	if len(f.Nodes) == 0 {
		return f.none(IDDisk, "nodes registered")
	}
	var out []doctor.Check
	for _, n := range f.Nodes {
		id, free := n.ID(), n.Cap.Resources.DiskFreeMB.Value
		orphans := f.ExpiredLeases[id]
		var problems []string
		st := doctor.Pass
		if free != nil && *free < DiskFailMB {
			st = doctor.Fail
			problems = append(problems, fmt.Sprintf("only %d MB free", *free))
		} else if free != nil && *free < DiskWarnMB {
			st = doctor.Warn
			problems = append(problems, fmt.Sprintf("%d MB free", *free))
		}
		if orphans > 0 {
			if st == doctor.Pass {
				st = doctor.Warn
			}
			problems = append(problems, fmt.Sprintf("%d expired lease(s) not yet fenced (a job root may be orphaned)", orphans))
		}
		switch {
		case len(problems) > 0:
			out = append(out, result(IDDisk, st, id, fmt.Sprintf("node %s: %s", id, strings.Join(problems, "; "))))
		case free == nil:
			out = append(out, result(IDDisk, doctor.Pass, id, fmt.Sprintf("node %s has not reported free disk (no expired leases)", id)))
		default:
			out = append(out, result(IDDisk, doctor.Pass, id, fmt.Sprintf("node %s has %d MB free and no expired leases", id, *free)))
		}
	}
	if f.LeaseErr != nil {
		out = append(out, result(IDDisk, doctor.Warn, "leases", "expired leases could not be read, so orphan job roots are not checked: "+f.LeaseErr.Error()))
	}
	return out
}

func checkExposure(f Facts) []doctor.Check {
	var out []doctor.Check
	for _, n := range f.Nodes {
		id := n.ID()
		if !hasClass(n, model.TrustClass("untrusted")) {
			continue
		}
		iso := ""
		if v := n.Cap.Trust.Isolation.Value; v != nil {
			iso = string(*v)
		}
		docker := boolFact(n.Cap.Tools.Docker)
		switch {
		case iso == "" || iso == "process":
			out = append(out, result(IDExposure, doctor.Fail, id, fmt.Sprintf("node %s takes untrusted work with no container isolation", id)))
		case docker && iso == "container" && !boolFact(n.Cap.Separation.UserNS):
			out = append(out, result(IDExposure, doctor.Fail, id, fmt.Sprintf("node %s takes untrusted work in plain containers with docker present and no user namespace", id)))
		default:
			out = append(out, result(IDExposure, doctor.Pass, id, fmt.Sprintf("node %s isolates untrusted work (%s)", id, iso)))
		}
	}
	if len(out) == 0 {
		return []doctor.Check{result(IDExposure, doctor.Pass, "", "no node accepts untrusted work")}
	}
	return out
}

func checkDeploy(f Facts) []doctor.Check {
	var out []doctor.Check
	for _, n := range f.Nodes {
		id, d := n.ID(), n.Cap.DeployHost
		if !d.Matched {
			continue
		}
		src := "the deploy inventory"
		if d.Source != nil {
			src = *d.Source
		}
		switch {
		case d.Override:
			out = append(out, result(IDDeploy, doctor.Warn, id, fmt.Sprintf("node %s is a deploy host (%s) used through an explicit override", id, src)))
		case n.Cap.Lifecycle.Eligible || len(n.Cap.Lifecycle.AuthorizedProjects) > 0:
			out = append(out, result(IDDeploy, doctor.Fail, id, fmt.Sprintf("node %s is a deploy host (%s) and still authorized for CI work", id, src)))
		}
	}
	for _, w := range f.ExclusionWarnings {
		out = append(out, result(IDDeploy, doctor.Warn, "exclusion", w))
	}
	if len(out) == 0 {
		return []doctor.Check{result(IDDeploy, doctor.Pass, "", "no node overlaps a deploy host and the exclusion refresh is current")}
	}
	return out
}

func checkBreaker(f Facts) []doctor.Check {
	var out []doctor.Check
	for _, n := range f.Nodes {
		if n.State == "maintenance" {
			why := n.Reason
			if why == "" {
				why = "no reason recorded"
			}
			out = append(out, result(IDBreaker, doctor.Warn, n.ID(), fmt.Sprintf("node %s is in maintenance: %s", n.ID(), why)))
		}
	}
	if len(out) == 0 {
		return []doctor.Check{result(IDBreaker, doctor.Pass, "", "no node is held in maintenance")}
	}
	return out
}

func checkDiscovery(f Facts) []doctor.Check {
	switch {
	case f.DiscoveryErr != nil:
		return []doctor.Check{result(IDDiscovery, doctor.Warn, "", "the discovery switch is unreadable, so discovery stays off: "+f.DiscoveryErr.Error())}
	case f.DiscoveryOff:
		return []doctor.Check{result(IDDiscovery, doctor.Pass, "", "LAN discovery is off")}
	}
	return []doctor.Check{result(IDDiscovery, doctor.Pass, "", "LAN discovery is on")}
}

func checkUpgrade(f Facts) []doctor.Check {
	if f.PinnedRelease == "" {
		return []doctor.Check{result(IDUpgrade, doctor.Pass, "", "no agent release is pinned in this build, so no upgrade can be pending")}
	}
	var out []doctor.Check
	for _, n := range f.Nodes {
		v := n.Cap.Protocol.AgentVersion
		if n.Cap.Identity.Transport == "local" || !verify.ValidVersion(v) {
			continue
		}
		if update.Compare(v, f.PinnedRelease) < 0 {
			out = append(out, result(IDUpgrade, doctor.Warn, n.ID(), fmt.Sprintf("agent %s on node %s is older than the pinned release %s; the upgrade is pending and a coordinator ceiling (E666) can hold it", v, n.ID(), f.PinnedRelease)))
		}
	}
	if len(out) == 0 {
		return []doctor.Check{result(IDUpgrade, doctor.Pass, "", "no agent upgrade is pending")}
	}
	return out
}

func checkGitHub(f Facts) []doctor.Check {
	var out []doctor.Check
	for _, n := range f.Nodes {
		id := n.ID()
		if !strings.HasPrefix(n.Cap.Identity.Provider, "github") || !boolFact(n.Cap.Availability.Persistence) {
			continue
		}
		switch f.RepoVisibility {
		case "public":
			out = append(out, result(IDGitHub, doctor.Fail, id, fmt.Sprintf("persistent GitHub runner %s and the repository doctor ran in is public, where anyone can submit workflow code", id)))
		case "":
			out = append(out, result(IDGitHub, doctor.Warn, id, fmt.Sprintf("persistent GitHub runner %s; the visibility of the repository doctor ran in could not be read", id)))
		default:
			out = append(out, result(IDGitHub, doctor.Pass, id, fmt.Sprintf("persistent GitHub runner %s; the repository doctor ran in is %s", id, f.RepoVisibility)))
		}
	}
	if len(out) == 0 {
		return []doctor.Check{result(IDGitHub, doctor.Pass, "", "no persistent GitHub runner is registered")}
	}
	return out
}

func checkSeparate(f Facts) []doctor.Check {
	if f.ModeErr != nil {
		return []doctor.Check{result(IDSeparate, doctor.Warn, "", "the coordinator mode could not be read, so TEAM offers are not checked: "+f.ModeErr.Error())}
	}
	if !f.TeamMode {
		return []doctor.Check{result(IDSeparate, doctor.Pass, "", "the coordinator is not in TEAM mode")}
	}
	var out []doctor.Check
	for _, n := range f.Nodes {
		offered := n.Cap.Lifecycle.Eligible || len(n.Cap.Lifecycle.AuthorizedProjects) > 0
		if offered && !boolFact(n.Cap.Separation.UIDSeparation) {
			out = append(out, result(IDSeparate, doctor.Fail, n.ID(), fmt.Sprintf("node %s has no UID separation and is offered to a TEAM coordinator", n.ID())))
		}
	}
	if len(out) == 0 {
		return []doctor.Check{result(IDSeparate, doctor.Pass, "", "every node offered to the TEAM coordinator separates jobs by UID")}
	}
	return out
}
