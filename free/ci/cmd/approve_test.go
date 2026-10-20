package main

// Purpose: tests of `nself ci runs approve|pending` and the `approval` admission step (P7-TRUST-24, Epic
// D18): a non-author release user admits a held run, the author gets E688 and a member E687, --spend is
// admin only and calls the spend approval once, pending lists held runs with reason keys, and a head that
// moves between approval and lease returns the run to pending.
// Inputs:  a real SQLite store seeded with untrusted pipeline rows and their recorded source facts, scoped
//          tokens issued through the roles service, a fake spend approver.
// Outputs: pass or fail; no network. Fake names only.
// Constraints: each test named by the Ticket's verify line asserts PASS and never skips.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

type approveRig struct {
	*adminRig
	token      string
	spendCalls []string
	spendErr   error
}

func newApproveRig(t *testing.T) *approveRig { return &approveRig{adminRig: newAdminRig(t)} }

func (r *approveRig) env() approveEnv {
	ae := r.adminRig.env()
	open := ae.Open
	ae.Open = func(ctx context.Context) (*roles.Service, roles.Actor, func(), error) {
		svc, op, closeFn, err := open(ctx)
		if err != nil || r.token == "" {
			return svc, op, closeFn, err
		}
		actor, err := svc.ActorFromToken(ctx, r.token)
		return svc, actor, closeFn, err
	}
	return approveEnv{adminEnv: ae, Now: r.now, Spend: func(_ context.Context, run string, a roles.Actor) (economics.Approval, error) {
		r.spendCalls = append(r.spendCalls, run+" by "+a.ID())
		return economics.Approval{Run: run, Actor: a.Label()}, r.spendErr
	}}
}

// as makes the rig act as the token holder principal with role.
func (r *approveRig) as(principal string, role roles.Role) {
	r.t.Helper()
	ctx := context.Background()
	svc, op, closeFn, err := r.adminRig.env().Open(ctx)
	if err != nil {
		r.t.Fatal(err)
	}
	defer closeFn()
	tok, _, err := svc.IssueToken(ctx, op, roles.Request{Principal: principal, Role: role, Projects: []string{roles.AllProjects}, TTL: time.Hour})
	if err != nil {
		r.t.Fatal(err)
	}
	r.token = tok
}

func (r *approveRig) store() *store.Store {
	r.t.Helper()
	st, err := store.Open(r.path, store.Options{Clock: r.now})
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = st.Close() })
	return st
}

// seed creates a queued attempt of an untrusted run whose recorded author is author.
func (r *approveRig) seed(run, author, sha string) string {
	r.t.Helper()
	ctx := context.Background()
	st := r.store()
	row := store.PipelineRow{ID: run, Revision: sha, Project: "/work/" + run, Trigger: "pull_request", SourceTrust: "untrusted", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "cfg-1", SelectionMode: "full", Status: "queued"}
	attempt := run + ".a1"
	if err := st.CreatePipeline(ctx, row); err != nil {
		r.t.Fatal(err)
	}
	if err := st.CreateJob(ctx, store.JobRow{ID: "job-" + run, PipelineID: run, Name: "test", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		r.t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: attempt, JobID: "job-" + run, N: 1}, ""); err != nil {
		r.t.Fatal(err)
	}
	if _, err := st.RecordApprovalFacts(ctx, store.ApprovalRunFacts{Run: run, Author: author, HeadRepo: "fork/app", RequiresApproval: true}, voidCheck); err != nil {
		r.t.Fatal(err)
	}
	return attempt
}

// sqlExec changes a stored row directly: the way a push moves the head a run is bound to.
func (r *approveRig) sqlExec(q string) {
	r.t.Helper()
	db, err := sql.Open("sqlite", r.path)
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(q); err != nil {
		r.t.Fatal(err)
	}
}

func (r *approveRig) approve(args ...string) int { return runApprove(args, r.env()) }

// result decodes the last envelope: the data payload or the error code and message.
func (r *approveRig) result() (data json.RawMessage, code, msg string) {
	r.t.Helper()
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error struct{ Code, Message string }
	}
	if err := json.Unmarshal(r.out.Bytes(), &env); err != nil {
		r.t.Fatalf("not one JSON envelope: %v\n%s", err, r.out.String())
	}
	return env.Data, env.Error.Code, env.Error.Message
}

func (r *approveRig) count(action string) int {
	r.t.Helper()
	var n int
	st := r.store()
	rows, err := st.AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		r.t.Fatal(err)
	}
	for _, a := range rows {
		if a.Action == action {
			n++
		}
	}
	return n
}

func (r *approveRig) approved(run string) bool {
	r.t.Helper()
	s, ok, err := r.store().ApprovalState(context.Background(), run)
	if err != nil || !ok {
		r.t.Fatal(ok, err)
	}
	return s.HasApproval && s.Approval.VoidedAt == 0
}

func TestApproveNonAuthorRelease(t *testing.T) {
	r := newApproveRig(t)
	r.seed("run-1", "mallory", "sha-1")
	r.as("bob", roles.Release)
	if code := r.approve("run-1", "--json"); code != 0 {
		t.Fatalf("exit %d: %s %s", code, r.out.String(), r.errb.String())
	}
	data, _, _ := r.result()
	var got approvedOut
	if err := json.Unmarshal(data, &got); err != nil || got.Run != "run-1" || got.HeadSHA != "sha-1" || got.ConfigDigest != "cfg-1" || got.HeadRepo != "fork/app" || !strings.Contains(got.Approver, "bob") || got.Spend {
		t.Fatalf("approved %+v %v", got, err)
	}
	if !r.approved("run-1") || r.count("approval.grant") != 1 || len(r.spendCalls) != 0 {
		t.Fatal("one approval, one grant row, no spend call")
	}
	// The local operator approves too; the next run is not the operator's own.
	r.token = ""
	r.seed("run-2", "mallory", "sha-2")
	if code := r.approve("run-2"); code != 0 || !strings.Contains(r.out.String(), "approved run-2 at sha-2") {
		t.Fatalf("operator: %d %s %s", code, r.out.String(), r.errb.String())
	}
	// A prefix names the run; --sha pins what the approver looked at.
	r.seed("run-3", "mallory", "sha-3")
	if code := r.approve("run-3", "--sha", "sha-OLD", "--json"); code != 1 {
		t.Fatalf("stale --sha exits %d", code)
	}
	if _, code, msg := r.result(); code != "E688" || !strings.Contains(msg, source.ApprovalBadBinding) || r.approved("run-3") {
		t.Fatalf("stale sha: %s %s", code, msg)
	}
	// A run that needs no approval is refused, after authorization.
	st := r.store()
	if err := st.CreatePipeline(context.Background(), store.PipelineRow{ID: "run-own", Revision: "s", Project: "/work/own", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "c", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if code := r.approve("run-own", "--json"); code != 1 {
		t.Fatalf("owner run exits %d", code)
	}
	if _, code, msg := r.result(); code != "E688" || !strings.Contains(msg, source.ApprovalNotRequired) {
		t.Fatalf("not required: %s %s", code, msg)
	}
}

func TestApproveAuthorRefused(t *testing.T) {
	r := newApproveRig(t)
	r.seed("run-1", "mallory", "sha-1")
	r.seed("run-noauthor", "", "sha-1")
	r.as("mallory", roles.Release)
	if code := r.approve("run-1", "--json"); code != 1 {
		t.Fatalf("author exits %d", code)
	}
	if _, code, msg := r.result(); code != "E688" || !strings.Contains(msg, source.ApprovalSelf) {
		t.Fatalf("author: %s %s", code, msg)
	}
	if r.approved("run-1") || r.count("approval.grant") != 0 {
		t.Fatal("a refused approval records nothing")
	}
	if r.count("authorize") == 0 {
		t.Fatal("the refused decision is audited")
	}
	// With no recorded author nobody can be told apart from the author: refused, never defaulted.
	r.as("bob", roles.Release)
	if code := r.approve("run-noauthor", "--json"); code != 1 {
		t.Fatalf("unknown author exits %d", code)
	}
	if _, code, msg := r.result(); code != "E688" || !strings.Contains(msg, source.ApprovalAuthorUnknown) {
		t.Fatalf("unknown author: %s %s", code, msg)
	}
	// A member is not authorized at all (E687), and learns nothing about unknown runs either.
	r.as("eve", roles.Member)
	for _, run := range []string{"run-1", "no-such-run"} {
		if code := r.approve(run, "--json"); code != 1 {
			t.Fatalf("member %s exits %d", run, code)
		}
		if _, code, msg := r.result(); code != "E687" || strings.Contains(msg, "no run matches") {
			t.Fatalf("member %s: %s %s", run, code, msg)
		}
	}
	r.as("bob", roles.Release)
	if code := r.approve("no-such-run", "--json"); code != 1 {
		t.Fatal(code)
	}
	if _, code, msg := r.result(); code != "E688" || !strings.Contains(msg, source.ApprovalUnknownRun) {
		t.Fatalf("unknown run for a release user: %s %s", code, msg)
	}
	if code := r.approve("--json"); code != 1 {
		t.Fatalf("no run argument exits %d", code)
	}
}

func TestApproveSpendAdminOnly(t *testing.T) {
	r := newApproveRig(t)
	r.seed("run-1", "mallory", "sha-1")
	r.as("bob", roles.Release)
	r.spendErr = &roles.Error{Reason: roles.ReasonRole, Msg: "bob needs role admin"} // what ApproveSpend answers a release user
	if code := r.approve("run-1", "--spend", "--json"); code != 1 {
		t.Fatalf("release --spend exits %d", code)
	}
	if _, code, _ := r.result(); code != "E687" {
		t.Fatalf("release --spend: %s", code)
	}
	if r.approved("run-1") {
		t.Fatal("a refused spend approval records no run approval")
	}
	r.spendCalls, r.spendErr = nil, nil
	r.as("carol", roles.Admin)
	if code := r.approve("run-1", "--spend", "--json"); code != 0 {
		t.Fatalf("admin --spend exits %d: %s", code, r.out.String())
	}
	if len(r.spendCalls) != 1 || !strings.HasPrefix(r.spendCalls[0], "run-1 by carol") {
		t.Fatalf("the spend approval is called once: %v", r.spendCalls)
	}
	st, _, _ := r.store().ApprovalState(context.Background(), "run-1")
	if !st.Approval.Spend || !r.approved("run-1") {
		t.Fatalf("admin approval: %+v", st.Approval)
	}
}

func TestPendingList(t *testing.T) {
	r := newApproveRig(t)
	r.seed("run-held", "mallory", "sha-1")
	r.seed("run-done", "mallory", "sha-2")
	r.as("bob", roles.Release)
	if code := r.approve("run-done"); code != 0 {
		t.Fatal(code)
	}
	list := func() []pendingOut {
		t.Helper()
		if code := runPending([]string{"--json"}, r.env()); code != 0 {
			t.Fatalf("pending exits %d: %s", code, r.out.String())
		}
		data, _, _ := r.result()
		var out []pendingOut
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	got := list()
	if len(got) != 1 || got[0].Run != "run-held" || got[0].HeadSHA != "sha-1" || got[0].ConfigDigest != "cfg-1" || got[0].Author != "mallory" ||
		strings.Join(got[0].Reasons, ",") != source.ReasonApprovalRequired {
		t.Fatalf("pending: %+v", got)
	}
	// A push after approval puts the approved run back on the list with the void reason, and the void is audited once.
	r.sqlExec("UPDATE pipeline SET revision='sha-9' WHERE id='run-done'")
	got = list()
	byRun := map[string]pendingOut{}
	for _, g := range got {
		byRun[g.Run] = g
	}
	if len(got) != 2 || strings.Join(byRun["run-done"].Reasons, ",") != source.ReasonApprovalRequired+","+source.VoidHeadSHA || strings.Join(byRun["run-held"].Reasons, ",") != source.ReasonApprovalRequired {
		t.Fatalf("after the push: %+v", got)
	}
	list()
	if r.count("approval.void") != 1 {
		t.Fatalf("one void row, got %d", r.count("approval.void"))
	}
	if code := runPending([]string{"extra"}, r.env()); code != 1 {
		t.Fatalf("arguments are usage errors: %d", code)
	}
	r.token = ""
	if code := runPending(nil, r.env()); code != 0 || !strings.Contains(r.out.String(), "run-held") || !strings.Contains(r.out.String(), "approval.required") {
		t.Fatalf("text output: %q", r.out.String())
	}
}

func admitLocal(st *store.Store, attempt string) (dispatch.Admitted, error) {
	return admitLocalAt(st, attempt, storedRevision(st, attempt))
}

// storedRevision is the head the run dispatcher takes the lease's checkout revision from: the pipeline row.
func storedRevision(st *store.Store, attempt string) string {
	run := strings.TrimSuffix(attempt, ".a1")
	if got, ok, err := st.ApprovalState(context.Background(), run); err == nil && ok {
		return got.Revision
	}
	return ""
}

func admitLocalAt(st *store.Store, attempt, revision string) (dispatch.Admitted, error) {
	return dispatch.Admit(context.Background(), st, dispatch.Request{AttemptID: attempt, Revision: revision,
		Placement: sched.Placement{JobKey: attempt, RunnerID: "local", CPU: 1, MemMB: 128}, Runner: sched.Runner{ID: "local", Class: "owned", Provider: "operator"},
		Job: sched.Job{ID: attempt, Name: attempt, Project: "project"}, Local: true, Host: "h-" + attempt, CapacityCPU: 4, CapacityMemMB: 4096})
}

func TestApprovedSHALease(t *testing.T) {
	r := newApproveRig(t)
	a1 := r.seed("run-1", "mallory", "sha-1")
	a2 := r.seed("run-2", "mallory", "sha-2")
	st := r.store()
	var ref *dispatch.Refusal
	// Unapproved: held with approval.required, and the attempt stays queued.
	if _, err := admitLocal(st, a1); !errors.As(err, &ref) || ref.Reason != model.PlacementReason(source.ReasonApprovalRequired) {
		t.Fatalf("unapproved run must hold with approval.required: %v", err)
	}
	if state, _, _ := st.AttemptStateEpoch(context.Background(), a1); state != "queued" {
		t.Fatalf("held attempt is %s", state)
	}
	r.as("bob", roles.Release)
	for _, run := range []string{"run-1", "run-2"} {
		if code := r.approve(run); code != 0 {
			t.Fatalf("approve %s: %d %s", run, code, r.errb.String())
		}
	}
	// A push lands after the approval and before the lease: run-1 returns to pending, nothing is leased.
	r.sqlExec("UPDATE pipeline SET revision='sha-1b' WHERE id='run-1'")
	ref = nil
	if _, err := admitLocal(st, a1); !errors.As(err, &ref) || ref.Reason != model.PlacementReason(source.ReasonApprovalRequired) || !strings.Contains(ref.Detail, source.VoidHeadSHA) {
		t.Fatalf("a moved head must hold the run: %v", err)
	}
	if state, _, _ := st.AttemptStateEpoch(context.Background(), a1); state != "queued" {
		t.Fatalf("attempt must stay queued, is %s", state)
	}
	// The untouched run leases, and the SHA it leases at is the approved SHA.
	if _, err := admitLocal(st, a2); err != nil {
		t.Fatalf("approved run: %v", err)
	}
	state, _, _ := st.ApprovalState(context.Background(), "run-2")
	if state.Revision != state.Approval.HeadSHA || state.Revision != "sha-2" {
		t.Fatalf("the executed head %q must equal the approved SHA %q", state.Revision, state.Approval.HeadSHA)
	}
	if got, _, _ := st.AttemptStateEpoch(context.Background(), a2); got != "leased" {
		t.Fatalf("approved attempt is %s", got)
	}
	// A run that never needed approval is untouched by the step.
	if err := st.CreatePipeline(context.Background(), store.PipelineRow{ID: "own", Revision: "s", Project: "/p", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "c", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(context.Background(), store.JobRow{ID: "job-own", PipelineID: "own", Name: "t", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(context.Background(), store.AttemptRow{ID: "own.a1", JobID: "job-own", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := admitLocal(st, "own.a1"); err != nil {
		t.Fatalf("an owner run is not held: %v", err)
	}
}

func TestApproveNamesTheRunExactly(t *testing.T) {
	r := newApproveRig(t)
	r.seed("run-aaaa1111", "mallory", "sha-1")
	r.as("bob", roles.Release)
	// A prefix (or "latest") may resolve to a run a stranger started a moment ago: refused without --sha.
	for _, ref := range []string{"run-aaaa", "latest"} {
		if code := r.approve(ref, "--json"); code != 1 {
			t.Fatalf("%s exits %d", ref, code)
		}
		if _, code, msg := r.result(); code != "E688" || !strings.Contains(msg, source.ApprovalBadBinding) || r.approved("run-aaaa1111") {
			t.Fatalf("%s: %s %s", ref, code, msg)
		}
	}
	if code := r.approve("run-aaaa", "--sha", "sha-OTHER", "--json"); code != 1 || r.approved("run-aaaa1111") {
		t.Fatalf("a wrong --sha is refused: %d", code)
	}
	if code := r.approve("run-aaaa", "--sha", "sha-1"); code != 0 || !r.approved("run-aaaa1111") {
		t.Fatalf("a prefix confirmed by the head is accepted: %d %s", code, r.errb.String())
	}
}

func TestApproveSpendOnlyAndOrdering(t *testing.T) {
	r := newApproveRig(t)
	ctx := context.Background()
	st := r.store()
	// A run that needs no trust approval can still be held for spend: --spend records the spend approval only.
	if err := st.CreatePipeline(ctx, store.PipelineRow{ID: "run-own", Revision: "s1", Project: "/work/own", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "c", SelectionMode: "full", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	r.as("carol", roles.Admin)
	if code := r.approve("run-own", "--spend", "--json"); code != 0 {
		t.Fatalf("spend-only exits %d: %s", code, r.out.String())
	}
	data, _, _ := r.result()
	var got approvedOut
	if err := json.Unmarshal(data, &got); err != nil || got.RunApproved || !got.Spend || len(r.spendCalls) != 1 {
		t.Fatalf("spend-only: %+v %v calls=%v", got, err, r.spendCalls)
	}
	if s, _, _ := st.ApprovalState(ctx, "run-own"); s.HasApproval {
		t.Fatal("spend-only records no run approval")
	}
	if code := r.approve("run-own", "--json"); code != 1 {
		t.Fatalf("without --spend a run that needs no approval is refused: %d", code)
	}
	// A wrong --sha is refused before the spend approval is called.
	r.spendCalls = nil
	r.seed("run-1", "mallory", "sha-1")
	if code := r.approve("run-1", "--spend", "--sha", "sha-OLD", "--json"); code != 1 || len(r.spendCalls) != 0 {
		t.Fatalf("stale --sha with --spend: exit %d, spend calls %v", code, r.spendCalls)
	}
}

func TestApprovalVoidedStaysHeldWhenBindingReturns(t *testing.T) {
	r := newApproveRig(t)
	attempt := r.seed("run-1", "mallory", "sha-1")
	r.as("bob", roles.Release)
	if code := r.approve("run-1"); code != 0 {
		t.Fatal(code)
	}
	ctx := context.Background()
	st := r.store()
	facts := func(repo string) {
		t.Helper()
		if _, err := st.RecordApprovalFacts(ctx, store.ApprovalRunFacts{Run: "run-1", Author: "mallory", HeadRepo: repo, RequiresApproval: true}, voidCheck); err != nil {
			t.Fatal(err)
		}
	}
	facts("other/app") // the head repo changes: the approval is void
	facts("fork/app")  // and changes back: the binding matches again, the void stands
	var ref *dispatch.Refusal
	if _, err := admitLocal(st, attempt); !errors.As(err, &ref) || !strings.Contains(ref.Detail, "voided") {
		t.Fatalf("a voided approval must keep the run held: %v", err)
	}
	if code := runPending([]string{"--json"}, r.env()); code != 0 {
		t.Fatal(code)
	}
	data, _, _ := r.result()
	var list []pendingOut
	if err := json.Unmarshal(data, &list); err != nil || len(list) != 1 || list[0].Run != "run-1" || !strings.Contains(strings.Join(list[0].Reasons, ","), source.VoidHeadRepo) {
		t.Fatalf("pending must list it with the void reason: %+v %v", list, err)
	}
	// Approving the current head again releases it.
	if code := r.approve("run-1"); code != 0 {
		t.Fatal(code)
	}
	if _, err := admitLocal(st, attempt); err != nil {
		t.Fatalf("re-approved run: %v", err)
	}
}

// TestApprovalRefusesForeignCheckoutRevision (D-1360, TM-34): a Request whose revision is not the stored
// head the approval binds is held with approval.required, even under a valid approval; the matching
// revision leases (compared case-insensitively, as the binding is).
func TestApprovalRefusesForeignCheckoutRevision(t *testing.T) {
	r := newApproveRig(t)
	attempt := r.seed("run-1", "mallory", "sha-1")
	r.as("bob", roles.Release)
	if code := r.approve("run-1"); code != 0 {
		t.Fatal(code)
	}
	st := r.store()
	for _, rev := range []string{"sha-evil", ""} {
		var ref *dispatch.Refusal
		if _, err := admitLocalAt(st, attempt, rev); !errors.As(err, &ref) || ref.Reason != model.PlacementReason(source.ReasonApprovalRequired) || !strings.Contains(ref.Detail, "sha-1") {
			t.Fatalf("lease revision %q under an approval for sha-1 must be held: %v", rev, err)
		}
		if state, _, _ := st.AttemptStateEpoch(context.Background(), attempt); state != "queued" {
			t.Fatalf("held attempt is %s", state)
		}
	}
	if _, err := admitLocalAt(st, attempt, "SHA-1"); err != nil {
		t.Fatalf("the approved revision must lease: %v", err)
	}
}
