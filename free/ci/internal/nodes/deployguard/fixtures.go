package deployguard

// Purpose: the fixture generator P7-TRUST-14's I15 test and this package's tests share: a
// `nself deploy targets --json` document, the matching `nself help --json` document, an ssh_config with an
// alias, ProxyJump and HostKeyAlias, a stub nself and host spellings.
// Inputs:  none (constants) or a list of hosts.
// Outputs: JSON documents, an ssh config text, a stub command runner and host spellings.
// Constraints: only 203.0.113.0/24, 2001:db8::/32 and *.example.test names; the fingerprint and machine id
//          are synthetic; no file and no process; names carry the Fixture prefix because they live in the
//          production package (the layers table has one row for it).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// The fixture deploy host in every spelling the boundary must catch.
const (
	FixtureHostV4      = "203.0.113.10"
	FixtureHostV6      = "2001:db8::10"
	FixtureHostName    = "deploy.example.test"
	FixtureAlias       = "ci-deploy-box"
	FixtureJumpHost    = "bastion.example.test"
	FixtureOther       = "203.0.113.99"
	FixtureFingerprint = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	FixtureMachineID   = "0123456789abcdef0123456789abcdef"
)

// FixtureTargets returns the `nself deploy targets --json` document for hosts (each becomes one target; the
// fixture fingerprint is attached to the first).
func FixtureTargets(hosts ...string) []byte {
	rows := []map[string]any{}
	for i, h := range hosts {
		fps := []string{}
		if i == 0 {
			fps = append(fps, FixtureFingerprint)
		}
		rows = append(rows, map[string]any{"env": "prod", "tier": "prod", "server": fmt.Sprintf("srv%d", i), "role": "app", "primary": i == 0,
			"user": nil, "host": h, "port": 22, "ssh_hostname": h, "addresses": []string{}, "host_key_fingerprints": fps, "arch": nil, "source": "inventory"})
	}
	b, _ := json.Marshal(map[string]any{"schema_version": "1", "command": "deploy targets", "data": map[string]any{"targets": rows, "warnings": []string{}}})
	return b
}

// FixtureHelp returns the `nself help --json deploy targets` document with the given side-effect class.
func FixtureHelp(sideEffect string) []byte {
	b, _ := json.Marshal(map[string]any{"schema_version": "1", "command": "help", "data": map[string]any{
		"commands": []map[string]any{{"path": "deploy targets", "name": "targets", "parent": "deploy", "side_effect": sideEffect}}}})
	return b
}

// FixtureRunner is a stub nself: help answers with helpSide, `deploy targets` with the document of hostsByRoot
// for the directory it runs in (an unknown directory fails). Calls records every argv joined by spaces.
type FixtureRunner struct {
	HelpSide    string
	HostsByRoot map[string][]string
	Calls       []string
}

// Run implements the command runner shape of deployguard.RunFunc.
func (r *FixtureRunner) Run(_ context.Context, dir string, argv ...string) ([]byte, int, error) {
	r.Calls = append(r.Calls, dir+"|"+strings.Join(argv, " "))
	if len(argv) >= 4 && argv[1] == "help" {
		return FixtureHelp(r.HelpSide), 0, nil
	}
	hosts, ok := r.HostsByRoot[dir]
	if !ok {
		return []byte("no project"), 1, nil
	}
	return FixtureTargets(hosts...), 0, nil
}

// FixtureSSHConfig is the ssh_config of the alias: a ProxyJump hop and a HostKeyAlias, resolving to FixtureHostV4.
func FixtureSSHConfig() string {
	return "Host " + FixtureAlias + "\n  FixtureHostName " + FixtureHostV4 + "\n  User ci\n  ProxyJump " + FixtureJumpHost + "\n  HostKeyAlias " + FixtureHostName + "\n"
}

// FixtureSpellings lists ways to write the fixture hosts: case, trailing dots, user@, port, brackets, mapped
// and shorthand forms.
func FixtureSpellings() []string {
	return []string{
		FixtureHostV4, "ci@" + FixtureHostV4, FixtureHostV4 + ":22", "ci@" + FixtureHostV4 + ":2222", "203.0.113.10.", "0xcb.0x00.0x71.0x0a", "3405803786", "0313.0.0161.012",
		"::ffff:203.0.113.10", "[::ffff:203.0.113.10]",
		FixtureHostV6, "[" + FixtureHostV6 + "]", "[" + FixtureHostV6 + "]:22", "ci@[" + FixtureHostV6 + "]:22", "2001:0db8:0000:0000:0000:0000:0000:0010", "2001:DB8::10", "2001:db8:0:0:0:0:0:10",
		FixtureHostName, strings.ToUpper(FixtureHostName), FixtureHostName + ".", "ci@" + FixtureHostName + ":22", "Ci@" + strings.ToUpper(FixtureHostName) + ".:2222",
	}
}
