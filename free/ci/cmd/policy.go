package main

// Purpose: `nself ci policy set|unset|pin` (P7-TRUST-26, Epic D15): the write
// commands of the operator policy. `policy show` is in policy_show.go; `policy
// protect` and `policy usage` belong to CI and SCHED.
// Inputs:  `set --scope user|team|project [--team id] <key> <value>`, `unset
// --scope ... <key>`, `pin <sha>|--clear`; env NSELF_CI_HOME (state.db),
// NSELF_CI_REPO (repository root), NSELF_INVOKED_BY (machine-invoked caller).
// Outputs: a text line or one v1 JSON envelope (--json); exit 0 written, 1
// refused (E687, E672, E670, E651), 2 infrastructure.
// Constraints: V15 only; the caller is the local operator (data-dir access is
// admin access, Epic D1) and every write is Authorize(admin)-gated and audited
// in the same transaction (internal/policy.Writer); a value is JSON when it
// parses as JSON, else the plain string; no Secret value is printed.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

const (
	policySetUsage   = "nself ci policy set --scope user|team|project [--team <id>] <key> <value> [--json]"
	policyUnsetUsage = "nself ci policy unset --scope user|team|project [--team <id>] <key> [--json]"
	policyPinUsage   = "nself ci policy pin <sha> | --clear [--json]"
	policyShowUsage  = "nself ci policy show [--effective] [--json]"
)

// policyFlags holds every flag of the policy commands; each command registers its own subset.
type policyFlags struct {
	json, clear, effective bool
	scope, team            string
}

func policyFlagSet(op string, f *policyFlags, w io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("policy "+op, flag.ContinueOnError)
	fs.SetOutput(w)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	switch op {
	case policy.OpSet, policy.OpUnset:
		fs.StringVar(&f.scope, "scope", "", "user, team or project")
		fs.StringVar(&f.team, "team", "", "team id (for --scope team)")
	case "show":
		fs.BoolVar(&f.effective, "effective", false, "print only the effective values")
		fs.StringVar(&f.team, "team", "", "also apply this team's rows")
	case policy.OpPin:
		fs.BoolVar(&f.clear, "clear", false, "remove the pin (back to the protected branch tip)")
	}
	return fs
}

func init() {
	for _, op := range []string{"show", policy.OpSet, policy.OpUnset, policy.OpPin} {
		op := op
		registerV15("policy "+op, subcommandEntry{V15Only: true,
			Flags:   func() *flag.FlagSet { var f policyFlags; return policyFlagSet(op, &f, io.Discard) },
			Handler: func(args []string) int { return runPolicy(op, args, productionPolicyEnv()) }})
	}
}

// policyEnv is the environment of the policy commands; tests replace its fields.
type policyEnv struct {
	adminEnv
	Run     policy.Runner
	Root    func() (string, error)
	Machine bool
}

func productionPolicyEnv() policyEnv {
	return policyEnv{adminEnv: productionAdminEnv(), Run: policy.Runner(report.Capture),
		Root: func() (string, error) { return evidenceRoot("") }, Machine: os.Getenv("NSELF_INVOKED_BY") != ""}
}

var leadingCode = regexp.MustCompile(`^(E\d{3}) `)

// policyFail prints err and returns its exit code; a policy error that leads with its E-code is a user error.
func policyFail(env policyEnv, command string, asJSON bool, err error) int {
	var typed *adminError
	if m := leadingCode.FindStringSubmatch(err.Error()); m != nil && !errors.As(err, &typed) {
		err = &adminError{m[1], strings.TrimPrefix(err.Error(), m[0])}
	}
	return adminFail(env.adminEnv, command, asJSON, err)
}

func policyArity(op string, f policyFlags, pos []string) error {
	switch {
	case op == policy.OpSet && len(pos) == 2, op == policy.OpUnset && len(pos) == 1, op == "show" && len(pos) == 0,
		op == policy.OpPin && f.clear && len(pos) == 0, op == policy.OpPin && !f.clear && len(pos) == 1:
		return nil
	}
	return adminUsage("usage: %s", map[string]string{policy.OpSet: policySetUsage, policy.OpUnset: policyUnsetUsage, policy.OpPin: policyPinUsage, "show": policyShowUsage}[op])
}

// jsonValue is arg when it parses as JSON, else the JSON string of arg.
func jsonValue(arg string) json.RawMessage {
	if json.Valid([]byte(arg)) {
		return json.RawMessage(arg)
	}
	b, _ := json.Marshal(arg)
	return b
}

func runPolicy(op string, args []string, env policyEnv) int {
	command := "ci policy " + op
	var f policyFlags
	pos, err := adminParse(policyFlagSet(op, &f, env.Err), args)
	if err != nil {
		return policyFail(env, command, adminHasJSON(args), err)
	}
	if err = policyArity(op, f, pos); err != nil {
		return policyFail(env, command, f.json, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root, err := env.Root()
	if err != nil {
		return policyFail(env, command, f.json, fmt.Errorf("the repository root is unknown: %w", err))
	}
	svc, actor, closeFn, err := env.Open(ctx)
	if err != nil {
		return policyFail(env, command, f.json, fmt.Errorf("the CI store cannot be opened: %w", err))
	}
	defer closeFn()
	var gitFailed atomic.Bool
	watch := func(c context.Context, argv ...string) (string, int, error) {
		out, exit, err := env.Run(c, argv...)
		if err != nil {
			gitFailed.Store(true)
		}
		return out, exit, err
	}
	project := policy.ProjectIDGit(ctx, watch, root)
	if gitFailed.Load() && (op == policy.OpPin || f.scope == "project") {
		return policyFail(env, command, f.json, adminUsage("git could not be run, so the project id is unknown; a project row or pin would not match the runs"))
	}
	if op == "show" {
		return showPolicy(ctx, env, svc.St, root, project, f)
	}
	w := &policy.Writer{Store: svc.St, Log: svc.Log, Run: env.Run, Machine: env.Machine}
	c := policy.Change{Op: op, Scope: f.scope, Team: f.team, Root: root, Project: project}
	switch {
	case op == policy.OpSet:
		c.Key, c.Value = pos[0], jsonValue(pos[1])
	case op == policy.OpUnset:
		c.Key = pos[0]
	case f.clear:
		c.Op = policy.OpClear
	default:
		c.SHA = pos[0]
	}
	res, err := w.Apply(ctx, actor, c)
	if err != nil {
		return policyFail(env, command, f.json, err)
	}
	if f.json {
		return adminWriteData(env.adminEnv, command, map[string]any{"op": res.Op, "scope": res.Scope, "subject": res.Subject, "key": res.Key, "value": res.Value, "sha": res.SHA})
	}
	_, _ = fmt.Fprintln(env.Out, plainText(describePolicyWrite(res)))
	return 0
}

func describePolicyWrite(r policy.Result) string {
	switch r.Op {
	case policy.OpSet:
		return fmt.Sprintf("set %s = %s in %s scope (%s)", r.Key, r.Value, r.Scope, r.Subject)
	case policy.OpUnset:
		return fmt.Sprintf("unset %s in %s scope (%s)", r.Key, r.Scope, r.Subject)
	case policy.OpPin:
		return fmt.Sprintf("pinned the protected branch policy of %s to %s", r.Subject, r.SHA)
	}
	return fmt.Sprintf("cleared the pin of %s; the protected branch tip applies", r.Subject)
}
