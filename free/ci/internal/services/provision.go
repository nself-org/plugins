package services

// Purpose: start disposable service dependencies for one job attempt through
// the nself CLI (Epic D9). Each Up creates a throwaway nSelf project in
// $NSELF_CI_HOME/work/<nselfci-id>/svc/ and starts only the named components;
// the plugin never writes compose and never touches the user's project.
// Inputs:  the specs (job-facing name, nSelf component, optional version).
// Outputs: a Handle with the injected connection environment.
// Constraints: canon verbs only (init, config set, config services
// enable|start|list, build); every call goes through the Runner (the exec
// funnel in production); ports bind 127.0.0.1; credentials are random per Up,
// reported to Secret and never logged; a failed Up tears down what it made.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Spec is one requested service.
type Spec struct {
	Name      string // the job-facing name (the NAME in NSELF_CI_SERVICE_<NAME>_*)
	Component string // the nSelf component; "" means Name
	Version   string // optional image tag
}

// Provisioner starts and stops service projects.
type Provisioner struct {
	Run    Runner
	Home   string        // NSELF_CI_HOME
	Nself  string        // the nself binary; "" means "nself"
	Docker string        // the docker binary; "" means "docker"
	Wait   time.Duration // readiness deadline per service; 0 means 90s
	// Ready replaces the protocol probe (tests); it gets the loopback address.
	Ready func(ctx context.Context, addr string) error
	// Secret is told each generated credential so the redactor can mask it.
	Secret func(string)
}

// Handle is a running provision.
type Handle struct {
	Project string            // the compose project, nselfci-<8 hex>
	Dir     string            // the throwaway project directory
	Network string            // the compose network a container job joins
	Env     map[string]string // connection environment for the job

	mu    sync.Mutex
	root  string
	lock  *ownerLock
	ports []int
	creds []string
	done  bool
}

func (p *Provisioner) nself() string  { return orDefault(p.Nself, "nself") }
func (p *Provisioner) docker() string { return orDefault(p.Docker, "docker") }
func (p *Provisioner) waitFor() time.Duration {
	if p.Wait > 0 {
		return p.Wait
	}
	return 90 * time.Second
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// cliEnv is what every nself call receives on top of the minimal environment:
// the engine only runs in v1.5 mode, so the CLI is told to speak it.
var cliEnv = map[string]string{"NSELF_V15": "1"}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("services: no entropy: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Up starts the services. On error nothing it created remains.
func (p *Provisioner) Up(ctx context.Context, specs []Spec) (*Handle, error) {
	conns, err := resolve(specs)
	if err != nil {
		return nil, err
	}
	if err = p.preflight(ctx); err != nil {
		return nil, err
	}
	project := "nselfci-" + randHex(4)
	h := &Handle{Project: project, root: filepath.Join(p.Home, "work", project)}
	h.Dir = filepath.Join(h.root, "svc")
	if err = os.MkdirAll(filepath.Dir(h.root), 0o700); err != nil {
		return nil, fail(CodeProvision, "cannot create the work directory: %v", err)
	}
	var held bool
	if h.lock, held, err = acquireLock(h.root + ".lock"); err != nil {
		return nil, fail(CodeProvision, "cannot lock the provision %s: %v", project, err)
	} else if held {
		return nil, fail(CodeProvision, "provision %s is already locked by another coordinator", project)
	}
	if err = p.provision(ctx, h, conns); err != nil {
		bg, done := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer done()
		_ = p.Down(bg, h)
		return nil, err
	}
	return h, nil
}

// provision runs the steps; the caller tears h down when it fails.
func (p *Provisioner) provision(ctx context.Context, h *Handle, conns []conn) error {
	if err := os.MkdirAll(h.Dir, 0o700); err != nil {
		return fail(CodeProvision, "cannot create the project directory: %v", err)
	}
	if err := p.configure(ctx, h, conns); err != nil {
		return err
	}
	if err := p.start(ctx, h, conns); err != nil {
		return err
	}
	h.Network = p.network(ctx, h)
	h.Env = buildEnv(conns, h.Network)
	return nil
}

// configure creates the project and sets ports, credentials and toggles.
func (p *Provisioner) configure(ctx context.Context, h *Handle, conns []conn) error {
	if err := p.cli(ctx, h, "init", "--non-interactive", "--name", h.Project); err != nil {
		return err
	}
	if err := p.checkNames(ctx, h, conns); err != nil {
		return err
	}
	ports, err := pickPorts(len(conns))
	if err != nil {
		return err
	}
	h.ports = ports
	for i := range conns {
		c := &conns[i]
		c.HostPort, c.Password = ports[i], randHex(12)
		if c.Comp.PasswordKey == "" {
			c.Password = ""
		}
		if c.Comp.UserKey != "" {
			c.User = c.Comp.User
		}
		if c.Comp.DBKey != "" {
			c.DB = c.Comp.DB
		}
		if c.Password != "" {
			h.creds = append(h.creds, c.Password)
			if p.Secret != nil {
				p.Secret(c.Password)
			}
		}
		for _, kv := range c.settings() {
			if err := p.cli(ctx, h, "config", "set", kv[0], kv[1]); err != nil {
				return err
			}
		}
		if !c.Comp.Required {
			if err := p.cli(ctx, h, "config", "services", "enable", c.Comp.Name); err != nil {
				return err
			}
		}
	}
	return p.cli(ctx, h, "build")
}

// settings are the `config set` pairs of a connection.
func (c conn) settings() [][2]string {
	out := [][2]string{{c.Comp.PortKey, itoa(c.HostPort)}}
	if c.Comp.PasswordKey != "" {
		out = append(out, [2]string{c.Comp.PasswordKey, c.Password})
	}
	if c.Comp.UserKey != "" {
		out = append(out, [2]string{c.Comp.UserKey, c.User})
	}
	if c.Comp.DBKey != "" {
		out = append(out, [2]string{c.Comp.DBKey, c.DB})
	}
	if c.Spec.Version != "" && c.Comp.VersionKey != "" {
		out = append(out, [2]string{c.Comp.VersionKey, c.Spec.Version})
	}
	return out
}

// start starts only the named components and waits for each port.
func (p *Provisioner) start(ctx context.Context, h *Handle, conns []conn) error {
	for i := range conns {
		if err := p.startOne(ctx, h, &conns[i]); err != nil {
			return err
		}
	}
	for _, c := range conns {
		var err error
		if p.Ready != nil {
			err = p.Ready(ctx, loopback(c.HostPort))
		} else {
			err = waitReady(ctx, loopback(c.HostPort), c.Comp.Probe, p.waitFor())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// startOne starts one component. A host port picked as free can be taken by
// another process before Docker binds it; then a new port is set and the start
// retried (twice at most).
func (p *Provisioner) startOne(ctx context.Context, h *Handle, c *conn) error {
	err := p.cli(ctx, h, "config", "services", "start", c.Comp.Name)
	for try := 0; err != nil && portTaken(err) && try < 2; try++ {
		ports, perr := pickPorts(1)
		if perr != nil {
			return perr
		}
		h.ports = append(h.ports, ports[0])
		c.HostPort = ports[0]
		if err = p.cli(ctx, h, "config", "set", c.Comp.PortKey, itoa(c.HostPort)); err != nil {
			return err
		}
		err = p.cli(ctx, h, "config", "services", "start", c.Comp.Name)
	}
	return err
}

func portTaken(err error) bool {
	m := err.Error()
	return strings.Contains(m, "port is already allocated") || strings.Contains(m, "address already in use") || strings.Contains(m, "Bind for")
}

// network finds the compose network of the project ("" when none is found).
func (p *Provisioner) network(ctx context.Context, h *Handle) string {
	res, err := p.Run.Run(ctx, Call{Argv: []string{p.docker(), "network", "ls", "--filter", "label=com.docker.compose.project=" + h.Project, "--format", "{{.Name}}"}})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	for _, line := range strings.Split(res.Output, "\n") {
		if n := strings.TrimSpace(line); n != "" {
			return n
		}
	}
	return ""
}
