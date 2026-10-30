package stats

// Purpose: the fixture store of the stats tests (P7-SCHED-20): two projects, three runners, retries, flakes,
// cache hits and misses, two ephemeral cold starts, ledger rows of every confidence, a stale lease and a
// two-hour-old outbox entry; buildFixture(t, 400) holds exactly 500 attempts, buildFixture(t, 80000) 100 000.
// Inputs:  the number of jobs.
// Outputs: an open store and the fixed clock the rows were written against.
// Constraints: rows are written through a second connection in one transaction (the store has no seam for
//          core rows); the values are formulas of the index, so the golden documents never change by chance.

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

var fixtureNow = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

type fx struct {
	t  *testing.T
	tx *sql.Tx
}

func (f fx) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.tx.Exec(q, args...); err != nil {
		f.t.Fatalf("%v\n%s", err, q)
	}
}

func buildFixture(t *testing.T, jobs int) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	f := fx{t, tx}
	f.attempts(jobs)
	f.platform()
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return st
}

func (f fx) attempt(id, job, pipe string, n int, state string) {
	f.exec(`INSERT INTO attempt(id,job_id,n,state) VALUES(?,?,?,?)`, id, job, n, state)
}

func (f fx) outcome(id, project, runner, class, result, failure string, i, cold int, ts time.Time) {
	f.exec(`INSERT INTO schedtel_outcome(attempt_id,project,job_key,runner,runner_class,wait_ms,runtime_ms,cold_start_ms,cost_micro,result,failure_class,ts) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, project, fmt.Sprintf("job%d", i%4), runner, class, 100+(i*37)%900, 1000+(i*53)%5000, cold, 0, result, failure, ts.UnixMilli())
}

// attempts writes jobs jobs: the first eighth fail once then pass (flaky), the next eighth fail once on
// infrastructure then pass, a seventeenth of the rest fail, the rest pass.
func (f fx) attempts(jobs int) {
	runners := []string{"mac1", "linux1", "eph1"}
	triggers := []string{"push", "push", "schedule", "manual", "push"}
	i := 0
	for k := 0; k < jobs; k++ {
		project := []string{"acme/a", "acme/b"}[k%2]
		pipe, job := fmt.Sprintf("p%d", k), fmt.Sprintf("j%d", k)
		f.exec(`INSERT INTO pipeline(id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			pipe, project, "rev", triggers[k%5], "owner", "z", "d", "i", "m", "r", "s", "passed", fixtureNow.UnixNano())
		f.exec(`INSERT INTO job(id,pipeline_id,name,kind,idempotent,infra_max,input_digest,required,status) VALUES(?,?,?,?,?,?,?,?,?)`, job, pipe, "build", "k", 1, 1, "i", 1, "passed")
		var results [][2]string
		switch {
		case k < jobs/8:
			results = [][2]string{{"failed", "test"}, {"passed", ""}}
		case k < jobs/4:
			results = [][2]string{{"failed", "infra"}, {"passed", ""}}
		case k%17 == 0:
			results = [][2]string{{"failed", "test"}}
		default:
			results = [][2]string{{"passed", ""}}
		}
		for n, r := range results {
			id := fmt.Sprintf("a%d-%d", k, n+1)
			f.attempt(id, job, pipe, n+1, r[0])
			runner, class, cold := runners[i%3], "owned", -1
			if i == 7 || i == 8 { // the two ephemeral intents
				runner, class, cold = "eph1", "ephemeral", 21000+3000*(i-7)
			}
			f.outcome(id, project, runner, class, r[0], r[1], i, cold, fixtureNow.Add(-time.Duration(i%1000)*time.Minute))
			if i%5 == 0 { // cache use on every fifth attempt
				f.exec(`INSERT INTO cache_attempt(attempt_id,project,trust,protection,secrets_declared,private_deps,recorded_at) VALUES(?,?,?,?,?,?,?)`, id, project, "owner", "none", 0, 0, 1)
				res := "hit"
				if i%3 == 0 {
					res = "miss"
				}
				f.exec(`INSERT INTO cache_use(attempt_id,spec,cache_key,result,bytes,ms,ts) VALUES(?,?,?,?,?,?,?)`, id, "go", "k", res, 1000, 5, fixtureNow.Add(-time.Minute).UnixNano())
				if i%10 == 0 {
					f.exec(`INSERT INTO cache_use(attempt_id,spec,cache_key,result,bytes,ms,ts) VALUES(?,?,?,?,?,?,?)`, id, "go", "k", "saved", 5000, 5, fixtureNow.Add(-time.Minute).UnixNano())
				}
			}
			i++
		}
	}
}

// platform writes the queue, ledger, cache store, lease, node and outbox rows.
func (f fx) platform() {
	for n, trig := range []string{"push", "schedule", "manual"} { // three queued attempts of project a
		pipe, job := fmt.Sprintf("q%d", n), fmt.Sprintf("qj%d", n)
		f.exec(`INSERT INTO pipeline(id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			pipe, "acme/a", "rev", trig, "owner", "z", "d", "i", "m", "r", "s", "running", fixtureNow.UnixNano())
		f.exec(`INSERT INTO job(id,pipeline_id,name,kind,idempotent,infra_max,input_digest,required,status) VALUES(?,?,?,?,?,?,?,?,?)`, job, pipe, "build", "k", 1, 1, "i", 1, "queued")
		f.attempt(fmt.Sprintf("qa%d", n), job, pipe, 1, "queued")
	}
	f.exec(`INSERT INTO coordinator(id,pid,process_start,boot_id,incarnation,heartbeat_at) VALUES('c1',1,'s','b',1,1)`)
	for n, id := range []string{"stale", "live"} { // one expired lease under a running attempt, one current
		pipe, job := "p"+id, "j"+id
		f.exec(`INSERT INTO pipeline(id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			pipe, "acme/b", "rev", "push", "owner", "z", "d", "i", "m", "r", "s", "running", fixtureNow.UnixNano())
		f.exec(`INSERT INTO job(id,pipeline_id,name,kind,idempotent,infra_max,input_digest,required,status) VALUES(?,?,?,?,?,?,?,?,?)`, job, pipe, "build", "k", 1, 1, "i", 1, "running")
		f.attempt("a"+id, job, pipe, 1, "running")
		exp := fixtureNow.Add(time.Hour)
		if n == 0 {
			exp = fixtureNow.Add(-time.Hour)
		}
		f.exec(`INSERT INTO lease(id,attempt_id,host,runner_id,expires_at,heartbeat_at,token_digest) VALUES(?,?,?,?,?,?,?)`, "l"+id, "a"+id, "h", "mac1", exp.UnixNano(), 1, "t")
	}
	for n, conf := range []string{"known", "computed", "estimated", "unknown"} {
		f.exec(`INSERT INTO ledger(attempt_id,pipeline_id,project,provider,class,remote,est_ms,est_micro,act_ms,act_micro,confidence,budget_refs,window_day,rate_micro,rate_unit_ms,inc_ms,min_micro,extras_micro,admitted_at,finalized_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("led%d", n), "p0", []string{"acme/a", "acme/b"}[n%2], "hetzner", "cloud", 1, 60000, 1000*(n+1), nil, nil, conf, "", "2026-10-26", 1, 1, 1, 0, 0, fixtureNow.Add(-time.Hour).UnixNano(), nil)
	}
	for n, size := range []int{4096, 8192} {
		f.exec(`INSERT INTO cache_entry(cache_key,namespace,spec,attempt_id,epoch,source_trust,secrets_received,private_deps,scanned_clean,bytes,created_at,last_used_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("ck%d", n), []byte("ns"), "go", "a0-1", 1, "owner", 0, 0, 1, size, 1, 1)
	}
	f.exec(`INSERT INTO cache_artifact(id,project,run_id,job,attempt_id,revision,decl,path,name,kind,sha256,size,platform,runner_id,isolation,producer,visibility,retention,release,epoch,state,created_at) VALUES('art1','acme/a','p0','build','a0-1','rev',0,'p','n','file',?,2048,'linux','mac1','none','x','project','30d',0,1,'committed',1)`, fmt.Sprintf("%064d", 1))
	for _, n := range []struct {
		id, state string
		age       time.Duration
	}{{"mac1", "online", 0}, {"linux1", "online", 0}, {"old", "offline", 30 * time.Hour}} {
		f.exec(`INSERT INTO node(id,name,version,lifecycle,trust_json,deploy_host_json,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, n.id, n.id, 1, "enrolled", "{}", "{}", n.state, 1, fixtureNow.Add(-n.age).UnixNano())
		f.exec(`INSERT INTO capability_snapshot(node_id,digest,doc_json,observed_at) VALUES(?,?,?,?)`, n.id, "d", fmt.Sprintf(`{"availability":{"slots":{"value":%d}}}`, map[string]int{"mac1": 4, "linux1": 2, "old": 1}[n.id]), 1)
	}
	f.exec(`INSERT INTO outbox(revision,context,seq,state,created_at) VALUES('rev','nself-ci',1,'pending',?)`, fixtureNow.Add(-2*time.Hour).UnixNano())
	f.exec(`INSERT INTO outbox(revision,context,seq,state,created_at) VALUES('rev','nself-ci',2,'posted',?)`, fixtureNow.Add(-3*time.Hour).UnixNano())
}
