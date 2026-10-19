package main

// Purpose: `nself ci serve admin audit [--since T] [--verify] [--json]` and `audit seal` (P7-TRUST-12,
// Epic D20): read, verify and seal the hash-chained audit log.
// Inputs:  --since (an RFC 3339 time or a duration back from now), --verify, --json; the verb `seal`.
// Outputs: rows, or the verify report ("unsealed: n"); `seal` prints how many rows it chained.
// Constraints: V15 only; --verify is read-only and never seals, and exits 1 when a sealed row was changed
//          or unsealed rows are older than the 60 s sealer interval; `seal` is admin and writes one row.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

type adminAuditFlags struct {
	json, verify bool
	since        string
}

func adminAuditFlagSet(f *adminAuditFlags, w io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("serve admin audit", flag.ContinueOnError)
	fs.SetOutput(w)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.BoolVar(&f.verify, "verify", false, "verify the hash chain (read-only; reports unsealed rows)")
	fs.StringVar(&f.since, "since", "", "only rows since this RFC 3339 time or this long ago (for example 24h)")
	return fs
}

func init() {
	registerV15("serve admin audit", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { var f adminAuditFlags; return adminAuditFlagSet(&f, io.Discard) },
		Handler: func(args []string) int { return runAdminAudit(args, productionAdminEnv()) },
	})
}

func parseSince(s string, now time.Time) (int64, error) {
	if s == "" {
		return 0, nil
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return now.Add(-d).UnixNano(), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0, adminUsage("--since takes an RFC 3339 time or a duration such as 24h")
	}
	return t.UnixNano(), nil
}

func runAdminAudit(args []string, env adminEnv) int {
	const command = "ci serve admin audit"
	var f adminAuditFlags
	pos, err := adminParse(adminAuditFlagSet(&f, env.Err), args)
	if err != nil {
		return adminFail(env, command, adminHasJSON(args), err)
	}
	if len(pos) > 1 || (len(pos) == 1 && pos[0] != "seal") {
		return adminFail(env, command, f.json, adminUsage("usage: nself ci serve admin audit [--since T] [--verify] [--json] | audit seal"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	svc, op, closeFn, err := env.Open(ctx)
	if err != nil {
		return adminFail(env, command, f.json, fmt.Errorf("the CI store cannot be opened: %w", err))
	}
	defer closeFn()
	if len(pos) == 1 {
		return adminAuditSeal(ctx, env, svc, op, f.json)
	}
	if err = op.Can(roles.Release, ""); err != nil {
		return adminFail(env, command, f.json, err)
	}
	if f.verify {
		return adminAuditVerify(ctx, env, svc, f.json)
	}
	since, err := parseSince(f.since, svc.Now())
	if err != nil {
		return adminFail(env, command, f.json, err)
	}
	var rows []map[string]any
	for after := int64(0); ; {
		page, err := svc.St.AuditEntries(ctx, after, 1000)
		if err != nil {
			return adminFail(env, command, f.json, err)
		}
		if len(page) == 0 {
			break
		}
		for _, e := range page {
			after = e.Seq
			if e.TS >= since {
				rows = append(rows, map[string]any{"seq": e.Seq, "ts": time.Unix(0, e.TS).UTC().Format(time.RFC3339Nano), "actor": e.Actor, "action": e.Action, "target": e.Target, "detail": e.Detail, "sealed": e.Head != nil})
			}
		}
	}
	if f.json {
		return adminWriteData(env, command, map[string]any{"rows": rows})
	}
	for _, r := range rows {
		_, _ = fmt.Fprintf(env.Out, "%d\t%s\t%s\t%s\t%s\t%s\n", r["seq"], r["ts"], r["actor"], r["action"], r["target"], r["detail"])
	}
	return 0
}

func adminAuditVerify(ctx context.Context, env adminEnv, svc *roles.Service, asJSON bool) int {
	const command = "ci serve admin audit"
	log := &audit.Log{St: svc.St}
	rep, err := log.Verify(ctx, audit.SealInterval)
	if err != nil {
		return adminFail(env, command, asJSON, err)
	}
	exit := 0
	if !rep.OK() {
		exit = 1
	}
	if asJSON {
		data := map[string]any{"ok": rep.OK(), "rows": rep.Rows, "sealed": rep.Sealed, "unsealed": rep.Unsealed, "overdue": rep.Overdue, "bad_seq": rep.BadSeq, "reason": rep.Reason}
		if adminWriteData(env, command, data) != 0 {
			return 2
		}
		return exit
	}
	_, _ = fmt.Fprintf(env.Out, "rows: %d sealed: %d unsealed: %d\n", rep.Rows, rep.Sealed, rep.Unsealed)
	if rep.BadSeq != 0 {
		_, _ = fmt.Fprintf(env.Err, "audit chain broken at row %d: %s\n", rep.BadSeq, rep.Reason)
	}
	if rep.Overdue > 0 {
		_, _ = fmt.Fprintf(env.Err, "unsealed: %d row(s) older than %s; run `nself ci serve admin audit seal`\n", rep.Overdue, audit.SealInterval)
	}
	return exit
}

func adminAuditSeal(ctx context.Context, env adminEnv, svc *roles.Service, op roles.Actor, asJSON bool) int {
	const command = "ci serve admin audit"
	if err := op.Can(roles.Admin, ""); err != nil {
		return adminFail(env, command, asJSON, err)
	}
	n, err := svc.Log.Seal(ctx)
	if err != nil {
		return adminFail(env, command, asJSON, err)
	}
	if err = svc.Log.Append(ctx, audit.Event{Actor: op.Label(), Action: "audit.seal", Target: "audit", Detail: fmt.Sprintf("sealed=%d", n)}); err != nil {
		return adminFail(env, command, asJSON, err)
	}
	if asJSON {
		return adminWriteData(env, command, map[string]any{"sealed": n})
	}
	_, _ = fmt.Fprintf(env.Out, "sealed %d row(s)\n", n)
	return 0
}
