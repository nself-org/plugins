package deployguard

// Purpose: the start guard. The agent (serve, join, pull) and `ci serve` refuse to start on a host that is
// a deploy host: its local identity is in the exclusion set, or a running compose project holds a
// production marker.
// Inputs:  Local (the seams that read this host) and the exclusion set.
// Outputs: nil, or E663 with reason matched or prod_marker; warnings for what could not be read.
// Constraints: local identity = hostname, interface addresses, /etc/ssh/ssh_host_*_key.pub fingerprints
//          and /etc/machine-id; the marker is a running compose project whose
//          `com.docker.compose.project.working_dir` holds `.env` or `.env.prod` with ENV=prod or
//          production; Docker absent skips the marker check, Docker present but unreadable is a warning
//          and the identity check still applies; no network.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const composeDirLabel = "com.docker.compose.project.working_dir"

// Local reads this host. Every field is a seam; NewLocal wires the real ones.
type Local struct {
	Hostname   func() (string, error)
	Addrs      func() ([]netip.Addr, error)
	ReadFile   func(path string) ([]byte, error)
	Glob       func(pattern string) ([]string, error)
	Run        RunFunc
	MachineIDs []string
	HostKeys   string
}

// NewLocal wires the real host seams; run is the docker runner (FunnelRun in production).
func NewLocal(run RunFunc) Local {
	return Local{Hostname: os.Hostname, Addrs: interfaceAddrs, ReadFile: readSmall, Glob: filepath.Glob, Run: run,
		MachineIDs: []string{"/etc/machine-id", "/var/lib/dbus/machine-id"}, HostKeys: "/etc/ssh/ssh_host_*_key.pub"}
}

func readSmall(p string) ([]byte, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if fi.Size() > 1<<20 {
		return nil, os.ErrInvalid
	}
	return os.ReadFile(p)
}

func interfaceAddrs() ([]netip.Addr, error) { return addrsFrom(net.InterfaceAddrs) }

// addrsFrom reads the interface addresses through list (net.InterfaceAddrs in production).
func addrsFrom(list func() ([]net.Addr, error)) ([]netip.Addr, error) {
	as, err := list()
	if err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, a := range as {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			out = append(out, p.Addr().Unmap())
		}
	}
	return out, nil
}

// Facts reads the identity of this host.
func (l Local) Facts() (Facts, []string) {
	var f Facts
	var warn []string
	if l.Hostname != nil {
		if h, err := l.Hostname(); err == nil {
			f.Names = append(f.Names, Canon(h))
		}
	}
	if l.Addrs != nil {
		if as, err := l.Addrs(); err == nil {
			f.Addrs = as
		} else {
			warn = append(warn, "interface addresses unreadable")
		}
	}
	if l.Glob != nil && l.ReadFile != nil && l.HostKeys != "" {
		paths, _ := l.Glob(l.HostKeys)
		sort.Strings(paths)
		for _, p := range paths {
			if b, err := l.ReadFile(p); err == nil {
				if fp := PublicKeyFingerprint(string(b)); fp != "" {
					f.Fingerprints = append(f.Fingerprints, fp)
				}
			}
		}
	}
	if l.ReadFile != nil {
		for _, p := range l.MachineIDs {
			if b, err := l.ReadFile(p); err == nil {
				if m := NormalizeMachineID(string(b)); m != "" {
					f.MachineIDs = append(f.MachineIDs, m)
				}
			}
		}
	}
	return f, warn
}

// PublicKeyFingerprint is the SHA256:<base64> fingerprint of an OpenSSH public key line.
func PublicKeyFingerprint(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ""
	}
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil || len(blob) == 0 {
		return ""
	}
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// Check runs the identity check and then the marker check for command (serve, join, pull or ci serve).
func (l Local) Check(ctx context.Context, s Set, command string) (warnings []string, err error) {
	f, warnings := l.Facts()
	if r := s.Match(f); r.Matched {
		return warnings, newErr(ReasonMatched, "this host is a deploy host (%s %s); %s refuses to start here", r.Source, r.Kind, command)
	}
	dir, mw, err := l.marker(ctx)
	warnings = append(warnings, mw...)
	if err != nil {
		return warnings, err
	}
	if dir != "" {
		return warnings, newErr(ReasonMarker, "a running compose project in %s is a production stack; %s refuses to start here", dir, command)
	}
	return warnings, nil
}

// marker returns the working dir of a running production compose project, if any.
func (l Local) marker(ctx context.Context) (string, []string, error) {
	if l.Run == nil {
		return "", nil, nil
	}
	if !onPath("docker") {
		return "", nil, nil // docker is not installed: the marker check is skipped
	}
	out, exit, err := l.Run(ctx, "", "docker", "ps", "--format", `{{.Label "`+composeDirLabel+`"}}`)
	if err != nil || exit != 0 {
		return "", []string{"docker is installed but not readable; the production marker check was skipped"}, nil
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		d := strings.TrimSpace(line)
		if d == "" || seen[d] || !filepath.IsAbs(d) {
			continue
		}
		seen[d] = true
		if l.prodEnv(d) {
			return d, nil, nil
		}
	}
	return "", nil, nil
}

func (l Local) prodEnv(dir string) bool {
	if l.ReadFile == nil {
		return false
	}
	for _, name := range []string{".env", ".env.prod"} {
		b, err := l.ReadFile(filepath.Join(dir, name))
		if err == nil && envSaysProd(string(b)) {
			return true
		}
	}
	return false
}

// envSaysProd reports an ENV=prod or ENV=production line (export prefix and quotes allowed).
func envSaysProd(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		l := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		k, v, ok := strings.Cut(l, "=")
		if !ok || strings.TrimSpace(k) != "ENV" {
			continue
		}
		v, _, _ = strings.Cut(v, " #")
		v = strings.ToLower(strings.Trim(strings.TrimSpace(v), `"'`))
		if v == "prod" || v == "production" {
			return true
		}
	}
	return false
}

// onPath reports whether name is an executable file in a PATH directory.
func onPath(name string) bool {
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if fi, err := os.Stat(filepath.Join(d, name)); err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0 {
			return true
		}
	}
	return false
}

// StartGuard is the guard the agent's PreStart hook and `ci serve` call. It builds the local exclusion
// set (the node's own state.db when present, plus the user never-list) and checks this host against it.
// A source that exists but cannot be read refuses the start (LocalSet); a missing one is empty.
func StartGuard(ctx context.Context, home string, l Local, command string) (warnings []string, err error) {
	s, err := LocalSet(ctx, home)
	if err != nil {
		return nil, err
	}
	return l.Check(ctx, s, command)
}
