package main

// Purpose: `nself ci serve admin roles grant|revoke|list` (P7-TRUST-12, Epic D19): role grants over the
// three roles, scoped to projects.
// Inputs:  the sub-verb, a principal id, --role, --project (repeatable, or "*"), --self, --json.
// Outputs: a text line or one v1 JSON envelope; one audit row per grant and per revoke.
// Constraints: V15 only; --self applies only to the operator's own principal; revoking a principal with no
//          grant exits 1 and writes nothing.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/roles"
)

type adminRolesFlags struct {
	json, self bool
	role       string
	projects   multiFlag
}

func adminRolesFlagSet(f *adminRolesFlags, w io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("serve admin roles", flag.ContinueOnError)
	fs.SetOutput(w)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.BoolVar(&f.self, "self", false, "the grant is for the operator's own identity (counts as self)")
	fs.StringVar(&f.role, "role", "", "member, release or admin")
	fs.Var(&f.projects, "project", "project id the role applies to (repeatable; * for all)")
	return fs
}

func init() {
	registerV15("serve admin roles", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { var f adminRolesFlags; return adminRolesFlagSet(&f, io.Discard) },
		Handler: func(args []string) int { return runAdminRoles(args, productionAdminEnv()) },
	})
}

func runAdminRoles(args []string, env adminEnv) int {
	const command = "ci serve admin roles"
	var f adminRolesFlags
	pos, err := adminParse(adminRolesFlagSet(&f, env.Err), args)
	if err != nil {
		return adminFail(env, command, adminHasJSON(args), err)
	}
	if len(pos) == 0 {
		return adminFail(env, command, f.json, adminUsage("usage: nself ci serve admin roles grant|revoke|list ..."))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	svc, op, closeFn, err := env.Open(ctx)
	if err != nil {
		return adminFail(env, command, f.json, fmt.Errorf("the CI store cannot be opened: %w", err))
	}
	defer closeFn()
	switch pos[0] {
	case "grant":
		if len(pos) != 2 || f.role == "" {
			return adminFail(env, command, f.json, adminUsage("usage: roles grant <principal> --role member|release|admin --project <id>... [--self]"))
		}
		req := roles.Request{Principal: pos[1], Role: roles.Role(f.role), Projects: f.projects, Self: f.self}
		if err = svc.Grant(ctx, op, req); err != nil {
			return adminFail(env, command, f.json, err)
		}
		if f.json {
			return adminWriteData(env, command, map[string]any{"principal": req.Principal, "role": f.role, "projects": []string(f.projects), "self": f.self})
		}
		_, _ = fmt.Fprintf(env.Out, "granted %s to %s on %s\n", f.role, req.Principal, strings.Join(f.projects, ","))
	case "revoke":
		if len(pos) != 2 {
			return adminFail(env, command, f.json, adminUsage("usage: roles revoke <principal>"))
		}
		if err = adminRestorePending(env); err != nil {
			return adminFail(env, command, f.json, err)
		}
		found, err := svc.Revoke(ctx, op, pos[1])
		if err != nil {
			return adminFail(env, command, f.json, err)
		}
		if !found {
			return adminFail(env, command, f.json, adminUsage("%s holds no grant", pos[1]))
		}
		if f.json {
			return adminWriteData(env, command, map[string]any{"principal": pos[1], "revoked": true})
		}
		_, _ = fmt.Fprintf(env.Out, "revoked the grant and the live tokens of %s\n", pos[1])
	case "list":
		gs, err := svc.List(ctx, op)
		if err != nil {
			return adminFail(env, command, f.json, err)
		}
		rows := make([]map[string]any, 0, len(gs))
		for _, g := range gs {
			rows = append(rows, map[string]any{"principal": g.Principal, "role": g.Role, "projects": g.Projects, "subject": g.Subject, "granted_by": g.GrantedBy, "granted_at": time.Unix(0, g.GrantedAt).UTC().Format(time.RFC3339)})
		}
		if f.json {
			return adminWriteData(env, command, map[string]any{"grants": rows})
		}
		for _, g := range gs {
			_, _ = fmt.Fprintf(env.Out, "%s\t%s\t%s\t%s\n", g.Principal, g.Role, strings.Join(g.Projects, ","), g.Subject)
		}
	default:
		return adminFail(env, command, f.json, adminUsage("unknown roles command %q (grant, revoke, list)", pos[0]))
	}
	return 0
}
