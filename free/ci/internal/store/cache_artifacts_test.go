package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func artHex(c string) string { return strings.Repeat(c, 64) }

// artRunning moves attempt a to running (epoch 2) and declares one output.
func artRunning(t *testing.T, s *Store, id string, decl int) {
	t.Helper()
	ctx := context.Background()
	for _, st := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"queued", "leased", 0}, {"leased", "running", 1}} {
		if err := s.Transition(ctx, id, st.from, st.to, TransitionDetail{Epoch: st.epoch}); err != nil {
			t.Fatal(err)
		}
	}
	var ds []records.Decl
	for i := 0; i < decl; i++ {
		ds = append(ds, records.Decl{Ord: i, Path: "out/" + string(rune('a'+i)), Kind: records.KindOther, Retention: "30d", Visibility: authz.VisibilityProject})
	}
	if err := s.ArtifactDeclare(ctx, id, 2, ds); err != nil {
		t.Fatal(err)
	}
}

func artRec(attempt, name string, decl int, sha string, size int64, now time.Time) records.Record {
	exp := now.Add(24 * time.Hour)
	return records.Record{ID: records.RecordID(attempt, name), Attempt: attempt, Name: name, Path: name, Decl: decl, Kind: records.KindOther, SHA256: sha, Size: size,
		Platform: "linux/amd64", Isolation: "process", Producer: "nself-ci/t", Visibility: authz.VisibilityProject, Retention: "1d", CreatedAt: now, ExpiresAt: &exp, State: records.StateStaged, Epoch: 2}
}

func artPolicy() records.Policy {
	return records.Policy{MaxFile: 1 << 30, MaxTotal: 1 << 20, ProjectShare: 50}
}

func TestCacheArtifactsMigrate(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	for _, tbl := range []string{"cache_artifact", "cache_artifact_decl"} {
		var n int
		if err := s.readers.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s missing (%d, %v)", tbl, n, err)
		}
	}
	seed(t, s, "a")
	artRunning(t, s, "a", 1)
	now := time.Now()
	// stage needs the running attempt at its epoch; a wrong epoch is the E711 capability refusal with one audit row
	rec := artRec("a", "out/a", 0, artHex("1"), 10, now)
	if err := s.ArtifactStage(ctx, "a", 1, []records.Record{rec}); !records.IsReason(err, records.ReasonEpoch) || !strings.Contains(err.Error(), "E711") {
		t.Fatalf("stale epoch stage: %v", err)
	}
	var audits int
	_ = s.readers.QueryRow("SELECT count(*) FROM audit WHERE action='artifact.stage.refused'").Scan(&audits)
	if audits != 1 {
		t.Fatalf("refusal audit rows = %d", audits)
	}
	if err := s.ArtifactStage(ctx, "a", 2, []records.Record{rec}); err != nil {
		t.Fatal(err)
	}
	// the facts of a record come from the store, not from the caller
	got, ok, err := s.ArtifactGet(ctx, rec.ID)
	if err != nil || !ok || got.Project != "proj" || got.Run != "pa" || got.Job != "a" || got.Revision != "rev" || got.State != records.StateStaged {
		t.Fatalf("stored record: %+v ok=%v err=%v", got, ok, err)
	}
	// commit requires finalizing: a running attempt is refused
	if _, err := s.ArtifactCommit(ctx, "a", 2, []records.Record{got}, artPolicy()); !records.IsReason(err, records.ReasonAttempt) {
		t.Fatalf("commit while running: %v", err)
	}
	// R-DS-20 backstop: a pass with a staged record becomes failed/infra with no seam involved
	if err := s.Transition(ctx, "a", "running", "finalizing", TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, "a", "finalizing", "passed", TransitionDetail{Epoch: 3}); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := s.AttemptStateEpoch(ctx, "a"); state != "failed" {
		t.Fatalf("uncommitted artifact passed: state %s", state)
	}
	var class string
	_ = s.readers.QueryRow("SELECT failure_class FROM attempt WHERE id='a'").Scan(&class)
	if class != "infra" {
		t.Fatalf("failure class %q", class)
	}
	// a second attempt of the same job commits, passes, and the record is immutable but for expiry and missing
	seed(t, s, "b")
	artRunning(t, s, "b", 1)
	rb := artRec("b", "out/a", 0, artHex("2"), 20, now)
	if err := s.ArtifactStage(ctx, "b", 2, []records.Record{rb}); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, "b", "running", "finalizing", TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	staged, _ := s.ArtifactStaged(ctx, "b")
	if _, err := s.ArtifactCommit(ctx, "b", 3, staged, artPolicy()); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, "b", "finalizing", "passed", TransitionDetail{Epoch: 3}); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := s.AttemptStateEpoch(ctx, "b"); state != "passed" {
		t.Fatalf("committed attempt state %s", state)
	}
	for _, q := range []string{"UPDATE cache_artifact SET sha256='" + artHex("9") + "' WHERE id=?", "UPDATE cache_artifact SET size=1 WHERE id=?", "UPDATE cache_artifact SET visibility='restricted' WHERE id=?", "UPDATE cache_artifact SET state='staged' WHERE id=?",
		"UPDATE cache_artifact SET producer='x' WHERE id=?", "UPDATE cache_artifact SET kind='binary' WHERE id=?", "UPDATE cache_artifact SET image='{}' WHERE id=?", "UPDATE cache_artifact SET isolation='vm' WHERE id=?", "UPDATE cache_artifact SET retention='keep' WHERE id=?", "UPDATE cache_artifact SET run_id='other' WHERE id=?"} {
		if _, err := s.writer.Exec(q, rb.ID); err == nil || !strings.Contains(err.Error(), "E712 immutable") {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := s.writer.Exec("UPDATE cache_artifact SET expires_at=1, missing=1 WHERE id=?", rb.ID); err != nil {
		t.Fatalf("expiry and missing must stay writable: %v", err)
	}
	// two attempts, same path: two records, neither replaced
	all, _ := s.ArtifactList(ctx, records.Filter{Name: "out/a"})
	if len(all) != 1 || all[0].Attempt != "b" {
		t.Fatalf("committed list: %+v", all)
	}
}

func TestCacheArtifactsRedeclareSupersedesOlderEpoch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	seed(t, s, "a")
	artRunning(t, s, "a", 1)
	if err := s.ArtifactStage(ctx, "a", 2, []records.Record{artRec("a", "n", 0, artHex("1"), 5, now)}); err != nil {
		t.Fatal(err)
	}
	decls := []records.Decl{{Ord: 0, Path: "out/a", Kind: records.KindOther, Retention: "30d", Visibility: authz.VisibilityProject}}
	if err := s.ArtifactDeclare(ctx, "a", 2, decls); !records.IsReason(err, records.ReasonDeclaration) {
		t.Fatalf("a second declare in the same epoch: %v", err)
	}
	// the attempt row runs again (lost -> retryable -> queued -> leased -> running): a later epoch
	for _, st := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"running", "lost", 2}, {"lost", "retryable", 3}, {"retryable", "queued", 4}, {"queued", "leased", 5}, {"leased", "running", 6}} {
		if err := s.Transition(ctx, "a", st.from, st.to, TransitionDetail{Epoch: st.epoch, Operator: st.from == "lost" || st.from == "retryable"}); err != nil {
			t.Fatalf("%s>%s: %v", st.from, st.to, err)
		}
	}
	if err := s.ArtifactDeclare(ctx, "a", 7, decls); err != nil {
		t.Fatalf("redeclare in a later epoch: %v", err)
	}
	if staged, _ := s.ArtifactStaged(ctx, "a"); len(staged) != 0 {
		t.Fatalf("the earlier epoch's staged records must be gone: %+v", staged)
	}
	if err := s.ArtifactStage(ctx, "a", 7, []records.Record{artRec("a", "n", 0, artHex("2"), 5, now)}); err != nil {
		t.Fatal(err)
	}
	// once committed, the attempt cannot be declared again
	if err := s.Transition(ctx, "a", "running", "finalizing", TransitionDetail{Epoch: 7}); err != nil {
		t.Fatal(err)
	}
	staged, _ := s.ArtifactStaged(ctx, "a")
	if _, err := s.ArtifactCommit(ctx, "a", 8, staged, artPolicy()); err != nil {
		t.Fatal(err)
	}
}

func TestCacheArtifactsRetention(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	type spec struct {
		id      string
		release bool
		expire  time.Duration // from now; 0 = keep
		age     time.Duration
	}
	for _, sp := range []spec{{"x", false, -time.Hour, 48 * time.Hour}, {"y", false, time.Hour, 2 * time.Hour}, {"z", false, 0, 0}} {
		seed(t, s, sp.id)
		artRunning(t, s, sp.id, 1)
		r := artRec(sp.id, "n", 0, artHex(sp.id), 100, now.Add(-sp.age))
		r.ExpiresAt = nil
		if sp.expire != 0 {
			e := now.Add(sp.expire)
			r.ExpiresAt = &e
		}
		stageCommit(t, s, sp.id, r)
	}
	// release-kind: job kind "release"
	if _, err := s.writer.Exec(`INSERT INTO job(id,pipeline_id,name,kind,idempotent,infra_max,input_digest,required,status) VALUES ('jr','px','rel','release',0,0,'i',1,'queued')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writer.Exec(`INSERT INTO attempt(id,job_id,n,state,epoch) VALUES ('r','jr',1,'queued',0)`); err != nil {
		t.Fatal(err)
	}
	artRunning(t, s, "r", 1)
	rel := artRec("r", "n", 0, artHex("r"), 500, now.AddDate(-2, 0, 0))
	rel.ExpiresAt = nil
	stageCommit(t, s, "r", rel)
	if got, _, _ := s.ArtifactGet(ctx, rel.ID); !got.Release {
		t.Fatalf("record of a release job must be release-kind: %+v", got)
	}
	dry, err := s.ArtifactPrune(ctx, records.PruneSpec{Now: now, DryRun: true})
	if err != nil || len(dry) != 1 || dry[0].Attempt != "x" {
		t.Fatalf("dry run: %+v %v", dry, err)
	}
	if all, _ := s.ArtifactList(ctx, records.Filter{}); len(all) != 4 {
		t.Fatalf("dry run changed the store: %d records", len(all))
	}
	removed, err := s.ArtifactPrune(ctx, records.PruneSpec{Now: now})
	if err != nil || len(removed) != 1 {
		t.Fatalf("prune: %+v %v", removed, err)
	}
	if all, _ := s.ArtifactList(ctx, records.Filter{}); len(all) != 3 {
		t.Fatalf("after prune: %d records", len(all))
	}
	// release-kind survives a plain prune, goes with --release once older than the age
	if _, err = s.ArtifactPrune(ctx, records.PruneSpec{Now: now.AddDate(5, 0, 0)}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.ArtifactGet(ctx, rel.ID); !ok {
		t.Fatal("a release record was pruned without --release")
	}
	young, _ := s.ArtifactPrune(ctx, records.PruneSpec{Now: now, Release: true, OlderThan: 5 * 365 * 24 * time.Hour})
	if len(young) != 0 {
		t.Fatalf("release prune ignored its age: %+v", young)
	}
	old, err := s.ArtifactPrune(ctx, records.PruneSpec{Now: now, Release: true, OlderThan: 365 * 24 * time.Hour})
	if err != nil || len(old) != 1 || old[0].ID != rel.ID {
		t.Fatalf("release prune: %+v %v", old, err)
	}
	// a staged record of an attempt that ended is an orphan and goes with any prune
	seed(t, s, "o")
	artRunning(t, s, "o", 1)
	if err := s.ArtifactStage(ctx, "o", 2, []records.Record{artRec("o", "n", 0, artHex("o"), 1, now)}); err != nil {
		t.Fatal(err)
	}
	if orph, _ := s.ArtifactPrune(ctx, records.PruneSpec{Now: now, DryRun: true}); len(orph) != 0 {
		t.Fatalf("a live attempt's staged record is not an orphan: %+v", orph)
	}
	if err := s.Transition(ctx, "o", "running", "lost", TransitionDetail{Epoch: 2}); err != nil { // lost keeps its rows until the attempt runs again or a prune finds them
		t.Fatal(err)
	}
	if orph, _ := s.ArtifactPrune(ctx, records.PruneSpec{Now: now}); len(orph) != 1 || orph[0].State != records.StateStaged {
		t.Fatalf("orphan prune: %+v", orph)
	}
	refs, err := s.ArtifactBlobRefs(ctx)
	if err != nil || len(refs) != 1 || refs[0].SHA256 != artHex("z") { // only z (keep) is left
		t.Fatalf("blob refs: %+v %v", refs, err)
	}
}

func TestCacheArtifactsCaps(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	seed(t, s, "a")
	artRunning(t, s, "a", 1)
	r := artRec("a", "n", 0, artHex("a"), 900<<10, now)
	if err := s.ArtifactStage(ctx, "a", 2, []records.Record{r}); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, "a", "running", "finalizing", TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	staged, _ := s.ArtifactStaged(ctx, "a")
	if warned, err := s.ArtifactCommit(ctx, "a", 3, staged, artPolicy()); err != nil || !strings.Contains(warned, "87%") {
		t.Fatalf("commit at 87%%: warn %q err %v", warned, err)
	}
	// the next attempt does not fit: size_cap, an audit row and no committed record
	seed(t, s, "b")
	artRunning(t, s, "b", 1)
	if err := s.ArtifactStage(ctx, "b", 2, []records.Record{artRec("b", "n", 0, artHex("b"), 300<<10, now)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, "b", "running", "finalizing", TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	stagedB, _ := s.ArtifactStaged(ctx, "b")
	if _, err := s.ArtifactCommit(ctx, "b", 3, stagedB, artPolicy()); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("over the cap: %v", err)
	}
	if left, _ := s.ArtifactStaged(ctx, "b"); len(left) != 1 {
		t.Fatal("a refused commit must leave the staged record staged")
	}
	var audits int
	_ = s.readers.QueryRow("SELECT count(*) FROM audit WHERE action='artifact.commit.refused'").Scan(&audits)
	if audits != 1 {
		t.Fatalf("size_cap audit rows = %d", audits)
	}
	// TEAM: the project share is refused while the whole store still has room
	team := artPolicy()
	team.MaxTotal, team.Team, team.ProjectShare = 4<<20, true, 25
	if _, err := s.ArtifactCommit(ctx, "b", 3, stagedB, team); !records.IsReason(err, records.ReasonSizeCap) || !strings.Contains(err.Error(), "NSELF_CI_PROJECT_SHARE") {
		t.Fatalf("project share: %v", err)
	}
	// while another project still commits under the same share
	if err := s.CreatePipeline(ctx, PipelineRow{ID: "pz", Revision: "rev", Project: "other", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateJob(ctx, JobRow{ID: "jz", PipelineID: "pz", Name: "z", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "z", JobID: "jz", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	artRunning(t, s, "z", 1)
	if err := s.ArtifactStage(ctx, "z", 2, []records.Record{artRec("z", "n", 0, artHex("c"), 100<<10, now)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, "z", "running", "finalizing", TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	stagedZ, _ := s.ArtifactStaged(ctx, "z")
	team.ProjectShare = 50
	if _, err := s.ArtifactCommit(ctx, "z", 3, stagedZ, team); err != nil {
		t.Fatalf("a project under its share must still commit: %v", err)
	}
	team.ProjectShare = 100
	if _, err := s.ArtifactCommit(ctx, "b", 3, stagedB, team); err != nil {
		t.Fatalf("with room: %v", err)
	}
}

func stageCommit(t *testing.T, s *Store, id string, r records.Record) {
	t.Helper()
	ctx := context.Background()
	if err := s.ArtifactStage(ctx, id, 2, []records.Record{r}); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, id, "running", "finalizing", TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	staged, err := s.ArtifactStaged(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ArtifactCommit(ctx, id, 3, staged, records.Policy{MaxFile: 1 << 40, MaxTotal: 1 << 50, ProjectShare: 100}); err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(ctx, id, "finalizing", "passed", TransitionDetail{Epoch: 3}); err != nil {
		t.Fatal(err)
	}
}

func TestCacheArtifactsPruneIsProjectScopedAndUsageCountsLiveStaged(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	seed(t, s, "a")
	artRunning(t, s, "a", 1)
	ra := artRec("a", "n", 0, artHex("a"), 100, now.Add(-48*time.Hour))
	ra.ExpiresAt = ptrTime(now.Add(-time.Hour))
	stageCommit(t, s, "a", ra)
	if err := s.CreatePipeline(ctx, PipelineRow{ID: "pq", Revision: "rev", Project: "other", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateJob(ctx, JobRow{ID: "jq", PipelineID: "pq", Name: "q", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "q", JobID: "jq", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	artRunning(t, s, "q", 1)
	rq := artRec("q", "n", 0, artHex("b"), 300, now.Add(-48*time.Hour))
	rq.ExpiresAt = ptrTime(now.Add(-time.Hour))
	stageCommit(t, s, "q", rq)
	// the live staged bytes of a running attempt count toward usage
	seed(t, s, "live")
	artRunning(t, s, "live", 1)
	if err := s.ArtifactStage(ctx, "live", 2, []records.Record{artRec("live", "n", 0, artHex("c"), 1000, now)}); err != nil {
		t.Fatal(err)
	}
	if u, err := s.ArtifactUsage(ctx, "proj"); err != nil || u.Total != 1400 || u.Project != 1100 {
		t.Fatalf("usage: %+v %v", u, err)
	}
	dry, err := s.ArtifactPrune(ctx, records.PruneSpec{Now: now, Project: "other", DryRun: true})
	if err != nil || len(dry) != 1 || dry[0].Attempt != "q" {
		t.Fatalf("a project-scoped prune lists only its project: %+v %v", dry, err)
	}
	if _, err = s.ArtifactPrune(ctx, records.PruneSpec{Now: now, Project: "other"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.ArtifactGet(ctx, ra.ID); !ok {
		t.Fatal("another project's expired record was removed")
	}
	if err = s.ArtifactSetMissing(ctx, ra.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.ArtifactGet(ctx, ra.ID); !got.Missing {
		t.Fatal("set missing")
	}
	if err = s.ArtifactSetMissing(ctx, ra.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.ArtifactGet(ctx, ra.ID); got.Missing {
		t.Fatal("clear missing")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestCacheArtifactsRecordsOfAnAttemptThatDidNotPassAreDropped(t *testing.T) {
	for _, end := range []struct {
		name string
		to   model.JobState
	}{{"failed", "failed"}, {"cancelled", "cancelled"}} {
		t.Run(end.name, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			seed(t, s, "a")
			artRunning(t, s, "a", 1)
			if err := s.ArtifactStage(ctx, "a", 2, []records.Record{artRec("a", "n", 0, artHex("1"), 5, time.Now())}); err != nil {
				t.Fatal(err)
			}
			if err := s.Transition(ctx, "a", "running", "finalizing", TransitionDetail{Epoch: 2}); err != nil {
				t.Fatal(err)
			}
			staged, _ := s.ArtifactStaged(ctx, "a")
			if _, err := s.ArtifactCommit(ctx, "a", 3, staged, artPolicy()); err != nil {
				t.Fatal(err)
			}
			if err := s.Transition(ctx, "a", "finalizing", end.to, TransitionDetail{Epoch: 3, FailureClass: "infra"}); err != nil {
				t.Fatal(err)
			}
			if all, _ := s.ArtifactList(ctx, records.Filter{}); len(all) != 0 {
				t.Fatalf("committed records of a %s attempt stay listed: %+v", end.name, all)
			}
			if _, ok, _ := s.ArtifactGet(ctx, staged[0].ID); ok {
				t.Fatal("record still readable")
			}
		})
	}
}

func TestCacheArtifactsReservationsStopConcurrentOverrun(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seed(t, s, "a")
	seed(t, s, "b")
	artRunning(t, s, "a", 1)
	artRunning(t, s, "b", 1)
	pol := artPolicy() // 1 MiB
	if err := s.ArtifactReserve(ctx, "a", 2, 700<<10, pol); err != nil {
		t.Fatal(err)
	}
	if err := s.ArtifactReserve(ctx, "b", 2, 400<<10, pol); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("a second claim over the cap: %v", err)
	}
	if err := s.ArtifactReserve(ctx, "b", 2, 300<<10, pol); err != nil {
		t.Fatalf("a claim that fits: %v", err)
	}
	if err := s.ArtifactReserve(ctx, "a", 2, 100<<10, pol); err != nil { // the same attempt replaces its claim
		t.Fatalf("re-reserve: %v", err)
	}
	if u, _ := s.ArtifactUsage(ctx, "proj"); u.Total != 400<<10 {
		t.Fatalf("usage counts live claims: %+v", u)
	}
	if err := s.ArtifactReserve(ctx, "a", 1, 1, pol); !records.IsReason(err, records.ReasonEpoch) {
		t.Fatalf("stale epoch: %v", err)
	}
	// staging replaces the claim with the rows; ending the attempt drops it
	if err := s.ArtifactStage(ctx, "a", 2, []records.Record{artRec("a", "n", 0, artHex("1"), 50<<10, time.Now())}); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.ArtifactUsage(ctx, "proj"); u.Total != (300+50)<<10 {
		t.Fatalf("after stage: %+v", u)
	}
	if err := s.Transition(ctx, "b", "running", "failed", TransitionDetail{Epoch: 2, FailureClass: "code"}); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.ArtifactUsage(ctx, "proj"); u.Total != 50<<10 {
		t.Fatalf("after b ended: %+v", u)
	}
	// a claim of an attempt that ended another way is dropped by the next prune
	seed(t, s, "c")
	artRunning(t, s, "c", 1)
	if err := s.ArtifactReserve(ctx, "c", 2, 10, pol); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, "c", "running", "lost", TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ArtifactPrune(ctx, records.PruneSpec{Now: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = s.readers.QueryRow("SELECT count(*) FROM cache_artifact_reserve WHERE attempt_id='c'").Scan(&left)
	if left != 0 {
		t.Fatalf("a lost attempt's claim survived a prune: %d", left)
	}
}
