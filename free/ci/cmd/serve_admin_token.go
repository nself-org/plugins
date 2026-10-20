package main

// Purpose: `nself ci serve admin token issue|revoke|list` (P7-TRUST-12, Epic D19): scoped user tokens.
// Inputs:  the sub-verb, a principal (issue) or token id, --role, --project, --ttl, --self, --json.
// Outputs: issue prints the token once (text: the token alone on stdout; JSON: data.token); the store
//          keeps only its sha256; revoke and list print ids and state, never a token or a digest.
// Constraints: V15 only; a token needs an expiry (default 720h, 1m to 8760h); no command re-prints a token.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/roles"
)

type adminTokenFlags struct {
	adminRolesFlags
	ttl time.Duration
}

func adminTokenFlagSet(f *adminTokenFlags, w io.Writer) *flag.FlagSet {
	fs := adminRolesFlagSet(&f.adminRolesFlags, w)
	fs.Usage = func() {}
	fs.DurationVar(&f.ttl, "ttl", 0, "token lifetime (default 720h)")
	return fs
}

func init() {
	registerV15("serve admin token", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { var f adminTokenFlags; return adminTokenFlagSet(&f, io.Discard) },
		Handler: func(args []string) int { return runAdminToken(args, productionAdminEnv()) },
	})
}

func runAdminToken(args []string, env adminEnv) int {
	const command = "ci serve admin token"
	var f adminTokenFlags
	pos, err := adminParse(adminTokenFlagSet(&f, env.Err), args)
	if err != nil {
		return adminFail(env, command, adminHasJSON(args), err)
	}
	if len(pos) == 0 {
		return adminFail(env, command, f.json, adminUsage("usage: nself ci serve admin token issue|revoke|list ..."))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	svc, op, closeFn, err := env.Open(ctx)
	if err != nil {
		return adminFail(env, command, f.json, fmt.Errorf("the CI store cannot be opened: %w", err))
	}
	defer closeFn()
	switch pos[0] {
	case "issue":
		if len(pos) != 2 || f.role == "" {
			return adminFail(env, command, f.json, adminUsage("usage: token issue <principal> --role member|release|admin --project <id>... [--ttl 720h] [--self]"))
		}
		clear, t, err := svc.IssueToken(ctx, op, roles.Request{Principal: pos[1], Role: roles.Role(f.role), Projects: f.projects, Self: f.self, TTL: f.ttl})
		if err != nil {
			return adminFail(env, command, f.json, err)
		}
		if f.json {
			return adminWriteData(env, command, map[string]any{"token": clear, "id": t.ID, "principal": t.Principal, "role": t.Role, "projects": t.Projects, "subject": t.Subject,
				"expires_at": time.Unix(0, t.ExpiresAt).UTC().Format(time.RFC3339), "shown_once": true})
		}
		_, _ = fmt.Fprintln(env.Out, clear)
		_, _ = fmt.Fprintf(env.Err, "token %s for %s (%s on %s) expires %s; this is the only time it is shown\n", t.ID, t.Principal, t.Role, strings.Join(t.Projects, ","), time.Unix(0, t.ExpiresAt).UTC().Format(time.RFC3339))
	case "revoke":
		if len(pos) != 2 {
			return adminFail(env, command, f.json, adminUsage("usage: token revoke <id>"))
		}
		if err = adminRestorePending(env); err != nil {
			return adminFail(env, command, f.json, err)
		}
		found, err := svc.RevokeToken(ctx, op, pos[1])
		if err != nil {
			return adminFail(env, command, f.json, err)
		}
		if !found {
			return adminFail(env, command, f.json, adminUsage("no live token %s", pos[1]))
		}
		if f.json {
			return adminWriteData(env, command, map[string]any{"id": pos[1], "revoked": true})
		}
		_, _ = fmt.Fprintf(env.Out, "revoked token %s\n", pos[1])
	case "list":
		ts, err := svc.Tokens(ctx, op)
		if err != nil {
			return adminFail(env, command, f.json, err)
		}
		now := svc.Now().UnixNano()
		rows := make([]map[string]any, 0, len(ts))
		for _, t := range ts {
			state := "live"
			switch {
			case t.RevokedAt != 0:
				state = "revoked"
			case t.ExpiresAt <= now:
				state = "expired"
			}
			rows = append(rows, map[string]any{"id": t.ID, "principal": t.Principal, "role": t.Role, "projects": t.Projects, "subject": t.Subject, "state": state,
				"expires_at": time.Unix(0, t.ExpiresAt).UTC().Format(time.RFC3339)})
		}
		if f.json {
			return adminWriteData(env, command, map[string]any{"tokens": rows})
		}
		for _, r := range rows {
			_, _ = fmt.Fprintf(env.Out, "%s\t%s\t%s\t%s\t%s\n", r["id"], r["principal"], r["role"], r["state"], r["expires_at"])
		}
	default:
		return adminFail(env, command, f.json, adminUsage("unknown token command %q (issue, revoke, list)", pos[0]))
	}
	return 0
}
