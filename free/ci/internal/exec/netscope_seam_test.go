//go:build !windows

package exec

import (
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSidecarImage = "gcr.io/distroless/static@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3"

// fakeNetDocker puts a `docker` script first on PATH. It appends each call's argv (one line) to calls, copies
// stdin of a call that reads --env-file /dev/stdin to stdin.txt, and answers inspect queries.
func fakeNetDocker(t *testing.T) (calls func() []string, stdin func() string) {
	t.Helper()
	dir := t.TempDir()
	log, in := filepath.Join(dir, "calls.txt"), filepath.Join(dir, "stdin.txt")
	script := `#!/bin/sh
printf '%s\n' "$*" >> ` + log + `
case "$*" in
  *"--env-file /dev/stdin"*) cat > ` + in + ` ;;
esac
case "$1 $2" in
  "network inspect") echo '[{"Name":"n","IPAM":{"Config":[{"Subnet":"172.30.0.0/24"}]}}]' ;;
esac
case "$1" in
  inspect)
    case "$*" in
      *State.Running*) echo true ;;
      *) echo 172.30.0.2 ;;
    esac ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
	return func() []string { return strings.Split(strings.TrimSpace(read(log)), "\n") }, func() string { return read(in) }
}

func TestNetworkCreateIsInternalWithNoHostAddress(t *testing.T) {
	calls, _ := fakeNetDocker(t)
	prefix, err := RuntimeNetworkCreate(context.Background(), "docker", "a1", "c1")
	if err != nil || prefix != netip.MustParsePrefix("172.30.0.0/24") {
		t.Fatalf("%v %v", prefix, err)
	}
	create := calls()[0]
	for _, want := range []string{"network create", "--driver bridge", "--internal", "--ipv6=false", "com.docker.network.bridge.inhibit_ipv4=true",
		"nself.ci.attempt=a1", "role=ci-ephemeral", "coordinator_id=c1", "nself-ci-a1-net"} {
		if !strings.Contains(create, want) {
			t.Errorf("network create lacks %q: %s", want, create)
		}
	}
}

func TestSidecarIsHardenedAndTakesItsConfigOnStdin(t *testing.T) {
	calls, stdin := fakeNetDocker(t)
	bin := filepath.Join(t.TempDir(), "nself-ci")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := SidecarSpec{Attempt: "a1", Coordinator: "c1", Image: testSidecarImage, ProxyBinary: bin, Config: `{"scope":"restricted","secret":"never-on-argv"}`}
	ip, err := RuntimeSidecarStart(context.Background(), "docker", spec, netip.MustParsePrefix("172.30.0.0/24"))
	if err != nil || ip != netip.MustParseAddr("172.30.0.2") {
		t.Fatalf("%v %v", ip, err)
	}
	all := strings.Join(calls(), "\n")
	create := calls()[0]
	for _, want := range []string{"create --name nself-ci-a1-px", "--network bridge", "--cap-drop ALL", "no-new-privileges", "--read-only", "--user 65532:65532",
		"--env-file /dev/stdin", "readonly", testSidecarImage + " /nself-ci netproxy"} {
		if !strings.Contains(create, want) {
			t.Errorf("sidecar create lacks %q: %s", want, create)
		}
	}
	if strings.Contains(all, "never-on-argv") {
		t.Error("the proxy configuration reached an argv")
	}
	if got := stdin(); !strings.Contains(got, "NSELF_NETPROXY_CONFIG={") || !strings.Contains(got, "NSELF_V15=1") {
		t.Errorf("stdin %q", got)
	}
	// dual-homed before the first instruction: create, connect to the job network, then start
	order := []string{"create", "network connect nself-ci-a1-net nself-ci-a1-px", "start nself-ci-a1-px"}
	last := -1
	for _, want := range order {
		idx := strings.Index(all, want)
		if idx <= last {
			t.Fatalf("order of %q wrong in:\n%s", want, all)
		}
		last = idx
	}
}

func TestNetscopeSeamRefusesUnsafeArguments(t *testing.T) {
	fakeNetDocker(t)
	ctx := context.Background()
	dir := t.TempDir()
	bin := filepath.Join(dir, "nself-ci")
	_ = os.WriteFile(bin, []byte("x"), 0o755)
	secret := filepath.Join(dir, "private")
	_ = os.WriteFile(secret, []byte("x"), 0o700)
	good := SidecarSpec{Attempt: "a1", Image: testSidecarImage, ProxyBinary: bin, Config: "{}"}
	with := func(f func(*SidecarSpec)) SidecarSpec { s := good; f(&s); return s }
	pfx := netip.MustParsePrefix("172.30.0.0/24")
	for name, call := range map[string]func() error{
		"create other binary":   func() error { _, err := RuntimeNetworkCreate(ctx, "sh", "a1", "c1"); return err },
		"create option attempt": func() error { _, err := RuntimeNetworkCreate(ctx, "docker", "--all", "c1"); return err },
		"sidecar tag image": func() error {
			_, err := RuntimeSidecarStart(ctx, "docker", with(func(s *SidecarSpec) { s.Image = "gcr.io/distroless/static:nonroot" }), pfx)
			return err
		},
		"sidecar dotdot image": func() error {
			_, err := RuntimeSidecarStart(ctx, "docker", with(func(s *SidecarSpec) { s.Image = "../x@sha256:" + strings.Repeat("a", 64) }), pfx)
			return err
		},
		"sidecar relative bin": func() error {
			_, err := RuntimeSidecarStart(ctx, "docker", with(func(s *SidecarSpec) { s.ProxyBinary = "nself-ci" }), pfx)
			return err
		},
		"sidecar comma bin": func() error {
			_, err := RuntimeSidecarStart(ctx, "docker", with(func(s *SidecarSpec) { s.ProxyBinary = dir + "/a,b" }), pfx)
			return err
		},
		"sidecar unreadable bin": func() error {
			_, err := RuntimeSidecarStart(ctx, "docker", with(func(s *SidecarSpec) { s.ProxyBinary = secret }), pfx)
			return err
		},
		"sidecar newline config": func() error {
			_, err := RuntimeSidecarStart(ctx, "docker", with(func(s *SidecarSpec) { s.Config = "a\nb=c" }), pfx)
			return err
		},
		"sidecar empty config": func() error {
			_, err := RuntimeSidecarStart(ctx, "docker", with(func(s *SidecarSpec) { s.Config = "" }), pfx)
			return err
		},
		"probe bad target": func() error {
			_, err := RuntimeNetProbeRun(ctx, "docker", "a1", testSidecarImage, bin, "172.30.0.2:3128", []string{"--privileged"})
			return err
		},
		"probe no target": func() error {
			_, err := RuntimeNetProbeRun(ctx, "docker", "a1", testSidecarImage, bin, "172.30.0.2:3128", nil)
			return err
		},
		"remove option attempt": func() error { return RuntimeRemoveAttempt(ctx, "docker", "-f") },
	} {
		var e *Error
		if err := call(); err == nil || !asError(err, &e) || e.Code != "E613" {
			t.Errorf("%s: want E613, got %v", name, err)
		}
	}
}

func TestNetProbeRunIsOnTheJobNetworkWithoutDNSOrCapabilities(t *testing.T) {
	calls, _ := fakeNetDocker(t)
	bin := filepath.Join(t.TempDir(), "nself-ci")
	_ = os.WriteFile(bin, []byte("x"), 0o755)
	if _, err := RuntimeNetProbeRun(context.Background(), "docker", "a1", testSidecarImage, bin, "172.30.0.2:3128", []string{"198.41.0.4:4000", "[2a00::1]:80"}); err != nil {
		t.Fatal(err)
	}
	run := calls()[0]
	for _, want := range []string{"run --rm", "--network nself-ci-a1-net", "--dns 127.0.0.1", "--cap-drop ALL", "--read-only", "netproxy --probe --proxy 172.30.0.2:3128 198.41.0.4:4000 [2a00::1]:80"} {
		if !strings.Contains(run, want) {
			t.Errorf("probe run lacks %q: %s", want, run)
		}
	}
}

func TestSubnetsInReadsDockerAndPodmanLayouts(t *testing.T) {
	for name, doc := range map[string]string{
		"docker": `[{"IPAM":{"Config":[{"Subnet":"172.30.0.0/24","Gateway":"172.30.0.1"}]}}]`,
		"podman": `[{"name":"n","subnets":[{"subnet":"10.89.1.0/24","gateway":"10.89.1.1"}]}]`,
	} {
		var v any
		if err := json.Unmarshal([]byte(doc), &v); err != nil {
			t.Fatal(err)
		}
		if got := subnetsIn(v); len(got) != 1 || !strings.Contains(got[0], "/24") {
			t.Errorf("%s: %v", name, got)
		}
	}
}
