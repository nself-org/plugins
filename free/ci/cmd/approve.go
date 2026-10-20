package main

// Purpose: `nself ci runs approve <run> [--spend] [--sha <sha>]` and `nself ci runs pending` (P7-TRUST-24,
// Epic D18), and the `approval` admission step that holds a run needing approval until a valid one exists.
// Inputs:  argv after `runs approve|pending`; env NSELF_CI_HOME (state.db).
// Outputs: a text line or one v1 JSON envelope (--json); exit 0 approved or listed, 1 refused (E687 role,
//          E688 author, unknown run, stale head, not required), 2 infrastructure.
// Constraints: V15 only; the caller is the local operator (data-dir access is admin access, Epic D1), the
//          role and non-author rules still run through roles.AuthorizeAction(run.approve), so an
//          author's own approval is E688 and a member's is E687; the project id roles are checked against
//          derives from the run's root, never from the command line; --spend runs economics.ApproveSpend
//          first (admin only, E687 otherwise) and records no approval when it fails; an approval binds to
//          the head SHA, config digest and head repo of the run as stored when it is recorded, so any later
//          change voids it (and voids first, with its own audit row, when a stale approval stands); the run
//          is named exactly or confirmed with --sha, never by a bare prefix or "latest"; --spend on a run
//          that needs no trust approval records the spend approval only; `pending` first writes the voids (one audit row each); the step reads only
//          stored facts inside the admission transaction (no network) and never writes.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func init() {
	registerRunsSub("approve", func(args []string) int { return runApprove(args, productionApproveEnv()) })
	registerRunsSub("pending", func(args []string) int { return runPending(args, productionApproveEnv()) })
	if err := dispatch.RegisterStep("approval", approvalStep); err != nil {
		panic(err)
	}
}

// approveEnv is the environment of the approval commands; tests replace its fields.
type approveEnv struct {
	adminEnv
	Run   policy.Runner
	Now   func() time.Time
	Spend func(ctx context.Context, run string, actor roles.Actor) (economics.Approval, error)
}

func productionApproveEnv() approveEnv {
	return approveEnv{adminEnv: productionAdminEnv(), Run: policy.Runner(report.Capture), Now: time.Now, Spend: economics.ApproveSpend}
}

// voidCheck and approvalNeeded adapt the pure rules in trust/source to the store's types.
func voidCheck(approved, cur store.ApprovalBinding) []string {
	return source.VoidReasons(source.Binding(approved), source.Binding(cur))
}

func approvalNeeded(st store.ApprovalState) bool {
	return source.RequiresApproval(st.Requires, st.Facts, model.TrustClass(st.SourceTrust))
}

// approvalStep is the `approval` admission step (D18). It refuses with approval.required while the run
// needs an approval and none is valid for its stored head SHA, config digest and head repo. Failing to
// read the facts fails the admission closed.
func approvalStep(ctx context.Context, v dispatch.View) error {
	st, ok, err := v.ApprovalState(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("approval: run %s has no stored facts", v.PipelineID)
	}
	if !approvalNeeded(st) {
		return nil
	}
	hold := func(why string) error {
		return &dispatch.Refusal{Reason: model.PlacementReason(source.ReasonApprovalRequired), Detail: why}
	}
	switch reasons := voidCheck(st.Approval.ApprovalBinding, st.Current()); {
	case !st.HasApproval:
		return hold("run " + st.Run + " waits for `nself ci runs approve " + st.Run + "`")
	case st.Approval.VoidedAt != 0:
		return hold("the approval was voided (" + st.Approval.VoidReason + "); approve the current head again")
	case len(reasons) > 0:
		return hold("the approval no longer matches the run (" + strings.Join(reasons, ",") + "); approve the current head again")
	}
	return nil
}

type approveFlags struct {
	json, spend bool
	sha         string
}

func approveFlagSet(f *approveFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("runs approve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.BoolVar(&f.spend, "spend", false, "also approve the run's spend (admin only)")
	fs.StringVar(&f.sha, "sha", "", "refuse unless the run's head is this SHA")
	return fs
}

// approveFail prints err and returns its exit code: E688 is a user error like E687.
func approveFail(env approveEnv, command string, asJSON bool, err error) int {
	var ae *source.ApprovalError
	if !errors.As(err, &ae) {
		return adminFail(env.adminEnv, command, asJSON, err)
	}
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if asJSON {
		_ = output.WriteError(env.Out, command, output.ErrorDetail{Code: source.CodeApproval, Message: msg, ExitCode: output.ExitUser}, nil)
	} else {
		_, _ = io.WriteString(env.Err, msg+"\n")
	}
	return output.ExitUser
}

type approvedOut struct {
	Run          string `json:"run"`
	HeadSHA      string `json:"head_sha"`
	ConfigDigest string `json:"config_digest"`
	HeadRepo     string `json:"head_repo,omitempty"`
	Approver     string `json:"approver"`
	Spend        bool   `json:"spend"`
	// RunApproved is false when only the spend was approved (the run needs no trust approval).
	RunApproved bool `json:"run_approved"`
}

func runApprove(args []string, env approveEnv) int {
	var f approveFlags
	pos, err := adminParse(approveFlagSet(&f), args)
	if err == nil && len(pos) != 1 {
		err = adminUsage("usage: nself ci runs approve <run> [--spend] [--sha <sha>] [--json]")
	}
	if err != nil {
		return approveFail(env, "ci runs approve", adminHasJSON(args), err)
	}
	ctx := context.Background()
	svc, actor, done, err := env.Open(ctx)
	if err != nil {
		return approveFail(env, "ci runs approve", f.json, err)
	}
	defer done()
	out, err := approveRun(ctx, env, svc, actor, pos[0], f)
	if err != nil {
		return approveFail(env, "ci runs approve", f.json, err)
	}
	if f.json {
		return adminWriteData(env.adminEnv, "ci runs approve", out)
	}
	if !out.RunApproved {
		fmt.Fprintf(env.Out, "spend approved for %s at %s by %s (the run needs no trust approval)\n", out.Run, out.HeadSHA, out.Approver)
		return 0
	}
	fmt.Fprintf(env.Out, "approved %s at %s by %s (config %s)\n", out.Run, out.HeadSHA, out.Approver, out.ConfigDigest)
	return 0
}

func approveRun(ctx context.Context, env approveEnv, svc *roles.Service, actor roles.Actor, ref string, f approveFlags) (approvedOut, error) {
	// An identity that cannot approve learns nothing about which runs exist.
	if err := actor.Can(roles.Release, ""); err != nil && roles.ReasonOf(err) != roles.ReasonProject {
		return approvedOut{}, err
	}
	row, err := cirun.ResolveRun(ctx, svc.St, ref)
	if err != nil {
		if cirun.CodeOf(err) == cirun.CodeRun {
			return approvedOut{}, source.NewApprovalError(source.ApprovalUnknownRun, "no run matches %q", ref)
		}
		return approvedOut{}, err
	}
	st, ok, err := svc.St.ApprovalState(ctx, row.ID)
	if err == nil && !ok {
		err = source.NewApprovalError(source.ApprovalUnknownRun, "run %s has no stored facts", row.ID)
	}
	if err != nil {
		return approvedOut{}, err
	}
	project := policy.ProjectIDGit(ctx, env.Run, row.Project)
	needed := approvalNeeded(st)
	// run.approve (and its non-author rule) guards the trust approval; a spend-only approval is guarded by
	// spend.approve inside ApproveSpend.
	if needed {
		if err = roles.AuthorizeAction(roles.WithAuthor(ctx, st.Author), actor, roles.ActRunApprove, project); err != nil {
			return approvedOut{}, authorToE688(err)
		}
	}
	// A prefix or "latest" can land on a run a stranger started a moment ago: the approver must name the
	// run exactly, or confirm the head they looked at with --sha.
	if ref != row.ID && f.sha == "" {
		return approvedOut{}, source.NewApprovalError(source.ApprovalBadBinding, "%q is not an exact run id (it resolves to %s); give the exact id, or --sha <head> to confirm what you reviewed", ref, row.ID)
	}
	if f.sha != "" && !strings.EqualFold(f.sha, st.Revision) {
		return approvedOut{}, source.NewApprovalError(source.ApprovalBadBinding, "run %s is at %s, not %s", st.Run, st.Revision, f.sha)
	}
	if !needed && !f.spend {
		return approvedOut{}, source.NewApprovalError(source.ApprovalNotRequired, "run %s needs no approval", st.Run)
	}
	if f.spend {
		sctx := economics.WithProject(economics.WithEnv(ctx, economics.Env{Store: svc.St, Now: env.Now, Coordinator: "cli"}), project)
		if _, err = env.Spend(sctx, st.Run, actor); err != nil {
			return approvedOut{}, err
		}
	}
	out := approvedOut{Run: st.Run, HeadSHA: st.Revision, ConfigDigest: st.ConfigDigest, HeadRepo: st.HeadRepo, Approver: actor.Label(), Spend: f.spend}
	if !needed {
		return out, nil // the budget step holds any run over the threshold, whatever its trust class
	}
	a, err := svc.St.ApproveRun(ctx, store.ApproveInput{Run: st.Run, Approver: actor.Label(), ExpectSHA: st.Revision, BindAuthor: true, ExpectAuthor: st.Author, Spend: f.spend, Check: voidCheck})
	switch {
	case errors.Is(err, store.ErrApprovalStale):
		return approvedOut{}, source.NewApprovalError(source.ApprovalBadBinding, "run %s changed while it was being approved; look again", st.Run)
	case errors.Is(err, store.ErrApprovalUnknownRun):
		return approvedOut{}, source.NewApprovalError(source.ApprovalUnknownRun, "run %s is gone", st.Run)
	case err != nil:
		return approvedOut{}, err
	}
	return approvedOut{Run: a.Run, HeadSHA: a.HeadSHA, ConfigDigest: a.ConfigDigest, HeadRepo: a.HeadRepo, Approver: a.Approver, Spend: a.Spend, RunApproved: true}, nil
}

// authorToE688 maps the non-author refusals of run.approve to E688; every other refusal stays E687.
func authorToE688(err error) error {
	switch roles.ReasonOf(err) {
	case roles.ReasonSelfApproval:
		return source.NewApprovalError(source.ApprovalSelf, "the author cannot approve their own run")
	case roles.ReasonNoAuthor:
		return source.NewApprovalError(source.ApprovalAuthorUnknown, "the author of the run is unknown, so it cannot be approved")
	}
	return err
}

type pendingOut struct {
	Run          string   `json:"run"`
	Project      string   `json:"project"`
	HeadSHA      string   `json:"head_sha"`
	ConfigDigest string   `json:"config_digest"`
	HeadRepo     string   `json:"head_repo,omitempty"`
	Author       string   `json:"author,omitempty"`
	Reasons      []string `json:"reasons"`
}

func runPending(args []string, env approveEnv) int {
	fs := flag.NewFlagSet("runs pending", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "print one v1 JSON envelope")
	pos, err := adminParse(fs, args)
	if err == nil && len(pos) != 0 {
		err = adminUsage("usage: nself ci runs pending [--json]")
	}
	if err != nil {
		return approveFail(env, "ci runs pending", adminHasJSON(args), err)
	}
	ctx := context.Background()
	svc, actor, done, err := env.Open(ctx)
	if err == nil {
		defer done()
		err = actor.Can(roles.Member, "")
	}
	var list []pendingOut
	if err == nil {
		list, err = pendingRuns(ctx, svc.St)
	}
	if err != nil {
		return approveFail(env, "ci runs pending", *asJSON, err)
	}
	if *asJSON {
		return adminWriteData(env.adminEnv, "ci runs pending", list)
	}
	for _, p := range list {
		fmt.Fprintf(env.Out, "%s  %s  %s  %s\n", plainText(p.Run), plainText(p.HeadSHA), strings.Join(p.Reasons, ","), plainText(p.Project))
	}
	if len(list) == 0 {
		fmt.Fprintln(env.Out, "no run waits for approval")
	}
	return 0
}

// pendingRuns writes the voids of stale approvals, then lists the runs that still wait.
func pendingRuns(ctx context.Context, st *store.Store) ([]pendingOut, error) {
	if _, err := st.SweepApprovals(ctx, "", voidCheck); err != nil {
		return nil, err
	}
	held, err := st.PendingApprovals(ctx, approvalNeeded, voidCheck)
	if err != nil {
		return nil, err
	}
	list := make([]pendingOut, 0, len(held))
	for _, h := range held {
		reasons := []string{source.ReasonApprovalRequired}
		if h.HasApproval && h.Approval.VoidReason != "" {
			reasons = append(reasons, strings.Split(h.Approval.VoidReason, ",")...)
		}
		list = append(list, pendingOut{Run: h.Run, Project: h.Project, HeadSHA: h.Revision, ConfigDigest: h.ConfigDigest, HeadRepo: h.HeadRepo, Author: h.Author, Reasons: reasons})
	}
	return list, nil
}
