package main

// Purpose: `nself ci runs queue [--json]` (P7-SCHED-06, Epic D9): the jobs
// waiting for a placement, in the order the dispatcher would place them, with
// each job's position, band, project and expected wait.
// Inputs:  argv after `queue`; env NSELF_CI_HOME.
// Outputs: a table or one v1 JSON envelope; exit 0, 1 (usage) or 2 (infrastructure).
// Constraints: V15 only; read only; no store on this machine is an empty queue.
// The order is the fair-queuing order (internal/fairq) of the queued attempts
// the store holds, computed with a fresh virtual clock, so it approximates what
// a running dispatcher does; that dispatcher keeps its own virtual clock, the
// effective policy, each job's priority and its real enqueue time. The store
// keeps none of those per attempt, so this view: derives the band from the run's
// trigger (no foreground flag), treats every job as normal priority, orders by the
// age of the run and never shows starvation promotion (run_age_ms is the age of
// the run, not of the wait). Expected wait is the jobs ahead x mean runtime /
// slots from the settled estimators, as in E694 (which counts the project's own
// depth ahead; this view counts every queued job ahead).

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/fairq"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const queueUsage = "nself ci runs queue [--json]"

func init() { registerRunsSub("queue", runQueue) }

// queueView is one row of the output.
type queueView struct {
	Position       int    `json:"position"`
	Band           string `json:"band"`
	Project        string `json:"project"`
	Run            string `json:"run"`
	Job            string `json:"job"`
	Attempt        string `json:"attempt"`
	RunAgeMs       int64  `json:"run_age_ms"`
	ExpectedWaitMs int64  `json:"expected_wait_ms"`
	WaitConfidence string `json:"wait_confidence"`
}

func runQueue(args []string) int {
	var asJSON bool
	for _, a := range args {
		if a != "--json" && a != "-json" && a != "--json=true" {
			return usageErr(queueUsage)
		}
		asJSON = true
	}
	ctx := context.Background()
	st, err := openHistory(false)
	if err != nil {
		return exitForError(err)
	}
	list := []queueView{}
	if st != nil {
		defer func() { _ = st.Close() }()
		if list, err = queueRows(ctx, st, time.Now(), ciexec.Probe().CPUs); err != nil {
			return exitForError(err)
		}
	}
	if asJSON {
		if err = output.WriteData(os.Stdout, "ci runs queue", list, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	if len(list) == 0 {
		fmt.Println("no jobs are waiting for a placement")
		return 0
	}
	for _, q := range list {
		fmt.Printf("%3d  %-11s %-20s %-30s run age %s, expected wait %s (%s)\n", q.Position, q.Band, q.Project, q.Run+"/"+q.Job, ms(q.RunAgeMs), ms(q.ExpectedWaitMs), q.WaitConfidence)
	}
	return 0
}

func ms(v int64) string { return (time.Duration(v) * time.Millisecond).Round(time.Second).String() }

// queueRows lists the queued attempts of st in fair-queue order at now.
func queueRows(ctx context.Context, st *store.Store, now time.Time, slots int) ([]queueView, error) {
	queued, err := st.TelQueued(ctx)
	if err != nil {
		return nil, err
	}
	ages, err := st.PipelineAges(ctx)
	if err != nil {
		return nil, err
	}
	created := map[string]int64{}
	for _, a := range ages {
		created[a.ID] = a.CreatedAt / int64(time.Millisecond)
	}
	byKey := map[string]store.TelAttempt{}
	items := make([]fairq.Item, len(queued))
	for i, a := range queued {
		byKey[a.ID] = a
		items[i] = fairq.Item{Key: a.ID, Pipeline: a.Run, Job: a.Job, Project: a.Project, Band: fairq.For("", a.Trigger, false), Priority: "normal", EnqueuedMs: created[a.Run], Seq: uint64(i), Cost: 1}
	}
	ranked, _ := fairq.Order(fairq.State{}, items, now.UnixMilli(), fairq.Config{})
	out := make([]queueView, len(ranked))
	type est struct {
		ms   int64
		conf string
	}
	runtimes := map[string]est{} // one estimator read per project, not per row
	for i, r := range ranked {
		a := byKey[r.Key]
		e, ok := runtimes[a.Project]
		if !ok {
			e.ms, e.conf = dispatch.RuntimeEstimate(ctx, st, a.Project)
			runtimes[a.Project] = e
		}
		wait, conf := dispatch.WaitFor(int64(i), e.ms, slots), e.conf
		out[i] = queueView{Position: i + 1, Band: r.Effective.String(), Project: a.Project, Run: a.Run, Job: a.JobName, Attempt: a.ID,
			RunAgeMs: max(0, now.UnixMilli()-r.EnqueuedMs), ExpectedWaitMs: wait, WaitConfidence: conf}
	}
	return out, nil
}
