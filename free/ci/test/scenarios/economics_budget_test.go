//go:build scenarios

package scenarios

// Purpose: Epic acceptance 5, budgets. (a) a placement whose worst-case cost does not fit a budget is
// ineligible with budget.exceeded at job, pipeline, day, month and provider scope, both in the
// scheduler (sched.Place) and at admission (dispatch.Admit with the budget step); (b) concurrent
// submissions racing for a small day or month budget never commit more than the budget, because the
// reservation is made inside the admission transaction.
// Inputs:  a 60 s job on a 3.6M micro/hour provider (60 000 micro), a seeded generator for the race.
// Outputs: assertions only.
// Constraints: run with -race; the seed is printed on failure (NSELF_CI_INVARIANT_SEED).

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const econJobMicro = 60_000 // a 60 s job at 3.6M micro per hour, one-minute increments

// TestEconomicsBudgetExceededIsIneligibleAtEveryScope: one micro short is ineligible, exact fit places.
func TestEconomicsBudgetExceededIsIneligibleAtEveryScope(t *testing.T) {
	scopes := []struct {
		name, key string
		set       func(*sched.Budgets, int64)
	}{
		{"job", schedpolicy.KeyBudgetJob, func(b *sched.Budgets, v int64) { b.Job = sched.Budget{Set: true, Remaining: v} }},
		{"pipeline", schedpolicy.KeyBudgetPipeline, func(b *sched.Budgets, v int64) { b.Pipeline = sched.Budget{Set: true, Remaining: v} }},
		{"day", schedpolicy.KeyBudgetDay, func(b *sched.Budgets, v int64) { b.Day = sched.Budget{Set: true, Remaining: v} }},
		{"month", schedpolicy.KeyBudgetMonth, func(b *sched.Budgets, v int64) { b.Month = sched.Budget{Set: true, Remaining: v} }},
		{"provider", "economics.budgets.provider.cloudco", func(b *sched.Budgets, v int64) {
			b.Providers = map[string]sched.Budget{"cloudco": {Set: true, Remaining: v}}
		}},
	}
	for _, sc := range scopes {
		t.Run(sc.name, func(t *testing.T) {
			// the scheduler: the world's remaining budget decides eligibility
			p := econPolicy(t, trust.SourceProject, econKV(nil), nil)
			for limit, want := range map[int64]bool{econJobMicro - 1: false, econJobMicro: true} {
				w := econPlaceWorld(p, econPaid("paid", "metered", "cloudco"))
				sc.set(&w.Budgets, limit)
				j := job("j", "linux/amd64")
				j.TimeoutMs = 60_000
				res := sched.Place(w, []sched.Job{j})
				if got := len(res.Placements) == 1; got != want {
					t.Fatalf("%s budget %d: placed=%v want %v: %+v", sc.name, limit, got, want, res.Explanations)
				}
				if !want && !hasCode(res.Explanations[0].Alternatives[0].Reasons, string(model.BudgetExceeded)) {
					t.Fatalf("%s budget %d: reasons %v, want budget.exceeded", sc.name, limit, codes(res.Explanations[0].Alternatives[0].Reasons))
				}
			}
			// admission: the same limits through the real budget step and store
			for limit, want := range map[int64]string{econJobMicro - 1: string(model.BudgetExceeded), econJobMicro: ""} {
				clk := newClock()
				st := econStore(t, clk)
				env := econEnv(st, econPolicy(t, trust.SourceProject, econKV(map[string]string{sc.key: fmt.Sprint(limit)}), nil), clk)
				a := queueAttempt(t, st, "pipe", "project", "build")
				if got := econRefusal(t, econAdmit(env, st, econRequest(a, "", "cloudco", 60_000))); got != want {
					t.Fatalf("%s budget %d at admission: refusal %q, want %q", sc.name, limit, got, want)
				}
			}
		})
	}
}

// TestEconomicsConcurrentSubmissionsNeverExceedTheBudget races many pipelines for a small day (or
// month) budget. The ledger's reservations must total at most the budget, every admitted attempt's
// estimate must be in them, and the budget must actually bind (some are refused budget.exceeded).
func TestEconomicsConcurrentSubmissionsNeverExceedTheBudget(t *testing.T) {
	seed := invariants.Seed()
	for round := 0; round < 6; round++ {
		key, window := schedpolicy.KeyBudgetDay, "day"
		if round%2 == 1 {
			key, window = schedpolicy.KeyBudgetMonth, "month"
		}
		r := rand.New(rand.NewSource(seed + int64(round)))
		budget := int64(3+r.Intn(5))*econJobMicro + int64(r.Intn(econJobMicro)) // room for 3 to 7 minimum jobs
		clk := newClock()
		st := econStore(t, clk)
		other := schedpolicy.KeyBudgetMonth
		if window == "month" {
			other = schedpolicy.KeyBudgetDay
		}
		env := econEnv(st, econPolicy(t, trust.SourceProject, econKV(map[string]string{key: fmt.Sprint(budget), other: "900000000"}), nil), clk)
		const attempts, pipelines = 36, 12
		reqs := make([]int64, attempts)
		ids := make([]string, attempts)
		for i := range ids {
			reqs[i] = int64(1+r.Intn(3)) * 60_000
			ids[i] = queueAttempt(t, st, fmt.Sprintf("pipe-%d", r.Intn(pipelines)), "project", "build")
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		admitted, refused, unexpected := 0, 0, []string{}
		start := make(chan struct{})
		for i := range ids {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				err := econAdmit(env, st, econRequest(ids[i], "", "cloudco", reqs[i]))
				mu.Lock()
				defer mu.Unlock()
				switch reason := econRefusalOf(err); {
				case err == nil:
					admitted++
				case reason == string(model.BudgetExceeded):
					refused++
				default:
					unexpected = append(unexpected, fmt.Sprint(err))
				}
			}()
		}
		close(start)
		wg.Wait()
		day := clk.Now().UTC().Format("2006-01-02")
		held, err := st.Reserved(ctxT(t), window, "", map[string]string{"day": day, "month": day[:7]}[window])
		if err != nil {
			t.Fatal(err)
		}
		var sum int64
		rows := 0
		for _, id := range ids {
			if row, found, _ := st.LedgerAttempt(ctxT(t), id); found && row.EstMicro > 0 {
				sum += row.EstMicro
				rows++
			}
		}
		if rows != admitted { // every admitted attempt holds exactly one reservation row
			t.Fatalf("seed %d round %d: %d ledger rows for %d admitted attempts", seed, round, rows, admitted)
		}
		if held > budget || sum != held || admitted == 0 || refused == 0 || len(unexpected) > 0 || admitted+refused != attempts {
			t.Fatalf("seed %d round %d (%s budget %d): reserved %d, ledger estimates %d, admitted %d, refused %d, other %v",
				seed, round, window, budget, held, sum, admitted, refused, unexpected)
		}
	}
}
