package coordclient

// Purpose: tests of the stats client (P7-SCHED-20) against the real user API with the store the sim serves:
// a member scoped to one project gets that project's document and no coordinator-wide block, an all-project
// member gets every project's, and both are the documents the coordinator host computes for the same caller.
// Inputs:  the sim of rig_test.go and two outcome rows. Outputs: pass or fail.
// Constraints: the stats source is process-global, restored on cleanup; real TLS on 127.0.0.1.

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/stats"
)

func seedOutcomes(t *testing.T, s *sim) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(s.dir, "state.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for i, project := range []string{"acme/app", "acme/other"} {
		id := string(rune('a' + i))
		for _, q := range []string{
			`INSERT INTO pipeline(id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at) VALUES('p` + id + `','` + project + `','r','push','t','z','d','i','m','r','s','passed',1)`,
			`INSERT INTO job(id,pipeline_id,name,kind,idempotent,infra_max,input_digest,required,status) VALUES('j` + id + `','p` + id + `','build','k',1,1,'i',1,'passed')`,
			`INSERT INTO attempt(id,job_id,n,state) VALUES('a` + id + `','j` + id + `',1,'passed')`,
			`INSERT INTO schedtel_outcome(attempt_id,project,job_key,runner,runner_class,wait_ms,runtime_ms,cold_start_ms,cost_micro,result,failure_class,ts) VALUES('a` + id + `','` + project + `','build','r1','owned',5,9,-1,0,'passed','',` + strconv.FormatInt(time.Now().UnixMilli(), 10) + `)`,
		} {
			if _, err = db.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestStatsClientSeesOnlyTheCallersProjects(t *testing.T) {
	s := newSim(t)
	seedOutcomes(t, s)
	clientapi.SetStatsSource(func(ctx context.Context, o stats.Options) (stats.Doc, error) { return stats.Compute(ctx, s.st, o) })
	t.Cleanup(func() { clientapi.SetStatsSource(nil) })
	ctx := context.Background()

	member := s.client(Creds{CA: s.rootFile(), TokenFile: s.tokenFile("mia", roles.Member, time.Hour, "acme/app")})
	raw, err := member.Stats(ctx, time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	var got stats.Doc
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Attempts == nil || got.Attempts.Finished != 1 || got.Nodes != nil || got.Reasons["nodes"] != stats.ReasonScoped {
		t.Fatalf("scoped member document: %s", raw)
	}
	if _, err = member.Stats(ctx, time.Hour, "acme/other"); err == nil {
		t.Fatal("a project outside the grant was served")
	}

	whole := s.client(Creds{CA: s.rootFile(), TokenFile: s.tokenFile("ops", roles.Member, time.Hour, roles.AllProjects)})
	raw, err = whole.Stats(ctx, time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	var all stats.Doc
	if err = json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	local, err := stats.Compute(ctx, s.st, stats.Options{Now: time.Now(), Since: time.Hour, Global: true})
	if err != nil {
		t.Fatal(err)
	}
	if all.Attempts == nil || all.Attempts.Finished != 2 || *all.Attempts != *local.Attempts || all.Nodes != nil && local.Nodes == nil {
		t.Fatalf("remote %+v differs from the coordinator host %+v", all.Attempts, local.Attempts)
	}
}
