package main

// Purpose: `nself ci providers authorize|revoke <id> --project <p> [--allow-private] [--json]`
// (P7-HOST-12, Epic D14, ADR 0025 section 6): the explicit, admin-only, audited, project-scoped
// authorization of a hosted provider. Both are sugar over the single policy write path of P7-TRUST-26
// (policy.Writer): authorize adds the provider id to trust.providers at project scope, revoke removes
// it, and --allow-private first sets trust.privacy_zone to provider-allowlist at project scope so a
// private repository may use the provider at all.
// Inputs:  the provider id, --project (the repository path of the project: it is canonicalized with
//          policy.ProjectIDGit, the same id admission, `policy set` and the doctor key their rows by),
//          --allow-private; the stored project row of trust.providers.
// Outputs: what was written and, for --allow-private, the SCHED ceiling command the operator runs
//          next (printed, never run); one v1 JSON envelope with --json.
// Constraints: V15 only; side-effect class write (operator policy only). Every write is
//          Authorize(admin)-gated and audited by the writer in one transaction; a refusal (E687, E670,
//          E651) writes nothing. The zone is written first: provider-allowlist with an empty
//          trust.providers admits nothing, so a failure between the two writes is safe. No provider
//          code is involved: a revision's .nself/ci.yaml cannot authorize anything here, because
//          trust.* keys in a revision only tighten. The read-modify-write of the list is not one
//          transaction; a compare-and-swap inside the writer refuses a stale list, so concurrent
//          administrators must read again and retry rather than overwrite another change. A non-admin,
//          a machine-invoked caller that would loosen trust.*, and a token without the project are refused by the writer, not here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	pdoctor "github.com/nself-org/plugins/free/ci/internal/providers/doctor"
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

// storedProviders returns the project list and its prior JSON for compare-and-swap
// (both nil when the row is absent).
func storedProviders(ctx context.Context, svc *roles.Service, project string) ([]string, json.RawMessage, error) {
	rows, err := svc.St.PolicyRows(ctx, "project", project)
	if err != nil {
		return nil, nil, fmt.Errorf("read the project policy: %w", err)
	}
	for _, r := range rows {
		if r.Key != keyTrustProviders {
			continue
		}
		var list []string
		if err := json.Unmarshal(r.Value, &list); err != nil {
			return nil, nil, fmt.Errorf("the stored %s of project %s is not a list of names", keyTrustProviders, project)
		}
		return list, r.Value, nil
	}
	return nil, nil, nil
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

// resolveProject turns --project into the project-scope subject: a repository path (absolute, ~/…, or
// starting with a dot) run through policy.ProjectIDGit, so a symlink or a linked worktree reaches the same row
// admission reads. A bare name is refused: no policy command keys a row by a name.
func resolveProject(ctx context.Context, run policy.Runner, p string) (string, error) {
	bad := adminUsage("--project %q is not a repository path: pass the project's directory (absolute, ~/…, or ./…)", p)
	if p == "" || len(p) > 4096 {
		return "", bad
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return "", bad
		}
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", bad
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if !filepath.IsAbs(p) && p != "." && !strings.HasPrefix(p, "./") && !strings.HasPrefix(p, "../") {
		return "", bad
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", bad
	}
	var spawnFailed atomic.Bool
	watch := func(c context.Context, argv ...string) (string, int, error) {
		out, exit, err := run(c, argv...)
		if err != nil {
			spawnFailed.Store(true)
		}
		return out, exit, err
	}
	id := policy.ProjectIDGit(ctx, watch, abs)
	if spawnFailed.Load() {
		// As cmd/policy.go: a project id built without git would key a row admission never reads.
		return "", adminUsage("git could not be run, so the project id of %s is unknown; a project row would not match the runs", abs)
	}
	return id, nil
}

func runAuthorize(ctx context.Context, op, id string, f providersFlags, env providersEnv) int {
	command := "ci providers " + op
	fail := func(err error) int { return policyFail(env.policyEnv, command, f.json, err) }
	if f.project == "" {
		return fail(adminUsage("--project <repository path> is required: authorization is per project"))
	}
	project, err := resolveProject(ctx, env.Run, f.project)
	if err != nil {
		return fail(err)
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
	current, prior, err := storedProviders(ctx, svc, project)
	if err != nil {
		return fail(err)
	}
	next := withProvider(current, id, op == "authorize")
	if op == "revoke" && !slices.Contains(current, id) {
		// Nothing to remove: writing [] here would also drop providers that outer scopes authorize.
		// The admin check still runs, so a non-admin learns nothing and is refused as usual.
		if err = roles.Authorize(ctx, actor, roles.Admin, project); err != nil {
			return fail(err)
		}
		return noRevoke(env, command, id, project, f.json)
	}
	set := func(key string, value any) error {
		raw, _ := json.Marshal(value)
		change := policy.Change{Op: policy.OpSet, Scope: "project", Project: project, Root: project, Key: key, Value: raw}
		if key == keyTrustProviders {
			change.Expect = &prior
		}
		_, err := apply(ctx, actor, change)
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
	zone := pdoctor.Load(ctx, project, env.Run, svc.St).PrivacyZone
	return reportWrite(env, command, op, id, project, next, zone, f)
}

// reportWrite prints what a write changed, with the ceiling command for --allow-private and the note
// that trust.providers matters only under the provider-allowlist zone.
func reportWrite(env providersEnv, command, op, id, project string, next []string, zone string, f providersFlags) int {
	data := map[string]any{"op": op, "provider": id, "project": project, "trust_providers": next, "effective_zone": zone}
	if f.allowPrivate {
		data["privacy_zone"] = zoneProviderAllow
		data["ceiling_command"] = ceilingCommand(id)
	}
	if f.json {
		return adminWriteData(env.adminEnv, command, data)
	}
	verb := map[string]string{"authorize": "authorized", "revoke": "revoked"}[op]
	w := func(format string, a ...any) { _, _ = fmt.Fprintf(env.Out, format, a...) }
	w("%s %s for project %s: %s = %s (the project row; admission uses its intersection with the user and team rows)\n", verb, plainText(id), plainText(project), keyTrustProviders, plainText(strings.Join(next, ",")))
	if zone != zoneProviderAllow {
		w("note: %s is read only when %s is %s; the effective zone of this project is %q (see debt D-1382 for private repositories)\n", keyTrustProviders, keyTrustPrivacyZone, zoneProviderAllow, zone)
	}
	if f.allowPrivate {
		w("set %s = %s for project %s\n", keyTrustPrivacyZone, zoneProviderAllow, plainText(project))
		w("next, declare what you accept to spend (run it inside %s; this command is not run for you):\n  %s\n", plainText(project), ceilingCommand(id))
	}
	return 0
}

// noRevoke reports a revoke that had nothing to remove (exit 0, nothing written).
func noRevoke(env providersEnv, command, id, project string, asJSON bool) int {
	if asJSON {
		return adminWriteData(env.adminEnv, command, map[string]any{"op": "revoke", "provider": id, "project": project, "changed": false})
	}
	_, _ = fmt.Fprintf(env.Out, "%s is not authorized at project scope for %s: nothing to revoke (a user or team scope row may still authorize it: see `nself ci policy show`)\n", plainText(id), plainText(project))
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
