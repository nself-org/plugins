package selftest

// Purpose: the checks CI-DS006–CI-DS009, the state half of the selftest: the
// watcher, its claims, leftover job roots and infrastructure failures.
// Inputs:  Facts and Limits. Outputs: doctor checks. Constraints: pure.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/claims"
	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

func watcherCheck(f Facts, l Limits) doctor.Check {
	const name = "watcher"
	w := f.Watcher
	if w.Err != "" {
		return unread(IDWatcher, name, "the demand file", w.Err)
	}
	if !w.Present {
		return check(IDWatcher, name, doctor.Pass, "no watcher runs on this host", "")
	}
	iv := w.Interval
	if iv <= 0 {
		iv = DefaultInterval
	}
	if limit := time.Duration(l.WatcherPasses) * iv; w.Age > limit {
		return check(IDWatcher, name, doctor.Fail, fmt.Sprintf("the demand file is %s old, over %d intervals of %s", w.Age.Round(time.Second), l.WatcherPasses, iv),
			"Restart the service: `nself ci serve install --launchd|--systemd`, or look at the watcher log")
	}
	return check(IDWatcher, name, doctor.Pass, fmt.Sprintf("the demand file is %s old", w.Age.Round(time.Second)), "")
}

func claimsCheck(f Facts) doctor.Check {
	const name = "stuck claims"
	c := f.Claims
	if c.Err != "" {
		return unread(IDClaims, name, "the claims of this host", c.Err)
	}
	if !c.Known {
		return check(IDClaims, name, doctor.Pass, "no claim source on this host", "")
	}
	limit := c.RunLimit + claims.DeadGrace
	var stuck []string
	for _, cl := range c.List {
		if cl.Age > limit {
			stuck = append(stuck, cl.Ref)
		}
	}
	if len(stuck) > 0 {
		sort.Strings(stuck)
		return check(IDClaims, name, doctor.Fail, fmt.Sprintf("%d claim(s) older than the run limit plus %s: %s", len(stuck), claims.DeadGrace, strings.Join(stuck, ", ")),
			"Restart the watcher; another host takes the commit once the claim is dead")
	}
	return check(IDClaims, name, doctor.Pass, fmt.Sprintf("%d open claim(s), none stuck", len(c.List)), "")
}

func orphansCheck(f Facts) doctor.Check {
	const name = "leaked temp folders"
	o := f.Orphans
	switch {
	case o.Err != "":
		return unread(IDOrphans, name, "the job roots", o.Err)
	case len(o.List) > 0:
		names := append([]string(nil), o.List...)
		sort.Strings(names)
		if len(names) > 5 {
			names = append(names[:5], "…")
		}
		return check(IDOrphans, name, doctor.Fail, fmt.Sprintf("%d job root(s) no live attempt owns: %s", len(o.List), strings.Join(names, ", ")), "Run `nself ci runs prune --tidy --apply`")
	}
	return check(IDOrphans, name, doctor.Pass, "no leftover job roots", "")
}

func infraCheck(f Facts, l Limits) doctor.Check {
	const name = "infrastructure failures"
	in := f.Infra
	if in.Err != "" {
		return unread(IDInfra, name, "the run history", in.Err)
	}
	msg := fmt.Sprintf("%d of %d run(s) in the last 24 h ended in an infrastructure error", in.Count, in.Total)
	fix := "Run `nself ci doctor` and read the failing runs with `nself ci runs list`"
	switch {
	case in.Count >= l.InfraFail:
		return check(IDInfra, name, doctor.Fail, msg, fix)
	case in.Count >= l.InfraWarn:
		return check(IDInfra, name, doctor.Warn, msg, fix)
	}
	return check(IDInfra, name, doctor.Pass, msg, "")
}
