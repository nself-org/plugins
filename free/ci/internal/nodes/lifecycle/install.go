package lifecycle

// Purpose: the steps of `nodes add` that touch the node before it is registered: the host-key
// confirmation and pin, the platform probe, the agent installation and the version check.
// Inputs:  the captured keys, the admin's `yes` or exact --host-key, the agent source.
// Outputs: pinned keys; an installed agent of a known version.
// Constraints: a key is pinned only after an interactive exact `yes` or an --host-key equal to a captured
//          fingerprint; no terminal and no flag refuses with E655 and pins nothing; nSelf never edits
//          the node's authorized_keys (ADR 0023); every remote command is a fixed argv; ssh/scp error
//          text is cut to one short, control-free line before it reaches a message.

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// DefaultInstaller is the installer `nodes add` uses without --installer.
const DefaultInstaller = "local-file"

func init() { RegisterAgentInstaller(DefaultInstaller, localFileInstaller) }

// transportErr is an E656 whose text carries one short line of the underlying error.
func transportErr(reason, what string, err error) *Error {
	return newErr(CodeNoRoute, reason, "%s: %s", what, oneLine(err))
}

func oneLine(err error) string {
	if err == nil {
		return ""
	}
	s := strings.SplitN(err.Error(), "\n", 2)[0]
	var b strings.Builder
	for _, r := range s {
		if r >= ' ' && r != 0x7f {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 160 {
		out = out[:160]
	}
	return out
}

// keyLine returns "<type> <base64>" of a captured key.
func keyLine(k remote.HostKey) string {
	f := strings.Fields(k.Line)
	if len(f) < 2 {
		return ""
	}
	return f[len(f)-2] + " " + f[len(f)-1]
}

// confirmKeys picks the keys to pin: the one --host-key names, or all captured keys after an
// interactive `yes`.
func (d *Deps) confirmKeys(id, dest string, keys []remote.HostKey, want string) ([]remote.HostKey, error) {
	if want != "" {
		for _, k := range keys {
			if k.Fingerprint == want {
				return []remote.HostKey{k}, nil
			}
		}
		return nil, newErr(CodeHostKey, ReasonKeyMismatch, "none of the host keys of %s matches --host-key; found: %s", dest, fingerprints(keys))
	}
	if !d.TTY {
		return nil, newErr(CodeHostKey, ReasonNoTTY, "no terminal and no --host-key: nothing pinned for %s; found: %s; re-run with --host-key SHA256:<fingerprint>", dest, fingerprints(keys))
	}
	_, _ = fmt.Fprintf(d.Stdout, "Host key fingerprints of %s:\n", dest)
	for _, k := range keys {
		_, _ = fmt.Fprintf(d.Stdout, "  %s (%s)\n", k.Fingerprint, k.Type)
	}
	_, _ = fmt.Fprintf(d.Stdout, "Pin these keys for node %s? Type yes to continue: ", id)
	line, _ := bufio.NewReader(d.Stdin).ReadString('\n')
	if strings.TrimSpace(line) != "yes" {
		return nil, newErr(CodeHostKey, ReasonDeclined, "host key not confirmed: nothing pinned for %s", dest)
	}
	return keys, nil
}

func fingerprints(keys []remote.HostKey) string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.Fingerprint)
	}
	return strings.Join(out, ", ")
}

// pin writes the chosen keys. A pin that already exists under the id of a node that is not registered
// is refused (a stale line, never silently replaced); a partial write is rolled back.
func (d *Deps) pin(id string, keys []remote.HostKey) error {
	have, err := d.pins().Lookup(pinAlias(id))
	if err != nil {
		return newErr(CodeHostKey, ReasonKeyMismatch, "cannot read the pinned host keys of %s", id)
	}
	if len(have) > 0 {
		return newErr(CodeHostKey, ReasonKeyMismatch, "a host key is already pinned for %s but the node is not registered; delete the %s line of the pin file first", id, pinAlias(id))
	}
	for _, k := range keys {
		if err := d.pins().Add(pinAlias(id), keyLine(k)); err != nil {
			_ = d.pins().Remove(pinAlias(id))
			return newErr(CodeHostKey, ReasonKeyMismatch, "cannot pin the host key of %s", id)
		}
	}
	return nil
}

// platform runs `uname -sm` on the node.
func (d *Deps) platform(ctx context.Context, t remote.Target) (Platform, error) {
	out, err := d.Run(ctx, t, "uname", "-sm")
	if err != nil {
		return Platform{}, transportErr("connect", "cannot run uname on the node", err)
	}
	return ParsePlatform(lastLine(out))
}

var versionRe = regexp.MustCompile(`^nself-ci-agent (\d+\.\d+\.\d+)(?: |$)`)

// agentVersion runs the installed agent's `version` and returns x.y.z.
func (d *Deps) agentVersion(ctx context.Context, t remote.Target) (string, error) {
	out, err := d.Run(ctx, t, agentPath, "version")
	if err != nil {
		return "", transportErr("agent", "the installed agent did not answer `version`", err)
	}
	m := versionRe.FindStringSubmatch(lastLine(out))
	if m == nil {
		return "", newErr(CodeNoRoute, "agent", "the installed agent printed an unexpected version line")
	}
	return m[1], nil
}

// localFileInstaller copies the verified local binary into ~/.nself/ci-agent/bin.
func localFileInstaller(ctx context.Context, d *Deps, r InstallRequest) error {
	if r.Source == nil {
		return usage("--agent-binary is required for the local-file installer")
	}
	path, cleanup, err := r.Source.Fetch(ctx, r.Platform)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	tmp := agentPath + ".new"
	steps := [][]string{{"mkdir", "-p", agentDir + "/bin"}, {"chmod", "700", agentDir}}
	for _, argv := range steps {
		if _, err = d.Run(ctx, r.Target, argv...); err != nil {
			return transportErr("install", "cannot prepare the agent directory", err)
		}
	}
	if err = d.Copy(ctx, r.Target, path, tmp); err != nil {
		return transportErr("install", "cannot copy the agent binary", err)
	}
	for _, argv := range [][]string{{"chmod", "755", tmp}, {"mv", "-f", tmp, agentPath}} {
		if _, err = d.Run(ctx, r.Target, argv...); err != nil {
			return transportErr("install", "cannot finish the agent installation", err)
		}
	}
	return nil
}
