package exec

// Purpose: the runtime CLI work of one attempt's network scope (P7-TRUST-08, Epic D11), in the one package
// that may start processes: an `--internal` network with no host address, the dual-homed proxy sidecar and
// the probe container that proves the scope.
// Inputs:  a runtime binary (docker or podman), an attempt id, a digest-pinned image, the host path of the
// static nself-ci binary and the proxy configuration (an environment value, fed through stdin).
// Outputs: the job network's subnet, the sidecar's address on it, the sidecar log and the probe output.
// Constraints: every argv is built here from fixed words and validated values; the sidecar is hardened like a
// job container (no capability, read-only root, small limits, a non-root user); the configuration reaches
// the sidecar through `--env-file /dev/stdin`, never argv; nothing here starts a job or opens a socket.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// NetworkSuffix and SidecarSuffix name the per-attempt network and proxy container.
	NetworkSuffix = "-net"
	SidecarSuffix = "-px"
	// sidecarMountDst is where the static binary appears inside the sidecar image.
	sidecarMountDst = "/nself-ci"
	// sidecarUID is the non-root user of the minimal image.
	sidecarUID = "65532:65532"
	// netSetupTimeout bounds one setup step.
	netSetupTimeout = 60 * time.Second
	maxSidecarLog   = 1 << 20 // 1000 events of at most ~500 bytes fit (netscope.maxEvents)
)

var safePath = regexp.MustCompile(`^/[A-Za-z0-9._/+-]{1,1000}$`)

// NetworkName is the per-attempt internal network of an attempt.
func NetworkName(attempt string) string { return "nself-ci-" + attempt + NetworkSuffix }

// SidecarName is the per-attempt proxy container of an attempt.
func SidecarName(attempt string) string { return "nself-ci-" + attempt + SidecarSuffix }

// SidecarSpec is what the runtime needs to start the proxy sidecar.
type SidecarSpec struct {
	Attempt, Coordinator string
	// Image is the minimal image, pinned by digest.
	Image string
	// ProxyBinary is the absolute host path of the static nself-ci binary (bound read-only).
	ProxyBinary string
	// Config is the encoded proxy configuration.
	Config string
}

func netRefused(what string) error { return coded("E613", "network scope runtime call refused: "+what) }

// checkNetArgs validates the values every seam call puts in argv.
func checkNetArgs(binary, attempt string) error {
	if !runtimeBinary.MatchString(binary) {
		return netRefused("unknown runtime")
	}
	if !safeID.MatchString(attempt) {
		return netRefused("invalid attempt id")
	}
	return nil
}

// attemptLabels are the labels every network-scope resource carries (the orphan sweep finds them by them).
func attemptLabels(attempt, coordinator string) []string {
	l := []string{"--label", AttemptLabel + "=" + attempt, "--label", "role=ci-ephemeral"}
	if safeID.MatchString(coordinator) {
		l = append(l, "--label", "coordinator_id="+coordinator)
	}
	return l
}

// runNet runs one runtime command with a time box and returns its combined, capped output.
func runNet(ctx context.Context, binary string, stdin io.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, netSetupTimeout)
	defer cancel()
	cmd := runtimeCommand(ctx, binary, args)
	cmd.Env = dockerHostEnv()
	cmd.Stdin = stdin
	var out bytes.Buffer
	cmd.Stdout = &limitWriter{w: &out, max: maxSidecarLog}
	cmd.Stderr = cmd.Stdout
	err := cmd.Run()
	if err != nil {
		return out.Bytes(), runtimeFailed(err)
	}
	return out.Bytes(), nil
}

type limitWriter struct {
	w   *bytes.Buffer
	max int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if room := l.max - l.w.Len(); room > 0 {
		l.w.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// RuntimeNetworkCreate creates the attempt's internal network and returns its subnet. On docker the bridge
// gets no host address (inhibit_ipv4), so no host service is reachable from it; podman's internal networks
// have no gateway, and the probe is what proves either claim.
func RuntimeNetworkCreate(ctx context.Context, binary, attempt, coordinator string) (netip.Prefix, error) {
	if err := checkNetArgs(binary, attempt); err != nil {
		return netip.Prefix{}, err
	}
	name := NetworkName(attempt)
	args := []string{"network", "create", "--driver", "bridge", "--internal", "--ipv6=false"}
	if binary == "docker" {
		args = append(args, "--opt", "com.docker.network.bridge.inhibit_ipv4=true")
	}
	args = append(append(args, attemptLabels(attempt, coordinator)...), name)
	if out, err := runNet(ctx, binary, nil, args...); err != nil {
		return netip.Prefix{}, netFailed("network create", out, err)
	}
	// docker lists IPAM.Config[].Subnet, podman Subnets[].subnet: read the JSON both print, whatever the layout
	out, err := runNet(ctx, binary, nil, "network", "inspect", name)
	if err != nil {
		removeNetworkQuietly(ctx, binary, name)
		return netip.Prefix{}, netFailed("network inspect", out, err)
	}
	var doc any
	if json.Unmarshal(out, &doc) == nil {
		for _, f := range subnetsIn(doc) {
			if p, err := netip.ParsePrefix(f); err == nil && p.Addr().Is4() {
				return p, nil
			}
		}
	}
	removeNetworkQuietly(ctx, binary, name)
	return netip.Prefix{}, coded("E613", "the job network has no IPv4 subnet")
}

// removeNetworkQuietly removes a network this call created and could not use (the caller has nothing to release).
func removeNetworkQuietly(ctx context.Context, binary, name string) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), netSetupTimeout)
	defer cancel()
	_, _ = runNet(rctx, binary, nil, "network", "rm", name)
}

// subnetsIn collects the string values of every "Subnet"/"subnet" key in a decoded inspect document.
func subnetsIn(v any) []string {
	var out []string
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			out = append(out, subnetsIn(e)...)
		}
	case map[string]any:
		for k, e := range x {
			if str, ok := e.(string); ok && strings.EqualFold(k, "subnet") {
				out = append(out, str)
				continue
			}
			out = append(out, subnetsIn(e)...)
		}
	}
	return out
}

// RuntimeSidecarStart creates the proxy sidecar on the runtime's normal bridge, joins it to the job network
// and starts it, so it is dual-homed before its first instruction. It returns the sidecar's address on the
// job network, which becomes the job's proxy address.
func RuntimeSidecarStart(ctx context.Context, binary string, s SidecarSpec, job netip.Prefix) (netip.Addr, error) {
	if err := checkNetArgs(binary, s.Attempt); err != nil {
		return netip.Addr{}, err
	}
	if !probeImageRef.MatchString(s.Image) || strings.Contains(s.Image, "..") {
		return netip.Addr{}, netRefused("sidecar image must be pinned by digest")
	}
	if !safePath.MatchString(s.ProxyBinary) || filepath.Clean(s.ProxyBinary) != s.ProxyBinary {
		return netip.Addr{}, netRefused("proxy binary path")
	}
	if fi, err := os.Stat(s.ProxyBinary); err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0005 != 0005 {
		return netip.Addr{}, coded("E613", "the proxy binary must be a regular file readable and executable by the sidecar user")
	}
	if strings.ContainsAny(s.Config, "\r\n\x00") || s.Config == "" {
		return netip.Addr{}, netRefused("proxy configuration")
	}
	name, jobNet := SidecarName(s.Attempt), NetworkName(s.Attempt)
	args := []string{"create", "--name", name, "--network", "bridge", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--read-only", "--log-driver", "json-file", "--log-opt", "max-size=4m", "--log-opt", "max-file=2", "--pids-limit", "256", "--memory", "128m", "--memory-swap", "128m", "--cpus", "0.5", "--user", sidecarUID,
		"--env-file", "/dev/stdin", "--mount", "type=bind,src=" + s.ProxyBinary + ",dst=" + sidecarMountDst + ",readonly"}
	args = append(append(args, attemptLabels(s.Attempt, s.Coordinator)...), s.Image, sidecarMountDst, "netproxy")
	env := strings.NewReader("NSELF_V15=1\nNSELF_NETPROXY_CONFIG=" + s.Config + "\n") // V15 selects the engine that has the hidden netproxy key
	if out, err := runNet(ctx, binary, env, args...); err != nil {
		return netip.Addr{}, netFailed("sidecar create", out, err)
	}
	if out, err := runNet(ctx, binary, nil, "network", "connect", jobNet, name); err != nil {
		return netip.Addr{}, netFailed("sidecar connect", out, err)
	}
	if out, err := runNet(ctx, binary, nil, "start", name); err != nil {
		return netip.Addr{}, netFailed("sidecar start", out, err)
	}
	out, err := runNet(ctx, binary, nil, "inspect", "--format", `{{(index .NetworkSettings.Networks "`+jobNet+`").IPAddress}}`, name)
	if err != nil {
		return netip.Addr{}, netFailed("sidecar inspect", out, err)
	}
	ip, perr := netip.ParseAddr(strings.TrimSpace(string(out)))
	if perr != nil || !job.Contains(ip) {
		return netip.Addr{}, coded("E613", "the sidecar has no address in the job network")
	}
	return ip, nil
}

// RuntimeSidecarLog returns the sidecar's stdout and stderr (capped): the readiness line and the denial events.
func RuntimeSidecarLog(ctx context.Context, binary, attempt string) ([]byte, error) {
	if err := checkNetArgs(binary, attempt); err != nil {
		return nil, err
	}
	return runNet(ctx, binary, nil, "logs", SidecarName(attempt))
}

// RuntimeSidecarRunning reports whether the attempt's sidecar is still up.
func RuntimeSidecarRunning(ctx context.Context, binary, attempt string) (bool, error) {
	if err := checkNetArgs(binary, attempt); err != nil {
		return false, err
	}
	out, err := runNet(ctx, binary, nil, "inspect", "--format", "{{.State.Running}}", SidecarName(attempt))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// RuntimeNetProbeRun runs the probe container on the attempt's job network: the static binary's `netproxy
// --probe` mode dials every target and prints the ones it reached, asks the proxy to open a tunnel to the
// metadata address (it must refuse) and tries one DNS lookup (it must fail). The container has no capability, a
// read-only root and the job's own network settings (internal network, no DNS), so what it reaches is what a
// job could reach. Targets are host:port literals.
func RuntimeNetProbeRun(ctx context.Context, binary, attempt, image, proxyBinary, proxy string, targets []string) ([]byte, error) {
	if err := checkNetArgs(binary, attempt); err != nil {
		return nil, err
	}
	if !probeImageRef.MatchString(image) || !safePath.MatchString(proxyBinary) || len(targets) == 0 || len(targets) > 64 {
		return nil, netRefused("probe arguments")
	}
	for _, t := range targets {
		if ap, err := netip.ParseAddrPort(t); err != nil || !ap.IsValid() {
			return nil, netRefused("probe target " + t)
		}
	}
	if ap, err := netip.ParseAddrPort(proxy); err != nil || !ap.IsValid() {
		return nil, netRefused("probe proxy " + proxy)
	}
	name := "nself-ci-" + attempt + "-probe-" + strconv.FormatInt(time.Now().UnixNano(), 36) // one name per run: chunks follow each other
	args := []string{"run", "--rm", "--name", name, "--network", NetworkName(attempt), "--dns", "127.0.0.1", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--read-only", "--pids-limit", "64", "--memory", "64m", "--memory-swap", "64m",
		"--cpus", "0.5", "--user", sidecarUID, "--label", AttemptLabel + "=" + attempt, "--label", "nself.ci.probe=1", "--env", "NSELF_V15=1",
		"--mount", "type=bind,src=" + proxyBinary + ",dst=" + sidecarMountDst + ",readonly", image, sidecarMountDst, "netproxy", "--probe", "--proxy", proxy}
	out, err := runNet(ctx, binary, nil, append(args, targets...)...)
	if err != nil {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), queryTimeout)
		_ = RuntimeRemove(rctx, binary, name)
		cancel()
		return out, netFailed("probe run", out, err)
	}
	return out, nil
}

// RuntimeRemoveAttempt removes every container and then every network labelled with the attempt: the job
// network's teardown. It is idempotent and reports the first failure after trying everything.
func RuntimeRemoveAttempt(ctx context.Context, binary, attempt string) error {
	if err := checkNetArgs(binary, attempt); err != nil {
		return err
	}
	var first error
	for _, kind := range []ResourceKind{KindContainer, KindNetwork} {
		list, err := RuntimeListLabelled(ctx, binary, kind, attempt)
		if err != nil && first == nil {
			first = err
		}
		for _, r := range list {
			if err := RuntimeRemoveLabelled(ctx, binary, kind, r.ID, attempt); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

// netFailed wraps a runtime failure with the first line of its output.
func netFailed(step string, out []byte, err error) error {
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if len(line) > 200 {
		line = line[:200]
	}
	return &Error{Code: "E613", Message: "network scope " + step + ": " + line, Err: err}
}
