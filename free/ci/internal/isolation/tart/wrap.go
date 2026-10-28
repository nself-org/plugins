package tart

// Purpose: turn a JobSpec into one ssh session into a fresh clone, with the clone's whole life hung on the
//          executor's hooks so it is destroyed on every exit path.
// Inputs:  the spec the Isolator hook hands to Wrap.
// Outputs: a spec whose Command is `ssh -F <config> <clone> /bin/bash -s`, whose Stdin is the lazily built
//          script (env, secrets, command), and whose BeforeJob, AfterJob and Release create the clone, bring
//          the declared outputs back and delete it.
// Constraints: nothing is created in Wrap but a slot reservation; the network scope is planned here so E679
//          surfaces before any workspace exists; a slot is released on every path; host mounts are not mounted.

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

// attempt is the state of one VM attempt.
type attempt struct {
	p       *Profile
	att     isolation.Attempt
	tool    Tool
	name    string
	dir     string
	image   string
	job     model.Job
	command []string
	cpu     float64
	memMB   int
	gateway netip.Addr
	vmIP    netip.Addr
	plan    softnetPlan
	secrets *capturingSource

	mu      sync.Mutex
	claim   *claim
	proc    Proc
	proxy   ProxyHandle
	booted  bool
	netEnv  []string
	cfgPath string
	once    sync.Once
}

// Wrap attaches the VM lifecycle to the spec.
func (p *Profile) Wrap(s exec.JobSpec) (exec.JobSpec, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	av := p.probe(ctx, true)
	// the slot the burst provider granted this attempt is what makes the probe read full: not a reason to refuse it
	if !av.Available() && !(av.Reason == ReasonSlots && p.slots.holds(s.AttemptID)) {
		return s, isolation.Unavailable(av.Reason, "%s isolation is unavailable: %s", p.level, av.Facts["hint"])
	}
	if len(s.Command) == 0 {
		return s, errors.New("tart: the job has no command")
	}
	if _, err := workdirIn("", s.Job.Workdir); err != nil {
		return s, err
	}
	for k := range s.Job.Env {
		if !envName.MatchString(k) {
			return s, fmt.Errorf("tart: environment name %q cannot be exported into the VM", k)
		}
	}
	if len(s.Job.Secrets) > 0 && s.Secrets == nil {
		return s, isolation.Unavailable(ReasonSecrets, "this job declares secrets but the run supplies no per-attempt secret source the VM profile can deliver over stdin")
	}
	a := &attempt{
		p: p, att: isolation.Attempt{ID: s.AttemptID, CoordinatorID: s.CoordinatorID, Slot: s.Slot},
		name: CloneName(s.AttemptID), image: av.Facts["base_image"], job: s.Job, command: append([]string(nil), s.Command...),
		cpu: s.CPU, memMB: s.MemoryMB,
	}
	home := p.home(s.Home)
	// markers live under the default CI home whatever home the spec names, so every sweep finds them
	a.dir, a.tool = stateDir(p.home("")), p.opts.NewTool(s.CoordinatorID, home)
	p.mu.Lock()
	p.dirs[a.dir] = true
	p.mu.Unlock()
	if err := a.planNetwork(ctx, s.Network, av.Facts["softnet_block"] == "true"); err != nil {
		return s, err
	}
	foreign, _ := strconv.Atoi(av.Facts["foreign_running"])
	own, _ := strconv.Atoi(av.Facts["own_running"])
	if !p.slots.takeOver(s.AttemptID) && !p.slots.reserve(foreign, own) {
		return s, isolation.Unavailable(ReasonSlots, "both macOS VM slots are in use")
	}
	if s.Secrets != nil {
		a.secrets = &capturingSource{inner: s.Secrets}
		s.Secrets = a.secrets
	}
	// the host-side ssh process must not see the job's own env: a repository-chosen DYLD_INSERT_LIBRARIES or
	// LD_* name would load code into it. The script carries the env into the VM from the attempt's own copy.
	s.Job.Env = nil
	a.cfgPath = sshConfigPath(a.dir, a.name)
	s.Command = a.sshArgv("/bin/bash -s")
	s.Stdin = &lazyReader{build: a.script}
	s.NetEnv = nil // the proxy variables go into the script: the address is only known once the VM is up
	s.Mounts = nil // never mounted into the VM
	s.BeforeJob = chainBefore(s.BeforeJob, a.start)
	s.AfterJob = chainAfter(s.AfterJob, a.collect)
	prev := s.Release
	s.Release = func(ctx context.Context) {
		a.release(ctx)
		if prev != nil {
			prev(ctx)
		}
	}
	return s, nil
}

func chainBefore(prev, ours func(context.Context, *os.Root) error) func(context.Context, *os.Root) error {
	return func(ctx context.Context, ws *os.Root) error {
		if prev != nil {
			if err := prev(ctx, ws); err != nil {
				return err
			}
		}
		return ours(ctx, ws)
	}
}

func chainAfter(prev, ours func(context.Context, *os.Root) error) func(context.Context, *os.Root) error {
	return func(ctx context.Context, ws *os.Root) error {
		if err := ours(ctx, ws); err != nil {
			return err
		}
		if prev != nil {
			return prev(ctx, ws)
		}
		return nil
	}
}

// planNetwork reads the facts and plans softnet; scopes below lan fail with E679 when they cannot be enforced.
func (a *attempt) planNetwork(ctx context.Context, scope model.NetworkScope, softnetBlock bool) error {
	gw, err := netip.ParseAddr(a.p.opts.Gateway)
	if err != nil {
		return isolation.ScopeUnenforceable("the Tart gateway %q does not parse", a.p.opts.Gateway)
	}
	a.gateway = gw
	var facts netscope.Facts
	if effectiveScope(scope) != "lan" && effectiveScope(scope) != "privileged" {
		if a.p.opts.Net.Facts == nil {
			return isolation.ScopeUnenforceable("no network facts source is registered, so the %s scope cannot be enforced", effectiveScope(scope))
		}
		if facts, err = a.p.opts.Net.Facts(ctx); err != nil {
			return err
		}
	}
	if a.plan, err = planSoftnet(scope, facts, gw, softnetBlock); err != nil {
		return err
	}
	if a.plan.NeedProxy && a.p.opts.Net.Proxy == nil {
		return isolation.ScopeUnenforceable("no egress proxy is available on this host, so the %s scope cannot be enforced", a.plan.Scope)
	}
	return nil
}

// script builds the stdin script once the executor resolved the secrets.
func (a *attempt) script() ([]byte, error) {
	env := map[string]string{}
	for k, v := range a.job.Env {
		env[k] = v
	}
	a.mu.Lock()
	for _, kv := range a.netEnv {
		if k, v, ok := cutEnv(kv); ok {
			env[k] = v
		}
	}
	a.mu.Unlock()
	var sec map[string]string
	if a.secrets != nil {
		sec = a.secrets.take()
	}
	wd, err := workdirIn(remoteBase(a.att.ID), a.job.Workdir)
	if err != nil {
		return nil, err
	}
	return buildScript(a.command, wd, env, sec)
}

func cutEnv(kv string) (string, string, bool) {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return "", "", false
}

// release destroys everything the attempt created; it is safe to call more than once and after a failed start.
func (a *attempt) release(ctx context.Context) {
	a.once.Do(func() {
		a.mu.Lock()
		proc, proxy, cl, booted := a.proc, a.proxy, a.claim, a.booted
		a.mu.Unlock()
		if proxy.Close != nil {
			proxy.Close()
		}
		if cl != nil { // a clone may exist only when the marker was claimed
			if err := destroyClone(ctx, a.tool, a.p.opts.Tart, a.name, proc); err != nil {
				a.p.opts.Log("clone %s could not be removed (%v); the next start sweeps it", a.name, err)
				cl.unlockKeep() // the marker stays: the next Prepare's sweep retries the removal
				a.p.slots.release(booted)
				return
			}
		}
		if proc != nil {
			_ = proc.Kill()
		}
		cl.release()
		a.p.slots.release(booted)
	})
}
