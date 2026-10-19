//go:build integration

package netscope

// Linux evidence for P7-TRUST-08 (Constitution 10.3): real jobs through the executor and the hardened
// container profile, a real internal network and a real proxy sidecar. A missing prerequisite is a failure,
// never a skip. The pull_request run of .github/workflows/ci-isolation-linux.yml is the proof for the head
// commit; macOS Docker Desktop evidence does not count.

import (
	"context"
	"net"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/container"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const lanPort = "9000"

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

// proxyBinary builds the static nself-ci the sidecar runs (the test binary has no netproxy key).
func proxyBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Fatalf("Linux required for network scope evidence, got %s", runtime.GOOS)
	}
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "netscope-bin")
		if err != nil {
			buildErr = err
			return
		}
		builtBin = filepath.Join(dir, "nself-ci")
		cmd := osexec.Command("go", "build", "-mod=vendor", "-o", builtBin, ".")
		cmd.Dir = "../.."
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &Error{Code: CodeScopeFailed, Message: "go build: " + err.Error() + "\n" + string(out)}
			return
		}
		buildErr = os.Chmod(builtBin, 0o755)
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return builtBin
}

func requireRuntime(t *testing.T, binary string) {
	t.Helper()
	if out, err := exec.RuntimeQuery(context.Background(), binary, "info"); err != nil {
		t.Fatalf("%s is required: %v\n%s", binary, err, out)
	}
}

type collected struct {
	mu     sync.Mutex
	events []Event
}

func (c *collected) add(_ string, e Event) {
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
}
func (c *collected) count(host string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, e := range c.events {
		if e.Host == host {
			n++
		}
	}
	return n
}
func (c *collected) dump() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.events...)
}

type rig struct {
	planner *Planner
	events  *collected
	profile *container.Profile
	image   string
}

func newRig(t *testing.T, excl Exclusions, extra []string) *rig {
	t.Helper()
	requireRuntime(t, "docker")
	image := os.Getenv("TRUST04_TEST_IMAGE")
	if image == "" {
		image = "alpine:3.19"
	}
	if _, err := exec.RuntimeQuery(context.Background(), "docker", "inspect", image); err != nil {
		t.Fatalf("image %s must be pulled first: %v", image, err)
	}
	bin := proxyBinary(t)
	ev := &collected{}
	pl := &Planner{Binary: func() (string, error) { return bin, nil }, Events: ev.add,
		Exclusion:  func(context.Context) (Exclusions, error) { return excl, nil },
		ExtraHosts: func(isolation.Attempt) []string { return extra }, Ready: 30 * time.Second}
	p := container.New(container.Options{Network: pl})
	if av := p.Probe(context.Background()); !av.Available() {
		t.Fatalf("container runtime required: %+v", av)
	}
	return &rig{planner: pl, events: ev, profile: p, image: image}
}

type wrapIsolator struct{ p *container.Profile }

func (w wrapIsolator) Prepare(_ context.Context, s *exec.JobSpec) error {
	wrapped, err := w.p.Wrap(*s)
	*s = wrapped
	return err
}

type capture struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (c *capture) write(b []byte) { c.mu.Lock(); c.buf.Write(b); c.mu.Unlock() }
func (c *capture) String() string { c.mu.Lock(); defer c.mu.Unlock(); return c.buf.String() }

func (r *rig) job(t *testing.T, id string, scope model.NetworkScope, env map[string]string, script string) (string, exec.AttemptResult) {
	t.Helper()
	out := &capture{}
	spec := exec.JobSpec{AttemptID: id, CoordinatorID: "trust08-coord", Home: t.TempDir(), Output: out.write, Command: []string{"sh", "-c", script},
		Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: r.image}, Env: env}, Network: scope, CPU: 1, MemoryMB: 128, Timeout: 120 * time.Second}
	e := &exec.Executor{Isolation: wrapIsolator{r.profile}, Grace: 2 * time.Second}
	res := e.Run(context.Background(), spec)
	t.Cleanup(func() { _ = exec.RuntimeRemoveAttempt(context.Background(), "docker", id) })
	return out.String(), res
}

// hostFixture listens on every host address; a connection it accepts means the job network reached the host.
func hostFixture(t *testing.T) (port string, addrs []string, accepted func() int) {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var mu sync.Mutex
	n := 0
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
			mu.Lock()
			n++
			mu.Unlock()
		}
	}()
	_, port, _ = net.SplitHostPort(ln.Addr().String())
	facts, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range facts.HostAddrs {
		if !a.IsLoopback() && a.Is4() {
			addrs = append(addrs, a.String())
		}
	}
	if len(addrs) == 0 {
		t.Fatal("the host has no non-loopback IPv4 address to test against")
	}
	return port, addrs, func() int { mu.Lock(); defer mu.Unlock(); return n }
}

// lanContainer starts a listener on the default bridge (a LAN-like neighbour) and returns its address.
func lanContainer(t *testing.T, image string) string {
	t.Helper()
	name := "nself-ci-t08-lan"
	_ = osexec.Command("docker", "rm", "-f", name).Run()
	out, err := osexec.Command("docker", "run", "-d", "--name", name, image, "sh", "-c", "while true; do echo hi | nc -l -p "+lanPort+"; done").CombinedOutput()
	if err != nil {
		t.Fatalf("lan container: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = osexec.Command("docker", "rm", "-f", name).Run() })
	ip, err := osexec.Command("docker", "inspect", "--format", "{{.NetworkSettings.IPAddress}}", name).Output()
	if err != nil || strings.TrimSpace(string(ip)) == "" {
		t.Fatalf("lan container address: %v %q", err, ip)
	}
	return strings.TrimSpace(string(ip))
}

// jobPrelude defines req (one request through the proxy, first response line) and the proxy address.
const jobPrelude = `
P=${HTTPS_PROXY#http://}; H=${P%:*}; PORT=${P##*:}
echo "PROXY=$P"
req() { printf "$1" | nc -w 8 "$H" "$PORT" | head -n1 | tr -d '\r'; }
connect() { req "CONNECT $1 HTTP/1.1\r\nHost: $1\r\n\r\n"; }
get() { req "GET http://$1/ HTTP/1.1\r\nHost: $1\r\nConnection: close\r\n\r\n"; }
direct() { if nc -w 3 "$1" "$2" </dev/null >/dev/null 2>&1; then echo "REACHED:$1:$2"; else echo "BLOCKED:$1:$2"; fi; }
`

func TestIntegrationRestrictedReachesAllowlistOnlyThroughTheProxy(t *testing.T) {
	r := newRig(t, Exclusions{}, []string{"10.0.0.1.nip.io"})
	lan := lanContainer(t, r.image)
	port, hostAddrs, accepted := hostFixture(t)
	script := jobPrelude + `
echo "GOLANG-CONNECT:$(connect proxy.golang.org:443)"
echo "GOLANG-GET:$(get proxy.golang.org)"
echo "REBIND:$(connect 10.0.0.1.nip.io:443)"
echo "NOTLISTED:$(connect example.org:443)"
echo "LAN-VIA-PROXY:$(connect "$LANIP:` + lanPort + `")"
echo "METADATA-VIA-PROXY:$(connect 169.254.169.254:80)"
echo "ULA-VIA-PROXY:$(connect "[fd00::1]:443")"
direct "$LANIP" ` + lanPort + `
direct 169.254.169.254 80
direct 1.1.1.1 443
for a in $HOSTADDRS; do direct "$a" "$HOSTPORT"; done
if nslookup example.com >/dev/null 2>&1; then echo DNS-RESOLVED; else echo DNS-FAILED; fi
`
	out, res := r.job(t, "t08-restricted", "restricted", map[string]string{"LANIP": lan, "HOSTADDRS": strings.Join(hostAddrs, " "), "HOSTPORT": port}, script)
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("%+v\n%s", res, out)
	}
	for _, want := range []string{"GOLANG-CONNECT:HTTP/1.1 200", "REBIND:HTTP/1.1 403", "NOTLISTED:HTTP/1.1 403", "LAN-VIA-PROXY:HTTP/1.1 403", "METADATA-VIA-PROXY:HTTP/1.1 403",
		"ULA-VIA-PROXY:HTTP/1.1 403", "BLOCKED:" + lan + ":" + lanPort, "BLOCKED:169.254.169.254:80", "BLOCKED:1.1.1.1:443", "DNS-FAILED"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if line := lineWith(out, "GOLANG-GET:"); !strings.Contains(line, "HTTP/1.1 2") && !strings.Contains(line, "HTTP/1.1 3") {
		t.Errorf("an allowlisted absolute-form GET did not get an upstream answer: %q", line)
	}
	for _, a := range hostAddrs {
		if !strings.Contains(out, "BLOCKED:"+a+":"+port) {
			t.Errorf("host address %s:%s was not blocked:\n%s", a, port, out)
		}
	}
	if strings.Contains(out, "REACHED:") || accepted() != 0 {
		t.Errorf("a direct connection got through (host fixture accepted %d):\n%s", accepted(), out)
	}
	// each denial is one structured event, and the resources are gone
	for _, host := range []string{"10.0.0.1.nip.io", "example.org", lan, "169.254.169.254", "fd00::1"} {
		if n := r.events.count(host); n != 1 {
			t.Errorf("%d denial events for %s, want 1: %+v", n, host, r.events.dump())
		}
	}
	assertNoLeftovers(t, "t08-restricted")
}

func TestIntegrationInternetAllowsPublicHostsAndKeepsThePrivateChecks(t *testing.T) {
	const exclusion = "93.184.216.34"
	r := newRig(t, Exclusions{Addresses: []string{exclusion}, Hosts: []string{"prod.example.org"}}, nil)
	lan := lanContainer(t, r.image)
	script := jobPrelude + `
echo "PUBLIC:$(connect 1.1.1.1:443)"
echo "ANYHOST:$(get example.net)"
echo "EXCLUDED-ADDR:$(connect ` + exclusion + `:443)"
echo "EXCLUDED-NAME:$(connect prod.example.org:443)"
echo "LAN:$(connect "$LANIP:` + lanPort + `")"
echo "MAPPED:$(connect "[::ffff:10.0.0.1]:443")"
echo "NAT64:$(connect "[64:ff9b::a00:1]:443")"
echo "6TO4:$(connect "[2002:a00:1::1]:443")"
echo "PRIVATE:$(connect 10.0.0.1:443)"
direct 1.1.1.1 443
`
	out, res := r.job(t, "t08-internet", "internet", map[string]string{"LANIP": lan}, script)
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("%+v\n%s", res, out)
	}
	for _, want := range []string{"PUBLIC:HTTP/1.1 200", "EXCLUDED-ADDR:HTTP/1.1 403", "EXCLUDED-NAME:HTTP/1.1 403", "LAN:HTTP/1.1 403", "MAPPED:HTTP/1.1 403",
		"NAT64:HTTP/1.1 403", "6TO4:HTTP/1.1 403", "PRIVATE:HTTP/1.1 403", "BLOCKED:1.1.1.1:443"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if line := lineWith(out, "ANYHOST:"); strings.Contains(line, "403") {
		t.Errorf("a public host was refused under internet: %q", line)
	}
	for _, host := range []string{exclusion, "prod.example.org", lan, "::ffff:10.0.0.1", "64:ff9b::a00:1", "2002:a00:1::1", "10.0.0.1"} {
		if n := r.events.count(host); n != 1 {
			t.Errorf("%d denial events for %s, want 1: %+v", n, host, r.events.dump())
		}
	}
	assertNoLeftovers(t, "t08-internet")
}

func TestIntegrationNoneHasNoInterfaceButLoopback(t *testing.T) {
	r := newRig(t, Exclusions{}, nil)
	out, res := r.job(t, "t08-none", "none", nil, `ls /sys/class/net | tr '\n' ' '; echo; env | grep -ci proxy; true`)
	if res.Err != nil || !strings.HasPrefix(out, "lo \n") {
		t.Fatalf("%+v\n%s", res, out)
	}
	if strings.Contains(out, "PROXY") {
		t.Errorf("proxy variables under none: %s", out)
	}
	assertNoLeftovers(t, "t08-none")
}

// The probe that proves a scope: on a real runner it yields restricted and internet.
func TestIntegrationProbeYieldsTheEnforcedScopes(t *testing.T) {
	r := newRig(t, Exclusions{}, nil)
	got := (&Prober{Planner: r.planner}).EnforcedScopes(context.Background(), "docker")
	if scopesOf(got) != "none,restricted,internet" {
		t.Fatalf("scopes %v", got)
	}
}

// The job network has no host address: a TCP connect to the host on a port where a fixture listens fails,
// under docker and under rootless podman.
func TestIntegrationHostPortUnreachableDocker(t *testing.T) { hostPortUnreachable(t, "docker") }
func TestIntegrationHostPortUnreachablePodman(t *testing.T) { hostPortUnreachable(t, "podman") }

func hostPortUnreachable(t *testing.T, binary string) {
	requireRuntime(t, binary)
	if binary == "podman" {
		out, err := exec.RuntimeQuery(context.Background(), "podman", "info", "--format", "{{.Host.Security.Rootless}}")
		if err != nil || strings.TrimSpace(string(out)) != "true" {
			t.Fatalf("rootless podman is required (rootless=%q, err=%v)", out, err)
		}
	}
	bin := proxyBinary(t)
	pl := &Planner{Binary: func() (string, error) { return bin, nil }, Ready: 45 * time.Second}
	id := "t08-hp-" + binary
	plan, err := pl.PlanAttempt(context.Background(), binary, isolation.Attempt{ID: id, CoordinatorID: "trust08-coord"}, "restricted")
	if plan.Release != nil {
		defer plan.Release(context.Background())
	}
	if err != nil {
		t.Fatalf("restricted scope on %s: %v", binary, err)
	}
	port, addrs, accepted := hostFixture(t)
	var targets []string
	for _, a := range addrs {
		targets = append(targets, net.JoinHostPort(a, port))
	}
	targets = append(targets, rawTargets...)
	out, err := pl.rt().ProbeRun(context.Background(), binary, id, ProxyImage, bin, targets)
	if err != nil {
		t.Fatalf("probe run: %v\n%s", err, out)
	}
	if !probeCompleted(out) || len(Reached(out)) != 0 || accepted() != 0 {
		t.Fatalf("job network reached something (fixture accepted %d):\n%s", accepted(), out)
	}
}

func lineWith(out, prefix string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

// assertNoLeftovers fails when a container or network labelled with the attempt survived the job.
func assertNoLeftovers(t *testing.T, id string) {
	t.Helper()
	time.Sleep(time.Second)
	for _, kind := range []exec.ResourceKind{exec.KindContainer, exec.KindNetwork} {
		list, err := exec.RuntimeListLabelled(context.Background(), "docker", kind, id)
		if err != nil || len(list) != 0 {
			t.Errorf("leftover %s for %s: %v %v", kind, id, list, err)
		}
	}
}
