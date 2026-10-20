package main

// Purpose: `nself ci providers authorize|revoke <id> --project <p> [--allow-private] [--json]`
// (P7-HOST-12, Epic D14, ADR 0025 section 6): the explicit, admin-only, audited, project-scoped
// authorization of a hosted provider. Both are sugar over the single policy write path of P7-TRUST-26
// (policy.Writer): authorize adds the provider id to trust.providers at project scope, revoke removes
// it, and --allow-private first sets trust.privacy_zone to provider-allowlist at project scope so a
// private repository may use the provider at all.
// Inputs:  the provider id, --project, --allow-private; the stored project row of trust.providers.
// Outputs: what was written and, for --allow-private, the SCHED ceiling command the operator runs
//          next (printed, never run); one v1 JSON envelope with --json.
// Constraints: V15 only; side-effect class write (operator policy only). Every write is
//          Authorize(admin)-gated and audited by the writer in one transaction; a refusal (E687, E670,
//          E651) writes nothing. The zone is written first: provider-allowlist with an empty
//          trust.providers admits nothing, so a failure between the two writes is safe. No provider
//          code is involved: a revision's .nself/ci.yaml cannot authorize anything here, because
//          trust.* keys in a revision only tighten. A non-admin, a machine-invoked caller that would
//          loosen trust.*, and a token without the project are refused by the writer, not here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

const (
	keyTrustProviders   = "trust.providers"
	keyTrustPrivacyZone = "trust.privacy_zone"
	zoneProviderAllow   = "provider-allowlist"
)

// ceilingCommand is the SCHED command that declares how much the operator accepts to spend on id. It
// is printed, never run: a ceiling is a spend decision (the owner's), and policy usage shows it.
func ceilingCommand(id string) string {
	return "nself ci policy set --scope project economics.ceilings." + id + " <micro-currency-units>"
}

// storedProviders is the trust.providers list of the project scope row (nil when there is none).
func storedProviders(ctx context.Context, svc *roles.Service, project string) ([]string, error) {
	rows, err := svc.St.PolicyRows(ctx, "project", project)
	if err != nil {
		return nil, fmt.Errorf("read the project policy: %w", err)
	}
	for _, r := range rows {
		if r.Key != keyTrustProviders {
			continue
		}
		var list []string
		if err := json.Unmarshal(r.Value, &list); err != nil {
			return nil, fmt.Errorf("the stored %s of project %s is not a list of names", keyTrustProviders, project)
		}
		return list, nil
	}
	return nil, nil
}

// withProvider returns list with id added (add) or removed, sorted and without duplicates.
func withProvider(list []string, id string, add bool) []string {
	out := []string{}
	for _, s := range list {
		if s != id {
			out = append(out, s)
		}
	}
	if add {
		out = append(out, id)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func runAuthorize(ctx context.Context, op, id string, f providersFlags, root string, env providersEnv) int {
	command := "ci providers " + op
	fail := func(err error) int { return policyFail(env.policyEnv, command, f.json, err) }
	if f.project == "" {
		return fail(adminUsage("--project <p> is required: authorization is per project"))
	}
	if !validProject(f.project) {
		return fail(adminUsage("--project %q is not a project id (an absolute repository path or a short name, no control characters, no .. segment)", f.project))
	}
	if _, ok := providers.Lookup(id); !ok {
		return fail(unknownProvider(id))
	}
	svc, actor, closeFn, err := env.Open(ctx)
	if err != nil {
		return fail(fmt.Errorf("the CI store cannot be opened: %w", err))
	}
	defer closeFn()
	apply := env.Apply
	if apply == nil {
		w := &policy.Writer{Store: svc.St, Log: svc.Log, Run: env.Run, Machine: env.Machine}
		apply = w.Apply
	}
	current, err := storedProviders(ctx, svc, f.project)
	if err != nil {
		return fail(err)
	}
	next := withProvider(current, id, op == "authorize")
	if op == "revoke" && !slices.Contains(current, id) {
		// Nothing to remove: writing [] here would also drop providers that outer scopes authorize.
		// The admin check still runs, so a non-admin learns nothing and is refused as usual.
		if err = roles.Authorize(ctx, actor, roles.Admin, f.project); err != nil {
			return fail(err)
		}
		return noRevoke(env, command, id, f)
	}
	set := func(key string, value any) error {
		raw, _ := json.Marshal(value)
		_, err := apply(ctx, actor, policy.Change{Op: policy.OpSet, Scope: "project", Project: f.project, Root: root, Key: key, Value: raw})
		return err
	}
	if f.allowPrivate {
		if err = set(keyTrustPrivacyZone, zoneProviderAllow); err != nil {
			return fail(allowPrivateHint(err))
		}
	}
	if err = set(keyTrustProviders, next); err != nil {
		return fail(err)
	}
	data := map[string]any{"op": op, "provider": id, "project": f.project, "trust_providers": next}
	if f.allowPrivate {
		data["privacy_zone"] = zoneProviderAllow
		data["ceiling_command"] = ceilingCommand(id)
	}
	if f.json {
		return adminWriteData(env.adminEnv, command, data)
	}
	verb := map[string]string{"authorize": "authorized", "revoke": "revoked"}[op]
	_, _ = fmt.Fprintf(env.Out, "%s %s for project %s: %s = %s (the project row; admission uses its intersection with the user and team rows)\n", verb, plainText(id), plainText(f.project), keyTrustProviders, plainText(strings.Join(next, ",")))
	if f.allowPrivate {
		_, _ = fmt.Fprintf(env.Out, "set %s = %s for project %s\n", keyTrustPrivacyZone, zoneProviderAllow, plainText(f.project))
		_, _ = fmt.Fprintf(env.Out, "next, declare what you accept to spend (this command is not run for you):\n  %s\n", ceilingCommand(id))
	}
	return 0
}

// allowPrivateHint says why the zone write was refused when the cause is the baseline, which a policy
// row cannot loosen today (debt D-1382), so the operator is not left with a bare E670.
func allowPrivateHint(err error) error {
	var typed *roles.Error
	if errors.As(err, &typed) || !strings.HasPrefix(err.Error(), "E670") {
		return err
	}
	return fmt.Errorf("%w; a private repository's baseline zone is private-infrastructure and the policy engine does not let an operator row loosen it yet (debt D-1382), so nothing was written", err)
}

// noRevoke reports a revoke that had nothing to remove (exit 0, nothing written).
func noRevoke(env providersEnv, command, id string, f providersFlags) int {
	if f.json {
		return adminWriteData(env.adminEnv, command, map[string]any{"op": "revoke", "provider": id, "project": f.project, "changed": false})
	}
	_, _ = fmt.Fprintf(env.Out, "%s is not authorized at project scope for %s: nothing to revoke\n", plainText(id), plainText(f.project))
	return 0
}

// validProject accepts the ids the policy commands use: an absolute repository path or a short name.
func validProject(p string) bool {
	if p == "" || len(p) > 1024 {
		return false
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return !slices.Contains(strings.Split(p, "/"), "..")
}
