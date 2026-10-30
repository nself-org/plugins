package stats

// Purpose: the `health` section of `nself ci doctor` (P7-SCHED-20, ids CI-STAT-01..05): warnings from the same
// numbers `nself ci runs stats` prints, over the last 24 hours of the local run store.
// Inputs:  the doctor Env (NSELF_CI_HOME); the run store file, read only.
// Outputs: one check per id, pass or warn.
// Constraints: registered from init through the doctor Section registry; never a failure; a machine with no
//          run store passes every check; a source that is null in the document passes its check with the
//          reason, since nothing can be said about it.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Doctor check ids and the thresholds that trigger them.
const (
	IDStaleLeases = "CI-STAT-01"
	IDQueueWait   = "CI-STAT-02"
	IDOutbox      = "CI-STAT-03"
	IDNodes       = "CI-STAT-04"
	IDStoreSize   = "CI-STAT-05"

	QueueWaitWarnMs  = int64(10 * 60 * 1000)
	OutboxAgeWarnMs  = int64(60 * 60 * 1000)
	OutboxCountWarn  = 50
	StoreDiskPercent = 20
)

func init() {
	doctor.RegisterSection(doctor.Section{Name: "health", Selector: doctor.SelectAlways, Run: healthSection})
}

func healthSection(ctx context.Context, env doctor.Env) []doctor.Check {
	path := filepath.Join(firstNonEmpty(env.Getenv("NSELF_CI_HOME"), homeDefault(env)), "state.db")
	if _, err := os.Stat(path); err != nil {
		return HealthChecks(nil)
	}
	st, err := store.Open(path, store.Options{})
	if err != nil {
		return []doctor.Check{{ID: IDStaleLeases, Name: "ci health", Status: doctor.Warn, Message: "the run store could not be read: " + err.Error(),
			Remediation: "Run: nself ci doctor, and fix the store check CI-D004 first"}}
	}
	defer func() { _ = st.Close() }()
	d, err := Compute(ctx, st, Options{Now: time.Now(), Since: DefaultSince, Global: true})
	if err != nil {
		return []doctor.Check{{ID: IDStaleLeases, Name: "ci health", Status: doctor.Warn, Message: "the health numbers could not be read: " + err.Error(),
			Remediation: "Run: nself ci runs stats, and fix the store error it reports"}}
	}
	return HealthChecks(&d)
}

func homeDefault(env doctor.Env) string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".nself", "ci")
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// HealthChecks turns a stats document into the five checks; nil means there is no run store.
func HealthChecks(d *Doc) []doctor.Check {
	chk := func(id, name string, warn bool, ok, bad, fix string) doctor.Check {
		c := doctor.Check{ID: id, Name: name, Status: doctor.Pass, Message: ok, Remediation: "None needed"}
		if warn {
			c.Status, c.Message, c.Remediation = doctor.Warn, bad, fix
		}
		return c
	}
	if d == nil {
		none := "no run store on this machine yet"
		return []doctor.Check{chk(IDStaleLeases, "no stale leases", false, none, "", ""), chk(IDQueueWait, "queue wait p95 under 10 minutes", false, none, "", ""),
			chk(IDOutbox, "status outbox drained", false, none, "", ""), chk(IDNodes, "no node offline for 24 hours", false, none, "", ""),
			chk(IDStoreSize, "run store small against free disk", false, none, "", "")}
	}
	var out []doctor.Check
	stale := 0
	if d.Leases != nil {
		stale = d.Leases.Stale
	}
	out = append(out, chk(IDStaleLeases, "no stale leases", stale > 0, "no lease has expired under a running job",
		fmt.Sprintf("%d lease(s) expired under a running job", stale), "Restart the agent on the node that holds it, or run: nself ci nodes show <node>"))
	var worst int64 = -1
	for _, b := range d.Queue {
		if b.WaitMs != nil {
			worst = max(worst, b.WaitMs.P95)
		}
	}
	out = append(out, chk(IDQueueWait, "queue wait p95 under 10 minutes", worst > QueueWaitWarnMs, "queue wait is within 10 minutes",
		fmt.Sprintf("queue wait p95 is %s", (time.Duration(worst)*time.Millisecond).Round(time.Second)), "Add capacity: nself ci nodes add, or lower what runs at once; see: nself ci runs queue"))
	old, pending := int64(0), int64(0)
	if d.Outbox != nil {
		pending = d.Outbox.Pending
		if d.Outbox.OldestAgeMs != nil {
			old = *d.Outbox.OldestAgeMs
		}
	}
	out = append(out, chk(IDOutbox, "status outbox drained", old > OutboxAgeWarnMs || pending > OutboxCountWarn, "the status outbox is draining",
		fmt.Sprintf("%d status report(s) waiting, the oldest %s", pending, (time.Duration(old)*time.Millisecond).Round(time.Minute)), "Check the reporter credentials and network; see: nself ci runs outbox"))
	off := 0
	if d.Nodes != nil {
		off = d.Nodes.OfflineOver24h
	}
	out = append(out, chk(IDNodes, "no node offline for 24 hours", off > 0, "no node has been offline for 24 hours",
		fmt.Sprintf("%d node(s) offline for more than 24 hours", off), "Bring the node back or remove it: nself ci nodes remove <node>"))
	big := d.Store != nil && d.Store.FreeBytes != nil && d.Store.SizeBytes*100 > *d.Store.FreeBytes*StoreDiskPercent
	out = append(out, chk(IDStoreSize, "run store small against free disk", big, "the run store is under 20% of free disk",
		"the run store is over 20% of the free disk", "Free disk space or prune old runs: nself ci runs prune"))
	return out
}
