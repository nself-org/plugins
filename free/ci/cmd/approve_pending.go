package main

// List pending approvals after voiding stale approval bindings.
import (
	"context"
	"flag"
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
	"io"
	"strings"
)

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
		_, _ = fmt.Fprintf(env.Out, "%s  %s  %s  %s\n", plainText(p.Run), plainText(p.HeadSHA), strings.Join(p.Reasons, ","), plainText(p.Project))
	}
	if len(list) == 0 {
		_, _ = fmt.Fprintln(env.Out, "no run waits for approval")
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
