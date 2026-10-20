package tart

// Purpose: the VM attempt's steps: claim, clone, boot, wait for ssh, check the network, copy the workspace in,
//          and bring the declared outputs back.
// Inputs:  the executor's workspace root (BeforeJob/AfterJob) and the attempt state from Wrap.
// Outputs: a running clone the job's ssh session enters; errors the executor reports as infra failures.
// Constraints: every tart and ssh start goes through the Tool; no secret is on any argv; the ssh options live
//          in a 0600 config file so the argv carries only the config path and the clone name; host keys of an
//          ephemeral clone on a private NAT are not pinned (ssh would otherwise ask), which is why the
//          session is BatchMode with no forwarding and no agent.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

var sshUser = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// Timeouts and the ssh poll interval; variables so tests can shorten them.
var (
	bootTimeout = 3 * time.Minute
	sshWait     = 2 * time.Minute
	pollEvery   = 2 * time.Second
)

func sshConfigPath(dir, name string) string { return filepath.Join(dir, name+".ssh") }

// sshArgv is the ssh command line; remote is the single remote command string.
func (a *attempt) sshArgv(remote string) []string {
	return []string{a.p.opts.SSH, "-F", a.cfgPath, a.name, remote}
}

func (a *attempt) writeSSHConfig(ip netip.Addr) error {
	user := a.p.str(a.p.opts.SSHUser, "admin")
	if !sshUser.MatchString(user) {
		return fmt.Errorf("ci.tart.ssh_user %q is not a valid user name", user)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Host %s\n  HostName %s\n  User %s\n", a.name, ip, user)
	if key := a.p.str(a.p.opts.SSHKey, ""); key != "" {
		if strings.ContainsAny(key, "\"\n\r\x00") {
			return errors.New("ci.tart.ssh_key contains a character that cannot be written to the ssh config")
		}
		fmt.Fprintf(&b, "  IdentityFile \"%s\"\n  IdentitiesOnly yes\n", key)
	}
	b.WriteString("  IdentityAgent none\n  BatchMode yes\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n  GlobalKnownHostsFile /dev/null\n" +
		"  LogLevel ERROR\n  ConnectTimeout 10\n  ServerAliveInterval 15\n  ServerAliveCountMax 4\n" +
		"  ForwardAgent no\n  ForwardX11 no\n  ClearAllForwardings yes\n  ControlMaster no\n  ControlPath none\n  RequestTTY no\n  Compression no\n")
	return os.WriteFile(a.cfgPath, []byte(b.String()), 0o600)
}

// start is the BeforeJob hook: it takes the clone from nothing to a running VM holding the workspace.
func (a *attempt) start(ctx context.Context, ws *os.Root) error {
	cl, err := claimMarker(a.dir, a.name, marker{Name: a.name, Attempt: a.att.ID, Coordinator: a.att.CoordinatorID, PID: os.Getpid(), Created: time.Now().UTC()})
	if err != nil {
		return isolation.Unavailable(isolation.ReasonProbeFailed, "cannot label the clone: %v", err)
	}
	a.mu.Lock()
	a.claim = cl
	a.mu.Unlock()
	tart := a.p.opts.Tart
	if err := a.must(ctx, "clone", []string{tart, "clone", a.image, a.name}); err != nil {
		return err
	}
	if a.cpu > 0 || a.memMB > 0 {
		set := []string{tart, "set", a.name}
		if a.cpu > 0 {
			set = append(set, "--cpu", fmt.Sprint(int(a.cpu+0.999)))
		}
		if a.memMB > 0 {
			set = append(set, "--memory", fmt.Sprint(a.memMB))
		}
		if err := a.must(ctx, "set", set); err != nil {
			return err
		}
	}
	run := append([]string{tart, "run", "--no-graphics"}, a.plan.Args...)
	proc, err := a.tool.Start(append(run, a.name), filepath.Join(a.dir, a.name+".log"))
	if err != nil {
		return fmt.Errorf("tart run: %w", err)
	}
	a.mu.Lock()
	a.proc, a.booted = proc, true
	a.mu.Unlock()
	a.p.slots.markBooted()
	ip, err := a.waitIP(ctx)
	if err != nil {
		return err
	}
	a.vmIP = ip
	if err := a.writeSSHConfig(ip); err != nil {
		return err
	}
	if err := a.waitSSH(ctx, proc); err != nil {
		return err
	}
	if err := a.enforceNetwork(ctx); err != nil {
		return err
	}
	return a.copyIn(ctx, ws)
}

// must runs a tart command that has to succeed.
func (a *attempt) must(ctx context.Context, what string, argv []string) error {
	res, err := a.tool.Run(ctx, argv, nil, nil)
	if err != nil {
		return fmt.Errorf("tart %s: %w", what, err)
	}
	if res.Exit != 0 {
		return fmt.Errorf("tart %s failed (exit %d): %s", what, res.Exit, strings.TrimSpace(firstLine(res.Out)))
	}
	return nil
}

func (a *attempt) waitIP(ctx context.Context) (netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, bootTimeout)
	defer cancel()
	res, err := a.tool.Run(ctx, []string{a.p.opts.Tart, "ip", "--wait", "150", a.name}, nil, nil)
	if err != nil || res.Exit != 0 {
		return netip.Addr{}, fmt.Errorf("the VM %s got no address (exit %d): %v", a.name, res.Exit, err)
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(firstLine(res.Out)))
	if err != nil || !ip.Is4() {
		return netip.Addr{}, fmt.Errorf("tart ip printed %q, not an IPv4 address", strings.TrimSpace(firstLine(res.Out)))
	}
	return ip, nil
}

func (a *attempt) waitSSH(ctx context.Context, proc Proc) error {
	ctx, cancel := context.WithTimeout(ctx, sshWait)
	defer cancel()
	for {
		res, err := a.tool.Run(ctx, a.sshArgv("true"), nil, nil)
		if err == nil && res.Exit == 0 {
			return nil
		}
		select {
		case <-proc.Exited():
			return fmt.Errorf("the VM %s stopped before ssh was ready (see %s)", a.name, filepath.Join(a.dir, a.name+".log"))
		case <-ctx.Done():
			return fmt.Errorf("ssh into %s was not ready: %w", a.name, ctx.Err())
		case <-time.After(pollEvery):
		}
	}
}

// enforceNetwork starts the proxy for restricted and internet and proves the scope from inside the VM.
func (a *attempt) enforceNetwork(ctx context.Context) error {
	if a.plan.Args == nil { // lan and privileged: Tart's default NAT
		return nil
	}
	var proxy *netip.AddrPort
	var facts netscope.Facts
	if a.plan.NeedProxy {
		var err error
		if facts, err = a.p.opts.Net.Facts(ctx); err != nil { // read again: the VM's bridge exists now
			return err
		}
		var hosts []string
		if a.plan.Scope == "restricted" && a.p.opts.Net.Hosts != nil {
			hosts = a.p.opts.Net.Hosts(a.att)
		}
		h, err := a.p.opts.Net.Proxy(ctx, ProxyRequest{Scope: a.plan.Scope, Attempt: a.att, Gateway: a.gateway, VMAddr: a.vmIP, Facts: facts, Hosts: hosts})
		if err != nil {
			return err
		}
		a.mu.Lock()
		a.proxy = h
		a.netEnv = proxyEnv(h.Addr)
		a.mu.Unlock()
		proxy = &h.Addr
	} else if a.p.opts.Net.Facts != nil {
		var err error
		if facts, err = a.p.opts.Net.Facts(ctx); err != nil { // without the facts the proof would test fewer targets
			return err
		}
	}
	return a.verifyNetwork(ctx, planChecks(a.plan, a.gateway, proxy, facts))
}

// proxyEnv is the proxy variables of the ssh session, upper and lower case (tools differ).
func proxyEnv(ap netip.AddrPort) []string {
	url := "http://" + ap.String()
	var env []string
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		env = append(env, k+"="+url)
	}
	return append(env, "NO_PROXY=", "no_proxy=")
}

// copyIn streams the workspace into the VM as a tar.
func (a *attempt) copyIn(ctx context.Context, ws *os.Root) error {
	base := remoteBase(a.att.ID)
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(tarWorkspace(ws, pw)) }()
	remote := "mkdir -p -- " + shQuote(base) + " && tar -xf - -C " + shQuote(base)
	res, err := a.tool.Run(ctx, a.sshArgv(remote), pr, nil)
	_ = pr.CloseWithError(io.ErrClosedPipe)
	if err != nil {
		return fmt.Errorf("copy the workspace into %s: %w", a.name, err)
	}
	if res.Exit != 0 {
		return fmt.Errorf("copy the workspace into %s failed (exit %d): %s", a.name, res.Exit, strings.TrimSpace(firstLine(res.Out)))
	}
	return nil
}

// collect is the AfterJob hook: it brings the declared artifact paths back into the host workspace.
func (a *attempt) collect(ctx context.Context, ws *os.Root) error {
	var declared []string
	for _, d := range a.job.Artifacts {
		declared = append(declared, d.Path)
	}
	if len(declared) == 0 {
		return nil
	}
	paths, err := cleanArtifactPaths(declared)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(a.dir, a.name+".out-")
	if err != nil {
		return err
	}
	defer func() { tmp.Close(); os.Remove(tmp.Name()) }()
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	lim := &limitFile{f: tmp, max: maxCollectBytes/3*4/76*77 + 4<<20, cancel: cancel} // base64, fold newlines, tar headers
	res, err := a.tool.Run(rctx, a.sshArgv(collectCommand(remoteBase(a.att.ID), paths)), nil, lim)
	if lim.exceeded {
		return fmt.Errorf("collect outputs from %s: the VM sent more than the %d byte cap", a.name, maxCollectBytes)
	}
	if err != nil {
		return fmt.Errorf("collect outputs from %s: %w", a.name, err)
	}
	if res.Exit != 0 {
		return fmt.Errorf("collect outputs from %s failed (exit %d)", a.name, res.Exit)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	payload, status := framedPayload(tmp)
	extractErr := extractArtifacts(payload, ws, paths, maxCollectBytes)
	_, _ = io.Copy(io.Discard, payload)
	code, serr := status()
	switch {
	case extractErr != nil:
		return extractErr
	case serr != nil:
		return serr
	case code != 0:
		return fmt.Errorf("collect outputs from %s: the VM could not pack the declared paths (status %d)", a.name, code)
	}
	return nil
}
