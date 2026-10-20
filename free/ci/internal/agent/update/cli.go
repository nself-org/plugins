package update

// Purpose: the command surface of `nself ci nodes upgrade [<node>|--all] [--version V] [--allow-downgrade]` (hq NODE D11,
//          P7-NODE-24): flags, node selection, one Engine run per node, the output and the exit class. cmd/nodes_upgrade.go only
//          registers it.
// Inputs:  argv after the key; an Env (the lifecycle env, the pinned version, the release mirror, the trusted verifier).
// Outputs: one line per node (or one v1 JSON envelope with --json); exit 0, 1 (refused, usage), 2 (transport or store) or 3
//          (host key, revoked).
// Constraints: ssh nodes run through lifecycle's rotate-mode seam (the registered mode "upgrade"), which gives them the node lock,
//          the revoked check and the pinned ssh target exactly as `nodes rotate` has them; `nodes rotate --mode upgrade` itself is
//          refused. A pull node is upgraded only inside the coordinator process that holds its control stream (AgentControl).
//          --all goes one node at a time and stops at the first failure so a bad release never reaches a second node.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/install"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// ModeName is the rotate mode the ssh path runs under.
const ModeName = "upgrade"

// Env is the composition root's contribution.
type Env struct {
	Lifecycle lifecycle.Env
	// DefaultVersion is the pinned release (empty until the first signed release); BaseURL is the mirror (NSELF_CI_AGENT_RELEASE_URL).
	DefaultVersion, BaseURL string
	Verifier                func() (*verify.Verifier, error)
}

type job struct {
	req Request
	env Env
	res Result
	err error
}

var (
	jobsMu sync.Mutex
	jobs   = map[string]*job{}
)

func init() { lifecycle.RegisterRotateMode(ModeName, sshMode) }

// FlagSet is the flag set of `nodes upgrade` (the subcommand table's Flags constructor).
func FlagSet(out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("nodes upgrade", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.String("version", "", "agent release to install (default: the version this build pins)")
	fs.Bool("all", false, "upgrade every ssh and pull node, one at a time")
	fs.Bool("allow-downgrade", false, "allow a version lower than the installed one (audited)")
	fs.Duration("drain-wait", DefaultDrainWait, "how long to wait for running jobs before giving up")
	fs.Duration("hello-wait", DefaultHelloWait, "how long the upgraded agent has to say hello before the upgrade is rolled back")
	fs.Bool("json", false, "print one v1 JSON envelope")
	return fs
}

// Run executes `nodes upgrade` and returns the exit code.
func Run(args []string, env Env, out, errOut io.Writer) int {
	fs := FlagSet(errOut)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 1
		}
		if fs.NArg() == 0 {
			break
		}
		pos, args = append(pos, fs.Arg(0)), fs.Args()[1:]
	}
	get := func(n string) flag.Value { return fs.Lookup(n).Value }
	asJSON := get("json").String() == "true"
	version := strings.TrimPrefix(get("version").String(), "v")
	if version == "" {
		version = env.DefaultVersion
	}
	all := get("all").String() == "true"
	fail := func(err error) int { return failOut(out, errOut, asJSON, err) }
	switch {
	case !verify.ValidVersion(version):
		return fail(usageErr("no agent release is pinned; pass --version <x.y.z>"))
	case all == (len(pos) == 1) || len(pos) > 1:
		return fail(usageErr("usage: nself ci nodes upgrade <node>|--all [--version V] [--allow-downgrade] [--json]"))
	}
	req := Request{Version: version, AllowDowngrade: get("allow-downgrade").String() == "true"}
	req.DrainWait, _ = time.ParseDuration(get("drain-wait").String())
	req.HelloWait, _ = time.ParseDuration(get("hello-wait").String())
	// Ctrl-C, SIGTERM and SIGHUP cancel the run; the engine then rolls back or puts the node back on a context that survives it.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	d, closeFn, err := env.Lifecycle.Open(ctx, "")
	if err != nil {
		return fail(fmt.Errorf("E607: the CI store cannot be opened: %w", err))
	}
	defer closeFn()
	ids := pos
	var skipped []string
	if all {
		if ids, skipped, err = upgradable(ctx, d); err != nil {
			return fail(err)
		}
		for _, id := range skipped {
			if !asJSON {
				_, _ = fmt.Fprintf(errOut, "skipped %s: a pull node is upgraded by the coordinator process that holds its control stream\n", id)
			}
		}
	} else if remote.ValidateNodeID(strings.ToLower(ids[0])) != nil {
		return fail(usageErr("the node id is not valid (letters, digits, dot, dash, underscore)"))
	}
	var done []Result
	for _, id := range ids {
		res, rerr := upgradeNode(ctx, d, env, strings.ToLower(id), req)
		if rerr == nil {
			done = append(done, res)
			if !asJSON {
				_, _ = fmt.Fprintln(out, describe(res))
			}
			continue
		}
		return fail(fmt.Errorf("%w (done: %d of %d)", rerr, len(done), len(ids)))
	}
	if asJSON {
		if output.WriteData(out, "ci nodes upgrade", map[string]any{"nodes": done, "skipped": skipped}, nil) != nil {
			return output.ExitInfra
		}
	}
	return 0
}

// upgradeNode picks the carrier of one node and runs the engine.
func upgradeNode(ctx context.Context, d *lifecycle.Deps, env Env, id string, req Request) (Result, error) {
	n, err := d.Registry.Get(ctx, id)
	if err != nil {
		return Result{Node: id}, err
	}
	j := &job{req: req, env: env}
	switch n.Capability.Identity.Transport {
	case "agent":
		return runAgent(ctx, d, j, n.Capability)
	case "ssh":
		jobsMu.Lock()
		jobs[id] = j
		jobsMu.Unlock()
		defer func() { jobsMu.Lock(); delete(jobs, id); jobsMu.Unlock() }()
		err = d.Rotate(ctx, lifecycle.RotateOptions{NodeID: id, Mode: ModeName})
		if j.err == nil {
			j.err = err
		}
		return j.res, j.err
	}
	return Result{Node: id}, usageErr("node %s uses the %q transport; only ssh and agent nodes have an agent to upgrade", id, n.Capability.Identity.Transport)
}

// sshMode is the rotate mode: it runs the engine with the pinned target the lifecycle command resolved.
func sshMode(ctx context.Context, d *lifecycle.Deps, r lifecycle.RotateRequest) error {
	jobsMu.Lock()
	j := jobs[r.NodeID]
	jobsMu.Unlock()
	if j == nil {
		return usageErr("use `nself ci nodes upgrade <node>`; the upgrade is not a rotate mode")
	}
	n, err := d.Registry.Get(ctx, r.NodeID)
	if err != nil {
		j.err = err
		return err
	}
	plat, err := platformOf(ctx, d, r.Target, n.Capability)
	if err != nil {
		j.err = err
		return err
	}
	node := &SSHNode{NodeID: r.NodeID, Plat: plat, Target: r.Target, Run: d.Run, Copy: d.Copy,
		DrainFn: drainFn(d, r.NodeID), ProbeFn: probeFn(d, r.NodeID)}
	j.res, j.err = runEngine(ctx, d, j, node)
	return j.err
}

func runAgent(ctx context.Context, d *lifecycle.Deps, j *job, c model.Capability) (Result, error) {
	id := c.Identity.ID
	ctl, err := CurrentAgentControl()
	if err != nil {
		return Result{Node: id}, err
	}
	plat, err := capabilityPlatform(c)
	if err != nil {
		return Result{Node: id}, err
	}
	node := &AgentNode{NodeID: id, Plat: plat, Ctl: ctl, DrainFn: drainFn(d, id), ProbeFn: probeFn(d, id)}
	return runEngine(ctx, d, j, node)
}

func runEngine(ctx context.Context, d *lifecycle.Deps, j *job, n Node) (Result, error) {
	ver, err := j.env.Verifier()
	if err != nil {
		return Result{Node: n.ID()}, err
	}
	rel := install.Release{Version: j.req.Version, BaseURL: j.env.BaseURL, Verifier: ver}
	j.req.ManifestURL = releaseBase(j.env.BaseURL, j.req.Version) + "/" + verify.BinaryName(verify.Target(n.Platform().OS, n.Platform().Arch)) + ".manifest.json"
	eng := Engine{Source: ReleaseSource{Release: rel}, Audit: StoreAuditor{Store: d.Store, Actor: d.Actor}}
	return eng.Run(ctx, n, j.req)
}

func releaseBase(base, version string) string {
	if base = strings.TrimRight(base, "/"); base != "" {
		return base
	}
	return install.DefaultBaseURL + "/v" + version
}

// upgradable lists the ssh and pull nodes that are not revoked; pull nodes this process cannot reach come back as skipped.
func upgradable(ctx context.Context, d *lifecycle.Deps) (ids, skipped []string, err error) {
	all, err := d.Registry.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	_, noCtl := CurrentAgentControl()
	for _, n := range all {
		t, id := n.Capability.Identity.Transport, n.Capability.Identity.ID
		switch {
		case n.Record.State == "revoked" || n.Capability.Lifecycle.RevokedAt != nil:
		case t == "ssh":
			ids = append(ids, id)
		case t == "agent" && noCtl != nil:
			skipped = append(skipped, id)
		case t == "agent":
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, skipped, usageErr("no ssh or reachable pull node is registered")
	}
	return ids, skipped, nil
}

func describe(r Result) string {
	switch {
	case r.Noop:
		return fmt.Sprintf("node %s already runs %s", r.Node, r.To)
	case r.Direction == Downgrade:
		return fmt.Sprintf("node %s downgraded %s -> %s (audited)", r.Node, r.From, r.To)
	}
	return fmt.Sprintf("node %s upgraded %s -> %s", r.Node, r.From, r.To)
}

func usageErr(format string, a ...any) error { return fmt.Errorf("E651: "+format, a...) }

// failOut prints the error and returns its exit class: E666 and usage are the caller's (1), host key and revoked are auth (3),
// everything else is infrastructure (2).
func failOut(out, errOut io.Writer, asJSON bool, err error) int {
	code := lifecycle.CodeOf(err)
	exit := output.ExitInfra
	switch code {
	case CodeRefused, "E651", "E650":
		exit = output.ExitUser
	case "E655", "E657":
		exit = output.ExitAuth
	}
	if asJSON {
		if code == "" {
			code = "E607"
		}
		_ = output.WriteError(out, "ci nodes upgrade", output.ErrorDetail{Code: code, Message: err.Error(), ExitCode: exit}, nil)
		return exit
	}
	_, _ = io.WriteString(errOut, err.Error()+"\n")
	return exit
}
