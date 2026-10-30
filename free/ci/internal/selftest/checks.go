package selftest

// Purpose: the checks CI-DS001–CI-DS005, the host-environment half of the selftest.
// Inputs:  Facts and Limits. Outputs: doctor checks. Constraints: pure.

import (
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

// The check ids of the selftest section (CI-DS…, owned by this package).
const (
	IDTools    = "CI-DS001"
	IDProvider = "CI-DS002"
	IDRepo     = "CI-DS003"
	IDDisk     = "CI-DS004"
	IDSinks    = "CI-DS005"
	IDWatcher  = "CI-DS006"
	IDClaims   = "CI-DS007"
	IDOrphans  = "CI-DS008"
	IDInfra    = "CI-DS009"
)

// Prefix starts the id of every selftest check; the alert looks at these only.
const Prefix = "CI-DS"

// Evaluate returns the nine selftest checks in id order.
func Evaluate(f Facts, l Limits) []doctor.Check {
	out := []doctor.Check{toolsCheck(f), providerCheck(f), repoCheck(f), diskCheck(f, l), sinksCheck(f, l)}
	return append(out, watcherCheck(f, l), claimsCheck(f), orphansCheck(f), infraCheck(f, l))
}

func check(id, name string, st doctor.Status, msg, fix string) doctor.Check {
	if st == doctor.Pass {
		fix = ""
	}
	return doctor.Check{ID: id, Name: name, Status: st, Message: msg, Remediation: fix}
}

func unread(id, name, what, why string) doctor.Check {
	return check(id, name, doctor.Warn, what+" could not be read: "+why, "Fix the cause above, then run `nself ci doctor --selftest` again")
}

func toolsCheck(f Facts) doctor.Check {
	const name = "gate tools"
	var fail, warn []string
	for _, t := range f.Tools {
		switch {
		case t.Found:
		case t.Required:
			fail = append(fail, t.Name)
		default:
			warn = append(warn, t.Name)
		}
	}
	fix := "Install the tool into a directory on the PATH the service runs with"
	switch {
	case len(fail) > 0:
		return check(IDTools, name, doctor.Fail, "missing on the service PATH: "+strings.Join(append(fail, warn...), ", "), fix)
	case len(warn) > 0:
		return check(IDTools, name, doctor.Warn, "optional tool missing on the service PATH: "+strings.Join(warn, ", "), fix)
	}
	return check(IDTools, name, doctor.Pass, fmt.Sprintf("%d gate tool(s) found on the service PATH", len(f.Tools)), "")
}

func providerCheck(f Facts) doctor.Check {
	const name = "provider"
	p := f.Provider
	switch {
	case p.Err != "":
		return unread(IDProvider, name, "the provider state", p.Err)
	case !p.LoggedIn:
		return check(IDProvider, name, doctor.Fail, "the provider CLI is not logged in", "Run `gh auth login` as the user the service runs as")
	case !p.Reachable:
		return check(IDProvider, name, doctor.Fail, "the provider API did not answer", "Check the network and the provider status page")
	}
	return check(IDProvider, name, doctor.Pass, "logged in and the API answers", "")
}

func repoCheck(f Facts) doctor.Check {
	if f.Repo.OK {
		return check(IDRepo, "repository read", doctor.Pass, "the repository is readable", "")
	}
	return check(IDRepo, "repository read", doctor.Fail, "the repository cannot be read: "+f.Repo.Err, "Check the path and its permissions for the user the service runs as")
}

func diskCheck(f Facts, l Limits) doctor.Check {
	const name = "free disk"
	d := f.Disk
	switch {
	case d.Err != "":
		return unread(IDDisk, name, "the free space", d.Err)
	case d.FreeMB < l.MinFreeMB:
		return check(IDDisk, name, doctor.Fail, fmt.Sprintf("%d MB free, below the %d MB floor", d.FreeMB, l.MinFreeMB), "Run `nself ci runs prune --tidy --apply`")
	case d.FreeMB < 2*l.MinFreeMB:
		return check(IDDisk, name, doctor.Warn, fmt.Sprintf("%d MB free, close to the %d MB floor", d.FreeMB, l.MinFreeMB), "Run `nself ci runs prune --tidy` to see what can be freed")
	}
	return check(IDDisk, name, doctor.Pass, fmt.Sprintf("%d MB free", d.FreeMB), "")
}

func sinksCheck(f Facts, l Limits) doctor.Check {
	const name = "notifier delivery"
	if f.Sinks.Err != "" {
		return unread(IDSinks, name, "the delivery counters", f.Sinks.Err)
	}
	var dead, recent []string
	for _, s := range f.Sinks.List {
		switch {
		case s.Failed > 0 && s.Delivered == 0:
			dead = append(dead, s.Name)
		case s.Failed > 0 && f.Now.Sub(s.LastFailedAt) < l.SinkWindow:
			recent = append(recent, s.Name)
		}
	}
	fix := "Check the sink settings (`notify.sinks`) and the receiver"
	switch {
	case len(dead) > 0:
		return check(IDSinks, name, doctor.Fail, "no delivery ever succeeded: "+strings.Join(dead, ", "), fix)
	case len(recent) > 0:
		return check(IDSinks, name, doctor.Warn, "a delivery failed in the last 24 h: "+strings.Join(recent, ", "), fix)
	}
	return check(IDSinks, name, doctor.Pass, fmt.Sprintf("%d sink(s), no recent delivery failure", len(f.Sinks.List)), "")
}
