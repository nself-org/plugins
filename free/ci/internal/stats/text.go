package stats

// Purpose: the human table `nself ci runs stats` prints.
// Inputs:  a Doc.
// Outputs: text, one line per block; a null figure prints "-" and its reason is listed at the end.
// Constraints: pure; keys are sorted so the output is stable.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func dur(ms int64) string {
	return (time.Duration(ms) * time.Millisecond).Round(time.Millisecond).String()
}

func pctText(p *Pct) string {
	if p == nil {
		return "-"
	}
	return fmt.Sprintf("p50 %s p95 %s (%d samples)", dur(p.P50), dur(p.P95), p.N)
}

func rate(v int) string { return fmt.Sprintf("%.1f%%", float64(v)/10) }

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Text renders d for a terminal.
func Text(d Doc) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
	w("window %s", dur(d.SinceMs))
	if d.Project != nil {
		w(", project %s", *d.Project)
	}
	w("\n")
	for _, k := range sortedKeys(d.Queue) {
		q := d.Queue[k]
		w("queue %-11s depth %d, wait %s\n", k, q.Depth, pctText(q.WaitMs))
	}
	if a := d.Attempts; a != nil {
		w("attempts %d: pass %s, fail %s, flaky %s, infra retry %s\n", a.Finished, rate(a.PassPermille), rate(a.FailPermille), rate(a.FlakyPermille), rate(a.InfraRetryPermille))
	}
	for _, k := range sortedKeys(d.Runners) {
		r, u := d.Runners[k], "-"
		if r.UtilisationPermille != nil {
			u = rate(*r.UtilisationPermille)
		}
		w("runner %-20s attempts %d, busy %s, utilisation %s\n", k, r.Attempts, dur(r.BusyMs), u)
	}
	for _, k := range sortedKeys(d.Jobs) {
		p := d.Jobs[k]
		w("job %-24s %s\n", k, pctText(&p))
	}
	if c := d.Cache; c != nil {
		h := "-"
		if c.HitPermille != nil {
			h = rate(*c.HitPermille)
		}
		w("cache hit %s, restored %d B, saved %d B\n", h, c.BytesRestored, c.BytesSaved)
	}
	w("provisioning %s\n", pctText(d.Provisioning))
	for _, k := range sortedKeys(d.Spend) {
		w("spend %-10s %d micro over %d attempts\n", k, d.Spend[k].Micro, d.Spend[k].Attempts)
	}
	if l := d.Leases; l != nil {
		w("leases active %d, stale %d, self-fenced %d\n", l.Active, l.Stale, l.SelfFenced)
	}
	if o := d.Outbox; o != nil {
		age := "-"
		if o.OldestAgeMs != nil {
			age = dur(*o.OldestAgeMs)
		}
		w("outbox pending %d, oldest %s\n", o.Pending, age)
	}
	reasons := make([]string, 0, len(d.Reasons))
	for k, v := range d.Reasons {
		reasons = append(reasons, "unavailable "+k+": "+v)
	}
	sort.Strings(reasons)
	for _, r := range reasons {
		w("%s\n", r)
	}
	return b.String()
}
