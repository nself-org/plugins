//go:build security

package redteam

// Purpose: red team group C, SSH carrier option forcing (TM-40). The attacker controls what a node
// record can carry (host name, alias, user, port) and tries to make the carrier run ssh with agent
// forwarding, multiplexing, a proxy or local command, a weaker host-key policy, another config file
// or a destination that ssh reads as an option. The checks under test are the production host-spec
// parser and the production argv allowlist in the vendored SDK (remote.RunArgv refuses before it
// starts a process when an option or destination is not allowed).
// Inputs:  node-record-shaped strings, option slices built the way internal/transport/ssh builds them.
// Outputs: pass (every forced option is refused before ssh starts) or fail.
// Constraints: loopback destination, 3 s timeout; a refusal is recognised by the SDK's own wording,
//          so a case that slips through to a real ssh start fails the test instead of passing silently.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

var cSSHVersion = remote.Version{Major: 9, Minor: 6}

// cRefused reports whether err is a refusal made before any process started.
func cRefused(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, w := range []string{"not allowed", "unsafe character", "must not start", "contains ':'", "newline or NUL", "is empty", "longer than", "needs a plain path", "needs a port"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func cCarrier(t *testing.T, pin string) []string {
	t.Helper()
	return append(remote.CISSHFlags(), remote.CIOptions("mac1", pin, cSSHVersion)...)
}

// TestC40ForcedOptionSet: the option block the carrier builds pins every dangerous knob, and the
// allowlist refuses each way to override, extend or replace that block.
func TestC40ForcedOptionSet(t *testing.T) {
	pin := t.TempDir() + "/known_hosts"
	base := cCarrier(t, pin)
	joined := " " + strings.Join(base, " ") + " "
	for _, want := range []string{" -a ", " -T ", " -x ", "ForwardAgent=no", "ForwardX11=no", "ClearAllForwardings=yes", "ControlMaster=no", "ControlPath=none", "StrictHostKeyChecking=yes",
		"PermitLocalCommand=no", "RemoteCommand=none", "BatchMode=yes", "UpdateHostKeys=no", "VerifyHostKeyDNS=no", "GlobalKnownHostsFile=/dev/null", "HostKeyAlias=nself-ci-mac1", "UserKnownHostsFile=" + pin} {
		if !strings.Contains(joined, want) {
			t.Errorf("the forced option set lacks %q", want)
		}
	}
	run := func(opts []string, dest string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err := remote.RunArgv(ctx, remote.Target{Dest: dest, Options: opts}, "true")
		return err
	}
	if err := run(append(append([]string{}, base...), "-o", "ConnectTimeout=1"), "127.0.0.1"); cRefused(err) {
		t.Fatalf("control: the honest option set was refused: %v", err)
	}
	ext := func(extra ...string) []string { return append(append([]string{}, base...), extra...) }
	if err := run(ext("-o", "IdentityFile=/tmp/k", "-o", "User=deploy", "-i", "/tmp/k", "-p", "22", "-o", "ConnectTimeout=1"), "127.0.0.1"); cRefused(err) {
		t.Fatalf("control: allowed keys with plain values were refused: %v", err)
	}
	for name, opts := range map[string][]string{
		"agent forwarding":                ext("-o", "ForwardAgent=yes"),
		"-A":                              ext("-A"),
		"multiplexing":                    ext("-o", "ControlMaster=auto"),
		"control path":                    ext("-o", "ControlPath=/tmp/cm-%r@%h"),
		"proxy command":                   ext("-o", "ProxyCommand=touch /tmp/pwned"),
		"proxy command, glued":            ext("-oProxyCommand=touch /tmp/pwned"),
		"local command":                   ext("-o", "PermitLocalCommand=yes", "-o", "LocalCommand=id"),
		"jump host":                       ext("-J", "attacker@evil"),
		"other config file":               ext("-F", "/tmp/evil_config"),
		"include":                         ext("-o", "Include=/tmp/evil"),
		"port forward":                    ext("-L", "8080:127.0.0.1:80"),
		"remote forward":                  ext("-R", "9000:127.0.0.1:22"),
		"strict checking off":             ext("-o", "StrictHostKeyChecking=no"),
		"host key algorithms":             ext("-o", "HostKeyAlgorithms=ssh-rsa"),
		"known hosts command":             ext("-o", "KnownHostsCommand=/tmp/x"),
		"option key case and spacing":     ext("-o", "proxycommand   =  touch"),
		"option separated by tab":         ext("-o", "ProxyCommand\ttouch"),
		"newline-joined second option":    ext("-o", "Port=22\nProxyCommand=touch /tmp/x"),
		"NUL in option":                   ext("-o", "Port=22\x00ProxyCommand=x"),
		"allowed key, value with a space": ext("-o", "IdentityFile=a b"),
		"allowed key, value with a quote": ext("-o", "IdentityFile=a'b"),
		"allowed key, option-like value":  ext("-o", "User=-oProxyCommand=x"),
		"allowed key, glued, spaced":      ext("-oIdentityFile=a b"),
		"-i with a space":                 ext("-i", "a b"),
		"-i option-like":                  ext("-i", "-oProxyCommand=x"),
		"numeric key with a command":      ext("-o", "Port=22;id"),
		"lone port with a newline":        ext("-o", "Port=22\n"),
		"pin file is /dev/null":           append(remote.CISSHFlags(), remote.CIOptions("mac1", "/dev/null", cSSHVersion)...),
		"pin file with token":             append(remote.CISSHFlags(), remote.CIOptions("mac1", "%d/known_hosts", cSSHVersion)...),
		"pin file with tilde":             append(remote.CISSHFlags(), remote.CIOptions("mac1", "~/known_hosts", cSSHVersion)...),
		"pin file with env":               append(remote.CISSHFlags(), remote.CIOptions("mac1", "$HOME/known_hosts", cSSHVersion)...),
		"pin file with space":             append(remote.CISSHFlags(), remote.CIOptions("mac1", "/tmp/a b/known_hosts", cSSHVersion)...),
		"pin file option-like":            append(remote.CISSHFlags(), remote.CIOptions("mac1", "-oProxyCommand=x", cSSHVersion)...),
		"host key alias of another id":    append(remote.CISSHFlags(), remote.CIOptions("../mac1", pin, cSSHVersion)...),
		"block reordered":                 append([]string{"-o", "StrictHostKeyChecking=no"}, base...),
		"block missing one knob":          append([]string{}, base[:len(base)-4]...),
	} {
		if err := run(opts, "127.0.0.1"); !cRefused(err) {
			t.Errorf("%s: the carrier did not refuse before starting ssh (err: %v)", name, err)
		}
	}
	for _, dest := range []string{"-oProxyCommand=touch", "-J", "host;id", "a b", "user@host\n", "host::module", "ho$st", "`id`", "host\x00x", "$(id)", "user@-oProxyCommand=x", "h'ost", "host|id", "--", "-",
		"user@host:/etc", "ho­st", strings.Repeat("a", 400)} {
		if err := run(base, dest); !cRefused(err) {
			t.Errorf("destination %q was not refused before starting ssh (err: %v)", dest, err)
		}
	}
}

// TestC40HostSpecs: the parser behind the carrier destination refuses everything that is not
// [user@]host[:port] or a bracketed IPv6, and a legacy ":/path" suffix is parsed out so the
// carrier (which refuses it) can see it.
func TestC40HostSpecs(t *testing.T) {
	for _, s := range []string{"", "-oProxyCommand=x", "user@-oProxyCommand=x", "host -oProxyCommand=x", "a;b", "$(id)", "`id`", "host\n", "ho st", "@host", "u@@host", "host:99999", "host:-1", "host:0", "host:",
		"UPPER@host", "1user@host", "us er@host", "host:/etc/passwd", "[host]:22", "[::1", "[::1]x", "::1", "ssh://host", "host/../x", "höst", "host%eth0", "a@b@c"} {
		spec, err := remote.ParseHostSpec(s)
		if err == nil && spec.LegacyPath == "" {
			t.Errorf("%q parsed as a plain destination: %+v", s, spec)
		}
	}
	if spec, err := remote.ParseHostSpec("deploy@ci-1.example.org:2222"); err != nil || spec.Port != 2222 || spec.User != "deploy" {
		t.Fatalf("control: %+v %v", spec, err)
	}
	spec, _ := remote.ParseHostSpec("ci-1.example.org:2222")
	if _, err := spec.Target(append(cCarrier(t, t.TempDir()+"/k"), "-o", "Port=22")); err == nil {
		t.Error("a second port option next to the spec port was accepted")
	}
}
