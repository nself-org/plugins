package main

// Purpose: the isolation-availability section of `nself ci doctor` (D-1354, D-1063). One check per registered
// isolation profile saying whether the level can run on this machine now, and if not why: the profile's reason,
// its install or fix hint, and its facts (for Tart the licence note, version, free VM slots, running VMs, base image
// and softnet block option). It sits beside the isolation last-verified section (doctor_isolation.go), which says
// when a level was last proved.
// Inputs:  the profiles in isolation.Default, each probed live (Probe is bounded and cached by the profile).
// Outputs: checks isolation.<level>.availability that always pass: an unavailable level is the honest state of a
//          host without that runtime, not a fault. The hint is in Detail (always printed) and in Remediation.
// Constraints: probes only; nothing is started or written beyond what a profile's own Probe does.

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

// isolationProfiles lists the profiles to probe; tests replace it.
var isolationProfiles = func() []isolation.Profile { return isolation.Default.Profiles() }

func init() {
	doctor.RegisterSection(doctor.Section{Name: "isolation-availability", Selector: doctor.SelectAlways, Run: isolationAvailability})
}

func isolationAvailability(ctx context.Context, _ doctor.Env) []doctor.Check {
	profiles := isolationProfiles()
	avs := make([]isolation.Availability, len(profiles))
	var wg sync.WaitGroup
	for i, p := range profiles {
		wg.Add(1)
		go func() {
			defer wg.Done()
			avs[i] = p.Probe(ctx)
		}()
	}
	wg.Wait()
	out := make([]doctor.Check, 0, len(profiles))
	for i, p := range profiles {
		out = append(out, availabilityCheck(string(p.Level()), avs[i]))
	}
	return out
}

func availabilityCheck(level string, av isolation.Availability) doctor.Check {
	c := doctor.Check{
		ID: "isolation." + level + ".availability", Name: "isolation " + level + " availability",
		Status: doctor.Pass, Subject: level,
	}
	hint := av.Facts["hint"]
	if av.Available() {
		c.Message = "available (" + string(av.Level) + ")"
	} else {
		c.Message = "unavailable"
		if av.Reason != "" {
			c.Message += ": " + av.Reason
		}
		c.Remediation = hint
	}
	var lines []string
	if hint != "" {
		lines = append(lines, hint)
	}
	keys := make([]string, 0, len(av.Facts))
	for k := range av.Facts {
		if k != "hint" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		lines = append(lines, k+"="+av.Facts[k])
	}
	c.Detail = strings.Join(lines, "\n")
	return c
}
